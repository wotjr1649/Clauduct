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
	want            nativeConfirmation
	reply           chan *nativeConfirmation
	rejected        chan struct{}
	lease           context.Context
	close           context.CancelFunc
	prepared        chan struct{}
	prepareDeadline time.Time
	confirmDeadline time.Time // nativeEvents.mu; A until prepared, then H
	connected       bool      // nativeEvents.mu; a nonce is consumed once
	// native is the process that sent the model request; only its own helper may answer.
	native processKey
	// unidentified: a helper the OS could not name answered this nonce (nativeEvents.mu).
	unidentified bool
	// answered: the requesting native's own helper reached this nonce (nativeEvents.mu). A
	// timeout without it is a slow machine, not a broken native.
	answered bool
}

func (p *pendingConfirmation) isRejected() bool {
	select {
	case <-p.rejected:
		return true
	default:
		return false
	}
}

// reject is called with nativeEvents.mu held. The channel records one terminal
// protocol failure and wakes the waiter without sharing the prepared transition.
func (p *pendingConfirmation) reject() {
	if !p.isRejected() {
		close(p.rejected)
	}
}

// answeredBy marks nonce answered when r comes from the native that requested it, so the
// request's timeout is that native's failure rather than a slow machine.
func (g *Gateway) answeredBy(r *http.Request, nonce string) {
	n := &g.nativeEvents
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.confirmations[nonce]
	if p == nil || p.lease.Err() != nil || g.closing.Err() != nil || !p.connected && !time.Now().Before(p.prepareDeadline) || p.connected && !time.Now().Before(p.confirmDeadline) {
		return
	}
	id, required := g.identityOf(r)
	if required && (id.native == (processKey{}) || id.native != p.native) {
		return
	}
	p.answered = true
	if p.connected || !required || p.native == (processKey{}) || id.native == (processKey{}) {
		return
	}
	p.reject()
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
	_, fieldsErr := wire.Fields(raw, []string{"nonce", "session", "agent", "turn", "index", "search", "auxiliary", "confirmations", "unmatched"})
	if parseErr := json.Unmarshal(raw, &reply); fieldsErr != nil || parseErr != nil {
		// Malformed, but naming a live nonce from the requesting native: it still answered.
		var named struct {
			Nonce string `json:"nonce"`
		}
		if json.Unmarshal(raw, &named) == nil {
			g.answeredBy(r, named.Nonce)
		}
		g.refuse(w, refuseHeader)
		return
	}
	n := &g.nativeEvents
	n.mu.Lock()
	p := n.confirmations[reply.Nonce]
	if p == nil || p.lease.Err() != nil || g.closing.Err() != nil ||
		!p.connected && !time.Now().Before(p.prepareDeadline) || p.connected && !time.Now().Before(p.confirmDeadline) {
		n.staleAnswers.Add(1)
		// The request this nonce served has already ended or been refused. The helper
		// is late, not wrong: say so, so the module does not latch policy failure.
		n.mu.Unlock()
		http.Error(w, "CLAUDUCT_CONFIRMATION_STALE", http.StatusGone)
		return
	}
	// Every native's module reads the one challenge file and answers what it sees, its own
	// sessions or not (#319 decision 18e, measured x4-dispatch): another native's answer is
	// not this request's proof. It is told the request is gone, which does not latch it.
	// Unnamed on either side is never a match, even of two unnamed keys.
	id, required := g.identityOf(r)
	if required && (id.native == (processKey{}) || p.native == (processKey{})) {
		p.unidentified = true
		n.mu.Unlock()
		n.unidentifiedAnswers.Add(1)
		http.Error(w, "CLAUDUCT_CONFIRMATION_STALE", http.StatusGone)
		return
	} else if required && id.native != p.native {
		n.mu.Unlock()
		n.foreignAnswers.Add(1)
		http.Error(w, "CLAUDUCT_CONFIRMATION_STALE", http.StatusGone)
		return
	}
	p.answered = true
	// The requesting native's own helper claiming another scope is a protocol violation: it is
	// recorded against the session, which no native process change clears (#319 decision 18f).
	violation := reply.Session != p.want.Session || reply.Search != p.want.Search || reply.Auxiliary != p.want.Auxiliary ||
		(reply.Unmatched && (reply.Confirmations || reply.Agent != "" || reply.Turn != "" || reply.Index != -1)) ||
		(!reply.Unmatched && (!reply.Confirmations || reply.Agent != p.want.Agent || reply.Turn != p.want.Turn || reply.Index != p.want.Index))
	if p.isRejected() {
		if violation && !p.connected && required && id.native != (processKey{}) && id.native == p.native {
			n.taintSession(p.want.Session)
			p.reject()
		}
		n.mu.Unlock()
		g.refuse(w, refuseHeader)
		return
	}
	if violation && !p.connected && required && id.native != (processKey{}) && id.native == p.native {
		n.taintSession(p.want.Session)
		p.reject()
		n.mu.Unlock()
		g.refuse(w, refuseHeader)
		return
	}
	if p.connected || r.Context().Err() != nil || violation {
		if _, required := g.identityOf(r); violation && required {
			n.taintSession(p.want.Session)
		}
		n.mu.Unlock()
		g.refuse(w, refuseHeader)
		return
	}
	// Reserve preparation only after the full identity and scope validation.
	if hooks := n.confirmationTestHooks; hooks != nil && hooks.beforeReservation != nil {
		hooks.beforeReservation()
	}
	requestErr, leaseErr, closingErr := r.Context().Err(), p.lease.Err(), g.closing.Err()
	now := time.Now()
	if requestErr != nil || leaseErr != nil || closingErr != nil || !now.Before(p.prepareDeadline) {
		n.staleAnswers.Add(1)
		n.mu.Unlock()
		http.Error(w, "CLAUDUCT_CONFIRMATION_STALE", http.StatusGone)
		return
	}
	deadline := now.Add(2 * time.Second)
	if p.confirmDeadline.Before(deadline) {
		deadline = p.confirmDeadline
	}
	p.confirmDeadline = deadline
	p.connected = true
	close(p.prepared)
	reply.lease, reply.close, reply.helper = p.lease, p.close, r.Context()
	n.mu.Unlock()
	defer p.close()
	stop := context.AfterFunc(r.Context(), p.close)
	defer stop()
	w.Header().Set("Content-Type", "text/plain")
	writeDeadline := time.Now().Add(3 * time.Second)
	if deadline.Before(writeDeadline) {
		writeDeadline = deadline
	}
	_ = control.SetWriteDeadline(writeDeadline)
	if _, err = io.WriteString(w, "CLAUDUCT_CONFIRMATION_CONNECTED\n"); err != nil || control.Flush() != nil {
		return
	}
	_ = control.SetWriteDeadline(time.Time{})
	if p.lease.Err() != nil || r.Context().Err() != nil || g.closing.Err() != nil || !time.Now().Before(deadline) {
		return
	}
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
