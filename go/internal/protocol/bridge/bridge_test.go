package bridge

import (
	"encoding/json"
	"errors"
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

// run feeds a whole event sequence and collects the frames it produced.
func run(t *testing.T, events ...stream.Event) ([]anthropic.Frame, error) {
	t.Helper()
	translator := NewTranslator("gpt-6-astra")
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

const completedNoUsage = `{"type":"response.completed","response":{"id":"resp_1"}}`

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
		textDelta(0, "x"), textDone(0, "x"), event(codex.Completed, completedNoUsage))
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
func TestEmptySnapshotWithNoDeltasIsAccepted(t *testing.T) {
	frames, err := run(t, textDone(0, ""), event(codex.Completed, completedNoUsage))
	if err != nil {
		t.Fatalf("an empty part with no deltas was refused: %v", err)
	}
	// It opened no block, so the client sees a message that started and stopped.
	if got := frameTypes(frames); strings.Join(got, ",") != "message_start,message_delta,message_stop" {
		t.Fatalf("frames = %v", got)
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
		event(codex.Completed, completedNoUsage),
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
		event(codex.Completed, completedNoUsage))
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
		"response.function_call_arguments.delta",
		"thread.started",
		"",
		"response.",
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
	frames, err := run(t, textDelta(0, ""), textDone(0, ""), event(codex.Completed, completedNoUsage))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := frameTypes(frames); strings.Join(got, ",") !=
		"message_start,content_block_start,content_block_delta,content_block_stop,message_delta,message_stop" {
		t.Fatalf("frames = %v", got)
	}
}

// A response that produced nothing at all still needs its message frames, or the client
// waits for a message that never started.
func TestEmptyResponseStillOpensAndClosesTheMessage(t *testing.T) {
	frames, err := run(t, event(codex.Completed, completedNoUsage))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := frameTypes(frames); strings.Join(got, ",") != "message_start,message_delta,message_stop" {
		t.Fatalf("frames = %v", got)
	}
}

// A block left open at completion is closed first, so a client reading sequentially never
// sees a message end with a block outstanding.
func TestOpenBlocksAreClosedAtCompletion(t *testing.T) {
	frames, err := run(t, textDelta(0, "unfinished"), event(codex.Completed, completedNoUsage))
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

// Unknown counts stay unknown. Writing 0 would turn "we do not know what this cost" into
// "it cost nothing", and a budget built on that number would be wrong in the cheap
// direction.
func TestUnknownUsageIsOmittedNotZeroed(t *testing.T) {
	frames, err := run(t, event(codex.Completed, completedNoUsage))
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
		if len(body.Usage) != 0 {
			t.Fatalf("usage = %v, want empty when the backend reported none", body.Usage)
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
	if out.Model != "gpt-6-astra" || out.MaxTokens != 2048 || !out.Stream {
		t.Fatalf("envelope = %+v", out)
	}
	if out.Instruction != "You are Claude Code." {
		t.Fatalf("instructions = %q", out.Instruction)
	}
	if out.Effort == nil || out.Effort.Effort != "high" {
		t.Fatalf("effort = %+v", out.Effort)
	}
	if len(out.Input) != 3 {
		t.Fatalf("input = %d turns, want 3", len(out.Input))
	}
	// The backend distinguishes text the user supplied from text the model produced.
	if out.Input[0].Content[0].Type != "input_text" || out.Input[1].Content[0].Type != "output_text" {
		t.Fatalf("part types = %q %q", out.Input[0].Content[0].Type, out.Input[1].Content[0].Type)
	}
	if out.Input[2].Content[0].Text != "second" {
		t.Fatalf("last turn = %q", out.Input[2].Content[0].Text)
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
	if fromString.Instruction != fromBlocks.Instruction || fromString.Instruction != "instructions" {
		t.Fatalf("%q vs %q", fromString.Instruction, fromBlocks.Instruction)
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
	if strings.Contains(string(encoded), "reasoning") {
		t.Fatalf("an absent effort became a field: %s", encoded)
	}
}
