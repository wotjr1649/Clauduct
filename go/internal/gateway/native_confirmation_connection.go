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
	// native is the process that sent the model request; only its own helper may answer.
	native processKey
	// unidentified: a helper the OS could not name answered this nonce (nativeEvents.mu).
	unidentified bool
	// answered: the requesting native's own helper reached this nonce (nativeEvents.mu). A
	// timeout without it is a slow machine, not a broken native.
	answered bool
}

// answeredBy marks nonce answered when r comes from the native that requested it, so the
// request's timeout is that native's failure rather than a slow machine.
func (g *Gateway) answeredBy(r *http.Request, nonce string) {
	n := &g.nativeEvents
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.confirmations[nonce]
	if p == nil {
		return
	}
	if id, required := g.identityOf(r); required && (id.native == (processKey{}) || id.native != p.native) {
		return
	}
	p.answered = true
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
	if p == nil || p.lease.Err() != nil {
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
	if id, required := g.identityOf(r); required && (id.native == (processKey{}) || p.native == (processKey{})) {
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
	if p.connected || r.Context().Err() != nil || violation {
		if _, required := g.identityOf(r); violation && required {
			n.taintSession(p.want.Session)
		}
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
