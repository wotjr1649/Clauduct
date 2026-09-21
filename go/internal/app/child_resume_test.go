package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type childResumeFixture struct {
	mu          sync.Mutex
	config      string
	root, child int
	id          string
}

func (f *childResumeFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f *childResumeFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := textStream("side", "done")
	if upstream.Conversation(string(call.Body)) {
		if call.Model == "gpt-5.6-sol" {
			f.child++
			if f.child == 1 {
				reply = textStream("first", "First public report: evidence checked; no changes; unverified none.")
			} else {
				reply = textStream("second", "Second public report: follow-up checked; no changes; unverified none.")
			}
		} else {
			f.root++
			switch f.root {
			case 1:
				reply = toolStream("initial_agent", "Agent", `{"subagent_type":"Plan","description":"public resume","prompt":"First public report","model":"gpt-5.6-sol","effort":"high","run_in_background":false}`)
			case 2:
				reply = toolStream("find_send", "ToolSearch", `{"query":"select:SendMessage,TaskOutput","max_results":2}`)
			case 3:
				_ = filepath.WalkDir(filepath.Join(f.config, "projects"), func(path string, d fs.DirEntry, err error) error {
					if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "agent-") && strings.HasSuffix(d.Name(), ".meta.json") {
						f.id = strings.TrimSuffix(strings.TrimPrefix(d.Name(), "agent-"), ".meta.json")
					}
					return err
				})
				args, _ := json.Marshal(map[string]string{"to": f.id, "message": "Continue the existing task: give the second public report.", "summary": "Follow up existing public proof"})
				reply = toolStream("resume_agent", "SendMessage", string(args))
			case 4:
				args, _ := json.Marshal(map[string]any{"task_id": f.id, "block": true, "timeout": 10000})
				reply = toolStream("existing_output", "TaskOutput", string(args))
			default:
				reply = textStream("parent", "Both public reports received.")
			}
		}
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}
func TestNativeCompletedChildResumeKeepsSelectionAndReports(t *testing.T) {
	buildHook(t)
	f := &childResumeFixture{config: t.TempDir()}
	out := (nativeRun{ConfigDir: f.config, Args: []string{"-p", "exercise existing child continuation", "--allowedTools", "Agent,ToolSearch,SendMessage,TaskOutput"}, transport: f, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || f.child != 2 {
		t.Fatalf("continuation exit=%d err=%v child turns=%d output=%s records=%+v", out.result.NativeExitCode, out.err, f.child, tail(out.output(), 1000), out.result.Diagnostics.Recent)
	}
	count := 0
	for _, r := range out.result.Diagnostics.Recent {
		if r.AgentID == f.id && r.Path == "/v1/messages" {
			count++
			if r.Model != "gpt-5.6-sol" || r.Effort != "high" || r.SelectionVerified == nil || !*r.SelectionVerified || r.UsageSource != "backend" || r.InputTokens == nil || r.OutputTokens == nil {
				t.Fatal("resumed selection changed")
			}
		}
	}
	if count != 2 {
		t.Fatal("same native child identity not observed twice")
	}
	if out.result.Diagnostics.AgentResults.Totals["parent_received"] != 2 {
		t.Fatalf("resumed result not acquired: %+v", out.result.Diagnostics.AgentResults)
	}
	t.Logf("native child turns=%d results=%+v", f.child, out.result.Diagnostics.AgentResults)
}
