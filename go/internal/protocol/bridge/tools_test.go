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

// functionCall builds one output item. The id and name are JSON-encoded rather than
// interpolated, so a hostile identifier produces well-formed JSON carrying a bad id —
// which is what puts the identifier check under test instead of the JSON decoder.
func functionCall(id, name, arguments string) string {
	encodedID, _ := json.Marshal(id)
	encodedName, _ := json.Marshal(name)
	encodedArgs, _ := json.Marshal(arguments)
	return `{"type":"function_call","call_id":` + string(encodedID) +
		`,"name":` + string(encodedName) + `,"arguments":` + string(encodedArgs) + `}`
}

func blockAt(t *testing.T, frames []anthropic.Frame, kind string, nth int) map[string]any {
	t.Helper()
	seen := 0
	for _, frame := range frames {
		if frame.Type != kind {
			continue
		}
		if seen == nth {
			var body map[string]any
			if err := json.Unmarshal(frame.Data, &body); err != nil {
				t.Fatalf("%s: %v", kind, err)
			}
			return body
		}
		seen++
	}
	t.Fatalf("no %s frame at index %d", kind, nth)
	return nil
}

// TOOL05, the property this whole design exists for: nothing executable reaches the client
// before the backend has said the response completed. A stream that fails midway must not
// have handed over a call.
func TestNoToolFrameEscapesBeforeCompletion(t *testing.T) {
	translator := NewTranslatorFor(callable("Read"))

	// Everything a backend sends while a call is being produced.
	for _, e := range []stream.Event{
		event(codex.Created, `{"type":"response.created","response":{"id":"r"}}`),
		event(codex.InProgress, `{"type":"response.in_progress"}`),
		event(codex.OutputItemAdd, `{"type":"response.output_item.added"}`),
		textDelta(0, "let me look"),
		textDone(0, "let me look"),
		event(codex.OutputItemDone, `{"type":"response.output_item.done"}`),
	} {
		frames, err := translator.Accept(e)
		if err != nil {
			t.Fatalf("Accept(%s): %v", e.Type, err)
		}
		for _, frame := range frames {
			if strings.Contains(string(frame.Data), "tool_use") {
				t.Fatalf("a tool frame was released before completion: %s", frame.Data)
			}
		}
	}
	if translator.ToolCallCount() != 0 {
		t.Fatal("a call was held before the backend said the response completed")
	}
}

// And the other half: a stream that fails after the text but before completion delivers no
// call at all, so nothing was executed on the strength of a response that never finished.
func TestFailureBeforeCompletionDeliversNoCall(t *testing.T) {
	frames, err := runFor(t, callable("Read"),
		textDelta(0, "working"), textDone(0, "working"),
		event(codex.Failed, `{"type":"response.failed"}`))

	if !errors.Is(err, codex.ErrResponseFailed) {
		t.Fatalf("err = %v, want the upstream failure", err)
	}
	for _, frame := range frames {
		if strings.Contains(string(frame.Data), "tool_use") {
			t.Fatalf("a tool frame survived a failed response: %s", frame.Data)
		}
	}
}

// TOOL03: the identifier, the name and the arguments all reach the client intact, because
// a result comes back addressed to that identifier.
func TestToolCallIdentityReachesTheClient(t *testing.T) {
	frames, err := runFor(t, callable("Read"),
		textDelta(0, "checking"), textDone(0, "checking"),
		completedWith(functionCall("call_abc123", "Read", `{"file_path":"C:\\a\\b.txt"}`)))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// The tool block is the second content block: text opened first.
	start := blockAt(t, frames, "content_block_start", 1)
	block := start["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "call_abc123" || block["name"] != "Read" {
		t.Fatalf("content_block = %v", block)
	}
	// The block opens with empty input; the arguments arrive as a delta.
	if input, ok := block["input"].(map[string]any); !ok || len(input) != 0 {
		t.Fatalf("input at open = %v, want empty", block["input"])
	}

	delta := blockAt(t, frames, "content_block_delta", 1)
	inner := delta["delta"].(map[string]any)
	if inner["type"] != "input_json_delta" {
		t.Fatalf("delta = %v", inner)
	}
	if !strings.Contains(inner["partial_json"].(string), `"C:\\a\\b.txt"`) {
		t.Fatalf("arguments were rewritten: %v", inner["partial_json"])
	}
}

