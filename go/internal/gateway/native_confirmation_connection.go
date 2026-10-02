package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type pendingConfirmation struct {
	want      nativeConfirmation
	reply     chan *nativeConfirmation
	lease     context.Context
	close     context.CancelFunc
	connected bool // nativeEvents.mu; a nonce is consumed once
}

// Authenticated by handle; never an upstream request or an execution approval.
// The same connection remains open until its model request ends or native returns
// the helper stream. A disconnected helper cannot publish another confirmation.
func (g *Gateway) handleNativeConfirmation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isJSON(r.Header.Get("Content-Type")) {
		g.refuse(w, refuseHeader)
		return
	}
	control := http.NewResponseController(w)
	_ = control.SetReadDeadline(time.Now().Add(3 * time.Second))
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	_ = control.SetReadDeadline(time.Time{}) // The body budget must not expire a live lease.
	var reply nativeConfirmation
	if err != nil {
		g.refuse(w, refuseHeader)
		return
	}
	if _, err = wire.Fields(raw, []string{"nonce", "session", "agent", "turn", "index", "search", "auxiliary", "confirmations", "unmatched"}); err != nil || json.Unmarshal(raw, &reply) != nil {
		g.refuse(w, refuseHeader)
		return
	}
	n := &g.nativeEvents
	n.mu.Lock()
	p := n.confirmations[reply.Nonce]
	if p == nil || p.lease.Err() != nil {
		// The request this nonce served has already ended or been refused. The helper
		// is late, not wrong: say so, so the module does not latch policy failure.
		n.mu.Unlock()
		http.Error(w, "CLAUDUCT_CONFIRMATION_STALE", http.StatusGone)
		return
	}
	if p.connected || r.Context().Err() != nil || reply.Session != p.want.Session || reply.Search != p.want.Search || reply.Auxiliary != p.want.Auxiliary ||
		(reply.Unmatched && (reply.Confirmations || reply.Agent != "" || reply.Turn != "" || reply.Index != -1)) ||
		(!reply.Unmatched && (!reply.Confirmations || reply.Agent != p.want.Agent || reply.Turn != p.want.Turn || reply.Index != p.want.Index)) {
		n.mu.Unlock()
		g.refuse(w, refuseHeader)
		return
	}
	p.connected = true
	reply.lease, reply.close, reply.helper = p.lease, p.close, r.Context()
	n.mu.Unlock()
	defer p.close()
	stop := context.AfterFunc(r.Context(), p.close)
	defer stop()
	w.Header().Set("Content-Type", "text/plain")
	_ = control.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err = io.WriteString(w, "CLAUDUCT_CONFIRMATION_CONNECTED\n"); err != nil || control.Flush() != nil {
		return
	}
	_ = control.SetWriteDeadline(time.Time{})
	select {
	case p.reply <- &reply:
	case <-p.lease.Done():
		return
	case <-r.Context().Done():
		return
	case <-g.closing.Done():
		return
	}
	select {
	case <-p.lease.Done():
	case <-r.Context().Done():
	case <-g.closing.Done():
	}
}
