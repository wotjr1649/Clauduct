package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
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

// --- session 36: what a long session can still say about its failures ---------------------
//
// The first real session ran 184 requests. Both defects below are invisible in a short one
// and certain in a long one, which is why a suite of short tests never found them.

// B. A refusal's reason has to outlive the ring it was recorded in.
//
// Measured 2026-09-17: a 184-request session reported refused=2 and could not say which two
// or why. The categories are there -- eight of them reach a reader as a record's Category --
// but the only cumulative counter is the aggregate, and the detail sits in sixteen slots
// that a real session overruns in its first minute.
func TestRefusalReasonsSurviveALongSession(t *testing.T) {
	g := start(t)
	const each = 10
	for i := 0; i < each; i++ {
		do(t, g, request{method: http.MethodGet, path: "/api/hello"})   // UNSUPPORTED_ROUTE
		do(t, g, request{method: http.MethodGet, path: "/v1/messages"}) // UNSUPPORTED_METHOD
	}

	account := status(t, g)
	if account.Requests.Refused != 2*each {
		t.Fatalf("refused = %d, want %d", account.Requests.Refused, 2*each)
	}

	byReason := account.Requests.RefusedBy
	if byReason["UNSUPPORTED_ROUTE"] != each || byReason["UNSUPPORTED_METHOD"] != each {
		t.Errorf("refusedBy = %v, want %d of each reason", byReason, each)
	}

	// The aggregate and the breakdown are one fact. A session that refuses something under a
	// reason nobody counted is exactly the session this test exists for, so the total is
	// checked against the sum rather than against the number this test happens to know.
	var summed int64
	for _, count := range byReason {
		summed += count
	}
	if summed != account.Requests.Refused {
		t.Errorf("refusedBy sums to %d and refused says %d: %d refusals have no reason",
			summed, account.Requests.Refused, account.Requests.Refused-summed)
	}

	// And the detail the ring holds is still the last sixteen, which is what it is for. The
	// breakdown is not a replacement for it; it is the half that had to outlive it.
	if len(account.Recent) != recentRequests {
		t.Errorf("recent = %d records, want the ring's %d", len(account.Recent), recentRequests)
	}
}

// B, second half. BrokenStreams counted the ring, so a long session always reported zero.
//
// The count exists because a broken stream is the one failure the client sees as a 200, so
// nothing else in the account can report it. Counting a sixteen-slot window meant the longer
// a session ran -- the longer it had had to break something -- the more certainly it said
// nothing broke. It is a session total now, and this is what holds it to one.
func TestBrokenStreamsSurviveALongSession(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE:     sse(created, delta("x"), done("x"), completed),
		ReadErr: errors.New("connection reset by peer"),
	})
	const streams = recentRequests + 4
	for i := 0; i < streams; i++ {
		post(t, g, validRequest)
	}

	if got := status(t, g).BrokenStreams(); got != streams {
		t.Fatalf("BrokenStreams = %d, want %d -- the ring holds %d and the rest are gone",
			got, streams, recentRequests)
	}
}

// B, the half that was still missing, found by running the fix in a real session.
//
// Measured 2026-09-18: an interactive session refused five requests with UNSUPPORTED_ROUTE
// and the account could not say which route. For that category the reason carries nothing --
// it means "a path this gateway does not serve" and the path is the whole finding. A -p
// session refuses none of these, so the paths a real client asks for are exactly what no
// short test can see.
//
// The paths were in Recent and Recent is sixteen slots, which is the same rolling window
// that made the counts useless. So the distinct ones are kept, bounded.
func TestRefusedPathsSurviveALongSession(t *testing.T) {
	g := start(t)
	paths := []string{"/api/hello", "/v1/messages/count_tokens", "/some/other/route"}
	for i := 0; i < 8; i++ {
		for _, path := range paths {
			do(t, g, request{method: http.MethodGet, path: path})
		}
	}

	account := status(t, g)
	if len(account.Recent) != recentRequests {
		t.Fatalf("recent = %d; this test needs the ring to have rolled", len(account.Recent))
	}

	named := map[string]bool{}
	for _, path := range account.Requests.RefusedPaths {
		named[path] = true
	}
	for _, path := range paths {
		if !named[path] {
			t.Errorf("%s was refused %d times and the account cannot name it: %v",
				path, 8, account.Requests.RefusedPaths)
		}
	}
}