// TOOL04: several calls keep their order and each keeps its own identifier.
func TestMultipleCallsKeepOrderAndIdentity(t *testing.T) {
	frames, err := runFor(t, callable("Read", "Write"),
		completedWith(
			functionCall("call_1", "Read", `{"file_path":"a"}`),
			functionCall("call_2", "Write", `{"file_path":"b"}`),
			functionCall("call_3", "Read", `{"file_path":"c"}`)))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	for index, want := range []struct{ id, name string }{
		{"call_1", "Read"}, {"call_2", "Write"}, {"call_3", "Read"},
	} {
		block := blockAt(t, frames, "content_block_start", index)["content_block"].(map[string]any)
		if block["id"] != want.id || block["name"] != want.name {
			t.Errorf("call %d = %v, want %s/%s", index, block, want.id, want.name)
		}
	}

	// Block indexes must be distinct and ascending, or a client cannot tell the deltas
	// apart.
	seen := map[float64]bool{}
	previous := -1.0
	for i := 0; i < 3; i++ {
		index := blockAt(t, frames, "content_block_start", i)["index"].(float64)
		if seen[index] || index <= previous {
			t.Fatalf("block index %v repeats or goes backwards", index)
		}
		seen[index] = true
		previous = index
	}
}

// TOOL07: an omitted optional, one explicitly cleared and one chosen are three different
// instructions to whatever runs the tool. They must reach it as three.
func TestOptionalArgumentShapesStayDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, arguments := range []string{`{}`, `{"isolation":null}`, `{"isolation":"worktree"}`} {
		frames, err := runFor(t, callable("Agent"),
			completedWith(functionCall("call_1", "Agent", arguments)))
		if err != nil {
			t.Fatalf("%s: %v", arguments, err)
		}
		delta := blockAt(t, frames, "content_block_delta", 0)["delta"].(map[string]any)
		got := delta["partial_json"].(string)
		if previous, clash := seen[got]; clash {
			t.Fatalf("%s and %s both reached the client as %s", previous, arguments, got)
		}
		seen[got] = arguments
	}
	if len(seen) != 3 {
		t.Fatalf("got %d distinct argument payloads, want 3", len(seen))
	}
}

// WIRE11: arguments that cannot be read as an object never reach the client. A tool is
// about to run with them, so a half-understood call is worse than no call.
func TestMalformedArgumentsNeverReachTheClient(t *testing.T) {
	for name, arguments := range map[string]string{
		"not json":       `not json`,
		"truncated":      `{"file_path":`,
		"an array":       `["a"]`,
		"a string":       `"text"`,
		"a number":       `42`,
		"trailing value": `{"a":1}{"b":2}`,
		"duplicate key":  `{"a":1,"a":2}`,
		"empty":          ``,
	} {
		t.Run(name, func(t *testing.T) {
			frames, err := runFor(t, callable("Read"),
				completedWith(functionCall("call_1", "Read", arguments)))
			if !errors.Is(err, anthropic.ErrInvalidToolCall) {
				t.Fatalf("err = %v, want INVALID_TOOL_CALL", err)
			}
			for _, frame := range frames {
				if strings.Contains(string(frame.Data), "tool_use") {
					t.Fatalf("a malformed call reached the client: %s", frame.Data)
				}
			}
		})
	}
}

// TOOL08, the half that protects the client: a new call must name a tool that is callable
// now. A model naming a withdrawn tool is asking for something the client will not run.
func TestNewCallMustNameACallableTool(t *testing.T) {
	_, err := runFor(t, callable("Read"),
		completedWith(functionCall("call_1", "Write", `{}`)))
	if !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
		t.Fatalf("err = %v, want UNSUPPORTED_TOOL_CALL", err)
	}
}

