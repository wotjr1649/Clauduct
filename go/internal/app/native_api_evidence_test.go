//go:build policy_evidence

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Native's own API declarations are public program data. Generate them in an
// isolated project with no plugins/MCP, no transport and no model inference.
func TestPolicyEvidenceNativeFunctionHookDeclarations(t *testing.T) {
	_, cwd := workspace(t)
	out := (nativeRun{Cwd: cwd, Args: []string{"-p", "/plugin-types"}, Env: map[string]string{"CLAUDE_CODE_ENABLE_FUNCTION_HOOKS": "1"}}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || out.backendCalls != 0 {
		t.Fatalf("declarations: exit=%d calls=%d err=%v output=%s", out.result.NativeExitCode, out.backendCalls, out.err, tail(out.output(), 1200))
	}
	path := filepath.Join(cwd, ".claude", "types", "claude-code.d.ts")
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 2<<20 {
		t.Fatalf("declarations not written: %v output=%s", err, tail(out.output(), 1200))
	}
	dest := filepath.Join("..", "..", "..", "verification", "policy-evidence-20260918", "native-function-api.d.ts")
	if err := os.WriteFile(dest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("native API declarations=%d bytes, model calls=0", len(raw))
}

func TestPolicyEvidenceNativeFunctionEvents(t *testing.T) {
	if os.Getenv("CLAUDUCT_FUNCTION_HOOK_PROBE") != "1" {
		t.Skip("experimental native function hooks require an explicitly trusted workspace; not a production dependency")
	}
	buildHook(t)
	var mu sync.Mutex
	var events []map[string]any
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event map[string]any
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&event) != nil {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer sink.Close()
	plugin := t.TempDir()
	for _, dir := range []string{".claude-plugin", "hooks"} {
		if err := os.MkdirAll(filepath.Join(plugin, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		".claude-plugin/plugin.json": `{"name":"public-native-event-proof","version":"1.0.0"}`,
		"hooks/hooks.json":           `{"modules":["./events.mjs"]}`,
		"hooks/events.mjs": fmt.Sprintf(`export const register = (on) => {
  on("agent.spawn", async ($, e, next) => {
    await $.http.fetch(%q, {method:"POST",body:JSON.stringify({kind:"spawn-before",call:e.tool_use_id,parent:e.parentAgentId,role:e.subagentType,model:e.model,parentModel:e.parentModel})});
    const result=await next(e);
    await $.http.fetch(%q, {method:"POST",body:JSON.stringify({kind:"spawn-after",call:e.tool_use_id,agent:result.agentId,model:result.model})});
    return result;
  });
  on("turn.step", async function* ($, e, next) {
    await $.http.fetch(%q, {method:"POST",body:JSON.stringify({kind:"step",agent:e.agentId,model:e.model,effort:e.effort,index:e.index})});
    return yield* next(e);
  });
  on("turn.complete", async ($, e, next) => {
    await $.http.fetch(%q, {method:"POST",body:JSON.stringify({kind:"complete",agent:e.agentId,reason:e.reason,answer:e.answer})});
    return next(e);
  });
};`, sink.URL, sink.URL, sink.URL, sink.URL),
	}
	for path, body := range files {
		if err := os.WriteFile(filepath.Join(plugin, path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	validation := exec.CommandContext(ctx, nativeAvailable(t), "plugin", "validate", plugin)
	validation.Dir = plugin
	validation.Env = append(os.Environ(), "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1", "CLAUDE_CONFIG_DIR="+t.TempDir())
	validated, validateErr := validation.CombinedOutput()
	t.Logf("plugin validation err=%v output=%s", validateErr, tail(string(validated), 3500))
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("delegate", "Agent", `{"subagent_type":"reviewer","description":"proof","prompt":"say public result"}`)},
		{When: upstream.Conversation, SSE: textStream("child", "PUBLIC_CHILD_RESULT")},
		{When: upstream.Conversation, SSE: textStream("parent", "PUBLIC_PARENT_RESULT")},
	}, Default: textStream("side", "title")}
	out := (nativeRun{Args: []string{"-p", "delegate public proof", "--effort", "low", "--allowedTools", "Agent", "--plugin-dir", plugin, "--agents", `{"reviewer":{"description":"public proof","prompt":"public fixture","tools":["Read"],"model":"gpt-5.6-sol","effort":"medium"}}`}, Env: map[string]string{"CLAUDE_CODE_ENABLE_FUNCTION_HOOKS": "1"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
	mu.Lock()
	defer mu.Unlock()
	for _, event := range events {
		raw, _ := json.Marshal(event)
		t.Log(string(raw))
	}
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("exit=%d err=%v output=%s", out.result.NativeExitCode, out.err, tail(out.output(), 1500))
	}
	childStep, childComplete := false, false
	for _, e := range events {
		if e["kind"] == "step" && e["agent"] != nil && e["model"] == "gpt-5.6-sol" && e["effort"] == "medium" {
			childStep = true
		}
		if e["kind"] == "complete" && e["agent"] != nil && e["answer"] == "PUBLIC_CHILD_RESULT" {
			childComplete = true
		}
	}
	if !childStep || !childComplete {
		t.Fatalf("resolved step=%v child complete=%v output=%s", childStep, childComplete, tail(out.output(), 2500))
	}
}
