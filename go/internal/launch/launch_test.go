package launch

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func envValue(spec Spec, name string) (string, bool) {
	for _, entry := range spec.Env {
		if key, value, ok := strings.Cut(entry, "="); ok && key == name {
			return value, true
		}
	}
	return "", false
}

func envNames(spec Spec) []string {
	names := make([]string, 0, len(spec.Env))
	for _, entry := range spec.Env {
		key, _, _ := strings.Cut(entry, "=")
		names = append(names, key)
	}
	return names
}

// ARG01: order, count and values survive unchanged.
func TestArgsPreservedExactly(t *testing.T) {
	forward := []string{"--resume", "abc-123", "-p", "hello", "world"}
	spec := Build("claude.exe", forward, nil, "C:\\work", Overlay{})
	if !reflect.DeepEqual(spec.Args, forward) {
		t.Fatalf("args changed\n got: %#v\nwant: %#v", spec.Args, forward)
	}
	if spec.File != "claude.exe" || spec.Dir != "C:\\work" {
		t.Fatalf("file/dir changed: %q %q", spec.File, spec.Dir)
	}
}

// Build must not alias the caller's slice: a later append by the caller must not reach
// into a spec that was already handed to a spawner.
func TestArgsAreCopied(t *testing.T) {
	forward := []string{"--model", "sol"}
	spec := Build("claude.exe", forward, nil, "", Overlay{})
	forward[1] = "MUTATED"
	if spec.Args[1] != "sol" {
		t.Fatalf("spec aliased caller slice: %q", spec.Args[1])
	}
}

// ARG02, ARG03, ARG10: the hostile shapes. Empty strings, quotes, backslashes, shell
// metacharacters, Korean and emoji all travel as data.
func TestArgsHostileShapes(t *testing.T) {
	forward := []string{
		"",
		" leading and trailing ",
		`he said "hi"`,
		`C:\path\with\trailing\`,
		`a"b\"c`,
		"&", "|", "<", ">", "^", "%PATH%", "!DELAYED!",
		"rm -rf / ; echo pwned",
		"$(whoami)", "`whoami`",
		"한국어 인자",
		"이모지 🙂 포함",
		`{"tool":"Read","input":{"file_path":"C:\\a\\b.txt"}}`,
	}
	spec := Build("claude.exe", forward, nil, "", Overlay{})
	if !reflect.DeepEqual(spec.Args, forward) {
		t.Fatalf("args changed\n got: %#v\nwant: %#v", spec.Args, forward)
	}
}

// ARG04, ARG05: there is no parser, so a value that looks like an option is still a value
// and everything after -- is untouched. This is the property the whole pass-through design
// rests on, and it is asserted here rather than assumed.
func TestOptionLookalikeValuesAreNotIntercepted(t *testing.T) {
	forward := []string{
		"--append-system-prompt", "--model 은 설명용 문자열이다",
		"--name", "--version",
		"--",
		"--model", "--help", "-p", "--settings",
	}
	spec := Build("claude.exe", forward, nil, "", Overlay{})
	if !reflect.DeepEqual(spec.Args, forward) {
		t.Fatalf("a lookalike value was touched\n got: %#v\nwant: %#v", spec.Args, forward)
	}
}

