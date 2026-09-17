package app

import (
	"fmt"
	"io"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// WriteUsage prints what this account has spent, from the backend's own accounting.
//
// It lives here rather than in either command because both ask the same question:
// `clauduct --usage` and `clauduct-dev usage` are the same view, and a second copy of it
// would drift.
//
// It exists at all because the client cannot show this. Measured against a controlled
// listener: the client's own /usage asks an Anthropic account endpoint, and against a
// gateway base URL it skips the request entirely -- under either credential shape. So the
// plan half of that command is blank here whatever this build does at the wire, while the
// figures arrive as headers on every response and each session already writes them down.
//
// No request is made. The reading comes from the account a session left behind and its age
// is printed beside it: a cached number presented as current is worse than none.
//
// dir empty means the directory sessions write to.
func WriteUsage(dir string, out io.Writer) int {
	reading, ok := LatestQuota(dir)
	if !ok {
		fmt.Fprintln(out, "no reading yet.")
		fmt.Fprintln(out, "The quota arrives as headers on a backend response, so it is known")
		fmt.Fprintln(out, "after a session has asked the backend something. Run one and look again.")
		return 1
	}

	limits := reading.Status.Gateway.Limits
	fmt.Fprintln(out, "quota    ", describeState(limits.State))
	if limits.ActiveLimit != "" {
		fmt.Fprintln(out, "in force ", limits.ActiveLimit)
	}
	writeWindow(out, "primary  ", limits.Primary)
	writeWindow(out, "secondary", limits.Secondary)
	for _, family := range limits.OtherFamilies {
		// The name only. A second family means another limit exists; what it says is not
		// something this build has read.
		fmt.Fprintln(out, "also     ", family, "(another family this account has, unread)")
	}
	if limits.InvalidReason != "" {
		fmt.Fprintln(out, "unread   ", limits.InvalidField, limits.InvalidReason)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "as of    ", roughly(reading.Age()), "ago,", reading.Path)

	sessions := Readings(dir, 0)
	attempts, inferences, spent := 0, 0, 0
	for _, session := range sessions {
		attempts += session.Status.Attempts
		inferences += session.Status.Inferences
		if session.Status.Attempts > 0 {
			spent++
		}
	}
	// Sessions that spent nothing are counted apart rather than folded in: most accounts in
	// that directory are test runs, and a session count they dominate says nothing about
	// what was used.
	fmt.Fprintf(out, "local     %d attempts, %d inferences over %s that spent (%d accounts kept)\n",
		attempts, inferences, plural(int64(spent), "session"), len(sessions))
	// The limit is the account's, not this machine's, and a reader comparing the two
	// numbers needs to know they are not the same denominator.
	fmt.Fprintln(out, "          the window above is the account's -- other machines and "+
		"codex itself spend against it too")
	return 0
}

func writeWindow(out io.Writer, label string, window *gateway.RateWindow) {
	if window == nil || window.UsedPercent == nil {
		return
	}
	line := fmt.Sprintf("%s %s used", label, percent(*window.UsedPercent))
	if window.WindowMinutes != nil && *window.WindowMinutes > 0 {
		line += " of a " + windowLength(*window.WindowMinutes) + " window"
	}
	if window.ResetAt != nil && *window.ResetAt > 0 {
		reset := time.Unix(*window.ResetAt, 0)
		if until := time.Until(reset); until > 0 {
			line += ", resets in " + roughly(until)
		} else {
			line += ", reset has passed"
		}
	}
	fmt.Fprintln(out, line)
}

func percent(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d%%", int64(value))
	}
	return fmt.Sprintf("%.1f%%", value)
}

// windowLength says a week rather than 10080 minutes. The number is right either way; one
// of them is the one a person is asking about.
func windowLength(minutes int64) string {
	switch {
	case minutes%(60*24*7) == 0:
		return plural(minutes/(60*24*7), "week")
	case minutes%(60*24) == 0:
		return plural(minutes/(60*24), "day")
	case minutes%60 == 0:
		return plural(minutes/60, "hour")
	default:
		return plural(minutes, "minute")
	}
}

func plural(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// roughly rounds to something a person reads. Precision here would be false anyway: the
// reading is as old as it is.
func roughly(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return plural(int64(d.Minutes()), "minute")
	case d < 48*time.Hour:
		return plural(int64(d.Hours()), "hour")
	default:
		return plural(int64(d.Hours()/24), "day")
	}
}

// describeState turns the observation's state into what it means for the reader.
func describeState(state string) string {
	switch state {
	case "observed":
		return "observed (every field the backend sends was read)"
	case "partial":
		return "partial (the backend left some fields empty)"
	case "missing":
		return "missing (the response carried no limit headers)"
	case "invalid":
		return "invalid (a field arrived in a shape this build does not read)"
	}
	return state
}
