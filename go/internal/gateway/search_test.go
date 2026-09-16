package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The client's side query, in the shape it actually sends it.
func sideQuery(query string) string {
	return `{"model":"gpt-6-astra","max_tokens":1024,"stream":true,
	  "tools":[{"type":"web_search_20250305","name":"web_search"}],
	  "messages":[{"role":"user","content":"Perform a web search for the query: ` + query + `"}]}`
}

const searchAnswer = `{"output":"Go 1.27.1 was released on 2026-09-01.","results":[
  {"title":"Release History","url":"https://go.dev/doc/devel/release"},
  {"title":"Go 1.27 Release Notes","url":"https://go.dev/doc/go1.27"}]}`

// A2. WebSearch is a server tool: the client issues a side query and reads the result
// blocks out of the reply. This gateway is that server.
func TestASearchSideQueryIsAnsweredFromTheSearchEndpoint(t *testing.T) {
	fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]"), SearchJSON: searchAnswer}
	g := startWith(t, fixture)

	resp := post(t, g, sideQuery("go 1.27.1 release"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, bodyText(t, resp))
	}
	body := bodyText(t, resp)

	// It is a search round trip, not a second model generation.
	if fixture.Searches() != 1 {
		t.Fatalf("searches = %d, want 1", fixture.Searches())
	}
	if fixture.Calls() != 0 {
		t.Fatalf("the side query reached the model path %d times", fixture.Calls())
	}

	// Only the query travels. Nothing from this machine goes with the search.
	sent := fixture.LastSearch()
	if !strings.Contains(sent, `"q":"go 1.27.1 release"`) {
		t.Fatalf("the query did not reach the endpoint: %s", sent)
	}
	// Exactly the query, with nothing appended. The conversation tail a side query can
	// carry is deliberately left behind, so anything extra in this field came from here.
	if !strings.Contains(sent, `"text":"go 1.27.1 release"`) {
		t.Fatalf("the search input is not exactly the query: %s", sent)
	}
	for _, want := range []string{`"external_web_access":true`, `"search_context_size":"medium"`,
		`"allowed_callers":["direct"]`, `"max_output_tokens":2500`, `"model":"gpt-6-astra"`} {
		if !strings.Contains(sent, want) {
			t.Errorf("search request missing %s: %s", want, sent)
		}
	}

	// The blocks the client reduces.
	for _, want := range []string{
		"event: message_start", `"type":"server_tool_use"`, `"name":"web_search"`,
		`"type":"web_search_tool_result"`, `"type":"web_search_result"`,
		`"url":"https://go.dev/doc/devel/release"`, `"title":"Go 1.27 Release Notes"`,
		"event: content_block_delta", `"type":"input_json_delta"`,
		`Go 1.27.1 was released`, "event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reply missing %q in:\n%s", want, body)
		}
	}
	// The server_tool_use id ties the call to its result, and the client pairs them by that
	// value. Two different ids leave the result belonging to no call.
	call := between(body, `"id":"srvtoolu_`, `"`)
	result := between(body, `"tool_use_id":"srvtoolu_`, `"`)
	if call == "" || result == "" || call != result {
		t.Errorf("the call id %q and the result id %q do not match:\n%s", call, result, body)
	}
	// A search is not a generation, so the token counts are zero rather than invented.
	if !strings.Contains(body, `"input_tokens":0`) || !strings.Contains(body, `"output_tokens":0`) {
		t.Errorf("usage was reported for a search round trip:\n%s", body)
	}
}

// Domain filters travel; nothing else about the request does.
func TestSearchDomainFiltersReachTheEndpoint(t *testing.T) {
	fixture := &upstream.Fixture{SearchJSON: searchAnswer}
	g := startWith(t, fixture)

	resp := post(t, g, `{"model":"gpt-6-astra","max_tokens":16,"stream":true,
	  "tools":[{"type":"web_search_20250305","name":"web_search",
	    "allowed_domains":["go.dev"],"blocked_domains":["example.invalid"]}],
	  "messages":[{"role":"user","content":"Perform a web search for the query: generics"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	sent := fixture.LastSearch()
	if !strings.Contains(sent, `"allowed_domains":["go.dev"]`) ||
		!strings.Contains(sent, `"blocked_domains":["example.invalid"]`) {
		t.Fatalf("filters did not travel: %s", sent)
	}
}

// Anything that is not exactly the side query is not answered locally.
//
// The refusal is deliberate and diverges from the Node baseline, which drops the tool and
// answers from the model without search results. A WebSearch that quietly returns nothing
// is worse than one that says it broke, because only the second one gets fixed.
func TestOnlyTheClientsOwnSideQueryShapeIsAnswered(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"a second tool alongside it", `{"model":"gpt-6-astra","max_tokens":16,"stream":true,
		  "tools":[{"type":"web_search_20250305","name":"web_search"},
		           {"name":"Read","description":"d","input_schema":{"type":"object"}}],
		  "messages":[{"role":"user","content":"Perform a web search for the query: x"}]}`},
		{"a conversation that merely mentions searching", `{"model":"gpt-6-astra","max_tokens":16,
		  "stream":true,"tools":[{"type":"web_search_20250305","name":"web_search"}],
		  "messages":[{"role":"user","content":"please search for x"}]}`},
		{"two turns", `{"model":"gpt-6-astra","max_tokens":16,"stream":true,
		  "tools":[{"type":"web_search_20250305","name":"web_search"}],
		  "messages":[{"role":"user","content":"Perform a web search for the query: x"},
		              {"role":"assistant","content":"ok"}]}`},
		{"a tool_choice the client never sends with a side query", `{"model":"gpt-6-astra",
		  "max_tokens":16,"stream":true,
		  "tools":[{"type":"web_search_20250305","name":"web_search"}],
		  "tool_choice":{"type":"any"},
		  "messages":[{"role":"user","content":"Perform a web search for the query: x"}]}`},
		{"an empty query", `{"model":"gpt-6-astra","max_tokens":16,"stream":true,
		  "tools":[{"type":"web_search_20250305","name":"web_search"}],
		  "messages":[{"role":"user","content":"Perform a web search for the query:    "}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			fixture := &upstream.Fixture{SSE: sse(created, completed, "[DONE]"), SearchJSON: searchAnswer}
			g := startWith(t, fixture)

			resp := post(t, g, c.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", resp.StatusCode, bodyText(t, resp))
			}
			if body := bodyText(t, resp); !strings.Contains(body, "HOSTED_TOOL_UNSUPPORTED") {
				t.Fatalf("body = %s", body)
			}
			if fixture.Searches() != 0 {
				t.Errorf("a request that is not the side query was searched %d times", fixture.Searches())
			}
			if fixture.Calls() != 0 {
				t.Errorf("it reached the model path %d times", fixture.Calls())
			}
		})
	}
}

