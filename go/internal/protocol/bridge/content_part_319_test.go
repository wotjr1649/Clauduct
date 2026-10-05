package bridge

import (
	"errors"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"testing"
)

func TestKnownContentPartPayload319(t *testing.T) {
	for _, raw := range []string{
		`{"type":"response.content_part.added"}`,
		`{"type":"response.content_part.added","item_id":"m","output_index":0,"content_index":0,"part":{"type":"future_text","text":"lost"}}`,
		`{"type":"response.content_part.added","item_id":"wrong","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
	} {
		tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
		if _, err := tr.Accept(itemAdded(0, `{"id":"m","type":"message"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Accept(event(codex.ContentPartAdd, raw)); err == nil {
			t.Errorf("invalid known event payload ignored: %s", raw)
		}
	}
}

func TestContentPartSnapshotMismatch319(t *testing.T) {
	tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
	if _, err := tr.Accept(itemAdded(0, `{"id":"m","type":"message"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(event(codex.TextDelta, `{"type":"response.output_text.delta","item_id":"m","content_index":0,"delta":"lost"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(event(codex.ContentPartDon, `{"type":"response.content_part.done","item_id":"m","output_index":0,"content_index":0,"part":{"type":"output_text","text":"lost"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(itemDone(0, messageItem("m", "different"))); err == nil {
		t.Fatal("contradictory content snapshot accepted")
	}
}

func TestContentPartReviewCounterexamples319(t *testing.T) {
	part := func(kind string, index int) stream.Event {
		return event(kind, fmt.Sprintf(`{"type":%q,"item_id":"m","output_index":0,"content_index":%d,"part":{"type":"output_text","text":""}}`, kind, index))
	}
	for name, events := range map[string][]stream.Event{
		"missing-part":       {itemAdded(0, `{"id":"m","type":"message"}`), part(codex.ContentPartAdd, 1), itemDone(0, messageItem("m", ""))},
		"duplicate-added":    {itemAdded(0, `{"id":"m","type":"message"}`), part(codex.ContentPartAdd, 0), part(codex.ContentPartAdd, 0)},
		"completed-mismatch": {itemAdded(0, `{"id":"m","type":"message"}`), event(codex.TextDelta, `{"type":"response.output_text.delta","item_id":"m","content_index":0,"delta":"kept"}`), event(codex.TextDone, `{"type":"response.output_text.done","item_id":"m","content_index":0,"text":"kept"}`), itemDone(0, messageItem("m", "kept")), event(codex.Completed, `{"type":"response.completed","response":{"id":"r","output":[{"id":"m","type":"message","content":[{"type":"output_text","text":"lost"}]}]}}`)},
	} {
		t.Run(name, func(t *testing.T) {
			tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
			var got error
			for i, e := range events {
				if _, got = tr.Accept(e); got != nil {
					if i != len(events)-1 {
						t.Fatalf("setup event %d failed: %v", i, got)
					}
					break
				}
			}
			if !errors.Is(got, ErrItemSnapshotMismatch) {
				t.Fatalf("want snapshot mismatch, got %v", got)
			}
		})
	}
}
