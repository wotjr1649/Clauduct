package gateway

import (
	"context"
	"errors"
	"sync"
)

// The cap on requests in flight at once. PROPOSED: chosen to be far above what an
// interactive session plus its subagents produce, low enough that a runaway client cannot
// make this process hold unbounded per-request state. It is a cap, not a queue — the Node
// baseline queues up to 128 with a 30s wait under memory pressure, and that belongs where
// request bodies are actually decoded and held. Revisit when WP03 introduces that work.
const maxActiveRequests = 64

var (
	// errGatewayClosed means shutdown has begun. New work is refused before it can
	// acquire anything that would then have to be waited for.
	errGatewayClosed = errors.New("GATEWAY_CLOSED")
	// errTooManyRequests means the in-flight cap is reached.
	errTooManyRequests = errors.New("TOO_MANY_REQUESTS")
	// errShuttingDown is the cancellation cause recorded on requests killed by shutdown,
	// so a handler can tell "the user cancelled me" from "the process is going away".
	errShuttingDown = errors.New("GATEWAY_SHUTDOWN")
)

// registry tracks the requests this gateway owns and gives each one a cancellable context.
//
// Owning them individually is the point. Shutdown has to be able to stop in-flight work
// rather than wait for it, and cancelling one request must not touch its siblings — a
// subagent's turn ending must not kill the main session's. A single shared context would
// give the second property away for free in the wrong direction.
type registry struct {
	mu     sync.Mutex
	closed bool
	next   uint64
	active map[uint64]context.CancelCauseFunc
}

func newRegistry() *registry {
	return &registry{active: make(map[uint64]context.CancelCauseFunc)}
}

// admit reserves a slot and returns a context scoped to this one request.
//
// release is idempotent. Handlers run it from a defer, and cancel paths can reach the same
// entry concurrently, so "already gone" has to be ordinary rather than a panic.
func (r *registry) admit(parent context.Context) (id uint64, ctx context.Context, release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, nil, nil, errGatewayClosed
	}
	if len(r.active) >= maxActiveRequests {
		return 0, nil, nil, errTooManyRequests
	}

	r.next++
	id = r.next
	ctx, cancel := context.WithCancelCause(parent)
	r.active[id] = cancel

	var once sync.Once
	release = func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.active, id)
			r.mu.Unlock()
			// Always cancel on release so the request's own goroutines and timers stop,
			// even on the success path. A context that is only cancelled on failure is a
			// leak that looks like correct code.
			cancel(context.Canceled)
		})
	}
	return id, ctx, release, nil
}

// cancel stops one request. It reports whether that request was still registered, which is
// how a caller tells "cancelled it" from "it had already finished" — those are different
// answers and collapsing them hides late-arriving cancellations.
func (r *registry) cancel(id uint64, cause error) bool {
	r.mu.Lock()
	cancel, ok := r.active[id]
	if ok {
		delete(r.active, id)
	}
	r.mu.Unlock()

	if !ok {
		return false
	}
	cancel(cause)
	return true
}

// closeAll refuses further admissions and cancels everything currently in flight.
//
// Closing first and cancelling second matters: a request admitted between the two steps
// would never be cancelled and would hold shutdown open until the deadline.
func (r *registry) closeAll(cause error) int {
	r.mu.Lock()
	r.closed = true
	pending := make([]context.CancelCauseFunc, 0, len(r.active))
	for id, cancel := range r.active {
		pending = append(pending, cancel)
		delete(r.active, id)
	}
	r.mu.Unlock()

	for _, cancel := range pending {
		cancel(cause)
	}
	return len(pending)
}

func (r *registry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.active)
}
