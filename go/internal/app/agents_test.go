package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// B5. The menu is the catalogue, and nobody writes it twice.
func TestTheDelegationMenuIsTheCatalogue(t *testing.T) {
	menu := agentDefinitions()

	names := make([]string, 0, len(menu))
	for name := range menu {
		names = append(names, name)
	}
	sort.Strings(names)

	// All twenty verified pairs, plus the parent-inheriting role.
	want := []string{bridge.InheritRole}
	for _, model := range bridge.Models {
		for _, effort := range bridge.Efforts {
			want = append(want, "clauduct-"+model.Key+"-"+effort)
		}
	}
	sort.Strings(want)
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Fatalf("menu =\n %v\nwant\n %v", names, want)
	}

	for name, definition := range menu {
		if name == bridge.InheritRole {
			if definition.Model != "inherit" || definition.Effort != "" {
				t.Errorf("%s = %s/%s", name, definition.Model, definition.Effort)
			}
			continue
		}
		// The name has to say where it runs, because the name is all the user sees when
		// they pick one. A menu entry whose name and model disagree is worse than none.
		if !strings.HasSuffix(name, "-"+definition.Effort) {
			t.Errorf("%s runs at %s", name, definition.Effort)
		}
		var model bridge.Model
		for _, entry := range bridge.Models {
			if entry.ID == definition.Model {
				model = entry
			}
		}
		if model.ID == "" {
			t.Errorf("%s names %s, which is not in the catalogue", name, definition.Model)
			continue
		}
		if !strings.HasPrefix(name, "clauduct-"+model.Key+"-") {
			t.Errorf("%s runs on %s", name, definition.Model)
		}
	}
}

// delegation runs a session whose first turn delegates to one agent type.
func delegation(t *testing.T, agentType string, extra ...string) (models []string, stderr string) {
	t.Helper()
	return delegationAs(t, agentType, "", extra...)
}

// delegationAs is the same, with the Agent tool's own model argument set.
func delegationAs(t *testing.T, agentType, callerModel string, extra ...string) (models []string, stderr string) {
	t.Helper()
	exe := nativeAvailable(t)
	// The effort half of a menu entry is decided at the gateway, and the gateway only learns
	// which entry a request belongs to from the hook. Without it the model still moves and
	// the effort does not, which is exactly the half-working state this build must not ship.
	buildHook(t)
	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: agentStream(agentType, "say ok", callerModel)},
			{When: upstream.Conversation, SSE: textStream("sub", "ok")},
			{When: upstream.Conversation, SSE: textStream("main", "done")},
		},
		Default: textStream("side", "untitled"),
	}
	_, cwd := workspace(t)
	var stdout, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()

	if _, err := Run(ctx, Options{
		Args: append([]string{"-p", "delegate", "--allowedTools", "Agent",
			"--strict-mcp-config"}, extra...),
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &errOut,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, request := range script.Requests() {
		var envelope struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Tools []struct{} `json:"tools"`
		}
		_ = json.Unmarshal([]byte(request), &envelope)
		if len(envelope.Tools) > 0 {
			models = append(models, envelope.Model+"/"+envelope.Reasoning.Effort)
		}
	}
	t.Logf("%s: models=%v", agentType, models)
	return models, errOut.String()
}

// contains reports whether the session sent a request on this route.
func ran(models []string, route string) bool {
	for _, model := range models {
		if model == route {
			return true
		}
	}
	return false
}

// B5 end to end: picking an agent type actually moves the work to that model.
//
// The menu only exists so the user can say where a piece of work goes, so what has to be
// true is that saying it moves the work. Measured with a real client and a scripted backend,
// which costs nothing: the client names the type in its own stderr as agent:custom:<name>.
func TestDelegatingToAMenuEntryRunsThere(t *testing.T) {
	models, stderr := delegation(t, "clauduct-terra-high")
	if !ran(models, "gpt-5.6-terra/high") {
		t.Fatalf("models = %v, want one on gpt-5.6-terra/high\nstderr: %s", models, stderr)
	}
	if !strings.Contains(stderr, "agent:custom:clauduct-terra-high") {
		t.Errorf("the client did not report the type it used: %s", tail(stderr, 400))
	}
}

// And the entry that names no model keeps the parent's.
//
// "inherit" is not a model. Whether the client accepts it as one is a question about the
// client, and the answer is that it does: the child ran on the session's own route.
func TestTheInheritEntryKeepsTheParentsModel(t *testing.T) {
	models, stderr := delegation(t, bridge.InheritRole)
	if len(models) < 2 {
		t.Fatalf("models = %v; nothing was delegated\nstderr: %s", models, stderr)
	}
	for _, model := range models {
		if model != "gpt-6-astra/low" {
			t.Fatalf("models = %v; the child left the session's route", models)
		}
	}
}

