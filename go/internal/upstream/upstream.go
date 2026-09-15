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
}

// Transport executes one backend request.
//
// The context governs cancellation: a caller that gives up must be able to stop work
// rather than wait for it, and an implementation that ignores the context turns a
// cancelled request into one that runs to completion unobserved.
type Transport interface {
	Execute(ctx context.Context, body []byte) (*Response, error)
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
	// ReadErr is returned in place of io.EOF once the body has been delivered. A
	// connection that drops after a complete-looking body is still a failed transfer, and
	// without a way to produce one nothing checks that the difference is noticed.
	ReadErr error

	calls    atomic.Int64
	lastBody atomic.Value
}

// Execute returns the canned body. It records the request so a test can assert what the
// bridge actually asked for rather than what it meant to ask for.
func (f *Fixture) Execute(ctx context.Context, body []byte) (*Response, error) {
	f.calls.Add(1)
	f.lastBody.Store(string(body))

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.Err != nil {
		return nil, f.Err
	}
	return &Response{Body: io.NopCloser(&replay{source: f.SSE, size: f.ChunkSize, end: f.ReadErr})}, nil
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

func (None) Execute(context.Context, []byte) (*Response, error) { return nil, ErrNoTransport }
