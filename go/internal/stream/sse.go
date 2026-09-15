// Package stream parses the Server-Sent Events the backend speaks and tracks the delivery
// state of a response.
//
// The framing rules here are a port of the Node baseline's parser, not a reading of the
// SSE specification, and the difference is deliberate. The baseline is stricter than the
// spec in several places — it rejects id: and retry: fields, rejects a bare carriage
// return, and rejects a frame it cannot fully account for — because a bridge that guesses
// at a malformed frame is a bridge that can hand the client a tool call the backend never
// made. Every strictness below exists to turn an ambiguous byte sequence into a named
// refusal rather than a plausible-looking event.
package stream

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Limits bound what one response may cost. The values match the Node baseline so a stream
// that works against one implementation works against the other.
type Limits struct {
	// MaxFrameBytes caps a single event. 8 MiB in the baseline: large enough for a legal
	// frame carrying a long tool argument, small enough to be a ceiling. A scanner's
	// default 64 KiB would truncate legitimate traffic, which is why this is explicit.
	MaxFrameBytes int
	// MaxEvents caps how many events one response may contain.
	MaxEvents int
	// MaxResponseBytes caps the whole response body.
	MaxResponseBytes int
}

// DefaultLimits mirrors NATIVE_TRANSPORT_LIMITS and NATIVE_LIMITS in the baseline.
func DefaultLimits() Limits {
	return Limits{
		MaxFrameBytes:    8 * 1024 * 1024,
		MaxEvents:        100_000,
		MaxResponseBytes: 16 * 1024 * 1024,
	}
}

// Fixed refusal categories. They are compared by identity, never by message text, so a
// caller can branch on what happened without matching on a string that might be reworded.
var (
	ErrInvalidSSE           = errors.New("INVALID_SSE")
	ErrFrameTooLarge        = errors.New("FRAME_TOO_LARGE")
	ErrTooManyEvents        = errors.New("TOO_MANY_EVENTS")
	ErrResponseTooLarge     = errors.New("RESPONSE_TOO_LARGE")
	ErrInvalidUTF8          = errors.New("INVALID_UTF8")
	ErrTruncatedStream      = errors.New("TRUNCATED_STREAM")
	ErrIncompleteResponse   = errors.New("INCOMPLETE_RESPONSE")
	ErrEventAfterCompletion = errors.New("EVENT_AFTER_COMPLETION")
	ErrSequenceMismatch     = errors.New("SEQUENCE_MISMATCH")
)

// Event is one parsed frame: the raw JSON payload and the type read out of it.
//
// The payload stays as bytes. Deciding what an event means belongs to the protocol
// packages; this one's job is to establish that a frame was well formed and to hand on
// exactly the bytes that arrived, so nothing is normalised away before anyone has looked
// at it.
type Event struct {
	Type string
	Raw  []byte
}

// Parser turns a byte stream into events. Feed it with Push in whatever sizes the network
// produces — one byte at a time is a supported and tested case — and call Finish once.
type Parser struct {
	limits Limits

	pending strings.Builder
	partial []byte // an incomplete UTF-8 rune carried between pushes
	total   int
	events  int
	closed  bool

	// completed records that a terminal event has been seen, sentinel that [DONE] has.
	// They are separate because the contract allows a terminal event without [DONE] and
	// refuses [DONE] without a terminal event, and collapsing them would permit both.
	completed bool
	sentinel  bool

	sequenceMode bool
	nextSequence int64

	// IsTerminal reports whether an event type ends the response. Supplied by the caller
	// because which types are terminal is a backend contract, not a framing rule.
	IsTerminal func(eventType string) bool
}

func NewParser(limits Limits) *Parser {
	if limits.MaxFrameBytes <= 0 {
		limits = DefaultLimits()
	}
	return &Parser{limits: limits, IsTerminal: func(string) bool { return false }}
}

// MarkCompleted records that a terminal event was seen. The caller decides what terminal
// means; the parser only needs to know it happened so it can police what may follow.
func (p *Parser) MarkCompleted() { p.completed = true }

// Completed reports whether a terminal event has been seen.
func (p *Parser) Completed() bool { return p.completed }

// SawDone reports whether the [DONE] sentinel has been seen.
func (p *Parser) SawDone() bool { return p.sentinel }

// Push feeds the next chunk of the response body and returns any events it completed.
//
// Chunk boundaries carry no meaning: a frame, a JSON token, a CRLF pair and a multi-byte
// rune may all be split across pushes. An incomplete rune is held rather than decoded,
// because replacing it with U+FFFD would silently turn a truncated stream into a
// well-formed one carrying different text.
func (p *Parser) Push(chunk []byte) ([]Event, error) {
	if p.closed {
		return nil, ErrInvalidSSE
	}
	p.total += len(chunk)
	if p.total > p.limits.MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}

	buffered := append(p.partial, chunk...)
	p.partial = nil

	// Hold back a trailing incomplete rune; reject anything else that is not valid UTF-8.
	for len(buffered) > 0 && !utf8.Valid(buffered) {
		r, size := utf8.DecodeLastRune(buffered)
		if r != utf8.RuneError || size != 1 {
			return nil, ErrInvalidUTF8
		}
		if len(p.partial) >= utf8.UTFMax {
			return nil, ErrInvalidUTF8
		}
		p.partial = append([]byte{buffered[len(buffered)-1]}, p.partial...)
		buffered = buffered[:len(buffered)-1]
	}

	p.pending.WriteString(string(buffered))
	return p.drain()
}

