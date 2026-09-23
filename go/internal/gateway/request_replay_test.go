package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNativeCancellationBeforeDispatchKeepsInputAvailable(t *testing.T) {
	for name, body := range map[string]string{"inference": validRequest, "search": sideQuery("public query")} {
		t.Run(name, func(t *testing.T) {
			f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]"), SearchJSON: searchAnswer}
			g := startWith(t, f)
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "undispatched"})
			rq := messages(strings.NewReader(body))
			rq.headers["X-Claude-Code-Session-Id"] = "public"
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r := httptest.NewRequest(http.MethodPost, g.BaseURL()+rq.path, strings.NewReader(body)).WithContext(ctx)
			r.RemoteAddr = "127.0.0.1:32100"
			r.Header.Set("Authorization", "Bearer "+g.Token())
			for key, value := range rq.headers {
				r.Header.Set(key, value)
			}
			w := httptest.NewRecorder()
			g.handle(w, r)
			if w.Code != 499 || !strings.Contains(w.Body.String(), "CANCELLED") || f.Calls()+f.Searches() != 0 {
				t.Fatalf("cancelled input reached transport: status=%d calls=%d searches=%d", w.Code, f.Calls(), f.Searches())
			}
			// A replacement connection may admit the same input only because the
			// cancelled handler never dispatched. Once dispatched it stays spent.
			for _, duplicate := range []bool{false, true} {
				rq.body = strings.NewReader(body)
				response := do(t, g, rq)
				reply := bodyText(t, response)
				if !duplicate && (response.StatusCode != 200 || !strings.Contains(reply, "message_stop")) || duplicate && (response.StatusCode != 400 || !strings.Contains(reply, "NATIVE_REQUEST_REPLAY_BLOCKED")) {
					t.Fatalf("replacement connection lost execution ownership: duplicate=%t status=%d", duplicate, response.StatusCode)
				}
			}
			if f.Calls()+f.Searches() != 1 {
				t.Fatal("input must execute exactly once")
			}
		})
	}
}

func TestNativeRequestReplayNeverDispatchesTwice(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	turn := nativeTurnReceipt{Session: "public", Turn: "turn-one"}
	putCancellationReceipt(t, dir, "active/root", turn)
	request := func(body string) (int, string) {
		rq := messages(strings.NewReader(body))
		rq.headers["X-Claude-Code-Session-Id"] = turn.Session
		rq.headers["X-Stainless-Retry-Count"] = "0"
		response := do(t, g, rq)
		return response.StatusCode, bodyText(t, response)
	}
	if code, _ := request(validRequest); code != 200 {
		t.Fatal("first request failed", code)
	}
	if code, body := request(validRequest); code != 400 || !strings.Contains(body, "NATIVE_REQUEST_REPLAY_BLOCKED") || f.Calls() != 1 {
		t.Fatalf("same native turn dispatched again: status=%d calls=%d", code, f.Calls())
	}
	// A real next tool step changes the input; an explicit new input changes the
	// native turn even if the user asks for the same thing. Both remain usable.
	changed := strings.Replace(validRequest, `"ping"`, `"next public input"`, 1)
	if changed == validRequest {
		t.Fatal("fixture input marker changed")
	}
	if code, _ := request(changed); code != 200 || f.Calls() != 2 {
		t.Fatal("changed input was withheld")
	}
	turn.Turn = "turn-two"
	putCancellationReceipt(t, dir, "active/root", turn)
	if code, _ := request(validRequest); code != 200 || f.Calls() != 3 {
		t.Fatal("new native turn was withheld")
	}
	if g.RefusalsByCategory()["NATIVE_REQUEST_REPLAY_BLOCKED"] != 1 {
		t.Fatal("duplicate refusal was not accounted")
	}
}

