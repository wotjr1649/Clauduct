package bridge

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// A reasoning item as the backend delivers it.
func reasoningItem(id, encrypted, summary string) string {
	return `{"id":"` + id + `","type":"reasoning","summary":` + summary +
		`,"encrypted_content":"` + encrypted + `"}`
}

const oneSummary = `[{"type":"summary_text","text":"considered two paths"}]`

func TestAnswerSurvivesTrailingReasoningButExcludesToolTurns(t *testing.T) {
	for _, tools := range []bool{false, true} {
		tr := NewTranslatorFor(callable("Read"), "gpt-5.6-luna")
		item := reasoningItem("rs_1", "PUBLIC_SYNTHETIC_ENVELOPE", `[]`)
		for _, ev := range []stream.Event{itemAdded(0, `{"id":"msg_1","type":"message"}`), textDelta(0, "PUBLIC_REPORT"), textDone(0, "PUBLIC_REPORT"), itemDone(0, messageItem("msg_1", "PUBLIC_REPORT")), itemAdded(1, openingOf(item)), itemDone(1, item)} {
			if _, err := tr.Accept(ev); err != nil {
				t.Fatal(err)
			}
		}
		if tr.Answer() != "" {
			t.Fatal("incomplete answer accepted")
		}
		if tools {
			call := `{"id":"fc_1","type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}`
			for _, ev := range []stream.Event{itemAdded(2, openingOf(call)), itemDone(2, call)} {
				if _, err := tr.Accept(ev); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := tr.Accept(event(codex.Completed, completedOK)); err != nil {
			t.Fatal(err)
		}
		want := "PUBLIC_REPORT"
		if tools {
			want = ""
		}
		if tr.Answer() != want {
			t.Fatal("reasoning or tool turn substituted for completed answer")
		}
	}
}

// thoughtData pulls the envelope out of the emitted frames.
func thoughtData(t *testing.T, frames []anthropic.Frame) string {
	t.Helper()
	for _, frame := range frames {
		if frame.Type != "content_block_start" {
			continue
		}
		var body struct {
			ContentBlock struct {
				Type string `json:"type"`
				Data string `json:"data"`
			} `json:"content_block"`
		}
		if json.Unmarshal(frame.Data, &body) != nil {
			continue
		}
		if body.ContentBlock.Type == "redacted_thinking" {
			return body.ContentBlock.Data
		}
	}
	return ""
}

// A4. The chain of thought the backend produced reaches the transcript, so the next turn
// can hand it back.
//
// This request asks for reasoning.encrypted_content. Dropping what comes back is asking
// for something and discarding it, and leaves the model starting over every turn.
func TestAChainOfThoughtReachesTheTranscript(t *testing.T) {
	item := reasoningItem("rs_1", "ENCRYPTEDPAYLOAD", oneSummary)
	frames, err := run(t,
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "answer"), textDone(0, "answer"),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	data := thoughtData(t, frames)
	if data == "" {
		t.Fatalf("no thought reached the client:\n%s", joined(frames))
	}
	if !strings.HasPrefix(data, anthropic.ReasoningPrefix) {
		t.Fatalf("the envelope is not this bridge's: %q", data)
	}

	// And it decodes to exactly what the backend said, because the next turn sends this
	// back and the backend validates it.
	saved, err := anthropic.DecodeRecordedThought(data)
	if err != nil {
		t.Fatalf("this bridge wrote an envelope it cannot read: %v", err)
	}
	if saved.ID != "rs_1" || saved.Encrypted != "ENCRYPTEDPAYLOAD" {
		t.Fatalf("the record changed on the way through: %+v", saved)
	}
	if len(saved.Summary) != 1 || saved.Summary[0].Text != "considered two paths" {
		t.Fatalf("the summary did not survive: %+v", saved.Summary)
	}
}

// The order is the baseline's, because a transcript recorded by either implementation is
// read by both.
func TestAThoughtIsEmittedAfterTheTextAndBeforeTheTools(t *testing.T) {
	item := reasoningItem("rs_1", "E", `[]`)
	call := `{"id":"fc_1","type":"function_call","call_id":"call_1","name":"Read","arguments":"{}"}`
	frames, err := runFor(t, callable("Read"),
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "answer"), textDone(0, "answer"),
		itemAdded(1, openingOf(call)), itemDone(1, call),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	body := joined(frames)
	text := strings.Index(body, `"type":"text"`)
	thought := strings.Index(body, `"type":"redacted_thinking"`)
	tool := strings.Index(body, `"type":"tool_use"`)
	if text < 0 || thought < 0 || tool < 0 {
		t.Fatalf("a block is missing (text=%d thought=%d tool=%d):\n%s", text, thought, tool, body)
	}
	if !(text < thought && thought < tool) {
		t.Fatalf("order is text=%d thought=%d tool=%d, want text before thought before tool:\n%s",
			text, thought, tool, body)
	}
}

// Held until the response completes, like a tool call and for the same reason.
func TestAThoughtIsNotWrittenBeforeTheResponseCompletes(t *testing.T) {
	item := reasoningItem("rs_1", "E", `[]`)
	frames, err := runFor(t, decodeRequest(t, minimalRequest),
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "partial"))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if data := thoughtData(t, frames); data != "" {
		t.Fatalf("a thought was written before the response completed: %q", data)
	}
}

// A reasoning item whose record cannot be kept is refused, not dropped.
//
// Dropping it produces an answer that looks ordinary and a next turn that starts from less
// than it should -- a quieter failure than refusing, and a worse one.
func TestAThoughtThatCannotBeKeptIsRefused(t *testing.T) {
	for _, c := range []struct{ name, item string }{
		{"a summary with no encrypted content",
			`{"id":"rs_1","type":"reasoning","summary":` + oneSummary + `}`},
		{"content with no encrypted content",
			`{"id":"rs_1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"x"}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := run(t,
				itemAdded(0, openingOf(c.item)), itemDone(0, c.item),
				textDelta(0, "answer"), textDone(0, "answer"),
				event(codex.Completed, completedOK))
			if !errors.Is(err, ErrMissingEncryptedReasoning) {
				t.Fatalf("err = %v, want %v", err, ErrMissingEncryptedReasoning)
			}
		})
	}
}

// A reasoning item that carried nothing at all is not an error. The backend is entitled to
// announce one and put nothing in it.
func TestAnEmptyReasoningItemIsNotAnError(t *testing.T) {
	item := `{"id":"rs_1","type":"reasoning","summary":[]}`
	frames, err := run(t,
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "answer"), textDone(0, "answer"),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if data := thoughtData(t, frames); data != "" {
		t.Fatalf("an empty reasoning item produced a thought: %q", data)
	}
	if !strings.Contains(joined(frames), "answer") {
		t.Fatalf("the answer went missing:\n%s", joined(frames))
	}
}

// What goes out must come back. The two halves are a whole session apart, which is too far
// for agreeing by inspection.
func TestWhatIsWrittenCanBeReadBackByTheOtherHalf(t *testing.T) {
	item := reasoningItem("rs_round", "PAYLOAD", oneSummary)
	frames, err := run(t,
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "a"), textDone(0, "a"),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	data := thoughtData(t, frames)

	// Straight into the next turn's request, the way the client would send it.
	built, err := thinkingRequest(t, data)
	if err != nil {
		t.Fatalf("the next turn could not carry back what this one wrote: %v", err)
	}
	encoded, _ := json.Marshal(built)
	for _, want := range []string{`"id":"rs_round"`, `"encrypted_content":"PAYLOAD"`,
		`"text":"considered two paths"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("the round trip lost %s: %s", want, encoded)
		}
	}
}

// The envelope is base64url without padding, which is what the other implementation writes
// and therefore what a shared transcript contains.
func TestTheEnvelopeIsUnpaddedBase64URL(t *testing.T) {
	// Three payload lengths, because base64 pads only two residues out of three and a
	// single fixture can miss the padding it was written to catch.
	for _, encrypted := range []string{"E", "EE", "EEE"} {
		t.Run(encrypted, func(t *testing.T) {
			item := reasoningItem("rs_1", encrypted, `[]`)
			frames, err := run(t,
				itemAdded(0, openingOf(item)), itemDone(0, item),
				textDelta(0, "a"), textDone(0, "a"),
				event(codex.Completed, completedOK))
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}
			payload := strings.TrimPrefix(thoughtData(t, frames), anthropic.ReasoningPrefix)
			if strings.Contains(payload, "=") {
				t.Fatalf("the envelope is padded: %q", payload)
			}
			if _, err := base64.RawURLEncoding.DecodeString(payload); err != nil {
				t.Fatalf("the envelope is not base64url: %v", err)
			}
		})
	}
}

