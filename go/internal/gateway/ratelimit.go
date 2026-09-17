package gateway

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// What the backend says about the quota, read off the response headers.
//
// Observation only. This does not authorise spending, estimate what is left, or change any
// request budget -- the ledger does that, from what this build itself approved. A number the
// backend volunteered is a thing to show a user, not a thing to act on.
//
// Measured against the real backend 2026-09-17. All six fields arrive, there is a second
// limit family (x-codex-bengalfox-*), and x-codex-active-limit says which one is in force.
//
// One measured departure from the Node baseline: x-codex-secondary-reset-at arrives *empty*.
// The baseline's regex calls an empty value numeric-format and turns the whole observation
// invalid, so against this backend it would report invalid on every single response. An
// empty header value is absent, not malformed, and a diagnostic that is wrong every time is
// one nobody reads. Here it is absent, and the state comes back partial: five of six.

// Header names. The prefix and the units are the backend's own, cited in the baseline to
// codex-rs/codex-api/src/rate_limits.rs and confirmed on the wire.
const (
	limitPrefix      = "x-codex"
	limitActiveName  = "x-codex-active-limit"
	limitUsedPercent = "used-percent"
	limitWindowMins  = "window-minutes"
	limitResetAt     = "reset-at"
)

// States. A reading that is short of a field is not the same as no reading and not the same
// as a broken one, and collapsing the three would lose the only distinction that matters.
const (
	limitMissing  = "missing"
	limitPartial  = "partial"
	limitObserved = "observed"
	limitInvalid  = "invalid"
)

// Why a reading is invalid.
const (
	reasonDuplicate = "duplicate"
	reasonFormat    = "numeric-format"
	reasonRange     = "numeric-range"
)

// limitValueMax bounds a header value before it is parsed. A quota number is never long and
// a value that is has nothing to do with one.
const limitValueMax = 64

// limitFamilies is how many other limit families are named. The baseline's eight.
const limitFamilies = 8

// limitLabelShape bounds what may be recorded from a header's text. Backend-controlled
// bytes reach a file that outlives the session; the numbers are parsed, and the two labels
// that are not numbers -- the active limit and a family prefix -- are shaped and bounded.
var limitLabelShape = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// RateWindow is one window's three numbers. Absent stays absent: a missing number is
// unknown, and reporting it as zero would turn "not stated" into "none used".
type RateWindow struct {
	UsedPercent   *float64 `json:"usedPercent,omitempty"`
	WindowMinutes *int64   `json:"windowMinutes,omitempty"`
	ResetAt       *int64   `json:"resetAt,omitempty"`
}

func (w RateWindow) stated() int {
	count := 0
	for _, present := range []bool{w.UsedPercent != nil, w.WindowMinutes != nil, w.ResetAt != nil} {
		if present {
			count++
		}
	}
	return count
}

// RateLimitReport is the most recent reading.
//
// The most recent rather than every one: this is a live quota and the newest answer is the
// only one still true.
type RateLimitReport struct {
	State string `json:"state"`
	// ActiveLimit names which family is in force, when the backend says. Without it a
	// reader who sees two families has to guess which number applies to them.
	ActiveLimit   string      `json:"activeLimit,omitempty"`
	Primary       *RateWindow `json:"primary,omitempty"`
	Secondary     *RateWindow `json:"secondary,omitempty"`
	OtherFamilies []string    `json:"otherFamilies,omitempty"`
	InvalidReason string      `json:"invalidReason,omitempty"`
	InvalidField  string      `json:"invalidField,omitempty"`
}

// limitLedger holds the latest reading.
type limitLedger struct {
	mu     sync.Mutex
	latest *RateLimitReport
}

func newLimitLedger() *limitLedger { return &limitLedger{} }

// observe reads one response's headers. A response with none leaves the last reading alone:
// an answer that arrived is better than no answer, and a transport with no headers at all
// has not contradicted it.
func (l *limitLedger) observe(header http.Header) {
	if len(header) == 0 {
		return
	}
	report := readLimits(header)
	if report.State == limitMissing {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.latest = &report
}

func (l *limitLedger) report() *RateLimitReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.latest
}

// readLimits turns one set of response headers into a reading.
func readLimits(header http.Header) RateLimitReport {
	report := RateLimitReport{State: limitMissing}

	if active := header.Values(limitActiveName); len(active) == 1 {
		if name := strings.TrimSpace(active[0]); limitLabelShape.MatchString(name) {
			report.ActiveLimit = name
		}
	}

	stated := 0
	for _, window := range []string{"primary", "secondary"} {
		read, reason, field := readWindow(header, limitPrefix, window)
		if reason != "" && report.InvalidReason == "" {
			report.InvalidReason, report.InvalidField = reason, field
		}
		stated += read.stated()
		if read.stated() == 0 {
			continue
		}
		if window == "primary" {
			report.Primary = &read
		} else {
			report.Secondary = &read
		}
	}

	report.OtherFamilies = otherFamilies(header)

	switch {
	case report.InvalidReason != "":
		report.State = limitInvalid
	case stated == 6:
		report.State = limitObserved
	case stated > 0:
		report.State = limitPartial
	case report.ActiveLimit != "" || len(report.OtherFamilies) > 0:
		// Something about the quota arrived even though none of the six numbers did.
		report.State = limitPartial
	}
	return report
}

// readWindow reads one window's three numbers.
func readWindow(header http.Header, prefix, window string) (RateWindow, string, string) {
	var read RateWindow
	for _, field := range []string{limitUsedPercent, limitWindowMins, limitResetAt} {
		name := prefix + "-" + window + "-" + field
		values := header.Values(name)
		switch {
		case len(values) == 0:
			continue
		case len(values) > 1:
			// Two answers to one question. Neither is taken: a reader who sees a number
			// should not have to wonder which of two the backend meant.
			return read, reasonDuplicate, window + "." + field
		}
		raw := strings.TrimSpace(values[0])
		if raw == "" {
			// Measured: the backend sends this for secondary reset-at. Not stated is not
			// malformed, and treating it as malformed makes every response invalid.
			continue
		}
		if len(raw) > limitValueMax {
			return read, reasonFormat, window + "." + field
		}
		switch field {
		case limitUsedPercent:
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return read, reasonFormat, window + "." + field
			}
			if value < 0 || value > 100 {
				return read, reasonRange, window + "." + field
			}
			read.UsedPercent = &value
		case limitWindowMins:
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return read, reasonFormat, window + "." + field
			}
			if value < 0 {
				return read, reasonRange, window + "." + field
			}
			read.WindowMinutes = &value
		default:
			// reset-at is signed upstream. A negative one is not a reset in the future,
			// but it is a number the backend sent and is reported as it arrived.
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return read, reasonFormat, window + "." + field
			}
			read.ResetAt = &value
		}
	}
	return read, "", ""
}

// otherFamilies names limits that are not this one.
//
// Measured: x-codex-bengalfox-* is a real second family. Named only -- no plan, balance or
// promotional text is read, and the name is shaped and bounded before it is kept.
func otherFamilies(header http.Header) []string {
	const suffix = "-primary-" + limitUsedPercent
	var found []string
	for name := range header {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "x-") || !strings.HasSuffix(lower, suffix) {
			continue
		}
		family := strings.TrimSuffix(lower, suffix)
		if family == limitPrefix {
			continue
		}
		if !limitLabelShape.MatchString(strings.TrimPrefix(family, "x-")) {
			continue
		}
		found = append(found, family)
	}
	sort.Strings(found)
	if len(found) > limitFamilies {
		found = found[:limitFamilies]
	}
	return found
}
