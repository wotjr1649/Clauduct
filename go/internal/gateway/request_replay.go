package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

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

// spentBy is the request that first claimed a key: its record's sequence number and when it
// claimed, so a refused replay can name what it repeated (#288). Never the body or the
// fingerprint. The claim time, not the record's start: keys are claimed in the order bodies
// finish arriving, so a request that started later can own the key.
type spentBy struct {
	seq     int64
	claimed time.Time
}

type nativeExecutions struct {
	sync.Mutex
	seen map[nativeExecutionKey]spentBy
	// Only identities with live replay keys remain in memory. Before forgetting an
	// identity, its newest turn is saved in the existing per-launcher event directory.
	current map[[2]string]claimedTurn
	// open is the child turns in current that have not ended, with identifiers checked when
	// stored, so the per-request retire walk visits what can still end rather than every
	// child the session has had (#109).
	open map[[2]string]string
	// How many finished child turns gave their keys back (#70).
	retired int64
	failed  bool // a journal failure cannot make forgotten history look unused
}

type claimedTurn struct {
	turn     string
	sequence int
	// Native reported this turn complete. Its keys are gone, and nothing more of it is
	// admitted.
	ended bool
}

type savedNativeTurn struct {
	Identity string `json:"identity"`
	Turn     string `json:"turn"`
	Sequence int    `json:"sequence"`
	Ended    *bool  `json:"ended"`
}

func nativeReplayFile(id [2]string) string {
	// Hash only the owner identity to keep the filename short and inside the root.
	return fmt.Sprintf("replay-%x.json", sha256.Sum256([]byte(id[0]+"\x00"+id[1])))
}

func (g *Gateway) nativeReplayKnown(id [2]string) (bool, error) {
	root, err := os.OpenRoot(g.nativeEvents.directory)
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Stat(nativeReplayFile(id) + ".known")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return false, errDelegationUnverified
	}
	return true, nil
}

// Caller holds executions.Mutex. No request body or fingerprint is persisted.
func (g *Gateway) savedNativeTurn(id [2]string) (last claimedTurn, found bool, err error) {
	var saved savedNativeTurn
	found, err = g.readNativeJSON(nativeReplayFile(id), []string{"identity", "turn", "sequence", "ended"}, &saved)
	if err != nil {
		return last, found, err
	}
	known, err := g.nativeReplayKnown(id)
	if err != nil || known != found {
		return last, false, errDelegationUnverified
	}
	if !found {
		return last, false, nil
	}
	if saved.Identity != id[0]+"\x00"+id[1] || !correlationShape.MatchString(saved.Turn) || saved.Sequence < 1 || saved.Sequence > maxNativeSequence || saved.Ended == nil {
		return last, false, errDelegationUnverified
	}
	return claimedTurn{turn: saved.Turn, sequence: saved.Sequence, ended: *saved.Ended}, true, nil
}

func (g *Gateway) saveNativeTurn(id [2]string, last claimedTurn) error {
	// A cached live owner must not silently repair damaged saved history.
	_, known, err := g.savedNativeTurn(id)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(savedNativeTurn{Identity: id[0] + "\x00" + id[1], Turn: last.turn, Sequence: last.sequence, Ended: &last.ended})
	if err != nil {
		return err
	}
	if err := g.writeNativeControl(nativeReplayFile(id), raw); err != nil {
		return err
	}
	if !known {
		// Absence of a previously saved state must never mean first use again.
		return g.writeNativeControl(nativeReplayFile(id)+".known", nil)
	}
	return nil
}

// Save an ended marker before releasing memory. Delayed requests still read it.
func (g *Gateway) retireNativeExecution(session, agent, turn string) {
	l := &g.executions
	l.Lock()
	defer l.Unlock()
	if l.failed {
		return
	}
	id := [2]string{session, agent}
	last, known := l.current[id]
	if !known {
		var err error
		last, known, err = g.savedNativeTurn(id)
		if err != nil {
			l.failed = true
			return
		}
	}
	if !known || last.turn != turn || turn == "" || last.ended {
		return // Never claimed, or a newer turn has already forgotten it.
	}
	last.ended = true
	if g.saveNativeTurn(id, last) != nil {
		l.failed = true
		return // Keep all in-memory history if storage could not preserve it.
	}
	delete(l.current, id)
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
	owner      *Gateway
	key        nativeExecutionKey
	dispatched bool // handler-owned, including hosted search
	// unkeyed is an execution the ledger does not hold (the auto mode classifier, #289). It
	// still records dispatch, which is what tells a failed reply not to be retried (#84).
	unkeyed bool
}

