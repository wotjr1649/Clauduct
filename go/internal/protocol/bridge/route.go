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

// catalogue is the set of models this backend offers, with the effort each one runs at when
// the request does not say. The efforts are the baseline's: they are not uniform, and
// normalising them would change what a request costs.
var catalogue = map[string]Route{
	"astra": {Model: "gpt-6-astra", Effort: "medium"},
	"sol":   {Model: "gpt-5.6-sol", Effort: "xhigh"},
	"terra": {Model: "gpt-5.6-terra", Effort: "high"},
	"luna":  {Model: "gpt-5.6-luna", Effort: "max"},
}

// aliases are the short names the native client accepts.
var aliases = map[string]string{
	"haiku":  "luna",
	"sonnet": "luna",
	"opus":   "sol",
	"fable":  "astra",
}

// families match a versioned Claude model id by prefix.
//
// By prefix so that no version suffix is pinned here: claude-opus-5 and claude-opus-4-1
// route the same way, and a new release does not need a code change to work. An unknown
// name still fails closed.
var families = []struct{ prefix, key string }{
	{"claude-haiku-", "luna"},
	{"claude-sonnet-", "luna"},
	{"claude-opus-", "sol"},
	{"claude-fable-", "astra"},
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

	key, source := resolveKey(requested)
	if key == "" {
		return Route{}, ErrUnsupportedRoute
	}
	route := catalogue[key]
	route.Source = source
	if effort != "" {
		route.Effort = effort
		route.Source = source + "+effort"
	}
	return route, nil
}

// resolveKey names the catalogue entry a requested model refers to, and the rule that said
// so. The order is the baseline's: an exact alias, then a family prefix, then a Codex model
// id named directly.
func resolveKey(requested string) (key, source string) {
	if requested == "" {
		return "", ""
	}
	if _, known := catalogue[requested]; known {
		return requested, "catalogue"
	}
	if mapped, known := aliases[requested]; known {
		return mapped, "alias"
	}
	for _, family := range families {
		if strings.HasPrefix(requested, family.prefix) {
			return family.key, "family"
		}
	}
	// A request naming a backend model outright. Accepted because the native client can be
	// pointed at one, and refusing would break a route the baseline allows.
	for name, route := range catalogue {
		if route.Model == requested {
			return name, "direct"
		}
	}
	return "", ""
}
