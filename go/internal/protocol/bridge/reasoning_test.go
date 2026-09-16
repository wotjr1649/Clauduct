package bridge

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// recorded builds the envelope this bridge writes into a transcript.
func recorded(saved string) string {
	return anthropic.ReasoningPrefix + base64.RawURLEncoding.EncodeToString([]byte(saved))
}

const savedReasoning = `{"type":"reasoning","id":"rs_abc123",
  "summary":[{"type":"summary_text","text":"weighed two options"}],
  "encrypted_content":"AAAAencryptedBBBB"}`

func thinkingRequest(t *testing.T, data string) (*Request, error) {
	t.Helper()
	request, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":64,
	  "stream":true,"messages":[
	    {"role":"user","content":"first"},
	    {"role":"assistant","content":[{"type":"redacted_thinking","data":"` + data + `"}]},
	    {"role":"user","content":"second"}]}`))
	if err != nil {
		return nil, err
	}
	return BuildRequest(request)
}

// A4. A chain of thought this bridge recorded goes back to the model that produced it.
//
// The request asks the backend for reasoning.encrypted_content. Dropping what comes back
// means asking for something and discarding it, and leaves the model starting its thinking
// again every turn.
func TestRecordedReasoningGoesBackToTheModel(t *testing.T) {
	built, err := thinkingRequest(t, recorded(savedReasoning))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	encoded, err := json.Marshal(built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`{"type":"reasoning","id":"rs_abc123"`,
		`"summary":[{"type":"summary_text","text":"weighed two options"}]`,
		`"encrypted_content":"AAAAencryptedBBBB"`,
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("the reasoning did not travel back.\nwant %s\n got %s", want, encoded)
		}
	}
	// Its own entry, between the two turns, in the order the transcript recorded them.
	if len(built.Input) != 3 {
		t.Fatalf("Input has %d entries, want the two turns and the reasoning: %s",
			len(built.Input), encoded)
	}
	if built.Input[1].Reasoning == nil {
		t.Fatalf("the reasoning is not the middle entry: %s", encoded)
	}
}

// An empty summary travels as an empty list. Dropping the field would hand the backend a
// different shape than the one it produced.
func TestAnEmptySummaryTravelsAsAnEmptyList(t *testing.T) {
	built, err := thinkingRequest(t, recorded(
		`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"x"}`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	encoded, _ := json.Marshal(built)
	if !strings.Contains(string(encoded), `"summary":[]`) {
		t.Fatalf("an empty summary did not survive: %s", encoded)
	}
}

// And nothing else grew a reasoning shape on the way past.
func TestOrdinaryEntriesAreUnchangedByTheReasoningShape(t *testing.T) {
	built, err := thinkingRequest(t, recorded(savedReasoning))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	encoded, _ := json.Marshal(built)
	if strings.Contains(string(encoded), `"encrypted_content":""`) {
		t.Fatalf("an empty reasoning was written beside an ordinary turn: %s", encoded)
	}
	if !strings.Contains(string(encoded), `{"role":"user","content":[{"type":"input_text","text":"first"}]}`) {
		t.Fatalf("an ordinary turn changed shape: %s", encoded)
	}
}

// The envelope is this build's own, so a malformed one is a transcript that was truncated
// or tampered with rather than something to hand to the backend.
func TestAMalformedThoughtIsRefusedRatherThanForwarded(t *testing.T) {
	for _, c := range []struct{ name, data, code string }{
		{"no envelope prefix", "just-some-text", anthropic.CodeUnsupportedThinking},
		{"the prefix and nothing else", anthropic.ReasoningPrefix, anthropic.CodeReasoningFields},
		{"not base64", anthropic.ReasoningPrefix + "!!!not base64!!!", anthropic.CodeUnsupportedThinking},
		{"not JSON inside", recorded("not json"), anthropic.CodeReasoningFields},
		{"a key the envelope does not have", recorded(
			`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"x","extra":1}`),
			anthropic.CodeReasoningFields},
		{"not a reasoning record", recorded(
			`{"type":"message","id":"rs_1","summary":[],"encrypted_content":"x"}`),
			anthropic.CodeUnsupportedThinking},
		{"no encrypted content", recorded(
			`{"type":"reasoning","id":"rs_1","summary":[]}`), anthropic.CodeUnsupportedThinking},
		{"no summary at all", recorded(
			`{"type":"reasoning","id":"rs_1","encrypted_content":"x"}`), anthropic.CodeUnsupportedThinking},
		{"a summary part of another kind", recorded(
			`{"type":"reasoning","id":"rs_1","summary":[{"type":"text","text":"x"}],"encrypted_content":"y"}`),
			anthropic.CodeUnsupportedThinking},
		{"an identifier that is not one", recorded(
			`{"type":"reasoning","id":"has spaces","summary":[],"encrypted_content":"x"}`),
			anthropic.CodeUnsupportedThinking},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := thinkingRequest(t, c.data)
			var refusal *anthropic.RequestError
			if err == nil {
				t.Fatalf("a malformed thought was forwarded")
			}
			if !errors.As(err, &refusal) || refusal.Code != c.code {
				t.Fatalf("err = %v, want %s", err, c.code)
			}
		})
	}
}

// A chain of thought belongs to the turn that produced it.
func TestAThoughtOnAnyTurnButAnAssistantTurnIsRefused(t *testing.T) {
	for _, role := range []string{"user", "system"} {
		t.Run(role, func(t *testing.T) {
			_, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":64,
			  "stream":true,"messages":[{"role":"` + role + `","content":[
			    {"type":"redacted_thinking","data":"` + recorded(savedReasoning) + `"}]}]}`))
			var refusal *anthropic.RequestError
			if err == nil || !errors.As(err, &refusal) || refusal.Code != anthropic.CodeRedactedRole {
				t.Fatalf("err = %v, want %s", err, anthropic.CodeRedactedRole)
			}
		})
	}
}

