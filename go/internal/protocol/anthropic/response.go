package anthropic

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Fixed refusal categories for response construction.
var (
	// ErrTextMismatch means the backend's own snapshot of a finished text part disagreed
	// with the deltas that built it. Something went missing in between, and emitting
	// either version would be choosing which one to believe.
	ErrTextMismatch = errors.New("TEXT_MISMATCH")
	// ErrStreamOrder means an event arrived somewhere it cannot belong.
	ErrStreamOrder = errors.New("STREAM_ORDER")
	// ErrResponseTooLarge means the accumulated response passed its ceiling.
	ErrResponseTooLarge = errors.New("RESPONSE_TOO_LARGE")
	// ErrUnsupportedToolCall means the backend called a tool that is not callable now.
	// History may name a tool that has since been withdrawn; a new call may not.
	ErrUnsupportedToolCall = errors.New("UNSUPPORTED_TOOL_CALL")
	// ErrInvalidToolCall means a call's arguments could not be read as an object. It is
	// refused rather than repaired: a tool is about to run with them.
	ErrInvalidToolCall = errors.New("INVALID_TOOL_CALL")
	// ErrEmptyReply means the backend completed having produced nothing at all. Handing
	// the client an empty assistant message would make a failure look like an answer.
	ErrEmptyReply = errors.New("EMPTY_REPLY")
)

// Ceilings on one response. The byte ceiling matches the baseline's responseBytes; the
// part ceiling stops an unbounded number of content blocks from being opened.
const (
	maxResponseBytes = 16 * 1024 * 1024
	maxTextParts     = 1024
)

// Frame is one Server-Sent Event for the client. Data is produced here rather than
// forwarded, so serialising it is the intended operation — unlike parsing, where the bytes
// that arrived are the only trustworthy version.
type Frame struct {
	Type string
	Data []byte
}

// WriteTo emits the frame in the shape the client reads: a named event and one data line.
func (f Frame) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte("event: " + f.Type + "\ndata: " + string(f.Data) + "\n\n"))
	return int64(n), err
}

// Usage is what the response cost. Unknown is not zero: a count the backend did not report
// is reported as unknown rather than as free.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	InputKnown   bool
	OutputKnown  bool
	// CacheRead is the part of InputTokens the backend read from its cache. The backend
	// counts it inside input_tokens; Anthropic reports it beside, so it is moved (#91).
	CacheRead      int64
	CacheReadKnown bool
}

type textPart struct {
	index   int
	builder []byte
	closed  bool
	// With stop sequences: how much of builder the client has been given, and where the
	// visible text ends once a sequence is found in this part. hidden is a part that began
	// after the stop, which the client never sees.
	emitted int
	cut     bool
	cutAt   int
	hidden  bool
}

// visible is what the client is shown of this part: the whole text without stop
// sequences, up to the sequence with one.
func (p *textPart) visible() []byte {
	switch {
	case p.hidden:
		return nil
	case p.cut:
		return p.builder[:p.cutAt]
	}
	return p.builder
}

// partKey identifies a text block by the item it belongs to as well as its index.
//
// The index alone is not an identity. content_index counts within one output item, so a
// response with two message items -- which is what a model that speaks, thinks and speaks
// again produces -- sends two blocks numbered zero. Keyed on the index alone the second
// one lands on the first one's part: closed, so the whole response died with STREAM_ORDER,
// and open, so two separate answers were silently concatenated into one block.
type partKey struct {
	item  string
	index int
}

// toolCall is a validated call waiting for the completion that releases it.
type toolCall struct {
	id    string
	name  string
	input []byte
}

