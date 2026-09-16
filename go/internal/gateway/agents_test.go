package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func binding(body string) request {
	return request{
		method:  http.MethodPost,
		path:    "/clauduct/agents",
		headers: map[string]string{"Content-Type": "application/json"},
		body:    strings.NewReader(body),
	}
}

func registeredBy(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var reply struct {
		Registered bool `json:"registered"`
	}
	if err := json.Unmarshal([]byte(bodyText(t, resp)), &reply); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return reply.Registered
}

// C1. A subagent exists because the client started one, so the client is the only thing
// that can say so -- and what the registration is for is deciding which model its requests
// run on.
func TestASubagentRegistersAndWithdraws(t *testing.T) {
	g := start(t)

	started := do(t, g, binding(`{"id":"agent_1","role":"Explore","stop":false,
	  "sessionId":"1b0b3297-ade4-4aa4-86c8-80eaf43db0a3",
	  "transcriptPath":"C:/Users/x/.claude/projects/p/s.jsonl"}`))
	if started.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", started.StatusCode, bodyText(t, started))
	}
	if !registeredBy(t, started) {
		t.Fatal("a SubagentStart did not register")
	}
	if role, known := g.agents.roleOf("agent_1"); !known || role != "Explore" {
		t.Fatalf("role = %q (known=%v), want Explore", role, known)
	}

	stopped := do(t, g, binding(`{"id":"agent_1","role":"Explore","stop":true}`))
	if stopped.StatusCode != http.StatusOK || registeredBy(t, stopped) {
		t.Fatalf("a SubagentStop left it registered: %s", bodyText(t, stopped))
	}
	if _, known := g.agents.roleOf("agent_1"); known {
		t.Fatal("the registration outlived its stop")
	}
}

// The hook runs inside the client. This build started that process but did not write what
// it sends, so a report is checked like anything else that crosses a boundary.
func TestAReportThatIsNotABindingIsRefused(t *testing.T) {
	for _, c := range []struct{ name, body, code string }{
		{"no identifier", `{"role":"Explore","stop":false}`, "INVALID_AGENT_BINDING"},
		{"an identifier that is not one", `{"id":"../etc","role":"Explore","stop":false}`,
			"INVALID_AGENT_BINDING"},
		{"no role", `{"id":"agent_1","stop":false}`, "INVALID_AGENT_BINDING"},
		{"an empty role", `{"id":"agent_1","role":"","stop":false}`, "INVALID_AGENT_BINDING"},
		{"a role longer than the bound", `{"id":"agent_1","role":"` + strings.Repeat("r", 201) +
			`","stop":false}`, "INVALID_AGENT_BINDING"},
		{"no stop flag", `{"id":"agent_1","role":"Explore"}`, "INVALID_AGENT_BINDING"},
		{"a stop flag that is not a boolean", `{"id":"agent_1","role":"Explore","stop":"yes"}`,
			"INVALID_AGENT_BINDING"},
		{"a key the binding does not have",
			`{"id":"agent_1","role":"Explore","stop":false,"prompt":"secret"}`,
			"INVALID_AGENT_BINDING"},
		// The length bound on the path is unreachable through the endpoint -- the body
		// limit is the same number and fires first -- so what is checked here is the type.
		{"a transcript path that is not a path",
			`{"id":"agent_1","role":"Explore","stop":false,"transcriptPath":42}`,
			"INVALID_AGENT_BINDING"},
		{"a session identifier that is not one",
			`{"id":"agent_1","role":"Explore","stop":false,"sessionId":"has spaces"}`,
			"INVALID_AGENT_BINDING"},
		{"a context policy short of its fields",
			`{"id":"agent_1","role":"Explore","stop":false,"contextPolicy":{"window":100}}`,
			"INVALID_AGENT_BINDING"},
		{"a context percentage outside its range",
			`{"id":"agent_1","role":"Explore","stop":false,"contextPolicy":` +
				`{"window":100,"autoCompactWindow":80,"compactPercent":140}}`,
			"INVALID_AGENT_BINDING"},
		{"not an object at all", `["agent_1"]`, "INVALID_AGENT_BINDING"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := start(t)
			resp := do(t, g, binding(c.body))
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, c.code) {
				t.Fatalf("body = %s, want %s", body, c.code)
			}
			if g.agents.Registered() != 0 {
				t.Errorf("a refused report registered something anyway")
			}
		})
	}
}

