package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Focused-review check of the /clear fix: a prompt, /clear, then a prompt that
// delegates once to a background child. Root and child requests after /clear must
// pass selection with the new session. Only markers, session prefixes and counts
// are logged. The live variant spends real requests, bounded by the ledger.
const (
	liveFirst  = "LIVE_CLEAR_FIRST_OK"
	liveChild  = "LIVE_CLEAR_CHILD_OK"
	liveSecond = "LIVE_CLEAR_SECOND_OK"
	liveAsk    = "Use the Agent tool exactly once with subagent_type \"general-purpose\", model \"gpt-5.6-luna\", effort \"low\" and the prompt \"Reply with exactly " + liveChild + ". Do not use tools.\". When its report arrives, reply with exactly " + liveSecond + "."
)

func TestFocusedReviewLiveClear(t *testing.T) {
	if os.Getenv("CLAUDUCT_LIVE") != "1" {
		t.Skip("live: set CLAUDUCT_LIVE=1 to spend real requests")
	}
	runLiveClear(t, nil)
}

func TestFocusedReviewLiveClearDryRun(t *testing.T) {
	runLiveClear(t, func() (*gateway.Gateway, error) {
		g, e := gateway.Start(&liveClearFixture{})
		if e == nil {
			g.EnableContextPolicy()
		}
		return g, e
	})
}

func runLiveClear(t *testing.T, start func() (*gateway.Gateway, error)) {
	buildHook(t)
	exe := nativeAvailable(t)
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 6})
	_, cwd := workspace(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	in, input := io.Pipe()
	output, stdout := io.Pipe()
	var stderr bytes.Buffer
	var result Result
	var runErr error
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { input.CloseWithError(ctx.Err()); output.CloseWithError(ctx.Err()) })
	defer stop()
	go func() {
		result, runErr = Run(ctx, Options{Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--model", "gpt-5.6-luna", "--effort", "low", "--allowedTools", "Agent", "--strict-mcp-config"}, Env: isolatedEnv(t), Cwd: cwd, Stdin: in, Stdout: stdout, Stderr: &stderr, Ledger: ledger, ResolveClaude: func() (string, bool, error) { return exe, true, nil }, StartGateway: start})
		stdout.Close()
		close(done)
	}()
	send := func(text string) {
		if err := json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}); err != nil {
			t.Error(err)
		}
	}
	short := func(s string) string { return s[:min(8, len(s))] }
	phase, firstSession, clearSession := 0, "", ""
	firstOK, clearChanged, secondMarker, childReported, failedResults, afterReport := false, false, false, false, 0, 0
	send("Reply with exactly " + liveFirst + ". Do not use tools.")
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var e struct {
			Type, Subtype, Status, Result, Summary string
			SessionID                              string `json:"session_id"`
			IsError                                bool   `json:"is_error"`
		}
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		if e.Subtype == "task_notification" {
			t.Logf("live: task_notification status=%s child_marker=%v", e.Status, strings.Contains(e.Summary, liveChild))
			childReported = childReported || e.Status == "completed" && strings.Contains(e.Summary, liveChild)
		}
		if e.Type != "result" {
			continue
		}
		t.Logf("live: result phase=%d error=%v session=%s", phase, e.IsError, short(e.SessionID))
		if e.IsError {
			failedResults++
			break
		}
		switch phase {
		case 0:
			firstOK, firstSession = strings.Contains(e.Result, liveFirst), e.SessionID
			phase++
			send("/clear")
		case 1:
			clearSession = e.SessionID
			clearChanged = clearSession != "" && clearSession != firstSession
			phase++
			send(liveAsk)
		default:
			secondMarker = secondMarker || strings.Contains(e.Result, liveSecond) && e.SessionID == clearSession
			if childReported {
				afterReport++
			}
		}
		if afterReport > 0 && secondMarker {
			break
		}
	}
	input.Close()
	<-done
	attempts, inferences, refused := ledger.Spent()
	var categories []string
	for _, r := range result.Diagnostics.RecentFailures {
		categories = append(categories, r.Category)
	}
	childBackend := 0
	for _, r := range result.Diagnostics.Recent {
		if r.AgentID != "" && r.Kind == "generation" && r.Status == 200 {
			childBackend++
		}
	}
	t.Logf("live: first_marker=%v clear_changed_session=%v second_marker=%v child_reported=%v child_generation_200=%d result_errors=%d",
		firstOK, clearChanged, secondMarker, childReported, childBackend, failedResults)
	t.Logf("live: attempts=%d inferences=%d refused=%d run_error=%v exit=%d cleanup_failed=%v failure_categories=%v stderr_present=%v",
		attempts, inferences, refused, runErr != nil, result.NativeExitCode, result.CleanupErr != nil, categories, stderr.Len() > 0)
	if !firstOK || !clearChanged || !secondMarker || !childReported || childBackend == 0 || failedResults != 0 || refused != 0 || len(categories) != 0 || runErr != nil || result.CleanupErr != nil {
		t.Fatal("/clear with a background child was not fully verified")
	}
}

// Parent: launch the child, then answer; the child answers its marker. Conversation
// requests are told apart by the instruction only the parent's history carries.
type liveClearFixture struct {
	mu      sync.Mutex
	parents int
}

func (f *liveClearFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f *liveClearFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	body := string(call.Body)
	reply := textStream("side", "untitled")
	f.mu.Lock()
	switch {
	case strings.Contains(body, "Use the Agent tool exactly once"):
		f.parents++
		if f.parents == 1 {
			reply = toolStream("live_agent", "Agent", `{"subagent_type":"general-purpose","description":"live proof","prompt":"Reply with exactly `+liveChild+`. Do not use tools.","model":"gpt-5.6-luna","effort":"low"}`)
		} else {
			reply = textStream("parent", liveSecond)
		}
	case strings.Contains(body, liveChild):
		reply = textStream("child", liveChild)
	case strings.Contains(body, liveFirst):
		reply = textStream("first", liveFirst)
	}
	f.mu.Unlock()
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}
