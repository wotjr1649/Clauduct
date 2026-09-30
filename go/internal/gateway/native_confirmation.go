package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type nativeConfirmation struct {
	Nonce         string `json:"nonce"`
	Session       string `json:"session"`
	Agent         string `json:"agent"`
	Turn          string `json:"turn"`
	Index         int    `json:"index"`
	Search        bool   `json:"search"`
	Confirmations bool   `json:"confirmations,omitempty"`
	Unmatched     bool   `json:"unmatched,omitempty"`
}

func anonymousNativeFork(r *http.Request) bool {
	return r.Header.Get("X-Claude-Code-Agent-Id") == "" && r.Header.Get("X-Claude-Code-Request-Class") == "subagent"
}

// One fresh reply per HTTP request: stale files never prove a failed write.
// ponytail: serialize the short local handshake (64 admitted model requests),
// not inference. Three fixed mailbox files bound storage over a long session.
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
		return nil
	case <-g.closing.Done():
		return nil
	}
	if n.confirmationFailed || ctx.Err() != nil || g.closing.Err() != nil {
		return nil
	}
	ctx, finishHandshake := context.WithTimeout(ctx, handshakeTimeout)
	defer finishHandshake()
	// An active sibling is not proof of which child sent an anonymous request.
	if anonymousNativeFork(r) {
		return &nativeConfirmation{Unmatched: true}
	}
	defer func() {
		if proof == nil && r.Context().Err() == nil && g.closing.Err() == nil {
			n.confirmationFailed = true
		}
	}()
	if n.directory == "" {
		return nil // No confirmation module was configured for this launcher.
	}
	want := nativeConfirmation{Session: r.Header.Get("X-Claude-Code-Session-Id"), Agent: r.Header.Get("X-Claude-Code-Agent-Id"), Index: -1, Search: search}
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
	if errors.Is(err, errNativeStepUnmatched) || errors.Is(err, wire.ErrMalformed) {
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
	body, _ := json.Marshal(want)
	if err = g.writeNativeControl("confirmation-request.json", body); err != nil {
		return nil
	}
	for {
		var ready struct {
			Nonce string `json:"nonce"`
		}
		published, readyErr := g.readNativeJSON("confirmation-ready.json", []string{"nonce"}, &ready)
		var reply nativeConfirmation
		var found bool
		if readyErr == nil && published && ready.Nonce == want.Nonce {
			found, err = g.readNativeJSON("confirmation-reply.json", []string{"nonce", "session", "agent", "turn", "index", "search", "confirmations", "unmatched"}, &reply)
		}
		// Native writes in place. A partial document is never admitted, but may
		// finish before the fixed deadline. The ready nonce is written only after
		// the body is complete, so a mixed old/new body cannot authorize early.
		if err == nil && found && reply.Nonce == want.Nonce {
			if reply.Session != want.Session || reply.Search != want.Search {
				return nil
			}
			// A policy-verified negative answer about a stale/missing scope rejects
			// this request only. Storage/policy failures still latch the launcher.
			if reply.Unmatched {
				if reply.Confirmations || reply.Agent != "" || reply.Turn != "" || reply.Index != -1 {
					return nil
				}
				return &reply
			}
			if !reply.Confirmations {
				return nil
			}
			if reply.Agent != want.Agent || reply.Turn != want.Turn || reply.Index != want.Index {
				return nil
			}
			step, found, err := readStep()
			if errors.Is(err, errNativeStepUnmatched) || errors.Is(err, wire.ErrMalformed) {
				return &nativeConfirmation{Unmatched: true}
			}
			if err != nil || ctx.Err() != nil || g.closing.Err() != nil {
				return nil
			}
			if !found || step.Turn != reply.Turn || step.Index != reply.Index {
				return &nativeConfirmation{Unmatched: true}
			}
			return &reply
		}
		select {
		case <-ctx.Done():
			return nil
		case <-g.closing.Done():
			return nil
		case <-tick.C:
		}
	}
}
