package bridge

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// tier (v0.3.4: 2026-09-24). It only explains a refusal: a name the account's list still
// offers is routed normally, and one it lacks is refused with this answer. Running the
// replacement instead would bill a model nobody chose. Product data: models.json.
var Retired = legacy.Retired

// Route is a resolved destination.
type Route struct {
	Model  string
	Effort string
	// Source records how the route was decided. CAP03 asks for the requested and effective
	// route together with where the answer came from, and a reader who sees a surprising
	// model needs to know which rule produced it.
	Source string
}

// Model is one backend model a session can route to, with every name that reaches it.
//
// The account's model list decides which models exist and which efforts they take
// (Catalogue). The embedded models.json only keeps the stable legacy identities --
// short keys, Claude aliases and families, the native Agent tier -- and the
// offline token-count validation. It no longer limits what a user may select.
type Model struct {
	// Key is the legacy short catalogue name, which the client may also send outright.
	// Empty for a model this build has no legacy identity for.
	Key string
	// ID is what the backend calls this model.
	ID string
	// Effort is the account's default_reasoning_level, or empty when the account names
	// none this build can transmit. It applies only after an explicit request effort
	// and the session's modelDefaults; it is never replaced by a guess.
	Effort string
	// Efforts are the account's supported levels this build can transmit, cheapest first.
	// A level the account lists but native cannot express (for example ultra) stays out
	// and is refused rather than clamped.
	Efforts []string
	// Visible is the account's picker visibility. Hidden models stay selectable by ID.
	Visible bool
	// Alias is the stable legacy native identity. ForAlias and Selection resolve
	// the effective configurable mapping; this field alone is not that mapping.
	Alias string
	// AgentAlias is the legacy tier written into native's Agent tool argument. Empty for a
	// model without a legacy identity: no tier is guessed for it, the full ID is used.
	AgentAlias string
	// Family is the versioned Claude prefix for that tier. Matched by prefix so no version
	// is pinned: claude-opus-5 and claude-opus-4-1 route the same way and a new release
	// needs no code change. An unknown name still fails closed.
	Family string
	// Menu names this model's delegation menu entry (MenuPrefix+Menu), or is empty when
	// the name would collide with another entry.
	Menu string
	// CountValidated admits the separately tested local token-count input shapes.
	// Routing a new model alone never opts it into an older tokenizer formula.
	CountValidated bool
}

// ContextPolicy supplies preventive management targets, not exact admission caps.
// All models share the launcher's configured window and compaction target.
type ContextPolicy struct {
	Window    int64 `json:"window"`
	CompactAt int64 `json:"compactAt"`
}

// lowToMax is the effort vocabulary native can express and this build transmits,
// cheapest first. A model's own subset comes from the account.
var lowToMax = []string{"low", "medium", "high", "xhigh", "max"}

// TransmittableEffort reports whether native can express effort and this build sends it.
func TransmittableEffort(effort string) bool { return slices.Contains(lowToMax, effort) }

//go:embed models.json
var catalogueDocument []byte

var catalogueKey = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
var catalogueID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

// ValidModelID is the shape every backend model ID must have before it is stored,
// shown to native or routed.
func ValidModelID(id string) bool { return catalogueID.MatchString(id) }

type catalogueData struct {
	Models         []Model
	Retired        map[string]string
	RetiredKeys    []string
	RetiredEfforts []string
}

var errCatalogue = errors.New("invalid model catalogue")

// legacy is decoded and checked once; an invalid embedded document stops the program
// rather than routing on a partial table. It holds identities, not availability.
var legacy = func() catalogueData {
	c, err := parseCatalogue(catalogueDocument)
	if err != nil {
		panic("invalid embedded model catalogue: " + err.Error())
	}
	return c
}()

