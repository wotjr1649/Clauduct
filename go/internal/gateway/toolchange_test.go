package gateway

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// A conversation that defines two tools and then changes the set mid-session.
func withToolChange(change string) string {
	return `{"model":"gpt-6-astra","max_tokens":64,"stream":true,
	  "tools":[{"name":"Read","description":"r","input_schema":{"type":"object"}},
	           {"name":"Write","description":"w","input_schema":{"type":"object"}}],
	  "messages":[{"role":"user","content":"hello"},
	              {"role":"system","content":[` + change + `]}]}`
}

func postWithBeta(t *testing.T, g *Gateway, beta, body string) *http.Response {
	t.Helper()
	rq := messages(strings.NewReader(body))
	if beta != "" {
		rq.headers["Anthropic-Beta"] = beta
	}
	return do(t, g, rq)
}

// A4. A withdrawn tool does not go upstream, so the backend cannot call something the
// client has just said it will not run.
func TestAWithdrawnToolIsNotSentUpstream(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, fixture)

	resp := postWithBeta(t, g, betaToolChanges, withToolChange(
		`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Write"}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}

	sent := fixture.LastRequest()
	if !strings.Contains(sent, `"name":"Read"`) {
		t.Errorf("a tool nobody withdrew went missing: %s", sent)
	}
	if strings.Contains(sent, `"name":"Write"`) {
		t.Errorf("a withdrawn tool was still offered to the backend: %s", sent)
	}
	// The change itself is not content. Describing the edit to the model would be putting
	// bookkeeping into the conversation.
	if strings.Contains(sent, "tool_removal") {
		t.Errorf("the change block travelled as input: %s", sent)
	}
}

// And an addition puts back what a removal took, in the order the turns arrived.
func TestAnAdditionUndoesAnEarlierRemoval(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, fixture)

	resp := postWithBeta(t, g, betaToolChanges, withToolChange(
		`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Write"}},
		 {"type":"tool_addition","tool":{"type":"tool_reference","name":"Write"}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	if sent := fixture.LastRequest(); !strings.Contains(sent, `"name":"Write"`) {
		t.Errorf("a tool that was added back is still missing: %s", sent)
	}
}

// Without the beta the capability was never negotiated, so it is refused rather than
// honoured quietly.
func TestAToolChangeWithoutItsBetaIsRefused(t *testing.T) {
	for name, beta := range map[string]string{
		"no beta header at all":   "",
		"a different beta":        "web-search-2025-03-05",
		"a near miss in the name": "mid-conversation-tool-changes-2026-07-02",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]")}
			g := startWith(t, fixture)

			resp := postWithBeta(t, g, beta, withToolChange(
				`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Write"}}`))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, "UNSUPPORTED_TOOL_CHANGE") {
				t.Fatalf("body = %s", body)
			}
			if fixture.Calls() != 0 {
				t.Errorf("it reached the backend %d times", fixture.Calls())
			}
		})
	}
}

// The beta is found however the client packs the header.
func TestTheToolChangeBetaIsFoundInEveryHeaderShape(t *testing.T) {
	for name, beta := range map[string]string{
		"alone":             betaToolChanges,
		"in a list":         "web-search-2025-03-05," + betaToolChanges + ",oauth-2025-04-20",
		"with extra spaces": " web-search-2025-03-05 ,  " + betaToolChanges + " ",
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")})
			resp := postWithBeta(t, g, beta, withToolChange(
				`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Write"}}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
			}
		})
	}
}

// Each malformed change is refused for what is actually wrong with it.
func TestAMalformedToolChangeIsRefusedByWhatIsWrongWithIt(t *testing.T) {
	for _, c := range []struct{ name, change, code string }{
		{"a name this request never defined",
			`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Nonexistent"}}`,
			"INVALID_TOOL_REFERENCE"},
		{"a reference that is not one",
			`{"type":"tool_addition","tool":{"type":"text","name":"Read"}}`,
			"INVALID_TOOL_REFERENCE"},
		{"an unknown key on the block",
			`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Read"},"why":"x"}`,
			"TOOL_CHANGE_FIELDS"},
		{"an unknown key on the reference",
			`{"type":"tool_removal","tool":{"type":"tool_reference","name":"Read","extra":1}}`,
			"TOOL_REFERENCE_FIELDS"},
		{"no tool named at all",
			`{"type":"tool_removal"}`,
			"TOOL_CHANGE_FIELDS"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]")}
			g := startWith(t, fixture)

			resp := postWithBeta(t, g, betaToolChanges, withToolChange(c.change))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, c.code) {
				t.Fatalf("body = %s, want %s", body, c.code)
			}
			if fixture.Calls() != 0 {
				t.Errorf("it reached the backend %d times", fixture.Calls())
			}
		})
	}
}

// A tool set is the session's to change, not the model's or the user's.
func TestAToolChangeOnAnyTurnButASystemTurnIsRefused(t *testing.T) {
	for _, role := range []string{"user", "assistant"} {
		t.Run(role, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: sse(created, completed, "[DONE]")})
			resp := postWithBeta(t, g, betaToolChanges, `{"model":"gpt-6-astra","max_tokens":64,
			  "stream":true,
			  "tools":[{"name":"Read","description":"r","input_schema":{"type":"object"}}],
			  "messages":[{"role":"`+role+`","content":[
			    {"type":"tool_removal","tool":{"type":"tool_reference","name":"Read"}}]}]}`)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, "UNSUPPORTED_TOOL_CHANGE") {
				t.Fatalf("body = %s", body)
			}
		})
	}
}
