package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// A request this build accepts: text only, no tools.
const validRequest = `{"model":"gpt-6-astra","max_tokens":1024,"stream":true,
  "messages":[{"role":"user","content":"ping"}]}`

func sse(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("data: " + line + "\n\n")
	}
	return b.String()
}

const (
	created   = `{"type":"response.created","response":{"id":"resp_1"}}`
	completed = `{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":5,"output_tokens":2}}}`
)

func delta(text string) string {
	return `{"type":"response.output_text.delta","item_id":"i","content_index":0,"delta":"` + text + `"}`
}

func done(text string) string {
	return `{"type":"response.output_text.done","item_id":"i","content_index":0,"text":"` + text + `"}`
}

func post(t *testing.T, g *Gateway, body string) *http.Response {
	t.Helper()
	return do(t, g, messages(strings.NewReader(body)))
}

// The whole path: HTTP boundary, credential, decode, convert, execute, parse, translate,
// emit. Nothing here reaches a network — the fixture replays bytes.
func TestTextRoundTripReachesTheClient(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, delta("Hel"), delta("lo"), done("Hello"), completed, "[DONE]")}
	g := startWith(t, fixture)

	resp := post(t, g, validRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, bodyText(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}

	body := bodyText(t, resp)
	for _, want := range []string{
		"event: message_start", "event: content_block_start",
		"event: content_block_delta", "event: content_block_stop",
		"event: message_delta", "event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `"text":"Hel"`) || !strings.Contains(body, `"text":"lo"`) {
		t.Errorf("the deltas did not reach the client:\n%s", body)
	}
	if !strings.Contains(body, `"input_tokens":5`) {
		t.Errorf("usage did not reach the client:\n%s", body)
	}

	if fixture.Calls() != 1 {
		t.Errorf("backend calls = %d, want exactly 1", fixture.Calls())
	}
}

// What the backend was actually asked for, not what the bridge meant to ask for.
func TestTheBackendRequestIsWhatWasBuilt(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, delta("x"), done("x"), completed)}
	g := startWith(t, fixture)

	if resp := post(t, g, validRequest); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	sent := fixture.LastRequest()
	for _, want := range []string{
		`"model":"gpt-6-astra"`,
		`"instructions":"Follow the developer instructions in the conversation."`,
		`"stream":true`,
		`"include":["reasoning.encrypted_content"]`,
		`"store":false`,
		`"ping"`,
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("missing %q in the backend request:\n%s", want, sent)
		}
	}
}

// Chunk boundaries carry no meaning across the real HTTP path either.
func TestByteAtATimeUpstreamStillProducesTheSameAnswer(t *testing.T) {
	stream := sse(created, delta("one"), delta("two"), done("onetwo"), completed, "[DONE]")
	g := startWith(t, &upstream.Fixture{SSE: stream, ChunkSize: 1})

	resp := post(t, g, validRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	body := bodyText(t, resp)
	if !strings.Contains(body, `"text":"one"`) || !strings.Contains(body, `"text":"two"`) {
		t.Fatalf("body:\n%s", body)
	}
}

// With no transport configured the answer says so. A build with nowhere to send a request
// must not look like one that answered.
//
// And it must not look like one worth retrying. Measured against claude 2.1.272, every 5xx
// is retried — eight requests in sixty seconds against a condition that is permanent for
// the life of the process. The status class is the retry instruction, so a permanent local
// condition is reported in the class that stops.
func TestNoTransportIsReportedAndNotRetryable(t *testing.T) {
	g := start(t)
	resp := post(t, g, validRequest)

	if resp.StatusCode >= 500 {
		t.Fatalf("status = %d; a 5xx tells the client to keep retrying a permanent condition", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if body := bodyText(t, resp); !strings.Contains(body, "NO_UPSTREAM_TRANSPORT") {
		t.Fatalf("body = %q", body)
	}
}

// A request this build cannot serve is named before anything is sent anywhere. The hosted
// search tool is executed by the backend rather than by the client, so it is a different
// capability with its own budget and result semantics — and it must be an answer, not a
// silent success that quietly drops the tool from the list.
func TestUnsupportedCapabilityIsRefusedWithoutContactingTheBackend(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, completed)}
	g := startWith(t, fixture)

	resp := post(t, g, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],
	  "tools":[{"type":"web_search_20250305","name":"web_search"}]}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if body := bodyText(t, resp); !strings.Contains(body, "HOSTED_TOOL_UNSUPPORTED") {
		t.Fatalf("body = %q", body)
	}
	if fixture.Calls() != 0 {
		t.Fatalf("backend calls = %d; a request refused here must cost nothing upstream", fixture.Calls())
	}
}