// This bridge does not write an envelope it cannot read.
//
// The writing and the reading are a whole session apart, so the only way to find a drift
// between them at the moment it happens is to read back what was just written. Here the id
// is not one the decoder accepts: without the read-back the envelope would be written
// anyway and refused next turn, when the thought it held is already unrecoverable.
func TestAnEnvelopeThisBridgeCannotReadIsNotWritten(t *testing.T) {
	item := reasoningItem("has spaces", "E", `[]`)
	_, err := run(t,
		itemAdded(0, openingOf(item)), itemDone(0, item),
		textDelta(0, "answer"), textDone(0, "answer"),
		event(codex.Completed, completedOK))
	if err == nil {
		t.Fatal("an envelope this bridge cannot read was written anyway")
	}
	var refusal *anthropic.RequestError
	if !errors.As(err, &refusal) || refusal.Code != anthropic.CodeUnsupportedThinking {
		t.Fatalf("err = %v, want %s", err, anthropic.CodeUnsupportedThinking)
	}
}

// A turn that produced only a thought produced no answer.
//
// Emitting a message whose single block is an opaque record would show the user an empty
// reply and tell the client the turn succeeded.
func TestAResponseWithOnlyAThoughtIsAnEmptyReply(t *testing.T) {
	item := reasoningItem("rs_1", "E", `[]`)
	_, err := run(t,
		itemAdded(0, openingOf(item)), itemDone(0, item),
		event(codex.Completed, completedOK))
	if !errors.Is(err, anthropic.ErrEmptyReply) {
		t.Fatalf("err = %v, want %v", err, anthropic.ErrEmptyReply)
	}
}
