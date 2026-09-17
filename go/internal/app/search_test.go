package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// searchingScript is a Script that can also answer the client's search side query.
//
// A Script on its own cannot: the gateway does not send a side query through Execute, it
// hands it to a Searcher, and a fixture with no search endpoint makes the gateway say
// SEARCH_UNSUPPORTED rather than answer out of nothing.
type searchingScript struct {
	*upstream.Script
	answer string

	mu   sync.Mutex
	sent []string
}

func (s *searchingScript) Search(_ context.Context, body []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, string(body))
	return []byte(s.answer), nil
}

func (s *searchingScript) searches() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
}

// WebSearch, from the client asking to the model reading the answer.
//
// Everything about search had been checked one layer at a time: the detector and the block
// synthesis against fixtures, the endpoint against the real backend with a probe. The layer
// nobody had run is the one the user actually uses -- the client deciding to search, the
// side query arriving here, and the results getting back into the conversation. It costs no
// model call, because the model is scripted and the search endpoint is a fixture.
func TestAWebSearchRoundTripsThroughTheBridge(t *testing.T) {
	const title = "Example release notes"
	const link = "https://example.org/release-notes"
	const query = "example release date"

	script := newScript(
		toolStream("call_search_1", "WebSearch", `{"query":"`+query+`"}`),
		textStream("resp_done", "done"))
	backend := &searchingScript{
		Script: script,
		answer: `{"output":"The example was released on a day.","results":[` +
			`{"title":"` + title + `","url":"` + link + `"}]}`,
	}

	scriptedOn(t, script, backend, []string{"-p", "search the web for it", "--allowedTools", "WebSearch"})

	sent := backend.searches()
	if len(sent) == 0 {
		t.Fatal("the client never issued a search side query, so nothing about search was measured")
	}
	if !strings.Contains(sent[0], query) {
		t.Fatalf("the query did not reach the search endpoint: %s", tail(sent[0], 300))
	}

	// And the answer got back into the conversation. This is the half a fixture cannot
	// fake: the client had to accept the synthesised blocks, reduce them to a tool result,
	// and send that result back through this bridge.
	conversations := script.Conversations()
	if len(conversations) < 2 {
		t.Fatalf("conversation requests = %d, want the call and its result", len(conversations))
	}
	withResult := conversations[len(conversations)-1]
	for _, want := range []string{title, link} {
		if !strings.Contains(withResult, want) {
			t.Fatalf("the search result never reached the model: %q missing; tail: %s",
				want, tail(withResult, 400))
		}
	}
}
