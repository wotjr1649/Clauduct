package bridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

func reasoningEvent319(kind, text string) stream.Event {
	fields := map[string]any{"type": kind, "item_id": "rs_public", "output_index": 0, "sequence_number": 1}
	switch kind {
	case codex.ReasoningTextDelta, codex.ReasoningTextDone:
		fields["content_index"] = 0
	default:
		fields["summary_index"] = 0
	}
	switch kind {
	case codex.ReasoningSummaryAdd, codex.ReasoningSummaryDone:
		fields["part"] = map[string]string{"type": "summary_text", "text": text}
	case codex.ReasoningSummaryTxtD, codex.ReasoningTextDelta:
		fields["delta"] = text
	default:
		fields["text"] = text
	}
	raw, _ := json.Marshal(fields)
	return event(kind, string(raw))
}

func TestReasoningPayloadFailClosed319(t *testing.T) {
	for _, kind := range []string{codex.ReasoningSummaryAdd, codex.ReasoningSummaryDone, codex.ReasoningSummaryTxtD, codex.ReasoningSummaryTxtF, codex.ReasoningTextDelta, codex.ReasoningTextDone, codex.ReasoningPartAdd, codex.ReasoningPartDone} {
		t.Run(kind, func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			if _, err := tr.Accept(event(kind, `{"type":"`+kind+`","future_content":{"answer":"public"}}`)); err == nil {
				t.Fatal("unvalidated content silently accepted")
			}
		})
	}
	for _, mutation := range []string{"orphan", "unknown-field", "nested-field", "incomplete", "wrong-item", "wrong-index", "null-index", "wrong-type", "duplicate-part", "after-close", "snapshot-mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			if mutation != "orphan" {
				if _, err := tr.Accept(itemAdded(0, `{"id":"rs_public","type":"reasoning"}`)); err != nil {
					t.Fatal(err)
				}
			}
			e := reasoningEvent319(codex.ReasoningSummaryAdd, "")
			var fields map[string]any
			json.Unmarshal(e.Raw, &fields)
			switch mutation {
			case "unknown-field":
				fields["future_answer"] = "public"
			case "nested-field":
				fields["part"].(map[string]any)["future_answer"] = "public"
			case "incomplete":
				e = reasoningEvent319(codex.ReasoningSummaryDone, "")
				json.Unmarshal(e.Raw, &fields)
				fields["status"] = "incomplete"
			case "wrong-item":
				fields["item_id"] = "other"
			case "wrong-index":
				fields["output_index"] = 1
			case "null-index":
				fields["summary_index"] = nil
			case "wrong-type":
				fields["type"] = codex.ReasoningTextDone
			case "duplicate-part":
				if _, err := tr.Accept(e); err != nil {
					t.Fatal(err)
				}
			case "after-close":
				if _, err := tr.Accept(itemDone(0, `{"id":"rs_public","type":"reasoning","summary":[]}`)); err != nil {
					t.Fatal(err)
				}
			case "snapshot-mismatch":
				for _, part := range []stream.Event{e, reasoningEvent319(codex.ReasoningSummaryTxtD, "public"), reasoningEvent319(codex.ReasoningSummaryTxtF, "public"), reasoningEvent319(codex.ReasoningSummaryDone, "public")} {
					if _, err := tr.Accept(part); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := tr.Accept(itemDone(0, `{"id":"rs_public","type":"reasoning","summary":[{"type":"summary_text","text":"different"}]}`)); err == nil {
					t.Fatal("reasoning disappeared from final snapshot")
				}
				return
			}
			e.Raw, _ = json.Marshal(fields)
			if mutation == "incomplete" {
				if _, err := tr.Accept(e); err != nil {
					t.Fatal("terminal accounting ended early", err)
				}
				if _, err := tr.Accept(event(codex.Completed, completedOK)); err != codex.ErrResponseIncomplt {
					t.Fatal("incomplete reasoning succeeded", err)
				}
				if !tr.usage.OutputKnown {
					t.Fatal("terminal usage lost")
				}
				return
			}
			if _, err := tr.Accept(e); err == nil {
				t.Fatal("invalid reasoning accepted")
			}
		})
	}
}

func TestReasoningStreamPreservesFinalSnapshot319(t *testing.T) {
	tr := NewTranslatorFor(callable("Read"), "")
	events := []stream.Event{
		itemAdded(0, `{"id":"rs_public","type":"reasoning"}`),
		reasoningEvent319(codex.ReasoningSummaryAdd, ""),
		reasoningEvent319(codex.ReasoningSummaryTxtD, "PUBLIC_SUMMARY"),
		reasoningEvent319(codex.ReasoningSummaryTxtF, "PUBLIC_SUMMARY"),
		reasoningEvent319(codex.ReasoningSummaryDone, "PUBLIC_SUMMARY"),
		reasoningEvent319(codex.ReasoningTextDelta, "PUBLIC_REASONING"),
		reasoningEvent319(codex.ReasoningTextDone, "PUBLIC_REASONING"),
		itemDone(0, `{"id":"rs_public","type":"reasoning","summary":[{"type":"summary_text","text":"PUBLIC_SUMMARY"}],"content":[{"type":"reasoning_text","text":"PUBLIC_REASONING"}],"encrypted_content":"public-opaque"}`),
		itemAdded(1, `{"id":"msg_1","type":"message"}`), textDelta("answer"), textDone("answer"), itemDone(1, messageItem("msg_1", "answer")), event(codex.Completed, completedOK),
	}
	for _, e := range events {
		frames, err := tr.Accept(e)
		if err != nil {
			t.Fatalf("%s: %v", e.Type, err)
		}
		if strings.Contains(joined(frames), "PUBLIC_REASONING") || strings.Contains(joined(frames), "PUBLIC_SUMMARY") {
			t.Fatal("reasoning leaked into native content")
		}
	}
	if len(tr.Summaries()) != 1 || tr.Summaries()[0] != "PUBLIC_SUMMARY" {
		t.Fatal("summary not retained")
	}
}

