package bridge

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

func event(kind, payload string) stream.Event {
	return stream.Event{Type: kind, Raw: []byte(payload)}
}

func textDelta(index int, text string) stream.Event {
	body, _ := json.Marshal(map[string]any{"item_id": "item_1", "content_index": index, "delta": text})
	return stream.Event{Type: codex.TextDelta, Raw: body}
}

func textDone(index int, text string) stream.Event {
	body, _ := json.Marshal(map[string]any{"item_id": "item_1", "content_index": index, "text": text})
	return stream.Event{Type: codex.TextDone, Raw: body}
}

// minimalRequest is the smallest request the decoder accepts. Its max_tokens is large
// enough that the completion check never fires by accident; the tests that mean to exercise
// that check set their own.
const minimalRequest = `{"model":"gpt-6-astra","max_tokens":100000,"stream":true,` +
	`"messages":[{"role":"user","content":"ping"}]}`

// run feeds a whole event sequence and collects the frames it produced.
func run(t *testing.T, events ...stream.Event) ([]anthropic.Frame, error) {
	t.Helper()
	translator := NewTranslatorFor(decodeRequest(t, minimalRequest))
	var frames []anthropic.Frame
	for _, e := range events {
		produced, err := translator.Accept(e)
		if err != nil {
			return frames, err
		}
		frames = append(frames, produced...)
	}
	return frames, nil
}

func frameTypes(frames []anthropic.Frame) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = f.Type
	}
	return out
}

func joined(frames []anthropic.Frame) string {
	var b strings.Builder
	for _, f := range frames {
		f.WriteTo(&b)
	}
	return b.String()
}

// The backend reports what it spent on every completion, and the caller's max_tokens is
// checked against that count. A completion without it is its own case, below.
const usageReported = `"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}`

const completedOK = `{"type":"response.completed","response":{"id":"resp_1",` + usageReported + `}}`

const completedNoUsage = `{"type":"response.completed","response":{"id":"resp_1"}}`

// completedWith builds a completed payload carrying the given output items.
func completedWith(items ...string) stream.Event {
	return stream.Event{Type: codex.Completed, Raw: []byte(
		`{"type":"response.completed","response":{"id":"resp_1",` + usageReported + `,"output":[` +
			strings.Join(items, ",") + `]}}`)}
}

// callable builds a request whose tool definitions make the given names callable.
func callable(names ...string) *anthropic.Request {
	body := `{"model":"gpt-6-astra","max_tokens":100000,"stream":true,
	  "messages":[{"role":"user","content":"x"}],"tools":[`
	for i, name := range names {
		if i > 0 {
			body += ","
		}
		body += `{"name":"` + name + `","input_schema":{"type":"object"}}`
	}
	body += `]}`
	request, err := anthropic.DecodeRequest([]byte(body))
	if err != nil {
		panic(err)
	}
	return request
}

// runFor feeds events through a translator that knows what is callable.
func runFor(t *testing.T, request *anthropic.Request, events ...stream.Event) ([]anthropic.Frame, error) {
	t.Helper()
	translator := NewTranslatorFor(request)
	var frames []anthropic.Frame
	for _, e := range events {
		produced, err := translator.Accept(e)
		if err != nil {
			return frames, err
		}
		frames = append(frames, produced...)
	}
	return frames, nil
}

// The ordinary text round trip, end to end through the two vocabularies.
func TestTextResponseProducesTheClientSequence(t *testing.T) {
	frames, err := run(t,
		event(codex.Created, `{"type":"response.created","response":{"id":"resp_abc"}}`),
		event(codex.InProgress, `{"type":"response.in_progress"}`),
		event(codex.OutputItemAdd, `{"type":"response.output_item.added"}`),
		event(codex.ContentPartAdd, `{"type":"response.content_part.added"}`),
		textDelta(0, "Hel"),
		textDelta(0, "lo"),
		textDone(0, "Hello"),
		event(codex.Completed, `{"type":"response.completed","response":{"id":"resp_abc","usage":{"input_tokens":12,"output_tokens":3}}}`),
	)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	want := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta",
		"content_block_stop",
		"message_delta", "message_stop",
	}
	if got := frameTypes(frames); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("frames\n got: %v\nwant: %v", got, want)
	}

	text := joined(frames)
	if !strings.Contains(text, `"id":"resp_abc"`) {
		t.Error("the backend's response id did not reach message_start")
	}
	if !strings.Contains(text, `"input_tokens":12`) || !strings.Contains(text, `"output_tokens":3`) {
		t.Errorf("usage did not reach message_delta:\n%s", text)
	}
}

