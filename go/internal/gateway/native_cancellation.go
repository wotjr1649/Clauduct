package gateway

import (
	"context"
	"net/http"
)

type nativeCancellation struct {
	identity nativeTurnReceipt
	cancel   context.CancelFunc
	record   *record
	reading  bool
}

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
	if class != "main" && class != "subagent" && class != "workflow" {
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
	if !reading {
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
	// New-request reconciliation already ran before reading. Let a complete
	// input reach the replay ledger; checkpoints still cancel a blocked read.
	if !reading {
		g.ReconcileNativeCancellations()
	}
	return ctx, func() {
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

// Called by the existing bounded session checkpoint and on new requests, not by
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
		var receipt nativeTurnReceipt
		found, err := g.readNativeJSON("cancel-"+p.identity.Turn+".json", []string{"session", "agent", "turn", "reason"}, &receipt)
		if !found || err != nil || receipt.Session != p.identity.Session || receipt.Agent != p.identity.Agent || receipt.Turn != p.identity.Turn {
			continue
		}
		var source string
		switch receipt.Reason {
		case "aborted":
			source = "native_abort_receipt"
		case "error":
			source = "native_error_receipt"
		case "refusal":
			source = "native_refusal_receipt"
		default:
			continue
		}
		n.mu.Lock()
		n.cancelLocked(p, source)
		n.mu.Unlock()
	}
}
