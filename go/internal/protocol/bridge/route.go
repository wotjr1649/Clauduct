package bridge

import (
	"errors"
	"strings"
)

// The client asks for a Claude model. The backend has never heard of one.
//
// Measured 2026-09-15: a real session forwarded "claude-opus-5" verbatim and the backend
// answered 400. Nothing offline could have shown it -- every fixture until now named a
// Codex model directly, because whoever wrote the fixture knew which one to name.
//
// The catalogue and the alias rules are the Node baseline's, read from src/models.mjs and
// src/agent-selection.mjs:23-27 rather than invented. Which model a request runs on decides
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
	// Alias is the Claude tier that belongs here.
	Alias string
	// Family is the versioned Claude prefix for that tier. Matched by prefix so no version
	// is pinned: claude-opus-5 and claude-opus-4-1 route the same way and a new release
	// needs no code change. An unknown name still fails closed.
	Family string
}

// Models is the catalogue in published order.
//
// The values came from the Node baseline's src/models.mjs and src/agent-selection.mjs:23-27
// rather than from a convention that looked reasonable, with one deliberate divergence
// recorded above: sonnet routes to terra here and to luna there.
var Models = []Model{
	{Key: "astra", ID: "gpt-6-astra", Effort: "medium", Alias: "fable", Family: "claude-fable-"},
	{Key: "sol", ID: "gpt-5.6-sol", Effort: "xhigh", Alias: "opus", Family: "claude-opus-"},
	{Key: "terra", ID: "gpt-5.6-terra", Effort: "high", Alias: "sonnet", Family: "claude-sonnet-"},
	{Key: "luna", ID: "gpt-5.6-luna", Effort: "max", Alias: "haiku", Family: "claude-haiku-"},
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

// roleRoutes is where a subagent of a given role runs, whatever model the client asked for.
//
// The Node baseline's ROLE_MODELS. The point of it is that a role's cost is a property of
// the role: exploring a repository and planning a change are not the same work, and neither
// is the model the conversation happens to be using. Plan runs on the top model at the
// cheapest effort because a plan is short and wants judgement; the other two run on the
// cheapest model at its own default.
var roleRoutes = map[string]Route{
	"Explore":         {Model: "gpt-5.6-luna", Effort: "max", Source: "role"},
	"Plan":            {Model: "gpt-6-astra", Effort: "low", Source: "role"},
	"general-purpose": {Model: "gpt-5.6-luna", Effort: "max", Source: "role"},
}

// RoleRoute reports where a subagent of this role runs.
//
// A role nobody has a route for is not an error and not a guess: the caller keeps the model
// the client asked for. Inventing one would run the user's work somewhere they did not
// choose, and refusing would end a turn over a routing preference.
func RoleRoute(role string) (Route, bool) {
	route, known := roleRoutes[role]
	return route, known
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

// efforts is the set the backend accepts. An effort outside it is refused rather than
// clamped: clamping "max" down to "high" would quietly produce a cheaper, worse answer than
// the one that was asked for.
var efforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// SelectRoute resolves what the client asked for into what the backend understands.
//
// effort is the caller's explicit choice and may be empty, in which case the model's own
// default applies. An empty model is refused: a request that names no model is not one to
// answer with a guess.
func SelectRoute(requested, effort string) (Route, error) {
	if effort != "" && !efforts[effort] {
		return Route{}, ErrUnsupportedRoute
	}

	model, source := resolveKey(requested)
	if source == "" {
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
