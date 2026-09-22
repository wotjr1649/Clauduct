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

// Keep only fingerprints, never request bodies. Spent entries are not evicted:
// forgetting one would allow a delayed transport retry to execute it again.
const maxNativeExecutions = 16384

type nativeExecutionKey struct {
	session, agent, turn, class string
	step                        int
	body                        [32]byte
}

type nativeExecutions struct {
	sync.Mutex
	seen map[nativeExecutionKey]struct{}
}

type nativeExecution struct {
	owner      *nativeExecutions
	key        nativeExecutionKey
	dispatched bool // handler-owned, including hosted search
}

// Reserve before selection/result mutation. A retry can arrive while its first
// handler is still returning from a failed write. Retry headers cannot identify
// it: the measured native resends with X-Stainless-Retry-Count: 0.
func (g *Gateway) claimNativeExecution(r *http.Request, body []byte, request *anthropic.Request) (*nativeExecution, string) {
	if g.nativeEvents.directory == "" {
		return nil, ""
	}
	key := nativeExecutionKey{session: r.Header.Get("X-Claude-Code-Session-Id"), agent: r.Header.Get("X-Claude-Code-Agent-Id"), class: r.Header.Get("X-Claude-Code-Request-Class"), step: -1, body: sha256.Sum256(body)}
	if !independentAuxiliary(r, request) {
		turn, found, err := g.readCurrentNativeTurn(key.agent)
		if err != nil || found && !validActiveReceipt(turn, key.session, key.agent) {
			return nil, "AGENT_SELECTION_UNVERIFIED"
		}
		if found {
			key.turn = turn.Turn
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
