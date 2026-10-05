package bridge

import (
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"testing"
)

func TestMalformedReturnedRoute319(t *testing.T) {
	for _, route := range []string{`"model":17`, `"model":null`, `"model":""`, `"reasoning":false`, `"reasoning":{"effort":17}`, `"reasoning":{"effort":""}`} {
		tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6-luna")
		tr.ExpectRoute("gpt-6-luna", "low")
		if _, err := tr.Accept(textDelta("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Accept(textDone("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Accept(event(codex.Completed, `{"type":"response.completed","response":{"id":"r",`+route+`,`+usageReported+`}}`)); !errors.Is(err, codex.ErrEventShape) {
			t.Errorf("malformed route accepted: %s", route)
		}
	}
}

func TestNullableReturnedEffort319(t *testing.T) {
	tr := NewTranslatorFor(decodeRequest(t, minimalRequest), "gpt-6-luna")
	if _, err := tr.Accept(textDelta("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(textDone("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Accept(event(codex.Completed, `{"type":"response.completed","response":{"id":"r","reasoning":{"effort":null},`+usageReported+`}}`)); err != nil {
		t.Fatal(err)
	}
}