// The shapes a real hook sends are carried.
func TestTheShapesAHookSendsAreAccepted(t *testing.T) {
	g := start(t)
	resp := do(t, g, binding(`{"id":"agent_01ABC-def_2","role":"general-purpose","stop":false,
	  "sessionId":"1b0b3297-ade4-4aa4-86c8-80eaf43db0a3",
	  "transcriptPath":"C:/Users/x/.claude/projects/p/s.jsonl",
	  "contextPolicy":{"window":400000,"autoCompactWindow":320000,"compactPercent":84.2}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
}

// One identifier, one role. Two different subagents wearing one name is not something to
// resolve by picking a side, because either answer is a guess about which one the next
// request belongs to.
func TestTheSameIdentifierMayNotChangeRole(t *testing.T) {
	g := start(t)
	if resp := do(t, g, binding(`{"id":"agent_1","role":"Explore","stop":false}`)); resp.StatusCode != http.StatusOK {
		t.Fatalf("first registration: %s", bodyText(t, resp))
	}
	resp := do(t, g, binding(`{"id":"agent_1","role":"Plan","stop":false}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
	}
	if body := bodyText(t, resp); !strings.Contains(body, "AGENT_BINDING_CONFLICT") {
		t.Fatalf("body = %s", body)
	}
	// And the first one is untouched.
	if role, known := g.agents.roleOf("agent_1"); !known || role != "Explore" {
		t.Fatalf("role = %q (known=%v), want the registration that was already there", role, known)
	}
}

// A stop that never arrives must not leak a registration forever.
func TestARegistrationThatWasNeverStoppedIsSweptByALaterOne(t *testing.T) {
	registry := newAgentRegistry()
	long := time.Now().Add(-2 * agentIdle)

	if _, err := registry.register(agentBinding{ID: "old", Role: "Explore"}, long); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := registry.register(agentBinding{ID: "busy", Role: "Plan"}, long); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Work in progress is not a leftover.
	registry.byID["busy"].active = 1

	if _, err := registry.register(agentBinding{ID: "new", Role: "Explore"}, time.Now()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, known := registry.roleOf("old"); known {
		t.Error("an idle registration past the window survived a later one")
	}
	if _, known := registry.roleOf("busy"); !known {
		t.Error("a registration with live requests was swept")
	}
	if _, known := registry.roleOf("new"); !known {
		t.Error("the registration that did the sweeping went missing")
	}
}

// And when even the window has not released enough, the cap is the backstop.
func TestAFullTableEvictsSomethingIdleAndRefusesWhenNothingIs(t *testing.T) {
	registry := newAgentRegistry()
	now := time.Now()
	for i := 0; i < maxAgents; i++ {
		if _, err := registry.register(agentBinding{ID: fmt.Sprintf("a%d", i), Role: "Explore"}, now); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	// Everything is recent, so the sweep releases nothing and the cap has to.
	if _, err := registry.register(agentBinding{ID: "one-more", Role: "Explore"}, now); err != nil {
		t.Fatalf("a full table refused instead of evicting something idle: %v", err)
	}
	if registry.Registered() != maxAgents {
		t.Fatalf("%d registrations, want the table to stay at its cap of %d",
			registry.Registered(), maxAgents)
	}

	// With every entry busy there is nothing to evict, and refusing beats dropping work.
	for _, state := range registry.byID {
		state.active = 1
	}
	if _, err := registry.register(agentBinding{ID: "no-room", Role: "Explore"}, now); err != errBindingLimit {
		t.Fatalf("err = %v, want %v", err, errBindingLimit)
	}
}

// The endpoint is not an open one, and it is not a model route.
func TestTheBindingEndpointNeedsTheSessionCredentialAndIsNotAModelRoute(t *testing.T) {
	g := start(t)

	unauthorised := do(t, g, request{
		method: http.MethodPost, path: "/clauduct/agents", noAuth: true,
		headers: map[string]string{"Content-Type": "application/json"},
		body:    strings.NewReader(`{"id":"agent_1","role":"Explore","stop":false}`),
	})
	if unauthorised.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", unauthorised.StatusCode)
	}

	wrongMethod := do(t, g, request{method: http.MethodGet, path: "/clauduct/agents"})
	if wrongMethod.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d, want 405", wrongMethod.StatusCode)
	}

	// A report larger than a report. Reading it to find out what it is would be doing the
	// thing the bound exists to prevent.
	oversized := do(t, g, binding(`{"id":"agent_1","role":"`+strings.Repeat("r", 8192)+`","stop":false}`))
	if oversized.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized = %d, want 413: %s", oversized.StatusCode, bodyText(t, oversized))
	}
}
