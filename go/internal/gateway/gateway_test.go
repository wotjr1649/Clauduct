package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func start(t *testing.T) *Gateway { return startWith(t, nil) }

func startWith(t *testing.T, transport upstream.Transport) *Gateway {
	t.Helper()
	g, err := Start(transport)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		g.Close(ctx)
	})
	return g
}

type request struct {
	method  string
	path    string
	headers map[string]string
	body    io.Reader
	noAuth  bool
}

func do(t *testing.T, g *Gateway, rq request) *http.Response {
	t.Helper()
	req, err := http.NewRequest(rq.method, g.BaseURL()+rq.path, rq.body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if !rq.noAuth {
		req.Header.Set("Authorization", "Bearer "+g.Token())
	}
	for k, v := range rq.headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do %s %s: %v", rq.method, rq.path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func bodyText(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(raw)
}

// messages is a request shaped like the ones a real client sends. The version header is
// not decoration: measured 2026-09-16, the installed client puts it on every request, and a
// helper that leaves it off would be testing a client nobody runs.
func messages(body io.Reader) request {
	return request{
		method: http.MethodPost,
		path:   "/v1/messages?beta=true",
		headers: map[string]string{
			"Content-Type":      "application/json",
			"Anthropic-Version": anthropicVersion,
		},
		body: body,
	}
}

func waitForActive(t *testing.T, g *Gateway, want int64, why string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, active := g.Stats(); active == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, _, active := g.Stats()
	t.Fatalf("active = %d, want %d (%s)", active, want, why)
}

// --- bind and readiness -------------------------------------------------------------------

// HTTP01: loopback, OS-chosen port, accepting before anyone is told about it.
func TestBindsLoopbackEphemeralPort(t *testing.T) {
	g := start(t)

	host, port, err := net.SplitHostPort(g.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", g.Addr(), err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("bound to %q, want 127.0.0.1; a gateway on all interfaces is reachable off-machine", host)
	}
	if port == "0" {
		t.Fatalf("port still 0; the child would be told an address it cannot dial")
	}
}

// Measured against claude 2.1.272: readiness arrives as HEAD with no Authorization at all,
// and the Node baseline answers 204 with keep-alive. Both are contract, not preference.
func TestReadinessMatchesTheMeasuredClient(t *testing.T) {
	g := start(t)
	resp := do(t, g, request{method: http.MethodHead, path: "/api/hello", noAuth: true})

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if body := bodyText(t, resp); body != "" {
		t.Errorf("body = %q, want empty; readiness answers without a credential so it must reveal nothing", body)
	}
	for name, values := range resp.Header {
		if strings.Contains(strings.Join(values, " "), g.Token()) {
			t.Fatalf("session token appeared in header %s", name)
		}
	}
}

// Readiness does not require a credential, but a wrong one is never quietly accepted.
func TestReadinessRefusesAWrongCredential(t *testing.T) {
	g := start(t)
	resp := do(t, g, request{
		method:  http.MethodHead,
		path:    "/api/hello",
		noAuth:  true,
		headers: map[string]string{"Authorization": "Bearer not-the-session-token"},
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// --- HTTP03: token -------------------------------------------------------------------------

func TestAuthenticationRefusals(t *testing.T) {
	g := start(t)
	other := start(t)

	for _, tc := range []struct{ name, header string }{
		{"missing", ""},
		{"empty bearer", "Bearer "},
		{"wrong token", "Bearer " + strings.Repeat("a", len(g.Token()))},
		{"no scheme", g.Token()},
		{"wrong scheme", "Basic " + g.Token()},
		{"lowercase scheme", "bearer " + g.Token()},
		// Not "Bearer <token> ": RFC 7230 optional whitespace is not part of a field
		// value and net/http strips it, so that case asserts the transport, not this code.
		{"double space after scheme", "Bearer  " + g.Token()},
		{"token with a suffix", "Bearer " + g.Token() + "x"},
		{"another session's token", "Bearer " + other.Token()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rq := messages(strings.NewReader("{}"))
			rq.noAuth = true
			if tc.header != "" {
				rq.headers["Authorization"] = tc.header
			}
			resp := do(t, g, rq)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if body := bodyText(t, resp); !strings.Contains(body, "LOCAL_SESSION_REQUIRED") {
				t.Fatalf("body = %q, want the fixed category", body)
			}
		})
	}
}

// The route is authenticated and its body is decoded. A decode refusal is the proof that
// the boundary, the credential, the method and the media type all passed first.
func TestValidTokenReachesTheDecoder(t *testing.T) {
	g := start(t)
	resp := do(t, g, messages(strings.NewReader(`{"model":"x"}`)))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 from the request decoder", resp.StatusCode)
	}
	if body := bodyText(t, resp); !strings.Contains(body, "REQUEST_STREAM_MISSING") {
		t.Fatalf("body = %q, want the decoder's own category", body)
	}
}

// --- HTTP06, HTTP07: more than one credential header ----------------------------------------

// Forward compatibility, measured as not-yet-needed: claude 2.1.272 sends exactly one
// Authorization header and no x-api-key. A future version presenting the same session
// token in both must not have discovery broken over it.
func TestSameTokenInBothHeadersIsAccepted(t *testing.T) {
	g := start(t)
	rq := messages(strings.NewReader("{}"))
	rq.headers["X-Api-Key"] = g.Token()

	resp := do(t, g, rq)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want the decoder to be reached; the same token twice is still one credential", resp.StatusCode)
	}
}

// HTTP07: a second header carrying anything else is a foreign credential.
func TestDifferentCredentialInSecondHeaderIsRefused(t *testing.T) {
	g := start(t)
	other := start(t)

	for name, value := range map[string]string{
		"X-Api-Key with a foreign key":   "sk-ant-not-this-session",
		"X-Api-Key with another session": other.Token(),
		"X-Api-Key with a bearer prefix": "Bearer " + g.Token(),
	} {
		t.Run(name, func(t *testing.T) {
			rq := messages(strings.NewReader("{}"))
			rq.headers["X-Api-Key"] = value
			resp := do(t, g, rq)
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", resp.StatusCode)
			}
			if body := bodyText(t, resp); !strings.Contains(body, "UNEXPECTED_CREDENTIAL_SOURCE") {
				t.Fatalf("body = %q", body)
			}
		})
	}
}

