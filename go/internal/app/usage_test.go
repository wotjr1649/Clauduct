package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// writeAccount puts one session's account in dir, with the age it should appear to have.
func writeAccount(t *testing.T, dir, name string, age time.Duration, status Status) string {
	t.Helper()
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

func quota(percent float64, minutes int64) *gateway.RateLimitReport {
	return &gateway.RateLimitReport{
		State:       "partial",
		ActiveLimit: "premium",
		Primary:     &gateway.RateWindow{UsedPercent: &percent, WindowMinutes: &minutes},
	}
}

// The newest reading that has a quota wins, not simply the newest.
//
// A session that asked the backend nothing has no headers to report, and there are many of
// those -- every test run leaves one. Taking the newest account outright would hide a real
// figure from ten minutes ago behind an empty one from ten seconds ago.
func TestTheNewestReadingWithAQuotaIsTheOneReported(t *testing.T) {
	dir := t.TempDir()
	writeAccount(t, dir, "status-1.json", 30*time.Minute, Status{
		Attempts: 3, Gateway: gateway.Diagnostics{Limits: quota(12, 10080)}})
	writeAccount(t, dir, "status-2.json", 10*time.Minute, Status{
		Attempts: 2, Gateway: gateway.Diagnostics{Limits: quota(47, 10080)}})
	// Newest, and says nothing.
	writeAccount(t, dir, "status-3.json", time.Minute, Status{})

	reading, ok := LatestQuota(dir)
	if !ok {
		t.Fatal("no reading found among three accounts")
	}
	if got := *reading.Status.Gateway.Limits.Primary.UsedPercent; got != 47 {
		t.Fatalf("reported %g%%, want the 47%% from ten minutes ago", got)
	}
	if age := reading.Age(); age < 9*time.Minute || age > 12*time.Minute {
		t.Fatalf("age = %v, want about ten minutes: a reading has to say how old it is", age)
	}
}

// Everything in that directory is read except what cannot be.
//
// It is a temporary directory, so anything may be in it, and a session that is still running
// may be halfway through writing its own. Neither is an error worth reporting.
func TestUnreadableAccountsAreSkippedRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	writeAccount(t, dir, "status-good.json", time.Minute, Status{
		Attempts: 1, Gateway: gateway.Diagnostics{Limits: quota(5, 300)}})
	if err := os.WriteFile(filepath.Join(dir, "status-half.json"), []byte(`{"cat`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not an account"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "status-dir.json"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	readings := Readings(dir, 0)
	if len(readings) != 1 {
		t.Fatalf("readings = %d, want the one that decodes", len(readings))
	}
	if _, ok := LatestQuota(dir); !ok {
		t.Fatal("the one good account was not found")
	}
	// And a directory with nothing in it is an answer, not a failure.
	if _, ok := LatestQuota(t.TempDir()); ok {
		t.Fatal("an empty directory produced a reading")
	}
}

// The exit line carries the quota, and says nothing when the session never heard one.
func TestTheExitLineCarriesTheQuotaWhenThereIsOne(t *testing.T) {
	for _, c := range []struct {
		name   string
		limits *gateway.RateLimitReport
		want   string
	}{
		{"a weekly window", quota(47, 10080), "quota=47%/7d"},
		{"a five hour window", quota(3.5, 300), "quota=3.5%/5h"},
		{"no reading", nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := quotaField(Status{Gateway: gateway.Diagnostics{Limits: c.limits}})
			if strings.TrimSpace(got) != c.want {
				t.Fatalf("field = %q, want %q", got, c.want)
			}
		})
	}

	// A report that arrived without a usable number must not produce an empty field.
	empty := &gateway.RateLimitReport{State: "missing"}
	if got := quotaField(Status{Gateway: gateway.Diagnostics{Limits: empty}}); got != "" {
		t.Fatalf("field = %q for a report with no primary window", got)
	}
}

// Report writes the field onto the line the user actually sees.
func TestReportPutsTheQuotaOnTheLine(t *testing.T) {
	var out strings.Builder
	Report(Result{
		Category:    CategorySuccess,
		Diagnostics: gateway.Diagnostics{Limits: quota(47, 10080)},
	}, &out, map[string]string{})

	line := out.String()
	if !strings.Contains(line, "quota=47%/7d") {
		t.Fatalf("the exit line does not carry the quota: %s", line)
	}
	// And a quota must not by itself make a session one with something to report. That
	// judgement decides whether the whole account is printed on every exit, and a figure
	// every healthy session carries would print it every time -- the diagnosis that cries
	// always, which this build spent a whole work package avoiding.
	quiet := Status{Category: CategorySuccess}
	loud := Status{Category: CategorySuccess, Gateway: gateway.Diagnostics{Limits: quota(47, 10080)}}
	if quiet.noteworthy() != loud.noteworthy() {
		t.Fatal("carrying a quota changed whether the session reports itself")
	}
}
