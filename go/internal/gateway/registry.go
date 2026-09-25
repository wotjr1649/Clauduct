package gateway

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Generation and counting retain the independent cap on per-request state.
// Control uploads have their own reserved budget, even when all these slots are busy.
const maxActiveRequests = 64

const (
	modelAdmission = iota
	controlAdmission
	modelReserveBase      = 16 << 20
	modelReserveFactor    = 24
	controlRequestReserve = 256 << 20
	admissionPoll         = 250 * time.Millisecond
)

var (
	// errGatewayClosed means shutdown has begun. New work is refused before it can
	// acquire anything that would then have to be waited for.
	errGatewayClosed = errors.New("GATEWAY_CLOSED")
	errMemoryBudget  = errors.New("MEMORY_BUDGET_EXCEEDED")
	errMemoryQueue   = errors.New("MEMORY_QUEUE_FULL")
	errMemoryTimeout = errors.New("MEMORY_ADMISSION_TIMEOUT")
	errMemoryStatus  = errors.New("MEMORY_STATUS_UNAVAILABLE")
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
	mu               sync.Mutex
	closed           bool
	next             uint64
	active           map[uint64]context.CancelCauseFunc
	budget, headroom uint64
	pools            [2]admissionPool
	changed          chan struct{}
	// The OS boundary is replaceable in tests; production always reads Windows.
	memory func() (physical, available uint64, err error)
}

type admissionPool struct {
	budget, reserved      uint64
	inFlight              int
	queue                 []*admissionWaiter
	maxQueued             int
	maxWait               time.Duration
	queuedTotal, timedOut int64
}

type admissionWaiter struct{ entered time.Time }

func newRegistry(physical uint64) *registry {
	budget := min(physical/8, 4<<30)
	control := min(budget/2, 512<<20)
	return &registry{
		active: make(map[uint64]context.CancelCauseFunc), changed: make(chan struct{}),
		budget: budget, headroom: max(256<<20, min(physical/10, 1<<30)), memory: systemMemory,
		pools: [2]admissionPool{
			{budget: budget - control, maxQueued: 128, maxWait: 30 * time.Second},
			{budget: control, maxQueued: 16, maxWait: 500 * time.Millisecond},
		},
	}
}

// admit waits before reading a body. wait can also end on a native turn receipt;
// the admitted request's parent stays separate so ending that wait cannot cancel
// the execution that just acquired its reservation.
func (r *registry) admit(parent, wait context.Context, bytes uint64, class int) (id uint64, ctx context.Context, release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, nil, nil, errGatewayClosed
	}
	if err := wait.Err(); err != nil {
		return 0, nil, nil, err
	}
	if err := parent.Err(); err != nil {
		return 0, nil, nil, err
	}
	p := &r.pools[class]
	if bytes > p.budget {
		return 0, nil, nil, errMemoryBudget
	}
	if len(p.queue) == 0 {
		room, err := r.room(bytes, class)
		if err != nil {
			return 0, nil, nil, err
		}
		if room {
			return r.reserve(parent, bytes, class)
		}
	}
	if len(p.queue) >= p.maxQueued {
		return 0, nil, nil, errMemoryQueue
	}
	waiter := &admissionWaiter{entered: time.Now()}
	p.queue = append(p.queue, waiter)
	p.queuedTotal++
	defer func() {
		// ponytail: linear removal is bounded by the 128/16-entry queues.
		p.queue = slices.DeleteFunc(p.queue, func(w *admissionWaiter) bool { return w == waiter })
		r.notify()
	}()
	deadline := waiter.entered.Add(p.maxWait)
	timer := time.NewTimer(p.maxWait)
	defer timer.Stop()
	poll := time.NewTicker(admissionPoll)
	defer poll.Stop()
	for {
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-parent.Done():
		case <-wait.Done():
		case <-timer.C:
		case <-changed:
		case <-poll.C:
		}
		r.mu.Lock()
		if r.closed {
			return 0, nil, nil, errGatewayClosed
		}
		if err := wait.Err(); err != nil {
			return 0, nil, nil, err
		}
		if err := parent.Err(); err != nil {
			return 0, nil, nil, err
		}
		if !time.Now().Before(deadline) {
			p.timedOut++
			return 0, nil, nil, errMemoryTimeout
		}
		// FIFO prevents a large request being starved by smaller newcomers.
		if p.queue[0] != waiter {
			continue
		}
		room, err := r.room(bytes, class)
		if err != nil {
			return 0, nil, nil, err
		}
		if room {
			return r.reserve(parent, bytes, class)
		}
	}
}

