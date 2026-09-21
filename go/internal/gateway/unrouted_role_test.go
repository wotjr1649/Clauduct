package gateway

import (
	"encoding/json"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// 0.2.x left a role it had no route for on the client's own model, and the child ran. The
// context policy this release added turned that into a refusal -- but on the child's first
// request, after native had already spawned it, so the Agent call looked like it worked and
// the subagent produced nothing. Native's own built-in types other than Explore, Plan and
// general-purpose take this path.
//
// The child runs again, on the caller's verified route. It has to be a recorded choice
// rather than waved through: results.start is reached only through cacheChoice, so a child
// with none is invisible to the completion evidence and its parent can answer as complete
// with the report outstanding.
func TestARoleWithNoRouteRunsOnTheCallersRouteAndIsTracked(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	scope.route = bridge.Route{Model: "gpt-5.6-terra", Effort: "high", Source: "catalogue"}
	raw, err := d.prepare(scope, "unrouted_call", "Agent",
		json.RawMessage(`{"subagent_type":"statusline-setup","prompt":"synthetic","description":"proof"}`))
	if err != nil {
		t.Fatalf("a native built-in type was refused at the Agent call: %v", err)
	}
	if raw == nil {
		t.Fatal("the call was dropped")
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
	if pending.route.Model != "gpt-5.6-terra" || pending.route.Effort != "high" {
		t.Fatalf("child did not take the caller's route: %s/%s", pending.route.Model, pending.route.Effort)
	}
	if pending.inherited {
		t.Fatal("a fallback was recorded as a task-bound selection, which would pin every descendant")
	}
	if pending.route.Source != "parent-route" {
		t.Fatalf("source %q is not one loadChoice accepts, so the choice dies on restore", pending.route.Source)
	}
	_ = binding
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
