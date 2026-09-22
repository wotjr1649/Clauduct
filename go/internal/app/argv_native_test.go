package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Compare the installed parser directly and through Run, with synthetic settings
// and a local fixture backend. No model service or user profile is used.
func TestCombinedShortOptionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		end   int
		known bool
	}{
		{[]string{"-cp", "--settings={}"}, 1, true},
		{[]string{"-pv", "--settings={}"}, 1, true},
		{[]string{"-pdapi", "--settings={}"}, 1, true},
		{[]string{"-pnPUBLIC", "--settings={}"}, 1, true},
		{[]string{"-pn", "--settings={}"}, 2, true},
		{[]string{"-pn"}, 2, true},
		{[]string{"-cr", "--settings={}"}, 1, true},
		{[]string{"-cr", "PUBLIC", "--settings={}"}, 2, true},
		{[]string{"-crw", "--settings={}"}, 1, true},
		{[]string{"-cz", "--settings={}"}, 1, false},
	} {
		end, known := nativeArgEnd(tc.args, 0)
		if end != tc.end || known != tc.known {
			t.Errorf("%q: end=%d known=%v, want %d %v", tc.args, end, known, tc.end, tc.known)
		}
	}
	for _, option := range []string{"-cp", "-pv", "-pdapi", "-pnPUBLIC"} {
		if _, _, slots, err := takeUserSettings([]string{option, "--settings={}"}, t.TempDir()); err != nil || len(slots) != 1 {
			t.Errorf("settings after %s: slots=%v err=%v", option, slots, err)
		}
		defs, _, err := roleCLI([]string{option, `--agents={"public":{"model":"haiku"}}`}, "")
		if err != nil || defs["public"].Model != "haiku" {
			t.Errorf("role after %s: %v", option, err)
		}
	}
}

func TestNativePublicOptionAritiesMatchScanner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, nativeAvailable(t), "--help")
	cmd.Dir = t.TempDir()
	help, err := cmd.Output()
	if err != nil {
		t.Fatal("native help unavailable", err)
	}
	option := regexp.MustCompile(`(?m)^  (?:-[a-zA-Z], )?(--[a-zA-Z0-9-]+)(?:, --[a-zA-Z0-9-]+)?(?: +(<[^>]+>|\[[^\]]+\]))?`)
	checked := 0
	for _, match := range option.FindAllStringSubmatch(string(help), -1) {
		name := match[1]
		if name == "--dangerously-skip-permissions" || name == "--allow-dangerously-skip-permissions" {
			continue // These never reach the argument scanner in the product.
		}
		want := 1
		if match[2] != "" {
			want = 2
		}
		end, known := nativeArgEnd([]string{name, "PUBLIC_VALUE"}, 0)
		if !known || end != want {
			t.Errorf("installed native contract drift: %s end=%d known=%v want=%d", name, end, known, want)
		}
		checked++
	}
	if checked < 40 {
		t.Fatal("native help option inventory incomplete")
	}
	t.Logf("installed native public option arities checked=%d", checked)
}

func TestSubcommandArgumentsRetainUnknownBoundaries(t *testing.T) {
	args := []string{"mcp", "add", "-t", "http", "public", "http://127.0.0.1"}
	forward, _, slots, err := takeUserSettings(args, t.TempDir())
	if err != nil || len(slots) != 0 || strings.Join(forward, "\n") != strings.Join(args, "\n") {
		t.Fatal("native subcommand arguments changed")
	}
	if _, _, _, err := takeUserSettings(append(args, "--settings={}"), t.TempDir()); err == nil {
		t.Fatal("subcommand option invented a settings boundary")
	}
}