// Called with mu held. Reservations also count against current free memory: some
// may already be allocated, but subtracting all of them leaves conservative room
// for admitted handlers that have not decoded their bodies yet. This is an
// admission estimate, not an RSS cap or a cross-process reservation ledger.
func (r *registry) room(bytes uint64, class int) (bool, error) {
	p := &r.pools[class]
	if bytes > p.budget-p.reserved || class == modelAdmission && p.inFlight >= maxActiveRequests {
		return false, nil
	}
	_, available, err := r.memory()
	if err != nil {
		return false, errMemoryStatus
	}
	reserved := r.pools[0].reserved + r.pools[1].reserved
	return available >= reserved+bytes+r.headroom, nil
}

// Called with mu held. Only release gives memory back: cancelling a context may
// still leave its handler decoding or cleaning up. release is idempotent.
func (r *registry) reserve(parent context.Context, bytes uint64, class int) (id uint64, ctx context.Context, release func(), err error) {
	r.next++
	id = r.next
	ctx, cancel := context.WithCancelCause(parent)
	r.active[id] = cancel
	r.pools[class].reserved += bytes
	r.pools[class].inFlight++

	var once sync.Once
	release = func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.active, id)
			r.pools[class].reserved -= bytes
			r.pools[class].inFlight--
			r.notify()
			r.mu.Unlock()
			// Always cancel on release so the request's own goroutines and timers stop,
			// even on the success path. A context that is only cancelled on failure is a
			// leak that looks like correct code.
			cancel(context.Canceled)
		})
	}
	return id, ctx, release, nil
}

// Called with mu held. Waiters own their timers; no background poll survives them.
func (r *registry) notify() { close(r.changed); r.changed = make(chan struct{}) }

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
	r.notify()
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

type AdmissionReport struct {
	BudgetBytes   uint64              `json:"budgetBytes"`
	HeadroomBytes uint64              `json:"headroomBytes"`
	Models        AdmissionPoolReport `json:"models"`
	Control       AdmissionPoolReport `json:"control"`
}

type AdmissionPoolReport struct {
	BudgetBytes   uint64 `json:"budgetBytes"`
	ReservedBytes uint64 `json:"reservedBytes"`
	InFlight      int    `json:"inFlight"`
	Queued        int    `json:"queued"`
	MaxQueued     int    `json:"maxQueued"`
	MaxWaitMs     int64  `json:"maxWaitMs"`
	OldestWaitMs  int64  `json:"oldestWaitMs"`
	QueuedTotal   int64  `json:"queuedTotal"`
	TimedOut      int64  `json:"timedOut"`
}

func (r *registry) report() AdmissionReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	var pools [2]AdmissionPoolReport
	for i, p := range r.pools {
		pools[i] = AdmissionPoolReport{BudgetBytes: p.budget, ReservedBytes: p.reserved,
			InFlight: p.inFlight, Queued: len(p.queue), MaxQueued: p.maxQueued,
			MaxWaitMs: p.maxWait.Milliseconds(), QueuedTotal: p.queuedTotal, TimedOut: p.timedOut}
		if len(p.queue) != 0 {
			pools[i].OldestWaitMs = time.Since(p.queue[0].entered).Milliseconds()
		}
	}
	return AdmissionReport{BudgetBytes: r.budget, HeadroomBytes: r.headroom, Models: pools[0], Control: pools[1]}
}