// Every frame must be a single well-formed SSE event whose data parses and whose declared
// type matches the event name. A client reads both.
func TestFramesAreWellFormedSSE(t *testing.T) {
	frames, err := run(t,
		textDelta(0, "x"), textDone(0, "x"), event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	for _, frame := range frames {
		var body strings.Builder
		frame.WriteTo(&body)
		text := body.String()
		if !strings.HasPrefix(text, "event: "+frame.Type+"\ndata: ") || !strings.HasSuffix(text, "\n\n") {
			t.Errorf("%s framing = %q", frame.Type, text)
		}
		if strings.Count(text, "\n\n") != 1 {
			t.Errorf("%s contains an interior blank line, which would split the frame: %q", frame.Type, text)
		}
		var decoded struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(frame.Data, &decoded); err != nil {
			t.Errorf("%s data is not JSON: %v", frame.Type, err)
			continue
		}
		if decoded.Type != frame.Type {
			t.Errorf("event name %q disagrees with payload type %q", frame.Type, decoded.Type)
		}
	}
}

// WIRE08. The backend's snapshot exists to be compared against what the deltas built. If
// they disagree a delta went missing, and neither version can be trusted.
func TestSnapshotMustMatchTheDeltas(t *testing.T) {
	t.Run("matching is accepted", func(t *testing.T) {
		if _, err := run(t, textDelta(0, "ab"), textDelta(0, "cd"), textDone(0, "abcd")); err != nil {
			t.Fatalf("Accept: %v", err)
		}
	})
	for name, events := range map[string][]stream.Event{
		"snapshot longer than the deltas":  {textDelta(0, "ab"), textDone(0, "abcd")},
		"snapshot shorter than the deltas": {textDelta(0, "abcd"), textDone(0, "ab")},
		"snapshot differs entirely":        {textDelta(0, "abcd"), textDone(0, "wxyz")},
		"a delta went missing":             {textDelta(0, "a"), textDelta(0, "c"), textDone(0, "abc")},
		"non-empty snapshot, no deltas":    {textDone(0, "text the client never saw")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, events...); !errors.Is(err, anthropic.ErrTextMismatch) {
				t.Fatalf("err = %v, want TEXT_MISMATCH", err)
			}
		})
	}
}

// The one allowed case, and the reason it is allowed: the baseline once failed on an empty
// done with no deltas, and a part that legitimately produced nothing is not a mismatch.
//
// It is not a mismatch and it is also not an answer. The empty part is accepted here; the
// response it belongs to is refused at completion for having produced nothing at all.
func TestEmptySnapshotWithNoDeltasIsNotAMismatch(t *testing.T) {
	_, err := run(t, textDone(0, ""), event(codex.Completed, completedOK))
	if errors.Is(err, anthropic.ErrTextMismatch) {
		t.Fatalf("an empty part with no deltas was read as a mismatch: %v", err)
	}
	if !errors.Is(err, anthropic.ErrEmptyReply) {
		t.Fatalf("err = %v, want EMPTY_REPLY", err)
	}
}

// WIRE12. Reasoning arriving before the text must not disturb what the client sees. The
// failure this guards against is the final text going missing behind other blocks.
func TestReasoningBeforeTextDoesNotDisplaceTheAnswer(t *testing.T) {
	frames, err := run(t,
		event(codex.Created, `{"type":"response.created","response":{"id":"r"}}`),
		event(codex.ReasoningPartAdd, `{"type":"response.reasoning_part.added"}`),
		event(codex.ReasoningSummaryTxtD, `{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`),
		event(codex.ReasoningSummaryTxtF, `{"type":"response.reasoning_summary_text.done"}`),
		event(codex.ReasoningPartDone, `{"type":"response.reasoning_part.done"}`),
		textDelta(0, "the answer"),
		textDone(0, "the answer"),
		event(codex.Completed, completedOK),
	)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	want := []string{"message_start", "content_block_start", "content_block_delta",
		"content_block_stop", "message_delta", "message_stop"}
	if got := frameTypes(frames); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("frames\n got: %v\nwant: %v", got, want)
	}
	if !strings.Contains(joined(frames), "the answer") {
		t.Fatal("the answer did not reach the client")
	}
}

