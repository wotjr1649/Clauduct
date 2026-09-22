package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestUserSettingsKeepUserHooksAndRequiredBindings(t *testing.T) {
	args := []string{"-p", "mention --settings safely", "--settings", `{"theme":"dark","integer":9007199254740993,"hooks":{"SubagentStart":[{"matcher":"reviewer","hooks":[{"type":"command","command":"public-user-hook"}]}]}}`, "--setting-sources", "user,project"}
	forward, user, slots, err := takeUserSettings(args, t.TempDir())
	if err != nil || !reflect.DeepEqual(slots, []int{2}) || !reflect.DeepEqual(forward, []string{"-p", "mention --settings safely", "--settings=" + args[3], "--setting-sources", "user,project"}) {
		t.Fatal("settings extraction changed unrelated arguments", err)
	}
	required, _ := sessionSettings("C:/public/clauduct-hook.exe")
	merged, err := mergeUserSettings(required, user)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Theme       string
		Integer     json.Number
		Hooks       map[string][]hookMatcher
		ModelPicker *modelPicker
	}
	if json.Unmarshal([]byte(merged), &got) != nil || got.Theme != "dark" || got.Integer != "9007199254740993" || got.ModelPicker == nil || len(got.Hooks["SubagentStart"]) != 2 {
		t.Fatal("user data or session settings lost")
	}
	if got.Hooks["SubagentStart"][0].Hooks[0].Command != "public-user-hook" || !strings.Contains(got.Hooks["SubagentStart"][1].Hooks[0].Command, "clauduct-hook.exe") {
		t.Fatal("hook order or identity changed")
	}
}

