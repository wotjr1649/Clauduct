package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// Probe native's effective role effort without a gateway role override. This
// discriminates client support from the routing rule that used to hide it.
func TestNativeCustomDefinitionEffortReachesRequest(t *testing.T) {
	buildHook(t)
	const role = `{"custom-proof":{"description":"Synthetic read-only role","prompt":"PUBLIC_CUSTOM_ROLE_PROOF","tools":["Read"],"model":"gpt-5.6-sol","effort":"medium"}}`
	script, _, result := scripted(t, []string{"-p", "delegate", "--allowedTools", "Agent", "--effort", "low", "--agents", role},
		toolStream("role_default", "Agent", `{"subagent_type":"custom-proof","description":"public role proof","prompt":"say ok"}`), textStream("child", "ok"), textStream("parent", "done"))
	if result.NativeExitCode != 0 {
		t.Fatalf("exit=%d", result.NativeExitCode)
	}
	found := false
	for _, raw := range script.Requests() {
		if !strings.Contains(raw, "PUBLIC_CUSTOM_ROLE_PROOF") {
			continue
		}
		var req struct {
			Model     string
			Reasoning struct{ Effort string }
		}
		if json.Unmarshal([]byte(raw), &req) != nil {
			t.Fatal("request parse")
		}
		found = true
		t.Logf("effective custom role: %s/%s", req.Model, req.Reasoning.Effort)
		if req.Model != "gpt-5.6-sol" || req.Reasoning.Effort != "medium" {
			t.Fatal("native role effort not applied")
		}
	}
	if !found {
		t.Fatal("custom child absent")
	}
}
