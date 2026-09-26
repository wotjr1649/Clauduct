package app

import (
	"strconv"

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
var startupModel = struct{ Model, Effort string }{Model: "gpt-6-astra", Effort: "low"}

// effortEnv is the name that pins the effort for a whole session.
//
// Named rather than inlined because what this build does with it is a decision: it is set by
// a user who wants the pin and never by this build. See launch.Overlay.Effort.
const effortEnv = "CLAUDE_CODE_EFFORT_LEVEL"

// Native has one process-wide envelope. The gateway enforces each model's
// smaller policy before dispatch. Native's own reservation remains a backstop.
const (
	contextWindow = 500000
	compactAt     = 500000
	outputReserve = 0
)

// compactPercent is where compaction lands once the output reserve is taken out.
func compactPercent() float64 {
	return float64(compactAt) / float64(contextWindow-outputReserve) * 100
}

// sessionEnvironment is what this build tells the native child about the session.
//
// Derived from bridge.Models rather than written out, so the tier defaults cannot drift
// from what the gateway will actually route. Adding a backend model is still one line in
// one table.
func sessionEnvironment() map[string]string {
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
		"ANTHROPIC_MODEL": startupModel.Model,

		// The picker entry for the startup route, and the discovery that fills the rest of
		// the list from GET /v1/models. Without discovery the user's /model list is the
		// client's built-in Anthropic one: names that do not exist on this backend.
		"ANTHROPIC_CUSTOM_MODEL_OPTION":              startupModel.Model,
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":         startupModel.Model + " via Clauduct",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION":  "Codex via Clauduct",
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1",
		"CLAUDE_CODE_GATEWAY_HINT_HEADERS":           "1",

		// The advisor runs on Anthropic's servers and cannot execute against this backend.
		"CLAUDE_CODE_DISABLE_ADVISOR_TOOL": "1",
		// So does auto mode's server-side classifier. 2.1.283 made auto mode the default and
		// asks for that classifier on every main request (the `safeguards` field); the
		// gateway refuses the field and native re-sends without it, one refused request per
		// turn. This is native's documented switch for a gateway that cannot provide the
		// check. Native's own classifier requests carry stop_sequences and are refused
		// (UNSUPPORTED_SAMPLING), so an action that needs a verdict is denied as before.
		"CLAUDE_CODE_AUTO_MODE_SERVER": "0",
		// Deferred schemas are discovered through the client's native ToolSearch.
		"ENABLE_TOOL_SEARCH": "true",

		// Shared native display/envelope, not the selected model's enforced limit.
		// The gateway signals native compaction at 239K or 450K using exact input.
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  strconv.Itoa(contextWindow),
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW": strconv.Itoa(compactAt),
		"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": strconv.FormatFloat(compactPercent(), 'f', -1, 64),
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
		if model, known := bridge.ForAlias(alias); known {
			session[name] = model.ID
		}
	}
	return session
}

// sessionRequirements is what the child does not get to run without.
//
// The transport needs streaming, request classes and a native envelope large
// enough for every fixed gateway policy. An inherited 400K environment must not
// silently replace the selected 500K/450K policy or obscure model transitions.
func sessionRequirements() map[string]string {
	values := map[string]string{"CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK": "1"}
	defaults := sessionEnvironment()
	for _, key := range []string{"CLAUDE_CODE_GATEWAY_HINT_HEADERS", "CLAUDE_CODE_MAX_CONTEXT_TOKENS", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"} {
		values[key] = defaults[key]
	}
	return values
}
