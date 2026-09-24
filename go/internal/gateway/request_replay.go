package gateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Keep only fingerprints, never request bodies. A spent entry is forgotten when a newer
// turn of its agent claims, or when native reports a child's turn complete: requests are
// keyed by the turn current when they arrive, so a key under an older turn cannot match
// again, and a request that read the older turn is refused rather than keyed under it. What
// remains counts each agent's latest unfinished turn and the turnless (--bare) keys, which
// stay for the session.
const maxNativeExecutions = 16384

type nativeExecutionKey struct {
	session, agent, turn, class string
	step                        int
	body                        [32]byte
}

type nativeExecutions struct {
	sync.Mutex
	seen map[nativeExecutionKey]struct{}
	// The newest turn claimed per session and agent, by publication number. An ended turn
	// stays here for the session: a stale request of it has no bound on when it can arrive,
	// and dropping the marker would admit it against keys that are gone.
	current map[[2]string]claimedTurn
	// open is the child turns in current that have not ended, with identifiers checked when
	// stored, so the per-request retire walk visits what can still end rather than every
	// child the session has had (#109).
	open map[[2]string]string
	// How many finished child turns gave their keys back (#70).
	retired int64
}

type claimedTurn struct {
	turn     string
	sequence int
	// Native reported this turn complete. Its keys are gone, and nothing more of it is
	// admitted.
	ended bool
}

// retire forgets a child agent's turn once native has reported it complete (#70). A finished
// child never starts another turn, so its last turn's keys used to stay for the session and
// a session with enough children reached maxNativeExecutions. The turn keeps its place in
// current, marked ended, so a later request of it is refused rather than admitted against
// keys that are gone: native does not send one after the turn completes, and if it ever did,
// refusing is the side that cannot execute twice.
func (l *nativeExecutions) retire(session, agent, turn string) {
	l.Lock()
	defer l.Unlock()
	id := [2]string{session, agent}
	last, known := l.current[id]
	if !known || last.turn != turn || turn == "" {
		return // Never claimed, or a newer turn has already forgotten it.
	}
	last.ended = true
	l.current[id] = last
	delete(l.open, id)
	l.retired++
	for spent := range l.seen {
		if spent.session == session && spent.agent == agent && spent.turn == turn {
			delete(l.seen, spent)
		}
	}
}

// claim makes turn the newest of its agent. Caller holds the lock. A child's turn is also
// recorded as one that can still end; root turns and identifiers a receipt file name could
// not carry are not, since no end receipt can retire them.
func (l *nativeExecutions) claim(id [2]string, turn string, sequence int) {
	if l.current == nil {
		l.current = make(map[[2]string]claimedTurn)
	}
	l.current[id] = claimedTurn{turn: turn, sequence: sequence}
	delete(l.open, id)
	if id[1] == "" || !correlationShape.MatchString(id[1]) || !correlationShape.MatchString(turn) {
		return
	}
	if l.open == nil {
		l.open = make(map[[2]string]string)
	}
	l.open[id] = turn
}

type nativeExecution struct {
	owner      *nativeExecutions
	key        nativeExecutionKey
	dispatched bool // handler-owned, including hosted search
}

