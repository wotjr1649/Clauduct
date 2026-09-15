package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func start(t *testing.T) *Gateway {
	t.Helper()
	g, err := Start()
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

func do(t *testing.T, g *Gateway, method, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, g.BaseURL()+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// HTTP01: the listener is on loopback, on an OS-chosen port, and is actually accepting
// before anyone is told about it.
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
	if resp := do(t, g, http.MethodHead, "/api/hello", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("readiness returned %d before the child was started", resp.StatusCode)
	}
}

// The listener must not be reachable from another interface on this machine.
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

// Readiness answers without a credential, so it has to be empty. A body here would be a
// place for account or upstream state to leak to anything that can reach the port.
func TestReadinessRevealsNothing(t *testing.T) {
	g := start(t)
	resp := do(t, g, http.MethodHead, "/api/hello", nil)

	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Fatalf("readiness body = %q, want empty", body)
	}
	for name, values := range resp.Header {
		joined := strings.Join(values, " ")
		if strings.Contains(joined, g.Token()) {
			t.Fatalf("session token appeared in header %s", name)
		}
	}
}

// A browser attaches Origin. The native client does not, so refusing it costs nothing.
func TestBrowserOriginRefused(t *testing.T) {
	g := start(t)
	resp := do(t, g, http.MethodHead, "/api/hello", map[string]string{"Origin": "http://evil.example"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Origin request returned %d, want 403", resp.StatusCode)
	}
}

// DNS rebinding sends a loopback request carrying an attacker's Host.
func TestNonLoopbackHostRefused(t *testing.T) {
	g := start(t)
	resp := do(t, g, http.MethodHead, "/api/hello", map[string]string{"Host": "evil.example"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host returned %d, want 403", resp.StatusCode)
	}
}

// Nothing else is implemented yet and the gateway has to say so. A silent 200 would let a
// caller believe a route works; forwarding an unknown path would make this an open relay.
func TestUnimplementedRoutesRefuseExplicitly(t *testing.T) {
	g := start(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/messages"},
		{http.MethodGet, "/v1/models"},
		{http.MethodPost, "/clauduct/agents"},
		{http.MethodGet, "/api/hello"}, // readiness is HEAD only, matching the baseline
		{http.MethodGet, "/"},
		{http.MethodGet, "/../etc/passwd"},
	} {
		resp := do(t, g, tc.method, tc.path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s returned %d, want 404", tc.method, tc.path, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "UNSUPPORTED_ROUTE") {
			t.Errorf("%s %s body = %q, want a fixed UNSUPPORTED_ROUTE code", tc.method, tc.path, body)
		}
	}
}

// A refusal must not echo the request back. Otherwise a crafted path places attacker bytes
// into a response some other tool may read.
func TestRefusalDoesNotEchoRequest(t *testing.T) {
	g := start(t)
	const marker = "ATTACKER-CONTROLLED-MARKER"
	resp := do(t, g, http.MethodGet, "/"+marker, map[string]string{"X-Probe": marker})
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), marker) {
		t.Fatalf("refusal echoed request content: %q", body)
	}
}

// HTTP02: two sessions share nothing. Same-token reuse across sessions is a separate test
// once a route actually checks the token; what must hold now is that they are different.
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
}

// LIFE03: after Close the port is released, not merely unreferenced.
func TestCloseReleasesThePort(t *testing.T) {
	g, err := Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := g.Addr()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := g.Close(ctx); err == nil {
		t.Log("second Close returned nil; idempotent close is acceptable")
	}

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("%s still accepting after Close", addr)
	}
}
