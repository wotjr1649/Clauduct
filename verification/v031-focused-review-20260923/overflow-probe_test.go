package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Review probe: a structured backend overflow after dispatch, then native compaction
// and native's own retry of the same turn. Synthetic backend, real native + gateway.
type overflowProbe struct {
	mu                           sync.Mutex
	file                         string
	conversations, compacts      int
	overflowed, read, resumed    bool
}

func (f *overflowProbe) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }

func (f *overflowProbe) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := string(call.Body)
	reply := textStream("side", "untitled")
	if strings.Contains(body, "CRITICAL: Respond with TEXT ONLY") {
		f.compacts++
		reply = textStream("summary", "<summary>OVERFLOW_SYNTHETIC_DONE: the public file was read. Continue by reporting OVERFLOW_RESUMED.</summary>")
	} else if upstream.Conversation(body) {
		f.conversations++
		switch {
		case f.compacts > 0:
			f.resumed = strings.Contains(body, "OVERFLOW_SYNTHETIC_DONE")
			reply = textStream("resumed", "OVERFLOW_RESUMED")
		case !fixtureHasTool(call, "Read"):
			reply = toolStream("discover_read", "ToolSearch", `{"query":"select:Read","max_results":1}`)
		case !f.read:
			f.read = true
			args, _ := json.Marshal(map[string]string{"file_path": f.file})
			reply = toolStream("call_read", "Read", string(args))
		case !f.overflowed:
			f.overflowed = true
			return nil, upstream.Failure{Category: "CONTEXT_LENGTH_EXCEEDED", Status: 400}
		}
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}

func TestReviewProbeBackendOverflowRetry(t *testing.T) {
	buildHook(t)
	file := filepath.Join(t.TempDir(), "public.txt")
	if err := os.WriteFile(file, []byte("public synthetic overflow proof"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &overflowProbe{file: file}
	out := (nativeRun{Args: []string{"-p", "Run the public overflow proof", "--model", "gpt-5.6-sol", "--effort", "high", "--allowedTools", "ToolSearch,Read"}, transport: f, ContextPolicy: true, Timeout: 60 * time.Second}).run(t)
	var categories []string
	for _, r := range out.result.Diagnostics.RecentFailures {
		categories = append(categories, r.Category)
	}
	t.Logf("probe: exit=%d err=%v conversations=%d compacts=%d overflowed=%v resumed=%v failures=%v", out.result.NativeExitCode, out.err, f.conversations, f.compacts, f.overflowed, f.resumed, categories)
	for _, k := range gateway.ReviewProbeKeys {
		t.Logf("ledger key: %s", k)
	}
	for _, r := range out.result.Diagnostics.Recent {
		t.Logf("record: class=%s kind=%s status=%d category=%s control=%s", r.RequestClass, r.Kind, r.Status, r.Category, r.Control)
	}
	if out.err != nil || out.result.NativeExitCode != 0 || f.compacts != 1 || !f.resumed {
		t.Fatalf("overflow retry did not resume; output tail: %s", tail(out.output(), 800))
	}
}
