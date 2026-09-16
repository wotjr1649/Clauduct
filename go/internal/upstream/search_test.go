package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// searchListener records the path as well as the headers, because where the search went is
// half of what is being checked.
type searchListener struct {
	*listener
	path atomic.Value
}

func serveSearch(t *testing.T, l *listener) *searchListener {
	t.Helper()
	tracked := &searchListener{listener: serve(t, l)}
	inner := tracked.Server.Config.Handler
	tracked.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracked.path.Store(r.URL.Path)
		inner.ServeHTTP(w, r)
	})
	return tracked
}

func (s *searchListener) Path() string {
	value, _ := s.path.Load().(string)
	return value
}

// A2. The side query goes to the backend's own search endpoint, not to the inference one.
//
// Derived from the inference address rather than written out, so the two can never end up
// on different hosts, and the derivation is checked rather than assumed.
func TestASearchGoesToTheSearchEndpointAndNotTheInferenceOne(t *testing.T) {
	l := serveSearch(t, &listener{payload: `{"output":"ok","results":[]}`})
	d := direct(t, l.listener, credentialStore(t, false), approved())
	d.endpoint = l.URL + "/responses"

	raw, err := d.Search(context.Background(), []byte(`{"model":"gpt-5.6-luna"}`))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if string(raw) != `{"output":"ok","results":[]}` {
		t.Fatalf("body = %s", raw)
	}
	if got := l.Path(); got != "/alpha/search" {
		t.Fatalf("the search went to %q. The inference path is /responses and sending a "+
			"search there would ask the model to answer it.", got)
	}

	header, _ := l.last.Load().(http.Header)
	// The reference client's search identity. originator is codex_exec here and
	// codex_cli_rs on an inference request: two different callers to the same backend.
	for name, want := range map[string]string{
		"Originator":   "codex_exec",
		"Accept":       "application/json",
		"Content-Type": "application/json",
	} {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if agent := header.Get("User-Agent"); !strings.HasPrefix(agent, "codex_exec/") {
		t.Errorf("User-Agent = %q, want the search client's", agent)
	}
	// A search is not an inference. The budget counts inferences, and spending one here
	// would make a cap stated in inferences stop meaning that.
	if attempts, inferences, _ := d.Ledger.Spent(); attempts != 0 || inferences != 0 {
		t.Errorf("the ledger charged %d attempts and %d inferences for a search round trip",
			attempts, inferences)
	}
}

// An endpoint this cannot derive a search address from is refused, not guessed at.
func TestASearchAddressIsRefusedRatherThanGuessed(t *testing.T) {
	l := serveSearch(t, &listener{payload: `{"output":"ok"}`})
	d := direct(t, l.listener, credentialStore(t, false), approved())
	d.endpoint = l.URL + "/something-else"

	_, err := d.Search(context.Background(), []byte(`{}`))
	if !errors.Is(err, ErrSearchEndpoint) {
		t.Fatalf("Search = %v, want %v", err, ErrSearchEndpoint)
	}
	if l.hits.Load() != 0 {
		t.Fatalf("it sent the search anyway, %d times", l.hits.Load())
	}
}

// A gone endpoint ends the feature; an unwell one is worth exactly one more try.
func TestASearchRetriesOnceAndOnlyWhenItCouldPass(t *testing.T) {
	for _, c := range []struct {
		name    string
		status  int
		hits    int64
		wantErr string
	}{
		{"a server fault is worth one more try", http.StatusBadGateway, 2, "SEARCH_HTTP_ERROR"},
		{"so is being rate limited", http.StatusTooManyRequests, 2, "SEARCH_HTTP_ERROR"},
		{"an endpoint that is gone is not", http.StatusNotFound, 1, "SEARCH_UNAVAILABLE"},
		{"neither is one that has been retired", http.StatusGone, 1, "SEARCH_UNAVAILABLE"},
		{"a refused request is not retried into a loop", http.StatusBadRequest, 1, "SEARCH_HTTP_ERROR"},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := serveSearch(t, &listener{status: c.status, payload: `{}`})
			d := direct(t, l.listener, credentialStore(t, false), approved())
			d.endpoint = l.URL + "/responses"

			_, err := d.Search(context.Background(), []byte(`{}`))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Search = %v, want %s", err, c.wantErr)
			}
			if got := l.hits.Load(); got != c.hits {
				t.Fatalf("%d requests, want %d. A search is an idempotent read, so one "+
					"retry cannot duplicate an effect -- and one is the limit, because a "+
					"side query the client is waiting on is not where a retry budget goes.",
					got, c.hits)
			}
		})
	}
}

