// Package bridge converts between the two wire formats. It is the only package that
// imports both, which is what keeps a field from crossing without someone deciding it
// should.
//
// WP03 carries the text path. Tool use, images, structured output and reasoning content
// are refused by the request decoder or dropped from the client's view here with a note
// saying so — never forwarded half-understood.
package bridge

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// ErrUnsupportedEvent means the backend sent an event type this build does not understand.
// It is refused rather than skipped: the unread event may be the one carrying the result.
var ErrUnsupportedEvent = errors.New("UNSUPPORTED_EVENT")

// Request is what the backend is asked for. It is assembled here from a decoded Anthropic
// request so that neither protocol package has to know the other's shape.
type Request struct {
	Model       string          `json:"model"`
	Instruction string          `json:"instructions,omitempty"`
	Input       []InputMessage  `json:"input"`
	MaxTokens   int64           `json:"max_output_tokens"`
	Stream      bool            `json:"stream"`
	Effort      *ReasoningParam `json:"reasoning,omitempty"`
}

// InputMessage is one turn in the backend's vocabulary.
type InputMessage struct {
	Role    string      `json:"role"`
	Content []InputPart `json:"content"`
}

// InputPart is one piece of a turn. The backend distinguishes input from output text.
type InputPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ReasoningParam carries the effort the caller asked for.
type ReasoningParam struct {
	Effort string `json:"effort"`
}

// BuildRequest converts a decoded Anthropic request into a backend request.
//
// It refuses nothing on its own: the decoder has already established that this request is
// within what the build supports, so anything arriving here is text. A block that is not
// text reaching this point would be a decoder defect, and it is reported as one rather
// than skipped.
func BuildRequest(request *anthropic.Request) (*Request, error) {
	out := &Request{
		Model:     request.Model,
		MaxTokens: request.MaxTokens,
		Stream:    true,
	}
	if request.Effort != "" {
		out.Effort = &ReasoningParam{Effort: request.Effort}
	}
	if text, ok := systemText(request.System); ok {
		out.Instruction = text
	}

	for _, message := range request.Messages {
		// The backend calls a system turn a developer turn. Mapping it here rather than in
		// the decoder keeps each protocol package speaking only its own vocabulary.
		role := message.Role
		if role == "system" {
			role = "developer"
		}
		converted := InputMessage{Role: role}
		kind := "input_text"
		if message.Role == "assistant" {
			kind = "output_text"
		}
		for _, block := range message.Blocks {
			if block.Type != "text" {
				return nil, fmt.Errorf("bridge: decoder passed a %q block to the text path", block.Type)
			}
			converted.Content = append(converted.Content, InputPart{Type: kind, Text: block.Text})
		}
		out.Input = append(out.Input, converted)
	}
	return out, nil
}

// systemText flattens the system prompt, which arrives either as a string or as an array
// of text blocks. A shape that is neither is left out rather than guessed at, and the
// caller is told nothing was read.
func systemText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	joined := ""
	for _, block := range blocks {
		if block.Type != "text" {
			continue
		}
		if joined != "" {
			joined += "\n\n"
		}
		joined += block.Text
	}
	return joined, joined != ""
}

// Translator drives an Anthropic response builder from backend events.
//
// It holds the whole mapping in one place so that adding an event type is a decision made
// here rather than a default that happens somewhere else.
type Translator struct {
	builder *anthropic.Builder
	usage   codex.Usage
}

func NewTranslator(model string) *Translator {
	return &Translator{builder: anthropic.NewBuilder(model)}
}

// Builder exposes the response under construction, for a caller that needs what was
// accumulated rather than the frames.
func (t *Translator) Builder() *anthropic.Builder { return t.builder }

// Accept translates one backend event into client frames.
//
// An event type outside the known set is an error, not a no-op. Reasoning events are known
// and produce nothing: the backend's private work is not carried to the client, and
// inventing a signature for it would be passing it off as an Anthropic feature.
func (t *Translator) Accept(event stream.Event) ([]anthropic.Frame, error) {
	if failure := codex.Failure(event.Type); failure != nil {
		return nil, failure
	}

	switch event.Type {
	case codex.Created:
		id, err := codex.DecodeResponseID(event.Raw)
		if err != nil {
			return nil, err
		}
		return nil, t.builder.SetResponseID(id)

	case codex.TextDelta:
		delta, err := codex.DecodeTextDelta(event.Raw)
		if err != nil {
			return nil, err
		}
		return t.builder.AppendText(delta.ContentIndex, delta.Delta)

	case codex.TextDone:
		done, err := codex.DecodeTextDone(event.Raw)
		if err != nil {
			return nil, err
		}
		return t.builder.FinishText(done.ContentIndex, done.Text)

	case codex.Completed:
		usage, err := codex.DecodeUsage(event.Raw)
		if err != nil {
			return nil, err
		}
		t.usage = usage
		return t.builder.Complete(anthropic.Usage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			InputKnown:   usage.InputKnown,
			OutputKnown:  usage.OutputKnown,
		})

	case codex.InProgress, codex.Queued,
		codex.OutputItemAdd, codex.OutputItemDone,
		codex.ContentPartAdd, codex.ContentPartDon,
		codex.Keepalive, codex.Ping:
		// Structural or keepalive events. They carry no client-visible content, and a
		// keepalive in particular is not progress — reading one as progress is how a
		// stalled upstream keeps a request alive forever.
		return nil, nil

	case codex.ReasoningPartAdd, codex.ReasoningPartDone,
		codex.ReasoningSummaryAdd, codex.ReasoningSummaryDone,
		codex.ReasoningSummaryTxtD, codex.ReasoningSummaryTxtF:
		// Known, and deliberately invisible. Reasoning arriving before text must not
		// disturb the order of what the client does see, which is why these produce no
		// frames rather than being treated as content of an unknown kind.
		return nil, nil
	}

	return nil, ErrUnsupportedEvent
}

// Text reports what the response accumulated.
func (t *Translator) Text() string { return t.builder.Text() }