// Before any byte of the response is written, a failure is a status the client can act on.
func TestUpstreamFailureBeforeAnyOutputIsAStatus(t *testing.T) {
	for name, tc := range map[string]struct{ stream, category string }{
		"backend reported failure": {sse(created, `{"type":"response.failed"}`), "UPSTREAM_RESPONSE_FAILED"},
		"backend error event":      {sse(created, `{"type":"error"}`), "UPSTREAM_ERROR_EVENT"},
		"malformed frame":          {"data: not json\n\n", "INVALID_SSE"},
		"unknown event":            {sse(created, `{"type":"response.output_audio.delta"}`), "UNSUPPORTED_EVENT"},
		"no terminal event":        {sse(created), "INCOMPLETE_RESPONSE"},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: tc.stream})
			resp := post(t, g, validRequest)
			if resp.StatusCode != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502", resp.StatusCode)
			}
			if body := bodyText(t, resp); !strings.Contains(body, tc.category) {
				t.Fatalf("body = %q, want %s", body, tc.category)
			}
		})
	}
}

// The client's max_tokens never reaches the backend — this wire has no parameter for it —
// so it is enforced at completion against the count the backend reports. Both answers
// arrive after text has streamed, which is what makes them terminal error events rather
// than statuses: a limit breach can only be known once the backend says what it spent.
func TestTheOutputLimitIsEnforcedAtCompletion(t *testing.T) {
	for name, tc := range map[string]struct{ completion, category string }{
		"over the caller's limit": {
			`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":5,"output_tokens":99999}}}`,
			"OUTPUT_TOKEN_LIMIT_EXCEEDED"},
		"no count to check against": {
			`{"type":"response.completed","response":{"id":"resp_1"}}`,
			"INVALID_USAGE"},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("x"), done("x"), tc.completion)})
			resp := post(t, g, validRequest)
			body := bodyText(t, resp)
			if !strings.Contains(body, "event: error") || !strings.Contains(body, tc.category) {
				t.Fatalf("body = %q, want a terminal %s", body, tc.category)
			}
			// And the response never reached message_stop: the client must not read the
			// partial answer as a finished one.
			if strings.Contains(body, "event: message_stop") {
				t.Fatalf("a refused response still ended cleanly: %s", body)
			}
		})
	}
}

// After output has been committed the status is already sent, so the only honest signal
// left is a terminal error event. Letting the stream simply stop would look to the client
// like a short answer rather than a failure.
func TestFailureAfterOutputBecomesATerminalErrorEvent(t *testing.T) {
	// A delta commits the response; the snapshot then contradicts it.
	g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("partial"), done("something else"))})

	resp := post(t, g, validRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: the response was already committed when the failure arrived", resp.StatusCode)
	}
	body := bodyText(t, resp)
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "TEXT_MISMATCH") {
		t.Fatalf("the stream ended without saying why:\n%s", body)
	}
	if !strings.Contains(body, "event: content_block_delta") {
		t.Fatalf("the committed output is missing:\n%s", body)
	}
	if strings.Contains(body, "event: message_stop") {
		t.Fatalf("a failed response claimed a clean end:\n%s", body)
	}
}

// Nothing from an upstream body reaches the client. A backend that echoes attacker bytes
// must not have them pass through into a payload another tool reads.
func TestUpstreamContentIsNotEchoedInARefusal(t *testing.T) {
	const marker = "UPSTREAM-CONTROLLED-MARKER"
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, `{"type":"response.failed","error":{"message":"`+marker+`"}}`),
	})

	resp := post(t, g, validRequest)
	if body := bodyText(t, resp); strings.Contains(body, marker) {
		t.Fatalf("upstream content was echoed: %s", body)
	}
}

// The request ceiling still applies now that the body is read for real rather than
// discarded.
func TestOversizedBodyIsStillRefused(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SSE: sse(created, completed)})
	resp := do(t, g, messages(io.LimitReader(endlessReader{}, maxRequestBytes+1024)))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

