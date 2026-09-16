package app

import "github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"

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

		// The startup route. Measured: ANTHROPIC_MODEL decides which model the client asks
		// for, CLAUDE_CODE_EFFORT_LEVEL decides the effort it sends, and a --model on the
		// command line beats both -- so the user keeps the choice this makes a default of.
		"ANTHROPIC_MODEL":          startupModel.Model,
		"CLAUDE_CODE_EFFORT_LEVEL": startupModel.Effort,

		// The picker entry for the startup route, and the discovery that fills the rest of
		// the list from GET /v1/models. Without discovery the user's /model list is the
		// client's built-in Anthropic one: names that do not exist on this backend.
		"ANTHROPIC_CUSTOM_MODEL_OPTION":              startupModel.Model,
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":         startupModel.Model + " via Clauduct",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION":  "Codex via Clauduct",
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1",

		// The advisor runs on Anthropic's servers and cannot execute against this backend.
		"CLAUDE_CODE_DISABLE_ADVISOR_TOOL": "1",
	}

	// The client's own tiers, pointed at the models they belong to. Measured: with these
	// set the client names the backend model outright instead of a Claude one, and the
	// background tier stops running on whatever the conversation is using.
	for alias, name := range map[string]string{
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
// One entry, and it earns the distinction: this build refuses a non-streaming request, so
// a client left free to fall back to one produces a broken turn rather than a slower
// answer. Everything else in the session is a preference the user may hold differently.
func sessionRequirements() map[string]string {
	return map[string]string{"CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK": "1"}
}
