package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// stopRun feeds deltas item by item into a builder with stop sequences, then a tool call and
// the completion, and returns the frames and the non-streaming message built from them.
func stopRun(t *testing.T, stops []string, deferred bool, items ...[]string) ([]Frame, map[string]any, *Builder) {
	t.Helper()
	b := NewBuilder("public-model")
	b.SetCallable(func(name string) bool { return name == "Read" })
	b.SetStopSequences(stops)
	if deferred {
		b.DeferTextUntilComplete()
	}
	var frames []Frame
	for i, deltas := range items {
		item := "item" + string(rune('0'+i))
		for _, delta := range deltas {
			produced, err := b.AppendText(item, 0, delta)
			if err != nil {
				t.Fatal(err)
			}
			frames = append(frames, produced...)
		}
		produced, err := b.FinishText(item, 0, strings.Join(deltas, ""))
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, produced...)
	}
	if err := b.AddToolCall("call_after", "Read", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	produced, err := b.Complete(Usage{OutputTokens: 5, OutputKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	frames = append(frames, produced...)
	var m ResponseMessage
	if err := m.Add(frames); err != nil {
		t.Fatal(err)
	}
	raw, err := m.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	return frames, message, b
}

// streamedText is what a streaming client assembles from the text deltas.
func streamedText(t *testing.T, frames []Frame) string {
	t.Helper()
	var out strings.Builder
	for _, f := range frames {
		var e struct{ Delta struct{ Type, Text string } }
		if f.Type == "content_block_delta" && json.Unmarshal(f.Data, &e) == nil && e.Delta.Type == "text_delta" {
			if !utf8.ValidString(e.Delta.Text) {
				t.Fatalf("delta splits a character: %q", e.Delta.Text)
			}
			out.WriteString(e.Delta.Text)
		}
	}
	return out.String()
}

// Native's auto mode classifier stops at </severity> (#149). The backend has no such
// parameter, so what it writes past the sequence -- in the same item, a later item, or as a
// tool call -- must not reach the client, streaming or not, and the reason must say so.
func TestStopSequenceCutsTheAnswer(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		frames, message, b := stopRun(t, []string{"</severity>"}, deferred,
			[]string{"<severity>1", "2</sev", "erity>\n<category>none</category>"},
			[]string{"a later item"})
		if !deferred {
			if got := streamedText(t, frames); got != "<severity>12" {
				t.Fatalf("streamed %q", got)
			}
		}
		content, _ := message["content"].([]any)
		if len(content) != 1 || content[0].(map[string]any)["text"] != "<severity>12" {
			t.Fatalf("deferred=%v content = %v", deferred, content)
		}
		if message["stop_reason"] != "stop_sequence" || message["stop_sequence"] != "</severity>" {
			t.Fatalf("deferred=%v stop = %v %v", deferred, message["stop_reason"], message["stop_sequence"])
		}
		if b.Answer() != "<severity>12" || !strings.Contains(b.Text(), "a later item") {
			t.Fatalf("answer %q, backend text %q", b.Answer(), b.Text())
		}
	}
}

// The first sequence to be completed wins, as it would during generation.
func TestStopSequenceEarliestEndWins(t *testing.T) {
	_, message, _ := stopRun(t, []string{"abc", "b"}, false, []string{"xabc"})
	content := message["content"].([]any)
	if content[0].(map[string]any)["text"] != "xa" || message["stop_sequence"] != "b" {
		t.Fatalf("message = %v", message)
	}
}

// Held-back text is released whole characters at a time, and all of it once the part ends
// without a stop.
func TestStopSequenceHoldKeepsCharactersWhole(t *testing.T) {
	frames, message, _ := stopRun(t, []string{"끝"}, false, []string{"가나", "다라", "마"})
	if got := streamedText(t, frames); got != "가나다라마" {
		t.Fatalf("streamed %q", got)
	}
	if message["stop_reason"] != "tool_use" {
		t.Fatalf("message = %v", message)
	}
}

func TestStopSequencesAreDecoded(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":false,"messages":[{"role":"user","content":"x"}],"stop_sequences":`
	request, refusal := decode(t, head+`["</severity>"]}`)
	if refusal != nil || len(request.StopSequences) != 1 || request.StopSequences[0] != "</severity>" {
		t.Fatalf("request = %v, refusal = %v", request, refusal)
	}
	for _, value := range []string{`[""]`, `[null]`, `[1]`, `"x"`, `{}`,
		`[` + strings.Repeat(`"s",`, 16) + `"s"]`, `["` + strings.Repeat("s", 257) + `"]`} {
		mustRefuse(t, head+value+`}`, CodeUnsupportedSample)
	}
}

// Parts the backend never finished: one still open when another reaches the stop shows
// nothing written after it, and one that ends with the response releases what it held.
func TestStopSequenceOpenParts(t *testing.T) {
	for _, tc := range []struct {
		deltas [][2]string
		want   string
	}{
		{[][2]string{{"a", "x"}, {"b", "y</s>z"}, {"a", "more"}}, "y"},
		{[][2]string{{"a", "hello"}}, "hello"},
	} {
		b := NewBuilder("public-model")
		b.SetStopSequences([]string{"</s>"})
		var frames []Frame
		for _, d := range tc.deltas {
			produced, err := b.AppendText(d[0], 0, d[1])
			if err != nil {
				t.Fatal(err)
			}
			frames = append(frames, produced...)
		}
		produced, err := b.Complete(Usage{})
		if err != nil {
			t.Fatal(err)
		}
		if got := streamedText(t, append(frames, produced...)); got != tc.want || b.Answer() != tc.want {
			t.Fatalf("streamed %q, answer %q, want %q", got, b.Answer(), tc.want)
		}
	}
}
