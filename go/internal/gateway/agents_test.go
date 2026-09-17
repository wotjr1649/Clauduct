package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
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

// withAgent is a request from a registered subagent.
func withAgent(agent, body string) request {
	rq := messages(strings.NewReader(body))
	rq.headers["X-Claude-Code-Agent-Id"] = agent
	return rq
}

// C2. A subagent's role decides where it runs, whatever model the client asked for.
//
// Exploring a repository and planning a change are not the same work, and neither is the
// model the conversation happens to be using.
func TestASubagentsRoleDecidesWhereItRuns(t *testing.T) {
	for _, c := range []struct{ role, model, effort string }{
		{"Explore", "gpt-5.6-luna", "max"},
		{"Plan", "gpt-6-astra", "low"},
		{"general-purpose", "gpt-5.6-luna", "max"},
	} {
		t.Run(c.role, func(t *testing.T) {
			seen := make(chan upstream.Call, 1)
			g := startWith(t, &recordingTransport{
				sse:  sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
				seen: seen,
			})
			if resp := do(t, g, binding(`{"id":"agent_1","role":"`+c.role+`","stop":false}`)); resp.StatusCode != http.StatusOK {
				t.Fatalf("register: %s", bodyText(t, resp))
			}

			// The client asks for opus, which would ordinarily route to sol.
			resp := do(t, g, withAgent("agent_1", `{"model":"claude-opus-5","max_tokens":16,
			  "stream":true,"messages":[{"role":"user","content":"x"}]}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
			}

			call := <-seen
			if call.Model != c.model || call.Effort != c.effort {
				t.Fatalf("ran on %s/%s, want %s/%s", call.Model, call.Effort, c.model, c.effort)
			}
			// CAP03: the record keeps both halves and says which rule reassigned it.
			if call.Requested != "claude-opus-5" {
				t.Errorf("Requested = %q; the client's own choice is the half a billing "+
					"record cannot reconstruct", call.Requested)
			}
			if call.Source != "role" {
				t.Errorf("Source = %q, want role. A reader who sees a model the client did "+
					"not ask for needs to know what reassigned it.", call.Source)
			}
		})
	}
}

// Every way this can fail leaves the client's own choice in place.
//
// A turn is not worth ending over a routing preference. The baseline refuses some of these,
// which is defensible there because it verifies the subagent's identity against the
// client's own metadata first; without that, the same refusal only adds a way to fail.
func TestRoutingThatCannotHappenDoesNotEndTheTurn(t *testing.T) {
	for _, c := range []struct {
		name, register, agent string
		wantUnregistered      int64
		wantUnrouted          int64
	}{
		{name: "no registration arrived yet", agent: "agent_unknown", wantUnregistered: 1},
		{name: "a role with no route",
			register: `{"id":"agent_1","role":"some-custom-agent","stop":false}`,
			agent:    "agent_1", wantUnrouted: 1},
		// Measured 2026-09-17: every agent the Workflow tool starts reports this one name.
		// It keeps the client's model on purpose, which is the same answer the baseline
		// reaches by reading and verifying a run journal on disk. Counting it as a miss
		// would report every session that ran a workflow as having something wrong, and a
		// diagnostic that cries wolf on ordinary use stops being read.
		{name: "a role that deliberately keeps the parent's model",
			register: `{"id":"agent_1","role":"workflow-subagent","stop":false}`,
			agent:    "agent_1"},
		{name: "the subagent was already stopped",
			register: `{"id":"agent_1","role":"Explore","stop":true}`,
			agent:    "agent_1", wantUnregistered: 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			seen := make(chan upstream.Call, 1)
			g := startWith(t, &recordingTransport{
				sse:  sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
				seen: seen,
			})
			if c.register != "" {
				do(t, g, binding(c.register))
			}

			resp := do(t, g, withAgent(c.agent, `{"model":"claude-opus-5","max_tokens":16,
			  "stream":true,"messages":[{"role":"user","content":"x"}]}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want the turn to go through: %s",
					resp.StatusCode, bodyText(t, resp))
			}
			call := <-seen
			if call.Model != "gpt-5.6-sol" || call.Source != "family" {
				t.Fatalf("ran on %s (%s), want the client's own choice", call.Model, call.Source)
			}

			// Counted, so "routing quietly did nothing" is a number rather than a silence.
			unregistered, unrouted := g.Unrouted()
			if unregistered != c.wantUnregistered || unrouted != c.wantUnrouted {
				t.Fatalf("counts = (%d, %d), want (%d, %d)",
					unregistered, unrouted, c.wantUnregistered, c.wantUnrouted)
			}
		})
	}
}

