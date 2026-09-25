package gateway

import (
	"context"
	"errors"
	"net/http"
)

// All admitted HTTP paths enter here after authentication and header validation.
// Content-Length is bounded before arithmetic; unknown-length bodies reserve the
// endpoint's entire allowed body. net/http owns Content-Length framing, and the
// reader still enforces its byte limit for chunked bodies and incomplete uploads.
func (g *Gateway) admitRequest(w http.ResponseWriter, r *http.Request, limit int64, class int) (context.Context, func(), bool) {
	if r.ContentLength > limit {
		g.refuse(w, refuseTooLarge)
		return nil, nil, false
	}
	length := r.ContentLength
	if length < 0 {
		length = limit
	}
	reserved := uint64(modelReserveBase) + uint64(length)*modelReserveFactor
	if class == controlAdmission {
		reserved = controlRequestReserve
	}
	wait := r.Context()
	if r.URL.Path == "/v1/messages" {
		var finish func()
		wait, finish = g.bindNativeCancellation(wait, r, recordOf(w), true)
		defer finish()
	}
	_, ctx, release, err := g.requests.admit(r.Context(), wait, reserved, class)
	if err == nil {
		return ctx, release, true
	}
	switch {
	case errors.Is(err, errGatewayClosed):
		g.refuse(w, refuseClosed)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		g.refuse(w, refuseCancelled)
	default:
		// Registry errors are fixed categories, never OS or request strings.
		g.refuseCategory(w, http.StatusTooManyRequests, err.Error())
	}
	return nil, nil, false
}
