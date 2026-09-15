package upstream

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
)

const testToken = "eyJhbGciOiJub25lIn0.eyJleHAiOjk5OTk5OTk5OTl9.SYNTHETIC-NOT-VERIFIED"

// credentialStore writes a Codex home holding one credential and returns a provider for it.
// synthetic marks whether the provider claims its credentials are test ones.
func credentialStore(t *testing.T, synthetic bool) *auth.Provider {
	t.Helper()
	home := t.TempDir()
	body, _ := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens":    map[string]any{"access_token": testToken, "account_id": "acct_test_1"},
	})
	if err := os.WriteFile(filepath.Join(home, "auth.json"), body, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	return &auth.Provider{Home: home, Environ: map[string]string{}, Synthetic: synthetic}
}

// listener is an HTTP server that counts what actually arrived. Counting requests at the
// server is the only way to establish that nothing was sent: asserting on the client's
// return value says what it reported, not what it did.
type listener struct {
	*httptest.Server
	hits    atomic.Int64
	last    atomic.Value // http.Header
	body    atomic.Value // string
	status  int
	headers http.Header
	payload string
}

func serve(t *testing.T, l *listener) *listener {
	t.Helper()
	if l.status == 0 {
		l.status = http.StatusOK
	}
	l.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.hits.Add(1)
		l.last.Store(r.Header.Clone())
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		l.body.Store(string(raw))
		for name, values := range l.headers {
			w.Header()[name] = values
		}
		w.WriteHeader(l.status)
		io.WriteString(w, l.payload)
	}))
	t.Cleanup(l.Server.Close)
	return l
}

func (l *listener) header(name string) string {
	if h, ok := l.last.Load().(http.Header); ok {
		return h.Get(name)
	}
	return ""
}

// direct builds a transport aimed at a local listener rather than at the real endpoint.
func direct(t *testing.T, l *listener, p *auth.Provider, budget Budget) *Direct {
	t.Helper()
	d := NewDirect(p, NewLedger(budget), "0.48.0", "gpt-5.6-luna", "low")
	if l != nil {
		d.endpoint = l.URL
	}
	return d
}

// REL12: with no budget, nothing is sent. The evidence is the server's own count, not the
// error this returned.
func TestAnUnauthorisedBudgetOpensNoSocket(t *testing.T) {
	l := serve(t, &listener{})
	d := direct(t, l, credentialStore(t, true), Budget{})

	if _, err := d.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Execute = %v, want %v", err, ErrBudgetExhausted)
	}
	if n := l.hits.Load(); n != 0 {
		t.Fatalf("a zero budget produced %d requests, want 0", n)
	}
}

// REL12 and LIFE08: the cap bounds requests that reach the backend, not requests this
// bridge admits to. Past the cap the server stops seeing them.
func TestTheCapBoundsWhatReachesTheBackend(t *testing.T) {
	l := serve(t, &listener{payload: "data: {}\n\n"})
	d := direct(t, l, credentialStore(t, false), Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 2})

	for i := 0; i < 2; i++ {
		response, err := d.Execute(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
		response.Body.Close()
	}
	if _, err := d.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("the third attempt = %v, want %v", err, ErrBudgetExhausted)
	}
	if n := l.hits.Load(); n != 2 {
		t.Fatalf("the backend saw %d requests under a cap of 2", n)
	}
}

// The budget is consulted before the credential is read. A request nobody authorised must
// not cost a credential read, let alone a socket.
func TestTheBudgetIsCheckedBeforeAnythingIsRead(t *testing.T) {
	var reads atomic.Int64
	provider := credentialStore(t, true)
	provider.ReadFile = func(path string) ([]byte, error) {
		reads.Add(1)
		return os.ReadFile(path)
	}
	d := direct(t, serve(t, &listener{}), provider, Budget{})

	if _, err := d.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Execute = %v, want %v", err, ErrBudgetExhausted)
	}
	if n := reads.Load(); n != 0 {
		t.Fatalf("an unauthorised request read %d credential files, want 0", n)
	}
}