// Reasoning content itself is never carried. Passing the backend's private work off as
// Anthropic reasoning would be inventing a feature.
func TestReasoningContentIsNotEmitted(t *testing.T) {
	frames, err := run(t,
		event(codex.ReasoningSummaryTxtD, `{"type":"response.reasoning_summary_text.delta","delta":"SECRET-REASONING"}`),
		textDelta(0, "answer"), textDone(0, "answer"),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if strings.Contains(joined(frames), "SECRET-REASONING") {
		t.Fatal("reasoning content reached the client")
	}
}

// An event type nobody recognises may be the one carrying the result. Skipping it produces
// a response that is short by exactly the part nobody read.
func TestUnknownEventIsRefused(t *testing.T) {
	for _, kind := range []string{
		"response.output_audio.delta",
		"thread.started",
		"",
		"response.",
		// Still refused on purpose. Each carries content this build does not support, so
		// accepting and dropping one would produce a reply short by exactly that part.
		"response.refusal.delta",
		"response.output_text.annotation.added",
		"response.custom_tool_call_input.delta",
		"item.started",
	} {
		if _, err := run(t, event(kind, `{"type":"x"}`)); !errors.Is(err, ErrUnsupportedEvent) {
			t.Errorf("%q: err = %v, want UNSUPPORTED_EVENT", kind, err)
		}
	}
}

// A terminal failure is terminal the moment it arrives, with its own category.
func TestUpstreamFailuresAreTerminalAndNamed(t *testing.T) {
	for kind, want := range map[string]error{
		codex.Failed:     codex.ErrResponseFailed,
		codex.Incomplete: codex.ErrResponseIncomplt,
		codex.ErrorEvent: codex.ErrErrorEvent,
	} {
		_, err := run(t, textDelta(0, "partial"), event(kind, `{"type":"`+kind+`","error":{"message":"UPSTREAM-DETAIL"}}`))
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", kind, err, want)
		}
	}
}