func TestNativeRequestReplayIsBlockedWhileFirstAttemptIsRunning(t *testing.T) {
	f := &nativeHeldTransport{entered: make(chan struct{})}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "in-flight"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Session-Id"] = "public"
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL()+"/v1/messages", strings.NewReader(validRequest))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range rq.headers {
		r.Header.Set(key, value)
	}
	r.Header.Set("Authorization", "Bearer "+g.Token())
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, _ := client.Do(r)
		if response != nil {
			response.Body.Close()
		}
	}()
	select {
	case <-f.entered:
	case <-ctx.Done():
		t.Fatal("first attempt not dispatched")
	}
	response := do(t, g, rq)
	if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "NATIVE_REQUEST_REPLAY_BLOCKED") {
		t.Error("in-flight duplicate not refused")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first attempt did not cancel")
	}
	rq.body = strings.NewReader(validRequest)
	response = do(t, g, rq)
	if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "NATIVE_REQUEST_REPLAY_BLOCKED") {
		t.Fatal("cancellation allowed an ambiguous replay")
	}
}

func TestNativeRequestReservationReleasesOnlyBeforeDispatch(t *testing.T) {
	g := start(t)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "public-turn"})
	r, _ := http.NewRequest(http.MethodPost, g.BaseURL()+"/v1/messages", nil)
	r.Header.Set("X-Claude-Code-Session-Id", "public")
	r.Header.Set("X-Claude-Code-Request-Class", "main")
	first, category := g.claimNativeExecution(r, []byte(validRequest), nil)
	if category != "" {
		t.Fatal(category)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.dispatch(cancelled); err == nil {
		t.Fatal("cancelled reservation crossed dispatch boundary")
	}
	first.release()
	second, category := g.claimNativeExecution(r, []byte(validRequest), nil)
	if category != "" {
		t.Fatal("undispatched reservation remained")
	}
	if err := second.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	second.release()
	if _, category = g.claimNativeExecution(r, []byte(validRequest), nil); category != "NATIVE_REQUEST_REPLAY_BLOCKED" {
		t.Fatal("spent reservation was forgotten")
	}
	// Saturation refuses new work; it must never evict spent executions.
	for i := len(g.executions.seen); i < maxNativeExecutions; i++ {
		key := nativeExecutionKey{turn: "capacity", body: [32]byte{byte(i), byte(i >> 8)}}
		g.executions.seen[key] = struct{}{}
	}
	if _, category = g.claimNativeExecution(r, []byte("different public input"), nil); category != "NATIVE_REQUEST_CAPACITY" {
		t.Fatal("capacity silently forgot prior executions")
	}
}

func TestNativeRequestWithoutTurnReceiptStillBlocksReplay(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	g.ConfigureNativeEvents(t.TempDir()) // native --bare does not publish receipts
	first := post(t, g, validRequest)
	bodyText(t, first)
	second := post(t, g, validRequest)
	if first.StatusCode != 200 || second.StatusCode != 400 || !strings.Contains(bodyText(t, second), "NATIVE_REQUEST_REPLAY_BLOCKED") || f.Calls() != 1 {
		t.Fatal("missing identity either refused first execution or disabled replay protection")
	}
}

