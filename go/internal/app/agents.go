package app

import (
	"encoding/json"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// The delegation menu: one agent type per model this build can reach.
//
// Role defaults come from bridge.RoleRoute. The gateway also accepts an explicit
// model and effort on Agent calls and correlates that choice with the native child.
//
// Measured 2026-09-16, both directions:
//   - An "agents" key in the settings blob does nothing. The type is not defined and the
//     call is dropped.
//   - --agents works. The subagent ran on gpt-5.6-terra and the client named the type in
//     its own stderr as agent:custom:clauduct-terra-high.
//   - Two --agents do not merge; the last one wins, exactly like --settings.
//
// That last fact decides the ordering rather than a policy. Ours goes ahead of the forwarded
// arguments, so a user who passes their own --agents replaces this menu entirely -- which is
// the same "the user's explicit choice wins" rule the session environment follows. Unlike
// --settings there is nothing here that has to survive: losing the menu loses the menu, and
// the client's own subagents still route by role because that happens at the gateway.

// agentDefinition is one entry in the menu, in the client's own shape.
type agentDefinition struct {
	Description string `json:"description"`
	// Prompt stays empty: the menu adds no instructions of its own, only where an entry
	// runs (#144). Native accepts an empty prompt and runs the child.
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
	Model  string   `json:"model"`
}

// agentTools is what a delegated worker gets. Enough to read, change and check.
var agentTools = []string{"ToolSearch", "Read", "Grep", "Glob", "Bash", "Edit", "Write", "Agent", "TaskOutput", "SendMessage"}

// agentDefinitions builds the menu from the catalogue: one agent per model.
//
// Every description is in the Agent tool's text on every request that carries it, so the
// menu is one entry per model (decided 2026-09-24), not one per model and effort. The effort
// comes from the gateway's Agent effort argument, or the model's default. No definition sets
// effort: native would show that fixed value on the task even when the argument changed it.
func agentDefinitions() map[string]agentDefinition {
	menu := make(map[string]agentDefinition, len(bridge.Models)+1)
	for _, model := range bridge.Models {
		menu[bridge.MenuPrefix+model.Key] = agentDefinition{
			Description: "Development worker on " + model.ID + ". Runs at " + model.Effort +
				" unless the effort argument names another.",
			Tools: agentTools,
			Model: model.ID,
		}
	}
	// Use the parent route when no separate task selection was requested. The
	// gateway preserves a task-bound explicit choice automatically in descendants.
	menu[bridge.InheritRole] = agentDefinition{
		// The second sentence is kept by #144: without it a parent passed a model with this
		// entry and moved the child off the session's route (1 of 5 runs).
		Description: "General development worker that keeps the model and effort this " +
			"session is already running on. Do not pass a model argument with this agent " +
			"type unless the user requested a separate model selection.",
		Tools: agentTools,
		Model: "inherit",
	}
	return menu
}

// sessionAgents encodes the menu, or reports that there is none to send.
func sessionAgents() (string, bool) {
	encoded, err := json.Marshal(agentDefinitions())
	if err != nil {
		return "", false
	}
	return string(encoded), true
}
