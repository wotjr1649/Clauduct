package upstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Disposition is what may be done about a failed attempt.
//
// The three are separate because they lead to different actions, and collapsing any two
// would either waste a request or hide a permanent failure behind a retry loop.
type Disposition int

const (
	// Terminal: retrying cannot help. A policy refusal, a certificate that does not
	// verify, a malformed request.
	Terminal Disposition = iota
	// Retryable: the same request could succeed. Subject to the budget, not to optimism.
	Retryable
	// Deferred: the server named a time. The delay is reported rather than slept through,
	// because a caller that has other work to do should not be blocked, and a caller that
	// waits should wait the amount the server asked for rather than one this bridge chose.
	Deferred
)

func (d Disposition) String() string {
	switch d {
	case Retryable:
		return "retryable"
	case Deferred:
		return "deferred"
	}
	return "terminal"
}

// Failure classifies why an attempt did not produce a usable response.
type Failure struct {
	Category    string
	Disposition Disposition
	// RetryAfter is the delay the server asked for, when it named one. Zero means it did
	// not, not that it asked for none.
	RetryAfter time.Duration
	// RetryAt is when the delay expires, computed once from the clock at classification
	// so a later reading does not shorten it.
	RetryAt time.Time
}

func (f Failure) Error() string { return f.Category }

// MaxGatewayRetries is how many times this bridge retries an attempt on its own.
//
// Zero, and measured rather than chosen for caution. The installed claude 2.1.272 retries
// every 5xx itself — eight requests in sixty seconds against a listener that kept
// answering 503. A retry here would multiply with the client's, so one inference could
// cost far more attempts than either side intended and a budget stated in attempts would
// not bound what the user is billed for.
//
// Raising this requires establishing who owns retrying, which is a decision with the
// client's behaviour on one side of it, not a constant to tune.
const MaxGatewayRetries = 0

// ClassifyStatus decides what a response status means for retrying.
//
// now is supplied so the deferred deadline is computed from one reading of the clock.
func ClassifyStatus(status int, header http.Header, now time.Time) Failure {
	switch {
	case status == http.StatusUnauthorized:
		// The credential may have been refreshed by the user's own tool between attempts,
		// so a single re-read of the same account is worth one more try. Following it to
		// a different account, or looping, is not.
		return Failure{Category: "UNAUTHENTICATED", Disposition: Retryable}

	case status == http.StatusTooManyRequests:
		return withRetryAfter(Failure{Category: "RATE_LIMITED"}, header, now)

	case status == http.StatusRequestTimeout, status == http.StatusConflict:
		return Failure{Category: "UPSTREAM_HTTP_ERROR", Disposition: Retryable}

	case status >= 500:
		return withRetryAfter(Failure{Category: "UPSTREAM_HTTP_ERROR"}, header, now)

	case status >= 400:
		// The request itself was not acceptable, or was refused. Sending it again produces
		// the same answer and spends another attempt to learn nothing — and for a 403 in
		// particular, working around the refusal would be defeating a control rather than
		// handling a failure.
		return Failure{Category: "UPSTREAM_HTTP_ERROR", Disposition: Terminal}
	}
	return Failure{Category: "UPSTREAM_HTTP_ERROR", Disposition: Terminal}
}

func withRetryAfter(failure Failure, header http.Header, now time.Time) Failure {
	failure.Disposition = Retryable
	delay, ok := ParseRetryAfter(header.Get("Retry-After"), now)
	if !ok {
		return failure
	}
	failure.RetryAfter = delay
	failure.RetryAt = now.Add(delay)
	failure.Disposition = Deferred
	return failure
}

// ParseRetryAfter reads the header in either of its forms.
//
// A delay outside a representable range is not the same as a malformed header: the server
// named a time this bridge cannot schedule, which forbids a retry rather than permitting
// an immediate one. Both return false here, and the caller treats an unparsed header as
// "no delay named" rather than as "no delay needed" — the Retryable disposition already
// says a retry is permitted, and nothing shortens a delay the server asked for.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return 0, false
	}

	// Delta-seconds. Checked before the date forms because a bare number is not a date.
	// ParseInt handles leading zeros, so "007" is seven rather than a syntax question.
	if isDigits(value) {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds > int64(maxRetryAfterSeconds) {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	// The three HTTP-date forms, parsed by the standard library rather than by three
	// regular expressions of this package's own.
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := when.Sub(now)
	if delay < 0 {
		// A time already past is a request to retry now, not an invalid header.
		return 0, true
	}
	if delay > time.Duration(maxRetryAfterSeconds)*time.Second {
		return 0, false
	}
	return delay, true
}

// The longest delay this bridge will represent. A server asking for longer is reported as
// unschedulable rather than rounded down, because rounding a delay down is retrying early.
const maxRetryAfterSeconds = 24 * 60 * 60

func isDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return len(value) > 0
}

// ClassifyTransport decides what a connection-level error means.
//
// A certificate that does not verify is terminal and stays terminal. Retrying it would not
// help, and the only thing that would — relaxing verification — is the one response this
// build never makes.
func ClassifyTransport(err error) Failure {
	switch {
	case err == nil:
		return Failure{Category: "", Disposition: Terminal}

	case errors.Is(err, context.Canceled):
		// The caller stopped asking. Retrying would answer a question nobody has.
		return Failure{Category: "CANCELLED", Disposition: Terminal}

	case errors.Is(err, context.DeadlineExceeded):
		return Failure{Category: "REQUEST_TIMEOUT", Disposition: Terminal}
	}

	var certErr *tls.CertificateVerificationError
	var hostErr x509.HostnameError
	var authErr x509.UnknownAuthorityError
	var invalidErr x509.CertificateInvalidError
	if errors.As(err, &certErr) || errors.As(err, &hostErr) ||
		errors.As(err, &authErr) || errors.As(err, &invalidErr) {
		return Failure{Category: "TLS_VERIFICATION_FAILED", Disposition: Terminal}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			// The name does not exist. It will not exist on the next attempt either.
			return Failure{Category: "DNS_NOT_FOUND", Disposition: Terminal}
		}
		return Failure{Category: "DNS_FAILURE", Disposition: Retryable}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Failure{Category: "CONNECTION_TIMEOUT", Disposition: Retryable}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return Failure{Category: "CONNECTION_FAILED", Disposition: Retryable}
	}
	return Failure{Category: "TRANSPORT_FAILED", Disposition: Terminal}
}
