package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The native client and gateway are real. Counts/replies are synthetic here;
// separate live paired evidence establishes the counter's accuracy.
type compactFixture struct {
	mu        sync.Mutex
	file      string
	threshold int64
	turns     int
	compacts  int
	resumed   bool
	fail      bool
	routes    []string
}

func (f *compactFixture) Count(_ context.Context, call upstream.Call) (int64, error) {
	body := string(call.Body)
	if strings.Contains(body, "COMPACT_SYNTHETIC_DONE") {
		return 1000, nil
	}
	if strings.Contains(body, "call_step_4") {
		return f.threshold, nil
	}
	return 1000, nil
}

func (f *compactFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := string(call.Body)
	reply := textStream("side", "untitled")
	if strings.Contains(body, "CRITICAL: Respond with TEXT ONLY") {
		f.compacts++
		f.routes = append(f.routes, call.Model+"/"+call.Effort)
		if f.fail {
			return nil, upstream.Failure{Category: "SYNTHETIC_COMPACT_FAILED", Status: 400}
		}
		reply = textStream("summary", "<summary>COMPACT_SYNTHETIC_DONE: read the public test file five times. All five reads succeeded. Continue by reporting CONTEXT_RESUMED.</summary>")
	} else if upstream.Conversation(body) {
		f.routes = append(f.routes, call.Model+"/"+call.Effort)
		if f.compacts > 0 {
			f.resumed = strings.Contains(body, "COMPACT_SYNTHETIC_DONE")
			reply = textStream("resumed", "CONTEXT_RESUMED")
		} else if !fixtureHasTool(call, "Read") {
			reply = toolStream("discover_read", "ToolSearch", `{"query":"select:Read","max_results":1}`)
		} else {
			f.turns++
			args, _ := json.Marshal(map[string]string{"file_path": f.file})
			reply = toolStream(fmt.Sprintf("call_step_%d", f.turns), "Read", string(args))
		}
	}
	count, _ := f.Count(ctx, call)
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, count)}).Execute(ctx, call)
}

func fixtureHasTool(call upstream.Call, name string) bool {
	var request struct{ Tools []struct{ Name string } }
	_ = json.Unmarshal(call.Body, &request)
	for _, tool := range request.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

var fixtureInputCount = regexp.MustCompile(`"input_tokens":\d+`)
var fixtureTotalCount = regexp.MustCompile(`,"total_tokens":\d+`)

// State-flow fixtures have a synthetic counter. Their response usage must use
// the same synthetic oracle; drift is tested separately with a deliberate mismatch.
func countedFixtureReply(reply string, call upstream.Call, fallback int64) string {
	var built bridge.Request
	if json.Unmarshal(call.Body, &built) == nil {
		if local, err := bridge.CountInput(&built); err == nil {
			fallback = local
		}
	}
	reply = fixtureInputCount.ReplaceAllString(reply, fmt.Sprintf(`"input_tokens":%d`, fallback))
	return fixtureTotalCount.ReplaceAllString(reply, "")
}

func TestNativeMeasuredUsageTriggersPreventiveCompaction(t *testing.T) {
	buildHook(t)
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "public.txt")
			if err := os.WriteFile(file, []byte("public synthetic context proof"), 0600); err != nil {
				t.Fatal(err)
			}
			f := &compactFixture{file: file, threshold: model.Context.CompactAt}
			out := (nativeRun{Args: []string{"-p", "Run the public context proof", "--model", model.ID, "--effort", "high", "--allowedTools", "ToolSearch,Read"}, transport: f, ContextPolicy: true, Timeout: 45 * time.Second}).run(t)
			if out.err != nil || out.result.NativeExitCode != 0 || f.compacts != 1 || !f.resumed {
				t.Fatalf("exit=%d err=%v compact=%d resumed=%v turns=%d records=%+v output=%s", out.result.NativeExitCode, out.err, f.compacts, f.resumed, f.turns, out.result.Diagnostics.Recent, tail(out.output(), 1500))
			}
			for _, route := range f.routes {
				if route != model.ID+"/high" {
					t.Fatalf("compaction changed route: %s", route)
				}
			}
			classes := map[string]bool{}
			for _, record := range out.result.Diagnostics.Recent {
				classes[record.RequestClass] = true
				if record.Kind == "compaction" && (record.InputTokens == nil || record.OutputTokens == nil || record.UsageSource != "backend" || record.CountMs != nil) {
					t.Fatal("compaction did not use backend usage independently of counting")
				}
			}
			if !classes["main"] || !classes["compaction"] {
				t.Fatal("native request class headers missing")
			}
			t.Logf("model=%s threshold=%d compactions=%d resumed=%v requests=%d", model.ID, model.Context.CompactAt, f.compacts, f.resumed, len(f.routes))
		})
	}
}

