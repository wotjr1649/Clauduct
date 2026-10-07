package ptydrive

import "time"

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