// Web content is not trusted. Each field is bounded and shape-checked, and a malformed
// result is dropped rather than passed on.
func TestSearchResultsFromTheWebAreNotTrusted(t *testing.T) {
	long := strings.Repeat("a", 3000)
	fixture := &upstream.Fixture{SearchJSON: `{"output":"summary","results":[
	  {"title":"good","url":"https://example.com/a"},
	  {"title":"scripturl","url":"javascript:alert(1)"},
	  {"title":"ftpurl","url":"ftp://example.com/x"},
	  {"title":"no url"},
	  {"url":"https://example.com/no-title"},
	  {"title":"control\u0007char","url":"https://example.com/b"},
	  {"title":"` + long + `","url":"https://example.com/c"},
	  {"title":"also good","url":"http://example.com/d"}]}`}
	g := startWith(t, fixture)

	resp := post(t, g, sideQuery("anything"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	body := bodyText(t, resp)

	for _, gone := range []string{"javascript:", "ftp://", "no-title", "control", long} {
		if strings.Contains(body, gone) {
			t.Errorf("a result that should have been dropped reached the client: %q", gone)
		}
	}
	for _, kept := range []string{"https://example.com/a", "http://example.com/d"} {
		if !strings.Contains(body, kept) {
			t.Errorf("a valid result was dropped: %q", kept)
		}
	}
}

// A reply with neither a link nor any text is a failed search, not an empty one.
func TestASearchWithNothingInItIsAFailureNotAnEmptyResult(t *testing.T) {
	for name, answer := range map[string]string{
		"no links and no text": `{"output":"   ","results":[]}`,
		"only unusable links":  `{"output":"","results":[{"title":"x","url":"javascript:1"}]}`,
		"no output field":      `{"results":[]}`,
		"not an object":        `["nope"]`,
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SearchJSON: answer})
			resp := post(t, g, sideQuery("anything"))
			if resp.StatusCode == http.StatusOK {
				t.Fatalf("a failed search was answered as a result: %s", bodyText(t, resp))
			}
			body := bodyText(t, resp)
			if !strings.Contains(body, "SEARCH_RESULTS_EMPTY") && !strings.Contains(body, "SEARCH_RESPONSE_SHAPE") {
				t.Fatalf("body = %s", body)
			}
		})
	}
}

// A transport that cannot search says so rather than answering out of nothing.
func TestATransportThatCannotSearchSaysSo(t *testing.T) {
	g := startWith(t, cannotSearch{})
	resp := post(t, g, sideQuery("anything"))
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501: %s", resp.StatusCode, bodyText(t, resp))
	}
	if body := bodyText(t, resp); !strings.Contains(body, "SEARCH_UNSUPPORTED") {
		t.Fatalf("body = %s", body)
	}
}

// A transport with no search of its own. Deliberately not a Fixture, which has one.
type cannotSearch struct{}

func (cannotSearch) Execute(context.Context, upstream.Call) (*upstream.Response, error) {
	return nil, errors.New("the model path must not be reached by a side query")
}

// The reply must be readable as the frames the client parses.
func TestTheSearchReplyIsWellFormedSSE(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SearchJSON: searchAnswer})
	resp := post(t, g, sideQuery("x"))
	body := bodyText(t, resp)

	frames := 0
	for _, chunk := range strings.Split(strings.TrimSpace(body), "\n\n") {
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") ||
			!strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("malformed frame %q in:\n%s", chunk, body)
		}
		var decoded struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &decoded); err != nil {
			t.Fatalf("frame data is not JSON: %v in %q", err, chunk)
		}
		if decoded.Type != strings.TrimPrefix(lines[0], "event: ") {
			t.Fatalf("event name %q does not match data type %q", lines[0], decoded.Type)
		}
		frames++
	}
	// message_start, three blocks of start/delta/stop or start/stop, message_delta,
	// message_stop. Fewer than eight means a block went missing.
	if frames < 8 {
		t.Fatalf("%d frames, want at least 8:\n%s", frames, body)
	}
}

// between returns the text after prefix up to the next end marker.
func between(body, prefix, end string) string {
	i := strings.Index(body, prefix)
	if i < 0 {
		return ""
	}
	rest := body[i+len(prefix):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
