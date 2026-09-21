package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type forkResumeFixture struct {
	root, outer, fork atomic.Int64
	mu                sync.Mutex
	forkID            string
	resumeSent        bool
	resumeDirective   bool
}

func (f *forkResumeFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f *forkResumeFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	reply := textStream("side", "done")
	if call.Source == "delegation-inherited" || call.Source == "verified-resume" {
		f.fork.Add(1)
		answer := "FORK_FIRST"
		if call.Source == "verified-resume" {
			var request struct {
				Input []struct {
					Role    string
					Content json.RawMessage
				}
			}
			if err := json.Unmarshal(call.Body, &request); err != nil {
				return nil, err
			}
			var lastUser json.RawMessage
			for _, entry := range request.Input {
				if entry.Role == "user" {
					lastUser = entry.Content
				}
			}
			f.mu.Lock()
			f.resumeDirective = bytes.Contains(lastUser, []byte("FORK_SECOND"))
			if f.resumeDirective {
				answer = "FORK_SECOND"
			}
			f.mu.Unlock()
		}
		reply = textStream("fork", answer)
	} else if call.Model == "gpt-5.6-terra" {
		switch f.outer.Add(1) {
		case 1:
			reply = toolStream("discover_inner", "ToolSearch", `{"query":"select:Agent","max_results":1}`)
		case 2:
			reply = toolStream("fork_call", "Agent", `{"subagent_type":"fork","description":"public fork","prompt":"Reply with FORK_FIRST without tools."}`)
		default:
			reply = textStream("outer", "OUTER_DONE")
		}
	} else if upstream.Conversation(string(call.Body)) {
		switch f.root.Add(1) {
		case 1:
			reply = toolStream("outer_call", "Agent", `{"subagent_type":"general-purpose","model":"gpt-5.6-terra","effort":"medium","description":"outer","prompt":"delegate a fork"}`)
		case 2:
			reply = toolStream("discover_send", "ToolSearch", `{"query":"select:SendMessage","max_results":1}`)
		default:
			f.mu.Lock()
			if f.forkID != "" && !f.resumeSent {
				f.resumeSent = true
				args, _ := json.Marshal(map[string]any{"to": f.forkID, "message": "Reply with FORK_SECOND without tools.", "summary": "Public fork resume", "notify_when_idle": true})
				reply = toolStream("fork_resume", "SendMessage", string(args))
			} else {
				reply = textStream("root", "PUBLIC_PARENT_DONE")
			}
			f.mu.Unlock()
		}
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}

// Keep input open and use native completion events before resuming the child.
// TaskOutput is not available in the current native agent runtime.
func TestNativeAncestorResumesForkWithOriginalSelection(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)
	f := &forkResumeFixture{}
	_, cwd := workspace(t)
	env := isolatedEnv(t)
	env["CLAUDE_CODE_FORK_SUBAGENT"] = "1"
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()
	in, input := io.Pipe()
	output, stdout := io.Pipe()
	defer in.Close()
	defer input.Close()
	defer output.Close()
	defer stdout.Close()
	var stderr bytes.Buffer
	var result Result
	var runErr error
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { input.CloseWithError(ctx.Err()); output.CloseWithError(ctx.Err()) })
	defer stop()
	go func() {
		result, runErr = Run(ctx, Options{Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--allowedTools", "Agent,ToolSearch,SendMessage", "--strict-mcp-config"}, Env: env, Cwd: cwd, Stdin: in, Stdout: stdout, Stderr: &stderr, ResolveClaude: func() (string, bool, error) { return exe, true, nil }, StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(f)
			if e == nil {
				g.EnableContextPolicy()
			}
			return g, e
		}})
		stdout.Close()
		close(done)
	}()
	defer func() { cancel(); input.Close(); output.Close(); <-done }()
	send := func(text string) error {
		return json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
	}
	if err := send("Run public fork resume proof"); err != nil {
		t.Fatal(err)
	}
	notifications := 0
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var e struct {
			Type, Subtype, Status, Result string
			Summary                       string
			TaskID                        string `json:"task_id"`
			ToolUseID                     string `json:"tool_use_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			t.Error("invalid native stream JSON")
			break
		}
		if e.Subtype == "task_notification" {
			t.Logf("native task event: call=%s status=%s", e.ToolUseID, e.Status)
		}
		if e.Subtype == "task_notification" && e.Status == "completed" {
			f.mu.Lock()
			id := f.forkID
			f.mu.Unlock()
			if notifications == 0 && e.ToolUseID == "fork_call" {
				f.mu.Lock()
				f.forkID = e.TaskID
				f.mu.Unlock()
				notifications++
				if err := send("Resume the completed public fork exactly once."); err != nil {
					t.Error(err)
					break
				}
			} else if notifications == 1 && e.TaskID == id {
				if !strings.Contains(e.Summary, "FORK_SECOND") {
					t.Errorf("resumed notification lacks the new report: %q", e.Summary)
				}
				notifications++
				if err := send("Collect the resumed report and finish."); err != nil {
					t.Error(err)
					break
				}
			}
		}
		if notifications == 2 && e.Type == "result" && strings.Contains(e.Result, "PUBLIC_PARENT_DONE") {
			input.Close()
		}
	}
	input.Close()
	cancel()
	<-done
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if runErr != nil || result.NativeExitCode != 0 || f.fork.Load() != 2 || notifications != 2 || !f.resumeDirective || len(result.Diagnostics.RecentFailures) != 0 {
		t.Fatalf("fork=%d notifications=%d exit=%d err=%v stderr=%s failures=%+v", f.fork.Load(), notifications, result.NativeExitCode, runErr, tail(stderr.String(), 1000), result.Diagnostics.RecentFailures)
	}
	verified := false
	for _, r := range result.Diagnostics.Recent {
		if r.Source == "verified-resume" {
			verified = r.AgentRole == "fork" && r.ParentAgentID == "" && r.Requested == "gpt-6-astra" && r.Model == "gpt-5.6-terra" && r.Effort == "medium" && r.UsageSource == "backend" && r.InputTokens != nil && r.OutputTokens != nil
		}
	}
	if !verified {
		t.Fatal("resumed fork did not retain its original choice")
	}
	for _, r := range result.Diagnostics.AgentResults.Recent {
		if r.Call == "fork_resume" && r.State == "parent_received" {
			return
		}
	}
	t.Fatal("new report not delivered to the resuming caller")
}
