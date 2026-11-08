package bridge

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSelectionSnapshotFreezesFallbacksWithoutPromotingThem(t *testing.T) {
	luna, sol6 := fixtureModel("luna").ID, fixtureModel("sol6").ID
	original := Selection{ModelDefaults: map[string]Default{luna: {Effort: "low"}}}.WithCatalogue(testCatalogue)
	frozen := original.Snapshot()
	if len(frozen.ModelDefaults) != 1 || len(frozen.ModelMapping) != tieredModels() || len(frozen.RoleDefaults) != len(roleRoutes) || len(frozen.Agents) != 0 {
		t.Fatal("snapshot is sparse or promoted builtin fallbacks")
	}
	original.ModelDefaults[luna] = Default{Effort: "high"}
	if effort, _ := frozen.DefaultFor(luna); effort != "low" {
		t.Fatal("snapshot shares caller maps")
	}
	frozen.RoleDefaults["Plan"] = Pair{Model: sol6, Effort: "medium"}
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ParseSelection(raw)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := restored.RoleRoute("Plan")
	if !ok || route.Model != sol6 || route.Effort != "medium" || len(restored.Agents) != 0 {
		t.Fatalf("saved builtin fallback not restored: %v", route)
	}
}

// tieredModels counts the models a Claude tier names; v0.6.3's GPT-6 Sol has none.
func tieredModels() int {
	n := 0
	for _, model := range Models {
		if model.Alias != "" {
			n++
		}
	}
	return n
}

func TestSelectionRoutes(t *testing.T) {
	sol, sol6, luna := fixtureModel("sol").ID, fixtureModel("sol6").ID, fixtureModel("luna").ID
	s := Selection{
		ModelDefaults: map[string]Default{sol: {Effort: "high"}, sol6: {Effort: "high"}},
		ModelMapping:  map[string]string{"opus": luna, "sonnet": luna},
		Agents:        map[string]Pair{"Explore": {Model: sol6, Effort: "low"}},
	}.WithCatalogue(testCatalogue)
	for _, check := range []struct {
		model, effort, wantModel, wantEffort, wantSource string
	}{
		{"opus", "", luna, "max", "alias"},
		{"claude-opus-5", "high", luna, "high", "family+effort"},
		{"sol", "", sol, "high", "catalogue"},
		{"sol6", "", sol6, "high", "catalogue"},
		{sol6, "xhigh", sol6, "xhigh", "direct+effort"},
	} {
		got, err := s.SelectRoute(check.model, check.effort)
		if err != nil || got != (Route{Model: check.wantModel, Effort: check.wantEffort, Source: check.wantSource}) {
			t.Fatalf("SelectRoute(%q,%q) = %+v, %v", check.model, check.effort, got, err)
		}
	}
	if got, ok := s.RoleRoute("explore"); !ok || got.Model != sol6 || got.Effort != "low" {
		t.Fatalf("configured role = %+v, %v", got, ok)
	}
	if got, ok := s.RoleRoute("Plan"); !ok || got != roleRoutes["Plan"] {
		t.Fatalf("fixed role = %+v, %v", got, ok)
	}
	if got, ok := s.RoleRoute("clauduct-sol"); !ok || got.Effort != "high" {
		t.Fatalf("menu default = %+v, %v", got, ok)
	}
	if _, err := s.SelectRoute("unknown", ""); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("unknown model: %v", err)
	}
	if _, err := s.SelectRoute("opus", "ultra"); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("unknown effort: %v", err)
	}
	clone := s.Clone()
	clone.ModelMapping["opus"] = sol6
	if s.ModelMapping["opus"] != luna {
		t.Fatal("Clone shares configuration maps")
	}
}
