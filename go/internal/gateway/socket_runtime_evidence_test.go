//go:build runtime_evidence

package gateway

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Retain the original single-connection FIN delivery assertion independently of
// the product's reconnect/no-duplicate acceptance test. A failure here remains
// a failed transport diagnostic; reconnection must not relabel it as success.
func TestRuntimeEvidenceImmediateFINReply(t *testing.T) {
	if os.Getenv("CLAUDUCT_SOCKET_EVIDENCE") != "1" {
		t.Skip("explicit socket probe switch absent")
	}
	for attempt := 0; attempt < 20; attempt++ {
		t.Run(fmt.Sprint(attempt), func(t *testing.T) {
			g := start(t)
			conn, err := net.DialTimeout("tcp4", g.Addr(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := fmt.Fprintf(conn, "HEAD /api/hello HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", g.Addr()); err != nil {
				t.Fatal(err)
			}
			if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Logf("gateway handler requests=%d", g.received.Load())
				t.Fatal("protocol response lost", err)
			}
			defer response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatal("readiness response changed", response.StatusCode)
			}
			n, err := io.Copy(io.Discard, response.Body)
			if err != nil {
				t.Fatal("protocol response truncated", err)
			}
			if response.ContentLength != n {
				t.Fatal("response relies on connection close")
			}
			waitForActive(t, g, 0, "immediate FIN")
		})
	}
}

// A raw loopback transport probe, not an HTTP/product success claim. Nothing
// changes the installed filter. No credentials or external destination are used.
func TestRuntimeEvidenceSocketClose(t *testing.T) {
	testSocketEvidence(t, []string{"immediate", "receiver_ack"})
}

func TestRuntimeEvidenceSocketShutdown(t *testing.T) {
	testSocketEvidence(t, []string{"explicit_shutdown"})
}

func testSocketEvidence(t *testing.T, modes []string) {
	t.Helper()
	if os.Getenv("CLAUDUCT_SOCKET_EVIDENCE") != "1" {
		t.Skip("explicit socket probe switch absent")
	}
	payload := bytes.Repeat([]byte("PUBLIC_SOCKET_PAYLOAD\n"), 2048)
	for _, mode := range modes {
		ack := mode != "immediate"
		graceful := mode == "explicit_shutdown"
		t.Run(mode, func(t *testing.T) {
			failures := 0
			clientErrors, serverErrors := map[string]int{}, map[string]int{}
			shortPayloads := 0
			deadline := time.Now().Add(40 * time.Second)
			for i := 0; i < 200; i++ {
				if time.Now().After(deadline) {
					t.Fatal("probe deadline")
				}
				trace := socketEvidenceTrace{start: time.Now()}
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal("listen")
				}
				done := make(chan error, 1)
				closed := make(chan struct{})
				go func() {
					defer close(closed)
					conn, e := listener.Accept()
					if e != nil {
						done <- e
						return
					}
					trace.record("server.accept", 0, nil)
					defer func() {
						trace.record("server.close.begin", 0, nil)
						e := conn.Close()
						trace.record("server.close.end", 0, e)
					}()
					_ = conn.SetDeadline(time.Now().Add(time.Second))
					trace.record("server.write.begin", 0, nil)
					n, e := io.Copy(conn, bytes.NewReader(payload))
					trace.record("server.write.end", int(n), e)
					if e == nil && graceful {
						trace.record("server.shutdown.begin", 0, nil)
						e = conn.(*net.TCPConn).CloseWrite()
						trace.record("server.shutdown.end", 0, e)
					}
					if e == nil && ack {
						var b [1]byte
						trace.record("server.ack.read.begin", 0, nil)
						var n int
						n, e = io.ReadFull(conn, b[:])
						trace.record("server.ack.read.end", n, e)
						if e == nil && (n != 1 || b[0] != 1) {
							e = errors.New("invalid acknowledgement")
						}
					}
					if e == nil && graceful {
						e = socketEvidenceEOF(conn, &trace, "server")
					}
					done <- e
				}()
				conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
				if err != nil {
					listener.Close()
					<-done
					<-closed
					t.Fatal("dial")
				}
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				trace.record("client.dial.end", 0, nil)
				trace.record("client.read.begin", 0, nil)
				var received []byte
				if ack {
					received = make([]byte, len(payload))
					var n int
					n, err = io.ReadFull(conn, received)
					trace.record("client.read.end", n, err)
					if err == nil {
						trace.record("client.ack.write.begin", 0, nil)
						n, err = conn.Write([]byte{1})
						trace.record("client.ack.write.end", n, err)
					}
				} else {
					received, err = io.ReadAll(io.LimitReader(conn, int64(len(payload)+1)))
					trace.record("client.read.end", len(received), err)
				}
				if err == nil && graceful {
					trace.record("client.shutdown.begin", 0, nil)
					err = conn.(*net.TCPConn).CloseWrite()
					trace.record("client.shutdown.end", 0, err)
					if err == nil {
						err = socketEvidenceEOF(conn, &trace, "client")
					}
				}
				trace.record("client.close.begin", 0, nil)
				trace.record("client.close.end", 0, conn.Close())
				trace.record("listener.close.begin", 0, nil)
				trace.record("listener.close.end", 0, listener.Close())
				serverErr := <-done
				<-closed // Account for the deferred close before the next connection.
				if err != nil || serverErr != nil || !bytes.Equal(received, payload) {
					failures++
					clientErrors[socketEvidenceError(err)]++
					serverErrors[socketEvidenceError(serverErr)]++
					if !bytes.Equal(received, payload) {
						shortPayloads++
					}
					if failures <= 3 {
						sort.SliceStable(trace.events, func(i, j int) bool { return trace.events[i].At < trace.events[j].At })
						t.Logf("iteration=%d failure_trace=%v", i, trace.events)
					}
				}
			}
			t.Logf("attempts=200 failures=%d", failures)
			t.Logf("client_errors=%v server_errors=%v payload_mismatches=%d", clientErrors, serverErrors, shortPayloads)
			if failures != 0 {
				t.Error("socket close counterexample reproduced")
			}
		})
	}
}

// The diagnostic control requires a successful EOF at both ends, not just a
// successful Write/Close. It does not replace either original close probe.
func socketEvidenceEOF(conn net.Conn, trace *socketEvidenceTrace, side string) error {
	var b [1]byte
	trace.record(side+".eof.read.begin", 0, nil)
	n, err := conn.Read(b[:])
	trace.record(side+".eof.read.end", n, err)
	if n != 0 || err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("expected EOF")
	}
	return nil
}

// Record only fixed event names, relative time, byte counts and error classes.
// Sorting after both sockets close avoids claiming scheduling order from a lock.
// Equal clock ticks do not establish an order between goroutines.
type socketEvidenceTrace struct {
	start  time.Time
	mu     sync.Mutex
	events []socketEvidenceEvent
}

type socketEvidenceEvent struct {
	At    time.Duration
	Event string
	Bytes int
	Error string
}

func (s *socketEvidenceTrace) record(event string, n int, err error) {
	e := socketEvidenceEvent{time.Since(s.start), event, n, socketEvidenceError(err)}
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func socketEvidenceError(err error) string {
	if err == nil {
		return "none"
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return fmt.Sprintf("errno_%d", code)
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	if errors.Is(err, io.EOF) {
		return "eof"
	}
	return "other"
}
