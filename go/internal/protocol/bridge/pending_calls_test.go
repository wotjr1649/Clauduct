package bridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

func TestPendingCallControlPreservesStopSequence(t *testing.T) {
	tr := NewTranslatorFor(callable("Agent", "Read"), "")
	tr.Builder().DeferTextUntilComplete()
	tr.Builder().SetStopSequences([]string{"STOP"})
	tr.PendingToolCall = func(name string, _ json.RawMessage) bool { return name == "Agent" }
	prepared := 0
	tr.PrepareToolCall = func(id, name string, raw json.RawMessage) (json.RawMessage, error) { prepared++; return raw, nil }
	events := []stream.Event{
		event(codex.Created, `{"response":{"id":"r"}}`),
		itemAdded(0, `{"id":"msg_1","type":"message"}`), textDelta("publicSTOPtail"), textDone("publicSTOPtail"),
		itemDone(0, messageItem("msg_1", "publicSTOPtail")),
		itemAdded(1, openingOf(functionCall("one", "Agent", `{}`))), itemDone(1, functionCall("one", "Agent", `{}`)),
		itemAdded(2, openingOf(functionCall("two", "Read", `{}`))), itemDone(2, functionCall("two", "Read", `{}`)),
		event(codex.Completed, completedOK),
	}
	var output strings.Builder
	for _, ev := range events {
		frames, err := tr.Accept(ev)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range frames {
			output.Write(f.Data)
		}
	}
	if prepared != 0 || tr.Builder().WaitingForChildren() || strings.Contains(output.String(), `"type":"tool_use"`) || !strings.Contains(output.String(), `"stop_reason":"stop_sequence"`) {
		t.Fatal("stop sequence was replaced by wait control")
	}
}

func TestPendingCallsRetainBarrierValidationAndIndependentWork(t *testing.T) {
	for _, variant := range []string{"duplicate", "parallel first", "mixed", "bad id", "duplicate id", "invalid args", "withdrawn tool", "limit"} {
		t.Run(variant, func(t *testing.T) {
			request := callable("Agent", "Read")
			if variant == "withdrawn tool" {
				request = callable("Read")
			}
			tr := NewTranslatorFor(request, "")
			tr.Builder().DeferTextUntilComplete()
			tr.PendingToolCall = func(name string, raw json.RawMessage) bool { return name == "Agent" && variant != "parallel first" }
			prepared := 0
			tr.PrepareToolCall = func(id, name string, raw json.RawMessage) (json.RawMessage, error) { prepared++; return raw, nil }
			calls := []string{functionCall("one", "Agent", `{"prompt":"PUBLIC"}`)}
			switch variant {
			case "parallel first":
				calls = append(calls, functionCall("two", "Agent", `{"prompt":"PUBLIC"}`))
			case "mixed":
				calls = append(calls, functionCall("two", "Read", `{}`))
			case "bad id":
				calls = []string{functionCall("bad id", "Agent", `{}`)}
			case "duplicate id":
				calls = append(calls, functionItem("other", "one", "Agent", `{}`))
			case "invalid args":
				calls = []string{functionCall("one", "Agent", `{"prompt":1,"prompt":2}`)}
			case "limit":
				tr.outputLimit = 1
			}
			_, err := tr.Accept(event(codex.Created, `{"response":{"id":"r"}}`))
			if err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			for _, ev := range completedWith(calls...) {
				frames, e := tr.Accept(ev)
				for _, f := range frames {
					output.Write(f.Data)
				}
				if e != nil {
					err = e
					break
				}
			}
			wantError := variant != "duplicate" && variant != "parallel first"
			if (err != nil) != wantError {
				t.Fatalf("err=%v", err)
			}
			if variant == "parallel first" {
				if prepared != 2 || strings.Count(output.String(), `"type":"tool_use"`) != 2 {
					t.Fatal("initial parallel calls lost")
				}
			} else if prepared != 0 || strings.Contains(output.String(), `"type":"tool_use"`) {
				t.Fatal("pending/invalid call was prepared or emitted")
			}
			if variant == "duplicate" && !tr.Builder().WaitingForChildren() {
				t.Fatal("not a native wait control")
			}
		})
	}
}
