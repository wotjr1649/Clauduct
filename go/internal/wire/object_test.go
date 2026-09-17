package wire

import (
	"errors"
	"testing"
)

func TestFieldsReturnsBytesUnchanged(t *testing.T) {
	// WIRE02: a large integer, a long identifier and a precise decimal must come back
	// exactly as they arrived. A decode-and-re-encode round trip rewrites all three.
	const raw = `{"big":9007199254740993,"id":"msg_01ABCdefGHIjklMNOpqr",` +
		`"exact":0.1000000000000000055511151231257827,"neg":-9223372036854775808}`
	fields, err := Fields([]byte(raw), nil)
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	for name, want := range map[string]string{
		"big":   "9007199254740993",
		"id":    `"msg_01ABCdefGHIjklMNOpqr"`,
		"exact": "0.1000000000000000055511151231257827",
		"neg":   "-9223372036854775808",
	} {
		if got := string(fields[name]); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}

// WIRE01: absent, null and present are three states, not two. An optional enum that was
// omitted, one explicitly set to null, and one with a value are three different
// instructions and a caller has to be able to tell them apart.
func TestPresenceDistinguishesAbsentNullAndValue(t *testing.T) {
	for name, tc := range map[string]struct {
		raw   string
		want  Presence
		value string
	}{
		"absent":       {`{}`, Absent, ""},
		"null":         {`{"isolation":null}`, Null, "null"},
		"value":        {`{"isolation":"worktree"}`, Present, `"worktree"`},
		"empty string": {`{"isolation":""}`, Present, `""`},
		"empty object": {`{"isolation":{}}`, Present, `{}`},
		"empty array":  {`{"isolation":[]}`, Present, `[]`},
		"false":        {`{"isolation":false}`, Present, `false`},
		"zero":         {`{"isolation":0}`, Present, `0`},
		"spaced null":  {`{"isolation": null }`, Null, "null"},
	} {
		t.Run(name, func(t *testing.T) {
			fields, err := Fields([]byte(tc.raw), nil)
			if err != nil {
				t.Fatalf("Fields: %v", err)
			}
			value, presence := Of(fields, "isolation")
			if presence != tc.want {
				t.Fatalf("presence = %v, want %v", presence, tc.want)
			}
			if tc.value != "" && string(value) != tc.value {
				t.Fatalf("value = %s, want %s", value, tc.value)
			}
		})
	}
}

// The three shapes the handoff names explicitly. They must not collapse into each other,
// because "the user omitted this", "the user cleared this" and "the user chose this" are
// three different instructions about a tool call.
func TestTheThreeIsolationShapesStayDistinct(t *testing.T) {
	seen := make(map[Presence]string)
	for _, raw := range []string{`{}`, `{"isolation":null}`, `{"isolation":"worktree"}`} {
		fields, err := Fields([]byte(raw), nil)
		if err != nil {
			t.Fatalf("Fields(%s): %v", raw, err)
		}
		_, presence := Of(fields, "isolation")
		if previous, clash := seen[presence]; clash {
			t.Fatalf("%s and %s both read as %v", previous, raw, presence)
		}
		seen[presence] = raw
	}
	if len(seen) != 3 {
		t.Fatalf("got %d distinct readings, want 3", len(seen))
	}
}

func TestRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		raw     string
		allowed []string
		want    error
	}{
		"duplicate key":       {`{"a":1,"a":2}`, nil, ErrDuplicateKey},
		"duplicate with null": {`{"a":null,"a":1}`, nil, ErrDuplicateKey},
		"unknown field":       {`{"a":1,"b":2}`, []string{"a"}, ErrUnknownField},
		"nothing allowed":     {`{"a":1}`, []string{}, ErrUnknownField},
		"trailing object":     {`{"a":1}{"b":2}`, nil, ErrTrailingData},
		"trailing scalar":     {`{"a":1} 7`, nil, ErrTrailingData},
		"array not object":    {`[1,2]`, nil, ErrNotObject},
		"string not object":   {`"text"`, nil, ErrNotObject},
		"number not object":   {`42`, nil, ErrNotObject},
		"truncated":           {`{"a":1`, nil, ErrMalformed},
		"not json":            {`nope`, nil, ErrMalformed},
		"empty":               {``, nil, ErrMalformed},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Fields([]byte(tc.raw), tc.allowed)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// A refusal names the field so a caller can say which one, without echoing the document
// back at whoever sent it.
func TestRefusalNamesTheField(t *testing.T) {
	_, err := Fields([]byte(`{"model":"x","surprise":1}`), []string{"model"})
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err = %v, want a *FieldError", err)
	}
	if fieldErr.Field != "surprise" {
		t.Fatalf("Field = %q, want surprise", fieldErr.Field)
	}
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("err does not unwrap to ErrUnknownField")
	}
}

// A duplicate is refused before the allowlist is consulted: the document is ambiguous
// whether or not the repeated name was one we wanted.
func TestDuplicateIsRefusedEvenForAnAllowedName(t *testing.T) {
	_, err := Fields([]byte(`{"model":"a","model":"b"}`), []string{"model"})
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("err = %v, want ErrDuplicateKey", err)
	}
}

// nil allows everything; an empty slice allows nothing. Those are different and a caller
// passing a filtered-to-empty list must not accidentally get "anything goes".
func TestNilAllowlistIsNotAnEmptyOne(t *testing.T) {
	if _, err := Fields([]byte(`{"anything":1}`), nil); err != nil {
		t.Fatalf("nil allowlist should accept any name: %v", err)
	}
	if _, err := Fields([]byte(`{"anything":1}`), []string{}); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("empty allowlist should accept none, got %v", err)
	}
}

func TestEmptyObjectIsValid(t *testing.T) {
	fields, err := Fields([]byte(`{}`), []string{"a"})
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(fields) != 0 {
		t.Fatalf("got %d fields, want 0", len(fields))
	}
}
