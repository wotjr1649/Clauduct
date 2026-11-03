package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type nativeConfirmation struct {
	Nonce         string          `json:"nonce"`
	Session       string          `json:"session"`
	Agent         string          `json:"agent"`
	Turn          string          `json:"turn"`
	Index         int             `json:"index"`
	Search        bool            `json:"search"`
	Auxiliary     bool            `json:"auxiliary,omitempty"`
	Confirmations bool            `json:"confirmations,omitempty"`
	Unmatched     bool            `json:"unmatched,omitempty"`
	cancellation  error           // request cancelled, or unanswered in time, before a proof was admitted; never input data
	helper        context.Context // actual helper owner; never native input data
	lease         context.Context
	close         context.CancelFunc
}

// nativeToolUseID marks a backend call with the native step its proof admitted.
// The native module recomputes the same tag at tool.call: a call native runs
// after its step ended cannot use a later step's scope (#214). The tag only
// separates steps; it is not a secret. A tool call without a step proof
// (compaction, an unproven path) is refused rather than left unmarked.
func nativeToolUseID(proof *nativeConfirmation, callID string) (string, error) {
	if proof == nil || !proof.Confirmations {
		return "", anthropic.ErrUnsupportedToolCall
	}
	sum := sha256.Sum256([]byte(proof.Session + "\n" + proof.Agent + "\n" + proof.Turn + "\n" + strconv.Itoa(proof.Index)))
	return bridge.NativeToolID(callID, hex.EncodeToString(sum[:6]))
}

func anonymousNativeFork(r *http.Request) bool {
	return r.Header.Get("X-Claude-Code-Agent-Id") == "" && r.Header.Get("X-Claude-Code-Request-Class") == "subagent"
}

// One fresh live helper connection per HTTP request. Stale files cannot grant a proof.
// Serialize the mailbox handshake, never inference or the admitted live lease.
func (g *Gateway) nativeConfirmationFor(r *http.Request, turn *nativeTurnReceipt, search bool, records ...*record) *nativeConfirmation {
	return g.nativeConfirmationForWithReceipt(r, turn, search, g.nativeCancellationSource, records...)
}

type confirmationPhaseSnapshot struct {
	connected bool
	now       time.Time
	deadline  time.Time
	expired   bool
}

func (g *Gateway) confirmationPhase(pending *pendingConfirmation, prepareDeadline, queueDeadline, requestDeadline time.Time, requestBounded bool) confirmationPhaseSnapshot {
	n := &g.nativeEvents
	n.mu.Lock()
	connected, deadline := false, prepareDeadline
	if pending != nil {
		connected, deadline = pending.connected, pending.prepareDeadline
		if connected {
			deadline = pending.confirmDeadline
		}
	}
	if queueDeadline.Before(deadline) {
		deadline = queueDeadline
	}
	if requestBounded && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	now := time.Now()
	n.mu.Unlock()
	return confirmationPhaseSnapshot{connected: connected, now: now, deadline: deadline, expired: !now.Before(deadline)}
}