// Builder turns semantic calls into the client's event sequence.
//
// It knows nothing about the backend. The bridge translates backend events into these
// calls, which is what keeps the two wire vocabularies from meeting inside one type.
type Builder struct {
	model      string
	responseID string

	started bool
	// announced is whether message_start has been handed to the client. Not the same as
	// started: deferred text opens the message and discards the frame until Complete.
	announced  bool
	completed  bool
	nextIndex  int
	parts      map[partKey]*textPart
	totalBytes int

	// Calls are held here until Complete releases them. Nothing writes a tool_use frame
	// before that point, which is the whole delivery barrier: a stream that fails midway
	// cannot have handed the client something to execute.
	calls    []toolCall
	callIDs  map[string]bool
	callable func(name string) bool

	// Thoughts are held beside the calls and for the same reason. A stream that fails
	// midway must not have handed the client a partial record of the model's reasoning,
	// which the next turn would then send back as though it were complete.
	thoughts          []string
	deferText         bool
	waitChildren      bool
	emptyNotification bool

	// stops are the client's stop_sequences; hold is how many trailing bytes could still be
	// the start of one; stopped is the sequence that ended the text.
	stops   []string
	hold    int
	stopped string
}

// SetStopSequences ends the response's text at the first of these strings, as the client's
// stop_sequences asks. The backend has no such parameter (#149), so the text is cut here:
// the sequence and everything after it are not delivered, a streamed delta holds back what
// could still begin one, and stop_reason says stop_sequence. The backend's own text is kept
// whole for the checks that compare it.
func (b *Builder) SetStopSequences(stops []string) {
	b.stops = stops
	b.hold = 0
	for _, s := range stops {
		b.hold = max(b.hold, len(s)-1)
	}
}

// release hands out as much of part's text as the client may see now.
func (b *Builder) release(part *textPart, final bool) string {
	if part.hidden || part.cut {
		return ""
	}
	if b.stopped != "" {
		// Text another part wrote after the stop.
		part.cut, part.cutAt = true, part.emitted
		return ""
	}
	end := len(part.builder)
	if len(b.stops) > 0 {
		text := string(part.builder)
		// Generation stops where the first sequence is complete: the earliest end, and of
		// two ending together the longer.
		at, which := -1, ""
		for _, s := range b.stops {
			i := strings.Index(text, s)
			if i >= 0 && (at < 0 || i+len(s) < at+len(which) || i+len(s) == at+len(which) && i < at) {
				at, which = i, s
			}
		}
		switch {
		case at >= 0:
			end, part.cut, part.cutAt, b.stopped = max(at, part.emitted), true, at, which
		case !final:
			end = max(part.emitted, len(part.builder)-b.hold)
			for end > part.emitted && end < len(part.builder) && !utf8.RuneStart(part.builder[end]) {
				end--
			}
		}
	}
	out := string(part.builder[part.emitted:end])
	part.emitted = end
	return out
}

// DeferTextUntilComplete lets consumers that return only the last assistant block
// receive reasoning before the answer. It uses the existing bounded text buffer.
// Workflow results are returned only after completion; ordinary text still streams.
func (b *Builder) DeferTextUntilComplete() { b.deferText = true }

// Only a verified native wait step may use this control response. It carries no
// invented answer; the native hook consumes it without an empty-response retry.
func (b *Builder) WaitForChildren() { b.waitChildren = true; b.deferText = true }

// A verified native task notification may have nothing new to report after its
// result was already delivered. Preserve any actual text or tool call.
func (b *Builder) ConsumeEmptyNotification() { b.emptyNotification = true; b.deferText = true }
func (b *Builder) WaitingForChildren() bool {
	return b.completed && len(b.calls) == 0 && (b.waitChildren || b.emptyNotification && b.ReplyEmpty())
}

func (b *Builder) ReplyEmpty() bool {
	if len(b.calls) > 0 {
		return false
	}
	for _, part := range b.parts {
		if len(part.builder) > 0 {
			return false
		}
	}
	return true
}

// AddThought records a chain of thought to emit when the response completes.
//
// The data is this bridge's own envelope, already built and already checked by the caller.
// Held rather than written, so the delivery barrier covers it: nothing about a response
// that fails halfway reaches the transcript.
func (b *Builder) AddThought(data string) {
	b.thoughts = append(b.thoughts, data)
}