// A request with no subagent header is an ordinary request and is not counted as a miss.
func TestAnOrdinaryRequestIsNotASubagentThatWentUnrouted(t *testing.T) {
	seen := make(chan upstream.Call, 1)
	g := startWith(t, &recordingTransport{
		sse:  sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
		seen: seen,
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	<-seen
	if unregistered, unrouted := g.Unrouted(); unregistered != 0 || unrouted != 0 {
		t.Fatalf("counts = (%d, %d) for a request that named no subagent", unregistered, unrouted)
	}
}

// A registration with a request in flight is not a leftover.
func TestARegistrationInUseIsHeldAgainstTheSweep(t *testing.T) {
	registry := newAgentRegistry()
	if _, err := registry.register(agentBinding{ID: "busy", Role: "Explore"}, time.Now().Add(-2*agentIdle)); err != nil {
		t.Fatalf("register: %v", err)
	}
	role, release, ok := registry.begin("busy")
	if !ok || role != "Explore" {
		t.Fatalf("begin = %q, %v", role, ok)
	}
	// A request that outlives the idle window. Refreshing lastUsed is not what protects
	// this -- a subagent can spend longer than agentIdle inside a single turn, and at that
	// point the only thing that knows the entry is still wanted is the count.
	registry.byID["busy"].lastUsed = time.Now().Add(-2 * agentIdle)

	if _, err := registry.register(agentBinding{ID: "other", Role: "Plan"}, time.Now()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, known := registry.roleOf("busy"); !known {
		t.Fatal("a registration with a request in flight was swept")
	}

	// And once the request is done it becomes a candidate like any other.
	release()
	registry.byID["busy"].lastUsed = time.Now().Add(-2 * agentIdle)
	if _, err := registry.register(agentBinding{ID: "later", Role: "Plan"}, time.Now()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, known := registry.roleOf("busy"); known {
		t.Fatal("a registration that finished and went idle was never released")
	}
}

// A registration that arrives twice does not forget the requests in flight.
//
// Found by review. Re-registering replaced the state object, so the release closure begin()
// had handed out decremented an orphan and the new state's active count stayed zero. Zero
// is what the idle sweep and the cap both read as "not busy", so a subagent that got a
// second SubagentStart while streaming became evictable mid-answer -- and after eviction
// its requests ran with no role, which is the one thing the registry exists to prevent.
func TestARepeatedRegistrationKeepsTheRequestsInFlight(t *testing.T) {
	registry := newAgentRegistry()
	binding := agentBinding{ID: "agent_1", Role: "Explore"}
	if _, err := registry.register(binding, time.Now()); err != nil {
		t.Fatalf("register: %v", err)
	}

	role, release, ok := registry.begin("agent_1")
	if !ok || role != "Explore" {
		t.Fatalf("begin = %q, %v", role, ok)
	}

	// The same subagent announces itself again while its request is still running.
	if _, err := registry.register(binding, time.Now()); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if got := registry.byID["agent_1"].active; got != 1 {
		t.Fatalf("active = %d after a repeated registration, want the request still counted", got)
	}

	release()
	if got := registry.byID["agent_1"].active; got != 0 {
		t.Fatalf("active = %d after release, want 0", got)
	}
}
