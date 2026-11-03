package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Inspect names only: file env stays native-owned, including its folder trust boundary.
func nativeSettingsEnvNames(o Options) ([]string, error) {
	cli := cliRoles{}
	if err := cli.scope(o.Args, o.Cwd); err != nil {
		return nil, err
	}
	var names []string
	var scanErr error
	for _, file := range []struct{ source, path string }{
		{"user", filepath.Join(nativeConfigDirectory(o.Env, o.Cwd), "settings.json")},
		{"project", filepath.Join(o.Cwd, ".claude", "settings.json")},
		{"local", filepath.Join(o.Cwd, ".claude", "settings.local.json")},
		{"managed", filepath.Join(managedRoot(), "managed-settings.json")},
	} {
		if file.source != "managed" && !cli.source(file.source) {
			continue
		}
		raw, err := boundedRoleFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		var settings struct {
			Env map[string]json.RawMessage `json:"env"`
		}
		if err != nil || json.Unmarshal(raw, &settings) != nil {
			scanErr = errRoleDefaults
			continue
		}
		for name := range settings.Env {
			names = append(names, name)
		}
	}
	return names, scanErr
}

func (config ClauductSettings) nativeSettingsEnv(o Options, base string) (map[string]string, error) {
	env := config.sessionEnvironment()
	var names []string
	if o.agentView() {
		var err error
		names, err = nativeSettingsEnvNames(o)
		// A partial scan keeps defaults; effectiveStartup still owns startup validation.
		if err == nil {
			for _, name := range names {
				delete(env, strings.ToUpper(name))
			}
		}
	}
	// Match launch's sorted, case-insensitive last-wins shell lookup.
	spellings := map[string]string{}
	for name := range o.Env {
		key := strings.ToUpper(name)
		if name > spellings[key] {
			spellings[key] = name
		}
	}
	for key, name := range spellings {
		if strings.HasPrefix(key, "ANTHROPIC_") {
			continue // Shell Anthropic values are dropped, including from settings.
		}
		if _, known := env[key]; !known {
			continue
		}
		value := o.Env[name]
		if !backgroundScalar.MatchString(value) {
			if BackgroundRequested(o.Args) {
				return nil, errSettingsConflict
			}
			delete(env, key) // Native keeps the user's own OS value in foreground/print.
			continue
		}
		env[key] = value
	}
	config.pinNativeSettingsEnv(env, names, o.Env, base)
	return env, nil
}

// Reapply after the user merge too: it can remove a file's pinned spelling.
func (config ClauductSettings) pinNativeSettingsEnv(env map[string]string, names []string, inherited map[string]string, base string) {
	pinned := config.sessionRequirements()
	for key, value := range routingSettingsEnv(inherited, base) {
		pinned[key] = value
	}
	if value, set := env["NO_PROXY"]; set {
		pinned["NO_PROXY"] = launch.WithLoopbackNoProxy(value)
	}
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_OAUTH_TOKEN"} {
		pinned[key] = ""
	}
	pinned["CLAUDE_CODE_ENABLE_FUNCTION_HOOKS"] = "1"
	for name := range env {
		names = append(names, name)
	}
	for key, value := range pinned {
		env[key] = value
	}
	// Native merges JSON keys case-sensitively before applying Windows env names.
	for _, name := range names {
		if value, required := pinned[strings.ToUpper(name)]; required {
			env[name] = value
		}
	}
}

// Strategy-free sessions carry these values in --settings env, keeping native's supervisor
// OS snapshot free of Clauduct values (AV2/M1). An explicit --model still wins.

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
		// Anthropic's reporting has nowhere to go from here.
		"DISABLE_TELEMETRY":       "1",
		"DISABLE_ERROR_REPORTING": "1",

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
	// Model discovery is also the session's routing proof (#299): its start-up request is how
	// the first hook knows native talks to this gateway, so a settings file must not turn it off.
	for _, key := range []string{"CLAUDE_CODE_GATEWAY_HINT_HEADERS", "CLAUDE_CODE_MAX_CONTEXT_TOKENS", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"} {
		values[key] = defaults[key]
	}
	// Clauduct does not sign in to or out of Claude or manage its plan (#319 decision 17,
	// routing r4 LN): native's documented switches hide those commands. /passes,
	// /privacy-settings and /design-login have no documented switch and stay visible.
	for _, key := range []string{"DISABLE_LOGIN_COMMAND", "DISABLE_LOGOUT_COMMAND", "DISABLE_UPGRADE_COMMAND", "DISABLE_EXTRA_USAGE_COMMAND", "DISABLE_INSTALL_GITHUB_APP_COMMAND"} {
		values[key] = "1"
	}
	if config.BoundaryProfile == boundaryStrict {
		values[nonessentialTraffic] = "1" // #301: a settings file must not turn it back on
		if config.marketplace != "" {
			values[marketplaceInstall] = config.marketplace // the user's value, kept (strictReady)
		}
	}
	return values
}
