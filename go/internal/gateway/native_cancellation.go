package gateway

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type nativeCancellation struct {
	identity nativeTurnReceipt
	cancel   context.CancelFunc
	record   *record
	reading  bool
	// unreadSince is when its receipt was first seen but not readable; guarded by mu.
	unreadSince time.Time
}

// errCancellationPending: a terminal receipt exists but cannot be read whole
// yet. Native writes it in place, so a reader can meet partial bytes or a
// read refused mid-write. That is not a verdict until it outlasts this grace.
var errCancellationPending = errors.New("NATIVE_CANCELLATION_PENDING")

const cancellationReadGrace = 2 * time.Second

// A filter can retain the client socket after native aborts or loses its response.
// Bind the terminal failure receipt to this exact request's turn. Unlike current
// progress, it survives a rapid follow-up; absence or inactivity never cancels.
// The request registry still owns its slot until the handler actually returns.
func (g *Gateway) bindNativeCancellation(ctx context.Context, r *http.Request, entry *record, reading bool) (context.Context, func()) {
	n := &g.nativeEvents
	if n.directory == "" {
		return ctx, func() {}
	}
	class := r.Header.Get("X-Claude-Code-Request-Class")
	// Root auxiliary is indistinguishable from an independent classifier before
	// decode. An identified child can pin its own reading turn without borrowing
	// a conversation's cancellation or replacing a sibling upload.
	if class != "main" && class != "subagent" && class != "workflow" && (class != "auxiliary" || reading && r.Header.Get("X-Claude-Code-Agent-Id") == "") {
		return ctx, func() {}
	}
	session, agent := r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id")
	if !correlationShape.MatchString(session) || agent != "" && !correlationShape.MatchString(agent) {
		return ctx, func() {}
	}
	var id nativeTurnReceipt
	if reading {
		turn, _ := g.pinNativeTurn(r, entry)
		if turn == nil {
			return ctx, func() {}
		}
		id = *turn
	} else {
		if entry == nil || entry.nativeTurn == nil {
			return ctx, func() {}
		}
		id = *entry.nativeTurn
	}
	if !validActiveReceipt(id, session, agent) {
		return ctx, func() {}
	}
	if !reading && class != "auxiliary" {
		g.cancelNativeReads(&id)
	}
	ctx, cancel := context.WithCancel(ctx)
	binding := &nativeCancellation{identity: id, cancel: cancel, record: entry, reading: reading}
	n.mu.Lock()
	if n.cancellations == nil {
		n.cancellations = map[*nativeCancellation]struct{}{}
	}
	n.cancellations[binding] = struct{}{}
	n.mu.Unlock()
	stopLease := func() bool { return false }
	if !reading && entry.nativeConfirmation != nil {
		lease := entry.nativeConfirmation.lease
		closeLease := func() { n.mu.Lock(); n.cancelLocked(binding, "native_confirmation_closed"); n.mu.Unlock() }
		stopLease = context.AfterFunc(lease, closeLease)
		if lease.Err() != nil {
			closeLease()
		}
	}
	// New-request reconciliation already ran before reading. Let a complete
	// input reach the replay ledger; checkpoints still cancel a blocked read.
	if !reading {
		g.ReconcileNativeCancellations()
	}
	return ctx, func() {
		stopLease()
		n.mu.Lock()
		delete(n.cancellations, binding)
		n.mu.Unlock()
		cancel()
	}
}

// A validated full replacement supersedes only unfinished input on this turn.
// An already decoded/dispatched request remains owned by the replay ledger.
func (g *Gateway) cancelNativeReads(id *nativeTurnReceipt) {
	if id == nil {
		return
	}
	n := &g.nativeEvents
	n.mu.Lock()
	defer n.mu.Unlock()
	for p := range n.cancellations {
		if p.reading && p.identity.Session == id.Session && p.identity.Agent == id.Agent && p.identity.Turn == id.Turn {
			n.cancelLocked(p, "native_reconnected_input")
		}
	}
}

func (n *nativeEventState) cancelLocked(p *nativeCancellation, source string) {
	if _, active := n.cancellations[p]; !active {
		return
	}
	delete(n.cancellations, p)
	if p.record != nil {
		p.record.mu.Lock()
		p.record.data.CancellationSource = source
		p.record.mu.Unlock()
	}
	p.cancel()
}

func (g *Gateway) nativeCancellationSource(id nativeTurnReceipt) (string, error) {
	var receipt nativeTurnReceipt
	found, err := g.readNativeJSON("cancel-"+id.Turn+".json", []string{"session", "agent", "turn", "reason"}, &receipt)
	if err != nil && !errors.Is(err, errDelegationUnverified) {
		return "", errCancellationPending
	}
	if !found || err != nil {
		return "", err
	}
	if receipt.Session != id.Session || receipt.Agent != id.Agent || receipt.Turn != id.Turn {
		return "", errNativeConfirmations
	}
	switch receipt.Reason {
	case "aborted":
		return "native_abort_receipt", nil
	case "error":
		return "native_error_receipt", nil
	case "refusal":
		return "native_refusal_receipt", nil
	}
	return "", errNativeConfirmations
}

// ReconcileNativeCancellations is called by the bounded session checkpoint and on new requests, not by
// model polling. Private task-owned receipts contain only identity and reason.
func (g *Gateway) ReconcileNativeCancellations() {
	n := &g.nativeEvents
	n.mu.Lock()
	pending := make([]*nativeCancellation, 0, len(n.cancellations))
	for p := range n.cancellations {
		pending = append(pending, p)
	}
	n.mu.Unlock()
	for _, p := range pending {
		source, err := g.nativeCancellationSource(p.identity)
		if errors.Is(err, errCancellationPending) {
			n.mu.Lock()
			if p.unreadSince.IsZero() {
				p.unreadSince = time.Now()
			}
			waited := time.Since(p.unreadSince)
			n.mu.Unlock()
			if waited < cancellationReadGrace {
				continue
			}
			err = errNativeConfirmations
		}
		if err != nil {
			n.confirmationFailed.Store(true)
			source = "native_cancellation_unverified"
		}
		if source == "" {
			continue
		}
		n.mu.Lock()
		n.cancelLocked(p, source)
		n.mu.Unlock()
	}
}
