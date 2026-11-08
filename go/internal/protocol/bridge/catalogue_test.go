package bridge

import (
	"errors"
	"slices"
	"testing"
)

func TestAccountCatalogueKeepsOnlyWhatNativeCanSend(t *testing.T) {
	solID := fixtureModel("sol").ID
	c, skipped, err := NewCatalogue([]AccountModel{
		{ID: solID, Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}, Default: "low", Visible: true},
		{ID: "gpt-new", Efforts: []string{"ultra", "medium", "none"}, Default: "ultra"},
		{ID: "gpt-only-ultra", Efforts: []string{"ultra"}, Default: "ultra", Visible: true},
		{ID: "sol", Efforts: []string{"low"}, Default: "low"},
		{ID: "inherit", Efforts: []string{"low"}, Default: "low"},
		{ID: "sol-high", Efforts: []string{"low"}, Default: "low"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(skipped, []string{"gpt-only-ultra"}) {
		t.Fatalf("skipped %v", skipped)
	}
	sol, _ := c.ByID(solID)
	if !slices.Equal(sol.Efforts, lowToMax) || sol.Effort != "low" || sol.Key != "sol" || sol.Alias != "opus" || sol.Menu != "sol" || !sol.Visible || !sol.CountValidated {
		t.Fatalf("legacy identity not joined: %+v", sol)
	}
	fresh, _ := c.ByID("gpt-new")
	if !slices.Equal(fresh.Efforts, []string{"medium"}) || fresh.Effort != "" || fresh.Key != "" || fresh.AgentAlias != "" || fresh.Menu != "gpt-new" || fresh.Visible || fresh.CountValidated {
		t.Fatalf("new model guessed an identity: %+v", fresh)
	}
	for _, id := range []string{"sol", "inherit", "sol-high"} {
		if m, ok := c.ByID(id); !ok || m.Menu != "" {
			t.Fatalf("%s: a colliding menu name was kept: %+v", id, m)
		}
	}
	if !slices.Equal(c.Efforts(), lowToMax) {
		t.Fatalf("union %v", c.Efforts())
	}
	s := Selection{}.WithCatalogue(c)
	if _, err := s.SelectRoute("gpt-new", ""); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("a model without a sendable default effort got one: %v", err)
	}
	if route, err := s.SelectRoute("gpt-new", "medium"); err != nil || route.Effort != "medium" {
		t.Fatalf("explicit effort: %+v %v", route, err)
	}
	for name, list := range map[string][]AccountModel{
		"duplicate": {{ID: "a", Efforts: []string{"low"}}, {ID: "a", Efforts: []string{"low"}}},
		"bad id":    {{ID: "A b", Efforts: []string{"low"}}},
		"empty":     nil,
		"unusable":  {{ID: "a", Efforts: []string{"ultra"}}},
	} {
		if _, _, err := NewCatalogue(list); !errors.Is(err, ErrNoAccountModels) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

// Effort order: explicit request, then modelDefaults, then the account default. A
// configured effort the model does not take fails instead of falling through.
func TestEffortComesFromRequestThenPreferencesThenAccount(t *testing.T) {
	luna, astra := fixtureModel("luna").ID, fixtureModel("astra").ID
	requireUnknownModels(t, "gpt-gone")
	c, _, err := NewCatalogue([]AccountModel{
		{ID: luna, Efforts: []string{"low", "medium", "high"}, Default: "medium"},
		{ID: astra, Efforts: []string{"low", "medium"}, Default: "low"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, offered := c.ByID("gpt-gone"); offered {
		t.Fatal("unknown model unexpectedly offered")
	}
	s := Selection{ModelDefaults: map[string]Default{luna: {Effort: "high"}, astra: {Effort: "max"}, "gpt-gone": {Effort: "low"}}}.WithCatalogue(c)
	for _, check := range []struct{ model, effort, want string }{
		{luna, "low", "low"},
		{luna, "", "high"},
	} {
		if got, err := s.SelectRoute(check.model, check.effort); err != nil || got.Effort != check.want {
			t.Fatalf("%+v: %+v %v", check, got, err)
		}
	}
	if got, err := (Selection{}).WithCatalogue(c).SelectRoute(luna, ""); err != nil || got.Effort != "medium" {
		t.Fatalf("account default: %+v %v", got, err)
	}
	if _, err := s.SelectRoute(astra, ""); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("an unsupported configured effort was replaced: %v", err)
	}
	if _, err := s.SelectRoute("gpt-gone", "low"); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("a model the account lacks was routed: %v", err)
	}
	want := []string{"modelDefaults." + astra + ": effort max is not supported", "modelDefaults.gpt-gone: not in the account model list"}
	slices.Sort(want)
	if got := s.Problems(); !slices.Equal(got[:2], want) {
		t.Fatalf("problems %v", got)
	}
}

// The retired table only explains a refusal; a name the account still lists is routed.
func TestARetiredNameTheAccountListsIsRouted(t *testing.T) {
	requireUnknownModels(t, "gpt-9-unknown")
	// gpt-5.5 is listed until the account retires it on 2026-10-14 (successor gpt-6.1-sol).
	for id, successor := range map[string]string{"gpt-5.6-sol": fixtureModel("sol6").ID, "gpt-5.5": fixtureModel("sol").ID} {
		if Retired[id] != successor {
			t.Fatalf("retirement %q must name %q", id, successor)
		}
		if _, offered := testCatalogue.ByID(id); offered {
			t.Fatalf("retired fixture model %q is still offered", id)
		}
		c, _, err := NewCatalogue([]AccountModel{{ID: id, Efforts: []string{"low"}, Default: "low", Visible: true}})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := (Selection{}).WithCatalogue(c).SelectRoute(id, ""); err != nil || got.Model != id {
			t.Fatalf("listed retired name %s: %+v %v", id, got, err)
		}
		if _, err := testSelection.SelectRoute(id, ""); !errors.Is(err, ErrRetiredRoute) {
			t.Fatalf("unlisted retired name %s: %v", id, err)
		}
	}
	if Retired["gpt-5.5"] != fixtureModel("sol").ID {
		t.Fatalf("gpt-5.5 successor = %q", Retired["gpt-5.5"])
	}
	// Review finding: a listed retired name with an effort it lacks is an ordinary refusal.
	listed, _, _ := NewCatalogue([]AccountModel{{ID: "gpt-5.5", Efforts: []string{"low"}, Default: "low", Visible: true}})
	if s := (Selection{}).WithCatalogue(listed); s.Retires("gpt-5.5") || !testSelection.Retires("gpt-5.5") || testSelection.Retires("gpt-9-unknown") {
		t.Fatal("Retires must mean retired and no longer listed")
	}
	if _, err := (Selection{}).WithCatalogue(listed).SelectRoute("gpt-5.5", "xhigh"); errors.Is(err, ErrRetiredRoute) || !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("listed gpt-5.5 at an effort it lacks: %v", err)
	}
}

// An override (role pair, classifier pair, compaction route) is checked like any request.
func TestAnOverrideTheAccountDoesNotOfferIsRefused(t *testing.T) {
	requireUnknownModels(t, "gpt-gone")
	luna := fixtureModel("luna").ID
	for _, route := range []Route{{Model: "gpt-gone", Effort: "low"}, {Model: luna, Effort: "ultra"}} {
		if _, err := testSelection.ResolveRoute(decodeRequest(t, `{"model":"`+luna+`","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"x"}]}`), route); !errors.Is(err, ErrUnsupportedRoute) {
			t.Fatalf("%+v accepted: %v", route, err)
		}
	}
}
