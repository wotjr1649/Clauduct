package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestDisplayOnlyHistoryEncodesAnEmptyInputArray(t *testing.T) {
	request, err := BuildRequest(&anthropic.Request{Model: "gpt-6-astra"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(request)
	if err != nil || !strings.Contains(string(raw), `"input":[]`) {
		t.Fatal("empty display-only input must not be null")
	}
}

// defaultItem is the item id the text helpers stream under.
//
// It matches the id the message item fixtures carry, which is not a detail: a delta names
// the item it belongs to, and the two agreeing is what the backend actually does. The
// fixtures disagreed for as long as nothing compared them -- item_id was "item_1" while
// the item said "msg_1" -- and the check that now reads both found it immediately.
const defaultItem = "msg_1"

func textDelta(text string) stream.Event {
	return textDeltaFor(defaultItem, text)
}

func textDone(text string) stream.Event {
	return textDoneFor(defaultItem, 0, text)
}

func textDeltaFor(item, text string) stream.Event {
	body, _ := json.Marshal(map[string]any{"item_id": item, "content_index": 0, "delta": text})
	return stream.Event{Type: codex.TextDelta, Raw: body}
}

func textDoneFor(item string, index int, text string) stream.Event {
	body, _ := json.Marshal(map[string]any{"item_id": item, "content_index": index, "text": text})
	return stream.Event{Type: codex.TextDone, Raw: body}
}

// minimalRequest is the smallest request the decoder accepts. Its max_tokens is large
// enough that the completion check never fires by accident; the tests that mean to exercise
// that check set their own.
const minimalRequest = `{"model":"gpt-6-astra","max_tokens":100000,"stream":true,` +
	`"messages":[{"role":"user","content":"ping"}]}`

// run feeds a whole event sequence and collects the frames it produced.
func run(t *testing.T, parts ...any) ([]anthropic.Frame, error) {
	t.Helper()
	return runFor(t, decodeRequest(t, minimalRequest), parts...)
}

// flatten lets a fixture be one event or a sequence of them. A completed response is
// several events on the wire, and a helper that can only return one cannot reproduce it.
func flatten(t *testing.T, parts []any) []stream.Event {
	t.Helper()
	var events []stream.Event
	for _, part := range parts {
		switch value := part.(type) {
		case stream.Event:
			events = append(events, value)
		case []stream.Event:
			events = append(events, value...)
		default:
			t.Fatalf("not an event or a sequence of them: %T", part)
		}
	}
	return events
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

// completedWith produces the events a backend actually sends to deliver items.
//
// It used to build one response.completed carrying an output array. Measured 2026-09-15:
// this backend's completed output array is empty on every response, and items arrive as
// output_item.added / output_item.done around the stream. The old fixture agreed with the
// decoder that read it and with the tests that checked it -- three layers, one wrong
// premise -- so it is now the real shape.
func completedWith(items ...string) []stream.Event {
	var events []stream.Event
	for i, item := range items {
		events = append(events, itemAdded(i, openingOf(item)), itemDone(i, item))
	}
	return append(events, stream.Event{Type: codex.Completed, Raw: []byte(
		`{"type":"response.completed","response":{"id":"resp_1",` + usageReported + `,"output":[]}}`)})
}

// completedStating builds a completion that does state its output, which this backend never
// does. It exists to check that a backend contradicting its own stream is refused.
func completedStating(items ...string) stream.Event {
	return stream.Event{Type: codex.Completed, Raw: []byte(
		`{"type":"response.completed","response":{"id":"resp_1",` + usageReported + `,"output":[` +
			strings.Join(items, ",") + `]}}`)}
}

// messageItem is the backend's finished account of a text item.
func messageItem(id string, texts ...string) string {
	parts := make([]any, 0, len(texts))
	for _, text := range texts {
		parts = append(parts, map[string]any{"type": "output_text", "text": text})
	}
	encoded, _ := json.Marshal(map[string]any{"id": id, "type": "message", "content": parts})
	return string(encoded)
}

func itemAdded(index int, item string) stream.Event {
	return stream.Event{Type: codex.OutputItemAdd, Raw: []byte(fmt.Sprintf(
		`{"type":"response.output_item.added","output_index":%d,"item":%s}`, index, item))}
}

func itemDone(index int, item string) stream.Event {
	return stream.Event{Type: codex.OutputItemDone, Raw: []byte(fmt.Sprintf(
		`{"type":"response.output_item.done","output_index":%d,"item":%s}`, index, item))}
}

// openingOf is the same item as the backend first announces it: identity, and none of the
// content that has not been written yet.
func openingOf(item string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(item), &fields) != nil {
		return item
	}
	opening := map[string]json.RawMessage{}
	for _, name := range []string{"id", "type"} {
		if value, ok := fields[name]; ok {
			opening[name] = value
		}
	}
	encoded, _ := json.Marshal(opening)
	return string(encoded)
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
func runFor(t *testing.T, request *anthropic.Request, parts ...any) ([]anthropic.Frame, error) {
	t.Helper()
	return runForOn(t, request, "", parts...)
}

// runForOn is the same with the effective model stated, which is what the gateway does.
//
// runFor passes an empty one, and for a while that was the only way these frames were ever
// produced in a test -- so TestTheReplyNamesTheRequestedModelNotTheRoutedOne asserted an
// invariant the product had stopped holding and went on passing, because the path it drove
// could not carry the value that would have broken it.
func runForOn(t *testing.T, request *anthropic.Request, effective string, parts ...any) ([]anthropic.Frame, error) {
	t.Helper()
	translator := NewTranslatorFor(request, effective)
	var frames []anthropic.Frame
	for _, e := range flatten(t, parts) {
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
		itemAdded(0, `{"id":"msg_1","type":"message"}`),
		event(codex.ContentPartAdd, `{"type":"response.content_part.added"}`),
		textDelta("Hel"),
		textDelta("lo"),
		textDone("Hello"),
		// The item closes before the response does, and its finished text is checked
		// against what streamed. Against the real backend this check had never run: it
		// only fired for a message item in the completed output, and none ever arrives.
		itemDone(0, messageItem("msg_1", "Hello")),
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
		textDelta("x"), textDone("x"), event(codex.Completed, completedOK))
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
		if _, err := run(t, textDelta("ab"), textDelta("cd"), textDone("abcd")); err != nil {
			t.Fatalf("Accept: %v", err)
		}
	})
	for name, events := range map[string][]any{
		"snapshot longer than the deltas":  {textDelta("ab"), textDone("abcd")},
		"snapshot shorter than the deltas": {textDelta("abcd"), textDone("ab")},
		"snapshot differs entirely":        {textDelta("abcd"), textDone("wxyz")},
		"a delta went missing":             {textDelta("a"), textDelta("c"), textDone("abc")},
		"non-empty snapshot, no deltas":    {textDone("text the client never saw")},
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
	_, err := run(t, textDone(""), event(codex.Completed, completedOK))
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
		textDelta("the answer"),
		textDone("the answer"),
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
		textDelta("answer"), textDone("answer"),
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
		_, err := run(t, textDelta("partial"), event(kind, `{"type":"`+kind+`","error":{"message":"UPSTREAM-DETAIL"}}`))
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
	frames, err := run(t, textDelta(""), textDone(""), event(codex.Completed, completedOK))
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
	frames, err := run(t, textDelta("unfinished"), event(codex.Completed, completedOK))
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
	_, err := run(t, textDelta("x"), textDone("x"), event(codex.Completed, completedNoUsage))
	if !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("err = %v, want %v", err, ErrUsageUnknown)
	}
}

// A response larger than the caller allowed is refused rather than trimmed. Since #309 the
// generation request also carries the cap, so the backend normally stops first; this is the
// check for a backend that reports more anyway -- handing the client more than it asked for
// would be answering a different request.
func TestAResponseOverTheCallerLimitIsRefused(t *testing.T) {
	translator := NewTranslatorFor(decodeRequest(t,
		`{"model":"sonnet","max_tokens":2,"stream":true,"messages":[{"role":"user","content":"x"}]}`), "")
	for _, e := range []stream.Event{textDelta("x"), textDone("x")} {
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
		`{"model":"sonnet","max_tokens":3,"stream":true,"messages":[{"role":"user","content":"x"}]}`), "")
	for _, e := range []stream.Event{textDelta("x"), textDone("x")} {
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
	frames, err := run(t, textDelta("x"), textDone("x"), event(codex.Completed,
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

// #91: the backend counts cached input inside input_tokens; Anthropic reports it beside, as
// cache_read_input_tokens. Passed on whole, the client saw every cached token as fresh input
// and no cache reads at all. The total is unchanged.
func TestCachedInputIsReportedAsCacheReads(t *testing.T) {
	frames, err := run(t, textDelta("x"), textDone("x"), event(codex.Completed,
		`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":16865,`+
			`"input_tokens_details":{"cached_tokens":3840},"output_tokens":3}}}`))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	for _, frame := range frames {
		if frame.Type != "message_delta" {
			continue
		}
		var body struct {
			Usage map[string]int64 `json:"usage"`
		}
		if err := json.Unmarshal(frame.Data, &body); err != nil {
			t.Fatalf("message_delta: %v", err)
		}
		if body.Usage["input_tokens"] != 13025 || body.Usage["cache_read_input_tokens"] != 3840 {
			t.Fatalf("usage = %v, want input 13025 and cache reads 3840", body.Usage)
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
	// No top-level instructions: the system prompt is the developer turn below (#144).
	if encoded, _ := json.Marshal(out); strings.Contains(string(encoded), `"instructions"`) {
		t.Fatalf("top-level instructions sent: %s", encoded)
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

// #309: the client's max_tokens becomes the generation request's max_output_tokens, which the
// backend honours since 2026-10-05 -- but not when the client turned thinking off, where the
// completion check leaves reasoning out and a cap would cut the backend's reasoning short of
// any answer. The built request itself, which a count shares, never carries it.
func TestTheOutputCapFollowsMaxTokensUnlessThinkingIsOff(t *testing.T) {
	for _, c := range []struct {
		body string
		want int64
	}{
		{`{"model":"sonnet","max_tokens":2048,"stream":true,"messages":[{"role":"user","content":"x"}]}`, 2048},
		{`{"model":"sonnet","max_tokens":64,"thinking":{"type":"disabled"},"stream":true,"messages":[{"role":"user","content":"x"}]}`, 0},
	} {
		request := decodeRequest(t, c.body)
		if got := OutputCap(request); got != c.want {
			t.Fatalf("OutputCap = %d, want %d", got, c.want)
		}
		out, err := BuildRequest(request)
		if err != nil {
			t.Fatalf("BuildRequest: %v", err)
		}
		encoded, _ := json.Marshal(out)
		if strings.Contains(string(encoded), "max_output_tokens") || strings.Contains(string(encoded), `"max_tokens"`) {
			t.Fatalf("the shared request carries an output limit: %s", encoded)
		}
		out.MaxOutputTokens = OutputCap(request)
		encoded, _ = json.Marshal(out)
		if c.want != 0 && !strings.Contains(string(encoded), `"max_output_tokens":2048`) {
			t.Fatalf("the cap did not encode: %s", encoded)
		}
	}
	out, err := BuildRequest(decodeRequest(t,
		`{"model":"sonnet","max_tokens":2048,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	encoded, _ := json.Marshal(out)
	// And the two settings that are always sent, are.
	for _, want := range []string{`"include":["reasoning.encrypted_content"]`, `"store":false`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s: %s", want, encoded)
		}
	}
}

// A string system prompt and a block-array one must reach the backend the same way.
func TestSystemPromptShapesAgree(t *testing.T) {
	head := `{"model":"sonnet","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],"system":`
	fromString, err := BuildRequest(decodeRequest(t, head+`"instructions"}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	fromBlocks, err := BuildRequest(decodeRequest(t, head+`[{"type":"text","text":"instructions"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	// Both arrive as the leading developer turn.
	if !reflect.DeepEqual(fromString.Input[0], fromBlocks.Input[0]) {
		t.Fatalf("%+v vs %+v", fromString.Input[0], fromBlocks.Input[0])
	}
	if fromString.Input[0].Role != "developer" || fromString.Input[0].Content != "instructions" {
		t.Fatalf("system turn = %+v", fromString.Input[0])
	}
}

// An effort the caller did not name is the model's own, not the backend's choice.
//
// This test used to assert the opposite -- that an absent effort sends no reasoning
// parameter. That was never the baseline's behaviour: the catalogue supplies a default, so
// src/native-protocol.mjs:429 sends reasoning.effort unconditionally and the backend is
// never left to pick. The fixture named a model that did not exist, which is how the
// question went unasked.
func TestAnAbsentEffortComesFromTheModelNotTheBackend(t *testing.T) {
	for name, tc := range map[string]struct{ model, want string }{
		"sonnet runs terra at its own default": {"sonnet", "high"},
		"haiku runs luna at its own default":   {"haiku", "max"},
		"opus runs sol at its own default":     {"opus", "xhigh"},
		"fable runs astra at its own default":  {"fable", "medium"},
		"a versioned id resolves the same":     {"claude-opus-5", "xhigh"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := BuildRequest(decodeRequest(t,
				`{"model":"`+tc.model+`","max_tokens":1,"stream":true,`+
					`"messages":[{"role":"user","content":"x"}]}`))
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			if out.Effort == nil || out.Effort.Effort != tc.want {
				t.Fatalf("effort = %+v, want %q", out.Effort, tc.want)
			}
		})
	}
}

// An effort the caller did name wins over the model's default.
func TestAnExplicitEffortWins(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t,
		`{"model":"sonnet","max_tokens":1,"stream":true,"output_config":{"effort":"low"},`+
			`"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if out.Effort == nil || out.Effort.Effort != "low" {
		t.Fatalf("effort = %+v, want low", out.Effort)
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
	translator := NewTranslatorFor(callable("Read"), "")

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
	var frames []anthropic.Frame
	for _, e := range completedWith(functionCall("call_1", "Read", `{"file_path":"a.txt"}`)) {
		produced, err := translator.Accept(e)
		if err != nil {
			t.Fatalf("Accept(%s): %v", e.Type, err)
		}
		frames = append(frames, produced...)
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
			events := []any{}
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
				textDelta("ok"), textDone("ok"),
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

// A schema the client asked for has to reach the backend.
//
// Validating one and then dropping it is the quietest failure available here: the request
// is accepted, so the caller believes the constraint holds, and the model is never told
// about it. The caller is usually a workflow agent or a subagent whose result is parsed,
// so what surfaces is a parse error one layer away from the cause.
func TestARequestedSchemaReachesTheBackend(t *testing.T) {
	const schema = `{"type":"object","properties":{"verdict":{"type":"boolean"}}}`
	const head = `{"model":"sonnet","max_tokens":1,"stream":true,` +
		`"messages":[{"role":"user","content":"x"}],"output_config":{"format":`

	named, err := BuildRequest(decodeRequest(t,
		head+`{"type":"json_schema","name":"review","schema":`+schema+`}}}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	encoded, _ := json.Marshal(named)
	for _, want := range []string{
		`"text":{"format":{`, `"type":"json_schema"`, `"name":"review"`, `"strict":true`,
		`"properties":{"verdict":{"type":"boolean"}}`,
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s: %s", want, encoded)
		}
	}

	// An unnamed schema still arrives named. The backend requires one, and inventing a
	// different default than the baseline's would make the same request from the same
	// client look like two schemas depending on which build served it.
	unnamed, err := BuildRequest(decodeRequest(t, head+`{"type":"json_schema","schema":`+schema+`}}}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	// Asserted as a literal, not as the constant. Every other test builds its expectation
	// from anthropic.DefaultSchemaName, so a drift in that name would leave all of them
	// green while the wire changed underneath -- checked by mutation, which survived until
	// this line stopped agreeing with the code it is checking.
	if unnamed.Text == nil || unnamed.Text.Format.Name != "structured_output" {
		t.Fatalf("unnamed schema = %+v, want name %q", unnamed.Text, "structured_output")
	}

	// And a request that asked for no format must not grow one. A constraint nobody asked
	// for is the same defect in the other direction.
	plain, err := BuildRequest(decodeRequest(t,
		`{"model":"sonnet","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if plain.Text != nil {
		t.Fatalf("format sent for a request that named none: %+v", plain.Text)
	}
}

// A response that speaks, thinks, and speaks again.
//
// Two message items, each numbering its content from zero, which is what content_index
// means: an index within one output item. Keyed on that index alone the second item landed
// on the first one's block -- closed, so the whole response died with STREAM_ORDER, and
// open, so two separate answers were concatenated into one. Found by review, reproduced
// before the fix, and it needed the fixtures to start agreeing on item ids as well: the
// deltas said item_1 while the item said msg_1, which nothing had compared.
//
// The second half of the same defect is the mismatch check. It compared one item's stated
// text against everything that had streamed, so item two's "B" was checked against "AB"
// and a sound response was called a lie.
func TestTwoMessageItemsEachKeepTheirOwnBlock(t *testing.T) {
	frames, err := runFor(t, decodeRequest(t, minimalRequest),
		itemAdded(0, messageItem("msg_1")),
		textDeltaFor("msg_1", "first"),
		textDoneFor("msg_1", 0, "first"),
		itemDone(0, messageItem("msg_1", "first")),
		itemAdded(1, messageItem("msg_2")),
		textDeltaFor("msg_2", "second"),
		textDoneFor("msg_2", 0, "second"),
		itemDone(1, messageItem("msg_2", "second")),
		event(codex.Completed, completedOK),
	)
	if err != nil {
		t.Fatalf("a response with two message items failed: %v", err)
	}

	// Two blocks, not one: the client is told they are separate, because they are.
	starts, stops := 0, 0
	for _, frame := range frames {
		switch frame.Type {
		case "content_block_start":
			starts++
		case "content_block_stop":
			stops++
		}
	}
	if starts != 2 || stops != 2 {
		t.Fatalf("blocks opened=%d closed=%d, want 2 and 2", starts, stops)
	}

	// And a real disagreement is still caught. The check got narrower, not weaker.
	_, err = runFor(t, decodeRequest(t, minimalRequest),
		itemAdded(0, messageItem("msg_1")),
		textDeltaFor("msg_1", "first"),
		textDoneFor("msg_1", 0, "first"),
		itemDone(0, messageItem("msg_1", "something else")),
		event(codex.Completed, completedOK),
	)
	if !errors.Is(err, anthropic.ErrTextMismatch) {
		t.Fatalf("err = %v, want TEXT_MISMATCH when an item contradicts its own stream", err)
	}
}

// #85: which item types a response opened is counted, the backend's names shaped the way an
// unsupported event's are.
func TestOutputItemTypesAreCounted(t *testing.T) {
	translator := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
	for i, kind := range []string{"message", "reasoning", "message", "Not A Name"} {
		item := fmt.Sprintf(`{"id":"item_%d","type":%q}`, i, kind)
		if _, err := translator.Accept(itemAdded(i, item)); (err != nil) != (kind == "Not A Name") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	got := translator.ItemTypes()
	if got["message"] != 2 || got["reasoning"] != 1 || got["<other>"] != 1 || len(got) != 3 {
		t.Fatalf("item types = %v", got)
	}
}

// #85: an output item of a type this build does not read is refused with a named category,
// and its type is still counted -- the name is what makes the fix one line.
func TestAnUnknownOutputItemIsRefused(t *testing.T) {
	translator := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
	_, err := translator.Accept(itemAdded(0, `{"id":"ws_1","type":"web_search_call"}`))
	if !errors.Is(err, ErrUnsupportedOutput) || translator.ItemTypes()["web_search_call"] != 1 {
		t.Fatalf("err = %v, types = %v", err, translator.ItemTypes())
	}
}

// #91: the completion is held to the route it was sent on. A model or effort it names that
// differs is refused before anything held is released; one it does not name is not a
// mismatch.
func TestTheReturnedRouteIsHeldToTheSentOne(t *testing.T) {
	for returned, want := range map[string]error{
		`"model":"gpt-6-luna","reasoning":{"effort":"low"},`:  nil,
		`"model":"gpt-6-sol","reasoning":{"effort":"low"},`:   ErrModelEffortMismatch,
		`"model":"gpt-6-luna","reasoning":{"effort":"high"},`: ErrModelEffortMismatch,
		``: nil,
	} {
		translator := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6-luna")
		translator.ExpectRoute("gpt-6-luna", "low")
		var err error
		for _, e := range []stream.Event{textDelta("x"), textDone("x"), event(codex.Completed,
			`{"type":"response.completed","response":{"id":"resp_1",`+returned+usageReported+`}}`)} {
			if _, err = translator.Accept(e); err != nil {
				break
			}
		}
		if !errors.Is(err, want) || (want == nil) != (err == nil) {
			t.Errorf("%s: err = %v, want %v", returned, err, want)
		}
	}
}

// #309: the backend stopping at the cap the request carried, having spent it, is the same
// refusal as running past it. A stop short of the cap, another incomplete reason, and a request
// that carried no cap (thinking off, or under the backend's minimum) keep the backend's own
// category. The usage the incomplete reply reports is kept either way.
func TestTheBackendStoppingAtTheCapIsTheOutputLimit(t *testing.T) {
	incomplete := func(reason string, output int) stream.Event {
		return event(codex.Incomplete, fmt.Sprintf(`{"type":"response.incomplete","response":{"id":"r","status":"incomplete","incomplete_details":{"reason":%q},"usage":{"input_tokens":9,"output_tokens":%d,"total_tokens":%d}}}`, reason, output, 9+output))
	}
	const capped = `{"model":"sonnet","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"x"}]}`
	for _, c := range []struct {
		body, reason string
		output       int
		limit        bool
	}{
		{capped, "max_output_tokens", 16, true},
		{capped, "max_output_tokens", 10, false},
		{capped, "content_filter", 16, false},
		{`{"model":"sonnet","max_tokens":64,"thinking":{"type":"disabled"},"stream":true,"messages":[{"role":"user","content":"x"}]}`, "max_output_tokens", 64, false},
		{`{"model":"sonnet","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"x"}]}`, "max_output_tokens", 8, false},
	} {
		translator := NewTranslatorFor(decodeRequest(t, c.body), "")
		_, err := translator.Accept(incomplete(c.reason, c.output))
		if errors.Is(err, ErrOutputLimitExceeded) != c.limit || err == nil {
			t.Fatalf("%s %s %d: err = %v", c.body, c.reason, c.output, err)
		}
		if usage := translator.ObservedUsage(); !usage.OutputKnown || usage.OutputTokens != int64(c.output) {
			t.Fatalf("%s: the incomplete reply's usage was not kept: %+v", c.body, usage)
		}
	}
	if OutputCap(decodeRequest(t, `{"model":"sonnet","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"x"}]}`)) != 0 {
		t.Fatal("a cap under the backend's minimum was sent")
	}
}
