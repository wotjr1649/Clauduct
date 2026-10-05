package bridge

import (
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"testing"
)

func TestArgumentEventBinding319(t *testing.T) {
	for _, mode := range []string{"orphan", "wrong-kind", "wrong-index", "after-item-done", "after-args-done", "duplicate-done", "unknown-content"} {
		t.Run(mode, func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			item := `{"id":"fc_1","type":"function_call","call_id":"call_1","name":"Read","arguments":""}`
			if mode == "wrong-kind" {
				item = `{"id":"fc_1","type":"reasoning"}`
			}
			if mode != "orphan" {
				if _, err := tr.Accept(itemAdded(0, item)); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "after-item-done" {
				if _, err := tr.Accept(itemDone(0, `{"id":"fc_1","type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}`)); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "after-args-done" || mode == "duplicate-done" {
				if _, err := tr.Accept(argsDelta("fc_1", "{}")); err != nil {
					t.Fatal(err)
				}
				if _, err := tr.Accept(argsDone("fc_1", "{}")); err != nil {
					t.Fatal(err)
				}
			}
			e := argsDelta("fc_1", "{}")
			if mode == "duplicate-done" {
				e = argsDone("fc_1", "{}")
			}
			if mode == "wrong-index" || mode == "unknown-content" {
				var fields map[string]any
				json.Unmarshal(e.Raw, &fields)
				if mode == "wrong-index" {
					fields["output_index"] = 999
				} else {
					fields["future_answer"] = "public"
				}
				e.Raw, _ = json.Marshal(fields)
			}
			if _, err := tr.Accept(e); err == nil {
				t.Fatal("unbound or closed argument event accepted")
			}
		})
	}
}

func TestTextEventBinding319(t *testing.T) {
	for _, mode := range []string{"orphan", "wrong-kind", "wrong-index", "after-item-done", "unknown-content"} {
		t.Run(mode, func(t *testing.T) {
			tr := NewTranslatorFor(callable("Read"), "")
			item := `{"id":"msg_1","type":"message"}`
			if mode == "wrong-kind" {
				item = `{"id":"msg_1","type":"reasoning"}`
			}
			if mode != "orphan" {
				if _, err := tr.Accept(itemAdded(0, item)); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "after-item-done" {
				if _, err := tr.Accept(itemDone(0, messageItem("msg_1"))); err != nil {
					t.Fatal(err)
				}
			}
			e := textDelta("public")
			if mode == "wrong-index" || mode == "unknown-content" {
				var fields map[string]any
				json.Unmarshal(e.Raw, &fields)
				if mode == "wrong-index" {
					fields["output_index"] = 999
				} else {
					fields["future_answer"] = "public"
				}
				e.Raw, _ = json.Marshal(fields)
			}
			if _, err := tr.Accept(e); err == nil {
				t.Fatal("unbound or closed text event accepted")
			}
		})
	}
}

func TestOpeningContentSnapshot319(t *testing.T) {
	for _, kind := range []string{"text", "arguments"} {
		tr := NewTranslatorFor(callable("Read"), "")
		opening, final := messageItem(defaultItem, "A"), messageItem(defaultItem, "B")
		if kind == "arguments" {
			opening = functionItem("fc", "call", "Read", `{"a":`)
			final = functionItem("fc", "call", "Read", `{"b":1}`)
		}
		if _, err := tr.Accept(itemAdded(0, opening)); err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Accept(itemDone(0, final)); err == nil {
			t.Error("opening content discarded", kind)
		}
	}
}

func TestIncompleteItemPreservesTerminalUsage319(t *testing.T) {
	for _, terminal := range []string{codex.Incomplete, codex.Completed} {
		tr := NewTranslatorFor(callable("Read"), "")
		if _, err := tr.Accept(itemAdded(0, `{"id":"fc","type":"function_call"}`)); err != nil {
			t.Fatal(err)
		}
		if frames, err := tr.Accept(itemDone(0, `{"id":"fc","type":"function_call","call_id":"c","name":"Read","arguments":"{","status":"incomplete"}`)); err != nil || len(frames) != 0 {
			t.Fatal("incomplete item ended before usage", err)
		}
		if frames, err := tr.Accept(event(terminal, `{"type":"`+terminal+`","response":{`+usageReported+`,"output":[]}}`)); !errors.Is(err, codex.ErrResponseIncomplt) || len(frames) != 0 {
			t.Fatal("incomplete item published", err)
		}
		if !tr.usage.InputKnown || !tr.usage.OutputKnown || tr.usage.OutputTokens != 3 {
			t.Fatal("terminal usage lost")
		}
	}
}

func TestEmptyArgumentDoneDoesNotEraseSnapshot319(t *testing.T) {
	tr := NewTranslatorFor(callable("Read"), "")
	if _, err := tr.Accept(itemAdded(0, `{"id":"fc","type":"function_call"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(argsDone("fc", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(itemDone(0, functionItem("fc", "c", "Read", "{}"))); err == nil {
		t.Fatal("empty done was silently replaced")
	}
}
func TestFinalTextPartsKeepTheirIndices319(t *testing.T) {
	for _, index := range []int{0, 42} {
		tr := NewTranslatorFor(callable("Read"), "")
		if _, err := tr.Accept(itemAdded(0, messageItem(defaultItem))); err != nil {
			t.Fatal(err)
		}
		e := textDelta("A")
		var fields map[string]any
		json.Unmarshal(e.Raw, &fields)
		fields["content_index"] = index
		e.Raw, _ = json.Marshal(fields)
		if _, err := tr.Accept(e); err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Accept(textDoneFor(defaultItem, index, "A")); err != nil {
			t.Fatal(err)
		}
		_, err := tr.Accept(itemDone(0, messageItem(defaultItem, "A")))
		if index == 0 && err != nil {
			t.Fatal(err)
		}
		if index != 0 && err == nil {
			t.Fatal("part index silently remapped")
		}
	}
}

func TestCompletedCannotHideIncompleteStatus319(t *testing.T) {
	for _, where := range []string{"response", "output"} {
		tr := NewTranslatorFor(callable("Read"), "")
		call := functionItem("fc", "c", "Read", "{}")
		for _, e := range []stream.Event{itemAdded(0, openingOf(call)), itemDone(0, call)} {
			if _, err := tr.Accept(e); err != nil {
				t.Fatal(err)
			}
		}
		raw := `{"response":{"status":"incomplete",` + usageReported + `,"output":[]}}`
		if where == "output" {
			var item map[string]any
			json.Unmarshal([]byte(call), &item)
			item["status"] = "incomplete"
			encoded, _ := json.Marshal(item)
			raw = `{"response":{` + usageReported + `,"output":[` + string(encoded) + `]}}`
		}
		if frames, err := tr.Accept(event(codex.Completed, raw)); !errors.Is(err, codex.ErrResponseIncomplt) || len(frames) != 0 {
			t.Fatal("incomplete accepted", err)
		}
		if !tr.usage.OutputKnown {
			t.Fatal("usage lost")
		}
	}
}
