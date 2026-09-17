package upstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

var base = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func header(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}

// LIFE08: what a status means for retrying. 403 and 400 are decisions, not transients;
// sending them again spends an attempt to receive the same answer.
func TestStatusDisposition(t *testing.T) {
	for name, tc := range map[string]struct {
		status   int
		category string
		want     Disposition
	}{
		"401 may be a refreshed credential": {401, "UNAUTHENTICATED", Retryable},
		"403 is a policy decision":          {403, "UPSTREAM_HTTP_ERROR", Terminal},
		"400 is a bad request":              {400, "UPSTREAM_HTTP_ERROR", Terminal},
		"404 is a bad request":              {404, "UPSTREAM_HTTP_ERROR", Terminal},
		"413 is a bad request":              {413, "UPSTREAM_HTTP_ERROR", Terminal},
		"408 is a transient":                {408, "UPSTREAM_HTTP_ERROR", Retryable},
		"409 is a transient":                {409, "UPSTREAM_HTTP_ERROR", Retryable},
		"429 is rate limiting":              {429, "RATE_LIMITED", Retryable},
		"500 is a server fault":             {500, "UPSTREAM_HTTP_ERROR", Retryable},
		"502 is a server fault":             {502, "UPSTREAM_HTTP_ERROR", Retryable},
		"503 is a server fault":             {503, "UPSTREAM_HTTP_ERROR", Retryable},
		"504 is a server fault":             {504, "UPSTREAM_HTTP_ERROR", Retryable},
		"a 3xx that got this far":           {304, "UPSTREAM_HTTP_ERROR", Terminal},
	} {
		t.Run(name, func(t *testing.T) {
			got := ClassifyStatus(tc.status, header(), base)
			if got.Category != tc.category {
				t.Fatalf("category = %s, want %s", got.Category, tc.category)
			}
			if got.Disposition != tc.want {
				t.Fatalf("disposition = %s, want %s", got.Disposition, tc.want)
			}
			if got.RetryAfter != 0 || !got.RetryAt.IsZero() {
				t.Fatalf("a response with no Retry-After reported a delay of %v at %v",
					got.RetryAfter, got.RetryAt)
			}
		})
	}
}

// LIFE09: a named delay is deferred, and the deadline is computed from one reading of the
// clock so a later reading cannot shorten it.
func TestANamedDelayIsDeferredAndDatedOnce(t *testing.T) {
	for name, status := range map[string]int{"429": 429, "503": 503} {
		t.Run(name, func(t *testing.T) {
			got := ClassifyStatus(status, header("Retry-After", "120"), base)
			if got.Disposition != Deferred {
				t.Fatalf("disposition = %s, want deferred", got.Disposition)
			}
			if got.RetryAfter != 2*time.Minute {
				t.Fatalf("RetryAfter = %v, want 2m", got.RetryAfter)
			}
			if !got.RetryAt.Equal(base.Add(2 * time.Minute)) {
				t.Fatalf("RetryAt = %v, want %v", got.RetryAt, base.Add(2*time.Minute))
			}
		})
	}
}

// LIFE09: a header the server did name but this bridge cannot schedule must not become an
// immediate retry. Unparsed means "no delay named", and the disposition stays retryable
// without a deadline rather than becoming a retry right now with one of zero.
func TestAnUnschedulableDelayDoesNotBecomeAnImmediateRetry(t *testing.T) {
	for name, value := range map[string]string{
		"longer than a day":  "90000",
		"a date past a day":  base.Add(48 * time.Hour).Format(http.TimeFormat),
		"not a number":       "soon",
		"negative":           "-30",
		"fractional":         "1.5",
		"absurdly long text": string(make([]byte, 200)),
		"empty":              "",
	} {
		t.Run(name, func(t *testing.T) {
			if delay, ok := ParseRetryAfter(value, base); ok {
				t.Fatalf("ParseRetryAfter(%q) = %v, true; want it refused", value, delay)
			}
			got := ClassifyStatus(429, header("Retry-After", value), base)
			if got.Disposition != Retryable {
				t.Fatalf("disposition = %s, want retryable", got.Disposition)
			}
			if got.RetryAfter != 0 || !got.RetryAt.IsZero() {
				t.Fatalf("an unschedulable header produced a deadline of %v at %v",
					got.RetryAfter, got.RetryAt)
			}
		})
	}
}

