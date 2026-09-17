package app

import (
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

func window(percent float64, minutes, resetAt int64) *gateway.RateWindow {
	w := &gateway.RateWindow{UsedPercent: &percent, WindowMinutes: &minutes}
	if resetAt != 0 {
		w.ResetAt = &resetAt
	}
	return w
}

// What a person reads when they ask what this account has spent.
//
// The figures are the measured ones: a weekly window, a second family the backend mentions
// and this build does not read, and a state of partial because one field arrives empty.
func TestUsageReadsTheAccountInWordsAPersonAsksIn(t *testing.T) {
	dir := t.TempDir()
	reset := time.Now().Add(50 * time.Hour).Unix()
	writeAccount(t, dir, "status-100.json", 12*time.Minute, Status{
		Attempts:   3,
		Inferences: 3,
		Gateway: gateway.Diagnostics{Limits: &gateway.RateLimitReport{
			State:         "partial",
			ActiveLimit:   "premium",
			Primary:       window(47, 10080, reset),
			Secondary:     window(0, 0, 0),
			OtherFamilies: []string{"x-codex-bengalfox"},
		}},
	})
	// A session that spent nothing, which is most of them.
	writeAccount(t, dir, "status-101.json", time.Minute, Status{})

	var out strings.Builder
	if code := WriteUsage(dir, &out); code != 0 {
		t.Fatalf("code = %d: %s", code, out.String())
	}
	printed := out.String()
	for _, want := range []string{
		"47% used",
		"of a 1 week window", // not 10080 minutes: the unit is the one being asked about
		"resets in 2 days",
		"premium",
		"x-codex-bengalfox",
		"12 minutes ago", // a cached reading has to say how old it is
		"3 attempts, 3 inferences over 1 session that spent",
		"the account's", // the limit is not this machine's
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("missing %q in:\n%s", want, printed)
		}
	}
	// The empty account must not be the one reported.
	if strings.Contains(printed, "status-101") {
		t.Fatalf("reported the session that heard nothing:\n%s", printed)
	}
}

// With nothing to read it says so, and says what would produce a reading.
//
// A quota arrives on a backend response, so a machine that has only ever run --version has
// none. That is not a failure of this command and the message should not read like one.
func TestUsageWithNoReadingSaysWhatWouldProduceOne(t *testing.T) {
	var out strings.Builder
	if code := WriteUsage(t.TempDir(), &out); code == 0 {
		t.Fatal("no reading reported success; a script cannot tell the difference")
	}
	printed := out.String()
	for _, want := range []string{"no reading yet", "headers on a backend response"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("missing %q in: %s", want, printed)
		}
	}
}

// A window is named in the unit it is asked about.
func TestWindowLengthsReadAsPeopleSayThem(t *testing.T) {
	for _, c := range []struct {
		minutes int64
		want    string
	}{
		{10080, "1 week"},
		{20160, "2 weeks"},
		{1440, "1 day"},
		{300, "5 hours"},
		{60, "1 hour"},
		{90, "90 minutes"},
	} {
		if got := windowLength(c.minutes); got != c.want {
			t.Fatalf("windowLength(%d) = %q, want %q", c.minutes, got, c.want)
		}
	}
}