func TestAuxiliaryReplayTrackingPreservesIndependentPreflightRefusals(t *testing.T) {
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 2})
	g := startWith(t, &upstream.Direct{Ledger: ledger})
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	active := filepath.Join(dir, "active", "root")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "1-public.json"), []byte(`{"session":"public","agent":"","turn":"public"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// A root publication in progress cannot block independent title/classifier
	// requests. Their local route refusal must not become a spent execution.
	for i := 0; i < 2; i++ {
		rq := messages(strings.NewReader(validRequest))
		rq.headers["X-Claude-Code-Request-Class"] = "auxiliary"
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		response := do(t, g, rq)
		if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "ROUTE_NOT_AUTHORISED") {
			t.Fatal("independent local refusal was misclassified")
		}
	}
	if attempts, _, refused := ledger.Spent(); attempts != 0 || refused != 2 {
		t.Fatal("local refusal reached backend or was swallowed")
	}
	response := post(t, g, validRequest)
	if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "AGENT_SELECTION_UNVERIFIED") {
		t.Fatal("conversation admitted an unpublished identity")
	}
}

func TestNativeNextStepCanReuseInputButNotReplayItsExecution(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed, "[DONE]")}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "one-turn"})
	for _, index := range []int{0, 1} {
		putCancellationReceipt(t, dir, "step-root.json", parentStep{Session: "public", Turn: "one-turn", Index: index, Mode: "native_tui"})
		for _, repeat := range []bool{false, true} {
			rq := messages(strings.NewReader(validRequest))
			rq.headers["X-Claude-Code-Session-Id"] = "public"
			response := do(t, g, rq)
			body := bodyText(t, response)
			if !repeat && response.StatusCode != 200 || repeat && (response.StatusCode != 400 || !strings.Contains(body, "NATIVE_REQUEST_REPLAY_BLOCKED")) {
				t.Fatal("next step and network replay were conflated", index, repeat, response.StatusCode)
			}
		}
	}
	if f.Calls() != 2 {
		t.Fatal("expected exactly one execution per native step")
	}
}

func TestNativeStepHasOneConversationOwner(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("public"), done("public"), completed)}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: "same-turn"})
	putCancellationReceipt(t, dir, "step-root.json", parentStep{Session: "public", Turn: "same-turn", Index: 1, Mode: "sdk"})
	request := func(class, body string) (int, string) {
		rq := messages(strings.NewReader(body))
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		rq.headers["X-Claude-Code-Request-Class"] = class
		response := do(t, g, rq)
		return response.StatusCode, bodyText(t, response)
	}
	if code, _ := request("main", validRequest); code != 200 {
		t.Fatal("first request", code)
	}
	changed := strings.Replace(validRequest, `"ping"`, `"different public bytes"`, 1)
	for _, class := range []string{"main", "subagent", "workflow"} {
		if code, body := request(class, changed); code != 400 || !strings.Contains(body, "NATIVE_REQUEST_REPLAY_BLOCKED") {
			t.Fatalf("a second body/class took ownership of one native step: %s %d", class, code)
		}
	}
	if f.Calls() != 1 {
		t.Fatal("same step dispatched twice")
	}
	if code, _ := request("compaction", validRequest); code != 200 || f.Calls() != 2 {
		t.Fatal("separate compaction operation confused with a conversation step", code)
	}
}

func TestRateLimitDoesNotAuthoriseNativeReplay(t *testing.T) {
	f := &upstream.Fixture{Err: upstream.ClassifyStatus(429, http.Header{}, time.Now()), SSE: sse(created, delta("public"), done("public"), completed)}
	g := startWith(t, f)
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	setTurn := func(turn string) {
		putCancellationReceipt(t, dir, "active/root", nativeTurnReceipt{Session: "public", Turn: turn})
		putCancellationReceipt(t, dir, "step-root.json", parentStep{Session: "public", Turn: turn, Mode: "sdk"})
	}
	request := func() (int, string) {
		rq := messages(strings.NewReader(validRequest))
		rq.headers["X-Claude-Code-Session-Id"] = "public"
		res := do(t, g, rq)
		return res.StatusCode, bodyText(t, res)
	}
	setTurn("limited")
	if code, body := request(); code != 429 || !strings.Contains(body, "RATE_LIMITED") {
		t.Fatal("first cause lost", code)
	}
	if code, body := request(); code != 400 || !strings.Contains(body, "NATIVE_REQUEST_REPLAY_BLOCKED") || f.Calls() != 1 {
		t.Fatal("rate limit allowed automatic redispatch", code, f.Calls())
	}
	f.Err = nil
	setTurn("explicit-next-input")
	if code, _ := request(); code != 200 || f.Calls() != 2 {
		t.Fatal("explicit new turn failed", code)
	}
}