func TestCookieAndProxyAuthorizationAreRefused(t *testing.T) {
	g := start(t)
	for _, name := range []string{"Cookie", "Proxy-Authorization"} {
		rq := messages(strings.NewReader("{}"))
		rq.headers[name] = "anything"
		resp := do(t, g, rq)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, resp.StatusCode)
		}
	}
}

// --- boundary --------------------------------------------------------------------------------

// DNS rebinding sends a loopback request carrying an attacker's Host. The comparison is to
// the exact address this session bound, not to "something loopback-ish".
func TestForeignHostRefused(t *testing.T) {
	g := start(t)
	for _, host := range []string{"evil.example", "localhost:1", "127.0.0.1", "[::1]:80"} {
		rq := messages(strings.NewReader("{}"))
		rq.headers["Host"] = host
		resp := do(t, g, rq)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Host %q: status = %d, want 403", host, resp.StatusCode)
		}
	}
}

func TestProxyAndBrowserHeadersRefused(t *testing.T) {
	g := start(t)
	for _, name := range []string{"Origin", "Sec-Fetch-Site", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host"} {
		rq := messages(strings.NewReader("{}"))
		rq.headers[name] = "http://evil.example"
		resp := do(t, g, rq)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, resp.StatusCode)
		}
		if body := bodyText(t, resp); !strings.Contains(body, "LOCAL_BOUNDARY_REJECTED") {
			t.Errorf("%s: body = %q", name, body)
		}
	}
}

// A repeated header name is where request smuggling hides a second value behind the one a
// reader checks. The measured client repeats nothing.
func TestDuplicateHeaderNameRefused(t *testing.T) {
	g := start(t)
	conn, err := net.Dial("tcp", g.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	raw := "POST /v1/messages HTTP/1.1\r\n" +
		"Host: " + g.Addr() + "\r\n" +
		"Authorization: Bearer " + g.Token() + "\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 2\r\n" +
		"\r\n{}"
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, 256)
	n, _ := conn.Read(got)
	if !strings.Contains(string(got[:n]), "400") {
		t.Fatalf("response = %q, want a 400 for the duplicated header name", got[:n])
	}
}

// --- HTTP04: method, media type, payload ------------------------------------------------------