// ENV01: the decision of record. Service and MCP credentials reach the child, because the
// native client is trusted with what it would see if the user ran it directly.
func TestServiceAndMCPSecretsAreInherited(t *testing.T) {
	source := map[string]string{
		"GITHUB_TOKEN":           "gh-dummy",
		"AWS_ACCESS_KEY_ID":      "aws-dummy",
		"AWS_SECRET_ACCESS_KEY":  "aws-dummy-secret",
		"SLACK_BOT_TOKEN":        "slack-dummy",
		"NOTION_API_KEY":         "notion-dummy",
		"DATABASE_PASSWORD":      "pw-dummy",
		"MY_SERVICE_PRIVATE_KEY": "pk-dummy",
		"CLAUDE_CONFIG_DIR":      "C:\\Users\\dev\\.claude",
		"PATH":                   "C:\\Windows",
	}
	spec := Build("claude.exe", nil, source, "", Overlay{BaseURL: "http://127.0.0.1:1", AuthToken: "t"})
	for name, want := range source {
		got, ok := envValue(spec, name)
		if !ok {
			t.Errorf("%s was dropped; the accepted rule inherits it", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// The regression that motivated the rule change. The Node baseline's substring heuristic
// matched TOKENS inside these size limits and silently removed a user's configuration.
func TestOutputTokenLimitsAreNotMistakenForSecrets(t *testing.T) {
	source := map[string]string{
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":           "32000",
		"CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS": "50000",
		"MAX_MCP_OUTPUT_TOKENS":                   "25000",
	}
	spec := Build("claude.exe", nil, source, "", Overlay{})
	for name, want := range source {
		if got, ok := envValue(spec, name); !ok || got != want {
			t.Errorf("%s = %q ok=%v, want %q; a size limit is not a credential", name, got, ok, want)
		}
	}
}

// ENV02: the invariant that is not negotiable. No parent Anthropic credential reaches the
// child, by name or by value.
func TestAnthropicCredentialsNeverReachTheChild(t *testing.T) {
	const leak = "SHOULD-NOT-APPEAR-ANYWHERE"
	source := map[string]string{
		"ANTHROPIC_API_KEY":        leak,
		"ANTHROPIC_AUTH_TOKEN":     leak,
		"ANTHROPIC_CUSTOM_HEADERS": "X-Api-Key: " + leak,
		"ANTHROPIC_BASE_URL":       "https://api.anthropic.com",
		"anthropic_api_key":        leak,
		"CLAUDE_CODE_OAUTH_TOKEN":  leak,

		// Names outside the overlay's five keys. Without them this test cannot fail:
		// the overlay overwrites those five unconditionally, so it would still pass with
		// the prefix rule deleted entirely. A mutation run caught exactly that.
		"ANTHROPIC_API_URL":            leak,
		"ANTHROPIC_BEDROCK_BASE_URL":   leak,
		"ANTHROPIC_VERTEX_PROJECT_ID":  leak,
		"ANTHROPIC_DEFAULT_OPUS_MODEL": leak,
		"Anthropic_Session_Key":        leak,

		"KEEP_ME": "kept",
	}
	spec := Build("claude.exe", nil, source, "", Overlay{
		BaseURL:   "http://127.0.0.1:9",
		AuthToken: "session-token",
	})

	for _, entry := range spec.Env {
		if strings.Contains(entry, leak) {
			key, _, _ := strings.Cut(entry, "=")
			t.Fatalf("parent credential survived in %s", key)
		}
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_OAUTH_TOKEN"} {
		value, ok := envValue(spec, name)
		if !ok {
			t.Errorf("%s absent; it must be present and empty so the client cannot fall back to a stored credential", name)
		}
		if value != "" {
			t.Errorf("%s = %q, want empty", name, value)
		}
	}
	if got, _ := envValue(spec, "ANTHROPIC_BASE_URL"); got != "http://127.0.0.1:9" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want the session gateway", got)
	}
	if got, _ := envValue(spec, "ANTHROPIC_AUTH_TOKEN"); got != "session-token" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want the session token", got)
	}
	if got, _ := envValue(spec, "KEEP_ME"); got != "kept" {
		t.Errorf("unrelated variable lost: %q", got)
	}
	// Any ANTHROPIC_* name outside the overlay must be gone, not merely blanked. A stale
	// endpoint or project id left in place can redirect the client away from the gateway.
	for _, name := range envNames(spec) {
		if !strings.HasPrefix(strings.ToUpper(name), "ANTHROPIC_") {
			continue
		}
		switch strings.ToUpper(name) {
		case "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_CUSTOM_HEADERS":
		default:
			t.Errorf("%s reached the child; only the overlay's ANTHROPIC_ names may exist", name)
		}
	}
}

// ENV03: Windows environment names are case-insensitive, so a source map can carry a
// collision that a Go map cannot represent as one entry. The child must receive exactly
// one, chosen by a rule this package owns rather than by whatever os/exec would do.
func TestWindowsEnvNameCollisionResolvesToOneEntry(t *testing.T) {
	spec := Build("claude.exe", nil, map[string]string{
		"Path": "C:\\first",
		"PATH": "C:\\second",
		"pAtH": "C:\\third",
	}, "", Overlay{})

	names := envNames(spec)
	count := 0
	for _, name := range names {
		if strings.EqualFold(name, "path") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d case variants of PATH, want exactly 1: %v", count, names)
	}
	// Sorted iteration makes the survivor deterministic: the name that sorts last wins,
	// which matches how os/exec collapses a duplicate and so cannot be re-resolved later.
	if got, _ := envValue(spec, "pAtH"); got != "C:\\third" {
		t.Fatalf("collision resolved to %q; want the last-sorting name's value C:\\third", got)
	}
}

// Determinism: the same inputs must produce byte-identical specs, or a comparison run
// cannot tell a real difference from map iteration order.
func TestEnvIsSortedAndStable(t *testing.T) {
	source := map[string]string{"B": "2", "A": "1", "C": "3", "a_lower": "4"}
	first := Build("claude.exe", nil, source, "", Overlay{})
	second := Build("claude.exe", nil, source, "", Overlay{})
	if !reflect.DeepEqual(first.Env, second.Env) {
		t.Fatalf("env not stable across calls")
	}
	if !slices.IsSorted(first.Env) {
		t.Fatalf("env not sorted: %v", first.Env)
	}
}

// The overlay is the entire set of keys Clauduct adds. If this list grows, it grew because
// someone decided it should, not because a helper quietly needed one more.
func TestOverlayAddsExactlyFiveKeys(t *testing.T) {
	spec := Build("claude.exe", nil, nil, "", Overlay{BaseURL: "http://127.0.0.1:1", AuthToken: "t"})
	want := []string{
		"ANTHROPIC_API_KEY=",
		"ANTHROPIC_AUTH_TOKEN=t",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:1",
		"ANTHROPIC_CUSTOM_HEADERS=",
		"CLAUDE_CODE_OAUTH_TOKEN=",
	}
	if !reflect.DeepEqual(spec.Env, want) {
		t.Fatalf("overlay changed\n got: %#v\nwant: %#v", spec.Env, want)
	}
}
