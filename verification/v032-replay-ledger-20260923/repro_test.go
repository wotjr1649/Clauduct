package gateway

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestV032ReproIndependentAuxiliaryIsSpentPerRootTurn(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	request := func() (int, string) {
		rq := messages(strings.NewReader(validRequest))
		rq.headers["X-Claude-Code-Request-Class"] = "auxiliary"
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		response := do(t, g, rq)
		return response.StatusCode, bodyText(t, response)
	}
	// The same title or classifier bytes in a later turn are a new side request.
	for _, turn := range []string{"first-turn", "second-turn"} {
		putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: turn})
		if code, _ := request(); code != 200 {
			t.Fatalf("identical side request refused in %s: status=%d", turn, code)
		}
		if code, body := request(); code != 400 || !strings.Contains(body, "NATIVE_REQUEST_REPLAY_BLOCKED") {
			t.Fatalf("side request replayed within %s: status=%d", turn, code)
		}
	}
	if f.Calls() != 2 {
		t.Fatal("expected one execution per root turn", f.Calls())
	}
}
func TestV032ReproARequestReadsItsTurnReceiptOnce(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")})
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	// A committed receipt with an unreviewed field fails every read and counts each
	// failure, so the invalid count is the number of reads.
	putCancellationReceipt(t, dir, "active/root", map[string]string{"session": "public", "agent": "", "turn": "public-turn", "unreviewed": "field"})
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Session-Id"] = "public"
	response := do(t, g, rq)
	if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "AGENT_SELECTION_UNVERIFIED") {
		t.Fatal("unverifiable receipt admitted", response.StatusCode)
	}
	if invalid := g.nativeEventReport().Invalid; invalid != 1 {
		t.Fatalf("one request read the receipt %d times", invalid)
	}
}
// A turn published while a request is still uploading belongs to the next request.
// Reading the receipt again after the body keyed this request under the newer turn,
// so the newer turn's identical input was refused as a replay.
func TestV032ReproARequestKeepsTheTurnItArrivedIn(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "arrival"})
	body, upload := io.Pipe()
	r, err := http.NewRequest(http.MethodPost, g.BaseURL()+"/v1/messages?beta=true", body)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range messages(nil).headers {
		r.Header.Set(key, value)
	}
	r.Header.Set("X-Claude-Code-Session-Id", "public")
	r.Header.Set("Authorization", "Bearer "+g.Token())
	status := make(chan int, 1)
	go func() {
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			status <- 0
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		status <- response.StatusCode
	}()
	half := len(validRequest) / 2
	if _, err := io.WriteString(upload, validRequest[:half]); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		g.nativeEvents.mu.Lock()
		reading := len(g.nativeEvents.cancellations)
		g.nativeEvents.mu.Unlock()
		if reading == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload never bound to its turn")
		}
	}
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "published-during-upload"})
	if _, err := io.WriteString(upload, validRequest[half:]); err != nil {
		t.Fatal(err)
	}
	upload.Close()
	if code := <-status; code != 200 {
		t.Fatal("uploading request", code)
	}
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Session-Id"] = "public"
	if response := do(t, g, rq); response.StatusCode != 200 {
		t.Fatal("the newer turn's input was taken for a replay", response.StatusCode, bodyText(t, response))
	}
	if f.Calls() != 2 {
		t.Fatal("unexpected executions", f.Calls())
	}
}

func TestV032ReproCapacityAfterANewTurn(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	request := func(body string) (int, string) {
		rq := messages(strings.NewReader(body))
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		response := do(t, g, rq)
		return response.StatusCode, bodyText(t, response)
	}
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "earlier"})
	if code, _ := request(validRequest); code != 200 {
		t.Fatal("first request", code)
	}
	g.executions.Lock()
	for i := len(g.executions.seen); i < maxNativeExecutions; i++ {
		g.executions.seen[nativeExecutionKey{session: "public", turn: "earlier", body: [32]byte{byte(i), byte(i >> 8)}}] = struct{}{}
	}
	g.executions.Unlock()
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "later"})
	code, body := request(validRequest)
	category := "none"
	if i := strings.Index(body, `"message":"`); i >= 0 {
		category = strings.SplitN(body[i+11:], `"`, 2)[0]
	}
	t.Logf("new turn after %d earlier executions: status=%d category=%s", maxNativeExecutions, code, category)
	if code != 200 {
		t.Fatal("a new turn stayed blocked by earlier turns' executions")
	}
}
