package httpclose

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRawTCP(t *testing.T) {
	for _, kind := range []string{"plain", "halfclose", "linger"} {
		t.Run(kind, func(t *testing.T) {
			failures := 0
			for i := 0; i < 200; i++ {
				l, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				sent := make(chan error, 1)
				go func() {
					c, e := l.Accept()
					if e != nil {
						sent <- e
						return
					}
					defer c.Close()
					var b [6]byte
					_, e = io.ReadFull(c, b[:])
					if e != nil {
						sent <- e
						return
					}
					_, e = io.WriteString(c, "public reply\n")
					if kind == "halfclose" {
						c.(*net.TCPConn).CloseWrite()
					}
					if kind == "linger" {
						c.(*net.TCPConn).SetLinger(1)
					}
					sent <- e
				}()
				c, e := net.DialTimeout("tcp", l.Addr().String(), time.Second)
				if e != nil {
					t.Fatal(e)
				}
				c.SetDeadline(time.Now().Add(time.Second))
				io.WriteString(c, "public")
				line, e := bufio.NewReader(c).ReadString('\n')
				if e != nil || line != "public reply\n" {
					failures++
					if failures < 4 {
						t.Logf("iteration=%d read_bytes=%d error=%v", i, len(line), e)
					}
				}
				if e := <-sent; e != nil {
					t.Fatal(e)
				}
				c.Close()
				l.Close()
			}
			t.Logf("stalled=%d/200", failures)
			if failures != 0 {
				t.Fail()
			}
		})
	}
}

// Public, loopback-only reproducer: no Clauduct imports or request data.
type gracefulListener struct{ net.Listener }

func (l gracefulListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	if e = c.(*net.TCPConn).SetLinger(1); e != nil {
		c.Close()
		return nil, e
	}
	return c, nil
}

func TestCancelledResponse(t *testing.T) {
	for _, name := range []string{"plain_close", "linger"} {
		t.Run(name, func(t *testing.T) {
			failures := 0
			for i := 0; i < 200; i++ {
				entered, release := make(chan struct{}), make(chan struct{})
				written := make(chan string, 1)
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				if name == "linger" {
					listener = gracefulListener{listener}
				}
				srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					io.Copy(io.Discard, r.Body)
					close(entered)
					<-release
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(499)
					n, e := io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"CANCELLED"}}`)
					flushErr := http.NewResponseController(w).Flush()
					written <- fmt.Sprintf("write=%d,%v flush=%v", n, e, flushErr)
				})}
				served := make(chan error, 1)
				go func() { served <- srv.Serve(listener) }()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String(), strings.NewReader("public"))
				done := make(chan error, 1)
				go func() {
					if name == "raw_client" {
						c, e := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
						if e != nil {
							done <- e
							return
						}
						defer c.Close()
						c.SetDeadline(time.Now().Add(time.Second))
						fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: 6\r\n\r\npublic", listener.Addr())
						_, e = io.Copy(io.Discard, c)
						done <- e
						return
					}
					resp, err := http.DefaultClient.Do(req)
					if err == nil {
						_, err = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					done <- err
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					cancel()
					srv.Close()
					t.Fatal("request did not enter")
				}
				time.Sleep(time.Millisecond)
				close(release)
				closeCtx, stop := context.WithTimeout(context.Background(), time.Second)
				err = srv.Shutdown(closeCtx)
				stop()
				if err != nil {
					srv.Close()
					cancel()
					t.Fatal(err)
				}
				<-served
				select {
				case <-done:
				case <-time.After(200 * time.Millisecond):
					failures++
					t.Logf("stalled iteration=%d %s", i, <-written)
					cancel()
					<-done
				}
				cancel()
			}
			t.Logf("stalled_clients=%d/200", failures)
			if failures != 0 {
				t.Fail()
			}
		})
	}
}