// Bounded, and bounded in the direction that keeps the first answer rather than the last.
//
// Refusals happen before a credential is checked, so anything on this machine that can guess
// the port can produce them. A set that grew without limit would be a memory cost handed to
// whoever asks for it, and one that evicted would let a flood erase the path a reader came
// for. Keeping the first ones means a flood fills slots it cannot take back.
func TestTheRefusedPathSetIsBoundedAndKeepsWhatItSawFirst(t *testing.T) {
	g := start(t)
	do(t, g, request{method: http.MethodGet, path: "/first-one"})
	for i := 0; i < refusedPathLimit*3; i++ {
		do(t, g, request{method: http.MethodGet, path: fmt.Sprintf("/flood-%d", i)})
	}

	account := status(t, g)
	if len(account.Requests.RefusedPaths) > refusedPathLimit {
		t.Errorf("the set holds %d paths, past the cap of %d",
			len(account.Requests.RefusedPaths), refusedPathLimit)
	}
	found := false
	for _, path := range account.Requests.RefusedPaths {
		if path == "/first-one" {
			found = true
		}
	}
	if !found {
		t.Errorf("a flood pushed out the path that arrived first: %v", account.Requests.RefusedPaths)
	}
	// The total still accounts for every refusal; only the naming is capped.
	if account.Requests.Refused != int64(1+refusedPathLimit*3) {
		t.Errorf("refused = %d, want %d", account.Requests.Refused, 1+refusedPathLimit*3)
	}
}

// A path is not named unless it is the shape a path is.
//
// This set is written into a diagnostic a person reads and other tools may parse, and the
// request decides the string. Recent already carries raw paths, so holding a cumulative set
// to a shape is the narrower rule of the two -- but it is the one that has to hold here,
// because these entries outlive the request that produced them.
func TestAPathThatIsNotOneIsNotNamed(t *testing.T) {
	g := start(t)
	// Percent-encoded on the wire; the server decodes it before anything here sees it.
	do(t, g, request{method: http.MethodGet, path: "/has%20a%20space"})
	do(t, g, request{method: http.MethodGet, path: "/plain-one"})

	account := status(t, g)
	if account.Requests.Refused != 2 {
		t.Fatalf("refused = %d, want both", account.Requests.Refused)
	}
	for _, path := range account.Requests.RefusedPaths {
		if !printablePath(path) {
			t.Errorf("the account named %q, which is not the shape a path is", path)
		}
	}
	named := strings.Join(account.Requests.RefusedPaths, " ")
	if !strings.Contains(named, "/plain-one") {
		t.Errorf("the ordinary path was dropped along with the other: %v",
			account.Requests.RefusedPaths)
	}
	if strings.Contains(named, "space") {
		t.Errorf("a path with a space in it reached the account: %v", account.Requests.RefusedPaths)
	}
}

// stoppedReader is a client that accepts the status and then stops reading.
//
// The one shape none of the writers here had: recordingWriter always succeeds, so the path
// where emit fails after the response is committed was never driven by a test.
type stoppedReader struct {
	recordingWriter
	after int
}

func (s *stoppedReader) Write(p []byte) (int, error) {
	if s.bytes >= s.after {
		return 0, errors.New("client stopped reading")
	}
	return s.recordingWriter.Write(p)
}