// A repeated call identifier makes two calls indistinguishable, and a result addressed to
// it could be matched to either.
func TestRepeatedCallIdentifierIsRefused(t *testing.T) {
	_, err := runFor(t, callable("Read"),
		completedWith(
			functionCall("call_1", "Read", `{}`),
			functionCall("call_1", "Read", `{}`)))
	if !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
		t.Fatalf("err = %v, want UNSUPPORTED_TOOL_CALL", err)
	}
}

// The turn is waiting on the client rather than finished, and the client reads stop_reason
// to know which.
func TestStopReasonSaysWhetherAToolIsPending(t *testing.T) {
	withCall, err := runFor(t, callable("Read"),
		completedWith(functionCall("call_1", "Read", `{}`)))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := blockAt(t, withCall, "message_delta", 0)["delta"].(map[string]any)["stop_reason"]; got != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", got)
	}

	textOnly, err := run(t, textDelta(0, "done"), textDone(0, "done"), event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got := blockAt(t, textOnly, "message_delta", 0)["delta"].(map[string]any)["stop_reason"]; got != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", got)
	}
}

// Text closes before a tool block opens. A client reading sequentially must not see a call
// arrive inside an unfinished answer.
func TestTextClosesBeforeAnyToolBlockOpens(t *testing.T) {
	frames, err := runFor(t, callable("Read"),
		textDelta(0, "looking"), textDone(0, "looking"),
		completedWith(functionCall("call_1", "Read", `{}`)))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	types := frameTypes(frames)
	firstToolStart, textStop := -1, -1
	for i, kind := range types {
		if kind == "content_block_stop" && textStop < 0 {
			textStop = i
		}
		if kind == "content_block_start" && i > 0 && firstToolStart < 0 && textStop >= 0 {
			firstToolStart = i
		}
	}
	if textStop < 0 || firstToolStart < 0 || textStop > firstToolStart {
		t.Fatalf("text did not close before the tool block opened: %v", types)
	}
}

// The completed payload's own account of the answer is checked against what was streamed.
// A disagreement means a delta went missing, and neither version can be handed on.
func TestCompletedTextIsCheckedAgainstTheStream(t *testing.T) {
	_, err := runFor(t, callable("Read"),
		textDelta(0, "streamed"), textDone(0, "streamed"),
		completedWith(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"something else"}]}`))
	if !errors.Is(err, anthropic.ErrTextMismatch) {
		t.Fatalf("err = %v, want TEXT_MISMATCH", err)
	}
}

func TestCompletedTextAgreeingWithTheStreamIsAccepted(t *testing.T) {
	_, err := runFor(t, callable("Read"),
		textDelta(0, "strea"), textDelta(0, "med"), textDone(0, "streamed"),
		completedWith(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"streamed"}]}`))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

// --- request construction ----------------------------------------------------------------

