package gateway

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Temporary v0.3.2 probe for #64, attached with -overlay to HEAD and to the fix.
var (
	reviewProbeAfterClaim func()
	reviewProbeReads      atomic.Int64
)

func TestV032TurnPublishedBetweenReads(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "claimed"})
	reviewProbeAfterClaim = func() {
		putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "published-between-reads"})
	}
	defer func() { reviewProbeAfterClaim = nil }()
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Session-Id"] = "public"
	response := do(t, g, rq)
	body := bodyText(t, response)
	category := "none"
	if i := strings.Index(body, `"message":"`); i >= 0 {
		category = strings.SplitN(body[i+11:], `"`, 2)[0]
	}
	t.Logf("turn published between claim and selection: status=%d category=%s backend_calls=%d", response.StatusCode, category, f.Calls())
	if response.StatusCode != 200 {
		t.Fatal("request refused")
	}
}

func TestV032ReceiptReadsPerRequest(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "measured"})
	for _, c := range []struct{ name, class, path, body string }{
		{"main", "main", "/v1/messages?beta=true", validRequest},
		{"auxiliary", "auxiliary", "/v1/messages?beta=true", strings.Replace(validRequest, `"ping"`, `"title"`, 1)},
		{"count_tokens", "main", "/v1/messages/count_tokens", `{"model":"gpt-6-astra","messages":[{"role":"user","content":"ping"}]}`},
	} {
		reviewProbeReads.Store(0)
		rq := messages(strings.NewReader(c.body))
		rq.path = c.path
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		rq.headers["X-Claude-Code-Request-Class"] = c.class
		response := do(t, g, rq)
		bodyText(t, response)
		t.Logf("request=%s status=%d receipt_directory_reads=%d", c.name, response.StatusCode, reviewProbeReads.Load())
	}
}