// parseCatalogue refuses unknown fields, half-declared aliases, duplicate names and
// retired routes that are live or point nowhere.
func parseCatalogue(document []byte) (c catalogueData, err error) {
	var doc struct {
		Version int `json:"version"`
		Models  []struct {
			Key            string `json:"key"`
			ID             string `json:"id"`
			Alias          string `json:"alias"`
			AgentAlias     string `json:"agentAlias"`
			Family         string `json:"family"`
			CountValidated bool   `json:"countValidated"`
		} `json:"models"`
		Retired      map[string]string `json:"retired"`
		RetiredRoles struct {
			Keys    []string `json:"keys"`
			Efforts []string `json:"efforts"`
		} `json:"retiredRoles"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(document)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&doc) != nil || decoder.Decode(&struct{}{}) != io.EOF || doc.Version != 1 || len(doc.Models) == 0 {
		return c, errCatalogue
	}
	seen := map[string]bool{}
	for _, m := range doc.Models {
		names := []string{m.Key, m.ID, m.Alias}
		if !catalogueKey.MatchString(m.Key) || !catalogueID.MatchString(m.ID) ||
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
		c.Models = append(c.Models, Model{Key: m.Key, ID: m.ID, Alias: m.Alias, AgentAlias: agentAlias, Family: m.Family, Menu: m.Key, CountValidated: m.CountValidated})
	}
	// Families are matched by prefix; overlapping prefixes would let one silently win.
	for i, a := range c.Models {
		for _, b := range c.Models[i+1:] {
			if a.Family != "" && b.Family != "" && (strings.HasPrefix(a.Family, b.Family) || strings.HasPrefix(b.Family, a.Family)) {
				return c, fmt.Errorf("%w: overlapping families %q and %q", errCatalogue, a.Family, b.Family)
			}
		}
	}
	// Every legacy model must be delegable: its Agent tier is one of the catalogue's tier aliases.
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

// LegacyModels lists the embedded legacy identities in published order. They say what
// old names mean, never which models an account offers.
func LegacyModels() []Model { return slices.Clone(legacy.Models) }

func legacyByID(id string) (Model, bool) {
	for _, model := range legacy.Models {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
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

// KnownRole reports a role this session can route or deliberately leaves to the parent.
func (s Selection) KnownRole(role string) bool {
	_, routed := s.RoleRoute(role)
	return routed || InheritsParent(role)
}

// MenuPrefix begins the name of every agent type this build defines.
const MenuPrefix = "clauduct-"

// menuRoute reads a route out of an agent type's own name.
//
// The launcher defines one agent type per account model, clauduct-<menu>, so the user can
// send a piece of work to a chosen model. The route is that model at its default effort; an
// effort the caller asks for comes through the gateway's Agent effort argument, which native
// lacks. The name already carries the model, the hook already reports the name, and the
// request already arrives with the identifier that finds it.
//
// clauduct-inherit names no model and gets no route, which is the whole point of it: the
// child keeps the parent's.
func (s Selection) menuRoute(role string) (Route, bool) {
	menu, ok := strings.CutPrefix(role, MenuPrefix)
	if !ok || menu == "" {
		return Route{}, false
	}
	for _, model := range s.catalogue.Models() {
		if model.Menu == menu {
			effort, _ := s.DefaultFor(model.ID)
			return Route{Model: model.ID, Effort: effort, Source: "role"}, true
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
	return ok && split && slices.Contains(legacy.RetiredKeys, key) && slices.Contains(legacy.RetiredEfforts, effort)
}

// LegacyAlias reports a Claude tier name this build maps (fable, opus, sonnet, haiku).
func LegacyAlias(alias string) bool {
	return alias != "" && slices.ContainsFunc(legacy.Models, func(m Model) bool { return m.Alias == alias })
}

// resolveKey names the model a requested name refers to, and the rule that said so. The
// order is the baseline's: an exact legacy key, then an alias, then a family prefix, then a
// backend model id named directly. Aliases and families resolve through the session's
// mapping; every answer must be in the session's account list.
func (s Selection) resolveKey(requested string) (model Model, source string) {
	if requested == "" {
		return Model{}, ""
	}
	for _, candidate := range legacy.Models {
		if candidate.Key == requested {
			model, ok := s.ModelByID(candidate.ID)
			if !ok {
				return Model{}, ""
			}
			return model, "catalogue"
		}
	}
	for _, candidate := range legacy.Models {
		if candidate.Alias == requested || candidate.Family != "" && strings.HasPrefix(requested, candidate.Family) {
			model, ok := s.ForAlias(candidate.Alias)
			if !ok {
				return Model{}, ""
			}
			if candidate.Alias == requested {
				return model, "alias"
			}
			return model, "family"
		}
	}
	// A request naming a backend model outright. Accepted because the native client can be
	// pointed at one, and refusing would break a route the baseline allows.
	if model, ok := s.ModelByID(requested); ok {
		return model, "direct"
	}
	return Model{}, ""
}
