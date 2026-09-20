package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNativeWorkflowResultsSurviveLauncherRestart(t *testing.T) {
	buildHook(t)
	const session = "2e629861-40c4-4301-b6df-a03ed0519a57"
	input, _ := json.Marshal(map[string]string{"script": probeWorkflow})
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("discover_wf", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
		{When: func(body string) bool { child, _ := workflowRequest([]byte(body)); return child }, SSE: textStream("wf_agent", "probe")},
		{When: func(body string) bool { _, returned := workflowRequest([]byte(body)); return returned }, SSE: textStream("main", "PUBLIC_FIRST_DONE")},
	}, Default: textStream("side", "untitled")}
	first := (nativeRun{Args: []string{"--session-id", session, "-p", "Run the public workflow", "--allowedTools", "ToolSearch,Workflow"}, transport: &workflowFixture{script: script, child: make(chan struct{})}, ContextPolicy: true}).run(t)
	if first.err != nil || first.result.NativeExitCode != 0 || first.result.Diagnostics.WorkflowPersistence.Saved != 1 || first.result.Diagnostics.WorkflowPersistence.Failed != 0 {
		t.Fatalf("first native run: %v exit=%d persistence=%+v %s", first.err, first.result.NativeExitCode, first.result.Diagnostics.WorkflowPersistence, tail(first.output(), 1000))
	}
	runID := ""
	err := filepath.Walk(filepath.Join(first.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".clauduct-workflow.json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var saved struct {
			Link struct {
				Run string `json:"runId"`
			}
		}
		if err = json.Unmarshal(raw, &saved); err != nil {
			return err
		}
		runID = saved.Link.Run
		return nil
	})
	if err != nil || runID == "" {
		t.Fatal("missing persisted run", err)
	}
	resume, _ := json.Marshal(map[string]string{"resumeFromRunId": runID})
	publicFile := filepath.Join(first.cwd, "public-next-turn.txt")
	if err := os.WriteFile(publicFile, []byte("PUBLIC_NEXT_TURN"), 0600); err != nil {
		t.Fatal(err)
	}
	secondScript := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("discover_recovery", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("resume_call", "Workflow", string(resume))},
	}, Default: textStream("side", "untitled")}
	f := &workflowRestartFixture{script: secondScript, file: publicFile}
	second := (nativeRun{ConfigDir: first.configDir, Cwd: first.cwd, Args: []string{"--resume", session, "-p", "Recover the stored workflow result; read public-next-turn.txt once, then report the stored result; never run its agent again.", "--allowedTools", "ToolSearch,Workflow,Read"}, transport: f, ContextPolicy: true}).run(t)
	if second.err != nil || second.result.NativeExitCode != 0 || f.children.Load() != 0 || !strings.Contains(second.output(), "PUBLIC_RESTART_DONE") || second.result.Diagnostics.WorkflowPersistence.Restored != 1 || second.result.Diagnostics.Totals.RejectedWorkflowCalls != 0 || len(second.result.Diagnostics.Totals.Failures) != 0 {
		t.Fatalf("resumed native: %v exit=%d children=%d evidence=%+v %s", second.err, second.result.NativeExitCode, f.children.Load(), second.result.Diagnostics.WorkflowPersistence, tail(second.output(), 2200))
	}
	if !f.report.Load() {
		t.Logf("public recovery fixture: calls=%d outputs=%s", f.calls.Load(), f.outputs)
		t.Logf("first public native ends: %+v", first.result.Diagnostics.AgentResults.Recent)
		t.Fatal("parent never received verified stored body")
	}
}

// The requested next action keeps the print-mode parent running while native
// delivers its completion event. No model polling or fabricated task result.
type workflowRestartFixture struct {
	script   *upstream.Script
	children atomic.Int32
	calls    atomic.Int32
	report   atomic.Bool
	outputs  string
	file     string
}

func (f *workflowRestartFixture) Execute(ctx context.Context, c upstream.Call) (*upstream.Response, error) {
	if child, _ := workflowRequest(c.Body); child {
		f.children.Add(1)
		return nil, upstream.ErrScriptExhausted
	}
	if !upstream.Conversation(string(c.Body)) {
		return (exactScript{f.script}).Execute(ctx, c)
	}
	n := f.calls.Add(1)
	if n <= 2 {
		return (exactScript{f.script}).Execute(ctx, c)
	}
	if n > 5 {
		return nil, upstream.ErrScriptExhausted
	}
	if strings.Contains(string(c.Body), "completed_result_reused") && strings.Contains(string(c.Body), "probe") {
		f.report.Store(true)
	}
	var req struct {
		Input []struct {
			Type   string
			Call   string `json:"call_id"`
			Output json.RawMessage
		}
	}
	_ = json.Unmarshal(c.Body, &req)
	for _, item := range req.Input {
		if item.Type != "function_call_output" {
			continue
		}
		var output string
		_ = json.Unmarshal(item.Output, &output)
		f.outputs += string(item.Output) + "\n"
		if strings.Contains(output, "completed_result_reused") && strings.Contains(output, "probe") {
			f.report.Store(true)
		}
	}
	if n == 3 {
		args, _ := json.Marshal(map[string]any{"file_path": f.file})
		return (exactScript{&upstream.Script{Default: toolStream("read_next", "Read", string(args))}}).Execute(ctx, c)
	}
	return (exactScript{&upstream.Script{Default: textStream("recovered", "PUBLIC_RESTART_DONE")}}).Execute(ctx, c)
}