// A reply that is not JSON is named as such rather than handed on.
func TestASearchReplyThatIsNotJSONIsRefused(t *testing.T) {
	l := serveSearch(t, &listener{payload: "not json at all"})
	d := direct(t, l.listener, credentialStore(t, false), approved())
	d.endpoint = l.URL + "/responses"

	_, err := d.Search(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "SEARCH_RESPONSE_SHAPE") {
		t.Fatalf("Search = %v, want SEARCH_RESPONSE_SHAPE", err)
	}
}

// The identity the endpoint expects, which offline agreement could not have shown.
//
// Measured 2026-09-16: without these the endpoint answered HTTP 400. It is not a 404, so
// the address and the credential were right and the body was not -- and every offline test
// passed the whole time, because a fixture accepts whatever it is handed.
func TestASearchCarriesTheIdentityTheEndpointExpects(t *testing.T) {
	l := serveSearch(t, &listener{payload: `{"output":"ok","results":[]}`})
	d := direct(t, l.listener, credentialStore(t, false), approved())
	d.endpoint = l.URL + "/responses"

	if _, err := d.Search(context.Background(), []byte(`{"model":"gpt-5.6-luna"}`)); err != nil {
		t.Fatalf("Search: %v", err)
	}
	sent, _ := l.body.Load().(string)

	// First in the body, which is where the reference client puts it. The order is kept
	// rather than re-encoded: this endpoint has already refused one body it did not
	// recognise, and there is no reason to differ from the client it was built for.
	if !strings.HasPrefix(sent, `{"id":"`) {
		t.Fatalf("no session identity leads the body: %s", sent)
	}
	if !strings.Contains(sent, `"model":"gpt-5.6-luna"`) {
		t.Fatalf("the caller's body did not survive the injection: %s", sent)
	}

	header, _ := l.last.Load().(http.Header)
	raw := header.Get("x-codex-turn-metadata")
	if raw == "" {
		t.Fatal("the turn envelope was not sent")
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("the turn envelope is not JSON: %v", err)
	}
	for _, field := range []string{"installation_id", "session_id", "thread_id", "agent_name",
		"turn_id", "root_turn_id", "window_id", "window_number", "context_window_id",
		"request_kind", "thread_source", "sandbox", "sandbox_mode", "auto_review_enabled",
		"node_repl_auto_review_required", "node_repl_disabled", "turn_started_at_unix_ms"} {
		if _, present := envelope[field]; !present {
			t.Errorf("the turn envelope has no %s", field)
		}
	}
	// Nothing in it describes this machine. It is generated per call and says so.
	if envelope["agent_name"] != "/root" || envelope["sandbox"] != "none" {
		t.Errorf("the envelope does not match the reference client's: %v", envelope)
	}
}

// One session per transport, a fresh turn per request. A new session every query would
// describe every search as the first one.
func TestASearchKeepsOneSessionAndMintsAFreshTurn(t *testing.T) {
	l := serveSearch(t, &listener{payload: `{"output":"ok","results":[]}`})
	d := direct(t, l.listener, credentialStore(t, false), approved())
	d.endpoint = l.URL + "/responses"

	identity := func() (string, string) {
		if _, err := d.Search(context.Background(), []byte(`{"model":"m"}`)); err != nil {
			t.Fatalf("Search: %v", err)
		}
		sent, _ := l.body.Load().(string)
		header, _ := l.last.Load().(http.Header)
		var envelope map[string]any
		_ = json.Unmarshal([]byte(header.Get("x-codex-turn-metadata")), &envelope)
		turn, _ := envelope["turn_id"].(string)
		return sent[:len(`{"id":"`)+36], turn
	}

	firstSession, firstTurn := identity()
	secondSession, secondTurn := identity()
	if firstSession != secondSession {
		t.Errorf("the session changed between queries: %q then %q", firstSession, secondSession)
	}
	if firstTurn == "" || firstTurn == secondTurn {
		t.Errorf("the turn did not change between queries: %q then %q", firstTurn, secondTurn)
	}
}