func TestMethodAndMediaTypeBoundaries(t *testing.T) {
	g := start(t)

	resp := do(t, g, request{method: http.MethodGet, path: "/v1/messages"})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /v1/messages = %d, want 405", resp.StatusCode)
	}

	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded", "application/jsonx"} {
		rq := messages(strings.NewReader("{}"))
		rq.headers["Content-Type"] = contentType
		resp := do(t, g, rq)
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q = %d, want 415", contentType, resp.StatusCode)
		}
	}

	// A charset parameter is ordinary and must not be treated as a different media type.
	rq := messages(strings.NewReader("{}"))
	rq.headers["Content-Type"] = "application/json; charset=utf-8"
	if resp := do(t, g, rq); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("application/json; charset=utf-8 = %d, want the decoder to be reached", resp.StatusCode)
	}
}

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

// The ceiling is enforced by reading, not by trusting Content-Length. A measured "ping"
// body is already 119 KB, so the limit has to be far above that and still be a limit.
func TestPayloadCeilingIsEnforced(t *testing.T) {
	g := start(t)

	resp := do(t, g, messages(io.LimitReader(endlessReader{}, maxRequestBytes+1024)))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if body := bodyText(t, resp); !strings.Contains(body, "INPUT_TOO_LARGE") {
		t.Fatalf("body = %q", body)
	}
}

// --- HTTP05: query ------------------------------------------------------------------------------

// Measured: the client sends ?beta=true. Deciding what a beta means is protocol work, but
// the query must not make the request unroutable in the meantime.
func TestQueryStringDoesNotChangeRouting(t *testing.T) {
	g := start(t)
	for _, path := range []string{
		"/v1/messages",
		"/v1/messages?beta=true",
		"/v1/messages?beta=true&unknown=1",
		"/v1/messages?",
	} {
		rq := messages(strings.NewReader("{}"))
		rq.path = path
		resp := do(t, g, rq)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want the decoder to be reached", path, resp.StatusCode)
		}
	}
}

// --- routes ---------------------------------------------------------------------------------------

func TestUnknownRoutesRefuseExplicitly(t *testing.T) {
	g := start(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/clauduct/status"}, // the account is read, never written
		{http.MethodGet, "/api/hello"},        // readiness is HEAD only, matching the baseline
		{http.MethodGet, "/"},
		{http.MethodPost, "/v1/messages/count_tokens"},
	} {
		resp := do(t, g, request{method: tc.method, path: tc.path})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.path, resp.StatusCode)
		}
		if body := bodyText(t, resp); !strings.Contains(body, "UNSUPPORTED_ROUTE") {
			t.Errorf("%s %s body = %q", tc.method, tc.path, body)
		}
	}
}

// A refusal must not echo the request. Otherwise a crafted path places attacker bytes into
// a response some other tool may read.
func TestRefusalDoesNotEchoRequest(t *testing.T) {
	g := start(t)
	const marker = "ATTACKER-CONTROLLED-MARKER"
	resp := do(t, g, request{
		method:  http.MethodGet,
		path:    "/" + marker + "?q=" + marker,
		headers: map[string]string{"X-Probe": marker},
	})
	if body := bodyText(t, resp); strings.Contains(body, marker) {
		t.Fatalf("refusal echoed request content: %q", body)
	}
}

// --- HTTP02: isolation ---------------------------------------------------------------------------------

func TestConcurrentSessionsAreIsolated(t *testing.T) {
	first, second := start(t), start(t)

	if first.Addr() == second.Addr() {
		t.Fatalf("two sessions bound the same address %s", first.Addr())
	}
	if first.Token() == second.Token() {
		t.Fatalf("two sessions share a token")
	}
	if len(first.Token()) < 32 {
		t.Fatalf("token is %d chars; too short to resist guessing", len(first.Token()))
	}

	// The registries are separate too: work on one must not appear in the other's counts.
	do(t, first, messages(strings.NewReader("{}")))
	if _, _, active := second.Stats(); active != 0 {
		t.Errorf("second session reports %d active after work on the first", active)
	}
}

func TestNotReachableOffLoopback(t *testing.T) {
	g := start(t)
	_, port, _ := net.SplitHostPort(g.Addr())

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("no interface list: %v", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
			continue
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ipNet.IP.String(), port), 2*time.Second)
		if err == nil {
			conn.Close()
			t.Fatalf("gateway answered on %s; it must bind loopback only", ipNet.IP)
		}
	}
}

