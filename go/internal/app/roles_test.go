package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// A synthetic count/usage oracle around the existing script; real native,
// protocol conversion, selection correlation and policy guard remain active.
type exactScript struct{ *upstream.Script }

func TestNativeCustomForkRetainsItsDefinitionAndExplicitChoice(t *testing.T) {
	buildHook(t)
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "definition", true: "explicit"}[explicit], func(t *testing.T) {
			config := t.TempDir()
			_, cwd := workspace(t)
			dir := filepath.Join(cwd, ".claude", "agents")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "Fork.md"), []byte("---\nname: Fork\ndescription: public proof\ntools: Read\nmodel: gpt-5.6-sol\neffort: medium\n---\nPUBLIC_CUSTOM_FORK_PROOF"), 0600); err != nil {
				t.Fatal(err)
			}
			args, model, effort := `{"subagent_type":"Fork","description":"public proof","prompt":"say ok"}`, "gpt-5.6-sol", "medium"
			if explicit {
				args, model, effort = `{"subagent_type":"Fork","model":"gpt-5.6-terra","effort":"high","description":"public proof","prompt":"say ok"}`, "gpt-5.6-terra", "high"
			}
			script := newScript(toolStream("custom_fork", "Agent", args), textStream("child", "ok"), textStream("parent", "done"))
			out := (nativeRun{Cwd: cwd, ConfigDir: config, Args: []string{"-p", "delegate public proof", "--allowedTools", "Agent"}, Env: map[string]string{"CLAUDE_CODE_FORK_SUBAGENT": "1"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
			found := false
			for _, raw := range script.Requests() {
				if !strings.Contains(raw, "PUBLIC_CUSTOM_FORK_PROOF") {
					continue
				}
				var request struct {
					Model     string
					Reasoning struct{ Effort string }
				}
				if json.Unmarshal([]byte(raw), &request) != nil || request.Model != model || request.Reasoning.Effort != effort {
					t.Fatal("custom Fork lost its selected route")
				}
				found = true
			}
			if out.err != nil || out.result.NativeExitCode != 0 || !found || out.result.Diagnostics.AgentResults.Totals["parent_received"] != 1 {
				t.Fatalf("custom Fork: err=%v exit=%d found=%v received=%d", out.err, out.result.NativeExitCode, found, out.result.Diagnostics.AgentResults.Totals["parent_received"])
			}
		})
	}
}

func TestNativeUnlistedRoleKeepsNativeModelAndCompletion(t *testing.T) {
	buildHook(t)
	script := newScript(toolStream("native_choice", "Agent", `{"subagent_type":"statusline-setup","description":"public proof","prompt":"say ok"}`), textStream("child", "ok"), textStream("parent", "done"))
	out := (nativeRun{Args: []string{"-p", "delegate public proof", "--allowedTools", "Agent"}, transport: exactScript{script}, ContextPolicy: true}).run(t)
	found := false
	for _, record := range out.result.Diagnostics.Recent {
		if record.Source == "native-selection" && record.Model == "gpt-5.6-terra" && record.AgentRole == "statusline-setup" && record.Status == 200 {
			found = true
			t.Logf("native %s selected %s/%s", out.result.Diagnostics.Client.Version, record.Model, record.Effort)
		}
	}
	if out.err != nil || out.result.NativeExitCode != 0 || !found || out.result.Diagnostics.AgentResults.Totals["parent_received"] != 1 {
		t.Fatalf("native choice: err=%v exit=%d found=%v received=%d selections=%+v", out.err, out.result.NativeExitCode, found, out.result.Diagnostics.AgentResults.Totals["parent_received"], out.result.Diagnostics.AgentSelections)
	}
}

func (f exactScript) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f exactScript) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	response, err := f.Script.Execute(ctx, call)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(strings.NewReader(countedFixtureReply(string(body), call, 1000)))
	return response, nil
}

