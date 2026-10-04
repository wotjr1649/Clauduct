package bridge

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSelectionSnapshotFreezesFallbacksWithoutPromotingThem(t *testing.T) {
	original := Selection{ModelDefaults: map[string]Default{"gpt-6-luna": {Effort: "low"}}}.WithCatalogue(testCatalogue)
	frozen := original.Snapshot()
	if len(frozen.ModelDefaults) != 1 || len(frozen.ModelMapping) != tieredModels() || len(frozen.RoleDefaults) != len(roleRoutes) || len(frozen.Agents) != 0 {
		t.Fatal("snapshot is sparse or promoted builtin fallbacks")
	}
	original.ModelDefaults["gpt-6-luna"] = Default{Effort: "high"}
	if effort, _ := frozen.DefaultFor("gpt-6-luna"); effort != "low" {
		t.Fatal("snapshot shares caller maps")
	}
	frozen.RoleDefaults["Plan"] = Pair{Model: "gpt-6-sol", Effort: "medium"}
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ParseSelection(raw)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := restored.RoleRoute("Plan")
	if !ok || route.Model != "gpt-6-sol" || route.Effort != "medium" || len(restored.Agents) != 0 {
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
	s := Selection{
		ModelDefaults: map[string]Default{"gpt-6.1-sol": {Effort: "high"}, "gpt-6-sol": {Effort: "high"}},
		ModelMapping:  map[string]string{"opus": "gpt-6-luna", "sonnet": "gpt-6-luna"},
		Agents:        map[string]Pair{"Explore": {Model: "gpt-6-sol", Effort: "low"}},
	}.WithCatalogue(testCatalogue)
	for _, check := range []struct {
		model, effort, wantModel, wantEffort, wantSource string
	}{
		{"opus", "", "gpt-6-luna", "max", "alias"},
		{"claude-opus-5", "high", "gpt-6-luna", "high", "family+effort"},
		{"sol", "", "gpt-6.1-sol", "high", "catalogue"},
		{"sol6", "", "gpt-6-sol", "high", "catalogue"},
		{"gpt-6-sol", "xhigh", "gpt-6-sol", "xhigh", "direct+effort"},
	} {
		got, err := s.SelectRoute(check.model, check.effort)
		if err != nil || got != (Route{Model: check.wantModel, Effort: check.wantEffort, Source: check.wantSource}) {
			t.Fatalf("SelectRoute(%q,%q) = %+v, %v", check.model, check.effort, got, err)
		}
	}
	if got, ok := s.RoleRoute("explore"); !ok || got.Model != "gpt-6-sol" || got.Effort != "low" {
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
	clone.ModelMapping["opus"] = "gpt-6-sol"
	if s.ModelMapping["opus"] != "gpt-6-luna" {
		t.Fatal("Clone shares configuration maps")
	}
}
