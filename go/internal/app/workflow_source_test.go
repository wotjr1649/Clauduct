package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNativeWorkflowSourceReadDenialCannotStartAgent(t *testing.T) {
	buildHook(t)
	_, cwd := workspace(t)
	path := filepath.Join(cwd, "denied.js")
	if err := os.WriteFile(path, []byte(probeWorkflow), 0600); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"scriptPath": path})
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("discover", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("denied_source", "Workflow", string(input))},
		{When: upstream.Conversation, SSE: textStream("after", "PUBLIC_SOURCE_DENIED_RECOVERED")},
	}, Default: textStream("side", "untitled")}
	out := (nativeRun{Cwd: cwd, Args: []string{"-p", "Attempt the public workflow, then report its denial.", "--allowedTools", "ToolSearch,Workflow", "--disallowedTools", "Read"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || !strings.Contains(out.output(), "PUBLIC_SOURCE_DENIED_RECOVERED") || len(out.result.Diagnostics.AgentSelections.Recent) != 0 {
		t.Fatalf("native source denial %v exit=%d %s", out.err, out.result.NativeExitCode, tail(out.output(), 1500))
	}
	for _, body := range script.Requests() {
		if child, _ := workflowRequest([]byte(body)); child {
			t.Fatal("denied source executed an agent")
		}
	}
}

func TestNativeWorkflowPipelineAndNestedParallelUseVerifiedAgent(t *testing.T) {
	buildHook(t)
	for _, body := range []string{
		"return await pipeline([1],x=>agent('Reply with the single word: probe',{label:'probe',model:'gpt-5.6-luna',tools:[]}));",
		"return await parallel([()=>parallel([()=>agent('Reply with the single word: probe',{label:'probe',model:'gpt-5.6-luna',tools:[]})])]);",
	} {
		input, _ := json.Marshal(map[string]string{"script": "export const meta={name:'helpers',description:'public helper proof'};\n" + body})
		script := &upstream.Script{Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: toolStream("discover", "ToolSearch", `{"query":"select:Workflow","max_results":1}`)},
			{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
			{When: func(body string) bool { child, _ := workflowRequest([]byte(body)); return child }, SSE: textStream("worker", "probe")},
			{When: func(body string) bool { _, returned := workflowRequest([]byte(body)); return returned }, SSE: textStream("parent", "PUBLIC_HELPER_DONE")},
		}, Default: textStream("side", "untitled")}
		out := (nativeRun{Args: []string{"-p", "Run the public helper proof", "--allowedTools", "ToolSearch,Workflow"}, transport: &workflowFixture{script: script, child: make(chan struct{})}, ContextPolicy: true}).run(t)
		if out.err != nil || out.result.NativeExitCode != 0 || len(out.result.Diagnostics.Totals.Failures) > 0 {
			t.Fatalf("native helper failed: %v %s", out.err, tail(out.output(), 1000))
		}
		children := 0
		for _, request := range script.Requests() {
			child, _ := workflowRequest([]byte(request))
			if !child {
				continue
			}
			children++
			var r struct {
				Model     string
				Reasoning struct{ Effort string }
				Tools     []any
			}
			_ = json.Unmarshal([]byte(request), &r)
			if r.Model != "gpt-5.6-luna" || r.Reasoning.Effort != "max" || len(r.Tools) != 0 {
				t.Fatal("helper bypassed model/effort/tool policy")
			}
		}
		if children != 1 {
			t.Fatal("wrong native helper child count", children)
		}
	}
}
