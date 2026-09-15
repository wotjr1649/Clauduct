package gateway

import (
	"encoding/json"
	"net/http"
)

// refusal is a fixed error category. The client sees the Anthropic error envelope it
// already parses, and the category is a constant this package chose — never a string taken
// from the request, and never one taken from an upstream response. Echoing either would put
// bytes someone else controls into a payload that tools downstream read.
type refusal struct {
	category string
	status   int
}

// The categories WP02 can produce. Each one names a specific refusal so a failure is
// diagnosable without turning on logging that would have to record request content.
var (
	refuseBoundary   = refusal{"LOCAL_BOUNDARY_REJECTED", http.StatusForbidden}
	refuseHeader     = refusal{"INVALID_HEADER", http.StatusBadRequest}
	refuseCredential = refusal{"UNEXPECTED_CREDENTIAL_SOURCE", http.StatusForbidden}
	refuseSession    = refusal{"LOCAL_SESSION_REQUIRED", http.StatusUnauthorized}
	refuseRoute      = refusal{"UNSUPPORTED_ROUTE", http.StatusNotFound}
	refuseMethod     = refusal{"UNSUPPORTED_METHOD", http.StatusMethodNotAllowed}
	refuseMediaType  = refusal{"UNSUPPORTED_MEDIA_TYPE", http.StatusUnsupportedMediaType}
	refuseTooLarge   = refusal{"INPUT_TOO_LARGE", http.StatusRequestEntityTooLarge}
	refuseBusy       = refusal{"TOO_MANY_REQUESTS", http.StatusTooManyRequests}
	refuseClosed     = refusal{"GATEWAY_CLOSED", http.StatusServiceUnavailable}
	refuseCancelled  = refusal{"CANCELLED", 499} // client went away; nothing will read this
	// The route exists and is intended, but its implementation lands in a later package.
	// Distinct from UNSUPPORTED_ROUTE on purpose: "not built yet" and "never going to be
	// answered here" are different facts and a reader should not have to guess which.
	refuseUnimplemented = refusal{"NOT_IMPLEMENTED", http.StatusNotImplemented}
)

// errorType maps a status onto the Anthropic error type the client expects, matching the
// Node baseline's mapping so a client cannot tell the two implementations apart by shape.
func errorType(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}

// refuseCategory refuses with a category decided by the caller. The category is always a
// constant from one of the protocol packages, never a value read out of a request or an
// upstream body.
func (g *Gateway) refuseCategory(w http.ResponseWriter, status int, category string) {
	g.refuse(w, refusal{category: category, status: status})
}

func (g *Gateway) refuse(w http.ResponseWriter, r refusal) {
	g.refused.Add(1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	body, err := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errorType(r.status),
			"message": r.category,
		},
	})
	if err != nil {
		return
	}
	w.Write(body)
}
