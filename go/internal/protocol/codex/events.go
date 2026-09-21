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

	// A tool call's arguments are streamed before the call exists. This build does not
	// build calls from them -- that is the delivery barrier, and a call comes only from
	// response.completed -- but they still arrive, and refusing an event that arrives on
	// every tool request kills every tool request. Measured 2026-09-15: a request carrying
	// a tool definition was refused as UNSUPPORTED_EVENT before any call came back.
	//
	// They are not merely tolerated. The done snapshot is checked against what was
	// streamed, the same way a text snapshot is, because a disagreement means a delta went
	// missing and a tool call is not a thing to execute on a stream nobody can account for.
	FuncArgsDelta = "response.function_call_arguments.delta"
	FuncArgsDone  = "response.function_call_arguments.done"

	// Reasoning text, the same family as the reasoning parts above and carried to the
	// client for the same reason: none.
	ReasoningTextDelta = "response.reasoning_text.delta"
	ReasoningTextDone  = "response.reasoning_text.done"

	// Informational events. They carry no part of the answer, so ignoring one cannot make
	// a response short by the part nobody read -- which is the reason the default is to
	// refuse. Naming them is what lets that default stay strict everywhere else.
	RateLimitsUpdated = "rate_limits.updated"
	CodexRateLimits   = "codex.rate_limits"
	CodexMetadata     = "codex.response.metadata"
	WebsocketTiming   = "responsesapi.websocket_timing"
)

// Events deliberately still refused, and why. Each carries content this build does not
// support, so accepting and dropping one would produce a reply short by exactly that part:
//
//	response.refusal.delta/.done          a refusal is the model's answer
//	response.output_text.annotation.added citations attached to text
//	response.custom_tool_call_input.*     custom tools, which this build does not offer
//	response.web_search_call.*            hosted search, deferred to WP07
//	item.started/.updated/.completed      a second item protocol nothing here reads
//
// The Node baseline names all of these. That it names them is evidence they can arrive, not
// evidence of what to do with them, and guessing a disposition for content is how a reply
// loses a part silently.

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

	// The counts that arrive alongside those two and were read by nothing until 2026-09-18.
	//
	// Measured that day on the real backend: a response.completed carries total_tokens,
	// input_tokens_details.cached_tokens, input_tokens_details.cache_write_tokens and
	// output_tokens_details.reasoning_tokens. cached_tokens read 3,840 on a 16,865-token
	// prefix, so it is a reading and not a field that is always zero -- while the client's
	// display said "0 cached" as a constant, because nothing here had ever looked.
	//
	// Reasoning tokens are billed. Not counting them made every cost this build reported an
	// undercount by an amount nobody could name.
	CachedInputTokens int64
	CacheWriteTokens  int64
	ReasoningTokens   int64
	TotalTokens       int64
	CachedInputKnown  bool
	CacheWriteKnown   bool
	ReasoningKnown    bool
	TotalKnown        bool
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

// ArgumentsDeltaEvent is one piece of a tool call's arguments as it streams.
type ArgumentsDeltaEvent struct {
	ItemID string
	Delta  string
}

// ArgumentsDoneEvent is the backend's account of the arguments it finished writing.
type ArgumentsDoneEvent struct {
	ItemID    string
	Arguments string
}

// DecodeArgumentsDelta reads a response.function_call_arguments.delta payload.
func DecodeArgumentsDelta(raw []byte) (ArgumentsDeltaEvent, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return ArgumentsDeltaEvent{}, ErrEventShape
	}
	var event ArgumentsDeltaEvent
	if err := stringField(fields, "item_id", &event.ItemID); err != nil {
		return ArgumentsDeltaEvent{}, err
	}
	// Required, and may legitimately be empty; absent is not the same thing.
	if err := stringField(fields, "delta", &event.Delta); err != nil {
		return ArgumentsDeltaEvent{}, err
	}
	return event, nil
}

// DecodeArgumentsDone reads a response.function_call_arguments.done payload.
//
// The arguments are taken as the string they are, not parsed. Whether they are valid JSON
// is decided where the call is built, from the completed output; here they are only the
// backend's statement of what it streamed, and re-encoding them would make them agree with
// the accumulated deltas for the wrong reason.
func DecodeArgumentsDone(raw []byte) (ArgumentsDoneEvent, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return ArgumentsDoneEvent{}, ErrEventShape
	}
	var event ArgumentsDoneEvent
	if err := stringField(fields, "item_id", &event.ItemID); err != nil {
		return ArgumentsDoneEvent{}, err
	}
	if err := stringField(fields, "arguments", &event.Arguments); err != nil {
		return ArgumentsDoneEvent{}, err
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
	readCount(usage, "input_tokens", &out.InputTokens, &out.InputKnown)
	readCount(usage, "output_tokens", &out.OutputTokens, &out.OutputKnown)
	readCount(usage, "total_tokens", &out.TotalTokens, &out.TotalKnown)
	readDetail(usage, "input_tokens_details", "cached_tokens",
		&out.CachedInputTokens, &out.CachedInputKnown)
	readDetail(usage, "input_tokens_details", "cache_write_tokens",
		&out.CacheWriteTokens, &out.CacheWriteKnown)
	readDetail(usage, "output_tokens_details", "reasoning_tokens",
		&out.ReasoningTokens, &out.ReasoningKnown)
	return out, nil
}

// readCount takes one count, leaving it unknown when it is absent or unreadable.
//
// Unreadable is deliberately the same answer as absent rather than an error. A usage object
// whose shape drifts in one member must not cost the counts beside it: losing input_tokens
// because a sibling changed would be a worse answer than losing the sibling.
func readCount(fields map[string]json.RawMessage, name string, into *int64, known *bool) {
	value, presence := wire.Of(fields, name)
	if presence != wire.Present {
		return
	}
	var count int
	if intValue(value, &count) == nil {
		*into, *known = int64(count), true
	}
}

// readDetail takes one count out of a nested details object, on the same terms.
func readDetail(fields map[string]json.RawMessage, parent, name string, into *int64, known *bool) {
	value, presence := wire.Of(fields, parent)
	if presence != wire.Present {
		return
	}
	nested, err := wire.Fields(value, nil)
	if err != nil {
		return
	}
	readCount(nested, name, into, known)
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
