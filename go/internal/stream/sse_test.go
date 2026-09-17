package stream

import (
	"errors"
	"strings"
	"testing"
)

// terminal marks message_stop as the event that ends a response, matching how the caller
// will configure it. The framing layer has no opinion of its own about which type is last.
func newTestParser(t *testing.T) *Parser {
	t.Helper()
	p := NewParser(DefaultLimits())
	p.IsTerminal = func(eventType string) bool { return eventType == "message_stop" }
	return p
}

// feed pushes a whole stream in one chunk and finishes it.
func feed(t *testing.T, p *Parser, text string) ([]Event, error) {
	t.Helper()
	events, err := p.Push([]byte(text))
	if err != nil {
		return events, err
	}
	return events, p.Finish(true)
}

const goodStream = "event: message_start\n" +
	`data: {"type":"message_start"}` + "\n\n" +
	`data: {"type":"content_block_delta","delta":{"text":"hi"}}` + "\n\n" +
	`data: {"type":"message_stop"}` + "\n\n" +
	"data: [DONE]\n\n"

func TestWellFormedStreamParses(t *testing.T) {
	p := newTestParser(t)
	events, err := feed(t, p, goodStream)
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	want := []string{"message_start", "content_block_delta", "message_stop"}
	for i, name := range want {
		if events[i].Type != name {
			t.Errorf("event %d type = %q, want %q", i, events[i].Type, name)
		}
	}
	if !p.Completed() || !p.SawDone() {
		t.Errorf("completed=%v done=%v, want both true", p.Completed(), p.SawDone())
	}
}

// WIRE04: a chunk boundary carries no meaning. Split at every single byte, which also
// splits multi-byte runes, JSON tokens, CRLF pairs and the blank line that ends a frame.
func TestByteAtATimeFragmentation(t *testing.T) {
	stream := "event: message_start\r\n" +
		`data: {"type":"message_start","text":"한국어 🙂"}` + "\r\n\r\n" +
		`data: {"type":"message_stop"}` + "\n\n" +
		"data: [DONE]\n\n"

	p := newTestParser(t)
	var events []Event
	for i := 0; i < len(stream); i++ {
		produced, err := p.Push([]byte{stream[i]})
		if err != nil {
			t.Fatalf("Push at byte %d: %v", i, err)
		}
		events = append(events, produced...)
	}
	if err := p.Finish(true); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if !strings.Contains(string(events[0].Raw), "한국어 🙂") {
		t.Errorf("multi-byte text did not survive: %q", events[0].Raw)
	}
}

// WIRE05: both line endings, a multi-line data payload, and comment heartbeats.
func TestLineEndingsCommentsAndMultilineData(t *testing.T) {
	p := newTestParser(t)
	events, err := feed(t, p,
		": heartbeat\n\n"+
			"event: message_start\r\n"+
			"data: {\"type\":\r\n"+
			"data: \"message_start\"}\r\n\r\n"+
			": another heartbeat\r\n\r\n"+
			`data: {"type":"message_stop"}`+"\n\n")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if got := string(events[0].Raw); got != "{\"type\":\n\"message_start\"}" {
		t.Errorf("multi-line data joined as %q", got)
	}
}

// A data line with no space after the colon keeps its first character.
func TestDataColonWithoutSpaceKeepsTheByte(t *testing.T) {
	p := newTestParser(t)
	events, err := feed(t, p, `data:{"type":"message_stop"}`+"\n\n")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := string(events[0].Raw); got != `{"type":"message_stop"}` {
		t.Fatalf("raw = %q", got)
	}
}

func TestMalformedFramesAreRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		stream string
		want   error
	}{
		// A field this parser cannot account for. id: and retry: are legal SSE and are
		// still refused: the backend does not send them and a frame half understood is a
		// frame not understood.
		"unknown field id":    {"id: 1\ndata: {\"type\":\"message_stop\"}\n\n", ErrInvalidSSE},
		"unknown field retry": {"retry: 100\ndata: {\"type\":\"message_stop\"}\n\n", ErrInvalidSSE},
		"bare text line":      {"hello\ndata: {\"type\":\"message_stop\"}\n\n", ErrInvalidSSE},
		// Two carriage-return cases, and only the first one actually tests the guard.
		// In the second, the trailing bytes break the JSON and the decoder refuses it
		// whatever the framing does — a mutation run showed that case passing with the
		// carriage-return check deleted. JSON treats \r as whitespace, so a frame whose
		// only defect is a trailing one parses cleanly and is accepted unless the framing
		// layer refuses it first. That is the case worth keeping.
		"trailing carriage return":  {"data: {\"type\":\"message_stop\"}\r\n\n", ErrInvalidSSE},
		"carriage return mid-frame": {"data: {\"type\":\"message_stop\"}\rextra\n\n", ErrInvalidSSE},
		"two event names":           {"event: a\nevent: b\ndata: {\"type\":\"a\"}\n\n", ErrInvalidSSE},
		"name without data":         {"event: message_stop\n\n", ErrInvalidSSE},
		"event name disagrees":      {"event: message_start\ndata: {\"type\":\"message_stop\"}\n\n", ErrInvalidSSE},
		"not json":                  {"data: not json\n\n", ErrInvalidSSE},
		"json array":                {"data: [1,2,3]\n\n", ErrInvalidSSE},
		"json string":               {"data: \"text\"\n\n", ErrInvalidSSE},
		"no type field":             {"data: {\"delta\":\"x\"}\n\n", ErrInvalidSSE},
		"type is not a string":      {"data: {\"type\":42}\n\n", ErrInvalidSSE},
		// WIRE03: a second value riding behind the first.
		"trailing json": {"data: {\"type\":\"message_stop\"}{\"type\":\"evil\"}\n\n", ErrInvalidSSE},
		// WIRE03: a decoder keeps the last duplicate, so the bytes and the reading differ.
		"duplicate key":        {"data: {\"type\":\"keepalive\",\"type\":\"message_stop\"}\n\n", ErrInvalidSSE},
		"duplicate nested-ish": {"data: {\"type\":\"message_stop\",\"a\":1,\"a\":2}\n\n", ErrInvalidSSE},
	} {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(t)
			_, err := p.Push([]byte(tc.stream))
			if err == nil {
				err = p.Finish(true)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// The duplicate-key refusal is a security property, not a tidiness one: without it the
// frame below is read as a heartbeat while the wire said the response was over.
func TestDuplicateTypeCannotDisguiseAnEvent(t *testing.T) {
	p := newTestParser(t)
	_, err := p.Push([]byte("data: {\"type\":\"keepalive\",\"type\":\"message_stop\"}\n\n"))
	if !errors.Is(err, ErrInvalidSSE) {
		t.Fatalf("err = %v, want INVALID_SSE", err)
	}
	if p.Completed() {
		t.Fatal("the disguised event was treated as terminal")
	}
}

// WIRE09 and WIRE10: what may follow a terminal event, and what may not.
func TestTerminalAndSentinelOrdering(t *testing.T) {
	for name, tc := range map[string]struct {
		stream string
		want   error
	}{
		"event after terminal": {
			`data: {"type":"message_stop"}` + "\n\n" + `data: {"type":"content_block_delta"}` + "\n\n",
			ErrEventAfterCompletion,
		},
		"event after done": {
			`data: {"type":"message_stop"}` + "\n\ndata: [DONE]\n\n" + `data: {"type":"x"}` + "\n\n",
			ErrEventAfterCompletion,
		},
		"duplicate terminal": {
			`data: {"type":"message_stop"}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n",
			ErrEventAfterCompletion,
		},
		"done before any terminal": {
			"data: [DONE]\n\n",
			ErrIncompleteResponse,
		},
		"done carrying an event name": {
			`data: {"type":"message_stop"}` + "\n\nevent: done\ndata: [DONE]\n\n",
			ErrIncompleteResponse,
		},
		"duplicate done": {
			`data: {"type":"message_stop"}` + "\n\ndata: [DONE]\n\ndata: [DONE]\n\n",
			ErrEventAfterCompletion,
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(t)
			_, err := p.Push([]byte(tc.stream))
			if err == nil {
				err = p.Finish(true)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// WIRE09: a stream that stops early is not a short success.
func TestIncompleteStreamsAreRefused(t *testing.T) {
	t.Run("no terminal event", func(t *testing.T) {
		p := newTestParser(t)
		if _, err := feed(t, p, `data: {"type":"content_block_delta"}`+"\n\n"); !errors.Is(err, ErrIncompleteResponse) {
			t.Fatalf("err = %v, want INCOMPLETE_RESPONSE", err)
		}
	})
	t.Run("frame never closed", func(t *testing.T) {
		p := newTestParser(t)
		if _, err := feed(t, p, `data: {"type":"message_stop"}`); !errors.Is(err, ErrTruncatedStream) {
			t.Fatalf("err = %v, want TRUNCATED_STREAM", err)
		}
	})
	t.Run("transport reported an early end", func(t *testing.T) {
		p := newTestParser(t)
		if _, err := p.Push([]byte(goodStream)); err != nil {
			t.Fatalf("Push: %v", err)
		}
		if err := p.Finish(false); !errors.Is(err, ErrTruncatedStream) {
			t.Fatalf("err = %v, want TRUNCATED_STREAM even though the frames were well formed", err)
		}
	})
}

// WIRE06: a legal frame above a scanner's default 64 KiB must not be truncated, and the
// real ceiling must still be a ceiling.
func TestFrameSizeBoundary(t *testing.T) {
	limits := DefaultLimits()

	t.Run("well past 64 KiB is accepted", func(t *testing.T) {
		p := newTestParser(t)
		big := strings.Repeat("x", 256*1024)
		events, err := feed(t, p,
			`data: {"type":"content_block_delta","text":"`+big+`"}`+"\n\n"+
				`data: {"type":"message_stop"}`+"\n\n")
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if len(events) != 2 || !strings.Contains(string(events[0].Raw), big) {
			t.Fatal("a legal frame larger than a default scanner buffer was lost")
		}
	})

	t.Run("past the ceiling is refused", func(t *testing.T) {
		p := NewParser(Limits{MaxFrameBytes: 1024, MaxEvents: 10, MaxResponseBytes: 1 << 20})
		_, err := p.Push([]byte("data: " + strings.Repeat("x", 2048) + "\n\n"))
		if !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("err = %v, want FRAME_TOO_LARGE", err)
		}
	})

	t.Run("an unterminated frame cannot grow past the ceiling", func(t *testing.T) {
		// Without this, a sender that never closes a frame holds memory for as long as it
		// likes and no per-frame check ever runs.
		p := NewParser(Limits{MaxFrameBytes: 1024, MaxEvents: 10, MaxResponseBytes: 1 << 20})
		_, err := p.Push([]byte("data: " + strings.Repeat("x", 4096)))
		if !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("err = %v, want FRAME_TOO_LARGE while still buffering", err)
		}
	})

	if limits.MaxFrameBytes != 8*1024*1024 {
		t.Errorf("MaxFrameBytes = %d, want the baseline's 8 MiB", limits.MaxFrameBytes)
	}
}

// WIRE07: event count and total bytes are both bounded.
func TestEventAndByteCeilings(t *testing.T) {
	t.Run("too many events", func(t *testing.T) {
		p := NewParser(Limits{MaxFrameBytes: 4096, MaxEvents: 3, MaxResponseBytes: 1 << 20})
		p.IsTerminal = func(string) bool { return false }
		_, err := p.Push([]byte(strings.Repeat(`data: {"type":"ping"}`+"\n\n", 5)))
		if !errors.Is(err, ErrTooManyEvents) {
			t.Fatalf("err = %v, want TOO_MANY_EVENTS", err)
		}
	})
	t.Run("response too large", func(t *testing.T) {
		p := NewParser(Limits{MaxFrameBytes: 1 << 20, MaxEvents: 1000, MaxResponseBytes: 512})
		_, err := p.Push([]byte(strings.Repeat("x", 600)))
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("err = %v, want RESPONSE_TOO_LARGE", err)
		}
	})
}

// WIRE03: a byte sequence that is not UTF-8 must not be repaired into different text.
func TestInvalidUTF8IsRefusedNotReplaced(t *testing.T) {
	t.Run("invalid byte mid-stream", func(t *testing.T) {
		p := newTestParser(t)
		_, err := p.Push([]byte{'d', 'a', 't', 'a', ':', ' ', '{', 0xFF, '}', '\n', '\n'})
		if !errors.Is(err, ErrInvalidUTF8) {
			t.Fatalf("err = %v, want INVALID_UTF8", err)
		}
	})
	t.Run("truncated rune at the end of the stream", func(t *testing.T) {
		p := newTestParser(t)
		// The first two bytes of a three-byte rune, then nothing more.
		if _, err := p.Push([]byte{0xED, 0x95}); err != nil {
			t.Fatalf("Push should hold an incomplete rune, not fail: %v", err)
		}
		if err := p.Finish(true); !errors.Is(err, ErrInvalidUTF8) {
			t.Fatalf("err = %v, want INVALID_UTF8", err)
		}
	})
	t.Run("a rune split across pushes is not an error", func(t *testing.T) {
		p := newTestParser(t)
		payload := []byte(`data: {"type":"message_stop","t":"한"}` + "\n\n")
		for i := 0; i < len(payload); i++ {
			if _, err := p.Push(payload[i : i+1]); err != nil {
				t.Fatalf("Push at %d: %v", i, err)
			}
		}
		if err := p.Finish(true); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	})
}

// WIRE02: the payload is handed on as the bytes that arrived. A decode-and-re-encode round
// trip would rewrite a large integer through float64 and change an identifier.
func TestPayloadBytesAreNeverRewritten(t *testing.T) {
	const raw = `{"type":"message_stop","id":"msg_01ABCdefGHIjklMNOpqr","big":9007199254740993,` +
		`"exact":0.1000000000000000055511151231257827,"neg":-9223372036854775808}`
	p := newTestParser(t)
	events, err := feed(t, p, "data: "+raw+"\n\n")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := string(events[0].Raw); got != raw {
		t.Fatalf("payload was rewritten\n got: %s\nwant: %s", got, raw)
	}
}

// Sequence numbering, when the backend uses it, must be contiguous from zero and must not
// start or stop partway. A gap is a dropped event nothing else would notice.
func TestSequenceNumbering(t *testing.T) {
	event := func(n string, seq string) string {
		if seq == "" {
			return `data: {"type":"` + n + `"}` + "\n\n"
		}
		return `data: {"type":"` + n + `","sequence_number":` + seq + `}` + "\n\n"
	}

	t.Run("contiguous from zero is accepted", func(t *testing.T) {
		p := newTestParser(t)
		_, err := feed(t, p, event("a", "0")+event("b", "1")+event("message_stop", "2"))
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
	})
	for name, stream := range map[string]string{
		"does not start at zero": event("a", "3"),
		"skips a number":         event("a", "0") + event("b", "2"),
		"repeats a number":       event("a", "0") + event("b", "0"),
		"stops partway":          event("a", "0") + event("b", ""),
		"negative":               event("a", "-1"),
		"not an integer":         event("a", "1.5"),
		"beyond int64":           event("a", "99999999999999999999"),
	} {
		t.Run(name, func(t *testing.T) {
			p := newTestParser(t)
			_, err := p.Push([]byte(stream))
			if err == nil {
				err = p.Finish(true)
			}
			if !errors.Is(err, ErrSequenceMismatch) {
				t.Fatalf("err = %v, want SEQUENCE_MISMATCH", err)
			}
		})
	}
	t.Run("absent throughout is accepted", func(t *testing.T) {
		p := newTestParser(t)
		if _, err := feed(t, p, event("a", "")+event("message_stop", "")); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	})
}

// Heartbeats keep a connection open and carry nothing. They must not be mistaken for
// progress, and an empty frame must not become an event.
func TestCommentsAndEmptyFramesProduceNoEvents(t *testing.T) {
	p := newTestParser(t)
	events, err := feed(t, p,
		"\n\n"+": ping\n\n"+"\r\n\r\n"+": ping\n: ping\n\n"+`data: {"type":"message_stop"}`+"\n\n")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

// A stream that ends with one newline too many is still a complete stream.
//
// Found by review. boundary() consumes the blank line that ends a frame, so any newline
// after it stayed in the buffer -- and a non-empty buffer was read as a frame that never
// closed. Every event had been delivered, the terminal event had arrived, and the response
// was thrown away as truncated. Those bytes are legal SSE and carry nothing.
func TestATrailingNewlineDoesNotTruncateACompleteStream(t *testing.T) {
	for _, tail := range []string{"", "\n", "\n\n", "\r\n"} {
		parser := newTestParser(t)
		if _, err := parser.Push([]byte(goodStream + tail)); err != nil {
			t.Fatalf("Push with tail %q: %v", tail, err)
		}
		if err := parser.Finish(true); err != nil {
			t.Fatalf("a complete stream ending %q was refused: %v", tail, err)
		}
	}

	// A frame that really never closed is still truncated.
	parser := newTestParser(t)
	if _, err := parser.Push([]byte(goodStream + "data: half")); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := parser.Finish(true); !errors.Is(err, ErrTruncatedStream) {
		t.Fatalf("err = %v, want TRUNCATED_STREAM for a frame with no boundary", err)
	}
}
