package codex

import (
	"errors"
	"testing"
)

const bufferingNotice = `{"use_cases":["cyber"],"reasons":["user_risk"]}`

func withMember(body, member string) []byte {
	return []byte(body[:len(body)-1] + `,"safety_buffering":` + member + `}`)
}

// Every strict decoder takes false and the codex-shaped object, and refuses everything else.
func TestSafetyBufferingValueClassesPerEventKind(t *testing.T) {
	valid := []string{"false", bufferingNotice, `{"use_cases":[],"reasons":[],"retry_model":"gpt-fast"}`, `{"use_cases":["a"],"reasons":["b"],"retry_model":null}`}
	invalid := []string{"true", "null", "0", `"x"`, `[]`, `{}`, `{"use_cases":["a"]}`, `{"reasons":["a"]}`,
		`{"use_cases":["a"],"reasons":[1]}`, `{"use_cases":"a","reasons":[]}`, `{"use_cases":[],"reasons":[],"retry_model":1}`,
		`{"use_cases":[],"reasons":[],"unknown":true}`, `{"use_cases":[[]],"reasons":[]}`, `{"use_cases":[],"reasons":[],"show_buffering_ui":true}`}
	kinds := map[string]func([]byte) error{
		"text delta":      func(b []byte) error { _, err := DecodeTextDelta(b); return err },
		"text done":       func(b []byte) error { _, err := DecodeTextDone(b); return err },
		"arguments delta": func(b []byte) error { _, err := DecodeArgumentsDelta(b); return err },
		"arguments done":  func(b []byte) error { _, err := DecodeArgumentsDone(b); return err },
		"reasoning delta": func(b []byte) error { _, err := DecodeReasoningEvent(ReasoningTextDelta, b); return err },
		"output item":     func(b []byte) error { _, err := DecodeOutputItem(b, false); return err },
		"content part":    func(b []byte) error { _, err := DecodeContentPart(b); return err },
	}
	bodies := map[string]string{
		"text delta":      `{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"pl","logprobs":[]}`,
		"text done":       `{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"pl"}`,
		"arguments delta": `{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{}"}`,
		"arguments done":  `{"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":0,"arguments":"{}"}`,
		"reasoning delta": `{"type":"response.reasoning_text.delta","item_id":"rs_1","output_index":0,"content_index":0,"delta":"x"}`,
		"output item":     `{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message"}}`,
		"content part":    `{"type":"response.content_part.added","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
	}
	for name, decode := range kinds {
		if err := decode([]byte(bodies[name])); err != nil {
			t.Fatalf("%s: baseline refused: %v", name, err)
		}
		for _, value := range valid {
			if err := decode(withMember(bodies[name], value)); err != nil {
				t.Errorf("%s: %s refused: %v", name, value, err)
			}
		}
		for _, value := range invalid {
			if err := decode(withMember(bodies[name], value)); !errors.Is(err, ErrEventShape) {
				t.Errorf("%s: %s accepted: %v", name, value, err)
			}
		}
	}
}

func TestSafetyBufferingOfReadsBothPlaces(t *testing.T) {
	got, err := SafetyBufferingOf("response.output_text.delta", withMember(`{"type":"response.output_text.delta"}`, `{"use_cases":["cyber"],"reasons":["user_risk"],"retry_model":"m"}`))
	if err != nil || got == nil || got.UseCases[0] != "cyber" || got.Reasons[0] != "user_risk" || got.RetryModel != "m" {
		t.Fatalf("top-level: %+v %v", got, err)
	}
	if got, err := SafetyBufferingOf("response.created", withMember(`{"type":"response.created"}`, "false")); got != nil || err != nil {
		t.Fatalf("false: %+v %v", got, err)
	}
	got, err = SafetyBufferingOf(MetadataEvent, []byte(`{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":["cyber"],"reasons":[]}}`))
	if err != nil || got == nil {
		t.Fatalf("metadata: %+v %v", got, err)
	}
	if got, err := SafetyBufferingOf(MetadataEvent, []byte(`{"type":"response.metadata","metadata":{"type":"other"}}`)); got != nil || err != nil {
		t.Fatalf("other metadata: %+v %v", got, err)
	}
	// A metadata member on any other event type is not a notice.
	if got, err := SafetyBufferingOf("response.created", []byte(`{"type":"response.created","metadata":{"type":"safety_buffering","use_cases":[],"reasons":[]}}`)); got != nil || err != nil {
		t.Fatalf("metadata on another event: %+v %v", got, err)
	}
}
