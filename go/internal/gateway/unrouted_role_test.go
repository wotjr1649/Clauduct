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
	_ = binding
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
