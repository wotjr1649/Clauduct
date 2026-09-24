package bridge

import (
	"errors"
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
// One entry per model and one place to edit. The routing table, the published order, the
// client's model list and the client's own tier defaults are all derived from this slice,
// so adding a model when the backend ships one is adding a line here and nothing else.
// Before this was a single list the same facts lived in three tables, and they drifted:
// sonnet and haiku both pointed at luna while terra had no Claude name at all, so two
// entries in the user's picker ran the identical route and a fourth model could not be
// selected. TestEveryModelIsReachableByAClaudeName now fails if that happens again.
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
	// Alias is the Claude tier that belongs here.
	Alias string
	// Family is the versioned Claude prefix for that tier. Matched by prefix so no version
	// is pinned: claude-opus-5 and claude-opus-4-1 route the same way and a new release
	// needs no code change. An unknown name still fails closed.
	Family  string
	Context ContextPolicy
	// CountValidated admits the separately tested token-count input shapes.
	// Routing a new model alone never opts it into an older tokenizer formula.
	CountValidated bool
}

// ContextPolicy supplies preventive management targets, not exact admission caps. Native displays its
// shared process envelope; it does not display an independent child window.
type ContextPolicy struct {
	Window    int64 `json:"window"`
	CompactAt int64 `json:"compactAt"`
}

// Models is the catalogue in published order.
//
// The values came from the Node baseline's src/models.mjs and src/agent-selection.mjs:23-27 at 1b1c5e1
// rather than from a convention that looked reasonable, with one deliberate divergence
// recorded above: sonnet routes to terra here and to luna there.
var Models = []Model{
	{Key: "astra", ID: "gpt-6-astra", Effort: "medium", Efforts: lowToMax, Alias: "fable", Family: "claude-fable-", Context: ContextPolicy{500000, 450000}, CountValidated: true},
	{Key: "sol", ID: "gpt-5.6-sol", Effort: "xhigh", Efforts: lowToMax, Alias: "opus", Family: "claude-opus-", Context: ContextPolicy{272000, 239000}, CountValidated: true},
	{Key: "terra", ID: "gpt-5.6-terra", Effort: "high", Efforts: lowToMax, Alias: "sonnet", Family: "claude-sonnet-", Context: ContextPolicy{272000, 239000}, CountValidated: true},
	{Key: "luna", ID: "gpt-5.6-luna", Effort: "max", Efforts: lowToMax, Alias: "haiku", Family: "claude-haiku-", Context: ContextPolicy{272000, 239000}, CountValidated: true},
}

var lowToMax = []string{"low", "medium", "high", "xhigh", "max"}

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

// roleRoutes is where a subagent of a given role runs, whatever model the client asked for.
//
// The Node baseline's ROLE_MODELS. The point of it is that a role's cost is a property of
// the role: exploring a repository and planning a change are not the same work, and neither
// is the model the conversation happens to be using. Plan runs on the top model because a
// plan is short and wants judgement; the other two run on the cheapest model at its own
// default.
//
// The second recorded divergence from the baseline, after sonnet. The baseline gives Plan
// the cheapest effort (src/models.mjs:20-21 at 1b1c5e1) and so did this until 2026-09-18, when the user
// raised it to medium: a plan is the one piece of work whose mistakes are paid for by
// everything built on it, and low was buying the saving in the wrong place.
//
// medium is also astra's catalogue default, which is a coincidence and not a reason. This
// entry must stay written out: an override pins the model as well as the effort, and letting
// it fall through to the catalogue would leave a Plan running on whatever the conversation
// happened to ask for.
var roleRoutes = map[string]Route{
	"Explore":         {Model: "gpt-5.6-luna", Effort: "max", Source: "role"},
	"Plan":            {Model: "gpt-6-astra", Effort: "medium", Source: "role"},
	"general-purpose": {Model: "gpt-5.6-luna", Effort: "max", Source: "role"},
}

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
	// menuRoute cannot cover it: "inherit" carries no -<effort> suffix to parse, because
	// there is no effort to name. That is the point of it.
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
// The launcher defines agent types called clauduct-<model>-<effort> so the user can send a
// piece of work to a chosen model. Native 2.1.276 supports a definition's effort
// (verified 2026-09-18). The gateway fixes the selected effort here as well, so
// native presentation or silent downgrades cannot change the backend route.
//
// So the effort comes from here instead. The name already carries it, the hook already
// reports the name, and the request already arrives with the identifier that finds it. The
// definition keeps its model so the client's own accounting is right; this decides what the
// backend is actually asked for.
//
// clauduct-inherit has no effort in its name and gets no route, which is the whole point of
// it: the child keeps the parent's.
func menuRoute(role string) (Route, bool) {
	if !strings.HasPrefix(role, MenuPrefix) {
		return Route{}, false
	}
	key, effort, split := strings.Cut(strings.TrimPrefix(role, MenuPrefix), "-")
	if !split {
		return Route{}, false
	}
	for _, model := range Models {
		if model.Key == key && slices.Contains(model.Efforts, effort) {
			return Route{Model: model.ID, Effort: effort, Source: "role"}, true
		}
	}
	return Route{}, false
}

// ForAlias reports the model a Claude tier belongs to.
//
// The launcher needs this to tell the client which backend model stands in for each of its
// own tiers, and deriving it here rather than repeating the pairs there is what keeps the
// two from disagreeing.
func ForAlias(alias string) (Model, bool) {
	for _, model := range Models {
		if model.Alias == alias {
			return model, true
		}
	}
	return Model{}, false
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
	model, source := resolveKey(requested)
	if source == "" || effort != "" && !slices.Contains(model.Efforts, effort) {
		return Route{}, ErrUnsupportedRoute
	}
	route := Route{Model: model.ID, Effort: model.Effort, Source: source}
	if effort != "" {
		route.Effort = effort
		route.Source = source + "+effort"
	}
	return route, nil
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