// Reserve before selection/result mutation. A retry can arrive while its first
// handler is still returning from a failed write. Retry headers cannot identify
// it: the measured native resends with X-Stainless-Retry-Count: 0.
func (g *Gateway) claimNativeExecution(r *http.Request, entry *record, body []byte, request *anthropic.Request) (*nativeExecution, string) {
	if g.nativeEvents.directory == "" {
		return nil, ""
	}
	key := nativeExecutionKey{session: r.Header.Get("X-Claude-Code-Session-Id"), agent: r.Header.Get("X-Claude-Code-Agent-Id"), class: r.Header.Get("X-Claude-Code-Request-Class"), step: -1, body: sha256.Sum256(body)}
	if g.nativeEvents.confirmationsRequired && anonymousNativeFork(r) {
		return nil, errNativeOriginUnverified.Error()
	}
	auxiliary := independentAuxiliary(r, request)
	// The auto mode classifier asks about an action; it is not one. Native asks it again in
	// the same bytes when sibling agents take the same action at once, and a refusal left the
	// second action unrun (#288). It keeps no replay key (#289), so native's own second ask
	// after an unreadable verdict reaches the backend too.
	if auxiliary {
		if classifier, err := autoModeClassifier(request, false); classifier && err == nil {
			return &nativeExecution{owner: g, key: key, unkeyed: true}, ""
		}
	}
	var turn *nativeTurnReceipt
	if auxiliary {
		// An independent side request (a title or a classifier) owns no turn, but it is
		// spent only within the root turn it arrived in: the same bytes in a later turn
		// are a new request. Without a readable root receipt it stays spent for the
		// session, like --bare.
		if root, found, err := g.readCurrentNativeTurn(""); err == nil && found && validActiveReceipt(g.selection, root, key.session, "") {
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
					// Keep the fingerprint too: a delayed old body cannot own a new step.
					key.class = "conversation"
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
	if ledger.failed {
		return nil, "NATIVE_REPLAY_STATE_UNVERIFIED"
	}
	agent := [2]string{key.session, key.agent}
	last, known := ledger.current[agent]
	retiring := 0
	if key.turn != "" {
		if !known {
			var err error
			last, known, err = g.savedNativeTurn(agent)
			if err != nil {
				ledger.failed = true
				return nil, "NATIVE_REPLAY_STATE_UNVERIFIED"
			}
			// A pre-dispatch release may leave no keys to reconcile. Check the
			// native end receipt before reopening that same saved child turn.
			if known && !last.ended && key.agent != "" && key.turn == last.turn {
				var end nativeTurnReceipt
				found, err := g.readNativeReceipt("end-"+key.agent+"-"+key.turn+".json", &end)
				if err != nil || found && (end.Session != key.session || end.Agent != key.agent || end.Turn != key.turn) {
					return nil, "NATIVE_TURN_UNVERIFIED"
				}
				if found {
					switch end.Reason {
					case "answer", "aborted", "refusal", "error":
						last.ended = true
						if g.saveNativeTurn(agent, last) != nil {
							ledger.failed = true
							return nil, "NATIVE_REPLAY_STATE_UNVERIFIED"
						}
					default:
						return nil, "NATIVE_TURN_UNVERIFIED"
					}
				}
			}
		}
		if known && key.turn == last.turn && last.ended {
			return nil, "NATIVE_TURN_ENDED"
		}
		if known && key.turn != last.turn {
			if sequence <= last.sequence {
				return nil, "NATIVE_TURN_UNVERIFIED"
			}
			for spent := range ledger.seen {
				if spent.session == key.session && spent.agent == key.agent && spent.turn != "" && spent.turn != key.turn {
					retiring++
				}
			}
		}
	}
	if prior, exists := ledger.seen[key]; exists {
		entry.replayOf(prior, "same_key")
		return nil, "NATIVE_REQUEST_REPLAY_BLOCKED"
	}
	if key.class == "conversation" {
		// ponytail: scan at most 16,384 existing keys, without another cache.
		// Index by turn if measured admission cost requires it.
		// Every match is read so the record names the earliest one, not whichever map order
		// happened to reach first.
		var first spentBy
		match := ""
		for spent, prior := range ledger.seen {
			if spent.class == key.class && spent.session == key.session && spent.agent == key.agent && spent.turn == key.turn && (spent.step == key.step || spent.body == key.body) {
				if match == "" || prior.seq < first.seq {
					first, match = prior, map[bool]string{true: "same_step", false: "same_body"}[spent.step == key.step]
				}
			}
		}
		if match != "" {
			entry.replayOf(first, match)
			return nil, "NATIVE_REQUEST_REPLAY_BLOCKED"
		}
	}
	if len(ledger.seen)-retiring >= maxNativeExecutions {
		return nil, "NATIVE_REQUEST_CAPACITY"
	}
	if key.turn != "" {
		if retiring > 0 {
			for spent := range ledger.seen {
				if spent.session == key.session && spent.agent == key.agent && spent.turn != "" && spent.turn != key.turn {
					delete(ledger.seen, spent)
				}
			}
		}
		ledger.claim(agent, key.turn, max(sequence, last.sequence))
	}
	if ledger.seen == nil {
		ledger.seen = make(map[nativeExecutionKey]spentBy)
	}
	ledger.seen[key] = entry.spentBy()
	return &nativeExecution{owner: g, key: key}, ""
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
	if e != nil && (errors.Is(err, upstream.ErrNoTransport) || errors.Is(err, upstream.ErrBudgetExhausted) || errors.Is(err, upstream.ErrRouteNotAuthorised) || deferredLocally(err)) {
		e.dispatched = false
	}
}

// deferredLocally is the transport refusing before a socket because a delay the backend named
// has not passed (#91).
func deferredLocally(err error) bool {
	var failure upstream.Failure
	return errors.As(err, &failure) && failure.Category == upstream.RetryDeferred
}

func (e *nativeExecution) release() {
	if e != nil && !e.dispatched && !e.unkeyed {
		l := &e.owner.executions
		l.Lock()
		defer l.Unlock()
		delete(l.seen, e.key)
		if l.failed || e.key.turn == "" {
			return
		}
		id := [2]string{e.key.session, e.key.agent}
		last, held := l.current[id]
		if !held {
			return
		}
		for spent := range l.seen {
			if spent.session == id[0] && spent.agent == id[1] {
				return
			}
		}
		if e.owner.saveNativeTurn(id, last) != nil {
			l.failed = true
			return
		}
		delete(l.current, id)
		delete(l.open, id)
	}
}
