package ptydrive

import (
	"strings"
	"time"
)

// submitCheck is a step's check that the input it typed was submitted. The driver cannot see
// the receiver, so the caller names a file it writes once the typed turn's request arrived
// (File) and, optionally, a screen pattern native only shows after a submission (Screen).
type submitCheck struct {
	Window time.Duration
	// Resend, when set, sends Enter once more if the window passes unconfirmed, then waits one
	// more window. It is only for a measured lost-Enter defect, and it is always recorded.
	Resend bool
}

// submitOutcome is what the check saw: recorded in the driver's output and marks so a case's
// evidence shows whether, how and how fast the input was submitted.
type submitOutcome struct {
	Confirmed bool
	By        string // "file", "screen" or ""
	Resent    bool
	Elapsed   time.Duration
	// ResentAt is when, after the check began, the extra Enter was sent.
	ResentAt time.Duration
}

// awaitSubmit polls seen (which names the confirming signal or returns "") every interval until
// the window passes. With Resend it sends one more Enter when the first window passes unconfirmed
// and waits one more window. now and sleep are injected for tests.
func awaitSubmit(c submitCheck, seen func() string, resend func(), now func() time.Time, sleep func(time.Duration)) submitOutcome {
	const interval = 200 * time.Millisecond
	began := now()
	out := submitOutcome{}
	windowEnd := began.Add(c.Window)
	for {
		if by := seen(); by != "" {
			out.Confirmed, out.By, out.Elapsed = true, by, now().Sub(began)
			return out
		}
		if !now().Before(windowEnd) {
			if c.Resend && !out.Resent {
				out.Resent, out.ResentAt = true, now().Sub(began)
				resend()
				windowEnd = now().Add(c.Window)
				continue
			}
			out.Elapsed = now().Sub(began)
			return out
		}
		sleep(interval)
	}
}

// awaitQuiet returns once size has not changed for quiet, or after max, and reports whether it
// went quiet. It is for typing into a TUI that is still drawing: input sent while it renders can
// be dropped.
func awaitQuiet(size func() int, quiet, max time.Duration, now func() time.Time, sleep func(time.Duration)) bool {
	const interval = 100 * time.Millisecond
	began := now()
	last, lastChange := size(), began
	for {
		if n := size(); n != last {
			last, lastChange = n, now()
		}
		if now().Sub(lastChange) >= quiet {
			return true
		}
		if now().Sub(began) >= max {
			return false
		}
		sleep(interval)
	}
}

// echoOutcome is what the echo gate saw before the next write.
type echoOutcome struct {
	// Seen: the typed text was on the screen. Quiet: and the screen then stood still for the
	// quiet period. A gate that is not both timed out.
	Seen, Quiet bool
	Elapsed     time.Duration
}

// echoed reports whether the screen text (whitespace removed) shows the typed text more often
// than it did before the text was written (before). Counting, not slicing from an earlier
// length: the stripped text's length shifts when an escape sequence split across two reads is
// completed, which moves such an offset past the start of the echo.
func echoed(screen, want string, before int) bool {
	return want != "" && strings.Count(screen, want) > before
}

// awaitEcho returns once seen has held and size has not changed for quiet, or after max. A TUI
// takes typed text and the Enter that follows in separate input reads only when the Enter comes
// after the text was drawn; the gate sends Enter on that evidence, not on a fixed delay.
func awaitEcho(seen func() bool, size func() int, quiet, max time.Duration, now func() time.Time, sleep func(time.Duration)) echoOutcome {
	const interval = 25 * time.Millisecond
	began := now()
	last, lastChange := size(), began
	var out echoOutcome
	for {
		if n := size(); n != last {
			last, lastChange = n, now()
		}
		out.Seen = seen()
		out.Elapsed = now().Sub(began)
		if out.Seen && now().Sub(lastChange) >= quiet {
			out.Quiet = true
			return out
		}
		if out.Elapsed >= max {
			return out
		}
		sleep(interval)
	}
}

func orDefault(v, def int) time.Duration {
	if v <= 0 {
		v = def
	}
	return time.Duration(v) * time.Millisecond
}
