package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// status reads the session's own account.
// status reads the account, once every record it holds has closed.
//
// A client holding the response is not proof that the handler has returned: the record
// closes in a defer that runs after the last byte is written, so a reader arriving in that
// window sees in-progress. That reading is accurate and the product is right to give it --
// what has to wait is the test.
//
// CI's race job found this, and it reproduced locally under load a few minutes later, so it
// was never a race-detector artefact. The wait lives here rather than at each call site
// because no test wants to observe an open record.
func status(t *testing.T, g *Gateway) Diagnostics {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		account := readStatus(t, g)
		if settled(account) || time.Now().After(deadline) {
			return account
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settled reports whether every record in the account has finished.
func settled(account Diagnostics) bool {
	for _, record := range account.Recent {
		if record.Outcome == outcomeProgress {
			return false
		}
	}
	return true
}

func readStatus(t *testing.T, g *Gateway) Diagnostics {
	t.Helper()
	resp := do(t, g, request{method: http.MethodGet, path: statusPath})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	var account Diagnostics
	if err := json.Unmarshal([]byte(bodyText(t, resp)), &account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return account
}

// D1, D3. A session can say what it ran, on what, and how long it took.
//
// Four counters could say that something was refused. They could not say which request, at
// which stage, on which model, or whether the model was the one the client asked for.
func TestASessionCanSayWhatItRan(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	account := status(t, g)
	if len(account.Recent) != 1 {
		t.Fatalf("recent = %d records, want the one request", len(account.Recent))
	}
	entry := account.Recent[0]

	if entry.Outcome != outcomeOK || entry.Stage != stageDelivery {
		t.Errorf("outcome=%s stage=%s, want a request that got all the way through",
			entry.Outcome, entry.Stage)
	}
	if entry.Status != http.StatusOK || entry.Path != "/v1/messages" || entry.Method != http.MethodPost {
		t.Errorf("record = %+v", entry)
	}
	// CAP03 again, now where a reader can see it. The client asked for opus and this ran
	// on sol; a record that kept only one of those cannot say that.
	if entry.Requested != "claude-opus-5" || entry.Model != "gpt-5.6-sol" {
		t.Errorf("requested=%q model=%q", entry.Requested, entry.Model)
	}
	if entry.Effort == "" || entry.Source == "" {
		t.Errorf("effort=%q source=%q", entry.Effort, entry.Source)
	}
	if entry.FirstByteMs == nil || entry.EndedMs == nil {
		t.Fatalf("timings = %v, %v", entry.FirstByteMs, entry.EndedMs)
	}
	if *entry.FirstByteMs < entry.StartedMs || *entry.EndedMs < *entry.FirstByteMs {
		t.Errorf("timings out of order: started=%d firstByte=%d ended=%d",
			entry.StartedMs, *entry.FirstByteMs, *entry.EndedMs)
	}
	if account.Requests.Received < 1 || account.UptimeMs < 0 {
		t.Errorf("counts = %+v uptime=%d", account.Requests, account.UptimeMs)
	}
}

// A refused request is a diagnosed failure, not an unrecorded 400.
//
// The record opens before the boundary, version and encoding checks for exactly this. The
// Node baseline orders it the same way and says why: otherwise the requests a reader most
// wants to see are the ones that left no trace.
func TestARefusedRequestSaysWhatRefusedIt(t *testing.T) {
	for name, tc := range map[string]struct {
		rq       request
		category string
		status   int
	}{
		"a route that is not one": {
			rq:       request{method: http.MethodGet, path: "/nowhere"},
			category: "UNSUPPORTED_ROUTE", status: http.StatusNotFound,
		},
		"a version this build does not speak": {
			rq: func() request {
				rq := messages(strings.NewReader(`{"model":"claude-opus-5","max_tokens":16,
				  "stream":true,"messages":[{"role":"user","content":"x"}]}`))
				rq.headers["Anthropic-Version"] = "2024-01-01"
				return rq
			}(),
			category: "UNSUPPORTED_VERSION", status: http.StatusBadRequest,
		},
		"a body this build cannot route": {
			rq: messages(strings.NewReader(`{"model":"gpt-9-nonesuch","max_tokens":16,
			  "stream":true,"messages":[{"role":"user","content":"x"}]}`)),
			category: "UNSUPPORTED_MODEL_OR_EFFORT", status: http.StatusBadRequest,
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: sse(created, completed, "[DONE]")})
			do(t, g, tc.rq)

			account := status(t, g)
			if len(account.Recent) != 1 {
				t.Fatalf("recent = %v", account.Recent)
			}
			entry := account.Recent[0]
			if entry.Outcome != outcomeRefused || entry.Category != tc.category {
				t.Fatalf("outcome=%s category=%q, want refused with %s",
					entry.Outcome, entry.Category, tc.category)
			}
			if entry.Status != tc.status {
				t.Errorf("status = %d, want %d", entry.Status, tc.status)
			}
			if entry.EndedMs == nil {
				t.Error("a refused request was left open")
			}
		})
	}
}

