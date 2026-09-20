package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type resumeContextFixture struct {
	file             string
	reads            int
	routes           []string
	compactRoutes    []string
	quiet            bool
	overThreshold    bool
	historyPreserved bool
}

func (f *resumeContextFixture) Count(_ context.Context, call upstream.Call) (int64, error) {
	if f.overThreshold && call.Model == "gpt-5.6-sol" {
		return 239000, nil
	}
	return 1000, nil
}
func (f *resumeContextFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	text := string(call.Body)
	reply := textStream("side", "title")
	if strings.Contains(text, "CRITICAL: Respond with TEXT ONLY") {
		f.overThreshold = false
		f.compactRoutes = append(f.compactRoutes, call.Model+"/"+call.Effort)
		reply = textStream("compact", "<summary>RESUME_SUMMARY: the five public file reads succeeded. The public marker is PUBLIC_RESUME_MARKER. No changes were made; continue the user's request.</summary>")
	} else if upstream.Conversation(text) {
		if call.Model == "gpt-5.6-sol" {
			f.historyPreserved = strings.Contains(text, "PUBLIC_RESUME_MARKER")
		}
		f.routes = append(f.routes, call.Model+"/"+call.Effort)
		if !f.quiet && !fixtureHasTool(call, "Read") {
			reply = toolStream("discover", "ToolSearch", `{"query":"select:Read","max_results":1}`)
		} else if f.reads < 5 {
			f.reads++
			args, _ := json.Marshal(map[string]string{"file_path": f.file})
			reply = toolStream("resume_read_"+string(rune('0'+f.reads)), "Read", string(args))
		} else {
			reply = textStream("ready", "PUBLIC_RESUME_MARKER ready")
		}
	}
	used := int64(1000)
	if f.overThreshold && call.Model == "gpt-6-astra" && f.reads >= 5 {
		used = 239000 // Persist actual prior usage, below Astra's own compact target.
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, used)}).Execute(ctx, call)
}

func TestNativeResumeSwitchCompactsOnPersistedOldModelAtDestinationThreshold(t *testing.T) {
	buildHook(t)
	_, cwd := workspace(t)
	config := t.TempDir()
	file := filepath.Join(cwd, "public.txt")
	if os.WriteFile(file, []byte("PUBLIC_RESUME_MARKER"), 0600) != nil {
		t.Fatal("fixture")
	}
	f := &resumeContextFixture{file: file, overThreshold: true}
	const id = "40a09ff1-629a-49fd-b572-d7b998e391a6"
	first := (nativeRun{Cwd: cwd, ConfigDir: config, Args: []string{"-p", "Read public file five times", "--session-id", id, "--model", "gpt-6-astra", "--effort", "high", "--allowedTools", "ToolSearch,Read"}, transport: f, ContextPolicy: true, Timeout: 45 * time.Second}).run(t)
	if first.err != nil || first.result.NativeExitCode != 0 || f.reads != 5 {
		t.Fatalf("initial failed: exit=%d err=%v reads=%d", first.result.NativeExitCode, first.err, f.reads)
	}
	second := (nativeRun{Cwd: cwd, ConfigDir: config, Args: []string{"-p", "Continue and report the marker", "--resume", id, "--model", "gpt-5.6-sol", "--effort", "medium", "--allowedTools", "ToolSearch,Read"}, transport: f, ContextPolicy: true, Timeout: 45 * time.Second}).run(t)
	if second.err != nil || second.result.NativeExitCode != 0 || len(f.compactRoutes) != 1 || f.compactRoutes[0] != "gpt-6-astra/high" || f.routes[len(f.routes)-1] != "gpt-5.6-sol/medium" {
		t.Fatalf("resume: exit=%d err=%v compaction=%v routes=%v output=%s", second.result.NativeExitCode, second.err, f.compactRoutes, f.routes, tail(second.output(), 1200))
	}
	if len(second.result.Diagnostics.AgentContexts) != 1 || !second.result.Diagnostics.AgentContexts[0].Persistent {
		t.Fatal("native session journal absent")
	}
	t.Log("actual native process restart: destination threshold reached; old astra/high compaction, new sol/medium generation")
}

func TestNativeResumeSwitchWithOneShortTurn(t *testing.T) {
	buildHook(t)
	_, cwd := workspace(t)
	config := t.TempDir()
	f := &resumeContextFixture{reads: 5, quiet: true}
	const id = "93c84b21-b687-4913-b9da-661d3711c681"
	first := (nativeRun{Cwd: cwd, ConfigDir: config, Args: []string{"-p", "Public short first turn", "--session-id", id, "--model", "gpt-6-astra", "--effort", "high"}, transport: f, ContextPolicy: true}).run(t)
	if first.err != nil || first.result.NativeExitCode != 0 || len(f.routes) != 1 {
		t.Fatal("initial short turn failed")
	}
	second := (nativeRun{Cwd: cwd, ConfigDir: config, Args: []string{"-p", "Continue public short turn", "--resume", id, "--model", "gpt-5.6-sol", "--effort", "medium"}, transport: f, ContextPolicy: true}).run(t)
	if second.err != nil || second.result.NativeExitCode != 0 || len(f.compactRoutes) != 0 || !f.historyPreserved || f.routes[len(f.routes)-1] != "gpt-5.6-sol/medium" {
		t.Fatalf("short switch: exit=%d err=%v compacts=%v routes=%v output=%s", second.result.NativeExitCode, second.err, f.compactRoutes, f.routes, tail(second.output(), 1500))
	}
}
