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
		"no arguments":          {},
		"a name and no flag":    {"limit"},
		"a flag and no name":    {"--send"},
		"an empty name":         {"", "--send"},
		"an unknown name":       {"tools", "--send"},
		"a near miss on name":   {"limits", "--send"},
		"a near miss on flag":   {"limit", "--sent"},
		"the flag misspelled":   {"limit", "-send"},
		"a prefix of the flag":  {"limit", "--send-it"},
		"the flag twice":        {"limit", "--send", "--send"},
		"extra arguments":       {"limit", "--send", "now"},
		"the order reversed":    {"--send", "limit"},
		"a name that is a flag": {"--send", "--send"},
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
	want := []string{budget.Model, budget.Effort, upstream.Endpoint, "max_output_tokens"}
	for name := range probes {
		want = append(want, name)
	}
	for _, text := range want {
		if !strings.Contains(stdout, text) {
			t.Fatalf("the notice does not mention %q:\n%s", text, stdout)
		}
	}
}

// One run may not spend the whole authorisation. Repeating the command has to be a visible
// decision rather than a way to drift past twenty without noticing.
func TestOneRunCannotSpendTheWholeAuthorisation(t *testing.T) {
	total := 0
	for name, p := range probes {
		if p.attempts >= upstream.ApprovedBudget().Limit {
			t.Fatalf("probe %q claims %d attempts against a cumulative cap of %d",
				name, p.attempts, upstream.ApprovedBudget().Limit)
		}
		total += p.attempts
	}
	// Running every probe once must also stay inside the authorisation.
	if total > upstream.ApprovedBudget().Limit {
		t.Fatalf("all probes together claim %d attempts against a cap of %d", total,
			upstream.ApprovedBudget().Limit)
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

// D5. A probe that prints response headers must not print the values of headers that carry
// credentials, and a real response carries one: set-cookie arrived on the measured run.
func TestTheHeaderProbePrintsValuesOnlyForRateLimitFields(t *testing.T) {
	for name, isField := range map[string]bool{
		"x-codex-primary-used-percent":             true,
		"x-codex-secondary-reset-at":               true,
		"x-codex-bengalfox-primary-window-minutes": true,
		"set-cookie":                                      false,
		"authorization":                                   false,
		"x-codex-credits-balance":                         false,
		"x-codex-plan-type":                               false,
		"x-codex-primary-reset-after-seconds":             false,
		"x-codex-primary-over-secondary-limit-percent":    false,
		"x-oai-request-id":                                false,
		"x-codex-primary-used-percent-and-something-else": false,
	} {
		if got := rateLimitField.MatchString(name); got != isField {
			t.Errorf("%q = %v, want %v", name, got, isField)
		}
	}
}