// ENV06 again, now that this build injects a menu: the user's own agents still work.
//
// Measured both ways on 2026-09-16. An injected --agents *merges* with the agents defined in
// the user's config directory -- the user's reviewer still ran, on the model its own file
// names. That is what makes this injection affordable. A second --agents on the command line
// is the one case that does not merge, and there the user's replaces this one, which is the
// same rule the session environment follows.
func TestTheUsersOwnAgentsSurviveTheMenu(t *testing.T) {
	configDir := t.TempDir()
	dir := filepath.Join(configDir, "agents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"),
		[]byte("---\nname: reviewer\ndescription: The user's own reviewer.\ntools: Read\n"+
			"model: gpt-5.6-luna\n---\nReview the change.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	exe := nativeAvailable(t)
	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: agentStream("reviewer", "say ok")},
			{When: upstream.Conversation, SSE: textStream("sub", "ok")},
			{When: upstream.Conversation, SSE: textStream("main", "done")},
		},
		Default: textStream("side", "untitled"),
	}
	env := isolatedEnv(t)
	env["CLAUDE_CONFIG_DIR"] = configDir
	_, cwd := workspace(t)
	var stdout, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()
	if _, err := Run(ctx, Options{
		Args:          []string{"-p", "delegate", "--allowedTools", "Agent", "--strict-mcp-config"},
		Env:           env,
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &errOut,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "agent:custom:reviewer") {
		t.Fatalf("the user's own agent did not run: %s", tail(errOut.String(), 600))
	}
}

// The menu and the router read the same names, and nothing keeps them in step but this.
//
// One side writes clauduct-<model>-<effort>, the other reads it. They are in different
// packages because the menu is a launcher concern and the route is a protocol one, and a
// shared constant would not have caught the case that matters: a name the menu offers that
// the router resolves somewhere else.
func TestEveryMenuEntryResolvesToItself(t *testing.T) {
	for name, definition := range agentDefinitions() {
		route, known := bridge.RoleRoute(name)
		if name == bridge.InheritRole {
			if known {
				t.Errorf("%s resolved to %s/%s; it exists to resolve to nothing",
					name, route.Model, route.Effort)
			}
			continue
		}
		if !known {
			t.Errorf("the menu offers %s and the router does not know it", name)
			continue
		}
		if route.Model != definition.Model || route.Effort != definition.Effort {
			t.Errorf("%s: the menu says %s/%s, the router says %s/%s",
				name, definition.Model, definition.Effort, route.Model, route.Effort)
		}
		if route.Source != "role" {
			t.Errorf("%s: Source = %q", name, route.Source)
		}
	}
}

// At the top level, an explicit model takes precedence over the inherit role.
// A task-bound descendant is different: the gateway rejects conflicting choices.
func TestACallersOwnModelTakesTheInheritEntryOffTheParentsRoute(t *testing.T) {
	models, stderr := delegationAs(t, bridge.InheritRole, "opus")
	if len(models) < 2 {
		t.Fatalf("models = %v; nothing was delegated\nstderr: %s", models, stderr)
	}
	left := false
	for _, model := range models {
		if model != startupModel.Model+"/"+startupModel.Effort {
			left = true
		}
	}
	if !left {
		t.Fatalf("models = %v; the caller named a model and every request stayed on the "+
			"session route, so this entry is no longer the one the description warns about",
			models)
	}
}

// The user's chosen precedence: an explicit model overrides the role's model,
// while the native role's prompt and tools remain in place.
func TestAnExplicitAliasOverridesANamedRoleModel(t *testing.T) {
	models, stderr := delegationAs(t, "clauduct-luna-max", "opus")
	if !ran(models, "gpt-5.6-sol/xhigh") || ran(models, "gpt-5.6-luna/max") {
		t.Fatalf("models = %v, want explicit opus=sol and its default effort\nstderr: %s", models, tail(stderr, 400))
	}
}

// The menu asks callers to omit unrequested model overrides.
func TestTheInheritEntrySaysWhatItNeedsFromTheCaller(t *testing.T) {
	entry, defined := agentDefinitions()[bridge.InheritRole]
	if !defined {
		t.Fatalf("%s is not in the menu", bridge.InheritRole)
	}
	if !strings.Contains(entry.Description, "model") {
		t.Errorf("the description never mentions the argument it depends on: %q", entry.Description)
	}
	if !strings.Contains(strings.ToLower(entry.Description), "do not") {
		t.Errorf("the description states a behaviour but asks nothing of the caller: %q",
			entry.Description)
	}
}
