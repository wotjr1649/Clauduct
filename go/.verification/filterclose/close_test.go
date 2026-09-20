package filterclose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Public loopback-only comparison. A correct body is checked separately from EOF.
func TestFilteredHTTPShutdown(t *testing.T) {
	for _, mode := range []string{"chunked", "length", "chunked_linger", "length_linger", "length_receipt", "length_shutdown_drain"} {
		t.Run(mode, func(t *testing.T) {
			const body = `{"type":"error","error":{"type":"invalid_request_error","message":"CANCELLED"}}`
			failures, slow := 0, 0
			shapes := map[string]int{}
			start := time.Now()
			for i := 0; i < 200; i++ {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal("listen")
				}
				if strings.HasSuffix(mode, "_linger") {
					listener = lingerListener{listener}
				}
				if mode == "length_shutdown_drain" {
					listener = drainListener{listener}
				}
				entered, release := make(chan struct{}), make(chan struct{})
				server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					io.Copy(io.Discard, r.Body)
					close(entered)
					<-release
					w.Header().Set("Content-Type", "application/json")
					if strings.HasPrefix(mode, "length") {
						w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					}
					w.WriteHeader(499)
					io.WriteString(w, body)
					http.NewResponseController(w).Flush()
				})}
				served := make(chan struct{})
				go func() { server.Serve(listener); close(served) }()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String(), strings.NewReader("public"))
				transport := &http.Transport{Proxy: nil}
				client := &http.Client{Transport: transport}
				done := make(chan string, 1)
				go func() {
					resp, e := client.Do(req)
					if e != nil {
						done <- "request_error=" + errorShape(e)
						return
					}
					b, e := io.ReadAll(resp.Body)
					resp.Body.Close()
					if e == nil && resp.StatusCode == 499 && string(b) == body {
						done <- "ok"
					} else {
						done <- fmt.Sprintf("status=%d bytes=%d read_error=%T matched=%t", resp.StatusCode, len(b), e, string(b) == body)
					}
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					cancel()
					server.Close()
					<-served
					transport.CloseIdleConnections()
					t.Fatal("request not received")
				}
				releasedAt := time.Now()
				close(release)
				received := ""
				if mode == "length_receipt" {
					// Independent control: finish consuming the response before
					// closing the origin, analogous to the owned client's exit.
					received = <-done // client context above bounds this to one second
					if time.Since(releasedAt) > 200*time.Millisecond {
						slow++
					}
				}
				closing, stop := context.WithTimeout(context.Background(), time.Second)
				err = server.Shutdown(closing)
				stop()
				if err != nil {
					server.Close()
				}
				<-served
				if received != "" {
					shapes[received]++
					if received != "ok" {
						failures++
					}
				} else {
					select {
					case shape := <-done:
						shapes[shape]++
						if shape != "ok" {
							failures++
						}
					case <-time.After(200 * time.Millisecond):
						slow++
						cancel()
						shape := <-done
						shapes[shape]++
						if shape != "ok" {
							failures++
						}
					}
				}
				cancel()
				transport.CloseIdleConnections()
				if err != nil {
					t.Fatal("shutdown")
				}
			}
			t.Logf("cases=200 failures=%d slow=%d elapsed_ms=%d", failures, slow, time.Since(start).Milliseconds())
			t.Logf("shapes=%v", shapes)
			if failures != 0 || slow != 0 {
				t.Fail()
			}
		})
	}
}

func errorShape(err error) string {
	parts := []string{}
	for i := 0; err != nil && i < 6; i++ {
		label := fmt.Sprintf("%T", err)
		if err == io.EOF {
			label = "EOF"
		}
		if err == io.ErrUnexpectedEOF {
			label = "unexpected_EOF"
		}
		if err == context.DeadlineExceeded {
			label = "deadline"
		}
		if code, ok := err.(syscall.Errno); ok {
			label = fmt.Sprintf("errno_%d", code)
		}
		parts = append(parts, label)
		err = errors.Unwrap(err)
	}
	return strings.Join(parts, "/")
}

type lingerListener struct{ net.Listener }

func (l lingerListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e == nil {
		e = c.(*net.TCPConn).SetLinger(1)
		if e != nil {
			c.Close()
		}
	}
	return c, e
}

// Unlike CloseWrite immediately followed by Close, drain peer input until EOF
// or the deadline. Neither event proves the peer application consumed our body.
type drainListener struct{ net.Listener }
type drainConn struct {
	*net.TCPConn
	once sync.Once
	err  error
}

func (l drainListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &drainConn{TCPConn: c.(*net.TCPConn)}, nil
}

func (c *drainConn) Close() error {
	c.once.Do(func() {
		c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		c.CloseWrite()
		io.Copy(io.Discard, c.TCPConn)
		c.err = c.TCPConn.Close()
	})
	return c.err
}