// An unknown key on the block is refused for being that, not for the envelope inside it.
func TestAnUnknownKeyOnAThoughtBlockIsNamedAsSuch(t *testing.T) {
	_, err := anthropic.DecodeRequest([]byte(`{"model":"gpt-6-astra","max_tokens":64,
	  "stream":true,"messages":[{"role":"assistant","content":[
	    {"type":"redacted_thinking","data":"` + recorded(savedReasoning) + `","signature":"x"}]}]}`))
	var refusal *anthropic.RequestError
	if err == nil || !errors.As(err, &refusal) || refusal.Code != anthropic.CodeRedactedFields {
		t.Fatalf("err = %v, want %s", err, anthropic.CodeRedactedFields)
	}
}

// The envelope prefix is an interop contract, not a detail.
//
// A transcript recorded under the Node implementation is resumed under this one and the
// other way round. Every test above builds its envelope from the constant, so all of them
// stay green if the constant drifts -- and every recorded thought becomes unreadable at
// exactly the moment it is needed. The literal is asserted where that cannot happen.
func TestTheEnvelopePrefixIsTheOneTheBaselineWrites(t *testing.T) {
	if anthropic.ReasoningPrefix != "clauduct-reasoning-v1:" {
		t.Fatalf("ReasoningPrefix = %q. src/native-protocol.mjs writes "+
			"'clauduct-reasoning-v1:', and a transcript written by one implementation is "+
			"read by the other.", anthropic.ReasoningPrefix)
	}
}

// A null summary is not an empty one.
func TestANullSummaryIsRefusedRatherThanReadAsEmpty(t *testing.T) {
	_, err := thinkingRequest(t, recorded(
		`{"type":"reasoning","id":"rs_1","summary":null,"encrypted_content":"x"}`))
	var refusal *anthropic.RequestError
	if err == nil || !errors.As(err, &refusal) || refusal.Code != anthropic.CodeUnsupportedThinking {
		t.Fatalf("err = %v, want %s", err, anthropic.CodeUnsupportedThinking)
	}
}

// And a reasoning entry built without a summary still writes an empty list rather than a
// null, whoever built it.
func TestAReasoningEntryNeverWritesANullSummary(t *testing.T) {
	encoded, err := json.Marshal(InputEntry{Reasoning: &Reasoning{ID: "rs_1", Encrypted: "x"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"summary":[]`) {
		t.Fatalf("a nil summary was written as %s", encoded)
	}
}