// A response that was committed and then could not be delivered is not a success.
//
// Found by a code review of session 36's own diff, 2026-09-18. relay funnels stream failures
// through fail(), which marks the record and counts the break -- and one path does not go
// through it. When emit() fails the client has stopped reading, so there is nobody to send an
// error frame to, and the handler returns. The record is then closed by finish(), which turns
// in-progress into ok, and the account files a response that never arrived as one that did.
//
// Only a cancelled request context establishes cancellation.
func TestADeliveryTheClientStoppedReadingIsNotFiledAsOK(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("one"), delta("two"), done("onetwo"), completed, "[DONE]"),
	})
	request, err := anthropic.DecodeRequest([]byte(`{"model":"claude-opus-5","max_tokens":16,
	  "stream":true,"messages":[{"role":"user","content":"x"}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	response, err := g.transport.Execute(context.Background(), upstream.Call{Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	defer response.Body.Close()

	// A record, because that is what the account is made of.
	entry := g.ring.open(http.MethodPost, "/v1/messages")
	w := &stoppedReader{after: 1}
	tracked := &tracked{ResponseWriter: w, rec: entry}
	g.relay(context.Background(), tracked, http.NewResponseController(w), response, request, "gpt-5.6-sol")
	entry.finish()

	account := g.Diagnose()
	if len(account.Recent) != 1 {
		t.Fatalf("recent = %d records", len(account.Recent))
	}
	if got := account.Recent[0].Outcome; got == outcomeOK {
		t.Errorf("a response the client never received is filed as %q", got)
	}
	if account.Recent[0].Category == "" {
		t.Errorf("the record does not say what stopped it: %+v", account.Recent[0])
	}
	// A write failure alone is no evidence that the request was cancelled.
	if account.BrokenStreams() != 1 || account.Recent[0].Category != "DELIVERY_FAILED" {
		t.Errorf("unconfirmed cancellation must be a delivery failure: %+v", account)
	}
}

func TestDeliveryFailureWithoutCancellation(t *testing.T) {
	for _, search := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "search"}[search], func(t *testing.T) {
			for _, flush := range []bool{false, true} {
				t.Run(map[bool]string{false: "write", true: "flush"}[flush], func(t *testing.T) {
					f := &upstream.Fixture{SSE: sse(created, delta("one"), done("one"), completed, "[DONE]"), SearchJSON: searchAnswer}
					g := startWith(t, f)
					entry := g.ring.open(http.MethodPost, "/v1/messages")
					var w http.ResponseWriter = &stoppedReader{after: 1}
					if flush {
						w = &deliveryFlushFailure{}
					}
					tracked := &tracked{ResponseWriter: w, rec: entry}
					ctx := context.Background()
					if search {
						rq, err := anthropic.DecodeRequest([]byte(sideQuery("synthetic")))
						if err != nil {
							t.Fatal(err)
						}
						query, ok := bridge.SideQuery(rq)
						if !ok {
							t.Fatal("query")
						}
						g.searchFor(ctx, tracked, http.NewResponseController(w), rq, query)
					} else {
						rq, err := anthropic.DecodeRequest([]byte(validRequest))
						if err != nil {
							t.Fatal(err)
						}
						up, err := f.Execute(ctx, upstream.Call{Body: []byte(`{}`)})
						if err != nil {
							t.Fatal(err)
						}
						defer up.Body.Close()
						g.relay(ctx, tracked, http.NewResponseController(w), up, rq, "gpt-5.6-sol")
					}
					entry.finish()
					got := g.Diagnose().Recent[0]
					t.Logf("context_error=%v outcome=%s category=%s", ctx.Err(), got.Outcome, got.Category)
					if got.Category == "CANCELLED" || got.Outcome == outcomeOK || got.Category == "" {
						t.Error("uncancelled delivery failure was not classified separately")
					}
				})
			}
		})
	}
}

type deliveryFlushFailure struct{ recordingWriter }

func (*deliveryFlushFailure) FlushError() error {
	return errors.New("synthetic flush failure without cancellation")
}

func TestDeliveryCancellationRequiresContextEvidence(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		g := start(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if cancelled {
			cancel()
		}
		entry := g.ring.open(http.MethodPost, "/v1/messages")
		g.deliveryFailed(ctx, &tracked{ResponseWriter: &recordingWriter{}, rec: entry})
		entry.finish()
		want, broken := "DELIVERY_FAILED", int64(1)
		if cancelled {
			want, broken = "CANCELLED", 0
		}
		if got := g.Diagnose(); got.Recent[0].Category != want || got.Requests.Broken != broken {
			t.Fatalf("cancelled=%v: %+v", cancelled, got)
		}
	}
}

func TestStreamReadCancellationAndFailureEvidenceSurviveEviction(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		g := start(t)
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		rq, err := anthropic.DecodeRequest([]byte(validRequest))
		if err != nil {
			t.Fatal(err)
		}
		fixture := &upstream.Fixture{ReadErr: errors.New("PRIVATE_ERROR_MUST_NOT_BE_RECORDED")}
		response, err := fixture.Execute(context.Background(), upstream.Call{Body: []byte(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		entry := g.ring.open(http.MethodPost, "/v1/messages")
		w := &tracked{ResponseWriter: &recordingWriter{}, rec: entry}
		if g.relay(ctx, w, http.NewResponseController(w), response, rq, "gpt-5.6-sol") {
			t.Fatal("read failure accepted")
		}
		response.Body.Close()
		cancel()
		entry.finish()
		for i := 0; i < recentRequests+1; i++ {
			g.ring.open(http.MethodGet, "/v1/models").finish()
		}
		account := g.Diagnose()
		if len(account.RecentFailures) != 1 {
			t.Fatalf("failure evicted: %+v", account.RecentFailures)
		}
		failure := account.RecentFailures[0]
		category, client := "TRUNCATED_STREAM", "none"
		if cancelled {
			category, client = "CANCELLED", "cancelled"
		}
		if failure.Category != category || failure.StreamEnd == nil || failure.StreamEnd.ClientContext != client || failure.StreamEnd.ReadError != "transport_error" || failure.StreamEnd.TerminalObserved {
			t.Fatalf("wrong evidence: %+v / %+v", failure, failure.StreamEnd)
		}
		encoded, err := json.Marshal(account)
		if err != nil || strings.Contains(string(encoded), "PRIVATE_ERROR") {
			t.Fatal("transport content escaped")
		}
	}
}

func TestSessionTotalsSurviveEvictionAndConcurrentFinish(t *testing.T) {
	r := newRing()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		e := r.open(http.MethodPost, "/v1/messages")
		e.kind("web_search")
		e.route("haiku", "gpt-5.6-luna", "max", "role")
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.brokeAfterCommitting("DELIVERY_FAILED")
			e.finish()
			e.finish()
		}()
	}
	wg.Wait()
	got := r.counts()
	if len(r.recent()) != 16 || got.Completed != 64 || got.Kinds["web_search"] != 64 ||
		got.Routes["gpt-5.6-luna/max/role"] != 64 || got.KindRoutes["web_search/gpt-5.6-luna/max/role"] != 64 || got.Failures["DELIVERY_FAILED"] != 64 {
		t.Fatalf("totals lost evicted requests: %+v", got)
	}
	got.Kinds["web_search"] = 0
	got.KindRoutes["web_search/gpt-5.6-luna/max/role"] = 0
	if r.counts().Kinds["web_search"] != 64 || r.counts().KindRoutes["web_search/gpt-5.6-luna/max/role"] != 64 {
		t.Fatal("snapshot aliases live counters")
	}
}

func TestDeadlineIsNotReportedAsCancellation(t *testing.T) {
	if categoryFor(context.DeadlineExceeded) != "REQUEST_TIMEOUT" || categoryFor(context.Canceled) != "CANCELLED" {
		t.Fatal("timeout and cancellation conflated")
	}
}

func TestSearchUsesRegisteredAgentModel(t *testing.T) {
	f := &upstream.Fixture{SearchJSON: searchAnswer}
	g := startWith(t, f)
	bodyText(t, do(t, g, binding(`{"id":"proof_child","role":"Explore","stop":false}`)))
	rq := messages(strings.NewReader(sideQuery("synthetic public query")))
	rq.headers["X-Claude-Code-Agent-Id"] = "proof_child"
	resp := do(t, g, rq)
	bodyText(t, resp)
	t.Logf("status=%d search_payload=%s", resp.StatusCode, f.LastSearch())
	if !strings.Contains(f.LastSearch(), `gpt-5.6-luna`) {
		t.Error("search did not use the agent route")
	}
}

func TestContextTargetsAreNotReportedAsVerifiedApplication(t *testing.T) {
	g := start(t)
	report := status(t, g)
	if len(report.ModelContexts) != 4 {
		t.Fatalf("models: %+v", report.ModelContexts)
	}
	for _, model := range report.ModelContexts {
		window, compact := int64(272000), int64(239000)
		if model.Model == "gpt-6-astra" {
			window, compact = 500000, 450000
		}
		if model.Target.Window != window || model.Target.CompactAt != compact {
			t.Fatalf("target: %+v", model)
		}
		if model.Application != "not_enforced" || model.Verification != "unverified" || model.Reason == "" {
			t.Fatalf("configuration was promoted to runtime proof: %+v", model)
		}
		if model.Observed.Requests != 0 || model.Observed.LastInputTokens != nil || model.Observed.PeakInputTokens != nil {
			t.Fatalf("fabricated backend observation: %+v", model)
		}
	}
}

func TestBackendContextObservationSurvivesRecentEviction(t *testing.T) {
	for _, value := range []string{"0", "239010", "null"} {
		t.Run(value, func(t *testing.T) {
			end := `{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":` + value + `,"output_tokens":1}}}`
			g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), end, "[DONE]")})
			bodyText(t, post(t, g, `{"model":"gpt-5.6-sol","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"synthetic"}]}`))
			for i := 0; i < 17; i++ {
				bodyText(t, do(t, g, request{method: http.MethodGet, path: "/v1/models"}))
			}
			got := status(t, g)
			for _, model := range got.ModelContexts {
				seen := model.Observed
				if model.Model != "gpt-5.6-sol" || value == "null" {
					if seen.Requests != 0 || seen.LastInputTokens != nil {
						t.Fatalf("unknown or other model usage fabricated: %+v", model)
					}
					continue
				}
				want := int64(0)
				if value == "239010" {
					want = 239010
				}
				if seen.Requests != 1 || seen.LastInputTokens == nil || *seen.LastInputTokens != want || seen.PeakInputTokens == nil || *seen.PeakInputTokens != want {
					t.Fatalf("backend usage lost: %+v", model)
				}
				if model.Verification != "unverified" || model.Application != "not_enforced" {
					t.Fatalf("usage is not policy proof: %+v", model)
				}
			}
		})
	}
}
