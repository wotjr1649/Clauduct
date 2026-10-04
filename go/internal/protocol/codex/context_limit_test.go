package codex

import "testing"

func TestContextLimitRequiresUnambiguousStructuredCode(t *testing.T) {
	for _, raw := range []string{`{"error":{"code":"context_length_exceeded"}}`, `{"response":{"error":{"code":"context_length_exceeded"}}}`} {
		if !ContextLimit([]byte(raw)) {
			t.Fatal("structured overflow lost")
		}
	}
	for _, raw := range []string{`{"error":{"message":"context_length_exceeded"}}`, `{"error":{"code":"rate_limit_exceeded"}}`, `{"error":{"code":"context_length_exceeded","code":"other"}}`, `{"error":null}`, `{"error":{"code":"context_length_exceeded"},"error":{}}`} {
		if ContextLimit([]byte(raw)) {
			t.Fatal("ambiguous failure authorized recovery")
		}
	}
}