func summaryStream319(text string) []stream.Event {
	final, _ := json.Marshal(map[string]any{"id": "rs_public", "type": "reasoning", "encrypted_content": "public-opaque", "summary": []map[string]string{{"type": "summary_text", "text": text}}})
	return []stream.Event{itemAdded(0, `{"id":"rs_public","type":"reasoning"}`), reasoningEvent319(codex.ReasoningSummaryAdd, ""), reasoningEvent319(codex.ReasoningSummaryTxtD, text), reasoningEvent319(codex.ReasoningSummaryTxtF, text), reasoningEvent319(codex.ReasoningSummaryDone, text), itemDone(0, string(final))}
}

func TestPlainReasoningLifecycleFailClosed319(t *testing.T) {
	for _, kind := range []string{"done-mismatch", "delta-after-done", "snapshot-mismatch", "missing-done", "after-item-close"} {
		t.Run(kind, func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			if _, err := tr.Accept(itemAdded(0, `{"id":"rs_public","type":"reasoning"}`)); err != nil {
				t.Fatal(err)
			}
			if _, err := tr.Accept(reasoningEvent319(codex.ReasoningTextDelta, "public")); err != nil {
				t.Fatal(err)
			}
			candidate := reasoningEvent319(codex.ReasoningTextDone, "different")
			if kind != "done-mismatch" && kind != "missing-done" {
				if _, err := tr.Accept(reasoningEvent319(codex.ReasoningTextDone, "public")); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "delta-after-done":
				candidate = reasoningEvent319(codex.ReasoningTextDelta, "extra")
			case "snapshot-mismatch":
				candidate = itemDone(0, `{"id":"rs_public","type":"reasoning","content":[{"type":"reasoning_text","text":"different"}]}`)
			case "missing-done":
				candidate = itemDone(0, `{"id":"rs_public","type":"reasoning","content":[{"type":"reasoning_text","text":"public"}]}`)
			case "after-item-close":
				if _, err := tr.Accept(itemDone(0, `{"id":"rs_public","type":"reasoning","content":[{"type":"reasoning_text","text":"public"}]}`)); err != nil {
					t.Fatal(err)
				}
				candidate = reasoningEvent319(codex.ReasoningTextDelta, "extra")
			}
			if _, err := tr.Accept(candidate); err == nil {
				t.Fatal("invalid plain reasoning lifecycle accepted")
			}
		})
	}
}

func TestReasoningTransportPadding319(t *testing.T) {
	for _, kind := range []string{codex.ReasoningSummaryTxtD, codex.ReasoningTextDelta} {
		for _, value := range []any{"public-padding", nil, 17, map[string]any{"content": "public"}} {
			e := reasoningEvent319(kind, "public")
			var fields map[string]any
			json.Unmarshal(e.Raw, &fields)
			fields["obfuscation"] = value
			e.Raw, _ = json.Marshal(fields)
			decoded, err := codex.DecodeReasoningEvent(kind, e.Raw)
			if _, valid := value.(string); valid {
				if err != nil || decoded.Text != "public" {
					t.Fatal("documented padding rejected", err)
				}
			} else if err == nil {
				t.Fatal("non-string transport padding admitted")
			}
		}
	}
}

func TestOpeningReasoningSnapshotPreserved319(t *testing.T) {
	for _, opening := range []string{
		`"summary":[{"type":"summary_text","text":"public"}]`,
		`"content":[{"type":"reasoning_text","text":"public"}]`,
		`"encrypted_content":"public-opaque"`,
	} {
		for _, preserved := range []bool{false, true} {
			tr := NewTranslatorFor(callable("Read"), "")
			if _, err := tr.Accept(itemAdded(0, `{"id":"rs_public","type":"reasoning",`+opening+`}`)); err != nil {
				t.Fatal(err)
			}
			final := `{"id":"rs_public","type":"reasoning"}`
			if preserved {
				final = `{"id":"rs_public","type":"reasoning",` + opening + `}`
			}
			_, err := tr.Accept(itemDone(0, final))
			if preserved && err != nil {
				t.Fatal("preserved opening rejected", err)
			}
			if !preserved && err == nil {
				t.Fatal("opening content silently discarded", opening)
			}
		}
	}
}

func TestEncryptedReasoningSnapshotCanRefresh319(t *testing.T) {
	tr := NewTranslatorFor(callable("Read"), "")
	if _, err := tr.Accept(itemAdded(0, `{"id":"rs_public","type":"reasoning","encrypted_content":"opening-opaque"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(itemDone(0, `{"id":"rs_public","type":"reasoning","encrypted_content":"final-opaque"}`)); err != nil {
		t.Fatal(err)
	}
	if tr.held[0].item.Encrypted != "final-opaque" {
		t.Fatal("final continuity snapshot lost")
	}
}
