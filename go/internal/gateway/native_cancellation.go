package gateway

import (
	"context"
	"net/http"
)

type nativeCancellation struct {
	identity nativeTurnReceipt
	cancel   context.CancelFunc
	record   *record
}

// A filter can retain the client socket after Esc. Bind the independent native
// abort receipt to this exact request's turn; absence or inactivity never cancels.
// The request registry still owns its slot until the handler actually returns.
func (g *Gateway) bindNativeCancellation(ctx context.Context, r *http.Request, entry *record) (context.Context, func()) {
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
	name := "active-main.json"
	if agent != "" {
		name = "active-" + agent + ".json"
	}
	var id nativeTurnReceipt
	found, err := g.readNativeReceipt(name, &id)
	if !found || err != nil || id.Session != session || id.Agent != agent || !correlationShape.MatchString(id.Turn) || id.Reason != "" {
		return ctx, func() {}
	}
	if agent != "" && !validActiveReceipt(id, session, agent) {
		return ctx, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	binding := &nativeCancellation{identity: id, cancel: cancel, record: entry}
	n.mu.Lock()
	if n.cancellations == nil {
		n.cancellations = map[*nativeCancellation]struct{}{}
	}
	n.cancellations[binding] = struct{}{}
	n.mu.Unlock()
	g.ReconcileNativeCancellations()
	return ctx, func() {
		n.mu.Lock()
		delete(n.cancellations, binding)
		n.mu.Unlock()
		cancel()
	}
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
		if !found || err != nil || receipt.Session != p.identity.Session || receipt.Agent != p.identity.Agent || receipt.Turn != p.identity.Turn || receipt.Reason != "aborted" {
			continue
		}
		n.mu.Lock()
		_, active := n.cancellations[p]
		if active {
			delete(n.cancellations, p)
			if p.record != nil {
				p.record.mu.Lock()
				p.record.data.CancellationSource = "native_abort_receipt"
				p.record.mu.Unlock()
			}
			p.cancel()
		}
		n.mu.Unlock()
	}
}
