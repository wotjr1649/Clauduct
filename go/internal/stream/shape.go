package stream

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// shape is the little the framing layer needs to know about an event's JSON: what it calls
// itself, how many top-level keys it had, and its sequence number if it has one.
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

// decodeShape reads the top level of one event object.
//
// The strictness lives in internal/wire, shared with request decoding, so that duplicate
// keys and trailing values are refused by one implementation rather than two. A decoder
// keeps the last value for a repeated key, so {"type":"keepalive","type":"message_stop"}
// would otherwise read as a harmless heartbeat while the bytes said the response was over.
func decodeShape(raw string) (shape, error) {
	var s shape
	fields, err := wire.Fields([]byte(raw), nil)
	if err != nil {
		return s, errShape
	}
	s.Keys = len(fields)

	if value, presence := wire.Of(fields, "type"); presence == wire.Present {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return s, errShape
		}
		s.Type = text
	}

	if value, presence := wire.Of(fields, "sequence_number"); presence != wire.Absent {
		s.HasSequence = true
		// Parsed from the literal text, so a value beyond float64's exact range is
		// reported as invalid rather than silently rounded into a plausible one.
		if number, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64); err == nil {
			s.Sequence = number
			s.SequenceValid = true
		}
	}
	return s, nil
}