// The stage says which half of the bridge a failure belongs to.
func TestTheStageSaysWhereARequestGotTo(t *testing.T) {
	g := startWith(t, &upstream.Fixture{Err: fmt.Errorf("no backend today")})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	entry := status(t, g).Recent[0]
	if entry.Stage != stageUpstream {
		t.Fatalf("stage = %s, want upstream: the request was built and the backend refused it",
			entry.Stage)
	}
	// And the route is on the record even though nothing ran, which is what says the
	// failure was not this build failing to choose one.
	if entry.Model != "gpt-5.6-sol" {
		t.Errorf("model = %q", entry.Model)
	}
	if entry.FirstByteMs != nil {
		t.Errorf("a request that never reached the client reported a first byte at %d",
			*entry.FirstByteMs)
	}
}

// Reading the account is not traffic.
//
// Sixteen reads would otherwise erase every record of what the session did, which is the
// one thing the reader came for.
func TestReadingTheAccountDoesNotEraseIt(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	for i := 0; i < recentRequests+4; i++ {
		if got := len(status(t, g).Recent); got != 1 {
			t.Fatalf("after %d reads the account held %d records", i+1, got)
		}
	}
}

// The ring keeps the most recent, and says how many there have been.
func TestTheRingKeepsTheMostRecent(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SSE: sse(created, completed, "[DONE]")})
	const sent = recentRequests + 5
	for i := 0; i < sent; i++ {
		do(t, g, request{method: http.MethodGet, path: "/nowhere"})
	}

	account := status(t, g)
	if len(account.Recent) != recentRequests {
		t.Fatalf("recent = %d, want %d", len(account.Recent), recentRequests)
	}
	first, last := account.Recent[0], account.Recent[len(account.Recent)-1]
	if last.Seq != sent {
		t.Errorf("the newest record is %d of %d sent", last.Seq, sent)
	}
	if first.Seq != sent-recentRequests+1 {
		t.Errorf("the oldest kept record is %d, so the window is not the last %d",
			first.Seq, recentRequests)
	}
}

// Nothing a request carried is in the account.
//
// The account is written to a file and printed at the end of a session, so anything it
// keeps is something that outlives the session. A prompt is the user's, a session
// identifier ties runs together, and neither is a diagnostic.
func TestTheAccountKeepsNothingTheRequestCarried(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("REPLY-SECRET"), done("REPLY-SECRET"), completed, "[DONE]"),
	})
	rq := messages(strings.NewReader(`{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"PROMPT-SECRET"}],"system":"SYSTEM-SECRET"}`))
	rq.headers["X-Claude-Code-Session-Id"] = "SESSION-SECRET"
	do(t, g, rq)

	resp := do(t, g, request{method: http.MethodGet, path: statusPath})
	body := bodyText(t, resp)
	for _, secret := range []string{
		"PROMPT-SECRET", "SYSTEM-SECRET", "REPLY-SECRET", "SESSION-SECRET",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("the account kept %q:\n%s", secret, body)
		}
	}
}

// The account is not public. This listener is reachable by anything on the machine that can
// guess a port, and what a session ran is not something to hand out.
func TestTheAccountNeedsTheSessionToken(t *testing.T) {
	g := startWith(t, &upstream.Fixture{})
	rq := request{method: http.MethodGet, path: statusPath}
	rq.headers = map[string]string{"Authorization": "Bearer not-the-token"}
	resp := do(t, g, rq)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("the account answered a request carrying someone else's token: %s",
			bodyText(t, resp))
	}
}

// A response that started and then broke is not a success.
//
// Found by review. Once the status is written nothing goes through the refusal path, so no
// record was marked and finish() closed it as ok -- a stream that stopped halfway with a
// truncated body was filed as a clean session, Refused stayed 0, and the exit line said
// nothing at all. The status stays 200 because that is what the client received; the
// outcome and the category carry what actually happened.
func TestAStreamThatBreaksAfterItsStatusIsNotRecordedAsSuccess(t *testing.T) {
	// A stream that opens, delivers, and then ends without its terminal event.
	g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("half"))})

	resp := post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the 200 the client actually got", resp.StatusCode)
	}
	bodyText(t, resp)

	entry := status(t, g).Recent[0]
	if entry.Outcome == outcomeOK {
		t.Fatalf("a broken stream is recorded as %q: %+v", entry.Outcome, entry)
	}
	if entry.Category == "" {
		t.Fatalf("the record does not say what broke: %+v", entry)
	}
	if entry.Status != http.StatusOK {
		t.Fatalf("status = %d; the client received 200 and the record must not claim otherwise",
			entry.Status)
	}
}
