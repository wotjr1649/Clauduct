//go:build policy_evidence

package gateway

import (
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"strings"
	"testing"
)

// Opt-in acceptance probes for the decisions of 2026-09-18. Failures are evidence,
// not reasons to rewrite the expected behavior to match the current implementation.
func TestPolicyEvidenceUnverifiedAgentsNeverReachBackend(t *testing.T) {
	for _, role := range []string{"", "unknown-role", "Explore"} {
		t.Run("role="+role, func(t *testing.T) {
			f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
			g := startWith(t, f)
			g.EnableContextPolicy() // Production launcher activates this before native starts.
			if role != "" {
				resp := do(t, g, binding(`{"id":"proof_child","role":"`+role+`","stop":false}`))
				bodyText(t, resp)
			}
			rq := messages(strings.NewReader(validRequest))
			rq.headers["X-Claude-Code-Agent-Id"] = "proof_child"
			resp := do(t, g, rq)
			bodyText(t, resp)
			t.Logf("status=%d backend_calls=%d role=%q", resp.StatusCode, f.Calls(), role)
			if f.Calls() != 0 {
				t.Errorf("unverified selection/context policy reached backend")
			}
		})
	}
}

func TestPolicyEvidenceCountFailureDoesNotBlockGeneration(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, f)
	rq := messages(strings.NewReader(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"hello"}],"tools":[{"name":"example","input_schema":{"type":"object","properties":{}}}]}`))
	rq.path = "/v1/messages/count_tokens"
	failed := do(t, g, rq)
	bodyText(t, failed)
	ok := post(t, g, validRequest)
	bodyText(t, ok)
	t.Logf("count_status=%d generation_status=%d calls=%d", failed.StatusCode, ok.StatusCode, f.Calls())
	if failed.StatusCode < 400 || ok.StatusCode != 200 || f.Calls() != 1 {
		t.Fatal("count refusal contaminated generation")
	}
}
