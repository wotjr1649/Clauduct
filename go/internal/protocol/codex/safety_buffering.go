package codex

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// SafetyBuffering is the backend's notice that it is holding output for extra safety checks.
//
// Shape from openai/codex codex-rs/codex-api (common.rs SafetyBuffering, sse/responses.rs;
// read 2026-10-08, main cba716c0f1): use_cases and reasons are required arrays of strings,
// retry_model is an optional string. Codex only shows a wait notice and keeps reading; a
// blocked result comes through the ordinary terminal events, so the notice changes nothing
// about how the event carrying it is read.
type SafetyBuffering struct {
	UseCases, Reasons []string
	RetryModel        string
}

const (
	// MetadataEvent is the event type that may carry the notice in its metadata member.
	// It is not CodexMetadata ("codex.response.metadata"), which is a different event.
	MetadataEvent = "response.metadata"

	safetyBufferingKey = "safety_buffering"
	// maxSafetyBufferingEntries bounds each list so a hostile stream cannot grow a record.
	maxSafetyBufferingEntries = 16
)

// DecodeSafetyBuffering reads the member's value. false means no buffering and returns
// (nil, nil); an object of exactly the codex shape returns the notice; anything else -- true,
// null, a number, a string, an array, an object with another key or a member of another type --
// (false measured 2026-10-08 on gpt-6.1-sol, on every output_text.delta of an affected stream and on
// response.created) is ErrEventShape, because an active value this build cannot read must not pass as ordinary.
func DecodeSafetyBuffering(value json.RawMessage) (*SafetyBuffering, error) {
	if string(bytes.TrimSpace(value)) == "false" {
		return nil, nil
	}
	return decodeBufferingObject(value, false)
}

func decodeBufferingObject(value json.RawMessage, metadata bool) (*SafetyBuffering, error) {
	allowed := []string{"use_cases", "reasons", "retry_model"}
	if metadata {
		allowed = append(allowed, "type")
	}
	fields, err := wire.Fields(value, allowed)
	if err != nil {
		return nil, ErrEventShape
	}
	var out SafetyBuffering
	if stringList(fields, "use_cases", &out.UseCases) != nil || stringList(fields, "reasons", &out.Reasons) != nil {
		return nil, ErrEventShape
	}
	if raw, present := fields["retry_model"]; present && string(raw) != "null" {
		if json.Unmarshal(raw, &out.RetryModel) != nil || !utf8.ValidString(out.RetryModel) {
			return nil, ErrEventShape
		}
	}
	if metadata {
		var kind string
		if stringField(fields, "type", &kind) != nil || kind != safetyBufferingKey {
			return nil, ErrEventShape
		}
	}
	return &out, nil
}

// stringList reads a required array of strings.
func stringList(fields map[string]json.RawMessage, name string, into *[]string) error {
	raw, present := fields[name]
	if !present || string(raw) == "null" {
		return ErrEventShape
	}
	var list []string
	if json.Unmarshal(raw, &list) != nil || len(list) > maxSafetyBufferingEntries {
		return ErrEventShape
	}
	*into = list
	return nil
}

// SafetyBufferingOf reads the notice an event carries, in either place codex looks: the
// top-level safety_buffering member of any event, or, for a response.metadata event, its
// metadata member when that is typed "safety_buffering". Nil with a nil error means none.
// Content events call it through their strict decoders; the translator calls it for every
// event to record what it saw. An event whose raw bytes are not an object is left to the
// decoder that owns it.
func SafetyBufferingOf(eventType string, raw []byte) (*SafetyBuffering, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, nil
	}
	if eventType == MetadataEvent {
		for name := range fields {
			switch name {
			case "type", "metadata", "sequence_number", safetyBufferingKey:
			default:
				return nil, nil
			}
		}
	}
	if value, present := fields[safetyBufferingKey]; present {
		return DecodeSafetyBuffering(value)
	}
	if eventType != MetadataEvent {
		return nil, nil
	}
	return metadataBuffering(fields)
}

// metadataBuffering reads the response.metadata form. Only a metadata member typed
// "safety_buffering" is a notice; any other metadata is nil here and refused by the translator
// as before. The event itself may carry no member besides type, metadata and sequence_number.
func metadataBuffering(fields map[string]json.RawMessage) (*SafetyBuffering, error) {
	raw, present := fields["metadata"]
	if !present {
		return nil, nil
	}
	nested, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, nil
	}
	var kind string
	if json.Unmarshal(nested["type"], &kind) != nil || kind != safetyBufferingKey {
		return nil, nil
	}
	for name := range fields {
		if name != "type" && name != "metadata" && name != "sequence_number" {
			return nil, ErrEventShape
		}
	}
	return decodeBufferingObject(raw, true)
}

// safetyBufferingOK is the check the strict decoders apply to their own member: absent, false,
// or an object of the codex shape. The translator records the notice separately.
func safetyBufferingOK(fields map[string]json.RawMessage) bool {
	value, present := fields[safetyBufferingKey]
	if !present {
		return true
	}
	_, err := DecodeSafetyBuffering(value)
	return err == nil
}