// AUTH06: a synthetic credential must not be sent anywhere. A run that dialled with a test
// credential is either leaking a fixture into production or claiming offline evidence from
// a request that was actually made.
func TestASyntheticCredentialIsNeverSent(t *testing.T) {
	l := serve(t, &listener{})
	d := direct(t, l, credentialStore(t, true), approved())

	_, err := d.Execute(context.Background(), []byte(`{}`))
	if !errors.Is(err, ErrSyntheticMixing) {
		t.Fatalf("Execute = %v, want %v", err, ErrSyntheticMixing)
	}
	if n := l.hits.Load(); n != 0 {
		t.Fatalf("a synthetic credential produced %d requests, want 0", n)
	}
	// And it cost an attempt. The reservation happens before the credential is inspected,
	// which is the order that keeps an unauthorised request from reading anything.
	if attempts, _, _ := d.Ledger.Spent(); attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// A credential that cannot be produced stops the request rather than sending an
// unauthenticated one. An anonymous request is not a degraded request; it is a different
// one, and it would be answered by something other than the user's account.
func TestAnUnreadableCredentialStopsTheRequest(t *testing.T) {
	l := serve(t, &listener{})
	d := direct(t, l, &auth.Provider{Home: t.TempDir(), Environ: map[string]string{}}, approved())

	_, err := d.Execute(context.Background(), []byte(`{}`))
	if got := auth.CategoryOf(err); got != auth.CategoryUnavailable {
		t.Fatalf("Execute = %v (category %q), want %s", err, got, auth.CategoryUnavailable)
	}
	if n := l.hits.Load(); n != 0 {
		t.Fatalf("a missing credential produced %d requests, want 0", n)
	}
}

// The wire identity is the reference client's. A header this gets wrong is a request the
// backend reads as coming from something else.
func TestTheRequestCarriesTheReferenceClientIdentity(t *testing.T) {
	l := serve(t, &listener{payload: "data: {}\n\n"})
	d := direct(t, l, credentialStore(t, false), approved())

	response, err := d.Execute(context.Background(), []byte(`{"model":"gpt-5.6-luna"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	response.Body.Close()

	for name, want := range map[string]string{
		"Authorization":      "Bearer " + testToken,
		"chatgpt-account-id": "acct_test_1",
		"Content-Type":       "application/json",
		"Accept":             "text/event-stream",
		"Accept-Encoding":    "identity",
		"Version":            "0.48.0",
		"User-Agent":         "codex-cli/0.48.0 (Windows; x64)",
		"Originator":         "codex_cli_rs",
		"Openai-Beta":        "responses=experimental",
	} {
		if got := l.header(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if got, _ := l.body.Load().(string); got != `{"model":"gpt-5.6-luna"}` {
		t.Fatalf("body = %q, want the bytes passed to Execute", got)
	}
}

// Asking for no compression is not decoration. Go transparently decompresses a gzip body
// it asked for, and a body this never asked for is one whose decompressed size nothing
// here bounds.
func TestCompressionIsNotRequested(t *testing.T) {
	transport, ok := newClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the client's transport is not an *http.Transport")
	}
	if !transport.DisableCompression {
		t.Fatal("the transport would add its own Accept-Encoding and decompress the reply")
	}
}

// A redirect carrying a credential to an origin named by the response is the thing this
// refuses. The second server must never be dialled.
func TestARedirectIsRefusedRatherThanFollowed(t *testing.T) {
	elsewhere := serve(t, &listener{})
	for name, status := range map[string]int{
		"301": 301, "302": 302, "303": 303, "307": 307, "308": 308,
	} {
		t.Run(name, func(t *testing.T) {
			before := elsewhere.hits.Load()
			l := serve(t, &listener{status: status, headers: http.Header{"Location": {elsewhere.URL}}})
			d := direct(t, l, credentialStore(t, false), approved())

			if _, err := d.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, ErrRedirected) {
				t.Fatalf("Execute = %v, want %v", err, ErrRedirected)
			}
			if n := elsewhere.hits.Load() - before; n != 0 {
				t.Fatalf("the redirect target saw %d requests, want 0", n)
			}
		})
	}
}

// The destination is a constant of this build. Nothing a request, a config file or an
// environment variable says may move where a credential is sent.
func TestTheEndpointIsTheApprovedOne(t *testing.T) {
	if Endpoint != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("Endpoint = %q; moving it sends the user's credential somewhere else", Endpoint)
	}
	if !strings.HasPrefix(Endpoint, "https://") {
		t.Fatalf("Endpoint = %q, which would send a bearer token in clear text", Endpoint)
	}
}

// Certificate verification is never relaxed, on any path. This is the one response this
// build does not make to a TLS failure.
func TestCertificateVerificationIsNeverRelaxed(t *testing.T) {
	transport, ok := newClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the client's transport is not an *http.Transport")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("no TLS configuration; nothing pins the minimum version")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify is set: the transport would accept any certificate")
	}
	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("MinVersion = %#04x, want at least TLS 1.2", transport.TLSClientConfig.MinVersion)
	}
}

// A non-200 is classified rather than handed to the caller as a body to parse. An error
// page read as an event stream is a wire failure reported as a model answer.
func TestANonSuccessStatusIsClassified(t *testing.T) {
	for name, tc := range map[string]struct {
		status      int
		category    string
		disposition Disposition
	}{
		"rate limited": {429, "RATE_LIMITED", Retryable},
		"forbidden":    {403, "UPSTREAM_HTTP_ERROR", Terminal},
		"unauthorised": {401, "UNAUTHENTICATED", Retryable},
		"server fault": {503, "UPSTREAM_HTTP_ERROR", Retryable},
	} {
		t.Run(name, func(t *testing.T) {
			l := serve(t, &listener{status: tc.status, payload: "an error page, not an event stream"})
			d := direct(t, l, credentialStore(t, false), approved())

			_, err := d.Execute(context.Background(), []byte(`{}`))
			var failure Failure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute = %v, want a Failure", err)
			}
			if failure.Category != tc.category || failure.Disposition != tc.disposition {
				t.Fatalf("Failure = %s/%s, want %s/%s",
					failure.Category, failure.Disposition, tc.category, tc.disposition)
			}
		})
	}
}

// A cancelled caller stops the request rather than leaving it running unobserved.
func TestACancelledContextStopsTheRequest(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	d := NewDirect(credentialStore(t, false), NewLedger(approved()), "0.48.0", "gpt-5.6-luna", "low")
	d.endpoint = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := d.Execute(ctx, []byte(`{}`))
	var failure Failure
	if !errors.As(err, &failure) || failure.Category != "CANCELLED" {
		t.Fatalf("Execute = %v, want CANCELLED", err)
	}
}

// A transport without a ledger is a transport without a cap. It refuses rather than
// treating an absent budget as an unlimited one.
func TestATransportWithNoLedgerRefuses(t *testing.T) {
	l := serve(t, &listener{})
	d := &Direct{Credentials: credentialStore(t, false), endpoint: l.URL}

	if _, err := d.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("Execute = %v, want %v", err, ErrBudgetExhausted)
	}
	if n := l.hits.Load(); n != 0 {
		t.Fatalf("a transport with no ledger produced %d requests, want 0", n)
	}
}

// A refusal must not carry the token out with it. Every error this can produce is checked
// against the token that would be the thing worth leaking.
func TestNoRefusalCarriesTheToken(t *testing.T) {
	cases := map[string]func(t *testing.T) error{
		"budget": func(t *testing.T) error {
			d := direct(t, serve(t, &listener{}), credentialStore(t, false), Budget{})
			_, err := d.Execute(context.Background(), []byte(`{}`))
			return err
		},
		"synthetic": func(t *testing.T) error {
			d := direct(t, serve(t, &listener{}), credentialStore(t, true), approved())
			_, err := d.Execute(context.Background(), []byte(`{}`))
			return err
		},
		"status": func(t *testing.T) error {
			l := serve(t, &listener{status: 403, payload: testToken})
			d := direct(t, l, credentialStore(t, false), approved())
			_, err := d.Execute(context.Background(), []byte(`{}`))
			return err
		},
		"redirect": func(t *testing.T) error {
			l := serve(t, &listener{status: 302, headers: http.Header{"Location": {"https://elsewhere.example/"}}})
			d := direct(t, l, credentialStore(t, false), approved())
			_, err := d.Execute(context.Background(), []byte(`{}`))
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := run(t)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("the refusal carries the access token: %v", err)
			}
			// The signature segment alone is enough to look for: a partial token in a log
			// is still a token in a log.
			if strings.Contains(err.Error(), "SYNTHETIC-NOT-VERIFIED") {
				t.Fatalf("the refusal carries part of the access token: %v", err)
			}
		})
	}
}
