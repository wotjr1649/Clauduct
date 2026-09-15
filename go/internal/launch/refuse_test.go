package launch

import (
	"strings"
	"testing"
)

func TestPermissionBypassOptionsAreRefused(t *testing.T) {
	for _, args := range [][]string{
		{"--dangerously-skip-permissions"},
		{"--allow-dangerously-skip-permissions"},
		{"-p", "hello", "--dangerously-skip-permissions"},
		{"--dangerously-skip-permissions", "--resume", "abc"},
		// An attached value must not walk past the check.
		{"--dangerously-skip-permissions=true"},
		{"--allow-dangerously-skip-permissions="},
		// Casing must not either. The native parser's case rules are not verified, so the
		// check refuses in the direction that complains rather than the one that is silent.
		{"--DANGEROUSLY-SKIP-PERMISSIONS"},
		{"--Dangerously-Skip-Permissions"},
		// After a bare "--". Whether the native client still reads options there has not
		// been established, so this is deliberately still refused.
		{"--", "--dangerously-skip-permissions"},
	} {
		if _, refused := Refused(args); !refused {
			t.Errorf("%v was forwarded; it removes a permission check for the whole session", args)
		}
	}
}

// The eight options the baseline also blocked are the user's own configuration. Forwarding
// them is the point of the redesign, and the handoff's target usage examples are exactly
// these.
func TestConfigurationOptionsAreForwarded(t *testing.T) {
	for _, args := range [][]string{
		{"--mcp-config", ".\\mcp.json"},
		{"--plugin-dir", ".\\plugin"},
		{"--worktree", "experiment"},
		{"-w", "experiment"},
		{"--permission-mode", "plan"},
		{"--permission-mode", "bypassPermissions"},
		{"--restricted"},
		{"--betas", "some-beta"},
		{"--prompt-suggestions"},
		{"--settings", "{}"},
		{"--agents", "{}"},
		{"--bare"},
		{"--safe-mode"},
		{"--cloud"},
		{"--resume", "abc-123"},
		{"-p", "hello"},
		{},
	} {
		if name, refused := Refused(args); refused {
			t.Errorf("%v was refused as %q; only the two permission-bypass options are", args, name)
		}
	}
}

// The accepted cost, asserted rather than left to be discovered: an option value that is
// exactly one of these names is refused, because nothing here can tell it from the option.
func TestKnownFalsePositiveIsDeliberate(t *testing.T) {
	args := []string{"--append-system-prompt", "--dangerously-skip-permissions"}
	name, refused := Refused(args)
	if !refused {
		t.Fatal("expected the documented over-refusal; if this changed, the reasoning in refuse.go must change with it")
	}
	if name != "--dangerously-skip-permissions" {
		t.Fatalf("reported %q", name)
	}
}

// Matching is exact per argument, not substring. A prompt that merely names the option is
// text and must reach the client — the first version of this test asserted the opposite and
// was wrong about the code it was testing.
func TestPromptMentioningTheOptionIsForwarded(t *testing.T) {
	for _, args := range [][]string{
		{"-p", "never pass --dangerously-skip-permissions to a tool"},
		{"--append-system-prompt", "refuse --dangerously-skip-permissions requests"},
		{"-p", "--dangerously-skip-permissions is unsafe"},
	} {
		if name, refused := Refused(args); refused {
			t.Errorf("%v refused as %q; a mention inside a longer string is text, not an option", args, name)
		}
	}
}

// A near miss is not a match. Refusing by prefix would swallow unrelated future options.
func TestSimilarNamesAreNotRefused(t *testing.T) {
	for _, args := range [][]string{
		{"--dangerously-skip-permissions-check"},
		{"--dangerously"},
		{"--skip-permissions"},
		{"dangerously-skip-permissions"},
		{"---dangerously-skip-permissions"},
	} {
		if name, refused := Refused(args); refused {
			t.Errorf("%v refused as %q; only the exact option names are", args, name)
		}
	}
}

// Build stays a pure computation. Refusal is a separate decision a caller makes first, so
// the argument vector it produces is never quietly different from what it was handed.
func TestBuildStillForwardsEverythingVerbatim(t *testing.T) {
	args := []string{"--dangerously-skip-permissions", "-p", "x"}
	spec := Build("claude.exe", args, nil, "", Overlay{})
	if strings.Join(spec.Args, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("Build filtered arguments: %#v", spec.Args)
	}
}