// --- lifecycle --------------------------------------------------------------------------------------------

// slowUpload starts a request whose body never finishes, so there is a genuine in-flight
// request to cancel. Reading the body is production work, not a hook added for the test.
func slowUpload(t *testing.T, g *Gateway) (cancel func(), done chan struct{}) {
	t.Helper()
	reader, writer := io.Pipe()
	ctx, cancelCtx := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL()+"/v1/messages", reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.Token())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", anthropicVersion)

	done = make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	go func() { writer.Write([]byte(`{"messages":[`)) }()

	return func() { cancelCtx(); writer.Close() }, done
}

// The HTTP wiring of LIFE06: an abandoned request is cancelled and leaves the registry.
// Sibling isolation itself is proved deterministically in the registry tests; what this
// adds is that the handler is actually registered and actually released.
func TestAbandonedRequestIsCancelledAndDrains(t *testing.T) {
	g := start(t)

	cancel, done := slowUpload(t, g)
	waitForActive(t, g, 1, "the slow upload should be registered while its body is read")

	cancel()
	<-done
	waitForActive(t, g, 0, "an abandoned request must leave the registry")
}

// Shutdown cancels what it owns rather than waiting for it. A bare Shutdown would block on
// a request stuck reading until the deadline expired.
func TestCloseCancelsInFlightRatherThanWaiting(t *testing.T) {
	g, err := Start(nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := g.Addr()

	cancel, done := slowUpload(t, g)
	defer func() { cancel(); <-done }()
	waitForActive(t, g, 1, "the slow upload should be in flight before Close")

	ctx, cancelCtx := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCtx()
	started := time.Now()
	if err := g.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("Close took %v; it waited for the stuck request instead of cancelling it", elapsed)
	}
	if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
		conn.Close()
		t.Fatalf("%s still accepting after Close", addr)
	}
}

func TestCloseIsIdempotentAndReleasesThePort(t *testing.T) {
	g, err := Start(nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := g.Addr()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := g.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
		conn.Close()
		t.Fatalf("%s still accepting after Close", addr)
	}
}

// The account records which client ran, and records nothing else from the header.
//
// This exists because the wire rules here were measured against one client version and the
// client updates itself. Refusing an unmeasured version would be broken by design; saying
// nothing about it leaves a session that broke after an update with no way to say so. The
// middle is to write the version down.
func TestTheClientVersionIsRecordedOnce(t *testing.T) {
	g := start(t)

	// The readiness probe names Bun, which is not a client version and must not become one.
	do(t, g, request{method: http.MethodHead, path: "/api/hello", noAuth: true,
		headers: map[string]string{"User-Agent": "Bun/1.4.3"}})
	if version := g.ClientVersion(); version != "" {
		t.Fatalf("version = %q after a request that named no client", version)
	}

	// The measured strings, not invented ones. /v1/models arrives under one product name
	// and /v1/messages under another with a suffix, and a pattern written from either alone
	// matches nothing in half the session.
	do(t, g, request{method: http.MethodGet, path: "/v1/models",
		headers: map[string]string{"User-Agent": "claude-code/2.1.274"}})
	if version := g.ClientVersion(); version != "2.1.274" {
		t.Fatalf("version = %q, want 2.1.274 from claude-code/", version)
	}

	// A second, different value does not replace the first: a session has one client, and a
	// changing answer is one this account cannot explain.
	do(t, g, request{method: http.MethodGet, path: "/v1/models",
		headers: map[string]string{"User-Agent": "claude-cli/9.9.9 (external, sdk-cli)"}})
	if version := g.ClientVersion(); version != "2.1.274" {
		t.Fatalf("version = %q after a second client named itself", version)
	}

	report := g.Diagnose().Client
	if report.Reference != ReferenceClient || report.Verified != (report.Version == ReferenceClient) {
		t.Fatalf("report = %+v", report)
	}
}

// The suffixed form the conversation actually uses is matched too.
func TestTheSuffixedClientAgentIsRead(t *testing.T) {
	g := start(t)
	do(t, g, request{method: http.MethodGet, path: "/v1/models",
		headers: map[string]string{"User-Agent": "claude-cli/2.1.274 (external, sdk-cli)"}})
	if version := g.ClientVersion(); version != "2.1.274" {
		t.Fatalf("version = %q, want 2.1.274 from the suffixed agent", version)
	}
}
