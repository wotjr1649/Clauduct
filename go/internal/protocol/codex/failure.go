package codex

import (
	"encoding/json"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// FailureDetail is what a failure event says about itself, reduced to fixed vocabularies
// (#84). A value in its list passes as itself, any other value is "other", and an absent
// one stays empty. The backend's message never passes: it is free text, and what reaches
// the client and the account is only ever a constant chosen here.
type FailureDetail struct {
	Code             string `json:"code,omitempty"`
	Type             string `json:"type,omitempty"`
	IncompleteReason string `json:"incompleteReason,omitempty"`
}

// The Node baseline's lists, with the codes codex-rs names in response.failed
// (codex-api/src/sse/responses.rs at 8c66a5a, read 2026-09-24).
var (
	failureCodes = set("server_error", "internal_error", "invalid_request_error", "invalid_prompt",
		"context_length_exceeded", "invalid_encrypted_content", "model_not_found", "unsupported_model",
		"rate_limit_exceeded", "slow_down", "usage_limit_reached", "usage_not_included",
		"insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "server_is_overloaded", "cyber_policy", "bio_policy",
		"misalignment_policy_violation", "invalid_api_key", "authentication_error", "permission_denied")
	failureTypes = set("server_error", "invalid_request_error", "rate_limit_error", "authentication_error",
		"permission_error", "not_found_error", "api_error", "overloaded_error")
	incompleteReasons = set("max_output_tokens", "max_tokens", "content_filter", "steered")
)

func set(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

// String is the detail as the client and the account read it, empty when there is none.
func (d FailureDetail) String() string {
	var parts []string
	for _, p := range [][2]string{{"upstream_code", d.Code}, {"upstream_type", d.Type}, {"incomplete_reason", d.IncompleteReason}} {
		if p[1] != "" {
			parts = append(parts, p[0]+"="+p[1])
		}
	}
	return strings.Join(parts, " ")
}

// FailureError is a terminal failure event and its detail. It unwraps to the event's
// category, so every existing errors.Is still names it.
type FailureError struct {
	category error
	Detail   FailureDetail
}

func (e *FailureError) Error() string { return e.category.Error() }
func (e *FailureError) Unwrap() error { return e.category }

// describe reads the detail from a failure event. response.failed and response.incomplete
// carry theirs under response; the error event carries its code at the top or under
// error, as the baseline read it. A body that does not parse describes nothing.
func describe(eventType string, raw []byte) FailureDetail {
	var d FailureDetail
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return d
	}
	container := fields
	if eventType != ErrorEvent {
		if container, err = wire.Fields(fields["response"], nil); err != nil {
			return d
		}
	}
	failure, _ := wire.Fields(container["error"], nil)
	code := failure["code"]
	if eventType == ErrorEvent {
		if top, ok := fields["code"]; ok {
			code = top
		}
	}
	d.Code, d.Type = label(code, failureCodes), label(failure["type"], failureTypes)
	if eventType == Incomplete {
		details, _ := wire.Fields(container["incomplete_details"], nil)
		d.IncompleteReason = label(details["reason"], incompleteReasons)
	}
	return d
}

func label(raw json.RawMessage, allowed map[string]bool) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !allowed[value] {
		return "other"
	}
	return value
}
