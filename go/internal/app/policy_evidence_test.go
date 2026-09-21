//go:build policy_evidence

package app

import (
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyEvidenceNativeDeferredToolRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "proof.txt")
	if err := os.WriteFile(file, []byte("POLICY_EVIDENCE_FILE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"file_path": file})
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("proof_search", "ToolSearch", `{"query":"select:Read","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("proof_read", "Read", string(args))},
		{When: upstream.Conversation, SSE: textStream("proof_done", "PROOF_DONE")},
	}, Default: textStream("side", "untitled")}
	buildHook(t)
	out := (nativeRun{Args: []string{"-p", "Run local synthetic tool proof", "--allowedTools", "ToolSearch,Read"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
	if out.err != nil {
		t.Fatal(out.err)
	}
	found := false
	for _, s := range script.Requests() {
		if strings.Contains(s, "POLICY_EVIDENCE_FILE") {
			found = true
		}
	}
	t.Logf("native_exit=%d backend_requests=%d file_result_reached_backend=%v", out.result.NativeExitCode, len(script.Requests()), found)
	if out.result.NativeExitCode != 0 || !found {
		t.Fatal("native -> gateway -> ToolSearch -> Read -> backend round trip failed")
	}
}

func TestPolicyEvidenceNativeDeferredMCPRoundTrip(t *testing.T) {
	config := stubServer(t)
	script := &upstream.Script{Turns: []upstream.ScriptTurn{
		{When: upstream.Conversation, SSE: toolStream("proof_search", "ToolSearch", `{"query":"select:mcp__stub__report","max_results":1}`)},
		{When: upstream.Conversation, SSE: toolStream("proof_mcp", "mcp__stub__report", `{"name":"POLICY_EVIDENCE_SYNTHETIC"}`)},
		{When: upstream.Conversation, SSE: textStream("proof_done", "PROOF_DONE")},
	}, Default: textStream("side", "untitled")}
	buildHook(t)
	out := (nativeRun{Args: []string{"-p", "Run local synthetic MCP proof", "--mcp-config", config, "--allowedTools", "ToolSearch,mcp__stub__report"}, Env: map[string]string{"POLICY_EVIDENCE_SYNTHETIC": "PUBLIC_TEST_VALUE"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
	if out.err != nil {
		t.Fatal(out.err)
	}
	found := false
	for _, s := range script.Requests() {
		if strings.Contains(s, "PRESENT:PUBLIC_TEST_VALUE") {
			found = true
		}
	}
	t.Logf("native_exit=%d backend_requests=%d mcp_result_reached_backend=%v", out.result.NativeExitCode, len(script.Requests()), found)
	if out.result.NativeExitCode != 0 || !found {
		t.Fatal("native -> gateway -> ToolSearch -> MCP -> backend round trip failed")
	}
}
