// Package bridge converts between the two wire formats. It is the only package that
// imports both, which is what keeps a field from crossing without someone deciding it
// should.
//
// WP03 carries the text path. Tool use, images, structured output and reasoning content
// are refused by the request decoder or dropped from the client's view here with a note
// saying so — never forwarded half-understood.
package bridge

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// ErrUnsupportedEvent means the backend sent an event type this build does not understand.
// It is refused rather than skipped: the unread event may be the one carrying the result.
var ErrUnsupportedEvent = errors.New("UNSUPPORTED_EVENT")

// How an unsupported event's name was judged. Only the first of these carries a name.
//
// The Node baseline has six formats and three of them -- missing, non-string, empty -- are
// impossible here: the SSE parser refuses a frame without a usable type before it ever
// reaches this, with its own error, so a reader still learns. Naming a category that cannot
// occur would put a zero in the account forever.
const (
	// EventNamed means the name was safe to record and is in Name.
	EventNamed = "identifier"
	// EventOversized means the type was too long to be a name.
	EventOversized = "oversized"
	// EventOther means a non-empty string that is not shaped like a name.
	EventOther = "other"
)

// eventNameShape is what may be written down.
//
// Lowercase segments, at most five of them, each bounded. The backend chooses this string
// and it reaches a file that outlives the session, so what leaves here is either a name of
// this shape or a fixed label.
//
// The dot is optional, and that is the baseline's own recorded mistake: requiring one lost
// the names this protocol actually uses without dots -- error, ping, message_start -- so a
// run classified the type as an identifier and left the list empty, missing the one name
// the capture exists for.
var eventNameShape = regexp.MustCompile(`^[a-z0-9_]{1,24}(\.[a-z0-9_]{1,24}){0,4}$`)

// eventNameMax bounds the whole name whatever its segments say.
const eventNameMax = 48

// UnsupportedEvent is the refusal, carrying what can safely be said about the event.
//
// The whole point of the type: without it a session that meets a new backend event fails
// every turn and says only UNSUPPORTED_EVENT, and the fix is one constant that nobody can
// name. This has already happened once -- see the note on the function-call arguments
// events in package codex.
type UnsupportedEvent struct {
	// Name is the event type, when it was safe to record. Empty otherwise.
	Name string
	// Format says how the type was judged, and is one of the three above.
	Format string
}

func (e *UnsupportedEvent) Error() string { return "UNSUPPORTED_EVENT" }

// Unwrap keeps errors.Is(err, ErrUnsupportedEvent) answering as it did.
func (e *UnsupportedEvent) Unwrap() error { return ErrUnsupportedEvent }

// unsupportedEvent judges an event type and refuses it.
func unsupportedEvent(eventType string) error {
	switch {
	case len(eventType) > eventNameMax:
		return &UnsupportedEvent{Format: EventOversized}
	case eventNameShape.MatchString(eventType):
		return &UnsupportedEvent{Name: eventType, Format: EventNamed}
	default:
		return &UnsupportedEvent{Format: EventOther}
	}
}

// Request is what the backend is asked for. It is assembled here from a decoded Anthropic
// request so that neither protocol package has to know the other's shape.
type Request struct {
	Model       string          `json:"model"`
	Instruction string          `json:"instructions,omitempty"`
	Input       []InputEntry    `json:"input"`
	Stream      bool            `json:"stream"`
	Effort      *ReasoningParam `json:"reasoning,omitempty"`
	// Source names the rule that resolved Model — catalogue, alias, family or direct. Not
	// part of the wire format: it is how the answer was reached, not part of the question.
	// It travels here so the ledger can record what a request was actually run on (CAP03).
	Source     string     `json:"-"`
	Tools      []ToolSpec `json:"tools,omitempty"`
	ToolChoice any        `json:"tool_choice,omitempty"`
	Parallel   *bool      `json:"parallel_tool_calls,omitempty"`
	Include    []string   `json:"include,omitempty"`
	// Store is always false and never omitted. Asking the backend not to retain the
	// conversation is a property of every request this bridge makes, so it is stated
	// rather than left to a default that could change on the other side.
	Store bool `json:"store"`
}

