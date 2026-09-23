package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func putCancellationReceipt(t *testing.T, dir, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeNativeTestFile(filepath.Join(dir, name), raw); err != nil {
		t.Fatal(err)
	}
}

func TestNativeReconnectCancelsOnlyMatchingInputRead(t *testing.T) {
	for _, mutation := range []string{"none", "session", "agent", "turn", "auxiliary", "compaction"} {
		t.Run(mutation, func(t *testing.T) {
			g := &Gateway{}
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			id := nativeTurnReceipt{Session: "public", Agent: "child", Turn: "A", Model: "gpt-5.6-sol", Effort: "high"}
			putCancellationReceipt(t, dir, "active/child-child", id)
			r := httptest.NewRequest("POST", "/v1/messages", nil)
			r.Header.Set("X-Claude-Code-Session-Id", id.Session)
			r.Header.Set("X-Claude-Code-Agent-Id", id.Agent)
			r.Header.Set("X-Claude-Code-Request-Class", "subagent")
			running, finish := g.bindNativeCancellation(context.Background(), r, &record{nativeTurn: &id}, false)
			defer finish()
			reading, stop := g.bindNativeCancellation(context.Background(), r, &record{}, true)
			defer stop()
			next := id
			switch mutation {
			case "session":
				next.Session = "foreign"
				r.Header.Set("X-Claude-Code-Session-Id", next.Session)
			case "agent":
				next.Agent = "sibling"
				r.Header.Set("X-Claude-Code-Agent-Id", next.Agent)
			case "turn":
				next.Turn = "B"
			case "auxiliary", "compaction":
				r.Header.Set("X-Claude-Code-Request-Class", mutation)
			}
			_, done := g.bindNativeCancellation(context.Background(), r, &record{nativeTurn: &next}, false)
			defer done()
			if (reading.Err() != nil) != (mutation == "none") || running.Err() != nil {
				t.Fatal("replacement cancelled an unrelated read or decoded execution")
			}
		})
	}
}

func TestNativeTerminalReceiptIsBoundToExactRequestWithoutDependingOnSocket(t *testing.T) {
	for _, reason := range []string{"aborted", "error", "refusal"} {
		t.Run(reason, func(t *testing.T) {
			for _, mutation := range []string{"none", "session", "agent", "turn", "answer", "extra_field", "auxiliary", "compaction", "foreign_active"} {
				t.Run(mutation, func(t *testing.T) {
					g := &Gateway{}
					dir := t.TempDir()
					g.ConfigureNativeEvents(dir)
					active := nativeTurnReceipt{Session: "public", Agent: "child", Turn: "turnA", Model: "gpt-5.6-sol", Effort: "high"}
					if mutation == "foreign_active" {
						active.Session = "foreign"
					}
					putCancellationReceipt(t, dir, "active/child-child", active)
					r := httptest.NewRequest("POST", "/v1/messages", nil)
					r.Header.Set("X-Claude-Code-Session-Id", "public")
					r.Header.Set("X-Claude-Code-Agent-Id", "child")
					r.Header.Set("X-Claude-Code-Request-Class", "subagent")
					if mutation == "auxiliary" || mutation == "compaction" {
						r.Header.Set("X-Claude-Code-Request-Class", mutation)
					}
					first := active
					ctx, finish := g.bindNativeCancellation(context.Background(), r, &record{nativeTurn: &first}, false)
					defer finish()
					// A resumed turn must survive an old turn's explicit abort.
					active.Session = "public"
					active.Turn = "turnB"
					putCancellationReceipt(t, dir, "active/child-child", active)
					sibling, done := g.bindNativeCancellation(context.Background(), r, &record{nativeTurn: &active}, false)
					defer done()
					receipt := map[string]string{"session": "public", "agent": "child", "turn": "turnA", "reason": reason}
					switch mutation {
					case "session", "agent", "turn":
						receipt[mutation] = "foreign"
					case "answer":
						receipt["reason"] = "answer"
					case "extra_field":
						receipt["unreviewed"] = "field"
					}
					putCancellationReceipt(t, dir, "cancel-turnA.json", receipt)
					g.ReconcileNativeCancellations()
					if (ctx.Err() != nil) != (mutation == "none") {
						t.Fatal("receipt cancelled the wrong scope or did not cancel the matching turn")
					}
					if sibling.Err() != nil {
						t.Fatal("old abort cancelled resumed turn")
					}
				})
			}
		})
	}
}

type nativeHeldTransport struct {
	upstream.Fixture
	entered chan struct{}
}

func (f *nativeHeldTransport) Execute(ctx context.Context, _ upstream.Call) (*upstream.Response, error) {
	close(f.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *nativeHeldTransport) Search(ctx context.Context, _ []byte) ([]byte, error) {
	close(f.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestNativeTerminalReceiptStopsUpstreamWhileHTTPConnectionRemainsOpen(t *testing.T) {
	for _, reason := range []string{"aborted", "error", "refusal"} {
		for kind, body := range map[string]string{"inference": validRequest, "search": sideQuery("public query")} {
			t.Run(reason+"/"+kind, func(t *testing.T) {
				f := &nativeHeldTransport{entered: make(chan struct{})}
				g := startWith(t, f)
				dir := t.TempDir()
				g.ConfigureNativeEvents(dir)
				putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "root_turn"})
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)).WithContext(ctx)
				r.Host = g.Addr()
				r.RemoteAddr = "127.0.0.1:1234"
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Anthropic-Version", anthropicVersion)
				r.Header.Set("Authorization", "Bearer "+g.Token())
				r.Header.Set("X-Claude-Code-Session-Id", "public")
				r.Header.Set("X-Claude-Code-Request-Class", "main")
				w := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { defer close(done); g.handle(w, r) }()
				select {
				case <-f.entered:
				case <-ctx.Done():
					t.Fatal("upstream never started")
				}
				putCancellationReceipt(t, dir, "cancel-root_turn.json", nativeTurnReceipt{Session: "public", Turn: "root_turn", Reason: reason})
				// A rapid follow-up replaces active/progress, but not the terminal receipt
				// pinned to the interrupted request. Never wait for the socket to report EOF.
				putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "next_turn"})
				g.ReconcileNativeCancellations()
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("upstream waited for filtered socket cancellation")
				}
				if ctx.Err() != nil {
					t.Fatal("test closed client context instead of exercising native receipt")
				}
				recent := g.ring.recent()
				wantSource := "native_" + reason + "_receipt"
				if reason == "aborted" {
					wantSource = "native_abort_receipt"
				}
				if len(recent) != 1 || recent[0].Category != "CANCELLED" || recent[0].CancellationSource != wantSource || g.requests.count() != 0 {
					t.Fatal("missing cancellation proof or leaked request")
				}
			})
		}
	}
}
