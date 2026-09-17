// Package wire holds the JSON strictness rules both wire formats share.
//
// It exists so that the one security-relevant decision here — what to do with a JSON
// object that has two readings — is made in a single place. A decoder keeps the last value
// for a repeated key, so a document whose bytes say one thing decodes into another, and a
// second implementation of that rule is a second chance to get it wrong. Everything in this
// package is a leaf: it imports nothing from the rest of the module and decides nothing
// about meaning.
package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Fixed refusals. Callers translate these into their own protocol's category.
var (
	// ErrNotObject means the value was not a JSON object at all.
	ErrNotObject = errors.New("not a JSON object")
	// ErrDuplicateKey means one name appeared twice, so the document has two readings.
	ErrDuplicateKey = errors.New("duplicate key")
	// ErrUnknownField means a name outside the caller's allowlist. Refusing is what keeps
	// an unrecognised field from being silently dropped along with whatever it meant.
	ErrUnknownField = errors.New("unknown field")
	// ErrTrailingData means a second value followed the object.
	ErrTrailingData = errors.New("trailing data")
	// ErrMalformed covers anything else the decoder rejected.
	ErrMalformed = errors.New("malformed JSON")
)

// FieldError names the offending key so a caller can report which field was refused
// without echoing the document.
type FieldError struct {
	Reason error
	Field  string
}

func (e *FieldError) Error() string { return e.Reason.Error() + ": " + e.Field }
func (e *FieldError) Unwrap() error { return e.Reason }

// Fields walks the top level of a JSON object and returns its members unparsed.
//
// Values come back as json.RawMessage — the bytes as they arrived. Nothing is decoded and
// nothing is re-encoded, which is what keeps a large integer or a long identifier intact:
// a round trip through float64 rewrites 9007199254740993 without complaining.
//
// allowed lists the accepted names. A nil slice accepts any name; an empty slice accepts
// none. A name outside the list is refused rather than ignored, because a field nobody
// recognises is a request nobody fully understood.
func Fields(raw []byte, allowed []string) (map[string]json.RawMessage, error) {
	var permitted map[string]bool
	if allowed != nil {
		permitted = make(map[string]bool, len(allowed))
		for _, name := range allowed {
			permitted[name] = true
		}
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()

	open, err := dec.Token()
	if err != nil {
		return nil, ErrMalformed
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, ErrNotObject
	}

	fields := make(map[string]json.RawMessage, 8)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, ErrMalformed
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, ErrMalformed
		}
		if _, seen := fields[key]; seen {
			return nil, &FieldError{Reason: ErrDuplicateKey, Field: key}
		}
		if permitted != nil && !permitted[key] {
			return nil, &FieldError{Reason: ErrUnknownField, Field: key}
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, ErrMalformed
		}
		fields[key] = value
	}

	closeToken, err := dec.Token()
	if err != nil {
		return nil, ErrMalformed
	}
	if delim, ok := closeToken.(json.Delim); !ok || delim != '}' {
		return nil, ErrMalformed
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrTrailingData
	}
	return fields, nil
}

// Presence distinguishes the three states a JSON field can be in. They are not the same
// thing and collapsing them loses a user's choice: an omitted optional enum, an explicit
// null and a value are three different instructions.
type Presence int

const (
	// Absent means the key was not in the document.
	Absent Presence = iota
	// Null means the key was present with the literal value null.
	Null
	// Present means the key carried a value.
	Present
)

func (p Presence) String() string {
	switch p {
	case Absent:
		return "absent"
	case Null:
		return "null"
	case Present:
		return "present"
	default:
		return fmt.Sprintf("Presence(%d)", int(p))
	}
}

// Of reports how a field appeared, given the map Fields returned.
func Of(fields map[string]json.RawMessage, name string) (json.RawMessage, Presence) {
	value, ok := fields[name]
	if !ok {
		return nil, Absent
	}
	if isNull(value) {
		return value, Null
	}
	return value, Present
}

func isNull(value json.RawMessage) bool {
	return strings.TrimSpace(string(value)) == "null"
}