// Instruction is what every request sends as its top-level instructions.
//
// It is a fixed string, not the caller's system prompt. The system prompt is a developer
// turn inside the conversation, where per-turn effort and cache markers apply to it the
// same way they apply to any other turn. Hoisting it up here would move it out of the
// conversation the caller described.
const Instruction = "Follow the developer instructions in the conversation."

// Include asks for the reasoning the backend would otherwise keep to itself. It is
// requested because a multi-turn tool exchange needs it carried forward, not because it is
// shown to anyone: nothing downstream hands it to the client.
var Include = []string{"reasoning.encrypted_content"}

// MaxOutputTokens is deliberately absent from Request.
//
// The baseline never sends max_output_tokens, and the PoC recorded the backend rejecting
// it. The client's max_tokens is enforced instead at completion, against the usage the
// backend reports — see Translator.outputLimit. That is a check after the fact rather than
// a cap on generation, and the difference is recorded here rather than papered over.

// InputEntry is one element of the backend's input array. A conversation turn, a recorded
// call and a recorded result are three different shapes in the same list, so the members
// that do not apply are omitted rather than sent empty.
type InputEntry struct {
	Type string `json:"type,omitempty"`

	// message. Content is either []InputPart for a conversation turn or a plain string
	// for the developer turn carrying the system prompt — the two shapes the baseline
	// sends, kept distinct because the wire has never been verified to accept one for the
	// other.
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`

	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// function_call_output
	Output []InputPart `json:"output,omitempty"`

	// reasoning. Carried apart from the rest because its shape shares no field with them
	// and because an empty summary has to travel as an empty list rather than vanish.
	Reasoning *Reasoning `json:"-"`
}

// Reasoning is a chain of thought going back to the model that produced it.
type Reasoning struct {
	ID        string          `json:"id"`
	Summary   []ReasoningPart `json:"summary"`
	Encrypted string          `json:"encrypted_content"`
}

// ReasoningPart is one piece of the visible summary.
type ReasoningPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MarshalJSON writes a reasoning entry in its own shape and everything else unchanged.
//
// The summary is written even when empty. omitempty would drop it, and the backend is
// being handed back exactly what it produced -- an empty summary is a summary with nothing
// in it, which is not the same as no summary at all.
func (e InputEntry) MarshalJSON() ([]byte, error) {
	if e.Reasoning != nil {
		summary := e.Reasoning.Summary
		if summary == nil {
			summary = []ReasoningPart{}
		}
		return json.Marshal(struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Summary   []ReasoningPart `json:"summary"`
			Encrypted string          `json:"encrypted_content"`
		}{"reasoning", e.Reasoning.ID, summary, e.Reasoning.Encrypted})
	}
	// The alias stops this from calling itself and keeps the existing shape exactly.
	type plain InputEntry
	return json.Marshal(plain(e))
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
	// ImageURL carries a data URL for an input_image part and is empty otherwise.
	ImageURL string `json:"-"`
}

// MarshalJSON writes the shape each part type actually has.
//
// An input_image carries an image_url and no text; an input_text carries a text and no
// image_url. A single struct with omitempty would get both wrong -- it would put an empty
// text beside every image, and it would drop the text field from a legitimately empty text
// part. The two shapes are written out rather than approximated.
func (p InputPart) MarshalJSON() ([]byte, error) {
	if p.Type == "input_image" {
		return json.Marshal(struct {
			Type     string `json:"type"`
			ImageURL string `json:"image_url"`
		}{p.Type, p.ImageURL})
	}
	return json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{p.Type, p.Text})
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
func BuildRequest(request *anthropic.Request, override ...Route) (*Request, error) {
	// The client asks for a Claude model; the backend has never heard of one. Resolved
	// here rather than forwarded, and refused rather than defaulted -- see route.go.
	route, err := SelectRoute(request.Model, request.Effort)
	if err != nil {
		return nil, err
	}
	// An override replaces that entirely and carries its own source, so a reader of the
	// record can tell a route the client chose from one this build reassigned.
	if len(override) > 0 {
		route = override[0]
	}
	// Last, because it is a cost guard rather than a choice of where to run. A compaction
	// keeps whichever model it was going to use and drops to medium if it was dearer; see
	// compact.go for why that is the one request worth capping.
	if route.Effort != "low" && route.Effort != compactEffort && IsCompactTemplate(textOf(request)) {
		route.Effort, route.Source = compactEffort, "compact"
	}

	out := &Request{
		Model:       route.Model,
		Instruction: Instruction,
		Stream:      true,
		Include:     Include,
		Store:       false,
		// Always sent. The catalogue supplies an effort when the request does not name
		// one, so there is no case where the backend is left to pick, and the baseline
		// sends it unconditionally for the same reason.
		Effort: &ReasoningParam{Effort: route.Effort},
		Source: route.Source,
	}
	// The system prompt leads the conversation as a developer turn. Its content is a plain
	// string here rather than a list of parts, which is the shape the baseline sends.
	if text, ok := systemText(request.System); ok {
		out.Input = append(out.Input, InputEntry{Role: "developer", Content: text})
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
		var parts []InputPart
		flush := func() {
			if len(parts) > 0 {
				out.Input = append(out.Input, InputEntry{Role: role, Content: parts})
			}
			parts = nil
		}

		for _, block := range message.Blocks {
			switch block.Type {
			case "text":
				parts = append(parts, InputPart{Type: kind, Text: block.Text})
			case "image":
				// Its own entry, which is the baseline's shape: an attached picture is not
				// a part of the sentence around it.
				flush()
				out.Input = append(out.Input, InputEntry{Role: "user", Content: []InputPart{{
					Type:     "input_image",
					ImageURL: anthropic.ImageURL(block.MediaType, block.Data),
				}}})
			case "redacted_thinking":
				// Its own entry, handed straight back. The content is encrypted and this
				// build has never read it; what it does is not lose it.
				flush()
				summary := make([]ReasoningPart, 0, len(block.Reasoning.Summary))
				for _, part := range block.Reasoning.Summary {
					summary = append(summary, ReasoningPart{Type: part.Type, Text: part.Text})
				}
				out.Input = append(out.Input, InputEntry{Reasoning: &Reasoning{
					ID:        block.Reasoning.ID,
					Summary:   summary,
					Encrypted: block.Reasoning.Encrypted,
				}})
			case "tool_addition", "tool_removal":
				// Nothing travels. A tool change edits which definitions go upstream, which
				// ActiveTools has already applied; putting the block itself in the input
				// would be describing the edit to the model as though it were content.
			case "tool_use":
				flush()
				out.Input = append(out.Input, InputEntry{
					Type:      "function_call",
					CallID:    block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
				})
			case "tool_result":
				flush()
				out.Input = append(out.Input, InputEntry{
					Type:   "function_call_output",
					CallID: block.ToolUseID,
					Output: resultParts(block),
				})
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
		case "image":
			parts = append(parts, InputPart{Type: "input_image",
				ImageURL: anthropic.ImageURL(part.MediaType, part.Data)})
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
	// outputLimit is the caller's max_tokens. Nothing asks the backend to stop at it, so
	// it is checked here against what the backend says it spent.
	outputLimit int64
	// streamedArgs accumulates tool call arguments as they stream, keyed by the item they
	// belong to. Nothing is built from them; they exist to be checked against the snapshot
	// the backend sends when it finishes writing them.
	streamedArgs map[string]string
	// held is the response being assembled, by output index, and order is the sequence the
	// backend declared them in.
	//
	// This is the delivery barrier. The items exist here from the moment the backend opens
	// them, and nothing in this map reaches the client until response.completed arrives --
	// so a stream that fails midway has still handed the client nothing to execute. The
	// barrier is when they are released, not where they come from.
	held  map[int]*heldItem
	order []int
	ids   map[string]bool
}

// heldItem is one output item and whether the backend has finished writing it.
type heldItem struct {
	item codex.OutputItem
	done bool
}

// maxOutputItems bounds what one response may open before it completes.
//
// The Builder bounds what is released, but items are held before they reach it, so without
// this a backend could accumulate memory here without ever completing. Chosen to be far
// above any plausible response and far below anything that matters.
const maxOutputItems = 1024

// NewTranslatorFor builds a translator for one request: which tools a new call may name,
// and the output limit that request asked for.
func NewTranslatorFor(request *anthropic.Request) *Translator {
	t := &Translator{builder: anthropic.NewBuilder(request.Model), outputLimit: request.MaxTokens}
	callable := request.CallableNames()
	t.builder.SetCallable(func(name string) bool { return callable[name] })
	return t
}

// ErrOutputLimitExceeded means the backend generated more than the caller allowed.
//
// The caller's max_tokens never reached the backend — it is not a parameter this wire
// accepts — so it cannot have been a cap on generation. Enforcing it here means an
// oversized response is refused after the fact rather than truncated during it, which is
// the baseline's behaviour and the only one available.
var ErrOutputLimitExceeded = errors.New("OUTPUT_TOKEN_LIMIT_EXCEEDED")

// ErrUsageUnknown means the backend finished without saying what it spent. With no count
// there is nothing to check the caller's limit against, and reporting success would be
// claiming a check that never ran.
var ErrUsageUnknown = errors.New("INVALID_USAGE")

// ErrArgumentsMismatch means the backend's finished tool arguments disagree with what it
// streamed. The client executes what a tool call says, so the two accounts disagreeing is
// where this stops rather than picking one.
var ErrArgumentsMismatch = errors.New("ARGUMENTS_MISMATCH")

// ErrOutputItemOrder means the backend's items did not arrive as a dense ascending sequence
// that closes before the response does. A response missing an item it declared is short by
// exactly the part nobody read.
var ErrOutputItemOrder = errors.New("INVALID_OUTPUT_ITEM")

// ErrItemSnapshotMismatch means two accounts of the same item disagree.
var ErrItemSnapshotMismatch = errors.New("SNAPSHOT_MISMATCH")

func (t *Translator) checkOutputLimit(usage codex.Usage) error {
	if t.outputLimit <= 0 {
		return nil
	}
	if !usage.OutputKnown {
		return ErrUsageUnknown
	}
	if usage.OutputTokens > t.outputLimit {
		return ErrOutputLimitExceeded
	}
	return nil
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

	case codex.OutputItemAdd:
		event, err := codex.DecodeOutputItem(event.Raw, false)
		if err != nil {
			return nil, err
		}
		return nil, t.openItem(event)

	case codex.OutputItemDone:
		event, err := codex.DecodeOutputItem(event.Raw, true)
		if err != nil {
			return nil, err
		}
		return nil, t.closeItem(event)

	case codex.FuncArgsDelta:
		delta, err := codex.DecodeArgumentsDelta(event.Raw)
		if err != nil {
			return nil, err
		}
		if t.streamedArgs == nil {
			t.streamedArgs = map[string]string{}
		}
		t.streamedArgs[delta.ItemID] += delta.Delta
		return nil, nil

	case codex.FuncArgsDone:
		done, err := codex.DecodeArgumentsDone(event.Raw)
		if err != nil {
			return nil, err
		}
		// The backend's own account of what it wrote, against what arrived. A tool call is
		// executed by the client, so a stream nobody can account for is not one to build a
		// call from -- even though the call itself comes from the completed output.
		if t.streamedArgs[done.ItemID] != done.Arguments {
			return nil, ErrArgumentsMismatch
		}
		return nil, nil

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
		// The completion is what releases the held items. Until this arrives nothing
		// assembled above has reached the client, which is the delivery barrier: a stream
		// that failed midway cannot have handed the client something to execute.
		//
		// The payload's own output array is checked rather than read. It is empty on this
		// backend -- measured, on every response -- so reading from it read nothing; a
		// backend that starts filling it and contradicts its own stream is worth stopping
		// for.
		if err := t.crossCheckCompleted(event.Raw); err != nil {
			return nil, err
		}
		if err := t.release(); err != nil {
			return nil, err
		}
		// Checked here, after the response itself has been read. A malformed call or an
		// empty reply is a defect in what arrived; the limit is a policy question about a
		// response that was otherwise fine, and answering the policy question first would
		// report a limit breach for a response that was never usable.
		if err := t.checkOutputLimit(usage); err != nil {
			return nil, err
		}
		return t.builder.Complete(anthropic.Usage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			InputKnown:   usage.InputKnown,
			OutputKnown:  usage.OutputKnown,
		})

	case codex.InProgress, codex.Queued,
		codex.ContentPartAdd, codex.ContentPartDon,
		codex.Keepalive, codex.Ping,
		codex.RateLimitsUpdated, codex.CodexRateLimits,
		codex.CodexMetadata, codex.WebsocketTiming:
		// Structural or keepalive events. They carry no client-visible content, and a
		// keepalive in particular is not progress — reading one as progress is how a
		// stalled upstream keeps a request alive forever.
		return nil, nil

	case codex.ReasoningPartAdd, codex.ReasoningPartDone,
		codex.ReasoningSummaryAdd, codex.ReasoningSummaryDone,
		codex.ReasoningSummaryTxtD, codex.ReasoningSummaryTxtF,
		codex.ReasoningTextDelta, codex.ReasoningTextDone:
		// Known, and deliberately invisible. Reasoning arriving before text must not
		// disturb the order of what the client does see, which is why these produce no
		// frames rather than being treated as content of an unknown kind.
		return nil, nil
	}

	return nil, unsupportedEvent(event.Type)
}

// openItem records an item the backend has started writing.
//
// Indices are the backend's own numbering and must be dense and ascending: a gap means an
// item was opened that nobody saw, and a response missing an item it declared is one short
// by exactly the part nobody read.
func (t *Translator) openItem(event codex.OutputItemEvent) error {
	if t.held == nil {
		t.held = map[int]*heldItem{}
		t.ids = map[string]bool{}
	}
	if event.Index != len(t.order) {
		return ErrOutputItemOrder
	}
	if len(t.order) >= maxOutputItems {
		return anthropic.ErrResponseTooLarge
	}
	// One id, one item. A repeated id would let a later snapshot be checked against the
	// wrong item, and the arguments stream is keyed by it.
	if event.Item.ID != "" && t.ids[event.Item.ID] {
		return ErrOutputItemOrder
	}
	t.ids[event.Item.ID] = true
	t.held[event.Index] = &heldItem{item: event.Item}
	t.order = append(t.order, event.Index)
	return nil
}

// closeItem takes the backend's final account of an item and checks it against the first.
//
// The final snapshot is authoritative -- it is what the response actually contains -- but it
// has to be the same item. An id or a kind changing between the two accounts means one of
// them is about something else.
func (t *Translator) closeItem(event codex.OutputItemEvent) error {
	held, open := t.held[event.Index]
	if !open || held.done {
		return ErrOutputItemOrder
	}
	final := event.Item
	if final.ID != held.item.ID || final.Type != held.item.Type {
		return ErrItemSnapshotMismatch
	}
	if final.Type == codex.ItemFunctionCall {
		// Only what the opening snapshot actually named. It is entitled to have carried an
		// identity and nothing else; it is not entitled to have named something different.
		if held.item.CallID != "" && final.CallID != held.item.CallID {
			return ErrItemSnapshotMismatch
		}
		if held.item.Name != "" && final.Name != held.item.Name {
			return ErrItemSnapshotMismatch
		}
		// Against what actually streamed, when anything did. An item whose arguments never
		// streamed is not a mismatch: the backend is entitled to deliver them whole.
		if streamed, saw := t.streamedArgs[final.ID]; saw && streamed != string(final.Arguments) {
			return ErrArgumentsMismatch
		}
	}
	held.item = final
	held.done = true
	return nil
}

// release hands the assembled response to the builder. It runs once, on completion.
func (t *Translator) release() error {
	for _, index := range t.order {
		held := t.held[index]
		if !held.done {
			// The backend said the response finished while an item was still open. Taking
			// the partial one would deliver something it never said it had written.
			return ErrOutputItemOrder
		}
		switch held.item.Type {
		case codex.ItemFunctionCall:
			if err := t.builder.AddToolCall(held.item.CallID, held.item.Name, held.item.Arguments); err != nil {
				return err
			}
		case codex.ItemMessage:
			// The backend's own account of what it said, checked against what was
			// streamed. A disagreement means a delta went missing.
			if err := t.checkStreamedText(held.item.Text); err != nil {
				return err
			}
		case codex.ItemReasoning:
			if err := t.recordThought(held.item); err != nil {
				return err
			}
		}
	}
	return nil
}

// recordThought turns a reasoning item into the block that carries it back next turn.
//
// This request asked for reasoning.encrypted_content. What comes back is opaque and is
// never read here; what happens to it is that it is kept, in an envelope the next turn's
// decoder can open, so the model does not begin its thinking again every turn.
//
// A reasoning item with no encrypted content is only acceptable when it carried nothing
// else either. The alternative would be dropping a thought the model did produce and
// letting the next turn proceed as though it had not -- a quieter failure than refusing,
// and a worse one, because the answer it leads to looks ordinary.
func (t *Translator) recordThought(item codex.OutputItem) error {
	if !item.HasEncrypted {
		if item.HasContent || len(item.Summary) > 0 {
			return ErrMissingEncryptedReasoning
		}
		return nil
	}

	summary := make([]ReasoningPart, 0, len(item.Summary))
	for _, part := range item.Summary {
		summary = append(summary, ReasoningPart{Type: part.Type, Text: part.Text})
	}
	saved, err := json.Marshal(struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Summary   []ReasoningPart `json:"summary"`
		Encrypted string          `json:"encrypted_content"`
	}{"reasoning", item.ID, summary, item.Encrypted})
	if err != nil {
		return ErrMissingEncryptedReasoning
	}
	data := anthropic.ReasoningPrefix + base64.RawURLEncoding.EncodeToString(saved)

	// Read back what was just written. The envelope is only worth anything if the decoder
	// on the other side of the next turn can open it, and the two halves are far enough
	// apart -- a whole session apart -- that agreeing by inspection is not agreeing.
	if _, err := anthropic.DecodeRecordedThought(data); err != nil {
		return err
	}
	t.builder.AddThought(data)
	return nil
}

// crossCheckCompleted compares the completion's output array with what was assembled.
//
// The array is empty on this backend, so this normally checks nothing. It exists because
// tool calls are executed by the client: if the backend ever does state its output here and
// states it differently, that is the moment to stop rather than pick one account.
func (t *Translator) crossCheckCompleted(raw []byte) error {
	items, present, err := codex.DecodeCompletedOutput(raw)
	if err != nil {
		return err
	}
	if !present || len(items) == 0 {
		return nil
	}
	if len(items) != len(t.order) {
		return ErrItemSnapshotMismatch
	}
	for i, stated := range items {
		held := t.held[t.order[i]].item
		if stated.Type != held.Type || stated.ID != held.ID ||
			stated.CallID != held.CallID || stated.Name != held.Name ||
			string(stated.Arguments) != string(held.Arguments) {
			return ErrItemSnapshotMismatch
		}
	}
	return nil
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
