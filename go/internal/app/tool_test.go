package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// TOOL01, TOOL02 and TOOL06: the real client running real tools through this bridge.
//
// Nothing had done this. The live sessions asked for a word and got one, which proves the
// text path and says nothing about whether this can do work. The tool wire was checked
// against the backend by a probe and the barrier was checked against fixtures, but the
// client executing a call and sending its result back had never happened end to end.
//
// It costs no model calls: the backend is scripted, so the tool call is exactly the one
// each test needs -- including one the user has refused, which a real model would not
// reliably produce on demand.

// toolStream is a backend turn that asks for one tool call.
//
// The shape is the measured one: an item opens, its arguments stream, the item closes, and
// the completion carries an empty output array. Writing a plausible shape here instead is
// what made WP04 wrong.
func toolStream(callID, name, arguments string) string {
	args, _ := json.Marshal(arguments)
	item := `{"id":"fc_1","type":"function_call","call_id":"` + callID +
		`","name":"` + name + `","arguments":` + string(args) + `}`
	return frames(
		`{"type":"response.created","response":{"id":"resp_tool"}}`,
		`{"type":"response.in_progress"}`,
		`{"type":"response.output_item.added","output_index":0,`+
			`"item":{"id":"fc_1","type":"function_call"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":`+string(args)+`}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":`+string(args)+`}`,
		`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
		`{"type":"response.completed","response":{"id":"resp_tool",`+
			`"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13},"output":[]}}`,
	)
}

// textStream is an ordinary reply. The client's side requests get one of these so the
// session proceeds without the test having to anticipate each of them.
func textStream(id, text string) string {
	encoded, _ := json.Marshal(text)
	return frames(
		`{"type":"response.created","response":{"id":"`+id+`"}}`,
		`{"type":"response.output_item.added","output_index":0,`+
			`"item":{"id":"msg_`+id+`","type":"message"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_`+id+`","content_index":0,"delta":`+
			string(encoded)+`}`,
		`{"type":"response.output_text.done","item_id":"msg_`+id+`","content_index":0,"text":`+
			string(encoded)+`}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_`+id+
			`","type":"message","content":[{"type":"output_text","text":`+string(encoded)+`}]}}`,
		`{"type":"response.completed","response":{"id":"`+id+`",`+
			`"usage":{"input_tokens":9,"output_tokens":1,"total_tokens":10},"output":[]}}`,
	)
}

func frames(bodies ...string) string {
	var b strings.Builder
	for _, body := range bodies {
		b.WriteString("data: " + body + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// scripted runs the real client against a backend that answers in order.
func scripted(t *testing.T, args []string, conversation ...string) (*upstream.Script, string, Result) {
	t.Helper()
	script := newScript(conversation...)
	return scriptedOn(t, script, script, args)
}

// newScript builds the backend's answers.
//
// Only the requests carrying tool definitions are the conversation. The client also
// generates a session title, which arrives without tools and is not what any of these tests
// are about; Default answers it so it cannot consume a scripted turn.
func newScript(conversation ...string) *upstream.Script {
	turns := make([]upstream.ScriptTurn, 0, len(conversation))
	for _, sse := range conversation {
		turns = append(turns, upstream.ScriptTurn{When: upstream.Conversation, SSE: sse})
	}
	return &upstream.Script{Turns: turns, Default: textStream("side", "untitled")}
}

// scriptedOn runs the client against transport, which is normally the script itself.
//
// Separated so a test can wrap it. A Script answers inferences and nothing else, and the
// search side query does not go through Execute at all -- the gateway hands it to a
// Searcher, so a test about search has to supply one.
func scriptedOn(t *testing.T, script *upstream.Script, transport upstream.Transport,
	args []string) (*upstream.Script, string, Result) {
	t.Helper()
	exe := nativeAvailable(t)

	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args:          append(args, "--strict-mcp-config"),
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(transport) },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testing.Verbose() {
		t.Logf("exit=%d stdout=%q", result.NativeExitCode, stdout.String())
		if stderr.Len() > 0 {
			t.Logf("stderr=%q", stderr.String())
		}
		t.Logf("requests=%d conversation=%d side=%d", script.Calls(),
			len(script.Conversations()), script.Unmatched())
		for i, request := range script.Requests() {
			var envelope struct {
				Model     string `json:"model"`
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			_ = json.Unmarshal([]byte(request), &envelope)
			t.Logf("request %d: %d bytes  model=%s effort=%s tools=%d",
				i+1, len(request), envelope.Model, envelope.Reasoning.Effort, len(envelope.Tools))
		}
	}
	return script, cwd, result
}

// tail shows the end of a request: the conversation this bridge built is at the end, after
// the tool schemas that make up most of the bytes.
func tail(body string, n int) string {
	if len(body) > n {
		return "..." + body[len(body)-n:]
	}
	return body
}

// TOOL01: a tool call this bridge delivered is executed, and the file on disk changes.
//
// The filesystem is the assertion. A frame that looks right proves the encoding; a file that
// exists afterwards proves the client received something it could act on.
func TestADeliveredToolCallActuallyRuns(t *testing.T) {
	const content = "written by a tool the bridge delivered"
	script, cwd, result := scripted(t,
		[]string{"-p", "create the file", "--allowedTools", "Write"},
		toolStream("call_write_1", "Write", `{"file_path":"`+
			// The client resolves relative paths against its working directory.
			`evidence.txt","content":"`+content+`"}`),
		frames(
			`{"type":"response.created","response":{"id":"resp_done"}}`,
			`{"type":"response.output_item.added","output_index":0,`+
				`"item":{"id":"msg_1","type":"message"}}`,
			`{"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"done"}`,
			`{"type":"response.output_text.done","item_id":"msg_1","content_index":0,"text":"done"}`,
			`{"type":"response.output_item.done","output_index":0,"item":`+
				`{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"done"}]}}`,
			`{"type":"response.completed","response":{"id":"resp_done",`+
				`"usage":{"input_tokens":9,"output_tokens":1,"total_tokens":10},"output":[]}}`,
		))

	written, err := os.ReadFile(filepath.Join(cwd, "evidence.txt"))
	if err != nil {
		t.Fatalf("the tool call never ran: %v\nbackend saw %d requests", err, script.Calls())
	}
	if !strings.Contains(string(written), content) {
		t.Fatalf("the file holds %q", written)
	}

	// And the result came back. Two conversation requests: the one that produced the call,
	// and the one carrying its result. Side requests are counted separately -- see
	// TestASessionCostsMoreThanTheUserAsksFor for what they are.
	conversations := script.Conversations()
	if len(conversations) < 2 {
		t.Fatalf("the backend saw %d conversation requests of %d total; the tool's result "+
			"never went back", len(conversations), script.Calls())
	}
	second := conversations[1]
	if !strings.Contains(second, "function_call_output") {
		t.Fatalf("the second request carries no tool result:\n%s", head(second))
	}
	if !strings.Contains(second, "call_write_1") {
		t.Fatalf("the result is not addressed to the call that was made:\n%s", head(second))
	}
	if result.NativeExitCode != 0 {
		t.Fatalf("exit = %d", result.NativeExitCode)
	}
}

// TOOL02: a tool the user has not allowed does not run.
//
// The backend asks for it anyway, which is the case that matters -- a bridge that delivered
// a call the client then refused must not end up with the effect happening. The client owns
// the decision; what this checks is that nothing here routes around it.
func TestARefusedToolDoesNotRun(t *testing.T) {
	script, cwd, _ := scripted(t,
		// Write is not in the allowed list, and -p cannot prompt for permission.
		[]string{"-p", "create the file", "--allowedTools", "Read"},
		toolStream("call_denied_1", "Write",
			`{"file_path":"must-not-exist.txt","content":"the refusal did not hold"}`),
		frames(
			`{"type":"response.created","response":{"id":"resp_denied"}}`,
			`{"type":"response.output_item.added","output_index":0,`+
				`"item":{"id":"msg_1","type":"message"}}`,
			`{"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"stopped"}`,
			`{"type":"response.output_text.done","item_id":"msg_1","content_index":0,"text":"stopped"}`,
			`{"type":"response.output_item.done","output_index":0,"item":`+
				`{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"stopped"}]}}`,
			`{"type":"response.completed","response":{"id":"resp_denied",`+
				`"usage":{"input_tokens":9,"output_tokens":1,"total_tokens":10},"output":[]}}`,
		))

	if _, err := os.Stat(filepath.Join(cwd, "must-not-exist.txt")); err == nil {
		t.Fatal("a tool the user did not allow wrote a file")
	}
	_ = script
}

// TOOL06: a delivered call is not run twice.
//
// The backend answers the second turn with a failure. Whatever the client does about that,
// it must not re-run a tool it has already run: the effect already happened, and doing it
// again is not a retry but a second edit.
func TestAToolIsNotReRunWhenTheTurnAfterItFails(t *testing.T) {
	script, cwd, _ := scripted(t,
		[]string{"-p", "append once", "--allowedTools", "Write"},
		toolStream("call_once_1", "Write", `{"file_path":"once.txt","content":"first"}`),
		// The turn carrying the result fails.
		frames(`{"type":"response.created","response":{"id":"resp_fail"}}`,
			`{"type":"response.failed"}`),
	)

	if _, err := os.ReadFile(filepath.Join(cwd, "once.txt")); err != nil {
		t.Skipf("the tool did not run, so there is no re-run to observe: %v", err)
	}

	// The file cannot answer this on its own: Write overwrites, so a second execution
	// leaves exactly the same bytes. What distinguishes a re-run is the conversation. Every
	// request carrying the result must be the same conversation -- one call, one result.
	// A second execution would add a second function_call and a second output.
	var carrying []string
	for _, request := range script.Conversations() {
		if strings.Contains(request, "function_call_output") {
			carrying = append(carrying, request)
		}
	}
	if len(carrying) == 0 {
		t.Fatal("no request carried the tool's result")
	}
	for i, request := range carrying[1:] {
		if request != carrying[0] {
			t.Fatalf("request %d carrying a result differs from the first; the tool was "+
				"run again rather than the request retried", i+2)
		}
	}
	if n := strings.Count(carrying[0], "function_call_output"); n != 1 {
		t.Fatalf("one conversation carries %d tool results for one call", n)
	}

	// Measured, and recorded rather than asserted: the client retried the failed request.
	// response.failed becomes 502, which this client retries, so an in-stream backend
	// failure costs one more real request. That is the client's behaviour and the
	// baseline's mapping; what matters here is that the retry repeated the request and not
	// the tool.
	t.Logf("one tool call, %d requests carrying its result (identical), %d requests total",
		len(carrying), script.Calls())
}

func head(body string) string {
	if len(body) > 400 {
		return body[:400] + "..."
	}
	return body
}

// What a session costs is not what the user asked for.
//
// Measured 2026-09-15 while the tool tests were being written: a session sends requests the
// user did not type. A tool exchange produced one alongside the conversation -- the client
// generates a session title -- and it is routed like everything else: same model, same
// effort, same subscription.
//
// A one-word `-p` run produced none, so this is not a fixed per-session surcharge and the
// count is not predictable from the number of turns. That is the point: a cost claim
// counting only the user's turns is wrong.
//
// It is the client's behaviour rather than this bridge's, and the Node baseline routes such
// requests the same way.
func TestNativeRequestAccountingIncludesAnyAuxiliaryCalls(t *testing.T) {
	script, _, result := scripted(t,
		[]string{"-p", "create the file", "--allowedTools", "ToolSearch,Write"},
		toolStream("discover_cost", "ToolSearch", `{"query":"select:Write","max_results":1}`),
		toolStream("call_cost_1", "Write", `{"file_path":"cost.txt","content":"x"}`),
		textStream("after", "done"))

	// 2.1.276 does not invariably generate a title for this print-mode run.
	// Verify accounting of every actual dispatch, not an assumed hidden request.
	observed := 0
	for _, r := range result.Diagnostics.Recent {
		if r.Path == "/v1/messages" {
			observed++
			if r.Outcome != "ok" || r.Model == "" {
				t.Fatal("dispatch missing route or completion")
			}
		}
	}
	if observed != script.Calls() || len(script.Conversations()) < 3 {
		t.Fatal("request accounting or actual tool exchange incomplete")
	}
	t.Logf("one user turn cost %d requests: %d conversation, %d the user did not type",
		script.Calls(), len(script.Conversations()), script.Unmatched())

	// The part that is this bridge's business: a side request is routed, not forwarded raw.
	// An unrouted one would carry a Claude model name to a backend that has never heard of
	// one, which is the defect G7 found.
	for _, request := range script.Requests() {
		if upstream.Conversation(request) {
			continue
		}
		if !strings.Contains(request, `"model":"gpt-`) {
			t.Fatalf("a side request went out unrouted:\n%s", head(request))
		}
	}
}

// LIFE05, and the defect that made every timeout in this package decorative.
//
// Run accepted a context and never looked at it: process.Wait was unconditional, so a caller
// with a deadline could not end a session. Nothing noticed, because main passes
// context.Background and every test session finished quickly on its own. It surfaced when a
// mutation made the scripted backend answer the same tool call forever and the client
// looped -- the suite ran past seven minutes against a twenty-five second bound.
//
// The loop is the point. A client that keeps working cannot be waited out, so the deadline
// has to reach the child.
func TestACallerThatGivesUpEndsTheSession(t *testing.T) {
	exe := nativeAvailable(t)

	// A backend that answers every conversation turn with the same tool call. The client
	// runs the tool, sends the result, and is handed the same call again.
	forever := &upstream.Script{
		Default: toolStream("call_loop_1", "Write",
			`{"file_path":"loop.txt","content":"again"}`),
	}

	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer

	const bound = 6 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()

	started := time.Now()
	result, err := Run(ctx, Options{
		Args:          []string{"-p", "loop", "--allowedTools", "Write", "--strict-mcp-config"},
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(forever) },
	})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatalf("a session that was never going to finish returned cleanly after %v", elapsed)
	}
	// Generous, because stopping a process and reaping it is not instant. What matters is
	// that it is bounded at all: before this, the same run went on past seven minutes.
	if elapsed > bound+20*time.Second {
		t.Fatalf("the deadline was %v and the session took %v; the context did not reach "+
			"the child", bound, elapsed)
	}
	if !result.NativeStarted {
		t.Fatal("the child never started, so nothing about stopping it was measured")
	}
	// The listener is released even when the session ended this way.
	if result.CleanupErr != nil {
		t.Fatalf("cleanup after a cancelled session: %v", result.CleanupErr)
	}
	t.Logf("stopped after %v, %d requests served", elapsed, forever.Calls())
}
