package app

import (
	"encoding/json"
	"os"
	"path/filepath"

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
// replace it and the hooks would never install. That is what makes refusing those options
// a consequence of injecting rather than a policy anyone chose.

// hookBinary is the name of the program the client runs on a subagent event.
const hookBinary = "clauduct-hook"

// hookTimeout is how long the client waits for it. The baseline's five seconds; the hook
// itself gives up on the gateway after three.
const hookTimeout = 5

// hookSuffix is what an executable is called here. Set by the platform file.

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
				Command: `"` + filepath.ToSlash(hookPath) + `"`,
				Timeout: hookTimeout,
			}},
		}}
		settings.Hooks = map[string][]hookMatcher{
			"SubagentStart": entry,
			"SubagentStop":  entry,
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

// findHook looks for the hook program beside this one.
//
// Beside, and nowhere else. A PATH search could find a program of the same name that this
// build did not ship, and the client is about to be told to run whatever this returns.
func findHook() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	path := filepath.Join(filepath.Dir(self), hookBinary+hookSuffix)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}
