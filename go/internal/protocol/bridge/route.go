package bridge

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The client asks for a Claude model. The backend has never heard of one.
//
// Measured 2026-09-15: a real session forwarded "claude-opus-5" verbatim and the backend
// answered 400. Nothing offline could have shown it -- every fixture until now named a
// Codex model directly, because whoever wrote the fixture knew which one to name.
//
// The catalogue and the alias rules are the Node baseline's, read from src/models.mjs and
// src/agent-selection.mjs:23-27 at 1b1c5e1 rather than invented. Which model a request runs on decides
// what it costs, so this is not a place to guess.

// ErrUnsupportedRoute means the requested model or effort is not one this build can route.
//
// CAP02: it is refused rather than quietly replaced. Falling back to a default would run
// the user's request on a model they did not ask for and bill them for it.
var ErrUnsupportedRoute = errors.New("UNSUPPORTED_MODEL_OR_EFFORT")

// ErrRetiredRoute is a route this build offered once and no longer does. It is an
// unsupported route (errors.Is holds), named apart so the refusal can say what replaced it.
var ErrRetiredRoute error = retiredRoute{}

type retiredRoute struct{}

func (retiredRoute) Error() string { return "MODEL_RETIRED" }
func (retiredRoute) Unwrap() error { return ErrUnsupportedRoute }

// Retired maps each backend model this build stopped routing to the model that took its
// tier (v0.3.4: 2026-09-24). A request, a picker value or a journal from an earlier session
// that names one is refused with this answer; running the replacement instead would bill a
// model nobody chose. Product data: models.json.
var Retired = catalogue.Retired

// Route is a resolved destination.
type Route struct {
	Model  string
	Effort string
	// Source records how the route was decided. CAP03 asks for the requested and effective
	// route together with where the answer came from, and a reader who sees a surprising
	// model needs to know which rule produced it.
	Source string
}

// Model is one backend model this build can route to, with every name that reaches it.
//
// Capabilities and legacy native identities live here. User-editable defaults
// come from settingsfile/defaults.json and a session's Selection overrides.
type Model struct {
	// Key is the short catalogue name, which the client may also send outright.
	Key string
	// ID is what the backend calls this model.
	ID string
	// Effort is what it runs at when the request names none. The efforts are not uniform
	// and normalising them would change what a request costs.
	Effort string
	// Efforts is what this build routes the model at, cheapest first. The backend's sets
	// differ by model (2026-09-23 catalogue: ultra on some, max missing from gpt-5.5), so one
	// global list would either refuse what a model takes or send what it refuses. An effort
	// the backend lists but nobody has measured here stays out.
	Efforts []string
	// Alias is the stable legacy native identity. ForAlias and Selection resolve
	// the effective configurable mapping; this field alone is not that mapping.
	Alias string
	// AgentAlias is the tier written into native's Agent tool argument, whose schema only
	// takes Claude tier names. It is Alias, or for a model without one the catalogue's
	// agentAlias. Routing never reads it: the native spawn event pins the full model ID.
	AgentAlias string
	// Family is the versioned Claude prefix for that tier. Matched by prefix so no version
	// is pinned: claude-opus-5 and claude-opus-4-1 route the same way and a new release
	// needs no code change. An unknown name still fails closed.
	Family  string
	Context ContextPolicy
	// CountValidated admits the separately tested token-count input shapes.
	// Routing a new model alone never opts it into an older tokenizer formula.
	CountValidated bool
}

// ContextPolicy supplies preventive management targets, not exact admission caps.
// All models share the launcher's configured window and compaction target.
type ContextPolicy struct {
	Window    int64  `json:"window"`
	CompactAt int64  `json:"compactAt"`
	EffortCap string `json:"autoCompactEffortCap"`
}

