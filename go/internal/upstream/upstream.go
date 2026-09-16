// Package upstream is where a backend request is executed.
//
// WP03 ships the interface and a fixture that replays canned bytes. There is no network
// client here and no credential reading: a real transport is WP05, and until it exists
// this module cannot reach a provider even by mistake. That is a structural claim, not an
// observation about a particular run.
package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
)

// ErrNoTransport means nothing is configured to execute the request. It is returned rather
// than falling back to anything, because a bridge that quietly answers from somewhere else
// is the failure mode this whole design exists to avoid.
var ErrNoTransport = errors.New("NO_UPSTREAM_TRANSPORT")

// Response is a backend response in progress. Body carries Server-Sent Events; the caller
// is responsible for closing it.
type Response struct {
	Body io.ReadCloser
	// Header is what the backend sent alongside the body, or nil from a transport that has
	// none. The rate limit observation reads six numeric fields out of it and nothing else;
	// it is kept whole here rather than digested because a transport is not the place to
	// decide which of them a reader is allowed to see.
	Header http.Header
}

// Call is one backend request together with how its route was decided.
//
// The route travels with the call because it belongs to the call. It used to be a pair of
// fields on the transport, which is only true when every request in a session runs on the
// same model -- and none do: a single `claude -p` run sends the conversation on the model
// the user chose and a session title on whatever the client picks for that. The product
// transport was built with both fields empty and reserved every attempt against a blank
// route, so the ledger counted the spending without recording what it was spent on.
type Call struct {
	// Body is the encoded backend request, exactly as it will be sent.
	Body []byte
	// Requested is the model the client named, before any alias or family rule.
	Requested string
	// Model and Effort are what the backend will actually run, which is what the user is
	// billed for. CAP03 and H09 ask for these kept apart from Requested rather than
	// collapsed into one "model" field: a session that silently ran something other than
	// what was asked for is exactly the thing a reader needs to be able to see.
	Model, Effort string
	// Source names the rule that produced Model — catalogue, alias, family or direct. A
	// reader who sees a surprising model needs to know which rule produced it.
	Source string
}

// Searcher is a transport that can answer the client's search side query.
//
// Optional on purpose. A fixture that replays one inference has no business pretending it
// can reach a search endpoint, and a gateway that finds a transport cannot search says so
// rather than answering the query out of nothing.
type Searcher interface {
	Search(ctx context.Context, body []byte) ([]byte, error)
}

// Transport executes one backend request.
//
// The context governs cancellation: a caller that gives up must be able to stop work
// rather than wait for it, and an implementation that ignores the context turns a
// cancelled request into one that runs to completion unobserved.
type Transport interface {
	Execute(ctx context.Context, call Call) (*Response, error)
}

// Fixture replays a fixed byte sequence. It exists so the whole pipeline can be driven end
// to end without a network, which is what makes the protocol tests cheap enough to run on
// every change.
type Fixture struct {
	// SSE is the response body to replay.
	SSE string
	// Err, when set, is returned instead of a body.
	Err error
	// ChunkSize splits the body across reads. Zero delivers it whole. Setting it to 1
	// drives the parser one byte at a time, which is how the framing tests establish that
	// chunk boundaries carry no meaning.
	ChunkSize int
	// SearchJSON is what Search replays. Empty makes a search request an error, which is
	// the right default for a fixture that was not set up to answer one.
	SearchJSON string
	// SearchErr, when set, is returned instead of SearchJSON.
	SearchErr error
	// ReadErr is returned in place of io.EOF once the body has been delivered. A
	// connection that drops after a complete-looking body is still a failed transfer, and
	// without a way to produce one nothing checks that the difference is noticed.
	ReadErr error

	calls      atomic.Int64
	searches   atomic.Int64
	lastSearch atomic.Value
	lastBody   atomic.Value
}

// Execute returns the canned body. It records the request so a test can assert what the
// bridge actually asked for rather than what it meant to ask for.
func (f *Fixture) Execute(ctx context.Context, call Call) (*Response, error) {
	f.calls.Add(1)
	f.lastBody.Store(string(call.Body))

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.Err != nil {
		return nil, f.Err
	}
	return &Response{Body: io.NopCloser(&replay{source: f.SSE, size: f.ChunkSize, end: f.ReadErr})}, nil
}

// Search replays a canned search answer and records the request.
func (f *Fixture) Search(ctx context.Context, body []byte) ([]byte, error) {
	f.searches.Add(1)
	f.lastSearch.Store(string(body))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.SearchErr != nil {
		return nil, f.SearchErr
	}
	if f.SearchJSON == "" {
		return nil, ErrSearchUnavailable
	}
	return []byte(f.SearchJSON), nil
}

// Searches reports how many search round trips this fixture answered.
func (f *Fixture) Searches() int64 { return f.searches.Load() }

// LastSearch reports the most recent search request body.
func (f *Fixture) LastSearch() string {
	if value, ok := f.lastSearch.Load().(string); ok {
		return value
	}
	return ""
}

// Calls reports how many times this fixture was asked to execute. A budget claim rests on
// a count, so the count is kept where the test can read it.
func (f *Fixture) Calls() int64 { return f.calls.Load() }

// LastRequest reports the most recent request body.
func (f *Fixture) LastRequest() string {
	if value, ok := f.lastBody.Load().(string); ok {
		return value
	}
	return ""
}

// replay hands out the body, optionally a fixed number of bytes at a time, and ends with
// io.EOF or with a chosen failure.
type replay struct {
	source string
	size   int
	end    error
	offset int
}

func (r *replay) Read(p []byte) (int, error) {
	if r.offset >= len(r.source) {
		if r.end != nil {
			return 0, r.end
		}
		return 0, io.EOF
	}
	limit := len(r.source)
	if r.size > 0 && r.offset+r.size < limit {
		limit = r.offset + r.size
	}
	n := copy(p, r.source[r.offset:limit])
	r.offset += n
	return n, nil
}

// None is a transport that refuses. It is the default so that a build with no transport
// configured fails loudly at the point of use rather than appearing to work.
type None struct{}

func (None) Execute(context.Context, Call) (*Response, error) { return nil, ErrNoTransport }
