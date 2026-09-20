package app

import (
	"context"
	"encoding/json"
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

type childSearchFixture struct {
	mu          sync.Mutex
	root, child int
	searches    []string
	done        chan struct{}
}

func (f *childSearchFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }
func (f *childSearchFixture) Search(_ context.Context, raw []byte) ([]byte, error) {
	var request struct{ Model string }
	_ = json.Unmarshal(raw, &request)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches = append(f.searches, request.Model)
	if len(f.searches) == 1 {
		close(f.done)
	}
	return []byte(`{"output":"Public fixture","results":[{"title":"Public search proof","url":"https://example.org/proof"}]}`), nil
}
func (f *childSearchFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	reply := textStream("side", "done")
	if upstream.Conversation(string(call.Body)) {
		f.mu.Lock()
		if call.Model == "gpt-5.6-sol" {
			f.child++
			step := f.child
			f.mu.Unlock()
			if step == 1 {
				reply = toolStream("discover_search", "ToolSearch", `{"query":"select:WebSearch","max_results":1}`)
			} else if step == 2 {
				reply = toolStream("search_public", "WebSearch", `{"query":"public synthetic search proof"}`)
			} else {
				reply = textStream("child_report", "Findings: public proof. Evidence: https://example.org/proof. Unverified: live web.")
			}
		} else {
			f.root++
			step := f.root
			f.mu.Unlock()
			if step == 1 {
				reply = toolStream("spawn_search", "Agent", `{"subagent_type":"searcher","description":"public search","prompt":"Search public synthetic evidence","model":"gpt-5.6-sol","effort":"high"}`)
			} else {
				select {
				case <-f.done:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				reply = textStream("parent_report", "done")
			}
		}
	}
	return (&upstream.Fixture{SSE: countedFixtureReply(reply, call, 1000)}).Execute(ctx, call)
}
func TestNativeChildWebSearchUsesConfirmedSelectionWithExactPolicy(t *testing.T) {
	buildHook(t)
	f := &childSearchFixture{done: make(chan struct{})}
	out := (nativeRun{Args: []string{"-p", "delegate public search", "--allowedTools", "Agent,ToolSearch,WebSearch", "--agents", `{"searcher":{"description":"public search fixture","prompt":"Use public search","tools":["ToolSearch","WebSearch"],"model":"gpt-5.6-luna","effort":"medium"}}`}, transport: f, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("child search failed exit=%d err=%v records=%+v", out.result.NativeExitCode, out.err, out.result.Diagnostics.Recent)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.searches) != 1 || f.searches[0] != "gpt-5.6-sol" {
		t.Fatalf("confirmed search model missing: %v", f.searches)
	}
	verified := false
	for _, r := range out.result.Diagnostics.Recent {
		if r.Kind == "web_search" {
			verified = r.AgentID != "" && r.SelectionVerified != nil && *r.SelectionVerified && r.Model == "gpt-5.6-sol" && r.Effort == "high"
		}
	}
	if !verified {
		t.Fatal("search agent provenance missing")
	}
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
		toolStream("discover_search", "ToolSearch", `{"query":"select:WebSearch","max_results":1}`),
		toolStream("call_search_1", "WebSearch", `{"query":"`+query+`"}`),
		textStream("resp_done", "done"))
	backend := &searchingScript{
		Script: script,
		answer: `{"output":"The example was released on a day.","results":[` +
			`{"title":"` + title + `","url":"` + link + `"}]}`,
	}

	scriptedOn(t, script, backend, []string{"-p", "search the web for it", "--allowedTools", "ToolSearch,WebSearch"})

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
