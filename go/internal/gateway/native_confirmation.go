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
	cancellation  error           // request cancelled before a proof was admitted; never input data
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
// ponytail: serialize the short local handshake (64 admitted model requests),
// not inference. One fixed challenge file bounds storage over a long session.
func (g *Gateway) nativeConfirmationFor(r *http.Request, turn *nativeTurnReceipt, search bool) (proof *nativeConfirmation) {
	n := &g.nativeEvents
	// A queued request does not own the mailbox yet. Bound the whole queue by
	// the admitted request count, while cancellation and shutdown remain immediate.
	const handshakeTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), maxActiveRequests*handshakeTimeout)
	defer cancel()
	select {
	case n.confirmationGate <- struct{}{}:
		defer func() { <-n.confirmationGate }()
	case <-ctx.Done():
		if r.Context().Err() != nil {
			return &nativeConfirmation{cancellation: r.Context().Err()}
		}
		return nil
	case <-g.closing.Done():
		return nil
	}
	if n.confirmationFailed.Load() || ctx.Err() != nil || g.closing.Err() != nil {
		return nil
	}
	cancelled := func() (*nativeConfirmation, error) {
		if err := r.Context().Err(); err != nil {
			return &nativeConfirmation{cancellation: err}, nil
		}
		if turn != nil {
			source, err := g.nativeCancellationSource(nativeTurnReceipt{Session: r.Header.Get("X-Claude-Code-Session-Id"), Agent: r.Header.Get("X-Claude-Code-Agent-Id"), Turn: turn.Turn})
			if errors.Is(err, errCancellationPending) {
				return nil, nil // Checked again on the next tick; the module also refuses cancelled turns.
			}
			if err != nil {
				return nil, err
			}
			if source != "" {
				return &nativeConfirmation{cancellation: context.Canceled}, nil
			}
		}
		return nil, nil
	}
	ctx, finishHandshake := context.WithTimeout(ctx, handshakeTimeout)
	defer finishHandshake()
	// An active sibling is not proof of which child sent an anonymous request.
	if anonymousNativeFork(r) {
		return &nativeConfirmation{Unmatched: true}
	}
	defer func() {
		if proof == nil && r.Context().Err() == nil && g.closing.Err() == nil {
			n.confirmationFailed.Store(true)
		}
	}()
	if n.directory == "" {
		return nil // No confirmation module was configured for this launcher.
	}
	want := nativeConfirmation{Session: r.Header.Get("X-Claude-Code-Session-Id"), Agent: r.Header.Get("X-Claude-Code-Agent-Id"), Index: -1, Search: search, Auxiliary: !search && r.Header.Get("X-Claude-Code-Request-Class") == "auxiliary"}
	if !correlationShape.MatchString(want.Session) {
		return nil
	}
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	// Native overwrites its fixed step in place. Wait for a complete document
	// within the same request deadline; never authorize from partial bytes.
	readStep := func() (parentStep, bool, error) {
		for {
			step, found, err := g.readNativeStep(want.Session, want.Agent)
			if !errors.Is(err, wire.ErrMalformed) {
				return step, found, err
			}
			select {
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
	lease, closeLease := context.WithCancel(r.Context())
	pending := &pendingConfirmation{want: want, reply: make(chan *nativeConfirmation, 1), lease: lease, close: closeLease}
	n.mu.Lock()
	if n.confirmations == nil {
		n.confirmations = map[string]*pendingConfirmation{}
	}
	n.confirmations[want.Nonce] = pending
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		delete(n.confirmations, want.Nonce)
		n.mu.Unlock()
		if proof == nil || !proof.Confirmations {
			closeLease()
		}
	}()
	body, _ := json.Marshal(want)
	if err = g.writeNativeControl("confirmation-request.json", body); err != nil {
		return nil
	}
	for {
		select {
		case reply := <-pending.reply:
			if reply.helper == nil || reply.Session != want.Session || reply.Search != want.Search || reply.Auxiliary != want.Auxiliary {
				return nil
			}
			if reply.helper != nil && reply.helper.Err() != nil || reply.lease.Err() != nil {
				if proof, err := cancelled(); proof != nil || err != nil {
					return proof
				}
				return &nativeConfirmation{Unmatched: true}
			}
			// A policy-verified negative answer about a stale/missing scope rejects
			// this request only. Storage/policy failures still latch the launcher.
			if reply.Unmatched {
				if reply.Confirmations || reply.Agent != "" || reply.Turn != "" || reply.Index != -1 {
					return nil
				}
				return reply
			}
			if !reply.Confirmations {
				return nil
			}
			if reply.Agent != want.Agent || reply.Turn != want.Turn || reply.Index != want.Index {
				return nil
			}
			step, found, err := readStep()
			if errors.Is(err, errNativeStepUnmatched) {
				return &nativeConfirmation{Unmatched: true}
			}
			if err != nil || ctx.Err() != nil || g.closing.Err() != nil {
				return nil
			}
			if !found || step.Turn != reply.Turn || step.Index != reply.Index {
				return &nativeConfirmation{Unmatched: true}
			}
			if proof, err := cancelled(); proof != nil || err != nil {
				return proof
			}
			if reply.helper != nil && reply.helper.Err() != nil || reply.lease.Err() != nil {
				return &nativeConfirmation{Unmatched: true}
			}
			proof = reply
			return proof
		case <-pending.lease.Done():
			// A returned helper stream ends this request, not the launcher's policy.
			if proof, err := cancelled(); proof != nil || err != nil {
				return proof
			}
			return &nativeConfirmation{Unmatched: true}
		case <-ctx.Done():
			proof, _ := cancelled()
			return proof
		case <-tick.C:
			if proof, err := cancelled(); proof != nil || err != nil {
				return proof
			}
		case <-g.closing.Done():
			return nil
		}
	}
}
