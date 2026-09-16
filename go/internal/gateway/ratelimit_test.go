package gateway

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// measuredLimits is what the backend actually sent on 2026-09-17, as `clauduct-dev probe
// headers --send` read it off the wire. Written out rather than summarised: the point of
// this fixture is that nobody chose these values.
func measuredLimits() http.Header {
	return http.Header{
		"X-Codex-Active-Limit":                         {"bengalfox"},
		"X-Codex-Primary-Used-Percent":                 {"46"},
		"X-Codex-Primary-Window-Minutes":               {"10080"},
		"X-Codex-Primary-Reset-At":                     {"1789806962"},
		"X-Codex-Primary-Reset-After-Seconds":          {""},
		"X-Codex-Primary-Over-Secondary-Limit-Percent": {""},
		"X-Codex-Secondary-Used-Percent":               {"0"},
		"X-Codex-Secondary-Window-Minutes":             {"0"},
		"X-Codex-Secondary-Reset-At":                   {""},
		"X-Codex-Bengalfox-Limit-Name":                 {""},
		"X-Codex-Bengalfox-Primary-Used-Percent":       {"0"},
		"X-Codex-Bengalfox-Primary-Window-Minutes":     {"300"},
		"X-Codex-Bengalfox-Primary-Reset-At":           {"1789608890"},
		"X-Codex-Bengalfox-Secondary-Used-Percent":     {"0"},
		"X-Codex-Bengalfox-Secondary-Window-Minutes":   {"10080"},
		"X-Codex-Bengalfox-Secondary-Reset-At":         {"1790195690"},
		"X-Codex-Plan-Type":                            {"pro"},
		"X-Codex-Credits-Balance":                      {"0"},
		"X-Codex-Credits-Unlimited":                    {"false"},
		"X-Codex-Turn-State":                           {"ok"},
		"Set-Cookie":                                   {"__cf_bm=SECRET-COOKIE-VALUE; Path=/"},
		"X-Oai-Request-Id":                             {"req_0123456789"},
	}
}

// D5. The reading against what the backend actually sends.
//
// The one test that could not be written from the baseline: its validator calls an empty
// value numeric-format, and this response has one, so it would report invalid every time.
func TestTheReadingAgainstWhatTheBackendActuallySends(t *testing.T) {
	report := readLimits(measuredLimits())

	if report.State != limitPartial {
		t.Fatalf("state = %s, want %s: five of the six numbers arrived and the sixth is "+
			"empty, which is absent rather than broken (%+v)", report.State, limitPartial, report)
	}
	if report.InvalidReason != "" {
		t.Errorf("invalidReason = %q for a response with nothing wrong with it",
			report.InvalidReason)
	}
	if report.ActiveLimit != "bengalfox" {
		t.Errorf("activeLimit = %q; without it a reader who sees two families has to guess",
			report.ActiveLimit)
	}
	if report.Primary == nil || report.Primary.UsedPercent == nil || *report.Primary.UsedPercent != 46 {
		t.Fatalf("primary = %+v", report.Primary)
	}
	if *report.Primary.WindowMinutes != 10080 || *report.Primary.ResetAt != 1789806962 {
		t.Errorf("primary = %+v", *report.Primary)
	}
	if report.Secondary == nil || report.Secondary.ResetAt != nil {
		t.Fatalf("secondary = %+v; the empty reset must stay absent rather than become a zero",
			report.Secondary)
	}
	if *report.Secondary.UsedPercent != 0 || *report.Secondary.WindowMinutes != 0 {
		t.Errorf("secondary = %+v", *report.Secondary)
	}
	if strings.Join(report.OtherFamilies, ",") != "x-codex-bengalfox" {
		t.Errorf("otherFamilies = %v", report.OtherFamilies)
	}
}

// Six numbers is a reading. Fewer is a partial one, and none is no reading at all.
func TestAReadingSaysHowMuchOfItArrived(t *testing.T) {
	whole := http.Header{
		"X-Codex-Primary-Used-Percent":     {"12.5"},
		"X-Codex-Primary-Window-Minutes":   {"300"},
		"X-Codex-Primary-Reset-At":         {"1789806962"},
		"X-Codex-Secondary-Used-Percent":   {"3"},
		"X-Codex-Secondary-Window-Minutes": {"10080"},
		"X-Codex-Secondary-Reset-At":       {"-1"},
	}
	if report := readLimits(whole); report.State != limitObserved {
		t.Errorf("state = %s for all six, want %s", report.State, limitObserved)
	}
	// Signed upstream. A negative reset is not a reset in the future, and it is reported as
	// it arrived rather than dropped for being odd.
	if report := readLimits(whole); *report.Secondary.ResetAt != -1 {
		t.Errorf("reset = %d", *report.Secondary.ResetAt)
	}

	none := http.Header{"X-Codex-Plan-Type": {"pro"}, "Date": {"today"}}
	if report := readLimits(none); report.State != limitMissing {
		t.Errorf("state = %s for a response that said nothing about the quota", report.State)
	}
}

