package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNativeWorkflowRefusalIsToolErrorAndConversationContinues(t *testing.T) {
	buildHook(t)
	var observed atomic.Bool
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("discover", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("refused_workflow", "Workflow", `{"resumeFromRunId":"wf_missing"}`)},
		{When: func(body string) bool {
			ok := strings.Contains(body, "function_call_output") && strings.Contains(body, "refused_workflow") && strings.Contains(body, bridge.RejectedWorkflowReason)
			if ok {
				observed.Store(true)
			}
			return ok
		}, SSE: textStream("recovered", "PUBLIC_NATIVE_DENIAL_RECOVERED")},
	}, Default: textStream("side", "untitled")}
	out := (nativeRun{Args: []string{"-p", "attempt the public recovery then report failure", "--allowedTools", "ToolSearch,Workflow"}, transport: exactScript{script}}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || !observed.Load() || !strings.Contains(out.output(), "PUBLIC_NATIVE_DENIAL_RECOVERED") {
		for _, raw := range script.Conversations() {
			var req struct {
				Input []struct {
					Type   string
					CallID string `json:"call_id"`
					Output json.RawMessage
				}
			}
			_ = json.Unmarshal([]byte(raw), &req)
			for _, item := range req.Input {
				if item.Type == "function_call_output" {
					t.Logf("synthetic tool result %s: %s", item.CallID, item.Output)
				}
			}
		}
		t.Fatalf("native denial did not recover: %v %s", out.err, tail(out.output(), 1600))
	}
	if out.result.Diagnostics.Totals.RejectedWorkflowCalls != 1 || len(out.result.Diagnostics.Totals.Failures) != 0 {
		t.Fatal("denial not separated from API failure", out.result.Diagnostics.Totals)
	}
}

// Wait for retained gateway evidence; raw end files are consumed by the gateway.
// Reading those transient files races its reconciler and is not a reliable test.
type workflowLimitFixture struct {
	script  *upstream.Script
	gateway *gateway.Gateway
	parents atomic.Int32
}

const workflowLimitSession = "fed54165-e147-4ca5-8c6c-d45a092d1b21"