func TestRoleDefaultsCLIHonoursValueBoundaries(t *testing.T) {
	const injected = `{"worker":{"model":"sol","effort":"high"}}`
	const explicit = `{"worker":{"model":"terra","effort":"medium"}}`
	for _, args := range [][]string{
		{"-p", "prompt", "--agents", explicit},
		{"--append-system-prompt", "--agents", "--agents=" + explicit},
		{"--worktree", "--agents", explicit},
		{"-n", "--agents", "--agents=" + explicit},
		{"--brief", "--agents=" + explicit},
	} {
		defs, _, err := roleCLI(args, injected)
		if err != nil || defs["worker"].Model != "terra" {
			t.Fatal("explicit CLI default lost")
		}
	}
	defs, _, err := roleCLI([]string{"--append-system-prompt", "--agents", explicit}, injected)
	if err != nil || defs["worker"].Model != "sol" {
		t.Fatal("prompt value interpreted as flag")
	}
	defs, plugins, err := roleCLI([]string{"--", "--agents", explicit, "--plugin-dir", "public-plugin"}, injected)
	if err != nil || defs["worker"].Model != "sol" || len(plugins) != 0 {
		t.Fatal("positional data interpreted as routing policy")
	}
	if _, _, err = roleCLI([]string{"--unknown-future-flag", "--agents", explicit}, injected); err == nil {
		t.Fatal("ambiguous flags accepted")
	}
}