func NewBuilder(model string) *Builder {
	return &Builder{
		model:    model,
		parts:    make(map[partKey]*textPart, 4),
		callIDs:  make(map[string]bool, 4),
		callable: func(string) bool { return false },
	}
}

// SetCallable supplies the names a new tool call may use. It is the current definitions
// only: a conversation's history may name a tool that has since been withdrawn, but a new
// call may not, because the client is about to be asked to run it.
func (b *Builder) SetCallable(callable func(name string) bool) {
	if callable != nil {
		b.callable = callable
	}
}

// AddToolCall validates a call and holds it.
//
// It returns no frames on purpose. A tool call is the one thing in a response that causes
// side effects outside this process, so it is released only by Complete, after the backend
// has said the response finished. Arguments are kept as the bytes that arrived — an
// omitted optional, an explicit null and a chosen value are three different instructions
// to whatever runs the tool, and a decode-and-re-encode round trip loses two of them.
func (b *Builder) AddToolCall(id, name string, arguments []byte) error {
	if b.completed {
		return ErrStreamOrder
	}
	if b.stopped != "" {
		// Generated after the stop sequence: the client asked for it not to exist.
		return nil
	}
	// The id has to be addressable: a result comes back naming it, and an id outside this
	// shape produces a result nothing can be matched to. The name is checked for the same
	// shape as defence in depth — a name outside it can never be a defined tool, so the
	// callable check below would refuse it anyway.
	if !identifier.MatchString(id) || !identifier.MatchString(name) {
		return ErrUnsupportedToolCall
	}
	if b.callIDs[id] {
		// One identifier, one call: a result addressed to a repeated id could be matched
		// to either of them.
		return ErrUnsupportedToolCall
	}
	if !b.callable(name) {
		return ErrUnsupportedToolCall
	}
	if !isJSONObject(arguments) {
		return ErrInvalidToolCall
	}

	b.totalBytes += len(arguments)
	if b.totalBytes > maxResponseBytes {
		return ErrResponseTooLarge
	}
	if len(b.calls) >= maxTextParts {
		return ErrResponseTooLarge
	}
	b.callIDs[id] = true
	b.calls = append(b.calls, toolCall{id: id, name: name, input: append([]byte(nil), arguments...)})
	return nil
}

// ToolCallCount reports how many calls are held. Used by a caller that needs to know a
// response carried tool use without inspecting the frames.
func (b *Builder) ToolCallCount() int { return len(b.calls) }

func isJSONObject(raw []byte) bool {
	_, err := wire.Fields(raw, nil)
	return err == nil
}

// SetResponseID records the identifier the backend assigned. It must arrive before any
// text, because the client's message_start carries it and that frame is emitted once.
func (b *Builder) SetResponseID(id string) error {
	if b.started {
		return ErrStreamOrder
	}
	b.responseID = id
	return nil
}

// ResponseID reports the recorded identifier, or a generated placeholder if the backend
// never sent one. The placeholder is visibly local so it cannot be mistaken for a backend
// identifier in a transcript.
func (b *Builder) ResponseID() string {
	if b.responseID != "" {
		return b.responseID
	}
	return "msg_clauduct_local"
}

