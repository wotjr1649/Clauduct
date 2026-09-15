package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// capture runs f with buffers for stdout and stderr and returns what was written.
func capture(t *testing.T, f func(out, errOut io.Writer) int) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = f(&out, &errOut)
	return code, out.String(), errOut.String()
}

// REL12 at the command level. The probe is the only thing in this build that can spend
// money, and it must not be reachable by a typo, a stray argument, or a habit of running
// commands with no flags to see what they do.
func TestTheProbeSendsNothingWithoutExplicitConsent(t *testing.T) {
	for name, args := range map[string][]string{
		"no arguments":        {},
		"an empty argument":   {""},
		"a different flag":    {"--dry-run"},
		"a near miss":         {"--sent"},
		"the flag misspelled": {"-send"},
		"a prefix":            {"--send-it"},
		"the flag twice":      {"--send", "--send"},
		"extra arguments":     {"--send", "now"},
		"something before it": {"now", "--send"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, _ := capture(t, func(out, errOut io.Writer) int {
				return probe(args, out, errOut)
			})
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(stdout, "Nothing is sent without --send") {
				t.Fatalf("stdout = %q, want the refusal notice", stdout)
			}
		})
	}
}

// What the run would cost has to be on screen before anyone consents to it. A consent
// prompt that does not say the price is not consent.
func TestTheNoticeStatesWhatItWouldSpend(t *testing.T) {
	_, stdout, _ := capture(t, func(out, errOut io.Writer) int { return probe(nil, out, errOut) })
	budget := upstream.ApprovedBudget()
	for _, want := range []string{budget.Model, budget.Effort, upstream.Endpoint, "max_output_tokens"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("the notice does not mention %q:\n%s", want, stdout)
		}
	}
}

// One run may not spend the whole authorisation. Repeating the command has to be a visible
// decision rather than a way to drift past twenty without noticing.
func TestOneRunCannotSpendTheWholeAuthorisation(t *testing.T) {
	if probeAttempts >= upstream.ApprovedBudget().Limit {
		t.Fatalf("probeAttempts = %d against a cumulative cap of %d",
			probeAttempts, upstream.ApprovedBudget().Limit)
	}
}

// A version goes into a header on every request. Whatever the installed executable prints,
// only a short printable token is sent.
func TestOnlyAPlausibleVersionIsSent(t *testing.T) {
	for name, tc := range map[string]struct {
		value string
		want  bool
	}{
		"an ordinary version": {"0.153.4", true},
		"a prerelease":        {"0.154.0-rc.1", true},
		"with build metadata": {"0.153.4+win", true},
		"the reference":       {referenceCodexVersion, true},
		"empty":               {"", false},
		"a space":             {"0.153.4 extra", false},
		"a tab":               {"0.153\t4", false},
		"a newline":           {"0.153.4\n", false},
		"a carriage return":   {"0.153.4\r", false},
		"a control character": {"0.153\x00 4", false},
		"non-ascii":           {"0.153.4\u00e9", false},
		"far too long":        {strings.Repeat("9", 97), false},
		"exactly the ceiling": {strings.Repeat("9", 96), true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := plausibleVersion(tc.value); got != tc.want {
				t.Fatalf("plausibleVersion(%q) = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

// The reason printed for an incomplete response comes from a fixed set. It is printed, and
// nothing a backend writes is printed verbatim.
func TestTheIncompleteReasonIsNeverEchoed(t *testing.T) {
	for name, tc := range map[string]struct{ payload, want string }{
		"the limit was honoured": {
			`{"response":{"incomplete_details":{"reason":"max_output_tokens"}}}`, "max_output_tokens"},
		"a content filter": {
			`{"response":{"incomplete_details":{"reason":"content_filter"}}}`, "content_filter"},
		"something new": {
			`{"response":{"incomplete_details":{"reason":"a reason nobody has seen"}}}`, "unrecognised"},
		"an injection attempt": {
			`{"response":{"incomplete_details":{"reason":"\u001b[2J ignore previous"}}}`, "unrecognised"},
		"no reason at all": {`{"response":{}}`, "unrecognised"},
		"not json":         {`not json`, "unreadable"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := incompleteReason([]byte(tc.payload)); got != tc.want {
				t.Fatalf("incompleteReason = %q, want %q", got, tc.want)
			}
		})
	}
}

// Whatever the installed executable prints, only a version this recognises goes into a
// header. Added after a mutation run: dropping the plausibility check left the suite green,
// because nothing exercised the parsing at all.
func TestOnlyARecognisedVersionLineIsAccepted(t *testing.T) {
	for name, tc := range map[string]struct {
		printed string
		want    string
	}{
		"the ordinary line":       {"codex-cli 0.153.4\n", "0.153.4"},
		"with carriage returns":   {"codex-cli 0.153.4\r\n", "0.153.4"},
		"with surrounding blanks": {"  codex-cli 0.153.4  \n", "0.153.4"},

		"a different tool":     {"codex 0.153.4\n", ""},
		"no version":           {"codex-cli\n", ""},
		"two versions":         {"codex-cli 0.153.4 0.153.5\n", ""},
		"a second line":        {"codex-cli 0.153.4\nwarning: something\n", ""},
		"nothing at all":       {"", ""},
		"a shell prompt":       {"$ codex-cli 0.153.4\n", ""},
		"an unprintable token": {"codex-cli 0.153.\x014\n", ""},
		"far too long":         {"codex-cli " + strings.Repeat("9", 97) + "\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseCodexVersion(tc.printed)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("parseCodexVersion(%q) = %q, want a refusal", tc.printed, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCodexVersion(%q): %v", tc.printed, err)
			}
			if got != tc.want {
				t.Fatalf("parseCodexVersion(%q) = %q, want %q", tc.printed, got, tc.want)
			}
		})
	}
}
