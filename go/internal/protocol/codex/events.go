// Package codex describes the backend's wire vocabulary. It is the other side of the
// bridge from package anthropic and neither imports the other: a type that spoke both
// formats would make it possible to forward a field without anyone deciding to.
//
// The vocabulary is the OpenAI Responses event set, read off the Node baseline's transport
// rather than from a specification, because what matters is what this backend sends.
package codex

import (
	"encoding/json"
	"errors"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Event types this build understands. An event outside this set is refused rather than
// skipped: an unrecognised event may be the one carrying the result, and continuing past
// it would produce a response that is short by exactly the part nobody read.
const (
	Created        = "response.created"
	InProgress     = "response.in_progress"
	Queued         = "response.queued"
	OutputItemAdd  = "response.output_item.added"
	OutputItemDone = "response.output_item.done"
	ContentPartAdd = "response.content_part.added"
	ContentPartDon = "response.content_part.done"
	TextDelta      = "response.output_text.delta"
	TextDone       = "response.output_text.done"
	Completed      = "response.completed"

	// Reasoning arrives as its own items. This build carries none of it to the client:
	// reasoning is opaque, and inventing a signature or leaking it into visible text
	// would be passing the backend's private work off as an Anthropic feature.
	ReasoningPartAdd     = "response.reasoning_part.added"
	ReasoningPartDone    = "response.reasoning_part.done"
	ReasoningSummaryAdd  = "response.reasoning_summary_part.added"
	ReasoningSummaryDone = "response.reasoning_summary_part.done"
	ReasoningSummaryTxtD = "response.reasoning_summary_text.delta"
	ReasoningSummaryTxtF = "response.reasoning_summary_text.done"

	// Failures. Each is terminal the moment it arrives; there is no trailer to wait for
	// and the body is never retained.
	Failed     = "response.failed"
	Incomplete = "response.incomplete"
	ErrorEvent = "error"

	// Connection keepalives carry nothing and must not be read as progress.
	Keepalive = "keepalive"
	Ping      = "ping"
)

// Fixed refusal categories.
var (
	ErrUnsupportedEvent = errors.New("UNSUPPORTED_EVENT")
	ErrEventShape       = errors.New("EVENT_SHAPE")
	ErrResponseFailed   = errors.New("UPSTREAM_RESPONSE_FAILED")
	ErrResponseIncomplt = errors.New("UPSTREAM_RESPONSE_INCOMPLETE")
	ErrErrorEvent       = errors.New("UPSTREAM_ERROR_EVENT")
)

// Terminal reports whether an event type ends the response, successfully or not. The SSE
// parser takes this so it can police what may follow.
func Terminal(eventType string) bool {
	switch eventType {
	case Completed, Failed, Incomplete, ErrorEvent:
		return true
	}
	return false
}

// Failure maps a terminal failure type to its category, or nil if the type is not one.
func Failure(eventType string) error {
	switch eventType {
	case Failed:
		return ErrResponseFailed
	case Incomplete:
		return ErrResponseIncomplt
	case ErrorEvent:
		return ErrErrorEvent
	}
	return nil
}

// TextDeltaEvent is the incremental text the model produced.
type TextDeltaEvent struct {
	ItemID       string
	ContentIndex int
	Delta        string
}

// TextDoneEvent is the backend's own snapshot of a finished text part. It exists to be
// compared against what the deltas built, which is the only way to notice that a delta
// went missing between here and there.
type TextDoneEvent struct {
	ItemID       string
	ContentIndex int
	Text         string
}

// Usage is what the backend reported it spent. Absent counts stay absent: a missing number
// is unknown, and reporting it as zero would turn "we do not know" into "it cost nothing".
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	InputKnown   bool
	OutputKnown  bool
}