// AppendText adds a text delta at a content index and returns the frames it produces.
//
// The first delta for a response opens the message, unless a keepalive already did; the
// first delta for a content index opens its block. Opening lazily is what keeps an empty
// response from announcing a block that never gets any content.
func (b *Builder) AppendText(item string, contentIndex int, delta string) ([]Frame, error) {
	if b.completed {
		return nil, ErrStreamOrder
	}
	if contentIndex < 0 {
		return nil, ErrStreamOrder
	}

	b.totalBytes += len(delta)
	if b.totalBytes > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}

	var frames []Frame
	if !b.started {
		b.started = true
		b.announced = !b.deferText
		frames = append(frames, b.messageStart())
	}

	key := partKey{item: item, index: contentIndex}
	part, existing := b.parts[key]
	if !existing {
		if len(b.parts) >= maxTextParts {
			return nil, ErrResponseTooLarge
		}
		if b.stopped != "" {
			// Kept for the snapshot check, never shown, and numbered past every block the
			// client can see so the ones it does see stay consecutive.
			part = &textPart{index: maxTextParts + len(b.parts), hidden: true}
		} else {
			part = &textPart{index: b.nextIndex}
			b.nextIndex++
			frames = append(frames, contentBlockStart(part.index))
		}
		b.parts[key] = part
	}
	if part.closed {
		return nil, ErrStreamOrder
	}
	part.builder = append(part.builder, delta...)
	if released := b.release(part, false); released != "" || len(b.stops) == 0 {
		frames = append(frames, contentBlockDelta(part.index, released))
	}
	if b.deferText {
		return nil, nil
	}
	return frames, nil
}

// FinishText checks the backend's snapshot against what the deltas built.
//
// This is the whole reason the snapshot is read. If they disagree, a delta went missing
// between the backend and here, and neither version can be trusted: emitting the snapshot
// would invent text the client never saw streamed, and emitting the deltas would hand back
// something the backend says it did not produce.
//
// One case is deliberately allowed. A snapshot of empty text with no deltas at all is a
// part that legitimately produced nothing; the baseline once failed on exactly that and
// the fix is carried over. A non-empty snapshot with no deltas is still refused, because
// that is the missing-delta case this check exists for.
func (b *Builder) FinishText(item string, contentIndex int, snapshot string) ([]Frame, error) {
	if b.completed {
		return nil, ErrStreamOrder
	}
	part, existing := b.parts[partKey{item: item, index: contentIndex}]
	if !existing {
		if snapshot == "" {
			// Nothing streamed and nothing claimed. There is no block to close.
			return nil, nil
		}
		return nil, ErrTextMismatch
	}
	if part.closed {
		return nil, ErrStreamOrder
	}
	if string(part.builder) != snapshot {
		return nil, ErrTextMismatch
	}
	part.closed = true
	if b.deferText || part.hidden {
		return nil, nil
	}
	var frames []Frame
	if rest := b.release(part, true); rest != "" {
		frames = append(frames, contentBlockDelta(part.index, rest))
	}
	return append(frames, contentBlockStop(part.index)), nil
}

// Ping keeps a quiet stream alive: an SSE ping, after message_start when the client has not
// been sent one. It opens the message only once the backend has named the response, since
// message_start carries that name, and never after the response is complete.
func (b *Builder) Ping() []Frame {
	if b.completed || !b.announced && b.responseID == "" {
		return nil
	}
	var frames []Frame
	if !b.announced {
		b.started, b.announced = true, true
		frames = append(frames, b.messageStart())
	}
	return append(frames, Frame{Type: "ping", Data: []byte(`{"type":"ping"}`)})
}