func TestUserSettingsFileAndLastOccurrence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "public.json"), []byte(`{"theme":"light"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, user, _, err := takeUserSettings([]string{"--settings=missing.json", "--settings", "public.json"}, dir)
	if err != nil || string(user["theme"]) != `"light"` {
		t.Fatal("last settings source not used", err)
	}
}

func TestUserSettingsArgumentBoundaries(t *testing.T) {
	const settings = `{"theme":"dark"}`
	const s = "--settings=" + settings
	for _, test := range []struct {
		name              string
		args, forward     []string
		settings, invalid bool
	}{
		{"terminator", []string{"-p", "--", "--settings", settings}, []string{"-p", "--", "--settings", settings}, false, false},
		{"required_value", []string{"--append-system-prompt", "--settings", "-p", "proof"}, []string{"--append-system-prompt", "--settings", "-p", "proof"}, false, false},
		{"value_then_option", []string{"--append-system-prompt", "--settings", "-p", "proof", s}, []string{"--append-system-prompt", "--settings", "-p", "proof", s}, true, false},
		{"terminator_as_value", []string{"--append-system-prompt", "--", "--settings", settings}, []string{"--append-system-prompt", "--", s}, true, false},
		{"attached_value", []string{"--append-system-prompt=--settings", "--settings", settings}, []string{"--append-system-prompt=--settings", s}, true, false},
		{"optional_absent", []string{"--debug", "--settings", settings}, []string{"--debug", s}, true, false},
		{"optional_present", []string{"--debug", "api", "--settings", settings}, []string{"--debug", "api", s}, true, false},
		{"variadic", []string{"--tools", "Read", "Glob", "--settings", settings}, []string{"--tools", "Read", "Glob", s}, true, false},
		{"short_value", []string{"-n", "--settings", "--settings", settings}, []string{"-n", "--settings", s}, true, false},
		{"after_prompt", []string{"-p", "proof", "--settings", settings}, []string{"-p", "proof", s}, true, false},
		{"print_is_boolean", []string{"-p", "--settings"}, nil, false, true},
		{"unknown_boundary", []string{"--future-option", "--settings", settings}, nil, false, true},
		{"unknown_consumes_terminator", []string{"--future-option", "--", "--settings", settings}, nil, false, true},
		{"unknown_consumes_option_name", []string{"--future-option", "--name", "--settings", settings}, nil, false, true},
		{"unknown_without_settings", []string{"--future-option", "proof"}, []string{"--future-option", "proof"}, false, false},
		{"unknown_after_terminator", []string{"--", "--future-option", "--settings"}, []string{"--", "--future-option", "--settings"}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			forward, user, slots, err := takeUserSettings(test.args, t.TempDir())
			if test.invalid {
				if !errors.Is(err, errUserSettings) {
					t.Fatalf("expected settings refusal, got %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(forward, test.forward) || (user != nil) != test.settings || (len(slots) > 0) != test.settings {
				t.Fatalf("forward=%q settings=%v err=%v", forward, user != nil, err)
			}
		})
	}
	// A required value may start with '--'; file validation, not its spelling,
	// decides whether it is a settings source.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "--public.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	_, user, _, err := takeUserSettings([]string{"--settings", "--public.json"}, dir)
	if err != nil || string(user["theme"]) != `"dark"` {
		t.Fatalf("option-shaped file: %v", err)
	}
}

func TestUserSettingsLiteralTokensReachChild(t *testing.T) {
	for _, args := range [][]string{
		{"--append-system-prompt", "--settings", "-p", "proof"},
		{"-p", "--", "--settings", `{"disableAllHooks":true}`},
	} {
		s := runSession(t, sessionOptions{args: args})
		argv := s.report.Argv
		if s.err != nil || len(argv) < len(args) || !reflect.DeepEqual(argv[len(argv)-len(args):], args) {
			t.Fatalf("literal argv did not reach child: err=%v", s.err)
		}
	}
}

func TestUserSettingsMergedAtEveryOriginalBoundary(t *testing.T) {
	args := []string{"--debug", "--settings=missing.json", "proof", "--settings", `{"theme":"dark"}`, "-p"}
	s := runSession(t, sessionOptions{args: args})
	if s.err != nil {
		t.Fatal(s.err)
	}
	var merged string
	count := 0
	for _, arg := range s.report.Argv {
		if value, ok := strings.CutPrefix(arg, "--settings="); ok {
			if count != 0 && merged != value {
				t.Fatal("settings slots disagree")
			}
			merged = value
			count++
		}
	}
	var settings struct {
		Theme       string
		ModelPicker *modelPicker
	}
	if count != 2 || json.Unmarshal([]byte(merged), &settings) != nil || settings.Theme != "dark" || settings.ModelPicker == nil {
		t.Fatal("last user source or required settings lost")
	}
	want := []string{"--debug", "--settings=" + merged, "proof", "--settings=" + merged, "-p"}
	argv := s.report.Argv
	if len(argv) < len(want) || !reflect.DeepEqual(argv[len(argv)-len(want):], want) {
		t.Fatal("settings rewrite removed an option boundary")
	}
}

func TestUserSettingsRejectAmbiguityAndConnectionReplacement(t *testing.T) {
	for _, value := range []string{
		`{"theme":"dark","theme":"light"}`, `{"hooks":null}`, `{"disableAllHooks":true}`,
		`{"env":{"ANTHROPIC_BASE_URL":"http://example.invalid"}}`,
		`{"env":{"ANTHROPIC_CUSTOM_HEADERS":"x-public: value"}}`,
		`{"env":{"claude_code_max_context_tokens":"1"}}`,
		`{"env":{"CLAUDE_CODE_ENABLE_FUNCTION_HOOKS":"0"}}`,
		`{"modelPicker":{"replaceBuiltInOptions":false}}`,
	} {
		_, user, _, err := takeUserSettings([]string{"--settings=" + value}, t.TempDir())
		if err == nil {
			required, _ := sessionSettings("public-hook")
			_, err = mergeUserSettings(required, user)
		}
		if !errors.Is(err, errUserSettings) && !errors.Is(err, errSettingsConflict) {
			t.Fatal("ambiguous or conflicting settings accepted")
		}
	}
	for _, args := range [][]string{{"--settings"}, {"--settings", "--model"}, {"--settings="}} {
		if _, _, _, err := takeUserSettings(args, t.TempDir()); err == nil {
			t.Fatal("missing settings accepted")
		}
	}
}

// Real native CLI and fixed public backend; no user config or inference service.
func TestNativeUserSettingsAndSourceSelection(t *testing.T) {
	for _, test := range []struct{ source, inline, want string }{
		{"user", "{}", "medium"}, {"project", "{}", "xhigh"},
		{"", `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"high"}}`, "high"},
	} {
		t.Run(test.source+test.want, func(t *testing.T) {
			profile := t.TempDir()
			_, project := workspace(t)
			if err := os.Mkdir(filepath.Join(project, ".claude"), 0700); err != nil {
				t.Fatal(err)
			}
			for file, body := range map[string]string{
				filepath.Join(profile, "settings.json"):            `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"medium"}}`,
				filepath.Join(project, ".claude", "settings.json"): `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"xhigh"}}`,
			} {
				if err := os.WriteFile(file, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			recorder := &recordingTransport{inner: &upstream.Script{Default: measuredStream("PUBLIC_SETTINGS_OK")}}
			out := (nativeRun{ConfigDir: profile, Cwd: project, Args: []string{"-p", "public settings proof", "--settings", test.inline, "--setting-sources", test.source}, transport: recorder}).run(t)
			if out.err != nil || out.result.NativeExitCode != 0 {
				t.Fatalf("native exit=%d err=%v", out.result.NativeExitCode, out.err)
			}
			calls := recorder.calls
			if len(calls) == 0 {
				t.Fatal("no native request")
			}
			main := 0
			for _, c := range calls {
				if upstream.Conversation(string(c.Body)) {
					main++
					if c.Effort != test.want {
						t.Fatalf("actual effort %s want %s", c.Effort, test.want)
					}
				}
			}
			if main == 0 {
				t.Fatal("no main native request")
			}
		})
	}
}
