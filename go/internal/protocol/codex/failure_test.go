package codex

import (
	"errors"
	"testing"
)

// Only a listed value passes as itself. Anything else the backend puts in these fields is
// "other", an absent or null one is empty, and a body that does not parse describes
// nothing (#84).
func TestAFailureIsDescribedOnlyInItsVocabulary(t *testing.T) {
	for name, tc := range map[string]struct {
		eventType, raw string
		want           FailureDetail
	}{
		"failed, listed":        {Failed, `{"response":{"error":{"code":"insufficient_quota","type":"invalid_request_error"}}}`, FailureDetail{Code: "insufficient_quota", Type: "invalid_request_error"}},
		"failed, codex-rs code": {Failed, `{"response":{"error":{"code":"server_is_overloaded"}}}`, FailureDetail{Code: "server_is_overloaded"}},
		"failed, unlisted":      {Failed, `{"response":{"error":{"code":"PUBLIC-NEW","type":"PUBLIC-NEW"}}}`, FailureDetail{Code: "other", Type: "other"}},
		"failed, not a string":  {Failed, `{"response":{"error":{"code":42}}}`, FailureDetail{Code: "other"}},
		"failed, null code":     {Failed, `{"response":{"error":{"code":null}}}`, FailureDetail{}},
		"failed, no response":   {Failed, `{"type":"response.failed"}`, FailureDetail{}},
		"failed, duplicate key": {Failed, `{"response":{"error":{"code":"server_error"}},"response":{}}`, FailureDetail{}},
		"incomplete":            {Incomplete, `{"response":{"incomplete_details":{"reason":"content_filter"}}}`, FailureDetail{IncompleteReason: "content_filter"}},
		"incomplete, unlisted":  {Incomplete, `{"response":{"incomplete_details":{"reason":"PUBLIC-NEW"}}}`, FailureDetail{IncompleteReason: "other"}},
		"error, top-level code": {ErrorEvent, `{"code":"rate_limit_exceeded","error":{"code":"server_error","type":"rate_limit_error"}}`, FailureDetail{Code: "rate_limit_exceeded", Type: "rate_limit_error"}},
		"error, nested code":    {ErrorEvent, `{"error":{"code":"server_error"}}`, FailureDetail{Code: "server_error"}},
		"error, malformed":      {ErrorEvent, `not json`, FailureDetail{}},
	} {
		t.Run(name, func(t *testing.T) {
			err := Failure(tc.eventType, []byte(tc.raw))
			var failure *FailureError
			if !errors.As(err, &failure) || failure.Detail != tc.want {
				t.Fatalf("detail = %+v, want %+v", failure, tc.want)
			}
		})
	}
	if got := (FailureDetail{Code: "server_error", Type: "api_error", IncompleteReason: "max_tokens"}).String(); got != "upstream_code=server_error upstream_type=api_error incomplete_reason=max_tokens" {
		t.Fatalf("String = %q", got)
	}
	if Failure(Completed, []byte(`{}`)) != nil {
		t.Fatal("a completion was called a failure")
	}
}
