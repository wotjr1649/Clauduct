package bridge

import (
	"errors"
	"testing"
)

// v0.6.3: GPT-6.1 Sol is the opus tier and the sol key; GPT-6 Sol stays routable.
func TestSol61TakesTheOpusTierAndSol6StaysRoutable(t *testing.T) {
	sol, sol6 := fixtureModel("sol").ID, fixtureModel("sol6").ID
	for requested, want := range map[string]string{
		"sol": sol, "opus": sol, "claude-opus-5": sol, "claude-opus-4-1": sol,
		sol: sol, sol6: sol6, "sol6": sol6,
	} {
		route, err := SelectRoute(requested, "")
		if err != nil || route.Model != want {
			t.Fatalf("%s -> %s %v, want %s", requested, route.Model, err, want)
		}
	}
	for role, want := range map[string]string{MenuPrefix + "sol": sol, MenuPrefix + "sol6": sol6} {
		if route, ok := RoleRoute(role); !ok || route.Model != want {
			t.Fatalf("%s -> %s", role, route.Model)
		}
	}
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if _, err := SelectRoute(sol, effort); err != nil {
			t.Fatalf("%s refused: %v", effort, err)
		}
	}
	if _, err := SelectRoute(sol, "ultra"); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatal("ultra was measured refused by the backend")
	}
	if route, _ := SelectRoute(sol, ""); route.Effort != "xhigh" {
		t.Fatalf("factory effort %s", route.Effort)
	}
	model, ok := ModelByID(sol)
	if !ok || !model.CountValidated {
		t.Fatal("measured count match not admitted")
	}
	if DefaultStartup().Model != sol {
		t.Fatal("factory startup did not move to GPT-6.1 Sol")
	}
	if _, retired := Retired[sol6]; retired {
		t.Fatal("GPT-6 Sol must stay routable")
	}
}
