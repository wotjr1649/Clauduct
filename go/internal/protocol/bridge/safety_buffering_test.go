package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

const bufferingObject = `{"use_cases":["cyber"],"reasons":["user_risk"],"retry_model":null}`

// buffered returns the event with a safety_buffering member added.
func buffered(t *testing.T, e stream.Event, value string) stream.Event {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(e.Raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["safety_buffering"] = json.RawMessage(value)
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return stream.Event{Type: e.Type, Raw: raw}
}

func bufferingStream() []stream.Event {
	return []stream.Event{
		event(codex.Created, `{"type":"response.created","response":{"id":"resp_abc"}}`),
		event(codex.InProgress, `{"type":"response.in_progress"}`),
		itemAdded(0, `{"id":"msg_1","type":"message"}`),
		event(codex.ContentPartAdd, `{"type":"response.content_part.added","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`),
		textDelta("Hel"),
		textDelta("lo"),
		textDone("Hello"),
		event(codex.ContentPartDon, `{"type":"response.content_part.done","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"Hello","annotations":[]}}`),
		itemDone(0, messageItem("msg_1", "Hello")),
		event(codex.Completed, `{"type":"response.completed","response":{"id":"resp_abc","usage":{"input_tokens":12,"output_tokens":3}}}`),
	}
}

func playStream(t *testing.T, events []stream.Event) (*Translator, string, error) {
	t.Helper()
	tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "")
	var out strings.Builder
	for _, e := range events {
		frames, err := tr.Accept(e)
		if err != nil {
			return tr, out.String(), err
		}
		out.WriteString(joined(frames))
	}
	return tr, out.String(), nil
}

// A stream carrying the buffering object on its events yields exactly the client bytes of the
// same stream without it, and the notices are counted rather than dropped silently.
func TestSafetyBufferingObjectsDoNotChangeClientOutput(t *testing.T) {
	_, plain, err := playStream(t, bufferingStream())
	if err != nil || plain == "" {
		t.Fatalf("plain stream: %q %v", plain, err)
	}
	events := bufferingStream()
	marked := 0
	for i, e := range events {
		switch i {
		case 0: // response.created carries false
			events[i] = buffered(t, e, "false")
		case 3, 4, 5, 6, 7, 8, 9: // content part, deltas, done, item done, completed
			events[i] = buffered(t, e, bufferingObject)
			marked++
		}
	}
	tr, got, err := playStream(t, events)
	if err != nil {
		t.Fatalf("buffered stream refused: %v", err)
	}
	if got != plain {
		t.Fatalf("client output differs\n got: %q\nwant: %q", got, plain)
	}
	counts := tr.SafetyBuffering()
	if counts["notices"] != marked || counts["use_case:cyber"] != marked || counts["reason:user_risk"] != marked {
		t.Fatalf("notices not recorded: %v (want %d)", counts, marked)
	}
}

func TestSafetyBufferingInvalidValueStillFailsTheStream(t *testing.T) {
	for _, value := range []string{"true", "null", "0", `"cyber"`, `[]`, `{}`,
		`{"use_cases":["cyber"]}`, `{"use_cases":["cyber"],"reasons":[1]}`,
		`{"use_cases":["cyber"],"reasons":[],"extra":1}`, `{"use_cases":["cyber"],"reasons":[],"retry_model":3}`,
		`{"use_cases":null,"reasons":[]}`, `{"use_cases":{"a":[{"b":1}]},"reasons":[]}`} {
		for _, index := range []int{0, 1, 4, 9} {
			events := bufferingStream()
			events[index] = buffered(t, events[index], value)
			if _, _, err := playStream(t, events); err == nil {
				t.Errorf("value %s on event %d accepted", value, index)
			}
		}
	}
}

func TestResponseMetadataIsAcceptedOnlyAsASafetyBufferingNotice(t *testing.T) {
	notice := `{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":["cyber"],"reasons":["user_risk"]}}`
	events := bufferingStream()
	withNotice := append([]stream.Event{events[0], event(codex.MetadataEvent, notice)}, events[1:]...)
	_, plain, _ := playStream(t, events)
	tr, got, err := playStream(t, withNotice)
	if err != nil || got != plain {
		t.Fatalf("notice changed the stream: %v\n%q\n%q", err, got, plain)
	}
	if tr.SafetyBuffering()["notices"] != 1 {
		t.Fatalf("not recorded: %v", tr.SafetyBuffering())
	}
	var unsupported *UnsupportedEvent
	for _, payload := range []string{
		`{"type":"response.metadata","metadata":{"type":"other"}}`,
		`{"type":"response.metadata","metadata":{"anything":1}}`,
		`{"type":"response.metadata"}`,
		`{"type":"response.metadata","safety_buffering":false}`,
	} {
		_, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, payload)})
		if !errors.As(err, &unsupported) {
			t.Errorf("%s: want unsupported event, got %v", payload, err)
		}
	}
	for _, payload := range []string{
		`{"type":"response.metadata","metadata":{"type":"safety_buffering"}}`,
		`{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":[],"reasons":[],"x":1}}`,
		`{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":"cyber","reasons":[]}}`,
		`{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":[],"reasons":[]},"extra":1}`,
		`{"type":"response.metadata","safety_buffering":true}`,
	} {
		if _, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, payload)}); err == nil {
			t.Errorf("%s accepted", payload)
		}
	}
}

func TestSafetyBufferingNamesAreShapedAndBounded(t *testing.T) {
	names := make([]string, 0, 40)
	for i := 0; i < 15; i++ {
		names = append(names, "u"+strings.Repeat("x", i))
	}
	events := bufferingStream()
	value, _ := json.Marshal(map[string]any{"use_cases": append(names, "has space\nand newline"), "reasons": []string{}})
	events[4] = buffered(t, events[4], string(value))
	tr, _, err := playStream(t, events)
	if err != nil {
		t.Fatal(err)
	}
	counts := tr.SafetyBuffering()
	if len(counts) > maxSafetyKeys+1 {
		t.Fatalf("unbounded: %d keys", len(counts))
	}
	for key := range counts {
		if strings.ContainsAny(key, " \n") {
			t.Errorf("unshaped key %q", key)
		}
	}
}