// Complete ends the response. Any block still open is closed first, in the order the
// blocks were opened, so a client reading sequentially never sees a message end with a
// block still outstanding.
func (b *Builder) Complete(usage Usage) ([]Frame, error) {
	if b.completed {
		return nil, ErrStreamOrder
	}
	b.completed = true
	var frames []Frame
	if !b.announced {
		// A response that produced only tool calls, or deferred its text, still needs its
		// message frames, or the client is left waiting for a message that never started.
		// One that a keepalive already opened must not be opened twice.
		b.started, b.announced = true, true
		frames = append(frames, b.messageStart())
	}
	if b.WaitingForChildren() {
		return append(frames, contentBlockStart(0), contentBlockDelta(0, ""), contentBlockStop(0), messageDelta(usage, false, ""), Frame{Type: "message_stop", Data: []byte(`{"type":"message_stop"}`)}), nil
	}

	// Closing in index order rather than map order: a client reading these sequentially
	// must see a stable sequence, and Go's map iteration is deliberately not one.
	ordered := b.ordered(nil)
	for _, part := range ordered {
		if part.closed {
			continue
		}
		part.closed = true
		if b.deferText || part.hidden {
			continue
		}
		if rest := b.release(part, true); rest != "" {
			frames = append(frames, contentBlockDelta(part.index, rest))
		}
		frames = append(frames, contentBlockStop(part.index))
	}
	if b.deferText {
		b.nextIndex = 0
	}

	// Streaming responses retain the baseline's text/thought/tool order. Deferred
	// Workflow responses put thoughts first so the last assistant block is the answer.
	for _, data := range b.thoughts {
		index := b.nextIndex
		b.nextIndex++
		frames = append(frames, thoughtBlockStart(index, data), contentBlockStop(index))
	}
	// One block, the parts joined as the baseline joined them: a reader that keeps only the
	// last assistant block -- Workflow agent(), the SDK -- otherwise saw only the last part
	// of an answer the backend wrote in two (#91).
	if b.deferText && len(b.parts) > 0 {
		texts := make([]string, 0, len(ordered))
		for _, part := range ordered {
			if !part.hidden {
				texts = append(texts, string(part.visible()))
			}
		}
		index := b.nextIndex
		b.nextIndex++
		frames = append(frames, contentBlockStart(index), contentBlockDelta(index, strings.Join(texts, "\n")), contentBlockStop(index))
	}

	// Every text block is closed before the first tool block opens. A client reading
	// sequentially must not see a tool call arrive inside an unfinished answer.
	for _, call := range b.calls {
		index := b.nextIndex
		b.nextIndex++
		frames = append(frames,
			toolBlockStart(index, call),
			toolBlockDelta(index, call.input),
			contentBlockStop(index))
	}

	// A response that produced neither text nor a call produced nothing. Emitting an empty
	// assistant message would make that failure look like an answer.
	if len(b.parts) == 0 && len(b.calls) == 0 {
		return nil, ErrEmptyReply
	}

	frames = append(frames, messageDelta(usage, len(b.calls) > 0, b.stopped),
		Frame{Type: "message_stop", Data: []byte(`{"type":"message_stop"}`)})
	return frames, nil
}

// thoughtBlockStart opens a redacted_thinking block, which carries its data inline rather
// than streaming it: there is nothing to render progressively in an opaque record.
func thoughtBlockStart(index int, data string) Frame {
	return frame("content_block_start", map[string]any{
		"type": "content_block_start", "index": index,
		"content_block": map[string]any{"type": "redacted_thinking", "data": data},
	})
}

func toolBlockStart(index int, call toolCall) Frame {
	// The block opens with empty input and the arguments follow as a delta, matching how
	// the client reads a streamed tool call.
	return frame("content_block_start", map[string]any{
		"type": "content_block_start", "index": index,
		"content_block": map[string]any{
			"type": "tool_use", "id": call.id, "name": call.name, "input": map[string]any{},
		},
	})
}

func toolBlockDelta(index int, input []byte) Frame {
	return frame("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)},
	})
}

// Text reports what was accumulated, for a caller that needs the whole answer rather than
// its deltas. Used by tests and by any non-streaming aggregation.
func (b *Builder) Text() string { return b.textOf(nil, false) }

// Answer excludes tool turns and incomplete responses. Reasoning is never text.
func (b *Builder) Answer() string {
	if b.WaitingForChildren() {
		return ""
	}
	if !b.completed || len(b.calls) != 0 {
		return ""
	}
	return b.textOf(nil, true)
}

// TextFor reports what one output item accumulated.
//
// Separate from Text because the two answer different questions, and answering the second
// with the first is a defect: the completed payload states each item's own text, so
// checking item two's account against the whole response compares "B" with "AB" and calls
// a sound response a mismatch.
func (b *Builder) TextFor(item string) string {
	return b.textOf(func(key partKey) bool { return key.item == item }, false)
}

