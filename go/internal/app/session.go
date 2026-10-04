package app

import (
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// The child is told about this session through its environment and through nothing else.
//
// The Node baseline gets the same result by passing --model, --effort and a --settings JSON
// blob on the command line. Measured 2026-09-16 against the installed client, every value
// that matters here is reachable by environment instead, and --model on the command line
// still overrides ANTHROPIC_MODEL. That difference is the whole reason for choosing this
// route: injecting --model would put a second one on a command line that may already carry
// the user's, and telling those apart means knowing which native options consume a
// following value -- the tracking the baseline's own blocklist needed and still got wrong
// on `--name --model`. Nothing here parses an argument, so nothing here can mistake a value
// for one.

// startupModel is what a session runs on when the user names nothing.
//
// The baseline's DEFAULT_SELECTION, and deliberately not an alias of the catalogue defaults:
// it is the main startup value and has to stay independently changeable.
var startupModel = bridge.DefaultStartup()

// effortEnv is the name that pins the effort for a whole session.
//
// Named rather than inlined because what this build does with it is a decision: it is set by
// a user who wants the pin and never by this build. See launch.Overlay.Effort.
const effortEnv = "CLAUDE_CODE_EFFORT_LEVEL"

// sessionEnvironment is what this build tells the native child about the session.
//
// Derived from the supported catalogue and this session's selection, so native
// tier defaults agree with the gateway's configured mapping.
func (config ClauductSettings) sessionEnvironment() map[string]string {
	session := map[string]string{
		"CLAUDE_CODE_RETRY_WATCHDOG": "0",
		// Anthropic's reporting has nowhere to go from here.
		"DISABLE_TELEMETRY":                   "1",
		"DISABLE_ERROR_REPORTING":             "1",
		"CLAUDE_CODE_RESUME_INTERRUPTED_TURN": "0",

		// The startup model. Measured: ANTHROPIC_MODEL decides which model the client asks
		// for and a --model on the command line beats it, so the user keeps the choice this
		// makes a default of.
		//
		// The effort is deliberately not here. It travels as --effort instead, because that
		// name does not behave the way this one does -- see launch.Overlay.Effort.
		"ANTHROPIC_MODEL": config.Startup.Model,

		// The picker entry for the startup route, and the discovery that fills the rest of
		// the list from GET /v1/models. Without discovery the user's /model list is the
		// client's built-in Anthropic one: names that do not exist on this backend.
		"ANTHROPIC_CUSTOM_MODEL_OPTION":              config.Startup.Model,
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":         config.Startup.Model + " via Clauduct",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION":  "Codex via Clauduct",
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1",
		"CLAUDE_CODE_GATEWAY_HINT_HEADERS":           "1",

		// The advisor runs on Anthropic's servers and cannot execute against this backend.
		"CLAUDE_CODE_DISABLE_ADVISOR_TOOL": "1",
		// So does auto mode's server-side classifier. 2.1.283 made auto mode the default and
		// asks for that classifier on every main request (the `safeguards` field); the
		// gateway refuses the field and native re-sends without it, one refused request per
		// turn. This is native's documented switch for a gateway that cannot provide the
		// check. Native then asks its own classifier, which the backend answers (#149).
		"CLAUDE_CODE_AUTO_MODE_SERVER": "0",
		// Deferred schemas are discovered through the client's native ToolSearch.
		"ENABLE_TOOL_SEARCH": "true",

		// The same launch-wide window serves every model. Native retains its
		// own output reserve and may compact before the gateway's estimate target.
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  strconv.FormatInt(config.ContextPolicy.Window, 10),
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW": strconv.FormatInt(config.ContextPolicy.Window, 10),
		"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": strconv.FormatInt(min(config.Context.Percent, 90), 10),
	}

	// The client's own tiers, pointed at the models they belong to. Measured: with these
	// set the client names the backend model outright instead of a Claude one, and the
	// background tier stops running on whatever the conversation is using.
	for alias, name := range map[string]string{
		"fable":  "ANTHROPIC_DEFAULT_FABLE_MODEL",
		"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
		"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
	} {
		if model, known := config.Selection.ForAlias(alias); known {
			session[name] = model.ID
		}
	}
	return session
}

// routingSettingsEnv goes in the child's --settings env, which native ranks above user,
// project and local settings files and keeps above them when one changes mid-session
// (measured 2.1.289, #295). A settings file env otherwise replaces the process environment:
// measured, it moved the endpoint, selected a cloud provider, and dropped the loopback hosts
// from NO_PROXY so the gateway request went through a proxy. ANTHROPIC_UNIX_SOCKET would send
// every API request to a socket. Empty clears a switch. The endpoint is not a secret; the
// token is, and never goes on a command line. NO_PROXY is the launch shell's plus loopback.
func routingSettingsEnv(env map[string]string, base string) map[string]string {
	noProxy := ""
	for name, value := range env {
		if strings.EqualFold(name, "NO_PROXY") {
			noProxy = value
		}
	}
	out := map[string]string{
		"ANTHROPIC_BASE_URL":    base,
		"ANTHROPIC_UNIX_SOCKET": "",
		launch.HostRoutedEnv:    "",
		"NO_PROXY":              launch.WithLoopbackNoProxy(noProxy),
	}
	for _, name := range launch.ProviderSwitches {
		out[name] = ""
	}
	return out
}

// sessionRequirements is what the child does not get to run without.
//
// The transport needs streaming, request classes and the resolved context
// settings. Inherited values must not silently replace this launch's policy.
func (config ClauductSettings) sessionRequirements() map[string]string {
	values := map[string]string{"CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK": "1"}
	defaults := config.sessionEnvironment()
	for _, key := range []string{"CLAUDE_CODE_GATEWAY_HINT_HEADERS", "CLAUDE_CODE_MAX_CONTEXT_TOKENS", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"} {
		values[key] = defaults[key]
	}
	return values
}
