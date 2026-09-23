package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Review probe: native /clear inside one process, then a new prompt. Synthetic backend.
type clearProbe struct {
	mu    sync.Mutex
	calls int
}

func (f *clearProbe) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f *clearProbe) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	reply := textStream("side", "untitled")
	if upstream.Conversation(string(call.Body)) {
		reply = textStream("answer", "PUBLIC_CLEAR_PROBE_OK")
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}

func TestReviewProbeClearThenPrompt(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)
	f := &clearProbe{}
	_, cwd := workspace(t)
	env := isolatedEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
		result, runErr = Run(ctx, Options{Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--model", "gpt-5.6-sol", "--effort", "high", "--tools", "", "--strict-mcp-config"}, Env: env, Cwd: cwd, Stdin: in, Stdout: stdout, Stderr: &stderr, ResolveClaude: func() (string, bool, error) { return exe, true, nil }, StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(f)
			if e == nil {
				g.EnableContextPolicy()
			}
			return g, e
		}})
		stdout.Close()
		close(done)
	}()
	type event struct {
		Type, Subtype, Result string
		SessionID             string `json:"session_id"`
		IsError               bool   `json:"is_error"`
	}
	events := make(chan event, 256)
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			var e event
			if json.Unmarshal(scanner.Bytes(), &e) == nil {
				events <- e
			}
		}
		close(events)
	}()
	short := func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	}
	send := func(text string) {
		if err := json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}); err != nil {
			t.Fatal(err)
		}
	}
	waitResult := func(label string, limit time.Duration) (event, bool) {
		timer := time.After(limit)
		for {
			select {
			case e, ok := <-events:
				if !ok {
					return event{}, false
				}
				if e.Type == "system" || e.Type == "result" {
					t.Logf("%s: type=%s subtype=%s session=%s is_error=%v result=%q", label, e.Type, e.Subtype, short(e.SessionID), e.IsError, strings.TrimSpace(e.Result))
				}
				if e.Type == "result" {
					return e, true
				}
			case <-timer:
				t.Logf("%s: no result within %s", label, limit)
				return event{}, false
			}
		}
	}
	send("First public probe prompt")
	first, _ := waitResult("first", 45*time.Second)
	send("/clear")
	waitResult("clear", 20*time.Second)
	send("Second public probe prompt")
	second, ok := waitResult("second", 45*time.Second)
	input.Close()
	<-done
	var categories []string
	for _, r := range result.Diagnostics.RecentFailures {
		categories = append(categories, r.Category)
	}
	t.Logf("probe: run_err=%v exit=%d backend_calls=%d failures=%v stderr_present=%v", runErr, result.NativeExitCode, f.calls, categories, stderr.Len() > 0)
	if first.IsError || !ok || second.IsError || !strings.Contains(second.Result, "PUBLIC_CLEAR_PROBE_OK") {
		t.Fatal("prompt after /clear did not complete")
	}
}