// A reading that is wrong says which field and why, and takes no number from it.
func TestABrokenFieldSaysWhichOneAndWhy(t *testing.T) {
	for name, tc := range map[string]struct {
		header http.Header
		reason string
		field  string
	}{
		"two answers to one question": {
			header: http.Header{"X-Codex-Primary-Used-Percent": {"10", "90"}},
			reason: reasonDuplicate, field: "primary.used-percent",
		},
		"a percentage that is not a number": {
			header: http.Header{"X-Codex-Primary-Used-Percent": {"lots"}},
			reason: reasonFormat, field: "primary.used-percent",
		},
		"a percentage past a hundred": {
			header: http.Header{"X-Codex-Primary-Used-Percent": {"140"}},
			reason: reasonRange, field: "primary.used-percent",
		},
		"a negative window": {
			header: http.Header{"X-Codex-Secondary-Window-Minutes": {"-5"}},
			reason: reasonRange, field: "secondary.window-minutes",
		},
		"a value far too long to be a number": {
			header: http.Header{"X-Codex-Primary-Reset-At": {strings.Repeat("9", 200)}},
			reason: reasonFormat, field: "primary.reset-at",
		},
	} {
		t.Run(name, func(t *testing.T) {
			report := readLimits(tc.header)
			if report.State != limitInvalid {
				t.Fatalf("state = %s, want %s", report.State, limitInvalid)
			}
			if report.InvalidReason != tc.reason || report.InvalidField != tc.field {
				t.Fatalf("reason=%q field=%q, want %q %q",
					report.InvalidReason, report.InvalidField, tc.reason, tc.field)
			}
			if report.Primary != nil && report.Primary.UsedPercent != nil {
				t.Errorf("a number was taken from a reading that is wrong: %+v", *report.Primary)
			}
		})
	}
}

// Nothing but the numbers and two shaped labels leaves the gateway.
//
// The response is the backend's bytes. The account is written to a file and printed at the
// end of a session, and a plan name, a credit balance or a cookie has no business in it.
func TestTheReadingKeepsNothingButTheQuota(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE:    sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
		Header: measuredLimits(),
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	body := bodyText(t, do(t, g, request{method: http.MethodGet, path: statusPath}))
	for _, secret := range []string{
		"SECRET-COOKIE-VALUE", "__cf_bm", "req_0123456789", "pro", "unlimited",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("the account kept %q:\n%s", secret, body)
		}
	}
	// And the quota itself did arrive, so the check above is not passing on an empty report.
	if !strings.Contains(body, `"usedPercent":46`) {
		t.Fatalf("the reading never reached the account:\n%s", body)
	}
}

// A backend-controlled label is shaped and bounded before it is kept.
func TestALabelThatIsNotOneIsNotKept(t *testing.T) {
	for _, active := range []string{
		"<script>alert(1)</script>",
		strings.Repeat("a", 200),
		"has spaces",
		"UPPER",
		"",
	} {
		header := measuredLimits()
		header.Set("X-Codex-Active-Limit", active)
		if got := readLimits(header).ActiveLimit; got != "" {
			t.Errorf("activeLimit = %q for %q", got, active)
		}
	}
}

// A response that says nothing does not erase what the last one said.
//
// The reading is a live quota. An answer that arrived an hour ago beats no answer, and a
// transport that carries no headers at all has not contradicted it.
func TestAResponseWithoutHeadersLeavesTheLastReadingAlone(t *testing.T) {
	fixture := &upstream.Fixture{
		SSE:    sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
		Header: measuredLimits(),
	}
	g := startWith(t, fixture)
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	if status(t, g).Limits == nil {
		t.Fatal("the first reading never arrived")
	}

	// Two ways a response says nothing: no headers at all, and headers with no quota in
	// them. The second is the one a real backend produces on a bad day.
	for name, header := range map[string]http.Header{
		"no headers at all":             nil,
		"headers with no quota in them": {"Date": {"today"}, "X-Codex-Plan-Type": {"pro"}},
	} {
		t.Run(name, func(t *testing.T) {
			fixture.Header = header
			post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
			  "messages":[{"role":"user","content":"y"}]}`)

			report := status(t, g).Limits
			if report == nil || report.Primary == nil || *report.Primary.UsedPercent != 46 {
				t.Fatalf("the reading was erased by a response that said nothing: %+v", report)
			}
		})
	}
}
