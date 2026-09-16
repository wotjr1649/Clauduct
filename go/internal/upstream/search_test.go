package upstream

import (
	"context"
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