// textOf joins the parts keep selects: as the backend wrote them, or as the client sees them.
func (b *Builder) textOf(keep func(partKey) bool, shown bool) string {
	var out []byte
	for _, part := range b.ordered(keep) {
		if shown {
			out = append(out, part.visible()...)
		} else {
			out = append(out, part.builder...)
		}
	}
	return string(out)
}

// ordered lists the parts keep selects (all, for nil) in the order they began.
func (b *Builder) ordered(keep func(partKey) bool) []*textPart {
	ordered := make([]*textPart, 0, len(b.parts))
	for key, part := range b.parts {
		if keep == nil || keep(key) {
			ordered = append(ordered, part)
		}
	}
	slices.SortFunc(ordered, func(x, y *textPart) int { return x.index - y.index })
	return ordered
}

func (b *Builder) messageStart() Frame {
	message := map[string]any{
		"id":            b.ResponseID(),
		"type":          "message",
		"role":          "assistant",
		"model":         b.model,
		"content":       []any{},
		"stop_reason":   nil,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": 0, "output_tokens": 0,
			"cache_read_input_tokens": 0, "cache_creation_input_tokens": 0,
		},
	}
	return frame("message_start", map[string]any{"type": "message_start", "message": message})
}

func contentBlockStart(index int) Frame {
	return frame("content_block_start", map[string]any{
		"type": "content_block_start", "index": index,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
}

func contentBlockDelta(index int, text string) Frame {
	return frame("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "text_delta", "text": text},
	})
}

func contentBlockStop(index int) Frame {
	return frame("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
}

func messageDelta(usage Usage, toolUse bool, stopped string) Frame {
	reason, sequence := "end_turn", any(nil)
	switch {
	case toolUse:
		// The client reads this to know the turn is waiting on it rather than finished.
		reason = "tool_use"
	case stopped != "":
		reason, sequence = "stop_sequence", stopped
	}
	counts := map[string]any{}
	// Unknown stays unknown. Writing 0 for a count the backend never reported would turn
	// "we do not know what this cost" into "it cost nothing".
	if usage.InputKnown {
		counts["input_tokens"] = usage.InputTokens
		if usage.CacheReadKnown && usage.CacheRead <= usage.InputTokens {
			counts["input_tokens"] = usage.InputTokens - usage.CacheRead
			counts["cache_read_input_tokens"] = usage.CacheRead
		}
	}
	if usage.OutputKnown {
		counts["output_tokens"] = usage.OutputTokens
	}
	return frame("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": reason, "stop_sequence": sequence},
		"usage": counts,
	})
}

// Frames exports the frame constructor for the search path, which synthesises a whole
// reply rather than translating a stream.
func Frames(kind string, body map[string]any) Frame { return frame(kind, body) }

func frame(kind string, body map[string]any) Frame {
	data, err := json.Marshal(body)
	if err != nil {
		// Every value here is built from typed fields this package controls, so a failure
		// would be a programming error rather than bad input. Say so rather than emit a
		// half-formed frame the client would try to read.
		return Frame{Type: kind, Data: []byte(`{"type":"error","error":{"type":"api_error","message":"FRAME_ENCODE_FAILED"}}`)}
	}
	return Frame{Type: kind, Data: data}
}

// ErrorFrame is the terminal event a client receives when a response cannot continue. The
// category is a fixed string this bridge chose; nothing from the backend's body is echoed.
func ErrorFrame(category string) Frame {
	return frame("error", map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "api_error", "message": category},
	})
}

func (b *Builder) String() string {
	return "anthropic.Builder{parts:" + strconv.Itoa(len(b.parts)) +
		" started:" + strconv.FormatBool(b.started) +
		" completed:" + strconv.FormatBool(b.completed) + "}"
}