func (g *Gateway) nativeConfirmationForWithReceipt(r *http.Request, turn *nativeTurnReceipt, search bool,
	readReceipt func(nativeTurnReceipt) (string, error), records ...*record) (proof *nativeConfirmation) {
	n := &g.nativeEvents
	const prepareBudget = 10 * time.Second
	const confirmBudget = 2 * time.Second
	queueLimit := time.Now().Add(128 * time.Second)
	requestDeadline, requestBounded := r.Context().Deadline()
	queueCtx, cancelQueue := context.WithDeadline(r.Context(), queueLimit)
	defer cancelQueue()
	queueDeadline, _ := queueCtx.Deadline()
	var native processKey
	if id, required := g.identityOf(r); required {
		native = id.native
	}
	shared, latchRegistered := false, false
	var pending *pendingConfirmation
	var rejectSignal <-chan struct{}
	var unreadSince time.Time
	unanswered := false
	var phaseDeadline time.Time
	var activeCtx, ctx context.Context
	var cancelActive, cancelPrepare, cancelConfirm context.CancelFunc
	var prepared <-chan struct{}
	confirming := false
	var refreshPhase func() confirmationPhaseSnapshot
	receiptFailure := func(err error) {
		shared = storageFailure(err)
		if !latchRegistered {
			if shared {
				n.confirmationFailed.Store(true)
			} else {
				n.latchConfirmation(native)
			}
		}
	}
	deadlineError := func(now time.Time, requestErr error) error {
		if requestErr != nil {
			return requestErr
		}
		if requestBounded && !now.Before(requestDeadline) {
			return context.DeadlineExceeded
		}
		return errNativeConfirmationTimeout
	}
	timedOut := func() *nativeConfirmation {
		requestErr := r.Context().Err()
		if reason := deadlineError(time.Now(), requestErr); reason != errNativeConfirmationTimeout {
			return &nativeConfirmation{cancellation: reason}
		}
		if pending != nil {
			n.mu.Lock()
			answered, unidentified, prepared := pending.answered, pending.unidentified, pending.connected
			n.mu.Unlock()
			if !prepared && unreadSince.IsZero() {
				if answered || unidentified {
					return nil
				}
				unanswered = true
			}
		}
		return &nativeConfirmation{cancellation: errNativeConfirmationTimeout}
	}
	// Keep the first partial-read grace across queue and both active phases.
	cancelled := func(deadline time.Time) (*nativeConfirmation, error) {
		if err := r.Context().Err(); err != nil {
			return &nativeConfirmation{cancellation: err}, nil
		}
		var oldGrace time.Time
		if !unreadSince.IsZero() {
			oldGrace = unreadSince.Add(cancellationReadGrace)
		}
		var err error
		if turn != nil {
			var source string
			source, err = readReceipt(nativeTurnReceipt{Session: r.Header.Get("X-Claude-Code-Session-Id"), Agent: r.Header.Get("X-Claude-Code-Agent-Id"), Turn: turn.Turn})
			if err != nil && !errors.Is(err, errCancellationPending) {
				return nil, err
			}
			if source != "" && err == nil {
				return &nativeConfirmation{cancellation: context.Canceled}, nil
			}
		}
		if requestErr := r.Context().Err(); requestErr != nil {
			return &nativeConfirmation{cancellation: requestErr}, nil
		}
		now := time.Now()
		if pending != nil && refreshPhase != nil {
			phase := refreshPhase()
			deadline, now = phase.deadline, phase.now
		}
		if errors.Is(err, errCancellationPending) {
			if unreadSince.IsZero() {
				unreadSince = now
			}
			if !now.Before(deadline) {
				return &nativeConfirmation{cancellation: deadlineError(now, nil)}, nil
			}
			if !now.Before(unreadSince.Add(cancellationReadGrace)) {
				return nil, errNativeConfirmations
			}
			return nil, errCancellationPending
		}
		unreadSince = time.Time{}
		// A completed read which itself crossed the last admissible instant is
		// late, not proof of either an intact request or a damaged receipt.
		if !oldGrace.IsZero() && !now.Before(oldGrace) {
			if result := timedOut(); result != nil {
				return result, nil
			}
			return nil, errNativeConfirmations
		}
		return nil, nil
	}
	var acquiredAt time.Time
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if g.closing.Err() != nil || n.confirmationBlocked(native) {
			return nil
		}
		if result, err := cancelled(queueDeadline); result != nil || err != nil && !errors.Is(err, errCancellationPending) {
			if err != nil {
				receiptFailure(err)
			}
			return result
		}
		if !time.Now().Before(queueDeadline) {
			return timedOut()
		}
		select {
		case n.confirmationGate <- struct{}{}:
			acquiredAt = time.Now()
			defer func() { <-n.confirmationGate }()
			goto acquired
		case <-queueCtx.Done():
		case <-tick.C:
		case <-g.closing.Done():
			return nil
		}
	}

