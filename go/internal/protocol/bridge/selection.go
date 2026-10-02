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

// Selection contains one session's read-only routing preferences and the account model
// list fixed at session start. Preferences are checked for shape when parsed and against
// the account list when a request actually uses them: an entry naming a model the account
// no longer lists is kept, and only selecting it fails. Global context preferences are
// launch preferences, not saved selection state.
type Selection struct {
	ModelDefaults map[string]Default `json:"modelDefaults,omitempty"`
	ModelMapping  map[string]string  `json:"modelMapping,omitempty"`
	Agents        map[string]Pair    `json:"agents,omitempty"`
	// RoleDefaults freezes builtin fallbacks in saved sessions. The manual
	// settings schema exposes Agents, not this snapshot field.
	RoleDefaults map[string]Pair `json:"roleDefaults,omitempty"`

	// catalogue is the session's account list. Never saved: a resumed session checks its
	// saved choice against the list of the session that resumes it.
	catalogue *Catalogue
}

// ParseSelection applies the same strict schema to preferences and saved sessions.
// It checks shape only; availability belongs to the session's account list.
func ParseSelection(raw []byte) (Selection, error) {
	bad := func() (Selection, error) { return Selection{}, ErrUnsupportedRoute }
	fields, err := wire.Fields(raw, []string{"modelDefaults", "modelMapping", "agents", "roleDefaults"})
	if err != nil {
		return bad()
	}
	var selection Selection
	if value, ok := fields["modelDefaults"]; ok {
		entries, err := wire.Fields(value, nil)
		if err != nil || len(entries) > 1024 {
			return bad()
		}
		selection.ModelDefaults = make(map[string]Default, len(entries))
		for id, value := range entries {
			fields, err := wire.Fields(value, []string{"effort"})
			var effort string
			if !ValidModelID(id) || err != nil || len(fields) != 1 || json.Unmarshal(fields["effort"], &effort) != nil || !TransmittableEffort(effort) {
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
			var id string
			if !LegacyAlias(alias) || json.Unmarshal(value, &id) != nil || !ValidModelID(id) {
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

// ParsePair requires both halves in the shape a backend model and a transmittable effort
// have. Whether the account offers the pair is checked when it is used.
func ParsePair(raw []byte) (Pair, error) {
	fields, err := wire.Fields(raw, []string{"model", "effort"})
	var pair Pair
	if err != nil || len(fields) != 2 || json.Unmarshal(fields["model"], &pair.Model) != nil || json.Unmarshal(fields["effort"], &pair.Effort) != nil ||
		!ValidModelID(pair.Model) || !TransmittableEffort(pair.Effort) {
		return Pair{}, ErrUnsupportedRoute
	}
	return pair, nil
}

// WithCatalogue fixes the session's account list.
func (s Selection) WithCatalogue(catalogue *Catalogue) Selection {
	s.catalogue = catalogue
	return s
}

// Catalogue is the session's account list; nil when none was fixed.
func (s Selection) Catalogue() *Catalogue { return s.catalogue }

// Clone gives a caller its own maps when it needs to retain a selection.
func (s Selection) Clone() Selection {
	return Selection{ModelDefaults: maps.Clone(s.ModelDefaults), ModelMapping: maps.Clone(s.ModelMapping), Agents: maps.Clone(s.Agents), RoleDefaults: maps.Clone(s.RoleDefaults), catalogue: s.catalogue}
}

// Snapshot records the effective alias mapping and role fallbacks, not just sparse
// overrides, so a future build's defaults never silently change an old session on resume.
// modelDefaults is recorded as configured: a model it does not name takes the account's
// default level of the session that runs (v0.6.4), never a hidden factory value. Builtin
// fallbacks remain separate from explicit agent overrides so recording them never
// promotes them above a native custom definition.
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

// ModelByID returns the account model with this ID.
func (s Selection) ModelByID(id string) (Model, bool) { return s.catalogue.ByID(id) }

// ValidPair checks both halves against the session's account list.
func (s Selection) ValidPair(pair Pair) bool {
	model, ok := s.ModelByID(pair.Model)
	return ok && slices.Contains(model.Efforts, pair.Effort)
}

// ForAlias resolves a Claude tier to the configured backend model, if the account offers it.
func (s Selection) ForAlias(alias string) (Model, bool) {
	id, configured := s.ModelMapping[alias]
	if !configured {
		id = builtinDefaults.ModelMapping[alias]
	}
	return s.ModelByID(id)
}

// DefaultFor resolves the effort for a model chosen without one: the session's
// modelDefaults, then the account's default level. A configured effort the model does not
// take is an error, not a reason to use another one.
func (s Selection) DefaultFor(id string) (string, bool) {
	model, ok := s.ModelByID(id)
	if !ok {
		return "", false
	}
	if configured, ok := s.ModelDefaults[id]; ok {
		return configured.Effort, slices.Contains(model.Efforts, configured.Effort)
	}
	return model.Effort, model.Effort != ""
}

// RoleRoute applies an explicitly configured agent pair, then the fixed role rules.
// The pair is checked against the account list when a request uses it.
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
	return s.menuRoute(role)
}

// SelectRoute preserves explicit model and effort choices. A configured Claude
// alias applies to both the short name and its versioned family.
func (s Selection) SelectRoute(requested, effort string) (Route, error) {
	model, source := s.resolveKey(requested)
	if source == "" {
		if _, retired := Retired[requested]; retired {
			return Route{}, ErrRetiredRoute
		}
		return Route{}, ErrUnsupportedRoute
	}
	if effort != "" && !slices.Contains(model.Efforts, effort) {
		return Route{}, ErrUnsupportedRoute
	}
	if effort == "" {
		effort, ok := s.DefaultFor(model.ID)
		if !ok {
			return Route{}, ErrUnsupportedRoute
		}
		return Route{Model: model.ID, Effort: effort, Source: source}, nil
	}
	return Route{Model: model.ID, Effort: effort, Source: source + "+effort"}, nil
}

// Problems lists preferences the account list cannot honour. They are kept as written and
// only fail when a request selects them; the launcher shows them so the user can edit.
func (s Selection) Problems() []string {
	var problems []string
	for _, id := range slices.Sorted(maps.Keys(s.ModelDefaults)) {
		model, ok := s.ModelByID(id)
		switch {
		case !ok:
			problems = append(problems, "modelDefaults."+id+": not in the account model list")
		case !slices.Contains(model.Efforts, s.ModelDefaults[id].Effort):
			problems = append(problems, "modelDefaults."+id+": effort "+s.ModelDefaults[id].Effort+" is not supported")
		}
	}
	for _, alias := range slices.Sorted(maps.Keys(s.ModelMapping)) {
		if _, ok := s.ForAlias(alias); !ok {
			problems = append(problems, "modelMapping."+alias+": "+s.ModelMapping[alias]+" is not in the account model list")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.Agents)) {
		if !s.ValidPair(s.Agents[name]) {
			problems = append(problems, "agents."+name+": "+s.Agents[name].Model+"/"+s.Agents[name].Effort+" is not offered by the account")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.RoleDefaults)) {
		if _, explicit := s.Agents[name]; !explicit && !s.ValidPair(s.RoleDefaults[name]) {
			problems = append(problems, "agents."+name+" (factory): "+s.RoleDefaults[name].Model+"/"+s.RoleDefaults[name].Effort+" is not offered by the account")
		}
	}
	return problems
}
