package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	if err = os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestNativeAbortIsBoundToExactRequestWithoutDependingOnSocket(t *testing.T) {
	for _, mutation := range []string{"none", "session", "agent", "turn", "answer", "extra_field", "auxiliary", "compaction", "foreign_active"} {
		t.Run(mutation, func(t *testing.T) {
			g := &Gateway{}
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			active := nativeTurnReceipt{Session: "public", Agent: "child", Turn: "turnA", Model: "gpt-5.6-sol", Effort: "high"}
			if mutation == "foreign_active" {
				active.Session = "foreign"
			}
			putCancellationReceipt(t, dir, "active-child.json", active)
			r := httptest.NewRequest("POST", "/v1/messages", nil)
			r.Header.Set("X-Claude-Code-Session-Id", "public")
			r.Header.Set("X-Claude-Code-Agent-Id", "child")
			r.Header.Set("X-Claude-Code-Request-Class", "subagent")
			if mutation == "auxiliary" || mutation == "compaction" {
				r.Header.Set("X-Claude-Code-Request-Class", mutation)
			}
			ctx, finish := g.bindNativeCancellation(context.Background(), r, nil)
			defer finish()
			// A resumed turn must survive an old turn's explicit abort.
			active.Session = "public"
			active.Turn = "turnB"
			putCancellationReceipt(t, dir, "active-child.json", active)
			sibling, done := g.bindNativeCancellation(context.Background(), r, nil)
			defer done()
			receipt := map[string]string{"session": "public", "agent": "child", "turn": "turnA", "reason": "aborted"}
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

func TestNativeAbortStopsUpstreamWhileHTTPConnectionRemainsOpen(t *testing.T) {
	f := &nativeHeldTransport{entered: make(chan struct{})}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active-main.json", nativeTurnReceipt{Session: "public", Turn: "root_turn"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(validRequest)).WithContext(ctx)
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
	putCancellationReceipt(t, dir, "cancel-root_turn.json", nativeTurnReceipt{Session: "public", Turn: "root_turn", Reason: "aborted"})
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
	if len(recent) != 1 || recent[0].Category != "CANCELLED" || recent[0].CancellationSource != "native_abort_receipt" || g.requests.count() != 0 {
		t.Fatal("missing cancellation proof or leaked request")
	}
}
