package codex

import "testing"

func TestNestedOutputSemanticBoundary319(t *testing.T) {
	for _, item := range []string{
		`{"type":"message","content":[{"type":"future_text","text":"lost"}]}`,
		`{"type":"message","content":[{"text":"missing kind"}]}`,
		`{"type":"message","content":[{"type":"output_text","text":"x","annotations":[{"type":"future"}]}]}`,
		`{"type":"reasoning","summary":[{"type":"future_summary","text":"x"}]}`,
		`{"type":"reasoning","summary":[{"type":"summary_text"}]}`,
		`{"type":"reasoning","content":[{"type":"future_reasoning","text":"x"}]}`,
		`{"type":"reasoning","content":[null]}`,
	} {
		if _, err := decodeItem([]byte(item), true); err == nil {
			t.Errorf("unknown or missing semantic accepted: %s", item)
		}
	}
	for _, item := range []string{
		`{"type":"message","content":[{"type":"output_text","text":"","annotations":[],"logprobs":[]}]}`,
		`{"type":"reasoning","encrypted_content":"opaque","summary":[{"type":"summary_text","text":"summary"}],"content":[{"type":"reasoning_text","text":"opaque reasoning"}]}`,
	} {
		if _, err := decodeItem([]byte(item), true); err != nil {
			t.Errorf("normal rejected: %v", err)
		}
	}
}

func TestNegativeUsageIsUnknown319(t *testing.T) {
	u, err := DecodeUsage([]byte(`{"response":{"usage":{"input_tokens":-1,"output_tokens":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if u.InputKnown || !u.OutputKnown || u.OutputTokens != 0 {
		t.Fatal("negative became known or explicit zero was lost")
	}
}

func TestNullableLogprobs319(t *testing.T) {
	if _, err := decodeItem([]byte(`{"type":"message","content":[{"type":"output_text","text":"ok","annotations":[],"logprobs":null}]}`), true); err != nil {
		t.Fatal(err)
	}
}
