package stream

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

// shape is the little the framing layer needs to know about an event's JSON: what it calls
// itself, how many top-level keys it really had, and its sequence number if it has one.
//
// Nothing else is decoded and the payload is never re-encoded. That is what keeps a long
// identifier or a large integer intact: a decode-then-encode round trip through float64
// would quietly rewrite 9007199254740993 and a tool-use id, and the only reliable way not
// to do that is not to do it at all. Event.Raw is the bytes that arrived.
type shape struct {
	Type          string
	Keys          int
	HasSequence   bool
	Sequence      int64
	SequenceValid bool
}

var errShape = errors.New("shape")

// decodeShape walks the top level of one event object.
//
// It is a token walk rather than an unmarshal into a map because a map cannot answer the
// two questions that matter here: how many keys the text actually contained, and whether
// any of them repeated. A decoder keeps the last value for a repeated key, so
// {"type":"keepalive","type":"message_stop"} unmarshals into a harmless heartbeat while
// the bytes on the wire said something else. Duplicate top-level keys are refused
// outright: ambiguous input with two readings is not something to pick a winner from.
func decodeShape(raw string) (shape, error) {
	var s shape
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()

	open, err := dec.Token()
	if err != nil {
		return s, errShape
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		// A top-level array, string or number is not an event.
		return s, errShape
	}

	seen := make(map[string]bool, 8)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return s, errShape
		}
		key, ok := keyToken.(string)
		if !ok {
			return s, errShape
		}
		if seen[key] {
			return s, errShape
		}
		seen[key] = true
		s.Keys++

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return s, errShape
		}
		switch key {
		case "type":
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return s, errShape
			}
			s.Type = text
		case "sequence_number":
			s.HasSequence = true
			// Parsed from the literal text, so a value beyond float64's exact range is
			// reported as invalid rather than silently rounded into a plausible one.
			number, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
			if err == nil {
				s.Sequence = number
				s.SequenceValid = true
			}
		}
	}

	closeToken, err := dec.Token()
	if err != nil {
		return s, errShape
	}
	if delim, ok := closeToken.(json.Delim); !ok || delim != '}' {
		return s, errShape
	}

	// A second value after the object. Ignoring it would mean acting on the first reading
	// of a frame that carried two.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return s, errShape
	}
	return s, nil
}
