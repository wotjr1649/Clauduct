package anthropic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNonStreamingMessagePreservesBlocksAndExactToolNumbers(t *testing.T) {
	b := NewBuilder("public-model")
	b.SetCallable(func(name string) bool { return name == "Read" })
	var m ResponseMessage
	for _, text := range []string{"가", "나다"} {
		frames, err := b.AppendText("part", 0, text)
		if err != nil {
			t.Fatal(err)
		}
		if err = m.Add(frames); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.JSON(); err == nil {
		t.Fatal("partial response exposed")
	}
	b.AddThought("opaque-record")
	if err := b.AddToolCall("call_public", "Read", []byte(`{"value":9007199254740993}`)); err != nil {
		t.Fatal(err)
	}
	frames, err := b.Complete(Usage{InputTokens: 42, OutputTokens: 7, InputKnown: true, OutputKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Add(frames); err != nil {
		t.Fatal(err)
	}
	raw, err := m.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Content    []json.RawMessage
		StopReason string `json:"stop_reason"`
		Usage      map[string]int64
	}
	if json.Unmarshal(raw, &got) != nil || len(got.Content) != 3 || got.StopReason != "tool_use" || got.Usage["input_tokens"] != 42 || got.Usage["output_tokens"] != 7 {
		t.Fatal("message structure lost")
	}
	for _, want := range []string{"가나다", "opaque-record", "9007199254740993"} {
		if !strings.Contains(string(raw), want) {
			t.Fatal("lost block content", want)
		}
	}
	if err = m.Add(frames); !errors.Is(err, ErrStreamOrder) {
		t.Fatal("duplicate completion accepted")
	}
}

func TestNonStreamingMessageBoundsAndOrder(t *testing.T) {
	var m ResponseMessage
	if err := m.Add([]Frame{contentBlockDelta(0, "x")}); !errors.Is(err, ErrStreamOrder) {
		t.Fatal(err)
	}
	b := NewBuilder("public-model")
	frames, err := b.AppendText("part", 0, "x")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Add(frames); err != nil {
		t.Fatal(err)
	}
	m.bytes = maxResponseBytes
	if err = m.Add([]Frame{contentBlockDelta(0, "x")}); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal(err)
	}
}
