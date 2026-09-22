// Package httpguard preserves framed HTTP responses across connection turnover.
// It uses only standard Go networking APIs and owns no OS or filter settings.
package httpguard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// HTTP/1.1 frames its response before net/http closes this socket. Give a
// closing peer time to consume that response and close first: sending
// FIN immediately can make an installed stream filter discard the final bytes.
// This is a bounded socket drain, not a request retry or a response deadline.
// Match net/http's own half-close grace. The former 100ms bound truncated a
// 64KiB SSE response under concurrent load before the filter delivered its tail.
const responseCloseGrace = 500 * time.Millisecond

// Serve installs connection protection before the server's first Serve call.
// maxDrain bounds discarded bytes when a completed response closes. The caller
// cancels closing before shutdown to interrupt any in-progress drain.
// Existing handler, connection context and state callbacks are preserved.
func Serve(server *http.Server, listener net.Listener, closing context.Context, maxDrain int64) error {
	if maxDrain <= 0 {
		return fmt.Errorf("httpguard: positive drain bound required")
	}
	handler := server.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Context().Value(responseConnKey{}).(*responseConn)
		if c != nil {
			c.finished.Store(false)
			c.handled.Store(true)
		}
		handler.ServeHTTP(w, r)
		if c != nil && r.ProtoMajor == 1 && r.ProtoMinor >= 1 {
			c.finished.Store(r.Context().Err() == nil)
		}
	})
	connContext, connState := server.ConnContext, server.ConnState
	server.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		if connContext != nil {
			ctx = connContext(ctx, conn)
		}
		return context.WithValue(ctx, responseConnKey{}, conn)
	}
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		if c, ok := conn.(*responseConn); ok && state == http.StateIdle {
			c.handled.Store(false)
		}
		if connState != nil {
			connState(conn, state)
		}
	}
	return server.Serve(responseListener{Listener: listener, closing: closing, maxDrain: maxDrain})
}

type responseConnKey struct{}

type responseListener struct {
	net.Listener
	closing  context.Context
	maxDrain int64
}

func (l responseListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &responseConn{Conn: c, closing: l.closing, maxDrain: l.maxDrain}, nil
}

type responseConn struct {
	net.Conn
	closing  context.Context
	maxDrain int64
	finished atomic.Bool
	handled  atomic.Bool
	once     sync.Once
	closeErr error
}

func (c *responseConn) Write(p []byte) (int, error) {
	wire := p
	insertAt, inserted := 0, 0
	if !c.handled.Load() {
		// Go 1.27 writes a complete parser refusal in one Write, without
		// Content-Length. Frame it so a closing filter cannot turn its EOF
		// delimiter into a truncated HTTP response. Keep status and body intact.
		const end = "\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\n"
		status, body, ok := bytes.Cut(p, []byte(end))
		if ok && (bytes.HasPrefix(status, []byte("HTTP/1.1 4")) || bytes.HasPrefix(status, []byte("HTTP/1.1 5"))) && !bytes.ContainsAny(status, "\r\n") {
			insertAt = len(status)
			wire = fmt.Appendf(bytes.Clone(status), "\r\nContent-Length: %d", len(body))
			inserted = len(wire) - insertAt
			wire = append(wire, p[insertAt:]...)
		}
	}
	n, err := c.Conn.Write(wire)
	if n > insertAt && inserted != 0 {
		n -= min(n-insertAt, inserted)
	}
	if err != nil {
		c.finished.Store(false)
	} else if n > 0 && !c.handled.Load() {
		// Only parser refusals complete here. Handler cancellation must keep
		// the handler's own unfinished state, even if buffered bytes flush.
		c.finished.Store(true)
	}
	return n, err
}

func (c *responseConn) Close() error {
	// Forced gateway shutdown must also interrupt a drain already in progress.
	if c.closing.Err() != nil {
		return c.Conn.Close()
	}
	c.once.Do(func() {
		c.drain()
		c.closeErr = c.Conn.Close()
	})
	return c.closeErr
}

func (c *responseConn) drain() {
	if !c.finished.Load() || c.closing.Err() != nil {
		return
	}
	if c.Conn.SetReadDeadline(time.Now().Add(responseCloseGrace)) == nil {
		stop := WatchReadCancellation(c.closing, func() { _ = c.Conn.SetReadDeadline(time.Now()) })
		defer stop()
		// Also consume a refused body, without executing another request or
		// waiting indefinitely for a client that keeps writing.
		_, _ = io.CopyN(io.Discard, c.Conn, c.maxDrain)
	}
}

// WatchReadCancellation expires a read when ctx ends. Its returned stop function
// joins any running update before the caller releases/reuses the response writer.
func WatchReadCancellation(ctx context.Context, expire func()) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		expire()
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}