func (f *workflowLimitFixture) Execute(ctx context.Context, c upstream.Call) (*upstream.Response, error) {
	child, returned := workflowRequest(c.Body)
	if !child && returned {
		for {
			for _, entry := range f.gateway.Diagnose().AgentResults.Recent {
				if entry.Session == workflowLimitSession && entry.NativeEndObserved {
					// Native can query again when the asynchronous completion event
					// arrives. Answer that separate turn too, with a finite ceiling.
					if f.parents.Add(1) > 3 {
						return nil, upstream.ErrScriptExhausted
					}
					return (exactScript{&upstream.Script{Default: textStream("main", "PUBLIC_LIMIT_PARENT_DONE")}}).Execute(ctx, c)
				}
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	return (exactScript{f.script}).Execute(ctx, c)
}

func TestNativeWorkflowRoleMaxTurnsStopsAfterOneToolTurn(t *testing.T) {
	buildHook(t)
	_, cwd := workspace(t)
	file := filepath.Join(cwd, "public.txt")
	if err := os.WriteFile(file, []byte("PUBLIC_LIMIT_FILE"), 0600); err != nil {
		t.Fatal(err)
	}
	read, _ := json.Marshal(map[string]string{"file_path": file})
	input, _ := json.Marshal(map[string]string{"script": strings.Replace(probeWorkflow, "{ label: 'probe' }", "{label:'probe',agentType:'public-reader',model:'gpt-5.6-terra',effort:'medium'}", 1)})
	childRequests := 0
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("discover", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
		{When: func(body string) bool {
			child, _ := workflowRequest([]byte(body))
			if child {
				childRequests++
			}
			return child
		}, SSE: toolStream("read_once", "Read", string(read))},
		{When: func(body string) bool {
			child, _ := workflowRequest([]byte(body))
			if child {
				childRequests++
			}
			return child
		}, SSE: textStream("overrun", "PUBLIC_LIMIT_OVERRUN")},
		{When: func(body string) bool { _, returned := workflowRequest([]byte(body)); return returned }, SSE: textStream("main", "PUBLIC_LIMIT_PARENT_DONE")},
	}, Default: textStream("side", "untitled")}
	f := &workflowLimitFixture{script: script}
	out := (nativeRun{ObserveGateway: func(g *gateway.Gateway) { f.gateway = g }, Cwd: cwd, Args: []string{"--session-id", workflowLimitSession, "-p", "Run the public Workflow and wait for its result.", "--allowedTools", "ToolSearch,Workflow,Read", "--agents", `{"public-reader":{"description":"Public reader","prompt":"Read the assigned public file once.","tools":["Read"],"maxTurns":1}}`}, Env: map[string]string{"CLAUDE_CODE_MAX_RETRIES": "0"}, transport: f, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || childRequests != 1 || !strings.Contains(out.output(), "PUBLIC_LIMIT_PARENT_DONE") {
		_ = filepath.Walk(filepath.Join(out.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasPrefix(info.Name(), "agent-") || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			raw, _ := os.ReadFile(path)
			t.Logf("public child transcript tail: %s", tail(string(raw), 3000))
			return nil
		})
		t.Logf("limit diagnostics: %+v", out.result.Diagnostics.Recent)
		for _, body := range script.Requests() {
			var r struct {
				Input []struct {
					Type, CallID string
					Output       json.RawMessage
				}
				Tools []struct{ Name string }
			}
			_ = json.Unmarshal([]byte(body), &r)
			for _, item := range r.Input {
				if item.Type == "function_call_output" {
					t.Logf("public tool result: %s", tail(string(item.Output), 700))
				}
			}
		}
		t.Fatalf("native turn limit childRequests=%d err=%v exit=%d %s", childRequests, out.err, out.result.NativeExitCode, tail(out.output(), 1200))
	}
	readObserved := false
	err := filepath.Walk(filepath.Join(out.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasPrefix(info.Name(), "agent-") || !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		readObserved = readObserved || strings.Contains(string(raw), "PUBLIC_LIMIT_FILE")
		return nil
	})
	if err != nil || !readObserved {
		t.Fatalf("actual Read result=%v readErr=%v", readObserved, err)
	}
}

// probeWorkflow is the smallest script the Workflow tool accepts: a literal meta block and
// one delegated agent. One agent, because the question is whether a workflow's agents reach
// this bridge at all, not how many of them can.
const probeWorkflow = `export const meta = { name: 'probe', description: 'one delegated agent' }
const out = await agent('Reply with the single word: probe', { label: 'probe' })
return { out }`

func TestNativeWorkflowToolAllowlists(t *testing.T) {
	buildHook(t)
	for _, variant := range []string{"raw_read", "raw_none", "plan_read", "custom_role", "role_default", "role_effort", "script_path", "named_file"} {
		t.Run(variant, func(t *testing.T) {
			tools := []string{"Read"}
			if variant == "raw_none" {
				tools = []string{}
			}
			var input []byte
			cwd := ""
			args := []string{"-p", "run the public workflow", "--effort", "low", "--allowedTools", "ToolSearch,Workflow,Read"}
			if variant == "plan_read" {
				input, _ = json.Marshal(map[string]any{"script": "clauduct:plan-v1", "args": map[string]any{"steps": []any{map[string]any{"id": "A", "prompt": "Reply with the single word: probe", "tools": tools}}}})
			} else if variant == "custom_role" {
				input, _ = json.Marshal(map[string]string{"script": strings.Replace(probeWorkflow, "{ label: 'probe' }", "{label:'probe',agentType:'public-reader',model:'gpt-5.6-terra',effort:'medium'}", 1)})
				args = append(args, "--agents", `{"public-reader":{"description":"Public read-only worker","prompt":"PUBLIC_ROLE_INSTRUCTION_KEEP","tools":["Read"],"maxTurns":1}}`)
			} else if variant == "role_default" || variant == "role_effort" {
				options := "{label:'probe',agentType:'public-reader'}"
				if variant == "role_effort" {
					options = "{label:'probe',agentType:'public-reader',effort:'medium'}"
				}
				input, _ = json.Marshal(map[string]string{"script": strings.Replace(probeWorkflow, "{ label: 'probe' }", options, 1)})
				args = append(args, "--agents", `{"public-reader":{"description":"Public read-only worker","prompt":"PUBLIC_ROLE_INSTRUCTION_KEEP","tools":["Read"],"model":"gpt-5.6-terra","effort":"medium","maxTurns":1}}`)
			} else if variant == "script_path" || variant == "named_file" {
				_, cwd = workspace(t)
				dir := filepath.Join(cwd, ".claude", "workflows")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "public-probe.js")
				source := strings.Replace(probeWorkflow, "{ label: 'probe' }", "{label:'probe',tools:['Read']}", 1)
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				input, _ = json.Marshal(map[string]string{"scriptPath": path})
				if variant == "named_file" {
					input, _ = json.Marshal(map[string]string{"name": "public-probe"})
				}
			} else {
				encoded, _ := json.Marshal(tools)
				input, _ = json.Marshal(map[string]string{"script": strings.Replace(probeWorkflow, "{ label: 'probe' }", "{ label: 'probe', tools:"+string(encoded)+" }", 1)})
			}
			script := &upstream.Script{Turns: []upstream.ScriptTurn{
				{When: upstream.Conversation, SSE: toolStream("discover_wf", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
				{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
				{When: func(body string) bool { child, _ := workflowRequest([]byte(body)); return child }, SSE: textStream("wf_agent", "probe")},
				{When: func(body string) bool { _, returned := workflowRequest([]byte(body)); return returned }, SSE: textStream("main", "done")},
			}, Default: textStream("side", "untitled")}
			out := (nativeRun{Cwd: cwd, Args: args, transport: &workflowFixture{script: script, child: make(chan struct{})}, Timeout: 3 * defaultNativeTimeout, ContextPolicy: true}).run(t)
			if out.err != nil || out.result.NativeExitCode != 0 {
				_ = filepath.Walk(filepath.Join(out.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".meta.json") {
						raw, _ := os.ReadFile(path)
						var m struct {
							AgentType, Model, Description string
							SpawnDepth                    int
						}
						_ = json.Unmarshal(raw, &m)
						t.Logf("public Workflow metadata: %+v", m)
					}
					return nil
				})
				t.Logf("workflow diagnostics: %+v", out.result.Diagnostics.Recent)
				t.Fatalf("native workflow %v exit %d %s", out.err, out.result.NativeExitCode, tail(out.output(), 1200))
			}
			children := 0
			for _, body := range script.Requests() {
				child, _ := workflowRequest([]byte(body))
				if !child {
					continue
				}
				children++
				if !strings.Contains(body, "one assigned Workflow task") {
					t.Fatal("native workflow child lacks worker scope")
				}
				if (variant == "custom_role" || strings.HasPrefix(variant, "role_")) && (!strings.Contains(body, "PUBLIC_ROLE_INSTRUCTION_KEEP") || !strings.Contains(body, `"model":"gpt-5.6-terra"`) || !strings.Contains(body, `"effort":"medium"`)) {
					t.Fatal("custom role instructions or explicit route changed")
				}
				var request struct{ Tools []struct{ Name string } }
				_ = json.Unmarshal([]byte(body), &request)
				if len(request.Tools) != len(tools) || len(tools) > 0 && request.Tools[0].Name != "Read" {
					t.Fatalf("native child tool catalogue not restricted: %+v", request.Tools)
				}
			}
			if children != 1 || len(out.result.Diagnostics.Totals.Failures) != 0 {
				t.Fatalf("children=%d failures=%v", children, out.result.Diagnostics.Totals.Failures)
			}
		})
	}
}

// Print mode exits when the scripted parent finishes. Keep that parent alive
// until the asynchronously launched native workflow has actually sent its child.
type workflowFixture struct {
	t      *testing.T
	script *upstream.Script
	child  chan struct{}
	once   sync.Once
}

func (f *workflowFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }

func TestWorkflowFixtureDoesNotWaitBeforeWorkflowStarts(t *testing.T) {
	script := &upstream.Script{Default: textStream("side", "untitled")}
	for range 2 {
		response, err := script.Execute(context.Background(), upstream.Call{Body: []byte(`{"input":[]}`)})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	f := &workflowFixture{script: script, child: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	response, err := f.Execute(ctx, upstream.Call{Body: []byte(`{"tools":[],"input":[{"role":"user","content":[{"text":"run the workflow"}]}]}`)})
	if err != nil {
		t.Fatalf("auxiliary requests blocked the parent before Workflow could run: %v", err)
	}
	response.Body.Close()
}

func workflowRequest(body []byte) (isChild, workflowReturned bool) {
	var req struct {
		Input []struct {
			Type    string
			CallID  string `json:"call_id"`
			Role    string
			Content json.RawMessage
		}
	}
	_ = json.Unmarshal(body, &req)
	for _, entry := range req.Input {
		if entry.Type == "function_call_output" && entry.CallID == "call_wf_1" {
			workflowReturned = true
		}
		if entry.Role == "user" {
			var parts []struct{ Text string }
			if json.Unmarshal(entry.Content, &parts) == nil {
				for _, part := range parts {
					if strings.TrimSpace(part.Text) == "Reply with the single word: probe" ||
						(strings.HasPrefix(part.Text, "[Workflow harness — computed task]") && strings.HasSuffix(part.Text, "\n  Reply with the single word: probe")) {
						isChild = true
					}
				}
			}
		}
	}
	return
}

func (f *workflowFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	isChild, workflowReturned := workflowRequest(call.Body)
	if f.t != nil {
		f.t.Logf("workflow fixture: child=%v returned=%v previousCalls=%d", isChild, workflowReturned, f.script.Calls())
	}
	if isChild {
		f.once.Do(func() { close(f.child) })
	} else if workflowReturned {
		select {
		case <-f.child:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return (exactScript{f.script}).Execute(ctx, call)
}

// A workflow, end to end, through the real client.
//
// What was measured before was a workflow agent's routing, by hand, once. What nobody had
// run is the whole path as a test: the client executing a Workflow call this bridge
// delivered, the agents it starts coming back here as requests, and the account saying the
// right thing about them afterwards.
//
// The account is half the point. A workflow agent carries the role name "workflow-subagent"
// and no route of its own, and an earlier build counted that as a routing failure -- which
// made every session that ran a workflow report itself as having something wrong. That is
// the diagnosis that cries every time and so stops being read.
func TestAWorkflowsAgentsReachTheBridge(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)

	input, err := json.Marshal(map[string]string{"script": probeWorkflow})
	if err != nil {
		t.Fatalf("encode tool input: %v", err)
	}
	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: toolStream("discover_wf", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
			{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
			{When: func(body string) bool { child, _ := workflowRequest([]byte(body)); return child }, SSE: textStream("wf_agent", "probe")},
			{When: func(body string) bool { _, returned := workflowRequest([]byte(body)); return returned }, SSE: textStream("main", "done")},
		},
		Default: textStream("side", "untitled"),
	}

	var g *gateway.Gateway
	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args: []string{"-p", "run the workflow", "--allowedTools", "ToolSearch,Workflow",
			"--strict-mcp-config"},
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			started, err := gateway.Start(&workflowFixture{t: t, script: script, child: make(chan struct{})})
			if err == nil {
				started.EnableContextPolicy()
			}
			g = started
			return started, err
		},
	})
	if err != nil {
		t.Logf("public fixture stdout=%q stderr=%q diagnostics=%+v", tail(stdout.String(), 2000), tail(stderr.String(), 1000), result.Diagnostics.Recent)
		t.Fatalf("Run: %v", err)
	}

	unregistered, unrouted := g.Unrouted()

	// Which request is the workflow's agent is decided by what it was handed, not by where
	// it falls: the session can send a side request at any point. A workflow agent cannot
	// start another workflow, so the conversation carrying tools but no Workflow among them
	// is the agent's.
	var spawner, agent string
	for _, request := range script.Requests() {
		var envelope struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if json.Unmarshal([]byte(request), &envelope) != nil || len(envelope.Tools) == 0 {
			continue
		}
		where := envelope.Model + "/" + envelope.Reasoning.Effort
		carries := false
		for _, tool := range envelope.Tools {
			if tool.Name == "Workflow" {
				carries = true
				break
			}
		}
		if carries {
			spawner = where
			continue
		}
		if agent != "" && agent != where {
			t.Fatalf("two different workflow agent requests: %s and %s", agent, where)
		}
		agent = where
	}

	t.Logf("exit=%d requests=%d conversation=%d spawner=%s agent=%s unregistered=%d unrouted=%d",
		result.NativeExitCode, script.Calls(), len(script.Conversations()),
		spawner, agent, unregistered, unrouted)

	if agent == "" {
		for _, raw := range script.Requests() {
			var envelope struct {
				Input []struct {
					Type   string
					CallID string `json:"call_id"`
					Output json.RawMessage
				}
			}
			if json.Unmarshal([]byte(raw), &envelope) == nil {
				for _, item := range envelope.Input {
					if item.Type == "function_call_output" && item.CallID == "call_wf_1" {
						t.Logf("public workflow fixture result: %s", tail(string(item.Output), 1600))
					}
				}
			}
		}
		t.Fatal("no workflow agent request arrived: the client either refused the call or " +
			"the workflow never started, and nothing about workflows was measured")
	}
	// Omitted runtime options inherit the parent, proven by the script digest,
	// native journal/metadata, and the independently recorded active native turn.
	if agent != spawner {
		t.Fatalf("the workflow agent ran on %s while the session ran on %s; a workflow agent "+
			"inherits the session route", agent, spawner)
	}
	// And the account has nothing to report. A workflow agent has a role name and no route,
	// which an earlier build counted as a routing failure -- so every session that ran a
	// workflow reported itself as having something wrong.
	if unregistered != 0 || unrouted != 0 {
		t.Fatalf("counts = (%d, %d), want (0, 0): the hook reached the gateway and "+
			"workflow-subagent is an intentional inherit, not a failure", unregistered, unrouted)
	}
	if result.NativeExitCode != 0 {
		t.Fatalf("exit = %d", result.NativeExitCode)
	}
	verified := false
	for _, entry := range result.Diagnostics.Recent {
		if entry.Source == "workflow-selection" {
			if entry.SelectionVerified == nil || !*entry.SelectionVerified || entry.UsageSource != "backend" || entry.InputTokens == nil || entry.OutputTokens == nil {
				t.Fatal("workflow selection or measured usage missing")
			}
			verified = true
		}
	}
	if !verified {
		t.Fatal("no production policy workflow request observed")
	}
	presenceVerified := false
	for _, entry := range result.Diagnostics.AgentSelections.Recent {
		if entry.Source == "workflow-selection" && entry.PresenceVerified && !entry.ModelProvided && !entry.EffortProvided {
			presenceVerified = true
		}
	}
	if !presenceVerified {
		t.Fatal("original omitted Workflow options not verified")
	}
}