func TestToolDefinitionsReachTheBackend(t *testing.T) {
	request := decodeRequest(t, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],
	  "tools":[{"name":"Read","description":"read a file","input_schema":{"type":"object",
	    "properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]}`)

	out, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if len(out.Tools) != 1 {
		t.Fatalf("tools = %+v", out.Tools)
	}
	tool := out.Tools[0]
	if tool.Type != "function" || tool.Name != "Read" || tool.Description != "read a file" {
		t.Fatalf("tool = %+v", tool)
	}
	// The schema is carried through unread: a second, weaker validator here would only
	// disagree with the one that decides.
	if !strings.Contains(string(tool.Parameters), `"required":["file_path"]`) {
		t.Fatalf("schema was rewritten: %s", tool.Parameters)
	}
}

// A deferred tool is held back until the conversation has named it. That is what deferring
// is for: a tool nobody has mentioned costs schema on every request.
func TestDeferredToolsAreHeldBackUntilDiscovered(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"tools":[
	  {"name":"Read","input_schema":{"type":"object"}},
	  {"name":"Rare","defer_loading":true,"input_schema":{"type":"object"}}],"messages":`

	out, err := BuildRequest(decodeRequest(t, head+`[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "Read" {
		t.Fatalf("an undiscovered deferred tool was sent: %+v", out.Tools)
	}

	// Naming it in history brings it back.
	out, err = BuildRequest(decodeRequest(t, head+`[
	  {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Rare","input":{}}]},
	  {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if len(out.Tools) != 2 {
		t.Fatalf("a discovered deferred tool was still held back: %+v", out.Tools)
	}
}

// TOOL08, the half that protects the transcript: completed history survives a changed tool
// set. A conversation recorded when Write existed still decodes after it is withdrawn.
func TestHistoryNamingAWithdrawnToolStillDecodes(t *testing.T) {
	request := decodeRequest(t, `{"model":"m","max_tokens":1,"stream":true,
	  "tools":[{"name":"Read","input_schema":{"type":"object"}}],
	  "messages":[
	    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Write","input":{"path":"a"}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"written"}]},
	    {"role":"user","content":"and now?"}]}`)

	out, err := BuildRequest(request)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	// The history is carried, but Write is not callable again.
	if request.CallableNames()["Write"] {
		t.Fatal("a name seen only in history became callable")
	}
	var sawCall, sawOutput bool
	for _, entry := range out.Input {
		if entry.Type == "function_call" && entry.Name == "Write" {
			sawCall = true
		}
		if entry.Type == "function_call_output" && entry.CallID == "t1" {
			sawOutput = true
		}
	}
	if !sawCall || !sawOutput {
		t.Fatalf("the recorded exchange did not reach the backend: %+v", out.Input)
	}
}

// A recorded call's arguments cross unchanged, for the same reason a new call's do.
func TestRecordedArgumentsCrossUnchanged(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t, `{"model":"m","max_tokens":1,"stream":true,
	  "tools":[{"name":"Agent","input_schema":{"type":"object"}}],
	  "messages":[
	    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Agent","input":{"isolation":null,"big":9007199254740993}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	for _, entry := range out.Input {
		if entry.Type != "function_call" {
			continue
		}
		if !strings.Contains(entry.Arguments, `"isolation":null`) {
			t.Errorf("an explicit null was lost: %s", entry.Arguments)
		}
		if !strings.Contains(entry.Arguments, "9007199254740993") {
			t.Errorf("a large integer was rewritten: %s", entry.Arguments)
		}
		return
	}
	t.Fatal("no function_call entry")
}

// A failed result is announced rather than left to be inferred from its text.
func TestFailedResultIsAnnounced(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t, `{"model":"m","max_tokens":1,"stream":true,
	  "tools":[{"name":"Bash","input_schema":{"type":"object"}}],
	  "messages":[
	    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"exit 1"}]}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	for _, entry := range out.Input {
		if entry.Type != "function_call_output" {
			continue
		}
		if len(entry.Output) < 2 || !strings.Contains(entry.Output[0].Text, "failed") {
			t.Fatalf("the failure was not announced: %+v", entry.Output)
		}
		return
	}
	t.Fatal("no function_call_output entry")
}

// tool_choice crosses in the backend's own vocabulary.
func TestToolChoiceIsTranslated(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],
	  "tools":[{"name":"Read","input_schema":{"type":"object"}}],"tool_choice":`

	for choice, want := range map[string]any{
		`{"type":"auto"}`: "auto",
		`{"type":"none"}`: "none",
		`{"type":"any"}`:  "required",
	} {
		out, err := BuildRequest(decodeRequest(t, head+choice+`}`))
		if err != nil {
			t.Fatalf("%s: %v", choice, err)
		}
		if out.ToolChoice != want {
			t.Errorf("%s = %v, want %v", choice, out.ToolChoice, want)
		}
	}

	out, err := BuildRequest(decodeRequest(t, head+`{"type":"tool","name":"Read"}}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	named, ok := out.ToolChoice.(NamedTool)
	if !ok || named.Name != "Read" || named.Type != "function" {
		t.Fatalf("tool_choice = %#v", out.ToolChoice)
	}
}

func TestDisableParallelToolUseCrosses(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],
	  "tools":[{"name":"Read","input_schema":{"type":"object"}}],"tool_choice":`

	out, err := BuildRequest(decodeRequest(t, head+`{"type":"auto","disable_parallel_tool_use":true}}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if out.Parallel == nil || *out.Parallel {
		t.Fatalf("parallel = %v, want an explicit false", out.Parallel)
	}

	out, err = BuildRequest(decodeRequest(t, head+`{"type":"auto"}}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if out.Parallel != nil {
		t.Fatalf("an unasked-for setting became a field: %v", *out.Parallel)
	}
}

// The recorded order of a turn's text, calls and results is what the backend sees.
func TestRecordedOrderIsPreserved(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t, `{"model":"m","max_tokens":1,"stream":true,
	  "tools":[{"name":"Read","input_schema":{"type":"object"}}],
	  "messages":[
	    {"role":"user","content":"start"},
	    {"role":"assistant","content":[
	      {"type":"text","text":"looking"},
	      {"type":"tool_use","id":"t1","name":"Read","input":{}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"contents"}]},
	    {"role":"assistant","content":[{"type":"text","text":"found it"}]}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	var shape []string
	for _, entry := range out.Input {
		if entry.Type != "" {
			shape = append(shape, entry.Type)
			continue
		}
		shape = append(shape, entry.Role)
	}
	want := "user,assistant,function_call,function_call_output,assistant"
	if strings.Join(shape, ",") != want {
		t.Fatalf("input shape = %v, want %s", shape, want)
	}
}

// An identifier the client cannot address is not an identifier. A result comes back
// addressed to the call's id, so an id carrying characters outside the accepted shape
// produces a result nothing can be matched to.
//
// Only the id is tested here. A name outside the shape can never be a defined tool, so the
// callable check refuses it first and the name half of the guard is defence in depth
// rather than the thing doing the work. Added after a mutation run showed the id half
// unguarded.
func TestCallIdentifierShapeIsChecked(t *testing.T) {
	for name, id := range map[string]string{
		"empty":          "",
		"with a space":   "call one",
		"with a quote":   `call"1`,
		"with a newline": "call\n1",
		"with a slash":   "call/1",
		"with a brace":   "call{1}",
		"absurdly long":  strings.Repeat("c", 300),
	} {
		t.Run(name, func(t *testing.T) {
			frames, err := runFor(t, callable("Read"),
				completedWith(functionCall(id, "Read", `{}`)))
			if !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
				t.Fatalf("err = %v, want UNSUPPORTED_TOOL_CALL", err)
			}
			for _, frame := range frames {
				if strings.Contains(string(frame.Data), "tool_use") {
					t.Fatalf("it reached the client anyway: %s", frame.Data)
				}
			}
		})
	}
}

// The event that would tempt a naive port into releasing a call early. Feeding it here is
// what makes the barrier test able to fail if someone adds that arm.
func TestStreamingItemEventsProduceNoToolFrames(t *testing.T) {
	translator := NewTranslatorFor(callable("Read"))
	for _, e := range []stream.Event{
		event(codex.OutputItemAdd, `{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}}`),
		event(codex.OutputItemDone, `{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}}`),
		event(codex.ContentPartDon, `{"type":"response.content_part.done"}`),
	} {
		frames, err := translator.Accept(e)
		if err != nil {
			t.Fatalf("Accept(%s): %v", e.Type, err)
		}
		for _, frame := range frames {
			if strings.Contains(string(frame.Data), "tool_use") {
				t.Fatalf("%s released a tool frame before completion: %s", e.Type, frame.Data)
			}
		}
	}
}