// Finish ends the stream. complete says whether the transport delivered the whole body;
// a false value means the connection ended early and no amount of well-formed framing
// makes the response trustworthy.
func (p *Parser) Finish(complete bool) error {
	if len(p.partial) > 0 {
		return ErrInvalidUTF8
	}
	p.closed = true
	if !complete {
		return ErrTruncatedStream
	}
	// Anything still buffered is a frame that never reached its blank-line boundary.
	if p.pending.Len() != 0 {
		return ErrTruncatedStream
	}
	if !p.completed {
		return ErrIncompleteResponse
	}
	return nil
}

func (p *Parser) drain() ([]Event, error) {
	var out []Event
	for {
		text := p.pending.String()
		index, width := boundary(text)
		if index < 0 {
			// No complete frame yet. What is buffered still counts against the ceiling,
			// or a sender could hold memory open by never closing a frame.
			if len(text) > p.limits.MaxFrameBytes {
				return out, ErrFrameTooLarge
			}
			return out, nil
		}
		value := text[:index]
		rest := text[index+width:]
		p.pending.Reset()
		p.pending.WriteString(rest)

		event, emit, err := p.frame(value)
		if err != nil {
			return out, err
		}
		if emit {
			out = append(out, event)
		}
	}
}

// boundary finds the blank line that ends a frame, accepting either line ending and
// preferring whichever comes first.
func boundary(text string) (index, width int) {
	lf := strings.Index(text, "\n\n")
	crlf := strings.Index(text, "\r\n\r\n")
	switch {
	case lf < 0 && crlf < 0:
		return -1, 0
	case lf < 0:
		return crlf, 4
	case crlf < 0 || lf < crlf:
		return lf, 2
	default:
		return crlf, 4
	}
}

func (p *Parser) frame(value string) (Event, bool, error) {
	if len(value) > p.limits.MaxFrameBytes {
		return Event{}, false, ErrFrameTooLarge
	}
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	// A carriage return that was not part of a CRLF pair. Treating it as whitespace would
	// let a sender hide content from a line-oriented reader.
	if strings.ContainsRune(normalized, '\r') {
		return Event{}, false, ErrInvalidSSE
	}
	if normalized == "" {
		return Event{}, false, nil
	}

	var name string
	var named bool
	var data []string
	for _, line := range strings.Split(normalized, "\n") {
		switch {
		case strings.HasPrefix(line, ":"):
			// A comment. Heartbeats arrive this way and carry nothing.
		case strings.HasPrefix(line, "event:"):
			if named {
				return Event{}, false, ErrInvalidSSE
			}
			name, named = strings.TrimSpace(line[len("event:"):]), true
		case strings.HasPrefix(line, "data:"):
			if strings.HasPrefix(line, "data: ") {
				data = append(data, line[len("data: "):])
			} else {
				data = append(data, line[len("data:"):])
			}
		default:
			// Includes id: and retry:. The backend does not send them, and a field this
			// parser does not understand is a frame it cannot claim to have understood.
			return Event{}, false, ErrInvalidSSE
		}
	}

	if len(data) == 0 {
		if named {
			return Event{}, false, ErrInvalidSSE
		}
		return Event{}, false, nil
	}
	raw := strings.Join(data, "\n")

	if p.sentinel {
		return Event{}, false, ErrEventAfterCompletion
	}
	if raw == "[DONE]" {
		if named || !p.completed {
			return Event{}, false, ErrIncompleteResponse
		}
		p.sentinel = true
		return Event{}, false, nil
	}
	if p.completed {
		return Event{}, false, ErrEventAfterCompletion
	}

	p.events++
	if p.events > p.limits.MaxEvents {
		return Event{}, false, ErrTooManyEvents
	}

	eventType, err := p.validate(raw, name, named)
	if err != nil {
		return Event{}, false, err
	}
	if p.IsTerminal(eventType) {
		p.completed = true
	}
	return Event{Type: eventType, Raw: []byte(raw)}, true, nil
}

func (p *Parser) validate(raw, name string, named bool) (string, error) {
	decoded, err := decodeShape(raw)
	if err != nil {
		return "", ErrInvalidSSE
	}
	if decoded.Type == "" {
		return "", ErrInvalidSSE
	}

	// An event: name that disagrees with the payload's own type is two claims about one
	// frame, and there is no rule that says which to believe.
	if named && name != decoded.Type {
		return "", ErrInvalidSSE
	}

	if decoded.HasSequence {
		if !decoded.SequenceValid || decoded.Sequence < 0 {
			return "", ErrSequenceMismatch
		}
		if !p.sequenceMode {
			if decoded.Sequence != 0 {
				return "", ErrSequenceMismatch
			}
			p.sequenceMode = true
		}
		if decoded.Sequence != p.nextSequence {
			return "", ErrSequenceMismatch
		}
		p.nextSequence++
	} else if p.sequenceMode {
		// Numbering that starts and then stops leaves a gap nothing can rule out.
		return "", ErrSequenceMismatch
	}

	return decoded.Type, nil
}

// Stats reports what this response has cost so far. Counts only; no content.
func (p *Parser) Stats() (events, bytes int) { return p.events, p.total }

func (p *Parser) String() string {
	return fmt.Sprintf("stream.Parser{events:%d bytes:%d completed:%t done:%t}",
		p.events, p.total, p.completed, p.sentinel)
}