// DecodeTextDelta reads a response.output_text.delta payload.
func DecodeTextDelta(raw []byte) (TextDeltaEvent, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return TextDeltaEvent{}, ErrEventShape
	}
	var event TextDeltaEvent
	if err := stringField(fields, "item_id", &event.ItemID); err != nil {
		return TextDeltaEvent{}, err
	}
	if err := intField(fields, "content_index", &event.ContentIndex); err != nil {
		return TextDeltaEvent{}, err
	}
	// A delta is required and may legitimately be empty; absent is not the same thing.
	if err := stringField(fields, "delta", &event.Delta); err != nil {
		return TextDeltaEvent{}, err
	}
	return event, nil
}

// DecodeTextDone reads a response.output_text.done payload.
func DecodeTextDone(raw []byte) (TextDoneEvent, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return TextDoneEvent{}, ErrEventShape
	}
	var event TextDoneEvent
	if err := stringField(fields, "item_id", &event.ItemID); err != nil {
		return TextDoneEvent{}, err
	}
	if err := intField(fields, "content_index", &event.ContentIndex); err != nil {
		return TextDoneEvent{}, err
	}
	if err := stringField(fields, "text", &event.Text); err != nil {
		return TextDoneEvent{}, err
	}
	return event, nil
}

// DecodeResponseID reads the identifier out of a response.created payload.
func DecodeResponseID(raw []byte) (string, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return "", ErrEventShape
	}
	value, presence := wire.Of(fields, "response")
	if presence != wire.Present {
		return "", ErrEventShape
	}
	response, err := wire.Fields(value, nil)
	if err != nil {
		return "", ErrEventShape
	}
	var id string
	if err := stringField(response, "id", &id); err != nil {
		return "", err
	}
	return id, nil
}

// DecodeUsage reads the counts out of a response.completed payload. A payload without
// usage is not an error: the counts are simply unknown.
func DecodeUsage(raw []byte) (Usage, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return Usage{}, ErrEventShape
	}
	value, presence := wire.Of(fields, "response")
	if presence != wire.Present {
		return Usage{}, nil
	}
	response, err := wire.Fields(value, nil)
	if err != nil {
		return Usage{}, ErrEventShape
	}
	usageValue, presence := wire.Of(response, "usage")
	if presence != wire.Present {
		return Usage{}, nil
	}
	usage, err := wire.Fields(usageValue, nil)
	if err != nil {
		return Usage{}, ErrEventShape
	}

	var out Usage
	if value, presence := wire.Of(usage, "input_tokens"); presence == wire.Present {
		var count int
		if intValue(value, &count) == nil {
			out.InputTokens, out.InputKnown = int64(count), true
		}
	}
	if value, presence := wire.Of(usage, "output_tokens"); presence == wire.Present {
		var count int
		if intValue(value, &count) == nil {
			out.OutputTokens, out.OutputKnown = int64(count), true
		}
	}
	return out, nil
}

func stringField(fields map[string]json.RawMessage, name string, into *string) error {
	value, presence := wire.Of(fields, name)
	if presence != wire.Present {
		return ErrEventShape
	}
	if err := json.Unmarshal(value, into); err != nil {
		return ErrEventShape
	}
	return nil
}

func intField(fields map[string]json.RawMessage, name string, into *int) error {
	value, presence := wire.Of(fields, name)
	if presence != wire.Present {
		return ErrEventShape
	}
	return intValue(value, into)
}

// intValue reads an integer from its literal text rather than through json.Number, which
// accepts the JSON string "3" as the number 3, or through float64, which accepts 1e100 as
// an integer. Measured on the request side and applied here for the same reason.
func intValue(value json.RawMessage, into *int) error {
	text := string(value)
	number := 0
	negative := false
	if len(text) > 0 && text[0] == '-' {
		negative, text = true, text[1:]
	}
	if text == "" {
		return ErrEventShape
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return ErrEventShape
		}
		number = number*10 + int(digit-'0')
		if number > 1<<40 {
			return ErrEventShape
		}
	}
	if negative {
		number = -number
	}
	*into = number
	return nil
}
