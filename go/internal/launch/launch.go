// Package launch builds the exact specification for running the native executable:
// argument vector, environment and working directory. It computes; it does not spawn.
// Keeping the spawn out means every argv and environment rule can be asserted without a
// process, which is what makes the WP01 argument-fidelity tests cheap enough to run often.
package launch

import (
	"sort"
	"strings"
)

// Spec is what to run. Env entries are "KEY=VALUE" and are sorted, so two runs with the
// same inputs produce byte-identical specs and a diff in a test failure is readable.
type Spec struct {
	File string
	Args []string
	Dir  string
	Env  []string
}

// Overlay is the entire set of session values Clauduct adds to the child environment.
// It is this small on purpose: the connection needs an endpoint and a token, and nothing
// else in WP01 has earned a key. Context window, model defaults, retry counts, telemetry
// and compaction are policy the gateway has not implemented yet; adding their keys now
// would claim a behaviour that no code backs.
type Overlay struct {
	BaseURL   string
	AuthToken string
	// Session is what this build would like the child to run with, and a name the user
	// already set in their own environment wins over it.
	//
	// Defaults rather than orders, which is where this parts company with the Node
	// baseline: that one assigns its whole settings block over the inherited environment
	// unconditionally, and compensates by owning --model and --effort on the command line.
	// Owning an option means knowing which native options consume a following value, which
	// is the tracking its own blocklist needed and still got wrong. Measured 2026-09-16: a
	// --model on the command line already beats ANTHROPIC_MODEL, so the user keeps that
	// choice without anything here parsing an argument -- and letting their environment win
	// gives them the effort choice too, which the baseline hands out as --effort.
	Session map[string]string
	// Enforced is what the child does not get to run without, whatever the environment
	// says. Kept apart from Session because "we prefer this" and "this build cannot
	// function otherwise" are different claims and should not be made by the same map.
	Enforced map[string]string
}

// denied reports whether a parent environment entry must not reach the native child.
//
// The list is this short by decision, not by omission. The invariant that matters is
// ENV02: the user's real Anthropic credential must never reach the loopback gateway or the
// upstream backend. Two mechanisms enforce it and neither is a name heuristic — every
// ANTHROPIC_* name is dropped and then overwritten with a session value, and the OAuth
// token is blanked. A broad "contains TOKEN or SECRET" rule buys nothing extra for that
// invariant while costing real compatibility: measured against the Node baseline's rule on
// a developer machine, three of its five hits were CLAUDE_CODE_MAX_OUTPUT_TOKENS,
// CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS and MAX_MCP_OUTPUT_TOKENS — size limits, caught
// because "TOKENS" contains "TOKEN" — and any real MCP server credential such as a Slack
// or Notion key was dropped too, which is why MCP servers do not work under the baseline.
//
// The tradeoff the user accepted: the native child now sees the same secrets it would see
// if the user ran claude directly. Clauduct is not an extra secret barrier and must not be
// described as one.
func denied(key string) bool {
	upper := strings.ToUpper(key)
	return strings.HasPrefix(upper, "ANTHROPIC_") || upper == "CLAUDE_CODE_OAUTH_TOKEN"
}

// Build produces the launch spec.
//
// forward is copied verbatim. There is no argument parser here and there must not be one:
// the product launcher owns no option, so no value can be mistaken for one. That is what
// makes `--append-system-prompt "--model is a string"` safe, and it is also why a future
// decision to refuse some native options has to be a deliberate, separately tested
// addition rather than a side effect of having a parser lying around.
func Build(exe string, forward []string, source map[string]string, cwd string, overlay Overlay) Spec {
	args := make([]string, len(forward))
	copy(args, forward)

	return Spec{
		File: exe,
		Args: args,
		Dir:  cwd,
		Env:  buildEnv(source, overlay),
	}
}

func buildEnv(source map[string]string, overlay Overlay) []string {
	// Windows environment names are case-insensitive but a Go map is not, so a source map
	// can hold both "Path" and "PATH". os/exec would silently resolve that collision for
	// us; resolving it here instead means the rule is ours, is deterministic, and is
	// pinned by a test rather than inherited from an implementation detail. Sorted
	// iteration makes "last wins" mean "the name that sorts last", every time.
	names := make([]string, 0, len(source))
	for name := range source {
		names = append(names, name)
	}
	sort.Strings(names)

	folded := make(map[string]string, len(source))
	kept := make(map[string]string, len(source))
	for _, name := range names {
		if denied(name) {
			continue
		}
		key := strings.ToUpper(name)
		folded[key] = name
		kept[key] = source[name]
	}

	// Preferences, and only where the user expressed none.
	for name, value := range overlay.Session {
		key := strings.ToUpper(name)
		if _, already := kept[key]; already {
			continue
		}
		folded[key] = name
		kept[key] = value
	}
	// Requirements, whatever the environment says.
	for name, value := range overlay.Enforced {
		key := strings.ToUpper(name)
		folded[key] = name
		kept[key] = value
	}

	// The session values. The three blanks are the second half of ENV02: a name that is
	// merely absent lets the client fall back to a stored credential, so each one is set
	// to the empty string to settle it rather than left to a fallback we do not control.
	for key, value := range map[string]string{
		"ANTHROPIC_BASE_URL":       overlay.BaseURL,
		"ANTHROPIC_AUTH_TOKEN":     overlay.AuthToken,
		"ANTHROPIC_API_KEY":        "",
		"ANTHROPIC_CUSTOM_HEADERS": "",
		"CLAUDE_CODE_OAUTH_TOKEN":  "",
	} {
		folded[key] = key
		kept[key] = value
	}

	out := make([]string, 0, len(kept))
	for key, value := range kept {
		out = append(out, folded[key]+"="+value)
	}
	sort.Strings(out)
	return out
}
