package codex

import (
	"errors"
	"strings"
	"testing"
)

func TestSafetyBufferingListsRefuseNullElementsAndHaveNoCount(t *testing.T) {
	for _, value := range []string{`{"use_cases":[null],"reasons":[]}`, `{"use_cases":[],"reasons":["a",null]}`} {
		if _, err := DecodeSafetyBuffering([]byte(value)); !errors.Is(err, ErrEventShape) {
			t.Errorf("%s accepted: %v", value, err)
		}
	}
	many := `"` + strings.Join(make([]string, 200), `","`) + `"`
	got, err := DecodeSafetyBuffering([]byte(`{"use_cases":[` + many + `],"reasons":[]}`))
	if err != nil || len(got.UseCases) != 200 {
		t.Fatalf("long list: %v", err)
	}
}

func TestSafetyBufferingOfResponseMetadataPrecedence(t *testing.T) {
	nested := `"metadata":{"type":"safety_buffering","use_cases":["nested"],"reasons":[]}`
	top := `"safety_buffering":{"use_cases":["top"],"reasons":[]}`
	check := func(body string, want string) {
		t.Helper()
		got, err := SafetyBufferingOf(MetadataEvent, []byte(body))
		switch {
		case want == "":
			if got != nil || err != nil {
				t.Errorf("%s: want none, got %+v %v", body, got, err)
			}
		case err != nil || got == nil || got.UseCases[0] != want:
			t.Errorf("%s: want %s, got %+v %v", body, want, got, err)
		}
	}
	check(`{"type":"response.metadata","safety_buffering":false,`+nested+`}`, "nested")
	check(`{"type":"response.metadata",`+top+`,`+nested+`}`, "top")
	check(`{"type":"response.metadata",`+nested+`}`, "nested")
	check(`{"type":"response.metadata",`+top+`}`, "")
	check(`{"type":"response.metadata",`+top+`,"metadata":{"type":"other"}}`, "")
	check(`{"type":"response.metadata",`+top+`,"metadata":null}`, "")
	if _, err := SafetyBufferingOf(MetadataEvent, []byte(`{"type":"response.metadata","safety_buffering":true,`+nested+`}`)); !errors.Is(err, ErrEventShape) {
		t.Errorf("true top-level beside a nested notice: %v", err)
	}
}

func TestSafetyBufferingOfRefusesUnparseableObjects(t *testing.T) {
	for _, body := range []string{
		`{"type":"response.in_progress","safety_buffering":true,"safety_buffering":false}`,
		`{"type":"response.in_progress","safety_buffering":false} trailing`,
	} {
		if _, err := SafetyBufferingOf("response.in_progress", []byte(body)); !errors.Is(err, ErrEventShape) {
			t.Errorf("%s: %v", body, err)
		}
	}
	if got, err := SafetyBufferingOf("response.in_progress", []byte(`[]`)); got != nil || err != nil {
		t.Errorf("non-object left to its decoder: %+v %v", got, err)
	}
}
