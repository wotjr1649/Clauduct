package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// Claude Code's WebSearch is a server tool. The client does not run it: it issues an
// isolated side query carrying the search tool and reads web_search_tool_result blocks out
// of the reply. This gateway is that server, so it answers the side query itself from the
// backend's standalone search endpoint -- the same endpoint and the same credential the
// reference client uses for its own web tool. That is a search round trip, not a second
// model generation.

// searchPrompt is the exact text the client puts in front of the query. Matching it is
// what separates a side query from a conversation that merely mentions searching.
const searchPrompt = "Perform a web search for the query: "

// The bounds. Every one of them applies to content the backend returns, which is web text
// and therefore attacker-influenced by definition.
const (
	maxSearchQuery  = 2048
	maxSearchLinks  = 20
	maxSearchTitle  = 512
	maxSearchURL    = 2048
	maxSearchOutput = 100000
)

// SearchQuery is the client's side query, ready to send.
type SearchQuery struct {
	Query   string
	Allowed []string
	Blocked []string
}

// SideQuery reports the query when this request is the client's search side query.
//
// Every condition is one the client sets itself, so a near miss falls through to the
// ordinary path rather than being answered locally with something the caller did not ask
// for. That is the whole reason the test is this narrow.
func SideQuery(request *anthropic.Request) (SearchQuery, bool) {
	hosted := request.HostedSearch
	if hosted == nil || hosted.Name != "web_search" || len(request.Tools) != 0 {
		return SearchQuery{}, false
	}
	// Live requests carrying this tool reached the upstream stage, so the client does not
	// always name the tool in tool_choice. Accept the shapes it can send and refuse
	// anything pointing at a different tool, which would be a conversation.
	if choice := request.ToolChoice; choice.Present {
		if choice.Type != "auto" && !(choice.Type == "tool" && choice.Name == "web_search") {
			return SearchQuery{}, false
		}
	}
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" {
		return SearchQuery{}, false
	}
	blocks := request.Messages[0].Blocks
	if len(blocks) != 1 || blocks[0].Type != "text" {
		return SearchQuery{}, false
	}
	body := blocks[0].Text
	if !strings.HasPrefix(body, searchPrompt) {
		return SearchQuery{}, false
	}
	query := strings.TrimSpace(body[len(searchPrompt):])
	if query == "" || len(query) > maxSearchQuery {
		return SearchQuery{}, false
	}
	return SearchQuery{Query: query, Allowed: hosted.Allowed, Blocked: hosted.Blocked}, true
}

// SearchRequest is the body the backend's search endpoint reads.
//
// Only the query travels. The conversation tail the client may send with a side query is
// deliberately left out, so nothing from this machine goes with the search.
type SearchRequest struct {
	Model    string         `json:"model"`
	Input    []InputEntry   `json:"input"`
	Commands SearchCommands `json:"commands"`
	Settings SearchSettings `json:"settings"`
	MaxOut   int            `json:"max_output_tokens"`
}

type SearchCommands struct {
	SearchQuery []SearchTerm `json:"search_query"`
}

type SearchTerm struct {
	Q string `json:"q"`
}

type SearchSettings struct {
	ExternalWebAccess bool           `json:"external_web_access"`
	ContextSize       string         `json:"search_context_size"`
	AllowedCallers    []string       `json:"allowed_callers"`
	Filters           *SearchFilters `json:"filters,omitempty"`
}

type SearchFilters struct {
	Allowed []string `json:"allowed_domains,omitempty"`
	Blocked []string `json:"blocked_domains,omitempty"`
}

// BuildSearchRequest converts the side query into the reference client's request shape.
func BuildSearchRequest(model string, query SearchQuery) *SearchRequest {
	out := &SearchRequest{
		Model: model,
		Input: []InputEntry{{
			Type:    "message",
			Role:    "user",
			Content: []InputPart{{Type: "input_text", Text: query.Query}},
		}},
		Commands: SearchCommands{SearchQuery: []SearchTerm{{Q: query.Query}}},
		Settings: SearchSettings{
			ExternalWebAccess: true,
			ContextSize:       "medium",
			AllowedCallers:    []string{"direct"},
		},
		MaxOut: 2500,
	}
	if len(query.Allowed) > 0 || len(query.Blocked) > 0 {
		out.Settings.Filters = &SearchFilters{Allowed: query.Allowed, Blocked: query.Blocked}
	}
	return out
}

// SearchResult is one link the backend returned.
type SearchResult struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// SearchResults is what survived checking.
type SearchResults struct {
	Links  []SearchResult
	Output string
}