func TestRetryAfterFormsThatAreUnderstood(t *testing.T) {
	for name, tc := range map[string]struct {
		value string
		want  time.Duration
	}{
		"delta seconds":         {"30", 30 * time.Second},
		"zero":                  {"0", 0},
		"leading zeros":         {"007", 7 * time.Second},
		"surrounding space":     {"  45  ", 45 * time.Second},
		"exactly the ceiling":   {"86400", 24 * time.Hour},
		"an RFC1123 date":       {base.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		"an ANSI C date":        {base.Add(time.Minute).Format(time.ANSIC), time.Minute},
		"a date already past":   {base.Add(-time.Hour).Format(http.TimeFormat), 0},
		"a date exactly at now": {base.Format(http.TimeFormat), 0},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ParseRetryAfter(tc.value, base)
			if !ok {
				t.Fatalf("ParseRetryAfter(%q) was refused", tc.value)
			}
			if got != tc.want {
				t.Fatalf("ParseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// LIFE13: connection-level failures. A certificate that does not verify is terminal and
// stays terminal — the only thing that would make it succeed is relaxing verification.
func TestTransportErrorClassification(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		category string
		want     Disposition
	}{
		"certificate verification": {
			&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
			"TLS_VERIFICATION_FAILED", Terminal},
		"unknown authority": {
			x509.UnknownAuthorityError{}, "TLS_VERIFICATION_FAILED", Terminal},
		"wrong hostname": {
			x509.HostnameError{Host: "elsewhere.example"}, "TLS_VERIFICATION_FAILED", Terminal},
		"expired certificate": {
			x509.CertificateInvalidError{Reason: x509.Expired}, "TLS_VERIFICATION_FAILED", Terminal},
		"a name that does not exist": {
			&net.DNSError{Err: "no such host", IsNotFound: true}, "DNS_NOT_FOUND", Terminal},
		"a resolver that failed": {
			&net.DNSError{Err: "server misbehaving", IsTemporary: true}, "DNS_FAILURE", Retryable},
		"a connection refused": {
			&net.OpError{Op: "dial", Err: errors.New("connection refused")}, "CONNECTION_FAILED", Retryable},
		"a timeout": {
			&net.OpError{Op: "dial", Err: timeoutError{}}, "CONNECTION_TIMEOUT", Retryable},
		"the caller gave up": {
			context.Canceled, "CANCELLED", Terminal},
		"the deadline passed": {
			context.DeadlineExceeded, "REQUEST_TIMEOUT", Terminal},
		"something else entirely": {
			errors.New("unrecognised"), "TRANSPORT_FAILED", Terminal},
	} {
		t.Run(name, func(t *testing.T) {
			got := ClassifyTransport(tc.err)
			if got.Category != tc.category {
				t.Fatalf("category = %s, want %s", got.Category, tc.category)
			}
			if got.Disposition != tc.want {
				t.Fatalf("disposition = %s, want %s", got.Disposition, tc.want)
			}
		})
	}
}

// The errors arrive wrapped by net/http rather than bare, so classification has to see
// through the wrapping it will actually meet.
func TestClassificationSeesThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf(`Post "%s": %w`, Endpoint,
		&url_error{Err: x509.UnknownAuthorityError{}})
	if got := ClassifyTransport(wrapped); got.Category != "TLS_VERIFICATION_FAILED" {
		t.Fatalf("a wrapped certificate failure classified as %s", got.Category)
	}
}

// LIFE10: this bridge does not retry on its own. The installed client retries every 5xx
// itself, and a retry here would multiply with it.
func TestThisBridgeDoesNotRetryOnItsOwn(t *testing.T) {
	if MaxGatewayRetries != 0 {
		t.Fatalf("MaxGatewayRetries = %d. Raising it needs a decision about who owns "+
			"retrying, because the installed client already retries every 5xx.",
			MaxGatewayRetries)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type url_error struct{ Err error }

func (u *url_error) Error() string { return u.Err.Error() }
func (u *url_error) Unwrap() error { return u.Err }

// A category says what kind of failure it was; the number says which one. Two failures in
// the same category can need different answers, and the probe's whole purpose is telling a
// 400 from a 429.
func TestTheStatusIsCarriedOnTheRefusal(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 409, 413, 429, 500, 502, 503, 504} {
		got := ClassifyStatus(status, header(), base)
		if got.Status != status {
			t.Fatalf("ClassifyStatus(%d).Status = %d, want %d", status, got.Status, status)
		}
	}
	// A named delay must not lose it either.
	if got := ClassifyStatus(429, header("Retry-After", "30"), base); got.Status != 429 {
		t.Fatalf("a deferred failure reported status %d, want 429", got.Status)
	}
	// A connection-level failure has no status, and zero must mean "none" rather than a
	// number someone might compare against.
	if got := ClassifyTransport(context.Canceled); got.Status != 0 {
		t.Fatalf("a transport failure reported status %d, want 0", got.Status)
	}
}