acquired:
	// This recheck precedes the latch defer as well as challenge publication.
	if result, err := cancelled(queueDeadline); result != nil || err != nil && !errors.Is(err, errCancellationPending) {
		if err != nil {
			receiptFailure(err)
		}
		return result
	}
	if g.closing.Err() != nil || n.confirmationBlocked(native) {
		return nil
	}
	if !time.Now().Before(queueDeadline) {
		return timedOut()
	}
	activeDeadline := acquiredAt.Add(prepareBudget + confirmBudget)
	if queueDeadline.Before(activeDeadline) {
		activeDeadline = queueDeadline
	}
	phaseDeadline = acquiredAt.Add(prepareBudget)
	if activeDeadline.Before(phaseDeadline) {
		phaseDeadline = activeDeadline
	}
	activeCtx, cancelActive = context.WithDeadline(queueCtx, activeDeadline)
	defer cancelActive()
	ctx, cancelPrepare = context.WithDeadline(activeCtx, phaseDeadline)
	defer cancelPrepare()
	refreshPhase = func() confirmationPhaseSnapshot {
		phase := g.confirmationPhase(pending, phaseDeadline, queueDeadline, requestDeadline, requestBounded)
		phaseDeadline = phase.deadline
		if phase.connected && !confirming {
			confirming, prepared, phaseDeadline = true, nil, phase.deadline
			ctx, cancelConfirm = context.WithDeadline(activeCtx, phaseDeadline)
			cancelPrepare()
		}
		if hooks := n.confirmationTestHooks; pending != nil && hooks != nil {
			ctxDeadline, _ := ctx.Deadline()
			if hooks.phaseDecision != nil {
				hooks.phaseDecision(phase.connected, phase.now, phase.deadline, ctxDeadline)
			}
			if !phase.connected && hooks.afterUnpreparedAdvance != nil {
				hooks.afterUnpreparedAdvance()
			}
		}
		return phase
	}
	tick.Reset(5 * time.Millisecond)
	if anonymousNativeFork(r) {
		return &nativeConfirmation{Unmatched: true}
	}
	defer func() {
		excused := false
		if pending != nil {
			n.mu.Lock()
			excused = pending.unidentified
			n.mu.Unlock()
		}
		if pending == nil && proof == nil && r.Context().Err() == nil && g.closing.Err() == nil && !excused {
			if shared {
				n.confirmationFailed.Store(true)
			} else {
				n.latchConfirmation(native)
			}
		}
	}()
	latchRegistered = true
	if n.directory == "" {
		return nil
	}
	want := nativeConfirmation{Session: r.Header.Get("X-Claude-Code-Session-Id"), Agent: r.Header.Get("X-Claude-Code-Agent-Id"), Index: -1, Search: search, Auxiliary: !search && r.Header.Get("X-Claude-Code-Request-Class") == "auxiliary"}
	if !correlationShape.MatchString(want.Session) {
		return nil
	}
	if n.sessionTainted(want.Session) {
		return &nativeConfirmation{Unmatched: true}
	}
	readStep := func() (parentStep, bool, error) {
		for {
			step, found, err := g.readNativeStep(want.Session, want.Agent)
			if !errors.Is(err, wire.ErrMalformed) {
				return step, found, err
			}
			if requestErr := r.Context().Err(); requestErr != nil {
				return step, false, requestErr
			}
			if requestBounded && !time.Now().Before(requestDeadline) {
				return step, false, context.DeadlineExceeded
			}
			if ctx.Err() != nil || !time.Now().Before(phaseDeadline) {
				return step, false, err
			}
			if result, receiptErr := cancelled(phaseDeadline); result != nil {
				if errors.Is(result.cancellation, errNativeConfirmationTimeout) {
					return step, false, err
				}
				return step, false, result.cancellation
			} else if receiptErr != nil && !errors.Is(receiptErr, errCancellationPending) {
				return step, false, receiptErr
			}
			select {
			case <-rejectSignal:
				return parentStep{}, false, errNativeConfirmations
			case <-ctx.Done():
				return step, false, err
			case <-g.closing.Done():
				return step, false, err
			case <-tick.C:
			}
		}
	}
	step, found, err := readStep()
	if errors.Is(err, errNativeStepUnmatched) {
		return &nativeConfirmation{Unmatched: true}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errNativeConfirmationTimeout) {
			return &nativeConfirmation{cancellation: err}
		}
		shared = storageFailure(err)
		return nil
	}
	if !found || turn == nil || step.Turn != turn.Turn {
		return &nativeConfirmation{Unmatched: true}
	}
	want.Turn, want.Index = step.Turn, step.Index
	want.Nonce, err = newToken()
	if err != nil {
		return nil
	}
	if result, receiptErr := cancelled(phaseDeadline); result != nil || receiptErr != nil && !errors.Is(receiptErr, errCancellationPending) {
		if receiptErr != nil {
			receiptFailure(receiptErr)
		}
		return result
	}
	if g.closing.Err() != nil {
		return nil
	}
	if !time.Now().Before(phaseDeadline) {
		return timedOut()
	}
	if len(records) > 0 {
		records[0].confirmation(want.Nonce)
	}
	lease, closeLease := context.WithCancel(r.Context())
	pending = &pendingConfirmation{want: want, reply: make(chan *nativeConfirmation, 1), rejected: make(chan struct{}), prepared: make(chan struct{}),
		prepareDeadline: phaseDeadline, confirmDeadline: activeDeadline, lease: lease, close: closeLease, native: native}
	rejectSignal = pending.rejected
	n.mu.Lock()
	if n.confirmations == nil {
		n.confirmations = map[string]*pendingConfirmation{}
	}
	n.confirmations[want.Nonce] = pending
	n.mu.Unlock()
	defer func() {
		if cancelConfirm != nil {
			cancelConfirm()
		}
	}()
	defer func() {
		n.mu.Lock()
		terminalRejected, excused := pending.isRejected(), pending.unidentified
		phaseDeadline := pending.prepareDeadline
		terminalConnected := pending.connected
		if terminalConnected {
			phaseDeadline = pending.confirmDeadline
		}
		var helperErr, leaseErr error
		if proof != nil && proof.helper != nil && proof.lease != nil {
			helperErr, leaseErr = proof.helper.Err(), proof.lease.Err()
		}
		requestErr, phaseErr, closingErr := r.Context().Err(), ctx.Err(), g.closing.Err()
		now := time.Now()
		if terminalRejected {
			proof = nil
		} else if proof != nil && proof.cancellation == nil && (proof.Confirmations || proof.Unmatched) {
			switch {
			case requestErr != nil:
				proof = &nativeConfirmation{cancellation: requestErr}
			case closingErr != nil:
				proof = nil
			case requestBounded && !now.Before(requestDeadline):
				proof = &nativeConfirmation{cancellation: context.DeadlineExceeded}
			case phaseErr != nil || !now.Before(phaseDeadline) || !now.Before(queueDeadline):
				proof = &nativeConfirmation{cancellation: errNativeConfirmationTimeout}
			case helperErr != nil || leaseErr != nil:
				proof = &nativeConfirmation{Unmatched: true}
			}
		}
		recordNativeFailure := terminalRejected || proof == nil && requestErr == nil && closingErr == nil && !excused && !shared
		if unanswered && !terminalRejected && requestErr == nil && closingErr == nil && !(requestBounded && !now.Before(requestDeadline)) &&
			!pending.connected && !pending.answered && !excused && unreadSince.IsZero() {
			n.unanswered.Add(1)
		}
		delete(n.confirmations, want.Nonce)
		n.mu.Unlock()
		if proof == nil || !proof.Confirmations {
			closeLease()
		}
		if shared {
			n.confirmationFailed.Store(true)
		}
		if recordNativeFailure {
			n.latchConfirmation(native)
		}
	}()
	body, _ := json.Marshal(want)
	if err = g.writeNativeControl("confirmation-request.json", body); err != nil {
		shared = true
		return nil
	}
	prepared = pending.prepared
	advance := func() {
		refreshPhase()
	}
	var candidate *nativeConfirmation
	for {
		advance()
		if pending.isRejected() {
			return nil
		}
		if result, receiptErr := cancelled(phaseDeadline); result != nil || receiptErr != nil && !errors.Is(receiptErr, errCancellationPending) {
			if receiptErr != nil {
				receiptFailure(receiptErr)
			}
			return result
		}
		if g.closing.Err() != nil {
			return nil
		}
		if refreshPhase().expired {
			return timedOut()
		}
		if candidate != nil {
			reply := candidate
			if reply.helper == nil || reply.lease == nil || reply.Session != want.Session || reply.Search != want.Search || reply.Auxiliary != want.Auxiliary {
				return nil
			}
			disconnected := reply.helper.Err() != nil || reply.lease.Err() != nil
			if !disconnected {
				if reply.Unmatched {
					if reply.Confirmations || reply.Agent != "" || reply.Turn != "" || reply.Index != -1 {
						return nil
					}
				} else {
					if !reply.Confirmations || reply.Agent != want.Agent || reply.Turn != want.Turn || reply.Index != want.Index {
						return nil
					}
					verifyPrepareContext, verifyCtx := !confirming, ctx
					step, found, stepErr := readStep()
					if errors.Is(stepErr, errNativeStepUnmatched) {
						if refreshPhase().expired {
							return timedOut()
						}
						return &nativeConfirmation{Unmatched: true}
					}
					if stepErr != nil {
						if pending != nil && verifyPrepareContext && !confirming && verifyCtx.Err() != nil &&
							(errors.Is(stepErr, wire.ErrMalformed) || errors.Is(stepErr, errNativeConfirmationTimeout)) {
							if requestErr := r.Context().Err(); requestErr != nil {
								return &nativeConfirmation{cancellation: requestErr}
							}
							phase := refreshPhase()
							if phase.connected && !phase.expired {
								continue
							}
							if phase.expired {
								return timedOut()
							}
						}
						if errors.Is(stepErr, context.Canceled) || errors.Is(stepErr, context.DeadlineExceeded) || errors.Is(stepErr, errNativeConfirmationTimeout) {
							return &nativeConfirmation{cancellation: stepErr}
						}
						shared = storageFailure(stepErr)
						return nil
					}
					if refreshPhase().expired {
						return timedOut()
					}
					if !found || step.Turn != reply.Turn || step.Index != reply.Index {
						return &nativeConfirmation{Unmatched: true}
					}
				}
			}
			if g.closing.Err() != nil {
				return nil
			}
			if result, receiptErr := cancelled(phaseDeadline); result != nil {
				return result
			} else if receiptErr != nil {
				if !errors.Is(receiptErr, errCancellationPending) {
					receiptFailure(receiptErr)
					return nil
				}
				select {
				case <-pending.rejected:
					return nil
				case <-ctx.Done():
				case <-tick.C:
				case <-g.closing.Done():
				}
				continue
			}
			phase := refreshPhase()
			if phase.expired {
				return timedOut()
			}
			helperErr, leaseErr := reply.helper.Err(), reply.lease.Err()
			requestErr, closingErr := r.Context().Err(), g.closing.Err()
			if requestErr != nil {
				return &nativeConfirmation{cancellation: requestErr}
			}
			if closingErr != nil {
				return nil
			}
			if requestBounded && !phase.now.Before(requestDeadline) {
				return &nativeConfirmation{cancellation: context.DeadlineExceeded}
			}
			if ctx.Err() != nil {
				return &nativeConfirmation{cancellation: errNativeConfirmationTimeout}
			}
			if disconnected || helperErr != nil || leaseErr != nil {
				return &nativeConfirmation{Unmatched: true}
			}
			proof = reply
			return proof
		}
		select {
		case reply := <-pending.reply:
			advance()
			candidate = reply
		case <-prepared:
		case <-pending.lease.Done():
			advance()
			if result, receiptErr := cancelled(phaseDeadline); result != nil || receiptErr != nil && !errors.Is(receiptErr, errCancellationPending) {
				if receiptErr != nil {
					receiptFailure(receiptErr)
				}
				return result
			} else if errors.Is(receiptErr, errCancellationPending) {
				// Retain the candidate-free request until its receipt verdict is complete.
				select {
				case <-pending.rejected:
					return nil
				case <-ctx.Done():
				case <-tick.C:
				case <-g.closing.Done():
				}
				continue
			}
			if refreshPhase().expired {
				return timedOut()
			}
			return &nativeConfirmation{Unmatched: true}
		case <-ctx.Done():
		case <-pending.rejected:
			return nil
		case <-tick.C:
		case <-g.closing.Done():
			return nil
		}
	}
}
