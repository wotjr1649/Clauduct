package anthropic

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
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
}

type textPart struct {
	index   int
	builder []byte
	closed  bool
}

// Builder turns semantic calls into the client's event sequence.
//
// It knows nothing about the backend. The bridge translates backend events into these
// calls, which is what keeps the two wire vocabularies from meeting inside one type.
type Builder struct {
	model      string
	responseID string

	started    bool
	completed  bool
	nextIndex  int
	parts      map[int]*textPart
	totalBytes int
}

func NewBuilder(model string) *Builder {
	return &Builder{model: model, parts: make(map[int]*textPart, 4)}
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
// The first delta for a response opens the message; the first delta for a content index
// opens its block. Opening lazily is what keeps an empty response from announcing a block
// that never gets any content.
func (b *Builder) AppendText(contentIndex int, delta string) ([]Frame, error) {
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
		frames = append(frames, b.messageStart())
	}

	part, existing := b.parts[contentIndex]
	if !existing {
		if len(b.parts) >= maxTextParts {
			return nil, ErrResponseTooLarge
		}
		part = &textPart{index: b.nextIndex}
		b.nextIndex++
		b.parts[contentIndex] = part
		frames = append(frames, contentBlockStart(part.index))
	}
	if part.closed {
		return nil, ErrStreamOrder
	}
	part.builder = append(part.builder, delta...)
	frames = append(frames, contentBlockDelta(part.index, delta))
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
func (b *Builder) FinishText(contentIndex int, snapshot string) ([]Frame, error) {
	if b.completed {
		return nil, ErrStreamOrder
	}
	part, existing := b.parts[contentIndex]
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
	return []Frame{contentBlockStop(part.index)}, nil
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
	if !b.started {
		// A response with no text at all still needs its message frames, or the client
		// is left waiting for a message that never started.
		b.started = true
		frames = append(frames, b.messageStart())
	}

	// Closing in index order rather than map order: a client reading these sequentially
	// must see a stable sequence, and Go's map iteration is deliberately not one.
	open := make([]*textPart, 0, len(b.parts))
	for _, part := range b.parts {
		if !part.closed {
			open = append(open, part)
		}
	}
	for i := 0; i < len(open); i++ {
		for j := i + 1; j < len(open); j++ {
			if open[j].index < open[i].index {
				open[i], open[j] = open[j], open[i]
			}
		}
	}
	for _, part := range open {
		part.closed = true
		frames = append(frames, contentBlockStop(part.index))
	}

	frames = append(frames, messageDelta(usage), Frame{Type: "message_stop", Data: []byte(`{"type":"message_stop"}`)})
	return frames, nil
}

// Text reports what was accumulated, for a caller that needs the whole answer rather than
// its deltas. Used by tests and by any non-streaming aggregation.
func (b *Builder) Text() string {
	ordered := make([]*textPart, 0, len(b.parts))
	for _, part := range b.parts {
		ordered = append(ordered, part)
	}
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].index < ordered[i].index {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	var out []byte
	for _, part := range ordered {
		out = append(out, part.builder...)
	}
	return string(out)
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

func messageDelta(usage Usage) Frame {
	counts := map[string]any{}
	// Unknown stays unknown. Writing 0 for a count the backend never reported would turn
	// "we do not know what this cost" into "it cost nothing".
	if usage.InputKnown {
		counts["input_tokens"] = usage.InputTokens
	}
	if usage.OutputKnown {
		counts["output_tokens"] = usage.OutputTokens
	}
	return frame("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": counts,
	})
}

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
