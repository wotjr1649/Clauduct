package httpguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServeFramesParserRefusalsOnNewAndReusedConnections(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closing, cancel := context.WithCancel(context.Background())
	type callerContext struct{}
	var active atomic.Int64
	server := &http.Server{
		MaxHeaderBytes: 1024,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Context().Value(callerContext{}) != true {
				t.Error("caller's connection context was lost")
			}
			io.WriteString(w, "PUBLIC_REPLY")
		}),
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			return context.WithValue(ctx, callerContext{}, true)
		},
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateActive {
				active.Add(1)
			}
		},
	}
	done := make(chan error, 1)
	go func() { done <- Serve(server, listener, closing, 32<<20) }()
	t.Cleanup(func() {
		cancel()
		server.Close()
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	})
	wantActive := int64(0)
	for _, tc := range []struct {
		name, header string
		status       int
	}{
		{"invalid-header", "Bad Header: public", 400},
		// Exceed net/http's read allowance plus already-buffered keep-alive
		// bytes. This exercises its 431 response, not an exact header-size cap.
		{"oversized-header", "X-Public: " + strings.Repeat("x", 32<<10), 431},
	} {
		for _, reuse := range []bool{false, true} {
			t.Run(tc.name+"/"+fmt.Sprint("reuse=", reuse), func(t *testing.T) {
				wantActive++
				conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				reader := bufio.NewReader(conn)
				if reuse {
					wantActive++
					if _, err := fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", listener.Addr()); err != nil {
						t.Fatal(err)
					}
					response, err := http.ReadResponse(reader, nil)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || string(body) != "PUBLIC_REPLY" || response.Close {
						t.Fatal("handler response or keep-alive changed")
					}
				}
				if _, err := fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n%s\r\nConnection: close\r\n\r\n", listener.Addr(), tc.header); err != nil {
					t.Fatal(err)
				}
				response, err := http.ReadResponse(reader, nil)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != tc.status || !strings.HasPrefix(string(body), fmt.Sprint(tc.status)+" ") || response.ContentLength != int64(len(body)) {
					t.Fatalf("parser refusal lost framing: status=%d length=%d bytes=%d error=%v", response.StatusCode, response.ContentLength, len(body), err)
				}
			})
		}
	}
	if active.Load() != wantActive {
		t.Fatal("caller's connection state callback was lost")
	}
}

func TestResponseCloseDrainIsBoundedAndInterruptible(t *testing.T) {
	for _, forced := range []bool{false, true} {
		t.Run(fmt.Sprint("forced=", forced), func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			peer, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			socket, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			closing, stopClose := context.WithCancel(context.Background())
			defer stopClose()
			c := &responseConn{Conn: socket, closing: closing, maxDrain: 32 << 20}
			c.finished.Store(true)
			done := make(chan error, 1)
			go func() { done <- c.Close() }()
			select {
			case <-done:
				t.Fatal("closed before allowing the silent peer to finish")
			case <-time.After(20 * time.Millisecond):
			}
			if forced {
				stopClose()
				select {
				case <-done:
				case <-time.After(50 * time.Millisecond):
					t.Fatal("gateway shutdown did not interrupt the pending drain")
				}
				forcedDone := make(chan error, 1)
				go func() { forcedDone <- c.Close() }()
				select {
				case <-forcedDone:
				case <-time.After(50 * time.Millisecond):
					t.Fatal("forced close waited for the normal drain")
				}
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("silent peer held the closing socket")
			}
		})
	}
}

// --- the abort watcher ------------------------------------------------------------------

// The watcher must not expire a deadline on a connection the handler has finished with.
func TestTheAbortWatcherLeavesAFinishedConnectionAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fired := 0
	stop := WatchReadCancellation(ctx, func() { fired++ })
	stop()
	cancel()
	if fired != 0 {
		t.Fatal("the watcher changed the deadline after the handler finished")
	}
}

// And it must still do its job, which is the mutation that matters: deleting the watcher
// also makes the test above pass.
func TestTheAbortWatcherStillStopsAReadThatTheClientAbandoned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fired := make(chan struct{})
	stop := WatchReadCancellation(ctx, func() { close(fired) })
	defer stop()

	cancel()
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("a client that went away mid-body must expire the read deadline; without " +
			"that the handler sits in the read and shutdown waits for it")
	}
}
