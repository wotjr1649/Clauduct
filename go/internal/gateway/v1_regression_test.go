package gateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestHalfClosedClientCanReconnectWithoutDuplicateExecution(t *testing.T) {
	for _, dispatched := range []bool{false, true} {
		t.Run(fmt.Sprint("dispatched=", dispatched), func(t *testing.T) {
			for attempt := 0; attempt < 10; attempt++ {
				t.Run(fmt.Sprint(attempt), func(t *testing.T) {
					f := &interruptedReply{Fixture: upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}, holdFirst: dispatched, entered: make(chan struct{})}
					g := startWith(t, f)
					dir := t.TempDir()
					g.ConfigureNativeEvents(dir)
					putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "interrupted"})
					conn, err := net.DialTimeout("tcp4", g.Addr(), time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(2 * time.Second))
					body := validRequest
					if !dispatched {
						body = body[:len(body)/2]
					}
					_, err = fmt.Fprintf(conn, "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAnthropic-Version: %s\r\nX-Claude-Code-Session-Id: public\r\nX-Claude-Code-Request-Class: main\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", g.Addr(), g.Token(), anthropicVersion, len(validRequest), body)
					if err != nil {
						t.Fatal("request write failed")
					}
					if dispatched {
						select {
						case <-f.entered:
						case <-time.After(time.Second):
							t.Fatal("transport did not start")
						}
					}
					if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
						t.Fatal(err)
					}
					response, readErr := http.ReadResponse(bufio.NewReader(conn), nil)
					if readErr == nil {
						_, readErr = io.Copy(io.Discard, response.Body)
						response.Body.Close()
						if response.StatusCode < 400 {
							t.Fatal("interrupted request reported success")
						}
					}
					conn.Close()
					if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
						t.Fatal("half-close did not release the connection")
					}
					if dispatched {
						// Native publishes turn.complete(error) after its connection
						// fails. This independent receipt must release a socket retained
						// by the local filter without retrying uncertain execution.
						putCancellationReceipt(t, dir, "cancel-interrupted.json", nativeTurnReceipt{Session: "public", Turn: "interrupted", Reason: "error"})
						g.ReconcileNativeCancellations()
					}
					if dispatched {
						waitForActive(t, g, 0, "terminal failed request")
					}
					before := int64(0)
					if dispatched {
						before = 1
					}
					if f.attempts.Load() != before {
						t.Fatal("unexpected first dispatch count")
					}
					rq := messages(strings.NewReader(validRequest))
					rq.headers["X-Claude-Code-Session-Id"] = "public"
					response = do(t, g, rq)
					reply := bodyText(t, response)
					if dispatched {
						// The terminal receipt may refuse during input read, before
						// the spent ledger is reached. Both paths must avoid dispatch.
						blocked := response.StatusCode == 400 && strings.Contains(reply, "NATIVE_REQUEST_REPLAY_BLOCKED")
						cancelled := response.StatusCode == 499 && strings.Contains(reply, "CANCELLED")
						if !blocked && !cancelled {
							t.Fatal("ambiguous execution was retried")
						}
					} else if response.StatusCode != 200 || !strings.Contains(reply, "message_stop") {
						t.Fatal("undispatched input could not reconnect")
					}
					if f.attempts.Load() != 1 {
						t.Fatal("input executed more than once")
					}
					waitForActive(t, g, 0, "replacement connection")
					putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "explicit-follow-up"})
					rq.body = strings.NewReader(validRequest)
					response = do(t, g, rq)
					followUp := bodyText(t, response)
					if response.StatusCode != 200 || !strings.Contains(followUp, "message_stop") || f.attempts.Load() != 2 {
						t.Fatalf("same gateway could not serve the next turn: status=%d calls=%d reply=%s", response.StatusCode, f.attempts.Load(), followUp)
					}
					waitForActive(t, g, 0, "follow-up")
				})
			}
		})
	}
}

func TestReplacementConnectionCancelsUndispatchedBody(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "reconnect"})
	conn, err := net.DialTimeout("tcp4", g.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = fmt.Fprintf(conn, "POST /v1/messages HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAnthropic-Version: %s\r\nX-Claude-Code-Session-Id: public\r\nX-Claude-Code-Request-Class: main\r\nContent-Length: %d\r\n\r\n%s", g.Addr(), g.Token(), anthropicVersion, len(validRequest), validRequest[:len(validRequest)/2])
	if err != nil {
		t.Fatal(err)
	}
	waitForActive(t, g, 1, "partial body")
	if f.Calls() != 0 {
		t.Fatal("partial body dispatched")
	}
	// Leave the old socket open. Recovery cannot depend on the filter forwarding
	// its FIN/RST, and only a validated replacement may cancel this pending read.
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Session-Id"] = "public"
	response := do(t, g, rq)
	if response.StatusCode != 200 || !strings.Contains(bodyText(t, response), "message_stop") || f.Calls() != 1 {
		t.Fatal("replacement did not execute exactly once")
	}
	waitForActive(t, g, 0, "partial body replaced")
	found := false
	for _, r := range g.ring.recent() {
		if r.CancellationSource == "native_reconnected_input" && r.Category == "CANCELLED" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing independent pre-dispatch cancellation proof")
	}
}

