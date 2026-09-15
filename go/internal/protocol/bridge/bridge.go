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
	Input       []InputEntry    `json:"input"`
	MaxTokens   int64           `json:"max_output_tokens"`
	Stream      bool            `json:"stream"`
	Effort      *ReasoningParam `json:"reasoning,omitempty"`
	Tools       []ToolSpec      `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
	Parallel    *bool           `json:"parallel_tool_calls,omitempty"`
}

// InputEntry is one element of the backend's input array. A conversation turn, a recorded
// call and a recorded result are three different shapes in the same list, so the members
// that do not apply are omitted rather than sent empty.
type InputEntry struct {
	Type string `json:"type,omitempty"`

	// message
	Role    string      `json:"role,omitempty"`
	Content []InputPart `json:"content,omitempty"`

	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// function_call_output
	Output []InputPart `json:"output,omitempty"`
}

// ToolSpec is a callable definition in the backend's vocabulary.
type ToolSpec struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// NamedTool constrains the choice to one tool.
type NamedTool struct {
	Type string `json:"type"`
	Name string `json:"name"`
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

	// Only the definitions that are callable now go upstream. A deferred tool nobody has
	// mentioned costs schema on every request, which is what deferring is for.
	for _, tool := range request.ActiveTools() {
		out.Tools = append(out.Tools, ToolSpec{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}
	if choice := request.ToolChoice; choice.Present {
		switch choice.Type {
		case "any":
			// The two vocabularies disagree on the name for the same instruction.
			out.ToolChoice = "required"
		case "tool":
			out.ToolChoice = NamedTool{Type: "function", Name: choice.Name}
		default:
			out.ToolChoice = choice.Type
		}
		if choice.DisableParallelToolUse {
			parallel := false
			out.Parallel = &parallel
		}
	}

	for _, message := range request.Messages {
		// The backend calls a system turn a developer turn. Mapping it here rather than in
		// the decoder keeps each protocol package speaking only its own vocabulary.
		role := message.Role
		if role == "system" {
			role = "developer"
		}
		kind := "input_text"
		if message.Role == "assistant" {
			kind = "output_text"
		}

		// A turn's text is one message entry; each recorded call and result is its own
		// entry, in the order they appeared, so the backend sees the same sequence the
		// client recorded.
		var turn InputEntry
		flush := func() {
			if len(turn.Content) > 0 {
				out.Input = append(out.Input, turn)
			}
			turn = InputEntry{}
		}
		turn = InputEntry{Role: role}

		for _, block := range message.Blocks {
			switch block.Type {
			case "text":
				turn.Content = append(turn.Content, InputPart{Type: kind, Text: block.Text})
			case "tool_use":
				flush()
				out.Input = append(out.Input, InputEntry{
					Type:      "function_call",
					CallID:    block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
				})
				turn = InputEntry{Role: role}
			case "tool_result":
				flush()
				out.Input = append(out.Input, InputEntry{
					Type:   "function_call_output",
					CallID: block.ToolUseID,
					Output: resultParts(block),
				})
				turn = InputEntry{Role: role}
			default:
				return nil, fmt.Errorf("bridge: decoder passed a %q block to the text path", block.Type)
			}
		}
		flush()
	}
	return out, nil
}

// resultParts flattens a tool result for the backend.
//
// A failed result is announced rather than left to be inferred from its text: the client
// said the tool failed, and dropping that flag would present the failure as output.
func resultParts(block anthropic.Block) []InputPart {
	parts := make([]InputPart, 0, len(block.Result)+1)
	if block.IsError {
		parts = append(parts, InputPart{Type: "input_text", Text: "Tool execution failed:"})
	}
	for _, part := range block.Result {
		switch part.Type {
		case "tool_reference":
			// A historical reference is data, not a definition that can reactivate a tool.
			parts = append(parts, InputPart{Type: "input_text",
				Text: `{"type":"tool_reference","tool_name":"` + part.Name + `"}`})
		default:
			parts = append(parts, InputPart{Type: "input_text", Text: part.Text})
		}
	}
	return parts
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

// NewTranslatorFor builds a translator that knows which tools a new call may name.
func NewTranslatorFor(request *anthropic.Request) *Translator {
	t := &Translator{builder: anthropic.NewBuilder(request.Model)}
	callable := request.CallableNames()
	t.builder.SetCallable(func(name string) bool { return callable[name] })
	return t
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
		// The completed payload is where tool calls come from. Reading them here rather
		// than from the streaming argument events is the delivery barrier: a call exists
		// only once the backend has said the response finished, so a stream that failed
		// midway cannot have handed the client something to execute.
		items, present, err := codex.DecodeCompletedOutput(event.Raw)
		if err != nil {
			return nil, err
		}
		if present {
			for _, item := range items {
				switch item.Type {
				case codex.ItemFunctionCall:
					if err := t.builder.AddToolCall(item.CallID, item.Name, item.Arguments); err != nil {
						return nil, err
					}
				case codex.ItemMessage:
					// The backend's own account of what it said, checked against what was
					// streamed. A disagreement means a delta went missing.
					if err := t.checkStreamedText(item.Text); err != nil {
						return nil, err
					}
				}
			}
		}
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

// checkStreamedText compares the completed payload's message text with what the deltas
// built. It is the same property the per-part snapshot checks, one level up: if the two
// accounts of the answer differ, neither can be handed on.
func (t *Translator) checkStreamedText(parts []string) error {
	joined := ""
	for _, part := range parts {
		joined += part
	}
	if joined != t.builder.Text() {
		return anthropic.ErrTextMismatch
	}
	return nil
}

// Text reports what the response accumulated.
func (t *Translator) Text() string { return t.builder.Text() }

// ToolCallCount reports how many calls the completed response released.
func (t *Translator) ToolCallCount() int { return t.builder.ToolCallCount() }