// A malformed payload is a shape error, not a silent skip.
func TestMalformedPayloadsAreRefused(t *testing.T) {
	for name, e := range map[string]stream.Event{
		"delta missing item_id":    event(codex.TextDelta, `{"content_index":0,"delta":"x"}`),
		"delta missing delta":      event(codex.TextDelta, `{"item_id":"i","content_index":0}`),
		"delta index is a string":  event(codex.TextDelta, `{"item_id":"i","content_index":"0","delta":"x"}`),
		"delta index is a float":   event(codex.TextDelta, `{"item_id":"i","content_index":0.5,"delta":"x"}`),
		"done missing text":        event(codex.TextDone, `{"item_id":"i","content_index":0}`),
		"created missing response": event(codex.Created, `{"type":"response.created"}`),
		"created missing id":       event(codex.Created, `{"response":{}}`),
		"duplicate key in payload": event(codex.TextDelta, `{"item_id":"i","item_id":"j","content_index":0,"delta":"x"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, e); err == nil {
				t.Fatal("accepted a malformed payload")
			}
		})
	}
}

// An empty delta is a real delta: the backend said something happened and produced no
// characters. Absent is a different thing and is refused above.
func TestEmptyDeltaIsCarried(t *testing.T) {
	frames, err := run(t, textDelta(0, ""), textDone(0, ""), event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := frameTypes(frames); strings.Join(got, ",") !=
		"message_start,content_block_start,content_block_delta,content_block_stop,message_delta,message_stop" {
		t.Fatalf("frames = %v", got)
	}
}

// A response that produced neither text nor a call produced nothing. Handing the client an
// empty assistant message would make that failure look like the model having said nothing,
// which is a plausible answer rather than the failure it is.
func TestEmptyResponseIsRefused(t *testing.T) {
	_, err := run(t, event(codex.Completed, completedOK))
	if !errors.Is(err, anthropic.ErrEmptyReply) {
		t.Fatalf("err = %v, want EMPTY_REPLY", err)
	}
}

// A response carrying only a tool call has produced something, so it is not empty.
func TestToolOnlyResponseIsNotEmpty(t *testing.T) {
	frames, err := runFor(t, callable("Read"),
		event(codex.Created, `{"type":"response.created","response":{"id":"r"}}`),
		completedWith(`{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}`))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	want := "message_start,content_block_start,content_block_delta,content_block_stop,message_delta,message_stop"
	if got := frameTypes(frames); strings.Join(got, ",") != want {
		t.Fatalf("frames = %v", got)
	}
}

// A block left open at completion is closed first, so a client reading sequentially never
// sees a message end with a block outstanding.
func TestOpenBlocksAreClosedAtCompletion(t *testing.T) {
	frames, err := run(t, textDelta(0, "unfinished"), event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	types := frameTypes(frames)
	stopAt, deltaAt := -1, -1
	for i, kind := range types {
		if kind == "content_block_stop" {
			stopAt = i
		}
		if kind == "message_delta" {
			deltaAt = i
		}
	}
	if stopAt < 0 || deltaAt < 0 || stopAt > deltaAt {
		t.Fatalf("content_block_stop must precede message_delta: %v", types)
	}
}

// The caller's max_tokens never reaches the backend, so it is checked at completion
// against the count the backend reports. A completion that reports no count leaves nothing
// to check it against, and reporting success would be claiming a check that never ran.
func TestACompletionWithNoUsageIsRefused(t *testing.T) {
	_, err := run(t, textDelta(0, "x"), textDone(0, "x"), event(codex.Completed, completedNoUsage))
	if !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("err = %v, want %v", err, ErrUsageUnknown)
	}
}

// A response larger than the caller allowed is refused rather than trimmed. Nothing asked
// the backend to stop, so by the time this is known the tokens are already spent — but
// handing the client more than it asked for would be answering a different request.
func TestAResponseOverTheCallerLimitIsRefused(t *testing.T) {
	translator := NewTranslatorFor(decodeRequest(t,
		`{"model":"m","max_tokens":2,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	for _, e := range []stream.Event{textDelta(0, "x"), textDone(0, "x")} {
		if _, err := translator.Accept(e); err != nil {
			t.Fatalf("Accept: %v", err)
		}
	}
	// The fixture reports three output tokens against a limit of two.
	_, err := translator.Accept(event(codex.Completed, completedOK))
	if !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("err = %v, want %v", err, ErrOutputLimitExceeded)
	}
}

// Exactly at the limit is within it. An off-by-one here refuses a response the caller asked
// for and paid for.
func TestAResponseExactlyAtTheLimitIsAccepted(t *testing.T) {
	translator := NewTranslatorFor(decodeRequest(t,
		`{"model":"m","max_tokens":3,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	for _, e := range []stream.Event{textDelta(0, "x"), textDone(0, "x")} {
		if _, err := translator.Accept(e); err != nil {
			t.Fatalf("Accept: %v", err)
		}
	}
	if _, err := translator.Accept(event(codex.Completed, completedOK)); err != nil {
		t.Fatalf("a response exactly at the limit was refused: %v", err)
	}
}

// Unknown counts stay unknown. Writing 0 would turn "we do not know what this cost" into
// "it cost nothing", and a budget built on that number would be wrong in the cheap
// direction. Output is reported here because the limit check needs it; input is not.
func TestUnknownUsageIsOmittedNotZeroed(t *testing.T) {
	frames, err := run(t, textDelta(0, "x"), textDone(0, "x"), event(codex.Completed,
		`{"type":"response.completed","response":{"id":"resp_1","usage":{"output_tokens":3}}}`))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	for _, frame := range frames {
		if frame.Type != "message_delta" {
			continue
		}
		var body struct {
			Usage map[string]any `json:"usage"`
		}
		if err := json.Unmarshal(frame.Data, &body); err != nil {
			t.Fatalf("message_delta: %v", err)
		}
		if _, present := body.Usage["input_tokens"]; present {
			t.Fatalf("usage = %v, want no input count when the backend reported none", body.Usage)
		}
		return
	}
	t.Fatal("no message_delta frame")
}

// --- request construction -------------------------------------------------------------

func decodeRequest(t *testing.T, body string) *anthropic.Request {
	t.Helper()
	request, err := anthropic.DecodeRequest([]byte(body))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	return request
}

// parts reads a conversation turn's content. The system turn carries a string instead, so
// asking for parts where there are none is itself the assertion.
func parts(t *testing.T, entry InputEntry) []InputPart {
	t.Helper()
	got, ok := entry.Content.([]InputPart)
	if !ok {
		t.Fatalf("turn content = %T (%v), want []InputPart", entry.Content, entry.Content)
	}
	return got
}

func TestBuildRequestCarriesTheConversation(t *testing.T) {
	request := decodeRequest(t, `{"model":"gpt-6-astra","max_tokens":2048,"stream":true,
	  "system":[{"type":"text","text":"You are Claude Code."}],
	  "output_config":{"effort":"high"},
	  "messages":[
	    {"role":"user","content":"first"},
	    {"role":"assistant","content":[{"type":"text","text":"reply"}]},
	    {"role":"user","content":"second"}]}`)

	out, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if out.Model != "gpt-6-astra" || !out.Stream {
		t.Fatalf("envelope = %+v", out)
	}
	if out.Instruction != Instruction {
		t.Fatalf("instructions = %q, want the fixed string", out.Instruction)
	}
	if out.Effort == nil || out.Effort.Effort != "high" {
		t.Fatalf("effort = %+v", out.Effort)
	}
	// Four entries: the system prompt leads as a developer turn, then the three messages.
	if len(out.Input) != 4 {
		t.Fatalf("input = %d entries, want 4", len(out.Input))
	}
	if out.Input[0].Role != "developer" || out.Input[0].Content != "You are Claude Code." {
		t.Fatalf("system turn = %+v", out.Input[0])
	}
	// The backend distinguishes text the user supplied from text the model produced.
	if parts(t, out.Input[1])[0].Type != "input_text" || parts(t, out.Input[2])[0].Type != "output_text" {
		t.Fatalf("part types = %q %q",
			parts(t, out.Input[1])[0].Type, parts(t, out.Input[2])[0].Type)
	}
	if parts(t, out.Input[3])[0].Text != "second" {
		t.Fatalf("last turn = %q", parts(t, out.Input[3])[0].Text)
	}
}

// The client's max_tokens never reaches the backend. Sending it was measured to be
// rejected, and the baseline has never sent it.
func TestTheOutputLimitIsNotSentUpstream(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t,
		`{"model":"m","max_tokens":2048,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	encoded, _ := json.Marshal(out)
	for _, forbidden := range []string{"max_output_tokens", "max_tokens", "2048"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("the backend request carries %q: %s", forbidden, encoded)
		}
	}
	// And the two settings that are always sent, are.
	for _, want := range []string{`"include":["reasoning.encrypted_content"]`, `"store":false`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s: %s", want, encoded)
		}
	}
}

// A string system prompt and a block-array one must reach the backend the same way.
func TestSystemPromptShapesAgree(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],"system":`
	fromString, err := BuildRequest(decodeRequest(t, head+`"instructions"}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	fromBlocks, err := BuildRequest(decodeRequest(t, head+`[{"type":"text","text":"instructions"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	// Both arrive as the leading developer turn, not as the top-level instructions.
	if fromString.Instruction != Instruction || fromBlocks.Instruction != Instruction {
		t.Fatalf("instructions = %q / %q, want the fixed string",
			fromString.Instruction, fromBlocks.Instruction)
	}
	if !reflect.DeepEqual(fromString.Input[0], fromBlocks.Input[0]) {
		t.Fatalf("%+v vs %+v", fromString.Input[0], fromBlocks.Input[0])
	}
	if fromString.Input[0].Role != "developer" || fromString.Input[0].Content != "instructions" {
		t.Fatalf("system turn = %+v", fromString.Input[0])
	}
}

// No effort asked for means no reasoning parameter sent. An omitted choice must not become
// a default the caller never made.
func TestAbsentEffortSendsNoReasoningParameter(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t,
		`{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if out.Effort != nil {
		t.Fatalf("effort = %+v, want nil", out.Effort)
	}
	encoded, _ := json.Marshal(out)
	// "reasoning" also appears inside include, which is sent on every request. The field
	// this must not gain is the parameter.
	if strings.Contains(string(encoded), `"reasoning":`) {
		t.Fatalf("an absent effort became a field: %s", encoded)
	}
}

// A tool request streams its arguments before the call exists. This build does not build
// calls from those events -- a call comes only from response.completed -- but they arrive on
// every tool request, and refusing one kills the request.
//
// Measured 2026-09-15 against the real backend: a request carrying a tool definition was
// refused as UNSUPPORTED_EVENT before any call came back. The test above previously listed
// response.function_call_arguments.delta as an event that should be refused, and it passed.
// A green test asserting the defect is what offline checking looks like when it agrees with
// itself.
func TestStreamedToolArgumentsAreAccountedForNotRefused(t *testing.T) {
	translator := NewTranslatorFor(callable("Read"))

	// Every streaming argument event, and not one client frame. This is the delivery
	// barrier stated as a measurement: before response.completed there is nothing to
	// deliver, so a stream that failed here could not have handed anything to execute.
	for _, e := range []stream.Event{
		argsDelta("fc_1", `{"file_`),
		argsDelta("fc_1", `path":"a.txt"}`),
		argsDone("fc_1", `{"file_path":"a.txt"}`),
	} {
		frames, err := translator.Accept(e)
		if err != nil {
			t.Fatalf("Accept(%s): %v", e.Type, err)
		}
		if len(frames) != 0 {
			t.Fatalf("%s produced %d client frames before completion: %v", e.Type, len(frames), frames)
		}
	}

	// And then the call arrives whole, from the completed output.
	frames, err := translator.Accept(
		completedWith(functionCall("call_1", "Read", `{"file_path":"a.txt"}`)))
	if err != nil {
		t.Fatalf("Accept(completed): %v", err)
	}
	whole := false
	for _, frame := range frames {
		if strings.Contains(string(frame.Data), `{\"file_path\":\"a.txt\"}`) {
			whole = true
		}
	}
	if !whole {
		t.Fatalf("the call's arguments never reached the client: %v", frames)
	}
}

// The backend's finished arguments against what it streamed. A tool call is executed by the
// client, so two accounts disagreeing is where this stops rather than picking one.
func TestArgumentsThatDisagreeWithTheStreamAreRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		deltas []string
		done   string
	}{
		"a delta went missing": {[]string{`{"file_`}, `{"file_path":"a.txt"}`},
		"a delta was invented": {[]string{`{"file_path":"a.txt"}`, `extra`}, `{"file_path":"a.txt"}`},
		"the value differs":    {[]string{`{"file_path":"b.txt"}`}, `{"file_path":"a.txt"}`},
		"nothing streamed":     {nil, `{"file_path":"a.txt"}`},
		"whitespace differs":   {[]string{`{"file_path": "a.txt"}`}, `{"file_path":"a.txt"}`},
	} {
		t.Run(name, func(t *testing.T) {
			events := []stream.Event{}
			for _, delta := range tc.deltas {
				events = append(events, argsDelta("fc_1", delta))
			}
			events = append(events, argsDone("fc_1", tc.done))
			if _, err := run(t, events...); !errors.Is(err, ErrArgumentsMismatch) {
				t.Fatalf("err = %v, want %v", err, ErrArgumentsMismatch)
			}
		})
	}
}

// Two calls stream at once and their fragments interleave. Keying by item is what keeps one
// call's arguments from being checked against another's.
func TestInterleavedArgumentStreamsStayApart(t *testing.T) {
	_, err := runFor(t, callable("Read"),
		argsDelta("fc_1", `{"a":`), argsDelta("fc_2", `{"b":`),
		argsDelta("fc_1", `1}`), argsDelta("fc_2", `2}`),
		argsDone("fc_1", `{"a":1}`), argsDone("fc_2", `{"b":2}`),
		completedWith(
			functionCall("call_1", "Read", `{"a":1}`),
			functionCall("call_2", "Read", `{"b":2}`)))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

// Informational events carry no part of the answer, so ignoring one cannot make a reply
// short by the part nobody read -- which is the reason the default is to refuse.
func TestInformationalEventsAreKnownAndCarryNothing(t *testing.T) {
	for _, kind := range []string{
		codex.RateLimitsUpdated, codex.CodexRateLimits,
		codex.CodexMetadata, codex.WebsocketTiming,
		codex.ReasoningTextDelta, codex.ReasoningTextDone,
	} {
		t.Run(kind, func(t *testing.T) {
			frames, err := run(t,
				event(kind, `{"type":"x","anything":1}`),
				textDelta(0, "ok"), textDone(0, "ok"),
				event(codex.Completed, completedOK))
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}
			for _, frame := range frames {
				if strings.Contains(string(frame.Data), "anything") {
					t.Fatalf("%s reached the client: %s", kind, frame.Data)
				}
			}
		})
	}
}

func argsDelta(itemID, delta string) stream.Event {
	body, _ := json.Marshal(map[string]any{
		"type": codex.FuncArgsDelta, "item_id": itemID, "delta": delta})
	return stream.Event{Type: codex.FuncArgsDelta, Raw: body}
}

func argsDone(itemID, arguments string) stream.Event {
	body, _ := json.Marshal(map[string]any{
		"type": codex.FuncArgsDone, "item_id": itemID, "arguments": arguments})
	return stream.Event{Type: codex.FuncArgsDone, Raw: body}
}
