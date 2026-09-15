// Package gateway owns the ephemeral loopback listener the native client talks to.
//
// WP01 scope: bind, hand out a session token, answer readiness, and refuse everything
// else with a fixed code. There is no /v1/messages here yet and no upstream at all, so
// this package cannot make a network request even by mistake.
package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Gateway is one session's listener. Two concurrent sessions share nothing: separate
// listener, separate port, separate token.
type Gateway struct {
	listener net.Listener
	server   *http.Server
	token    string
	served   chan error

	closeOnce sync.Once
	closeErr  error
}

// Start binds 127.0.0.1 on a port the operating system chooses and begins serving.
//
// The port comes from bind, never from a search. Probing for a free port and closing it
// before rebinding opens a window where another process takes it; asking for :0 and
// keeping the listener has no such window. The caller passes the resulting address to the
// child, so there is never a guess about which port is live.
func Start() (*Gateway, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		listener.Close()
		return nil, err
	}
	g := &Gateway{listener: listener, token: token, served: make(chan error, 1)}
	g.server = &http.Server{
		Handler: http.HandlerFunc(g.handle),
		// Header deadline only. A global WriteTimeout would eventually cut a long
		// streaming response, and this server is going to carry exactly that. The
		// per-phase deadlines that replace it belong with the streaming code.
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		err := g.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		g.served <- err
	}()
	return g, nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Addr is the live bound address, host:port.
func (g *Gateway) Addr() string { return g.listener.Addr().String() }

// BaseURL is what the child's ANTHROPIC_BASE_URL is set to.
func (g *Gateway) BaseURL() string { return "http://" + g.Addr() }

// Token is the session bearer token. It is never logged, never placed on a command line,
// and never written to an error string.
func (g *Gateway) Token() string { return g.token }

func (g *Gateway) handle(w http.ResponseWriter, r *http.Request) {
	// A browser attaches Origin; the native client does not. Refusing it costs nothing and
	// closes the drive-by path where a page on localhost talks to this port.
	if r.Header.Get("Origin") != "" {
		refuse(w, http.StatusForbidden, "ORIGIN_REFUSED")
		return
	}
	if !loopbackHost(r.Host) {
		refuse(w, http.StatusForbidden, "HOST_REFUSED")
		return
	}
	// Readiness answers without a credential on purpose, so it stays usable before the
	// child has anything, and therefore it must reveal nothing: no body, no token, no port
	// beyond the one the caller already dialled, no account, no upstream state.
	if r.Method == http.MethodHead && r.URL.Path == "/api/hello" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Everything else is genuinely not implemented yet. Saying so with a fixed code is the
	// point: a silent 200 would let a caller believe a route works, and forwarding an
	// unknown path upstream would be an open relay.
	refuse(w, http.StatusNotFound, "UNSUPPORTED_ROUTE")
}

func refuse(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Fixed strings only. Nothing from the request is echoed, so a crafted path or header
	// cannot place attacker bytes into a response another tool might read.
	w.Write([]byte(`{"error":{"type":"` + code + `"}}`))
}

func loopbackHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.Trim(name, "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// Close stops accepting, waits for in-flight handlers up to the context deadline, and
// releases the listener. It reports a cleanup failure as its own error so a caller can
// keep it separate from whatever the native process did.
//
// Idempotent, and it has to be: shutdown can be reached from a normal exit and from a
// cancellation path at the same time, and the serve result can only be received once. A
// second caller reading an already-drained channel would hang forever holding up the very
// cleanup it was trying to perform.
func (g *Gateway) Close(ctx context.Context) error {
	g.closeOnce.Do(func() {
		shutdownErr := g.server.Shutdown(ctx)
		serveErr := <-g.served
		if shutdownErr != nil {
			g.closeErr = shutdownErr
			return
		}
		g.closeErr = serveErr
	})
	return g.closeErr
}