// The registry must drain whichever way a request ended.
func TestRequestsDrainAfterEveryOutcome(t *testing.T) {
	for name, tc := range map[string]struct {
		transport upstream.Transport
		body      string
	}{
		"success":        {&upstream.Fixture{SSE: sse(created, delta("x"), done("x"), completed)}, validRequest},
		"decode refusal": {&upstream.Fixture{SSE: sse(created, completed)}, `{"model":"x"}`},
		"upstream error": {&upstream.Fixture{SSE: "data: not json\n\n"}, validRequest},
		"no transport":   {nil, validRequest},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, tc.transport)
			post(t, g, tc.body)
			waitForActive(t, g, 0, "every outcome must release its admission slot")
		})
	}
}

// A transport that drops after delivering a complete-looking body has still failed, and the
// parser is told so rather than being asked to judge well-formed framing.
//
// Added after a mutation run: reporting every read end as a clean one left the suite green,
// because nothing could produce a non-EOF failure.
func TestConnectionDropAfterACompleteBodyIsNotASuccess(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE:     sse(created, delta("x"), done("x"), completed),
		ReadErr: errors.New("connection reset by peer"),
	})

	resp := post(t, g, validRequest)
	body := bodyText(t, resp)
	if resp.StatusCode == http.StatusOK && !strings.Contains(body, "TRUNCATED_STREAM") {
		t.Fatalf("a dropped connection was reported as a clean response:\n%s", body)
	}
	if resp.StatusCode != http.StatusOK && !strings.Contains(body, "TRUNCATED_STREAM") {
		t.Fatalf("status %d without naming the truncation: %s", resp.StatusCode, body)
	}
}

// --- G7: what a real transport's failures look like to the client -------------------------
//
// These matter because of a measurement, not a preference. Against claude 2.1.272 every 5xx
// is retried -- eight requests in sixty seconds and still going -- so answering a permanent
// upstream refusal with 502 buys the same answer eight times on the user's subscription.

// failingTransport refuses every request with a chosen error, standing in for a backend
// that answered badly without needing one.
type failingTransport struct{ err error }

func (f failingTransport) Execute(context.Context, upstream.Call) (*upstream.Response, error) {
	return nil, f.err
}

func TestUpstreamFailuresMapToStatusesTheClientActsOnCorrectly(t *testing.T) {
	deferred := upstream.ClassifyStatus(429,
		http.Header{"Retry-After": {"120"}}, time.Now())

	for name, tc := range map[string]struct {
		err      error
		status   int
		category string
	}{
		// A named delay. 429 is the one status this client waits on rather than hammers.
		"rate limited with a delay": {deferred, http.StatusTooManyRequests, "RATE_LIMITED"},

		// Retrying could genuinely succeed, so the class that retries is right.
		"a server fault": {
			upstream.ClassifyStatus(503, http.Header{}, time.Now()),
			http.StatusBadGateway, "UPSTREAM_HTTP_ERROR"},
		"a credential that may have been refreshed": {
			upstream.ClassifyStatus(401, http.Header{}, time.Now()),
			http.StatusBadGateway, "UNAUTHENTICATED"},

		// Permanent. Retrying spends real money to receive the same refusal.
		"a policy refusal": {
			upstream.ClassifyStatus(403, http.Header{}, time.Now()),
			http.StatusBadRequest, "UPSTREAM_HTTP_ERROR"},
		"a request the backend would not accept": {
			upstream.ClassifyStatus(400, http.Header{}, time.Now()),
			http.StatusBadRequest, "UPSTREAM_HTTP_ERROR"},
		"a certificate that does not verify": {
			upstream.Failure{Category: "TLS_VERIFICATION_FAILED", Disposition: upstream.Terminal},
			http.StatusBadRequest, "TLS_VERIFICATION_FAILED"},
		"a host that does not exist": {
			upstream.Failure{Category: "DNS_NOT_FOUND", Disposition: upstream.Terminal},
			http.StatusBadRequest, "DNS_NOT_FOUND"},

		// The credential. 503 so a client that keeps asking recovers the moment the user
		// logs in again, and retrying costs nothing upstream.
		"no credential": {
			&auth.Error{Category: auth.CategoryUnavailable},
			http.StatusServiceUnavailable, auth.CategoryUnavailable},
		"an expired token": {
			&auth.Error{Category: auth.CategoryTokenExpired},
			http.StatusServiceUnavailable, auth.CategoryTokenExpired},
		"the account changed mid-session": {
			&auth.Error{Category: auth.CategoryAccountChanged},
			http.StatusServiceUnavailable, auth.CategoryAccountChanged},

		// This machine's configuration. Asking again will not change it.
		"TLS key logging is on": {
			&auth.Error{Category: auth.CategoryRuntimeUnsupported},
			http.StatusBadRequest, auth.CategoryRuntimeUnsupported},
		"the credentials live in a keyring": {
			&auth.Error{Category: auth.CategoryStoreUnsupported},
			http.StatusBadRequest, auth.CategoryStoreUnsupported},

		// The wrapper's own refusals.
		"no budget": {upstream.ErrBudgetExhausted, http.StatusBadGateway, "UPSTREAM_FAILURE"},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, failingTransport{err: tc.err})
			resp := post(t, g, validRequest)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.status, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, tc.category) {
				t.Fatalf("body = %q, want %s", body, tc.category)
			}
		})
	}

	// The deferred case must carry its delay, or 429 is just a number.
	if deferred.RetryAfter != 2*time.Minute {
		t.Fatalf("the fixture's own delay is %v", deferred.RetryAfter)
	}
}