type interruptedReply struct {
	upstream.Fixture
	holdFirst bool
	entered   chan struct{}
	attempts  atomic.Int64
}

func (f *interruptedReply) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if f.attempts.Add(1) == 1 && f.holdFirst {
		close(f.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.Fixture.Execute(ctx, call)
}

// Keep V1's failing socket scenarios on the Go product path. The upstream bytes
// are public fixtures; listener, parser, registry, budget and cleanup are real.
func TestThirtyTwoRepliesThenBudgetRefusal(t *testing.T) {
	f := &limitedReply{
		Fixture: &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")},
		ledger:  upstream.NewLedger(upstream.Budget{Model: "gpt-6-astra", Effort: "medium", Limit: 32}),
	}
	g := startWith(t, f)
	for i := 0; i < 33; i++ {
		response := post(t, g, validRequest)
		body := bodyText(t, response)
		response.Body.Close()
		if i < 32 && (response.StatusCode != 200 || !strings.Contains(body, "message_stop")) {
			t.Fatalf("reply %d lost: status=%d", i, response.StatusCode)
		}
		if i == 32 && (response.StatusCode != 400 || !strings.Contains(body, "REQUEST_BUDGET")) {
			t.Fatal("budget refusal did not reach the client")
		}
	}
	waitForActive(t, g, 0, "budget refusal")
	if attempts, inferences, refused := f.ledger.Spent(); attempts != 32 || inferences != 32 || refused != 1 || f.Calls() != 32 {
		t.Fatalf("unexpected execution counts: attempts=%d inferences=%d refused=%d calls=%d", attempts, inferences, refused, f.Calls())
	}
	response := do(t, g, request{method: "GET", path: "/v1/models"})
	if response.StatusCode != 200 {
		t.Fatal("budget refusal closed the gateway")
	}
	response.Body.Close()
}

type limitedReply struct {
	*upstream.Fixture
	ledger *upstream.Ledger
}

func (f *limitedReply) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if err := f.ledger.Reserve(upstream.Attempt{Requested: call.Requested, Model: call.Model, Effort: call.Effort, Source: call.Source}); err != nil {
		return nil, err
	}
	return f.Fixture.Execute(ctx, call)
}

func TestTwentyConcurrentAgentsReleaseAcrossEightRounds(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	send := func(path, body, agent string) error {
		req, err := http.NewRequest("POST", g.BaseURL()+path, strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+g.Token())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Anthropic-Version", anthropicVersion)
		req.Header.Set("X-Claude-Code-Session-Id", "public")
		if agent != "" {
			req.Header.Set("X-Claude-Code-Agent-Id", agent)
			req.Header.Set("X-Claude-Code-Request-Class", "subagent")
		}
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		if response.StatusCode != 200 || agent != "" && !strings.Contains(string(raw), "message_stop") {
			return fmt.Errorf("status=%d incomplete=%t", response.StatusCode, agent != "")
		}
		return nil
	}
	for round := 0; round < 8; round++ {
		var wg sync.WaitGroup
		for worker := 0; worker < 20; worker++ {
			wg.Go(func() {
				id := fmt.Sprintf("public_%d_%d", round, worker)
				for _, operation := range []struct{ path, body, agent string }{
					{"/clauduct/agents", fmt.Sprintf(`{"id":%q,"role":"general-purpose","stop":false,"sessionId":"public"}`, id), ""},
					{"/v1/messages", validRequest, id},
					{"/clauduct/agents", fmt.Sprintf(`{"id":%q,"role":"general-purpose","stop":true,"sessionId":"public"}`, id), ""},
				} {
					if err := send(operation.path, operation.body, operation.agent); err != nil {
						t.Error("agent round failed", err)
						return
					}
				}
			})
		}
		wg.Wait()
		waitForActive(t, g, 0, "concurrent agent round")
		if g.agents.Registered() != 0 || f.Calls() != int64((round+1)*20) {
			t.Fatalf("round=%d registrations=%d executions=%d", round, g.agents.Registered(), f.Calls())
		}
	}
}