// Models is the catalogue in published order.
//
// The values came from the Node baseline's src/models.mjs and src/agent-selection.mjs:23-27 at 1b1c5e1
// rather than from a convention that looked reasonable, with one deliberate divergence
// recorded above: sonnet routes to terra here and to luna there.
//
// v0.3.4 moved opus and haiku to GPT-6 Sol and Luna, keeping each family's default effort
// (there is no GPT-6 Terra). Measured 2026-09-24 with probe accept: both take low..max,
// tools, the reasoning round trip and images, and the local count matched the backend's
// input_tokens on every text request. ultra was refused (HTTP 400) on sol, astra and terra,
// so no model lists it. Context defaults come from the shared settings document.
//
// v0.6.3 added GPT-6.1 Sol as the opus tier and the sol key (decided 2026-10-02). Measured
// the same day with probe accept: low..max accepted, ultra refused (HTTP 400), tools,
// reasoning and an image passed, and the local count matched input_tokens on 5 of 5 text
// requests. GPT-6 Sol stays routable by its ID and the sol6 key; it is not retired.
//
// The entries are product data (models.json), not code: adding a measured model changes
// that file. The loader refuses a malformed catalogue at start, and an unknown model is
// still refused per request; nothing here lets a user setting add a model.
var Models = func() []Model {
	models := slices.Clone(catalogue.Models)
	for i := range models {
		models[i].Effort = builtinDefaults.ModelDefaults[models[i].ID].Effort
		models[i].Context = DefaultContextPolicy()
		if !slices.Contains(models[i].Efforts, models[i].Effort) {
			panic("embedded defaults give no routable effort for " + models[i].ID)
		}
	}
	return models
}()

// lowToMax is the backend's effort vocabulary, cheapest first. A model's own subset is data.
var lowToMax = []string{"low", "medium", "high", "xhigh", "max"}

//go:embed models.json
var catalogueDocument []byte

var catalogueKey = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
var catalogueID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

type catalogueData struct {
	Models         []Model
	Retired        map[string]string
	RetiredKeys    []string
	RetiredEfforts []string
}

var errCatalogue = errors.New("invalid model catalogue")

// catalogue is decoded and checked once; an invalid embedded document stops the program
// rather than routing on a partial table.
var catalogue = func() catalogueData {
	c, err := parseCatalogue(catalogueDocument)
	if err != nil {
		panic("invalid embedded model catalogue: " + err.Error())
	}
	return c
}()