func TestRoleDefaultsYAMLAndPrecedence(t *testing.T) {
	base := t.TempDir()
	high, low := filepath.Join(base, "high"), filepath.Join(base, "low")
	for _, dir := range []string{high, low} {
		if os.MkdirAll(dir, 0700) != nil {
			t.Fatal("mkdir")
		}
	}
	if os.WriteFile(filepath.Join(high, "role.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: 'sol'\neffort: medium\n---\nPrivate prompt is never retained."), 0600) != nil {
		t.Fatal("write")
	}
	if os.WriteFile(filepath.Join(low, "role.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: luna\neffort: max\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{cli: map[string]roleDefault{"reviewer": {Model: "terra", Effort: "high"}}, directories: []roleDirectory{{path: high}, {path: low}}, managed: 1}
	parent := bridge.Route{Model: "gpt-6-astra", Effort: "low"}
	route, found, err := s.resolve("reviewer", parent)
	if err != nil || !found || route.Model != "gpt-5.6-sol" || route.Effort != "medium" {
		t.Fatal("managed precedence")
	}
	s.managed = 0
	route, _, err = s.resolve("reviewer", parent)
	if err != nil || route.Model != "gpt-5.6-terra" {
		t.Fatal("CLI precedence")
	}
	delete(s.cli, "reviewer")
	route, _, err = s.resolve("reviewer", parent)
	if err != nil || route.Model != "gpt-5.6-sol" {
		t.Fatal("project/user precedence")
	}
	for _, raw := range []string{
		"---\nname: reviewer\ndescription: proof\nmodel: [sol]\n---\n",
		"---\nname: reviewer\ndescription: proof\nmodel: &cycle [*cycle]\n---\n",
		"---\nname: reviewer\ndescription: proof\nmodel: sol\nmodel: luna\n---\n",
	} {
		if _, _, err := parseRoleFile([]byte(raw)); err == nil {
			t.Fatal("malformed/ambiguous default accepted")
		}
	}
}

func TestNativeCustomRoleDefaultsWithProductionGuard(t *testing.T) {
	buildHook(t)
	for _, source := range []string{"cli", "user", "project", "plugin-cli", "plugin-installed"} {
		t.Run(source, func(t *testing.T) {
			config := t.TempDir()
			_, cwd := workspace(t)
			args := []string{"-p", "delegate public proof", "--effort", "low", "--allowedTools", "Agent"}
			role := "reviewer"
			if source == "cli" {
				args = append(args, "--agents", `{"reviewer":{"description":"public proof","prompt":"PUBLIC_ROLE_DEFAULT_PROOF","tools":["Read"],"model":"gpt-5.6-sol","effort":"medium"}}`)
			} else {
				dir := filepath.Join(config, "agents")
				if source == "project" {
					dir = filepath.Join(cwd, ".claude", "agents")
				}
				if strings.HasPrefix(source, "plugin-") {
					plugin := filepath.Join(config, "plugins", "public-role")
					if source == "plugin-installed" {
						plugin = filepath.Join(config, "plugins", "marketplaces", "proof", "public-role")
					}
					if os.MkdirAll(filepath.Join(plugin, ".claude-plugin"), 0700) != nil || os.WriteFile(filepath.Join(plugin, ".claude-plugin", "plugin.json"), []byte(`{"name":"public-role","version":"1.0.0"}`), 0600) != nil {
						t.Fatal("plugin fixture")
					}
					dir = filepath.Join(plugin, "agents")
					role = "public-role:reviewer"
					if source == "plugin-cli" {
						args = append(args, "--plugin-dir", plugin)
					} else {
						market := filepath.Dir(plugin)
						if os.MkdirAll(filepath.Join(market, ".claude-plugin"), 0700) != nil || os.WriteFile(filepath.Join(market, ".claude-plugin", "marketplace.json"), []byte(`{"name":"proof","owner":{"name":"Public fixture"},"plugins":[{"name":"public-role","source":"./public-role"}]}`), 0600) != nil {
							t.Fatal("marketplace fixture")
						}
						known, _ := json.Marshal(map[string]any{"proof": map[string]any{"source": map[string]string{"source": "directory", "path": market}, "installLocation": market, "lastUpdated": "2026-09-18T00:00:00Z", "autoUpdate": false}})
						if os.WriteFile(filepath.Join(config, "plugins", "known_marketplaces.json"), known, 0600) != nil {
							t.Fatal("known marketplace fixture")
						}
						installed, _ := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{"public-role@proof": []any{map[string]string{"scope": "user", "installPath": plugin, "version": "1.0.0", "installedAt": "2026-09-18T00:00:00Z", "lastUpdated": "2026-09-18T00:00:00Z"}}}})
						if os.WriteFile(filepath.Join(config, "plugins", "installed_plugins.json"), installed, 0600) != nil || os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"enabledPlugins":{"public-role@proof":true}}`), 0600) != nil {
							t.Fatal("installed fixture")
						}
					}
				}
				if os.MkdirAll(dir, 0700) != nil || os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: public proof\ntools: Read\nmodel: gpt-5.6-sol\neffort: medium\n---\nPUBLIC_ROLE_DEFAULT_PROOF"), 0600) != nil {
					t.Fatal("role fixture")
				}
			}
			script := &upstream.Script{Turns: []upstream.ScriptTurn{
				{When: upstream.Conversation, SSE: toolStream("delegate", "Agent", `{"subagent_type":"`+role+`","description":"proof","prompt":"say ok"}`)},
				{When: upstream.Conversation, SSE: textStream("child", "ok")},
				{When: upstream.Conversation, SSE: textStream("parent", "done")},
			}, Default: textStream("side", "title")}
			out := (nativeRun{Cwd: cwd, ConfigDir: config, Args: args, transport: exactScript{script}, ContextPolicy: true}).run(t)
			if out.err != nil || out.result.NativeExitCode != 0 {
				t.Fatalf("exit=%d error=%v output=%s", out.result.NativeExitCode, out.err, tail(out.output(), 1000))
			}
			found := false
			for _, raw := range script.Requests() {
				if !strings.Contains(raw, "PUBLIC_ROLE_DEFAULT_PROOF") {
					continue
				}
				var req struct {
					Model     string
					Reasoning struct{ Effort string }
					Tools     []struct{ Name string }
				}
				_ = json.Unmarshal([]byte(raw), &req)
				if req.Model != "gpt-5.6-sol" || req.Reasoning.Effort != "medium" || len(req.Tools) != 1 || req.Tools[0].Name != "Read" {
					t.Fatal("role route or restrictions changed")
				}
				found = true
			}
			if !found {
				t.Fatal("custom role did not execute")
			}
			if out.result.Diagnostics.AgentResults.Totals["parent_received"] != 1 {
				t.Fatalf("result not received by parent: %+v", out.result.Diagnostics.AgentResults)
			}
			t.Logf("source=%s verified sol/medium, Read-only tools preserved", source)
		})
	}
}
