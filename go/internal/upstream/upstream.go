// Package upstream executes requests on the configured backend.
package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// ErrNoTransport means nothing is configured to execute the request. It is returned rather
// than falling back to anything, because a bridge that quietly answers from somewhere else
// is the failure mode this whole design exists to avoid.
var ErrNoTransport = errors.New("NO_UPSTREAM_TRANSPORT")

// Response is a backend response in progress. Body carries Server-Sent Events; the caller
// is responsible for closing it.
type Response struct {
	Body io.ReadCloser
	// RequestBytes is the transmitted JSON body length, including the session key,
	// when an HTTP 200 response was received. Zero means unmeasured, not no request.
	RequestBytes int64
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
	// Session is sent as the session-id header, from which the backend derives prompt cache
	// affinity (Codex rust-v0.157.0 client.rs), and as the body's prompt_cache_key. Body stays
	// the canonical payload that counts and the exact-input cache compare; the transport adds
	// the key when it sends. Measured on the real backend (#135): with both, a growing
	// conversation's later turns hit the cache for 71-78% of their input, against 29-47%.
	Session string
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

// None is a transport that refuses. It is the default so that a build with no transport
// configured fails loudly at the point of use rather than appearing to work.
type None struct{}

func (None) Execute(context.Context, Call) (*Response, error) { return nil, ErrNoTransport }
