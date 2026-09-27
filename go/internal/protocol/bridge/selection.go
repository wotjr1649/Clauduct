package bridge

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Pair is a configured model and effort. Both must be validated before use.
type Pair struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

type Default struct {
	Effort string `json:"effort"`
}

// Selection contains one session's validated, read-only routing preferences.
// The catalogue, accepted efforts, context policies and classifier stay in code.
type Selection struct {
	ModelDefaults map[string]Default `json:"modelDefaults,omitempty"`
	ModelMapping  map[string]string  `json:"modelMapping,omitempty"`
	Agents        map[string]Pair    `json:"agents,omitempty"`
	// RoleDefaults freezes builtin fallbacks in saved sessions. The manual
	// settings schema exposes Agents, not this snapshot field.
	RoleDefaults map[string]Pair `json:"roleDefaults,omitempty"`
}

// ParseSelection applies the same strict schema to preferences and saved sessions.
func ParseSelection(raw []byte) (Selection, error) {
	bad := func() (Selection, error) { return Selection{}, ErrUnsupportedRoute }
	fields, err := wire.Fields(raw, []string{"modelDefaults", "modelMapping", "agents", "roleDefaults"})
	if err != nil {
		return bad()
	}
	var selection Selection
	if value, ok := fields["modelDefaults"]; ok {
		entries, err := wire.Fields(value, nil)
		if err != nil {
			return bad()
		}
		selection.ModelDefaults = make(map[string]Default, len(entries))
		for id, value := range entries {
			model, ok := ModelByID(id)
			if !ok {
				return bad()
			}
			fields, err := wire.Fields(value, []string{"effort"})
			var effort string
			if err != nil || len(fields) != 1 || json.Unmarshal(fields["effort"], &effort) != nil || !slices.Contains(model.Efforts, effort) {
				return bad()
			}
			selection.ModelDefaults[id] = Default{Effort: effort}
		}
	}
	if value, ok := fields["modelMapping"]; ok {
		entries, err := wire.Fields(value, nil)
		if err != nil {
			return bad()
		}
		selection.ModelMapping = make(map[string]string, len(entries))
		for alias, value := range entries {
			if _, ok := ForAlias(alias); !ok {
				return bad()
			}
			var id string
			if json.Unmarshal(value, &id) != nil {
				return bad()
			}
			if _, ok := ModelByID(id); !ok {
				return bad()
			}
			selection.ModelMapping[alias] = id
		}
	}
	if value, ok := fields["agents"]; ok {
		entries, err := wire.Fields(value, nil)
		if err != nil || len(entries) > 1024 {
			return bad()
		}
		selection.Agents = make(map[string]Pair, len(entries))
		for name, value := range entries {
			if name == "" || len(name) > 200 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\x00\r\n") || InheritsParent(name) {
				return bad()
			}
			pair, err := ParsePair(value)
			if err != nil {
				return bad()
			}
			canonical := CanonicalRole(name)
			if _, duplicate := selection.Agents[canonical]; duplicate {
				return bad()
			}
			selection.Agents[canonical] = pair
		}
	}
	if value, ok := fields["roleDefaults"]; ok {
		entries, err := wire.Fields(value, nil)
		if err != nil {
			return bad()
		}
		selection.RoleDefaults = make(map[string]Pair, len(entries))
		for name, value := range entries {
			if _, known := roleRoutes[name]; !known {
				return bad()
			}
			pair, err := ParsePair(value)
			if err != nil {
				return bad()
			}
			selection.RoleDefaults[name] = pair
		}
	}
	return selection, nil
}

func ParsePair(raw []byte) (Pair, error) {
	fields, err := wire.Fields(raw, []string{"model", "effort"})
	var pair Pair
	if err != nil || len(fields) != 2 || json.Unmarshal(fields["model"], &pair.Model) != nil || json.Unmarshal(fields["effort"], &pair.Effort) != nil || !ValidPair(pair) {
		return Pair{}, ErrUnsupportedRoute
	}
	return pair, nil
}

