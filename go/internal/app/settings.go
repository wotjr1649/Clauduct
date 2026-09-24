package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// What goes in the child's --settings, and why anything does at all.
//
// The session's preferences travel by environment, which is what keeps this launcher from
// having to parse an argument. Two things cannot: a hook is a settings entry and has no
// environment form, and so is the model picker. Those go here.
//
// Measured 2026-09-16: a second --settings does not merge with the first, the last one
// wins. So the moment this injects one, a --settings the user also passed would silently
// replace it and the hooks would never install. user_settings.go now combines user
// settings with these required bindings at each actual settings option's position.

// hookTimeout is how long the client waits for it. The baseline's five seconds; the hook
// itself gives up on the gateway after three.
const hookTimeout = 5

type childSettings struct {
	Hooks       map[string][]hookMatcher `json:"hooks,omitempty"`
	ModelPicker *modelPicker             `json:"modelPicker,omitempty"`
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
func sessionSettings(hookPath string) (string, bool) {
	settings := childSettings{ModelPicker: pickerRows()}
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
			"PostToolUse":        {{Matcher: "Workflow", Hooks: entry[0].Hooks}},
			"PostToolUseFailure": entry,
		}
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

// pickerRows is the /model list, named by what will actually run.
//
// Derived from bridge.Models, so a backend model added there appears here without anyone
// editing this. The label is the backend identifier itself: a name that says something
// else is the problem this replaces, where picking "Opus 5" ran gpt-5.6-sol.
//
// No behavesAs field, and that is measured rather than overlooked. Setting one does silence
// the client's "model not in this version's catalogue" line, and it also takes the effort
// with it -- a session pinned to low came back at medium. The context window that line was
// warning about is settled by CLAUDE_CODE_MAX_CONTEXT_TOKENS instead, which leaves the
// effort where the user put it.
func pickerRows() *modelPicker {
	routes := bridge.Models
	rows := make([]modelPickerRow, 0, len(routes))
	for _, model := range routes {
		rows = append(rows, modelPickerRow{
			Model:       model.ID,
			Label:       model.ID,
			Description: "Default effort: " + model.Effort,
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
