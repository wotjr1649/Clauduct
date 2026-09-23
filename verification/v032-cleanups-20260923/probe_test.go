package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Temporary v0.3.2 probes for #65 and #66, attached with -overlay to HEAD and to the fix.
func TestV032RetainedHandlerPointers(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")})
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "measured"})
	for i := 0; i < 20; i++ {
		rq := messages(strings.NewReader(strings.Replace(validRequest, `"ping"`, fmt.Sprintf(`"ping %d"`, i), 1)))
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		if response := do(t, g, rq); response.StatusCode != 200 {
			t.Fatal(response.StatusCode, bodyText(t, response))
		}
	}
	waitForActive(t, g, 0, "every measured request finished")
	g.ring.mu.Lock()
	defer g.ring.mu.Unlock()
	turns, executions, results := 0, 0, 0
	for _, e := range g.ring.entries {
		if e.nativeTurn != nil {
			turns++
		}
		if e.execution != nil {
			executions++
		}
		if e.nativeResult != nil {
			results++
		}
	}
	t.Logf("after 20 finished requests: ring records=%d holding nativeTurn=%d execution=%d nativeResult=%d", len(g.ring.entries), turns, executions, results)
}

func TestV032ClosingRefusalLatency(t *testing.T) {
	for _, c := range []struct {
		name string
		sent int64
		busy bool
	}{{"busy", 1024, true}, {"too-large", maxRequestBytes + 1, false}} {
		g := start(t)
		var releases []func()
		if c.busy {
			for i := 0; i < maxActiveRequests; i++ {
				_, _, release, err := g.requests.admit(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				releases = append(releases, release)
			}
		}
		body, upload := io.Pipe()
		sent := make(chan time.Time, 1)
		go func() { _, _ = io.CopyN(upload, endlessReader{}, c.sent); sent <- time.Now() }()
		r, _ := http.NewRequest(http.MethodPost, g.BaseURL()+"/v1/messages", body)
		for key, value := range messages(nil).headers {
			r.Header.Set(key, value)
		}
		r.Header.Set("Authorization", "Bearer "+g.Token())
		started := time.Now()
		response, err := http.DefaultClient.Do(r)
		answered := time.Now()
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		after := answered.Sub(started)
		if c.busy {
			t.Logf("%s: status=%d close=%v refusal %v after the request started", c.name, response.StatusCode, response.Close, after.Round(time.Millisecond))
		} else {
			t.Logf("%s: status=%d close=%v refusal %v after the last byte was sent", c.name, response.StatusCode, response.Close, answered.Sub(<-sent).Round(time.Millisecond))
		}
		upload.Close()
		for _, release := range releases {
			release()
		}
	}
}