func TestNativeSettingsArgumentBoundaries(t *testing.T) {
	native := nativeAvailable(t)
	for _, test := range []struct {
		name                 string
		args                 []string
		prompt, appendPrompt string
		invalid              bool
	}{
		{"print_is_boolean", []string{"-p", "--settings"}, "", "", true},
		{"terminator", []string{"-p", "--", "--settings"}, "--settings", "", false},
		{"required_value", []string{"--append-system-prompt", "--settings", "-p", "PUBLIC_ARGV_PROMPT"}, "PUBLIC_ARGV_PROMPT", "--settings", false},
		{"terminator_as_value", []string{"--append-system-prompt", "--", "--settings={}", "-p", "PUBLIC_ARGV_PROMPT"}, "PUBLIC_ARGV_PROMPT", "--", false},
		{"option_shaped_file", []string{"--settings", "--public.json", "-p", "PUBLIC_ARGV_PROMPT"}, "PUBLIC_ARGV_PROMPT", "", false},
		{"optional_barrier", []string{"--debug", "--settings={}", "PUBLIC_ARGV_PROMPT", "-p"}, "PUBLIC_ARGV_PROMPT", "", false},
		{"variadic_barrier", []string{"--tools", "Read", "--settings={}", "PUBLIC_ARGV_PROMPT", "-p"}, "PUBLIC_ARGV_PROMPT", "", false},
		{"combined_name", []string{"-pnPUBLIC", "--settings={}", "PUBLIC_ARGV_PROMPT"}, "PUBLIC_ARGV_PROMPT", "", false},
		{"combined_debug", []string{"-pdapi", "--settings={}", "PUBLIC_ARGV_PROMPT"}, "PUBLIC_ARGV_PROMPT", "", false},
	} {
		for _, wrapped := range []bool{false, true} {
			mode := "native"
			if wrapped {
				mode = "clauduct"
			}
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				config := t.TempDir()
				_, cwd := workspace(t)
				if err := os.WriteFile(filepath.Join(cwd, "--public.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
				env := map[string]string{}
				for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATH", "PATHEXT"} {
					env[key] = os.Getenv(key)
				}
				for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "CLAUDE_CONFIG_DIR"} {
					env[key] = config
				}
				for _, key := range []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "DISABLE_AUTOUPDATER", "DISABLE_TELEMETRY", "DISABLE_ERROR_REPORTING"} {
					env[key] = "1"
				}
				args := append([]string{"--strict-mcp-config", "--bare", "--model", "gpt-5.6-luna", "--effort", "low", "--system-prompt", "PUBLIC_ARGV_BASE"}, test.args...)
				script := &upstream.Script{Default: measuredStream("PUBLIC_ARGV_OK")}
				ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
				defer cancel()
				var output bytes.Buffer
				var runErr error
				if wrapped {
					result, err := Run(ctx, Options{Args: args, Env: env, Cwd: cwd, Stdin: strings.NewReader(""), Stdout: &output, Stderr: &output,
						ResolveClaude: func() (string, bool, error) { return native, true, nil },
						StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
					})
					runErr = err
					if result.NativeExitCode != 0 && runErr == nil {
						runErr = errors.New("native exited nonzero")
					}
				} else {
					g, err := gateway.Start(script)
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						drain, stop := context.WithTimeout(context.Background(), time.Second)
						defer stop()
						if err := g.Close(drain); err != nil {
							t.Error(err)
						}
					}()
					env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_AUTH_TOKEN"] = g.BaseURL(), g.Token()
					cmd := exec.CommandContext(ctx, native, args...)
					cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = cwd, strings.NewReader(""), &output, &output
					for key, value := range env {
						cmd.Env = append(cmd.Env, key+"="+value)
					}
					cmd.WaitDelay = time.Second
					runErr = cmd.Run()
				}
				requests := script.Requests()
				if test.invalid {
					if runErr == nil || len(requests) != 0 {
						t.Fatal("missing required value accepted or reached backend")
					}
					return
				}
				if runErr != nil {
					t.Fatalf("argv failed: %v; %s", runErr, tail(output.String(), 800))
				}
				found := false
				for _, raw := range requests {
					var request struct {
						Input []struct {
							Role    string
							Content json.RawMessage
						}
					}
					if json.Unmarshal([]byte(raw), &request) != nil {
						t.Fatal("invalid fixture request")
					}
					prompt, _ := json.Marshal(test.prompt)
					appendFound := test.appendPrompt == ""
					for _, input := range request.Input {
						if input.Role == "user" && strings.Contains(string(input.Content), `"text":`+string(prompt)) {
							found = true
						}
						var system string
						if input.Role == "developer" && json.Unmarshal(input.Content, &system) == nil && strings.HasSuffix(strings.TrimSpace(system), test.appendPrompt) {
							appendFound = true
						}
					}
					if !appendFound {
						t.Fatal("system prompt value changed")
					}
				}
				if !found {
					t.Fatal("literal prompt did not reach backend")
				}
			})
		}
	}
}
