package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
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
	refuseEncoding   = refusal{"UNSUPPORTED_ENCODING", http.StatusUnsupportedMediaType}
	refuseVersion    = refusal{"UNSUPPORTED_VERSION", http.StatusBadRequest}
	refuseSessionID  = refusal{"INVALID_SESSION_ID", http.StatusBadRequest}
)

// Refusals a subagent registration can produce. Errors rather than refusals because the
// registry raises them and the handler decides the status.
var (
	// errInvalidBinding means the hook reported something that is not a binding.
	errInvalidBinding = errors.New("INVALID_AGENT_BINDING")
	// errBindingConflict means the same identifier arrived with a different role. Two
	// different subagents wearing one name, and either answer about which one the next
	// request belongs to would be a guess.
	errBindingConflict = errors.New("AGENT_BINDING_CONFLICT")
	// errBindingLimit means the table is full of registrations that are all busy.
	errBindingLimit = errors.New("AGENT_BINDING_LIMIT")
	refuseTooLarge  = refusal{"INPUT_TOO_LARGE", http.StatusRequestEntityTooLarge}
	refuseBusy      = refusal{"TOO_MANY_REQUESTS", http.StatusTooManyRequests}
	refuseClosed    = refusal{"GATEWAY_CLOSED", http.StatusServiceUnavailable}
	refuseCancelled = refusal{"CANCELLED", 499} // client went away; nothing will read this
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

func (g *Gateway) refuse(w http.ResponseWriter, r refusal) { g.refuseDetail(w, r, "") }

// refuseDetail refuses with a detail after the category. The detail is empty or built from
// fixed vocabularies, like the category itself.
func (g *Gateway) refuseDetail(w http.ResponseWriter, r refusal, detail string) {
	// After dispatch the replay ledger refuses any repeat of this request, so a status that
	// invites a retry only trades this category for NATIVE_REQUEST_REPLAY_BLOCKED in front of
	// the user (#84). The client checks x-should-retry before the status class (claude
	// 2.1.281); without the ledger nothing is refused and the class decides as before.
	if recordOf(w).dispatched() {
		w.Header().Set("X-Should-Retry", "false")
	}
	g.countRefusal(r.category, recordOf(w).path())
	recordOf(w).refusedWith(r.status, r.category)
	control := http.NewResponseController(w)
	switch r.category {
	case refuseCancelled.category, refuseTooLarge.category, refuseBusy.category:
		// These close without reading more. A cancelled read must not be pooled with the
		// next turn, even when a filter still delivers this refusal; an oversized body is
		// already past its limit and the closing socket is drained by httpguard; a busy
		// gateway must not wait on a slow upload. The expired read deadline also stops
		// net/http's background reader.
		_ = control.SetReadDeadline(time.Now())
		w.Header().Set("Connection", "close")
	default:
		// net/http closes early refusals with more than 256KiB unread. Consume a
		// bounded body before writing the error so a normal native upload can keep
		// using its connection. Unfinished bodies still close, within a one-second
		// budget; they never reach decoding or backend execution.
		if control.SetReadDeadline(time.Now().Add(time.Second)) == nil {
			if tracked, ok := w.(*tracked); ok && tracked.body != nil {
				if _, err := io.CopyN(io.Discard, tracked.body, maxRequestBytes+1); err != io.EOF {
					w.Header().Set("Connection", "close")
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	body, err := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errorType(r.status),
			"message": refusalMessage(r.category) + detail,
		},
	})
	if err != nil {
		return
	}
	w.Write(body)
}

func refusalMessage(category string) string {
	switch category {
	case "NATIVE_REQUEST_REPLAY_BLOCKED":
		return category + "; an earlier attempt may already have executed. Automatic replay was blocked. Check the previous outcome before submitting a new prompt; the current native session can continue."
	case "NATIVE_TURN_ENDED":
		return category + "; native had already reported this agent turn complete, so nothing was executed. The current native session can continue."
	case "NATIVE_REQUEST_CAPACITY":
		return category + "; this session reached its execution tracking limit. Start a new session before sending more requests. No replacement was executed."
	case "CONTEXT_REQUEST_CLASS_UNVERIFIED":
		return category + "; this session requires X-Claude-Code-Request-Class. Update Claude Code or repair the local integration. Reference client: " + ReferenceClient + "."
	case "CONTEXT_SESSION_UNVERIFIED":
		return category + "; the Clauduct session hook has not registered a transcript. Check the hook error, restore the connection and submit the prompt again. Context recovery was not bypassed."
	}
	if category != "UNSUPPORTED_MODEL_OR_EFFORT" {
		return category
	}
	models := make([]string, 0, len(bridge.Models))
	for _, model := range bridge.Models {
		models = append(models, model.ID)
	}
	return category + "; supported models: " + strings.Join(models, ", ") +
		"; supported efforts for each: " + strings.Join(bridge.Efforts, ", ") + ". No replacement was executed."
}
