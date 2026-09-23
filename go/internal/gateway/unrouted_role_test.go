package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Native chooses unlisted roles; pending and resolved children both block completion.
func TestARoleWithNoRouteKeepsNativeChoiceAndIsTracked(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	delete(d.pending, delegationKey{scope.session, "proof_call"})
	scope.route = bridge.Route{Model: "gpt-5.6-terra", Effort: "high", Source: "catalogue"}
	raw, err := d.prepare(scope, "unrouted_call", "Agent",
		json.RawMessage(`{"subagent_type":"statusline-setup","prompt":"synthetic","description":"proof"}`))
	if err != nil {
		t.Fatalf("a native built-in type was refused at the Agent call: %v", err)
	}
	if raw == nil {
		t.Fatal("the call was dropped")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields["model"] != nil {
		t.Fatal("the caller's model replaced native's selection")
	}
	if d.unroutedRoles.Load() != 1 {
		t.Fatalf("the routing miss went uncounted: %d", d.unroutedRoles.Load())
	}
	// The pending choice is what makes the child visible to results and completion evidence.
	var pending delegatedChoice
	held := false
	for key, choice := range d.pending {
		if key.call == "unrouted_call" {
			pending, held = choice, true
		}
	}
	if !held {
		t.Fatal("no choice was recorded, so the child would never reach results.start")
	}
	if pending.route.Model != "" || pending.route.Effort != "" {
		t.Fatal("an unobserved native selection was reported as known")
	}
	if pending.inherited {
		t.Fatal("a fallback was recorded as a task-bound selection, which would pin every descendant")
	}
	if pending.route.Source != "native-selection" {
		t.Fatalf("source %q is not one loadChoice accepts, so the choice dies on restore", pending.route.Source)
	}
	_, before := d.deliverWithReadiness(&anthropic.Request{}, scope.session, "")
	if before.Eligible || len(before.Pending) != 1 {
		t.Fatal("unstarted native child did not block completion")
	}
	binding.Role = "statusline-setup"
	path := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
	if err := os.WriteFile(path, []byte(`{"toolUseId":"unrouted_call","agentType":"statusline-setup","model":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := d.route(scope, binding.ID, binding); err == nil || found {
		t.Fatal("unobserved native turn granted a model choice")
	}
	scope.nativeTurn = &nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "native_turn", Model: "gpt-5.6-luna", Effort: "low"}
	route, found, err := d.route(scope, binding.ID, binding)
	if err != nil || !found || route.Model != "gpt-5.6-luna" || route.Effort != "low" {
		t.Fatalf("native choice: %+v found=%v err=%v", route, found, err)
	}
	_, after := d.deliverWithReadiness(&anthropic.Request{}, scope.session, "")
	if after.Eligible || len(after.Pending) != 1 {
		t.Fatal("resolved native child disappeared from completion tracking")
	}
	delete(d.resolved, binding.ID)
	restored, found, err := d.loadChoice(scope, binding.ID, binding)
	if err != nil || !found || restored.route != route {
		t.Fatalf("native choice was not restored: %+v found=%v err=%v", restored.route, found, err)
	}
}

// inherited is set for any clauduct-* role the menu cannot resolve, and it means a
// task-bound selection is fixed for every descendant. The fallback has to clear it: a
// fallback is not a selection anybody made, and leaving it set refuses any child of this one
// that names a model. The other test in this file cannot catch it -- statusline-setup has no
// menu prefix, so it passes for a reason unrelated to the flag.
func TestAMenuNameTheMenuCannotResolveIsNotTreatedAsTaskBound(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	scope.route = bridge.Route{Model: "gpt-5.6-terra", Effort: "high", Source: "catalogue"}
	if _, err := d.prepare(scope, "menu_call", "Agent",
		json.RawMessage(`{"subagent_type":"clauduct-inherit-high","prompt":"synthetic","description":"proof"}`)); err != nil {
		t.Fatalf("a menu-shaped name the menu does not define was refused: %v", err)
	}
	for key, choice := range d.pending {
		if key.call == "menu_call" && choice.inherited {
			t.Fatal("a fallback pinned the model for every descendant")
		}
	}
}

func TestMenuDefinitionStillPinsTheDescendantSelection(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	d.roleDefaults = func(string, bridge.Route) (bridge.Route, bool, error) {
		return bridge.Route{Model: "gpt-5.6-terra", Effort: "high"}, true, nil
	}
	if _, err := d.prepare(scope, "menu_definition", "Agent", []byte(`{"subagent_type":"clauduct-terra-high","prompt":"public","description":"proof"}`)); err != nil {
		t.Fatal(err)
	}
	choice, found := d.pending[delegationKey{scope.session, "menu_definition"}]
	if !found || !choice.inherited {
		t.Fatal("resolving the menu definition removed its task-bound selection")
	}
}

func TestNativeChoiceRefusesUnverifiedIdentityModelAndEffort(t *testing.T) {
	for _, changed := range []string{"session", "agent", "turn", "model", "effort", "empty_effort", "metadata_model"} {
		t.Run(changed, func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			delete(d.pending, delegationKey{scope.session, "proof_call"})
			scope.route = bridge.Route{Model: "gpt-6-astra", Effort: "low"}
			if _, err := d.prepare(scope, "proof_call", "Agent", []byte(`{"subagent_type":"statusline-setup","prompt":"public","description":"proof"}`)); err != nil {
				t.Fatal(err)
			}
			binding.Role = "statusline-setup"
			model := ""
			if changed == "metadata_model" {
				model = "haiku"
			}
			meta, _ := json.Marshal(map[string]string{"toolUseId": "proof_call", "agentType": binding.Role, "model": model})
			path := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
			if err := os.WriteFile(path, meta, 0600); err != nil {
				t.Fatal(err)
			}
			active := &nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "native_turn", Model: "gpt-5.6-luna", Effort: "low"}
			switch changed {
			case "session":
				active.Session = "another_session"
			case "agent":
				active.Agent = "another_agent"
			case "turn":
				active.Turn = ""
			case "model":
				active.Model = "unlisted"
			case "effort":
				active.Effort = "unlisted"
			case "empty_effort":
				active.Effort = ""
			}
			scope.nativeTurn = active
			if _, found, err := d.route(scope, binding.ID, binding); err == nil || found {
				t.Fatal("unverified native selection was accepted")
			}
			if len(d.resolved) != 0 || len(d.results.entries) != 0 || len(d.pending) != 1 {
				t.Fatal("refusal changed unresolved completion evidence")
			}
		})
	}
}

func TestNativeChoiceChecksTheActualRequestEffort(t *testing.T) {
	for _, effort := range []string{"low", "high"} {
		d, scope, binding := preparedDelegation(t)
		delete(d.pending, delegationKey{scope.session, "proof_call"})
		scope.route = bridge.Route{Model: "gpt-6-astra", Effort: "low"}
		if _, err := d.prepare(scope, "proof_call", "Agent", []byte(`{"subagent_type":"statusline-setup","prompt":"public","description":"proof"}`)); err != nil {
			t.Fatal(err)
		}
		binding.Role = "statusline-setup"
		path := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
		if err := os.WriteFile(path, []byte(`{"toolUseId":"proof_call","agentType":"statusline-setup","model":""}`), 0600); err != nil {
			t.Fatal(err)
		}
		backend := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
		g := startWith(t, backend)
		g.delegations = d
		events := t.TempDir()
		g.ConfigureNativeEvents(events)
		raw, _ := json.Marshal(nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "native_turn", Model: "gpt-5.6-luna", Effort: "low"})
		if err := writeNativeTestFile(filepath.Join(events, "active/child-"+binding.ID), raw); err != nil {
			t.Fatal(err)
		}
		if _, err := g.agents.register(binding, time.Now()); err != nil {
			t.Fatal(err)
		}
		request := messages(strings.NewReader(`{"model":"gpt-5.6-luna","max_tokens":16,"stream":true,"output_config":{"effort":"` + effort + `"},"messages":[{"role":"user","content":"public"}]}`))
		request.headers["X-Claude-Code-Session-Id"] = scope.session
		request.headers["X-Claude-Code-Agent-Id"] = binding.ID
		response := do(t, g, request)
		body := bodyText(t, response)
		if effort == "high" {
			if response.StatusCode != http.StatusBadRequest || backend.Calls() != 0 || !strings.Contains(body, "AGENT_SELECTION_UNVERIFIED") {
				t.Fatal("request effort disagreed with native but reached the backend")
			}
		} else if response.StatusCode != http.StatusOK || backend.Calls() != 1 {
			t.Fatal("verified native effort did not reach the backend")
		}
	}
}

// Built-in role names resolve case-insensitively, the way native resolves them and the way
// the gateway already reconciles a child's reported role. Exact-only matching sent "explore"
// down the fallback, so an Explore child ran on the caller's route and the EqualFold
// reconciliation let it pass.
func TestABuiltInRoleResolvesWhateverItsCasing(t *testing.T) {
	for _, name := range []string{"Explore", "explore", "EXPLORE", "general-purpose", "General-Purpose"} {
		route, known := bridge.RoleRoute(name)
		if !known {
			t.Fatalf("%s missed the role table", name)
		}
		if name != "general-purpose" && name != "General-Purpose" && route.Model != "gpt-5.6-luna" {
			t.Fatalf("%s routed to %s", name, route.Model)
		}
	}
}

// An explicit effort on a role with no route is still unsupported: there is no catalogue
// entry to apply it to, and substituting one would be the guess this build does not make.
func TestAnEffortOnARoleWithNoRouteIsStillRefused(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	scope.route = bridge.Route{Model: "gpt-5.6-terra", Effort: "high", Source: "catalogue"}
	if _, err := d.prepare(scope, "effort_call", "Agent",
		json.RawMessage(`{"subagent_type":"statusline-setup","effort":"max","prompt":"x","description":"p"}`)); err == nil {
		t.Fatal("an effort was accepted for a role with no route")
	}
}
