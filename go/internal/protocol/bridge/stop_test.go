package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// The request's stop_sequences reach the translator: text past the sequence never streams
// and the reply says why it ended (#149).
func TestStopSequencesReachTheTranslator(t *testing.T) {
	request := decodeRequest(t, `{"model":"gpt-6-astra","max_tokens":100000,"stream":true,`+
		`"messages":[{"role":"user","content":"ping"}],"stop_sequences":["</severity>"]}`)
	frames, err := runFor(t, request,
		event(codex.Created, `{"type":"response.created","response":{"id":"resp_1"}}`),
		itemAdded(0, `{"id":"msg_1","type":"message"}`),
		textDelta("<severity>7</severity>tail"),
		textDone("<severity>7</severity>tail"),
		itemDone(0, messageItem("msg_1", "<severity>7</severity>tail")),
		event(codex.Completed, completedOK))
	if err != nil {
		t.Fatal(err)
	}
	all := joined(frames)
	text, _ := json.Marshal("<severity>7")
	stop, _ := json.Marshal("</severity>")
	if strings.Contains(all, "tail") || !strings.Contains(all, `"text":`+string(text)) ||
		!strings.Contains(all, `"stop_reason":"stop_sequence","stop_sequence":`+string(stop)) {
		t.Fatalf("frames = %s", all)
	}
}

// A caller that turned thinking off limits the answer; the backend's reasoning is not part
// of it (#149: native's classifier asks for 64 tokens). Anything else still counts it all.
func TestThinkingOffLimitCountsTheAnswer(t *testing.T) {
	usage := func(details string) stream.Event {
		return event(codex.Completed, `{"type":"response.completed","response":{"id":"resp_1",`+
			`"usage":{"input_tokens":5,"output_tokens":5`+details+`}}}`)
	}
	reasoning := func(n string) string { return `,"output_tokens_details":{"reasoning_tokens":` + n + `}` }
	for _, tc := range []struct {
		thinking, details string
		refused           bool
	}{
		{`,"thinking":{"type":"disabled"}`, reasoning("4"), false},
		{`,"thinking":{"type":"disabled"}`, reasoning("2"), true},
		{`,"thinking":{"type":"disabled"}`, "", true},
		{`,"thinking":{"type":"disabled"}`, reasoning("6"), true},
		{`,"thinking":{"type":"adaptive"}`, reasoning("4"), true},
		{``, reasoning("4"), true},
	} {
		translator := NewTranslatorFor(decodeRequest(t, `{"model":"sonnet","max_tokens":2,"stream":true,`+
			`"messages":[{"role":"user","content":"x"}]`+tc.thinking+`}`), "")
		for _, e := range []stream.Event{textDelta("x"), textDone("x")} {
			if _, err := translator.Accept(e); err != nil {
				t.Fatal(err)
			}
		}
		_, err := translator.Accept(usage(tc.details))
		if refused := errors.Is(err, ErrOutputLimitExceeded); refused != tc.refused || err != nil && !refused {
			t.Fatalf("thinking %q details %q: err = %v", tc.thinking, tc.details, err)
		}
	}
}