// parseCatalogue refuses unknown fields, unordered or unknown efforts, half-declared
// aliases, duplicate names and retired routes that are live or point nowhere.
func parseCatalogue(document []byte) (c catalogueData, err error) {
	var doc struct {
		Version int `json:"version"`
		Models  []struct {
			Key            string   `json:"key"`
			ID             string   `json:"id"`
			Efforts        []string `json:"efforts"`
			Alias          string   `json:"alias"`
			AgentAlias     string   `json:"agentAlias"`
			Family         string   `json:"family"`
			CountValidated bool     `json:"countValidated"`
		} `json:"models"`
		Retired      map[string]string `json:"retired"`
		RetiredRoles struct {
			Keys    []string `json:"keys"`
			Efforts []string `json:"efforts"`
		} `json:"retiredRoles"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(document)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&doc) != nil || decoder.More() || doc.Version != 1 || len(doc.Models) == 0 {
		return c, errCatalogue
	}
	seen := map[string]bool{}
	for _, m := range doc.Models {
		ordered := len(m.Efforts) > 0
		for i, effort := range m.Efforts {
			at := slices.Index(lowToMax, effort)
			ordered = ordered && at >= 0 && (i == 0 || at > slices.Index(lowToMax, m.Efforts[i-1]))
		}
		names := []string{m.Key, m.ID, m.Alias}
		if !catalogueKey.MatchString(m.Key) || !catalogueID.MatchString(m.ID) || !ordered ||
			(m.Alias == "") != (m.Family == "") || m.Alias != "" && (!catalogueKey.MatchString(m.Alias) || !strings.HasPrefix(m.Family, "claude-")) {
			return c, fmt.Errorf("%w: model %q", errCatalogue, m.ID)
		}
		for _, name := range names {
			if name != "" && seen[name] {
				return c, fmt.Errorf("%w: duplicate name %q", errCatalogue, name)
			}
			seen[name] = true
		}
		agentAlias := m.Alias
		if m.Alias == "" {
			agentAlias = m.AgentAlias
		} else if m.AgentAlias != "" {
			return c, fmt.Errorf("%w: model %q names agentAlias beside its alias", errCatalogue, m.ID)
		}
		c.Models = append(c.Models, Model{Key: m.Key, ID: m.ID, Efforts: m.Efforts, Alias: m.Alias, AgentAlias: agentAlias, Family: m.Family, CountValidated: m.CountValidated})
	}
	// Families are matched by prefix; overlapping prefixes would let one silently win.
	for i, a := range c.Models {
		for _, b := range c.Models[i+1:] {
			if a.Family != "" && b.Family != "" && (strings.HasPrefix(a.Family, b.Family) || strings.HasPrefix(b.Family, a.Family)) {
				return c, fmt.Errorf("%w: overlapping families %q and %q", errCatalogue, a.Family, b.Family)
			}
		}
	}
	// Every model must be delegable: its Agent tier is one of the catalogue's tier aliases.
	for _, m := range c.Models {
		if !slices.ContainsFunc(c.Models, func(t Model) bool { return t.Alias != "" && t.Alias == m.AgentAlias }) {
			return c, fmt.Errorf("%w: model %q has no Agent tier", errCatalogue, m.ID)
		}
	}
	for old, replacement := range doc.Retired {
		if seen[old] || !seen[replacement] || !catalogueID.MatchString(old) {
			return c, fmt.Errorf("%w: retired route %q", errCatalogue, old)
		}
	}
	for _, effort := range doc.RetiredRoles.Efforts {
		if !slices.Contains(lowToMax, effort) {
			return c, fmt.Errorf("%w: retired role effort %q", errCatalogue, effort)
		}
	}
	c.Retired, c.RetiredKeys, c.RetiredEfforts = doc.Retired, doc.RetiredRoles.Keys, doc.RetiredRoles.Efforts
	return c, nil
}

// Catalogue lists the routes this build offers, in published order.
//
// Returned as a copy so a caller that serves this list to a client cannot edit what the
// router resolves against.
func Catalogue() []Route {
	out := make([]Route, 0, len(Models))
	for _, model := range Models {
		out = append(out, Route{Model: model.ID, Effort: model.Effort, Source: "catalogue"})
	}
	return out
}

// Factory role pairs are independent of per-model effort defaults. They remain
// fallbacks, below native definitions, until the user's file sets an agent pair.
var roleRoutes = func() map[string]Route {
	routes := make(map[string]Route, len(builtinDefaults.Agents))
	for name, pair := range builtinDefaults.Agents {
		routes[name] = Route{Model: pair.Model, Effort: pair.Effort, Source: "role"}
	}
	return routes
}()

// inheritRoles are roles this build knows about and deliberately does not reassign.
//
// Measured 2026-09-17: an agent started by the Workflow tool reports agent_type
// "workflow-subagent" -- one fixed name for all of them. The Node baseline gives those the
// parent's own route, which it works out by reading and verifying a run journal on disk.
// Keeping the client's model is the same answer arrived at by not doing that, because the
// client already sends the parent's model.
//
// Named rather than left to fall through, because falling through is counted as a routing
// miss. Every workflow agent would bump that counter, every session that used one would be
// reported as having something wrong with it, and a diagnostic that cries wolf on ordinary
// use stops being read -- which is the failure it exists to prevent. This is the same
// distinction the beta report makes between a name nobody has classified and one that has
// been looked at.
var inheritRoles = map[string]bool{
	"workflow-subagent": true,
	"fork":              true, // Native forks retain the parent's model and history.
	// The launcher's own inherit entry, and the reason this is a named constant rather
	// than a string in two files. Measured in a real session 2026-09-18: the menu shipped
	// clauduct-inherit, the router had never heard of it, and every session that delegated
	// to it was filed with agents.unrouted=1 and dumped its whole account at exit -- the
	// cry-wolf failure this map exists to prevent, about this build's own agent.
	//
	// menuRoute cannot cover it: "inherit" names no model, because there is no model to
	// name. That is the point of it.
	InheritRole: true,
}

// InheritRole is the agent type that keeps whatever the parent was running on.
//
// Defined here, beside the router that has to recognise it, and used by the launcher that
// builds the menu. One spelling: two was the defect.
const InheritRole = MenuPrefix + "inherit"

// InheritsParent reports whether this role deliberately keeps the model the client chose.
func InheritsParent(role string) bool {
	return inheritRoles[CanonicalRole(role)]
}

// IsFork folds the one role name five call sites compared exactly. Native resolves these
// case-insensitively; this build did not, in five different places.
func IsFork(role string) bool { return CanonicalRole(role) == "fork" }

// CanonicalRole names the built-ins in the role tables. Callers must resolve
// custom definitions first: native allows a custom Fork distinct from fork.
func CanonicalRole(role string) string {
	for name := range roleRoutes {
		if strings.EqualFold(name, role) {
			return name
		}
	}
	for name := range inheritRoles {
		if strings.EqualFold(name, role) {
			return name
		}
	}
	return role
}

// BuiltinRole is one of native's own roles in the role table (not a menu entry).
func BuiltinRole(role string) bool {
	_, ok := roleRoutes[CanonicalRole(role)]
	return ok
}

func KnownRole(role string) bool {
	_, routed := RoleRoute(role)
	return routed || InheritsParent(role)
}

// RoleRoute reports where a subagent of this role runs.
//
// A role nobody has a route for is not an error and not a guess here: this function reports
// what it knows and invents nothing, because inventing one would run the user's work
// somewhere the user did not choose.
//
// The caller tracks an unknown role as pending until native's actual model and
// effort are verified. An effort alone still needs a known role model.
func RoleRoute(role string) (Route, bool) {
	role = CanonicalRole(role)
	if route, known := roleRoutes[role]; known {
		return route, true
	}
	return menuRoute(role)
}

// MenuPrefix begins the name of every agent type this build defines.
const MenuPrefix = "clauduct-"

// menuRoute reads a route out of an agent type's own name.
//
// The launcher defines one agent type per model, clauduct-<key>, so the user can send a
// piece of work to a chosen model. The route is that model at its default effort; an effort
// the caller asks for comes through the gateway's Agent effort argument, which native lacks.
// The name already carries the model, the hook already reports the name, and the request
// already arrives with the identifier that finds it. The definition keeps its model so the
// client's own accounting is right; this decides what the backend is actually asked for.
//
// clauduct-inherit names no model and gets no route, which is the whole point of it: the
// child keeps the parent's.
func menuRoute(role string) (Route, bool) {
	key, ok := strings.CutPrefix(role, MenuPrefix)
	for _, model := range Models {
		if ok && model.Key == key {
			return Route{Model: model.ID, Effort: model.Effort, Source: "role"}, true
		}
	}
	return Route{}, false
}

// RetiredRole reports an agent type from the per-effort menu v0.3.4 replaced, such as
// clauduct-sol-high. Those names meant a GPT-5.6 model for sol and luna, so reading them
// through the new table would silently run GPT-6; they are refused, astra and terra alike,
// rather than parsed (decided 2026-09-24).
//
// The keys and efforts are that menu's, written out: they are history, and a later table change
// must not widen or narrow what counts as an old name.
func RetiredRole(role string) bool {
	rest, ok := strings.CutPrefix(role, MenuPrefix)
	key, effort, split := strings.Cut(rest, "-")
	return ok && split && slices.Contains(catalogue.RetiredKeys, key) && slices.Contains(catalogue.RetiredEfforts, effort)
}

// ForAlias reports the model a Claude tier belongs to.
//
// The default mapping comes from the same document used to create preferences.
func ForAlias(alias string) (Model, bool) {
	return ModelByID(builtinDefaults.ModelMapping[alias])
}

// Efforts is every effort some model accepts, cheapest first: the union of the models' own
// sets, for the places that can only hold one list (the Agent schema, receipt labels). An
// effort outside a model's set is refused rather than clamped: clamping "max" down to "high"
// would quietly produce a cheaper, worse answer than the one that was asked for.
var Efforts = func() (all []string) {
	for _, model := range Models {
		for _, effort := range model.Efforts {
			if !slices.Contains(all, effort) {
				all = append(all, effort)
			}
		}
	}
	return all
}()

// SelectRoute resolves what the client asked for into what the backend understands.
//
// effort is the caller's explicit choice and may be empty, in which case the model's own
// default applies. An empty model is refused: a request that names no model is not one to
// answer with a guess.
func SelectRoute(requested, effort string) (Route, error) {
	return (Selection{}).SelectRoute(requested, effort)
}

// resolveKey names the catalogue entry a requested model refers to, and the rule that said
// so. The order is the baseline's: an exact catalogue key, then an alias, then a family
// prefix, then a Codex model id named directly.
func resolveKey(requested string) (model Model, source string) {
	if requested == "" {
		return Model{}, ""
	}
	for _, candidate := range Models {
		if candidate.Key == requested {
			return candidate, "catalogue"
		}
	}
	for _, candidate := range Models {
		if candidate.Alias == requested {
			return candidate, "alias"
		}
	}
	for _, candidate := range Models {
		if candidate.Family != "" && strings.HasPrefix(requested, candidate.Family) {
			return candidate, "family"
		}
	}
	// A request naming a backend model outright. Accepted because the native client can be
	// pointed at one, and refusing would break a route the baseline allows.
	for _, candidate := range Models {
		if candidate.ID == requested {
			return candidate, "direct"
		}
	}
	return Model{}, ""
}
