package bridge

import (
	"errors"
	"testing"
)

// v0.6.3: GPT-6.1 Sol is the opus tier and the sol key; GPT-6 Sol stays routable.
func TestSol61TakesTheOpusTierAndSol6StaysRoutable(t *testing.T) {
	for requested, want := range map[string]string{
		"sol": "gpt-6.1-sol", "opus": "gpt-6.1-sol", "claude-opus-5": "gpt-6.1-sol", "claude-opus-4-1": "gpt-6.1-sol",
		"gpt-6.1-sol": "gpt-6.1-sol", "gpt-6-sol": "gpt-6-sol", "sol6": "gpt-6-sol",
	} {
		route, err := SelectRoute(requested, "")
		if err != nil || route.Model != want {
			t.Fatalf("%s -> %s %v, want %s", requested, route.Model, err, want)
		}
	}
	for role, want := range map[string]string{MenuPrefix + "sol": "gpt-6.1-sol", MenuPrefix + "sol6": "gpt-6-sol"} {
		if route, ok := RoleRoute(role); !ok || route.Model != want {
			t.Fatalf("%s -> %s", role, route.Model)
		}
	}
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if _, err := SelectRoute("gpt-6.1-sol", effort); err != nil {
			t.Fatalf("%s refused: %v", effort, err)
		}
	}
	if _, err := SelectRoute("gpt-6.1-sol", "ultra"); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatal("ultra was measured refused by the backend")
	}
	if route, _ := SelectRoute("gpt-6.1-sol", ""); route.Effort != "xhigh" {
		t.Fatalf("factory effort %s", route.Effort)
	}
	model, ok := ModelByID("gpt-6.1-sol")
	if !ok || !model.CountValidated {
		t.Fatal("measured count match not admitted")
	}
	if DefaultStartup().Model != "gpt-6.1-sol" {
		t.Fatal("factory startup did not move to GPT-6.1 Sol")
	}
	if _, retired := Retired["gpt-6-sol"]; retired {
		t.Fatal("GPT-6 Sol must stay routable")
	}
}
