package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// Names are shaped before they are recorded, for use cases and reasons alike, and the shaped
// label is what is counted -- not a bound that happens to be hit first.
func TestSafetyBufferingNamesAreShaped(t *testing.T) {
	events := bufferingStream()
	value, _ := json.Marshal(map[string]any{
		"use_cases": []string{"cyber", "has space\nand newline"},
		"reasons":   []string{"user_risk", strings.Repeat("x", 65), ""},
	})
	events[4] = buffered(t, events[4], string(value))
	tr, _, err := playStream(t, events)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"notices": 1, "use_case:cyber": 1, "use_case:<other>": 1,
		"reason:user_risk": 1, "reason:<other>": 2,
	}
	got := tr.SafetyBuffering()
	if len(got) != len(want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
	for key, n := range want {
		if got[key] != n {
			t.Errorf("%s = %d, want %d (all: %v)", key, got[key], n, got)
		}
	}
}

// Recording is bounded even though acceptance is not: many distinct names fold into <more>.
func TestSafetyBufferingRecordingIsBoundedNotAcceptance(t *testing.T) {
	names := make([]string, 100)
	for i := range names {
		names[i] = "u" + strings.Repeat("x", i)
	}
	events := bufferingStream()
	value, _ := json.Marshal(map[string]any{"use_cases": names, "reasons": []string{}})
	events[4] = buffered(t, events[4], string(value))
	tr, _, err := playStream(t, events)
	if err != nil {
		t.Fatalf("a valid notice with many entries was refused: %v", err)
	}
	counts := tr.SafetyBuffering()
	if len(counts) > maxSafetyKeys+1 || counts["<more>"] == 0 {
		t.Fatalf("not bounded: %d keys, %v", len(counts), counts)
	}
}

// A response.metadata is accepted only through a valid metadata notice. A top-level false does
// not hide it; a top-level notice does not stand in for it.
func TestResponseMetadataNeedsItsOwnNotice(t *testing.T) {
	nested := `"metadata":{"type":"safety_buffering","use_cases":["cyber"],"reasons":["r"]}`
	tr, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, `{"type":"response.metadata","safety_buffering":false,`+nested+`}`)})
	if err != nil || tr.SafetyBuffering()["use_case:cyber"] != 1 {
		t.Fatalf("top-level false hid the nested notice: %v %v", err, tr.SafetyBuffering())
	}
	var unsupported *UnsupportedEvent
	for _, payload := range []string{
		`{"type":"response.metadata","safety_buffering":{"use_cases":["a"],"reasons":[]}}`,
		`{"type":"response.metadata","safety_buffering":{"use_cases":["a"],"reasons":[]},"metadata":{"type":"other"}}`,
		`{"type":"response.metadata","safety_buffering":{"use_cases":["a"],"reasons":[]},"metadata":"x"}`,
		`{"type":"response.metadata","safety_buffering":{"use_cases":["a"],"reasons":[]},"metadata":null}`,
	} {
		tr, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, payload)})
		if !errors.As(err, &unsupported) || len(tr.SafetyBuffering()) != 0 {
			t.Errorf("%s: want unsupported with nothing recorded, got %v %v", payload, err, tr.SafetyBuffering())
		}
	}
	// An invalid metadata notice is a shape error, not a silent drop.
	if _, _, err := playStream(t, []stream.Event{event(codex.MetadataEvent, `{"type":"response.metadata","metadata":{"type":"safety_buffering","use_cases":[null],"reasons":[]}}`)}); err == nil || errors.As(err, &unsupported) {
		t.Errorf("invalid nested notice: %v", err)
	}
}

// Events without a dedicated decoder still refuse an unreadable object, and an event type this
// build does not know stays reported as unsupported whatever its safety_buffering says.
func TestSafetyBufferingParseFailureRefusesAndUnknownTypesStayNamed(t *testing.T) {
	_, _, err := playStream(t, []stream.Event{event(codex.InProgress, `{"type":"response.in_progress","safety_buffering":true,"safety_buffering":false}`)})
	if !errors.Is(err, codex.ErrEventShape) {
		t.Errorf("duplicate key on in_progress: %v", err)
	}
	for _, kind := range []string{codex.Keepalive, codex.WebsocketTiming, codex.CodexMetadata, codex.RateLimitsUpdated} {
		if _, _, err := playStream(t, []stream.Event{event(kind, `{"type":"x","safety_buffering":true}`)}); !errors.Is(err, codex.ErrEventShape) {
			t.Errorf("%s with true: %v", kind, err)
		}
	}
	var unsupported *UnsupportedEvent
	for _, value := range []string{"true", `{"use_cases":[null],"reasons":[]}`} {
		_, _, err := playStream(t, []stream.Event{event("response.future_thing", `{"type":"response.future_thing","safety_buffering":`+value+`}`)})
		if !errors.As(err, &unsupported) || unsupported.Name != "response.future_thing" {
			t.Errorf("unknown type with %s: %v", value, err)
		}
	}
	// Every known type keeps its shape error.
	for kind := range acceptsEventType {
		if kind == codex.MetadataEvent {
			continue // without a nested notice it stays unsupported, covered above
		}
		_, _, err := playStream(t, []stream.Event{event(kind, `{"type":"`+kind+`","safety_buffering":true}`)})
		if !errors.Is(err, codex.ErrEventShape) {
			t.Errorf("%s with true: %v", kind, err)
		}
	}
}
