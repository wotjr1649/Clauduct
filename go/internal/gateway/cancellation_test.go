package gateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type interruptedCounter struct {
	upstream.Fixture
	started chan struct{}
}

func TestCancellationNameWithoutRequestCancellationStaysAFailure(t *testing.T) {
	f := &countingFixture{countErr: context.Canceled}
	g := startWith(t, f)
	g.EnableContextPolicy()
	response := countPost(t, g, "gpt-6-astra", "public uncancelled request")
	if body := bodyText(t, response); response.StatusCode != 400 || !strings.Contains(body, "COUNT_TOKENS_FAILED_CANCELLED") {
		t.Fatalf("uncancelled request lost its count failure: status=%d body=%s", response.StatusCode, body)
	}
}

func (f *interruptedCounter) Count(ctx context.Context, _ upstream.Call) (int64, error) {
	close(f.started)
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestCancellationDuringCountIsNotAPIFailure(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages/count_tokens"} {
		t.Run(endpoint, func(t *testing.T) {
			f := &interruptedCounter{started: make(chan struct{})}
			g := startWith(t, f)
			g.EnableContextPolicy()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(validRequest)).WithContext(ctx)
			req.Host = g.Addr()
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Anthropic-Version", anthropicVersion)
			req.Header.Set("Authorization", "Bearer "+g.Token())
			req.Header.Set("X-Claude-Code-Request-Class", "main")
			req.Header.Set("X-Claude-Code-Session-Id", "public-session")
			// count_tokens does not accept generation-only fields.
			if endpoint == "/v1/messages/count_tokens" {
				req.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"ping"}]}`))
			}
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); g.handle(w, req) }()
			select {
			case <-f.started:
			case <-time.After(5 * time.Second):
				t.Fatal("counter did not start")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("cancelled count did not finish")
			}
			failures := g.ring.counts().Failures
			if len(failures) != 1 || failures["CANCELLED"] != 1 || w.Code != 499 || f.Calls() != 0 {
				t.Fatalf("status=%d failures=%v inferenceCalls=%d; want cancellation only", w.Code, failures, f.Calls())
			}
		})
	}
}

func TestCloseCancelsEventBodyReads(t *testing.T) {
	for _, path := range []string{"/clauduct/agents", "/clauduct/context", "/clauduct/workflows", "/clauduct/tool-failures"} {
		t.Run(path, func(t *testing.T) {
			g := start(t)
			g.delegations = &delegations{}
			cancel, done := slowUpload(t, g, path)
			defer func() { cancel(); <-done }()
			deadline := time.Now().Add(time.Second)
			for g.received.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if g.received.Load() == 0 {
				t.Fatal("event request never arrived")
			}
			ctx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			if err := g.Close(ctx); err != nil {
				t.Errorf("shutdown did not cancel the event body read: %v", err)
			}
		})
	}
}

func TestRefusalBoundsUnfinishedBody(t *testing.T) {
	g := start(t)
	conn, err := net.DialTimeout("tcp", g.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	// No credentials: a sender stalled mid-body must receive a complete refusal
	// and cannot hold the server open. The server's drain budget is one second.
	if _, err = fmt.Fprintf(conn, "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nContent-Length: 100\r\n\r\n{", g.Addr()); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("refusal exceeded its body-drain budget: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("incomplete refusal reply: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := g.Close(ctx); err != nil {
		t.Fatalf("refused upload still held the server open: %v", err)
	}
}

type deadlineBarrier struct {
	*httptest.ResponseRecorder
	entered, proceed chan struct{}
	expirations      atomic.Int32
}

func (w *deadlineBarrier) SetReadDeadline(at time.Time) error {
	if !at.IsZero() && !at.After(time.Now()) && w.expirations.Add(1) == 1 {
		close(w.entered)
		<-w.proceed
	}
	return nil
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

// Hold the deadline operation after cancellation, then finish reading the body.
// A handler must join that operation before returning its ResponseWriter to HTTP.
func TestCancellationDeadlineFinishesBeforeHandlerReturns(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		t.Run(endpoint, func(t *testing.T) {
			g := start(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &deadlineBarrier{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), proceed: make(chan struct{})}
			r := httptest.NewRequest(http.MethodPost, endpoint, nil).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Anthropic-Version", anthropicVersion)
			r.Body = io.NopCloser(readFunc(func([]byte) (int, error) {
				cancel()
				<-w.entered
				return 0, context.Canceled
			}))
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				if endpoint == "/v1/messages" {
					g.handleMessages(w, r)
				} else {
					g.handleCountTokens(w, r)
				}
			}()
			select {
			case <-w.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not interrupt the read")
			}
			select {
			case <-returned:
				t.Error("handler returned while its deadline operation was still running")
			case <-time.After(50 * time.Millisecond):
			}
			close(w.proceed)
			select {
			case <-returned:
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not finish after the deadline operation")
			}
			waitForActive(t, g, 0, "cancelled handler must release its admission")
		})
	}
}