// Clone gives a caller its own maps when it needs to retain a selection.
func (s Selection) Clone() Selection {
	return Selection{ModelDefaults: maps.Clone(s.ModelDefaults), ModelMapping: maps.Clone(s.ModelMapping), Agents: maps.Clone(s.Agents), RoleDefaults: maps.Clone(s.RoleDefaults)}
}

// Snapshot records effective defaults, not just sparse overrides. Otherwise a
// future build's defaults would silently change an old session on resume.
// Builtin fallbacks remain separate from explicit agent overrides so recording
// them never promotes them above a native custom definition.
func (s Selection) Snapshot() Selection {
	copy := s.Clone()
	if copy.ModelDefaults == nil {
		copy.ModelDefaults = map[string]Default{}
	}
	if copy.ModelMapping == nil {
		copy.ModelMapping = map[string]string{}
	}
	if copy.RoleDefaults == nil {
		copy.RoleDefaults = map[string]Pair{}
	}
	for _, model := range Models {
		if _, exists := copy.ModelDefaults[model.ID]; !exists {
			copy.ModelDefaults[model.ID] = Default{Effort: model.Effort}
		}
	}
	for alias, id := range builtinDefaults.ModelMapping {
		if _, exists := copy.ModelMapping[alias]; !exists {
			copy.ModelMapping[alias] = id
		}
	}
	for name, route := range roleRoutes {
		if _, exists := copy.RoleDefaults[name]; !exists {
			copy.RoleDefaults[name] = Pair{Model: route.Model, Effort: route.Effort}
		}
	}
	return copy
}

// ModelByID returns only a backend model supported by this build.
func ModelByID(id string) (Model, bool) {
	for _, model := range Models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

// ValidPair checks both halves against the fixed catalogue.
func ValidPair(pair Pair) bool {
	model, ok := ModelByID(pair.Model)
	return ok && slices.Contains(model.Efforts, pair.Effort)
}

// ForAlias resolves a Claude tier to the configured backend model, if any.
func (s Selection) ForAlias(alias string) (Model, bool) {
	if id, configured := s.ModelMapping[alias]; configured {
		return ModelByID(id)
	}
	return ForAlias(alias)
}

// DefaultFor resolves the configured effort for a backend model.
func (s Selection) DefaultFor(id string) (string, bool) {
	model, ok := ModelByID(id)
	if !ok {
		return "", false
	}
	if configured, ok := s.ModelDefaults[id]; ok {
		return configured.Effort, true
	}
	return model.Effort, true
}

// RoleRoute applies an explicitly configured agent pair, then the fixed role rules.
func (s Selection) RoleRoute(role string) (Route, bool) {
	role = CanonicalRole(role)
	if pair, ok := s.Agents[role]; ok {
		return Route{Model: pair.Model, Effort: pair.Effort, Source: "role"}, true
	}
	if pair, ok := s.RoleDefaults[role]; ok {
		return Route{Model: pair.Model, Effort: pair.Effort, Source: "role"}, true
	}
	if route, ok := roleRoutes[role]; ok {
		return route, true
	}
	if route, ok := menuRoute(role); ok {
		route.Effort, _ = s.DefaultFor(route.Model)
		return route, true
	}
	return Route{}, false
}

// SelectRoute preserves explicit model and effort choices. A configured Claude
// alias applies to both the short name and its versioned family.
func (s Selection) SelectRoute(requested, effort string) (Route, error) {
	if _, retired := Retired[requested]; retired {
		return Route{}, ErrRetiredRoute
	}
	model, source := s.resolve(requested)
	if source == "" || effort != "" && !slices.Contains(model.Efforts, effort) {
		return Route{}, ErrUnsupportedRoute
	}
	if effort == "" {
		effort, _ = s.DefaultFor(model.ID)
		return Route{Model: model.ID, Effort: effort, Source: source}, nil
	}
	return Route{Model: model.ID, Effort: effort, Source: source + "+effort"}, nil
}

func (s Selection) resolve(requested string) (Model, string) {
	model, source := resolveKey(requested)
	if source == "alias" || source == "family" {
		selected, ok := s.ForAlias(model.Alias)
		if !ok {
			return Model{}, ""
		}
		model = selected
	}
	return model, source
}