// DecodeSearchResults bounds and shape-checks every field of a search reply.
//
// This is web content. Nothing here is trusted: each field is bounded, control characters
// are refused rather than stripped, and a URL that is not http or https is dropped. A
// malformed result never silently becomes no result -- it is dropped and the reply is
// still required to carry something.
func DecodeSearchResults(raw []byte) (SearchResults, error) {
	var reply struct {
		Output  *string           `json:"output"`
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || reply.Output == nil {
		return SearchResults{}, ErrSearchResponseShape
	}
	if len(*reply.Output) > maxSearchOutput {
		return SearchResults{}, ErrSearchResponseShape
	}

	links := make([]SearchResult, 0, len(reply.Results))
	for i, entry := range reply.Results {
		if i >= maxSearchLinks {
			break
		}
		if link, ok := decodeSearchLink(entry); ok {
			links = append(links, link)
		}
	}
	// A reply with neither links nor text is a failed search, not an empty one. Say so.
	if len(links) == 0 && strings.TrimSpace(*reply.Output) == "" {
		return SearchResults{}, ErrSearchResultsEmpty
	}
	return SearchResults{Links: links, Output: *reply.Output}, nil
}

func decodeSearchLink(raw json.RawMessage) (SearchResult, bool) {
	var item struct {
		Title *string `json:"title"`
		URL   *string `json:"url"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Title == nil || item.URL == nil {
		return SearchResult{}, false
	}
	if !cleanSearchText(*item.Title, maxSearchTitle) || !cleanSearchText(*item.URL, maxSearchURL) {
		return SearchResult{}, false
	}
	parsed, err := url.Parse(*item.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return SearchResult{}, false
	}
	return SearchResult{Type: "web_search_result", Title: *item.Title, URL: *item.URL}, true
}

// cleanSearchText bounds a field and refuses the control characters that would let web
// content rewrite a terminal or split a frame.
func cleanSearchText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r <= 0x08 || r == 0x0b || r == 0x0c || (r >= 0x0e && r <= 0x1f) {
			return false
		}
	}
	return true
}

// Refusals the search path produces.
var (
	// ErrSearchResponseShape means the reply was not the shape a search answers in.
	ErrSearchResponseShape = errors.New("SEARCH_RESPONSE_SHAPE")
	// ErrSearchResultsEmpty means the reply carried neither a link nor any text. That is a
	// failed search, not an empty one, and answering it as an empty result would tell the
	// user the web had nothing to say.
	ErrSearchResultsEmpty = errors.New("SEARCH_RESULTS_EMPTY")
)

// SearchFrames builds the reply the client reduces.
//
// The client reads title and url out of the result block and takes everything else from
// the text blocks around it, so the endpoint's own digest goes into a text block after the
// result. Usage is zero on both counts and that is not a placeholder: a search round trip
// is not a model generation and reporting tokens for it would be inventing a number.
func SearchFrames(model string, query SearchQuery, results SearchResults, id func() string) []anthropic.Frame {
	useID := "srvtoolu_" + id()
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0}

	links := make([]any, 0, len(results.Links))
	for _, link := range results.Links {
		links = append(links, map[string]any{"type": link.Type, "title": link.Title, "url": link.URL})
	}

	type block struct {
		start map[string]any
		delta map[string]any
	}
	blocks := []block{
		{
			start: map[string]any{"type": "server_tool_use", "id": useID, "name": "web_search",
				"input": map[string]any{}},
			delta: map[string]any{"type": "input_json_delta",
				"partial_json": string(mustJSON(map[string]any{"query": query.Query}))},
		},
		{start: map[string]any{"type": "web_search_tool_result", "tool_use_id": useID, "content": links}},
	}
	if strings.TrimSpace(results.Output) != "" {
		blocks = append(blocks, block{
			start: map[string]any{"type": "text", "text": ""},
			delta: map[string]any{"type": "text_delta", "text": results.Output},
		})
	}

	frames := []anthropic.Frame{anthropic.Frames("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_" + id(), "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage,
		},
	})}
	for index, b := range blocks {
		frames = append(frames, anthropic.Frames("content_block_start", map[string]any{
			"type": "content_block_start", "index": index, "content_block": b.start,
		}))
		if b.delta != nil {
			frames = append(frames, anthropic.Frames("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": index, "delta": b.delta,
			}))
		}
		frames = append(frames, anthropic.Frames("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": index,
		}))
	}
	return append(frames,
		anthropic.Frames("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": usage,
		}),
		anthropic.Frames("message_stop", map[string]any{"type": "message_stop"}))
}

func mustJSON(value any) []byte {
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return out
}

// SearchID produces the identifiers the client expects: a bare thirty-two character hex
// string, which is what a UUID with its dashes removed looks like.
func SearchID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		// A failure here is a broken system RNG. An identifier that repeats is better than
		// a reply that never arrives, and nothing downstream treats it as a secret.
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(raw)
}