// Nothing a backend wrote reaches the client. Every category above is a constant chosen in
// this project, and a refusal is not a channel for backend text.
func TestAnUpstreamFailureCarriesNoBackendText(t *testing.T) {
	g := startWith(t, failingTransport{err: upstream.Failure{
		Category: "UPSTREAM_HTTP_ERROR", Disposition: upstream.Terminal, Status: 403}})
	body := bodyText(t, post(t, g, validRequest))
	for _, forbidden := range []string{"403", "Retry-After", "http"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the refusal leaked %q: %s", forbidden, body)
		}
	}
}

// --- the abort watcher ------------------------------------------------------------------

// The watcher must not expire a deadline on a connection the handler has finished with.
//
// Calling it once proves nothing: the defect was a select between two closed channels, so
// it only shows as a rate. Two thousand rounds put the odds of a silent pass past any
// number worth writing down.
func TestTheAbortWatcherLeavesAFinishedConnectionAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	close(done)
	cancel()

	fired := 0
	for i := 0; i < 2000; i++ {
		abortOnCancel(ctx, done, func() { fired++ })
	}
	if fired != 0 {
		t.Fatalf("the watcher expired the read deadline %d times in 2000 rounds after the "+
			"handler had finished. Each one resets a connection whose response is still in "+
			"the server's write buffer, so the client gets no reply at all.", fired)
	}
}

// And it must still do its job, which is the mutation that matters: deleting the watcher
// also makes the test above pass.
func TestTheAbortWatcherStillStopsAReadThatTheClientAbandoned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer close(done)

	fired := make(chan struct{})
	go abortOnCancel(ctx, done, func() { close(fired) })

	cancel()
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("a client that went away mid-body must expire the read deadline; without " +
			"that the handler sits in the read and shutdown waits for it")
	}
}

// A rate limit is answered with the status a client waits on, delay or no delay.
//
// Found by review, reproduced before fixing: a 429 whose Retry-After is absent or
// unparsable stays Retryable, and the disposition switch answered that with 502 -- the one
// class this client was measured to retry eight times in sixty seconds. The account had
// just said it was out of room and the answer told the client to hurry.
//
// The guard that was supposed to prevent this sat after the errors.As block, where every
// arm returns and the zero value has no category. It could never run.
func TestARateLimitIsNeverAnsweredWithAStatusThatMakesTheClientHurry(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		name   string
		header http.Header
		want   int
	}{
		{"with a delay", http.Header{"Retry-After": []string{"30"}}, http.StatusTooManyRequests},
		{"with no delay", http.Header{}, http.StatusTooManyRequests},
		{"with a delay it cannot read", http.Header{"Retry-After": []string{"soon"}}, http.StatusTooManyRequests},
	} {
		t.Run(c.name, func(t *testing.T) {
			failure := upstream.ClassifyStatus(http.StatusTooManyRequests, c.header, now)
			if got := statusForUpstream(failure); got != c.want {
				t.Fatalf("status = %d, want %d (disposition %v)", got, c.want, failure.Disposition)
			}
		})
	}

	// And an ordinary upstream failure keeps the answer it had: retryable is 502, terminal
	// is 400. The fix is about one category, not about the classes around it.
	retryable := upstream.ClassifyStatus(http.StatusBadGateway, http.Header{}, now)
	if got := statusForUpstream(retryable); got != http.StatusBadGateway {
		t.Fatalf("a retryable 502 is answered %d, want 502", got)
	}
	terminal := upstream.ClassifyStatus(http.StatusBadRequest, http.Header{}, now)
	if got := statusForUpstream(terminal); got != http.StatusBadRequest {
		t.Fatalf("a terminal 400 is answered %d, want 400", got)
	}
}
