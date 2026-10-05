package codex

import "testing"

func TestOutputItemSemanticBoundary319(t *testing.T) {
	for _, extra := range []string{`,"future_content":"public"`, `,"content":[{"type":"refusal","refusal":"public"}]`, `,"status":null`, `,"status":"future"`} {
		raw := `{"output_index":0,"item":{"id":"f","type":"function_call","name":"Read","call_id":"c","arguments":"{}"` + extra + `}}`
		if _, err := DecodeOutputItem([]byte(raw), true); err == nil {
			t.Error("unknown semantic accepted", extra)
		}
	}
	for _, role := range []string{`"user"`, `null`, `"future"`} {
		raw := `{"output_index":0,"item":{"id":"m","type":"message","role":` + role + `,"content":[{"type":"output_text","text":"public"}]}}`
		if _, err := DecodeOutputItem([]byte(raw), true); err == nil {
			t.Error("non-assistant output accepted", role)
		}
	}
}

func TestContentEnvelopeSemanticBoundary319(t *testing.T) {
	if _, err := DecodeOutputItem([]byte(`{"output_index":0,"future_content":"public","item":{"id":"m","type":"message","content":[]}}`), true); err == nil {
		t.Error("output envelope silently dropped semantic")
	}
	if _, err := DecodeContentPart([]byte(`{"item_id":"m","output_index":0,"content_index":0,"future_content":"public","part":{"type":"output_text","text":"x"}}`)); err == nil {
		t.Error("content envelope silently dropped semantic")
	}
}