// Reserve before selection/result mutation. A retry can arrive while its first
// handler is still returning from a failed write. Retry headers cannot identify
// it: the measured native resends with X-Stainless-Retry-Count: 0.
func (g *Gateway) claimNativeExecution(r *http.Request, entry *record, body []byte, request *anthropic.Request) (*nativeExecution, string) {
	if g.nativeEvents.directory == "" {
		return nil, ""
	}
	key := nativeExecutionKey{session: r.Header.Get("X-Claude-Code-Session-Id"), agent: r.Header.Get("X-Claude-Code-Agent-Id"), class: r.Header.Get("X-Claude-Code-Request-Class"), step: -1, body: sha256.Sum256(body)}
	auxiliary := independentAuxiliary(r, request)
	var turn *nativeTurnReceipt
	if auxiliary {
		// An independent side request (a title or a classifier) owns no turn, but it is
		// spent only within the root turn it arrived in: the same bytes in a later turn
		// are a new request. Without a readable root receipt it stays spent for the
		// session, like --bare.
		if root, found, err := g.readCurrentNativeTurn(""); err == nil && found && validActiveReceipt(root, key.session, "") {
			turn = &root
		}
	} else {
		var ok bool
		if turn, ok = g.pinNativeTurn(r, entry); !ok {
			return nil, "AGENT_SELECTION_UNVERIFIED"
		}
	}
	sequence := 0
	if turn != nil {
		key.turn, sequence = turn.Turn, turn.sequence
		if !auxiliary {
			step, present, err := g.readNativeStep(key.session, key.agent)
			if err != nil || present && step.Turn != turn.Turn {
				return nil, "AGENT_SELECTION_UNVERIFIED"
			}
			if present {
				key.step = step.Index
				if key.class != "compaction" && request != nil && conversationRequest(r, request) {
					// One native step owns its control decision. A changed body or
					// conversation class cannot admit a second writer for that step.
					key.class, key.body = "conversation", [32]byte{}
				}
			}
		}
	}
	// --bare has no turn receipts. Its identical input stays spent for the whole
	// session: absence of identity must not disable replay protection or invent a
	// new turn. Normal selection/context validation still decides first admission.
	ledger := &g.executions
	ledger.Lock()
	defer ledger.Unlock()
	if key.turn != "" {
		agent := [2]string{key.session, key.agent}
		last, known := ledger.current[agent]
		if known && key.turn == last.turn && last.ended {
			return nil, "NATIVE_TURN_ENDED"
		}
		if known && key.turn != last.turn {
			if sequence <= last.sequence {
				return nil, "NATIVE_TURN_UNVERIFIED"
			}
			for spent := range ledger.seen {
				if spent.session == key.session && spent.agent == key.agent && spent.turn != "" && spent.turn != key.turn {
					delete(ledger.seen, spent)
				}
			}
		}
		if !known || sequence > last.sequence {
			ledger.claim(agent, key.turn, sequence)
		}
	}
	if _, exists := ledger.seen[key]; exists {
		return nil, "NATIVE_REQUEST_REPLAY_BLOCKED"
	}
	if len(ledger.seen) >= maxNativeExecutions {
		return nil, "NATIVE_REQUEST_CAPACITY"
	}
	if ledger.seen == nil {
		ledger.seen = make(map[nativeExecutionKey]struct{})
	}
	ledger.seen[key] = struct{}{}
	return &nativeExecution{owner: ledger, key: key}, ""
}

func independentAuxiliary(r *http.Request, request *anthropic.Request) bool {
	return request != nil && r.Header.Get("X-Claude-Code-Request-Class") == "auxiliary" && r.Header.Get("X-Claude-Code-Agent-Id") == "" && r.Header.Get("X-Claude-Code-Parent-Agent-Id") == "" && len(request.Tools) == 0 && request.HostedSearch == nil
}

func (e *nativeExecution) dispatch(ctx context.Context) error {
	// A connection may disappear before dispatch. That known local cancellation
	// must not spend the input or reach a transport that reads credentials/budget.
	// Cancellation after this boundary remains ambiguous and never permits replay.
	if err := ctx.Err(); err != nil {
		return err
	}
	if e != nil {
		e.dispatched = true
	}
	return nil
}

func (e *nativeExecution) rejectedBeforeDispatch(err error) {
	// These transport refusals prove no network request was made. Unknown
	// failures and cancellations remain spent, even if no response arrived.
	if e != nil && (errors.Is(err, upstream.ErrNoTransport) || errors.Is(err, upstream.ErrBudgetExhausted) || errors.Is(err, upstream.ErrRouteNotAuthorised)) {
		e.dispatched = false
	}
}

func (e *nativeExecution) release() {
	if e != nil && !e.dispatched {
		e.owner.Lock()
		delete(e.owner.seen, e.key)
		e.owner.Unlock()
	}
}
