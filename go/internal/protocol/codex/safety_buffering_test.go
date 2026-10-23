package codex

import (
	"errors"
	"testing"
)

// The backend's safety_buffering flag (measured 2026-10-08, gpt-6.1-sol): false is the ordinary
// event and is read; any other value, or a value of another type, is refused.
func TestSafetyBufferingAcceptedOnlyWhenFalse(t *testing.T) {
	delta := func(flag string) []byte {
		body := `{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"pl","logprobs":[]`
		if flag != "" {
			body += `,"safety_buffering":` + flag
		}
		return []byte(body + `}`)
	}
	for _, flag := range []string{"", "false"} {
		got, err := DecodeTextDelta(delta(flag))
		if err != nil || got.Delta != "pl" {
			t.Fatalf("safety_buffering %q: %+v %v", flag, got, err)
		}
	}
	for _, flag := range []string{"true", `"false"`, "null", "0"} {
		if _, err := DecodeTextDelta(delta(flag)); !errors.Is(err, ErrEventShape) {
			t.Errorf("safety_buffering %s accepted: %v", flag, err)
		}
	}
	reasoning := func(flag string) []byte {
		return []byte(`{"type":"response.reasoning_text.delta","sequence_number":3,"item_id":"rs_1","output_index":0,"content_index":0,"delta":"x","safety_buffering":` + flag + `}`)
	}
	if _, err := DecodeReasoningEvent(ReasoningTextDelta, reasoning("false")); err != nil {
		t.Fatalf("reasoning delta with false: %v", err)
	}
	if _, err := DecodeReasoningEvent(ReasoningTextDelta, reasoning("true")); !errors.Is(err, ErrEventShape) {
		t.Fatalf("reasoning delta with true accepted: %v", err)
	}
}
