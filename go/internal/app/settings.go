package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
)

// What goes in the child's --settings, and why anything does at all.
//
// Model/context defaults travel by environment. Hooks and the model picker travel through
// settings. Permissions are native's: v0.6.4 adds no ask or hard_deny rule of its own, and
// native applies its defaults, user and managed policy and in-session mode changes.
// App resolves actual option boundaries.
//
// Measured 2026-09-16: a second --settings does not merge with the first, the last one
// wins. So the moment this injects one, a --settings the user also passed would silently
// replace it and the hooks would never install. user_settings.go now combines user
// settings with these required bindings at each actual settings option's position.

// hookTimeout is how long the client waits for it. The baseline's five seconds; the hook
// itself gives up on the gateway after three.
const hookTimeout = 5

type childSettings struct {
	Hooks        map[string][]hookMatcher `json:"hooks,omitempty"`
	ModelPicker  *modelPicker             `json:"modelPicker,omitempty"`
	Env          map[string]string        `json:"env,omitempty"`
	APIKeyHelper string                   `json:"apiKeyHelper,omitempty"`
}

type hookMatcher struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type modelPicker struct {
	ReplaceBuiltInOptions bool             `json:"replaceBuiltInOptions"`
	Options               []modelPickerRow `json:"options"`
}

type modelPickerRow struct {
	Model       string `json:"model"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// sessionSettings builds the settings blob, or reports that there is nothing to send.
//
// hookPath is where the hook program is, and empty means there is none. Installing a hook
// that points at a program which is not there would make the client report a failed hook on
// every subagent it starts -- worse than not routing them, because it is noise the user
// cannot act on.
func (config ClauductSettings) sessionSettings(hookPath string) (string, bool) {
	settings := childSettings{ModelPicker: config.pickerRows()}
	if hookPath != "" {
		entry := []hookMatcher{{
			Matcher: "*",
			Hooks: []hookEntry{{
				Type: "command",
				// Quoted: the path has spaces on an ordinary Windows install.
				Command: `"` + filepath.ToSlash(hookPath) + `" ` + hookcmd.Arg,
				Timeout: hookTimeout,
			}},
		}}
		settings.Hooks = map[string][]hookMatcher{
			"SessionStart":       entry,
			"UserPromptSubmit":   entry,
			"SubagentStart":      entry,
			"SubagentStop":       entry,
			"PreCompact":         entry,
			"PreToolUse":         {{Matcher: "Workflow", Hooks: entry[0].Hooks}},
			"PostToolUse":        {{Matcher: "Workflow|EnterWorktree|ExitWorktree", Hooks: entry[0].Hooks}},
			"PostToolUseFailure": entry,
		}
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

// foregroundSettings binds a foreground session's settings to its routing (#295). Every hook
// command names this session's gateway, so the hook can compare it with where native's
// requests actually go. The settings env carries the values a settings file must not replace
// (routingSettingsEnv and the requirements): --settings outranks user, project and local
// settings (measured 2.1.289). The token never goes on a command line.
func foregroundSettings(settings, gateway string, env map[string]string) (string, error) {
	var s childSettings
	if json.Unmarshal([]byte(settings), &s) != nil {
		return "", errSettingsConflict
	}
	for _, matchers := range s.Hooks {
		for i := range matchers {
			for j := range matchers[i].Hooks {
				matchers[i].Hooks[j].Command += " " + gateway
			}
		}
	}
	s.Env = env
	encoded, err := json.Marshal(s)
	return string(encoded), err
}

// pickerRows is the /model list, named by what will actually run.
//
// Derived from the session's account list, so a model the account adds appears here
// without a Clauduct release. Hidden models stay selectable by ID but are not listed. The label is the backend identifier itself: a name that says something
// else is the problem this replaces, where picking "Opus 5" ran gpt-5.6-sol.
//
// No behavesAs field, and that is measured rather than overlooked. Setting one does silence
// the client's "model not in this version's catalogue" line, and it also takes the effort
// with it -- a session pinned to low came back at medium. The context window that line was
// warning about is settled by CLAUDE_CODE_MAX_CONTEXT_TOKENS instead, which leaves the
// effort where the user put it.
func (config ClauductSettings) pickerRows() *modelPicker {
	models := config.Selection.Catalogue().Models()
	rows := make([]modelPickerRow, 0, len(models))
	for _, model := range models {
		if !model.Visible {
			continue
		}
		description := "No default effort: choose one"
		if effort, ok := config.Selection.DefaultFor(model.ID); ok {
			description = "Default effort: " + effort
		}
		rows = append(rows, modelPickerRow{
			Model:       model.ID,
			Label:       model.ID,
			Description: description,
		})
	}
	return &modelPicker{ReplaceBuiltInOptions: true, Options: rows}
}

// executable is os.Executable. A variable so this package's tests can name a built
// clauduct.exe as the running file: the test binary would answer the hook too, but under
// -race it starts slowly enough that one per hook event pushed the package past its timeout.
var executable = os.Executable

// findHook is this program, which answers hookcmd.Arg itself (#112).
//
// Never a PATH search: the client is about to be told to run whatever this returns. Empty
// only when the platform cannot say which file is running.
func findHook() string {
	self, err := executable()
	if err != nil {
		return ""
	}
	return self
}