func TestNativeExactCompactionFailureDoesNotResume(t *testing.T) {
	buildHook(t)
	file := filepath.Join(t.TempDir(), "public.txt")
	if err := os.WriteFile(file, []byte("public proof"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &compactFixture{file: file, threshold: 239000, fail: true}
	out := (nativeRun{Args: []string{"-p", "Run public context proof", "--model", "gpt-5.6-sol", "--effort", "high", "--allowedTools", "ToolSearch,Read"}, transport: f, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode == 0 || f.compacts != 1 || f.resumed || f.turns != 5 {
		t.Fatalf("failure rerun/hidden: err=%v exit=%d compact=%d resumed=%v turns=%d", out.err, out.result.NativeExitCode, f.compacts, f.resumed, f.turns)
	}
	t.Logf("native_exit=%d compactions=%d resumed=%v", out.result.NativeExitCode, f.compacts, f.resumed)
}

type parallelCompactFixture struct {
	child              *compactFixture
	peer               *compactFixture
	root               atomic.Int64
	sol, terra         chan struct{}
	solOnce, terraOnce sync.Once
}

func (f *parallelCompactFixture) Count(ctx context.Context, call upstream.Call) (int64, error) {
	if call.Model == "gpt-5.6-sol" {
		return f.child.Count(ctx, call)
	}
	if call.Model == "gpt-5.6-terra" && f.peer != nil {
		return f.peer.Count(ctx, call)
	}
	return 1000, nil
}
func (f *parallelCompactFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if call.Model == "gpt-5.6-sol" {
		response, err := f.child.Execute(ctx, call)
		f.child.mu.Lock()
		resumed := f.child.resumed
		f.child.mu.Unlock()
		if resumed {
			f.solOnce.Do(func() { close(f.sol) })
		}
		return response, err
	}
	reply := textStream("side", "untitled")
	if call.Model == "gpt-5.6-terra" {
		if call.Effort != "medium" {
			return nil, upstream.ErrScriptExhausted
		}
		if f.peer != nil {
			response, err := f.peer.Execute(ctx, call)
			f.peer.mu.Lock()
			resumed := f.peer.resumed
			f.peer.mu.Unlock()
			if resumed {
				f.terraOnce.Do(func() { close(f.terra) })
			}
			return response, err
		}
		f.terraOnce.Do(func() { close(f.terra) })
		reply = textStream("terra", "TERRA_UNAFFECTED")
	} else if upstream.Conversation(string(call.Body)) && !fixtureHasTool(call, "Agent") {
		reply = toolStream("discover_agent", "ToolSearch", `{"query":"select:Agent","max_results":1}`)
	} else if upstream.Conversation(string(call.Body)) {
		switch f.root.Add(1) {
		case 1:
			reply = toolStream("sol_child", "Agent", `{"subagent_type":"Plan","description":"sol proof","prompt":"read public file five times","model":"gpt-5.6-sol","effort":"high"}`)
		case 2:
			reply = toolStream("terra_child", "Agent", `{"subagent_type":"Plan","description":"terra proof","prompt":"say ok","model":"gpt-5.6-terra","effort":"medium"}`)
		default:
			for _, done := range []chan struct{}{f.sol, f.terra} {
				select {
				case <-done:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			reply = textStream("parent", "PARENT_DONE")
		}
	}
	count, _ := f.Count(ctx, call)
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, count)}).Execute(ctx, call)
}

func TestNativeExactCompactionKeepsParallelAgentsSeparate(t *testing.T) {
	buildHook(t)
	for _, both := range []bool{false, true} {
		t.Run(fmt.Sprintf("both=%v", both), func(t *testing.T) { parallelCompactionProof(t, both) })
	}
}

func parallelCompactionProof(t *testing.T, both bool) {
	file := filepath.Join(t.TempDir(), "public.txt")
	if err := os.WriteFile(file, []byte("parallel public proof"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &parallelCompactFixture{child: &compactFixture{file: file, threshold: 239000}, sol: make(chan struct{}), terra: make(chan struct{})}
	if both {
		f.peer = &compactFixture{file: file, threshold: 239000}
	}
	out := (nativeRun{Args: []string{"-p", "Delegate public synthetic work", "--allowedTools", "ToolSearch,Agent,Read"}, transport: f, ContextPolicy: true, Timeout: 45 * time.Second}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || f.child.compacts != 1 || !f.child.resumed {
		t.Fatalf("parallel context proof: err=%v exit=%d compact=%d resumed=%v diagnostics=%+v", out.err, out.result.NativeExitCode, f.child.compacts, f.child.resumed, out.result.Diagnostics.Recent)
	}
	for _, route := range f.child.routes {
		if route != "gpt-5.6-sol/high" {
			t.Fatal("child compaction changed route")
		}
	}
	found := false
	for _, record := range out.result.Diagnostics.Recent {
		if record.Kind == "compaction" {
			found = true
			if record.AgentID == "" || (record.Model != "gpt-5.6-sol" && !(both && record.Model == "gpt-5.6-terra")) || record.SelectionVerified == nil || !*record.SelectionVerified {
				t.Fatal("compaction lost verified child binding")
			}
		}
	}
	if !found {
		t.Fatal("child compaction evidence absent")
	}
	select {
	case <-f.terra:
	default:
		t.Fatal("terra did not finish")
	}
	peerCompacts := 0
	if both {
		peerCompacts = f.peer.compacts
		if peerCompacts != 1 || !f.peer.resumed {
			t.Fatal("parallel peer did not compact and resume")
		}
		for _, route := range f.peer.routes {
			if route != "gpt-5.6-terra/medium" {
				t.Fatal("parallel peer selection changed")
			}
		}
	}
	t.Logf("sol_compactions=%d sol_resumed=%v terra_compactions=%d terra_completed=true", f.child.compacts, f.child.resumed, peerCompacts)
}
