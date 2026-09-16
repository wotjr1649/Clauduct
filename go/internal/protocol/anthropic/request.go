// Package anthropic decodes the requests the native client sends and emits the events it
// expects back. It is the Claude side of the wire and knows nothing about the backend.
//
// The validation here is an allowlist, not a filter. A field nobody recognises is refused
// rather than dropped, because dropping it would mean acting on a request while ignoring
// part of what it asked for — and the part ignored could be the part that made a tool call
// safe. Every refusal carries a fixed category so a failure is diagnosable without logging
// request content.
package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// The fields a request may carry, ported from the Node baseline. The measured claude
// 2.1.272 sends ten of them on every inference call.
var requestFields = []string{
	"model", "messages", "system", "max_tokens", "stream",
	"tools", "tool_choice", "thinking", "metadata",
	"output_config", "context_management",
	"temperature", "top_p", "stop_sequences",
}

// Fixed refusal categories, matching the baseline's names so a client sees the same
// vocabulary from either implementation.
const (
	CodeRequestFields        = "REQUEST_FIELDS"
	CodeRequestShape         = "REQUEST_SHAPE"
	CodeStreamFalse          = "REQUEST_STREAM_FALSE"
	CodeStreamMissing        = "REQUEST_STREAM_MISSING"
	CodeStreamInvalid        = "REQUEST_STREAM_INVALID"
	CodeMessagesInvalid      = "REQUEST_MESSAGES_INVALID"
	CodeMessagesEmpty        = "REQUEST_MESSAGES_EMPTY"
	CodeInvalidOutputLimit   = "INVALID_OUTPUT_LIMIT"
	CodeOutputConfigFields   = "OUTPUT_CONFIG_FIELDS"
	CodeOutputFormatShape    = "OUTPUT_FORMAT_SHAPE"
	CodeOutputFormatFields   = "OUTPUT_FORMAT_FIELDS"
	CodeOutputFormatType     = "OUTPUT_FORMAT_TYPE"
	CodeOutputFormatSchema   = "OUTPUT_FORMAT_SCHEMA"
	CodeOutputFormatName     = "OUTPUT_FORMAT_NAME"
	CodeThinkingFields       = "THINKING_FIELDS"
	CodeThinkingType         = "THINKING_TYPE"
	CodeThinkingBudget       = "THINKING_BUDGET"
	CodeContextFields        = "CONTEXT_FIELDS"
	CodeUnsupportedEdit      = "UNSUPPORTED_CONTEXT_EDIT"
	CodeUnsupportedSample    = "UNSUPPORTED_SAMPLING"
	CodeToolsShape           = "TOOLS_SHAPE"
	CodeMessageFields        = "MESSAGE_FIELDS"
	CodeUnsupportedMessage   = "UNSUPPORTED_MESSAGES"
	CodeMessageEffortRole    = "MESSAGE_EFFORT_ROLE"
	CodeTextFields           = "TEXT_FIELDS"
	CodeTextValue            = "TEXT_VALUE"
	CodeCacheFields          = "CACHE_FIELDS"
	CodeCacheValue           = "CACHE_VALUE"
	CodeUnsupportedContent   = "UNSUPPORTED_CONTENT"
	CodeImageFields          = "IMAGE_FIELDS"
	CodeImageSourceFields    = "IMAGE_SOURCE_FIELDS"
	CodeUnsupportedImage     = "UNSUPPORTED_IMAGE"
	CodeImageRole            = "IMAGE_ROLE"
	CodeInvalidModel         = "INVALID_MODEL"
	CodeToolUseFieldsCode    = "TOOL_USE_FIELDS"
	CodeToolResultFieldsCode = "TOOL_RESULT_FIELDS"

	// Not a defect in the request: a shape this build has not implemented yet. Named
	// separately so "you sent something wrong" and "we have not built that" never read as
	// the same answer, and so a capability gap can never pass as a silent success.
	CodeToolUseUnsupported = "TOOL_USE_UNSUPPORTED"
)

// RequestError is a refusal with a fixed category.
type RequestError struct {
	Code string
	// Field names the offending member when one is identifiable. It is a name this
	// package chose or a key from the allowlist, never a value from the request.
	Field string
}

func (e *RequestError) Error() string {
	if e.Field != "" {
		return e.Code + " " + e.Field
	}
	return e.Code
}

func refuse(code, field string) error { return &RequestError{Code: code, Field: field} }

// identifier matches the baseline's id() shape, used for names the backend will echo.
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// Block is one piece of message content. Raw keeps the bytes so nothing is lost for the
// blocks this build does not yet interpret.
//
// Input is the bytes of a tool call's arguments, never a decoded map. Re-encoding one would
// collapse {} , {"isolation":null} and {"isolation":"worktree"} into fewer readings than
// the caller wrote, and a "default tidy-up" is how a deliberate choice disappears.
type Block struct {
	Type string
	Text string
	Raw  json.RawMessage

	// tool_use
	ID    string
	Name  string
	Input json.RawMessage

	// tool_result
	ToolUseID string
	IsError   bool
	Result    []ResultPart

	// image
	MediaType string
	Data      string
}

// ImageURL is the data URL the backend reads an image from.
//
// The backend takes an image as a data URL rather than as a source object, so the
// media type and the payload are joined here and nowhere else. The shape is the
// baseline's, byte for byte.
func ImageURL(mediaType, data string) string {
	return "data:" + mediaType + ";base64," + data
}

// Message is one turn.
//
// Effort is the per-turn override the client sends on a system turn. It is carried rather
// than flattened away because a turn that asked for a different effort asked for it.
type Message struct {
	Role   string
	Effort string
	Blocks []Block
}

// Request is a decoded inference request. Unparsed members stay in Fields so a later
// package can read them without this one having to guess what they mean.
type Request struct {
	Model     string
	MaxTokens int64
	Messages  []Message
	System    json.RawMessage
	Effort    string
	Fields    map[string]json.RawMessage

	// Tools are the definitions as sent. Discovered additionally holds every name the
	// conversation has already used, which is what lets a transcript recorded under a
	// different tool set decode without making those tools callable again.
	Tools      []Tool
	ToolChoice ToolChoice
	Discovered map[string]bool
	// Removed holds the names a mid-conversation tool_removal withdrew. A withdrawn tool
	// is not sent upstream, so the backend cannot call something the client has just said
	// it will not run.
	Removed map[string]bool

	// HostedSearch is the server-side search tool, when the request carried one.
	//
	// Recorded rather than refused here, and kept out of Tools: it is not a definition the
	// client can be told to call, it is a request for this gateway to perform a search
	// itself. Whether that is what the request actually is takes more than the tool's
	// presence to decide, so the decision belongs to the caller that can see the whole
	// shape. Nothing puts this in the tool list sent upstream.
	HostedSearch *HostedSearch
}

// HostedSearch is a server-side search tool definition.
type HostedSearch struct {
	Type string
	Name string
	// Allowed and Blocked are the domain filters, already bounded. Nil means the request
	// named none, which is different from naming an empty list.
	Allowed []string
	Blocked []string
}

// ToolCount reports how many definitions were supplied.
func (r *Request) ToolCount() int { return len(r.Tools) }

// maxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER. The baseline validates against
// it, and a client that round-trips a larger value through a JSON number cannot be relied
// on to have sent what it meant.
const maxSafeInteger = int64(1)<<53 - 1

// DecodeRequest validates an inference request and returns what this build understands.
//
// It refuses rather than repairs. A stream flag that is missing, a sampling parameter this
// bridge cannot honour, an unknown top-level field: each gets its own category, because
// "the request was malformed" and "we do not support that" lead a user to different
// actions.
// Options are the facts about a request that do not live in its body.
//
// A struct rather than a parameter so that adding the next one does not touch every caller
// again, and variadic so that the callers who have nothing to say stay unchanged.
type Options struct {
	// ToolChanges is whether the request carried the mid-conversation tool changes beta.
	// Without it a tool_addition or tool_removal block is a request for a capability the
	// caller did not negotiate, which is refused rather than honoured quietly.
	ToolChanges bool
}

func DecodeRequest(body []byte, options ...Options) (*Request, error) {
	var settings Options
	if len(options) > 0 {
		settings = options[0]
	}
	fields, err := wire.Fields(body, requestFields)
	if err != nil {
		return nil, translateFieldError(err)
	}

	request := &Request{Fields: fields}

	// stream. Three distinct answers, because a client that omitted it and one that asked
	// for a non-streaming response have different problems.
	switch value, presence := wire.Of(fields, "stream"); {
	case presence == wire.Absent:
		return nil, refuse(CodeStreamMissing, "stream")
	case string(value) == "true":
		// the only accepted form
	case string(value) == "false":
		return nil, refuse(CodeStreamFalse, "stream")
	default:
		return nil, refuse(CodeStreamInvalid, "stream")
	}

	// Sampling controls the backend does not honour. Accepting and ignoring them would
	// hand back output that silently disobeyed the request.
	for _, name := range []string{"temperature", "top_p", "stop_sequences"} {
		if _, presence := wire.Of(fields, name); presence != wire.Absent {
			return nil, refuse(CodeUnsupportedSample, name)
		}
	}

	if err := decodeModel(fields, request); err != nil {
		return nil, err
	}
	if err := decodeMaxTokens(fields, request); err != nil {
		return nil, err
	}
	if err := decodeTools(fields, request); err != nil {
		return nil, err
	}
	if err := decodeToolChoice(fields, request); err != nil {
		return nil, err
	}
	if err := decodeMessages(fields, request, settings); err != nil {
		return nil, err
	}
	if request.ToolChoice.Present && request.ToolChoice.Type == "tool" {
		// Naming a tool that is not callable asks for something that cannot happen.
		if !request.CallableNames()[request.ToolChoice.Name] {
			return nil, refuse(CodeUnsupportedTools, "tool_choice")
		}
	}
	if err := decodeOutputConfig(fields, request); err != nil {
		return nil, err
	}
	if err := decodeThinking(fields); err != nil {
		return nil, err
	}
	if err := decodeContextManagement(fields); err != nil {
		return nil, err
	}
	if value, presence := wire.Of(fields, "system"); presence == wire.Present {
		request.System = value
	}
	return request, nil
}

func translateFieldError(err error) error {
	var fieldErr *wire.FieldError
	if errors.As(err, &fieldErr) {
		return refuse(CodeRequestFields, fieldErr.Field)
	}
	return refuse(CodeRequestShape, "")
}

func decodeModel(fields map[string]json.RawMessage, request *Request) error {
	value, presence := wire.Of(fields, "model")
	if presence != wire.Present {
		return refuse(CodeInvalidModel, "model")
	}
	if err := json.Unmarshal(value, &request.Model); err != nil || request.Model == "" {
		return refuse(CodeInvalidModel, "model")
	}
	return nil
}

func decodeMaxTokens(fields map[string]json.RawMessage, request *Request) error {
	value, presence := wire.Of(fields, "max_tokens")
	if presence != wire.Present {
		return refuse(CodeInvalidOutputLimit, "max_tokens")
	}
	number, err := exactInteger(value)
	if err != nil || number <= 0 || number > maxSafeInteger {
		return refuse(CodeInvalidOutputLimit, "max_tokens")
	}
	request.MaxTokens = number
	return nil
}

func decodeMessages(fields map[string]json.RawMessage, request *Request, settings Options) error {
	value, presence := wire.Of(fields, "messages")
	if presence != wire.Present {
		return refuse(CodeMessagesInvalid, "messages")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(value, &raw); err != nil {
		return refuse(CodeMessagesInvalid, "messages")
	}
	if len(raw) == 0 {
		return refuse(CodeMessagesEmpty, "messages")
	}
	state := newToolState()
	state.toolChanges = settings.ToolChanges
	// The baseline lets a change name only a tool this request already defines, which is
	// what keeps a removal from withdrawing something that was never there and an addition
	// from conjuring a definition out of a name.
	for _, tool := range request.Tools {
		state.defined[tool.Name] = true
	}
	for _, entry := range raw {
		message, err := decodeMessage(entry, state)
		if err != nil {
			return err
		}
		request.Messages = append(request.Messages, message)
	}
	// A call the conversation never answered is left pending on purpose: the client is
	// mid-turn and the backend is being asked to continue, which is ordinary.
	request.Discovered = state.discovered
	request.Removed = state.removed
	return nil
}

func decodeMessage(raw json.RawMessage, state *toolState) (Message, error) {
	fields, err := wire.Fields(raw, []string{"role", "content", "output_config"})
	if err != nil {
		return Message{}, refuse(CodeMessageFields, "messages")
	}

	var message Message
	roleValue, presence := wire.Of(fields, "role")
	if presence != wire.Present || json.Unmarshal(roleValue, &message.Role) != nil {
		return Message{}, refuse(CodeUnsupportedMessage, "role")
	}
	// Three roles, not two. The measured client sends system turns, and a decoder that
	// knew only user and assistant refused a real session at the first request — which is
	// how this list was corrected.
	if message.Role != "user" && message.Role != "assistant" && message.Role != "system" {
		return Message{}, refuse(CodeUnsupportedMessage, "role")
	}

	// A per-turn effort override. It rides on a system turn only: an override attached to
	// a user or assistant turn is a request nobody defined, not one to interpret loosely.
	if config, present := wire.Of(fields, "output_config"); present != wire.Absent {
		if message.Role != "system" {
			return Message{}, refuse(CodeMessageEffortRole, "output_config")
		}
		turn, err := wire.Fields(config, []string{"effort"})
		if err != nil {
			return Message{}, refuse(CodeOutputConfigFields, "output_config")
		}
		if effort, present := wire.Of(turn, "effort"); present == wire.Present {
			if json.Unmarshal(effort, &message.Effort) != nil {
				return Message{}, refuse(CodeOutputConfigFields, "effort")
			}
		}
	}

	contentValue, presence := wire.Of(fields, "content")
	if presence != wire.Present {
		return Message{}, refuse(CodeMessageFields, "content")
	}

	// A bare string is shorthand for a single text block. It is expanded here rather than
	// left for every reader to remember.
	var text string
	if json.Unmarshal(contentValue, &text) == nil {
		message.Blocks = []Block{{Type: "text", Text: text, Raw: contentValue}}
		return message, nil
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(contentValue, &blocks); err != nil {
		return Message{}, refuse(CodeMessageFields, "content")
	}
	for _, entry := range blocks {
		block, err := decodeContentBlock(entry, message.Role, state)
		if err != nil {
			return Message{}, err
		}
		message.Blocks = append(message.Blocks, block)
	}
	return message, nil
}

// decodeContentBlock dispatches on the block kind. Tool blocks need the conversation's
// identifier bookkeeping; a text block does not, which is why decodeBlock stays a pure
// function that other readers can call.
func decodeContentBlock(raw json.RawMessage, role string, state *toolState) (Block, error) {
	loose, err := wire.Fields(raw, nil)
	if err != nil {
		return Block{}, refuse(CodeUnsupportedContent, "content")
	}
	kindValue, present := wire.Of(loose, "type")
	var kind string
	if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil {
		return Block{}, refuse(CodeUnsupportedContent, "content")
	}
	switch kind {
	case "image":
		// Role is checked here rather than at the conversion, because a picture attached
		// to an assistant turn is a malformed request and not something to reinterpret.
		if role != "user" {
			return Block{}, refuse(CodeImageRole, "image")
		}
		return decodeImage(raw)
	case "tool_use":
		return decodeToolUse(raw, role, state)
	case "tool_result":
		return decodeToolResult(raw, role, state)
	case "tool_addition", "tool_removal":
		return decodeToolChange(raw, kind, role, state)
	}
	return decodeBlock(raw)
}

func decodeBlock(raw json.RawMessage) (Block, error) {
	// The type is read from a loose parse first, so that an unimplemented block is named
	// as unimplemented rather than as having the wrong fields for a text block.
	loose, err := wire.Fields(raw, nil)
	if err != nil {
		return Block{}, refuse(CodeUnsupportedContent, "content")
	}
	typeValue, presence := wire.Of(loose, "type")
	var kind string
	if presence != wire.Present || json.Unmarshal(typeValue, &kind) != nil {
		return Block{}, refuse(CodeUnsupportedContent, "content")
	}

	if kind != "text" {
		// Images, documents, reasoning. Each is a real shape this build has not
		// implemented, and naming it is what keeps the gap visible instead of turning a
		// request into a shorter one that happens to succeed.
		return Block{}, refuse(CodeUnsupportedContent, kind)
	}

	fields, err := wire.Fields(raw, []string{"type", "text", "cache_control"})
	if err != nil {
		return Block{}, refuse(CodeTextFields, "content")
	}
	block := Block{Type: kind, Raw: raw}
	textValue, presence := wire.Of(fields, "text")
	if presence != wire.Present || json.Unmarshal(textValue, &block.Text) != nil {
		return Block{}, refuse(CodeTextValue, "text")
	}
	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	return block, nil
}

// imageMediaTypes is what the backend accepts, and the list is the baseline's.
//
// Not a guess and not a superset. A media type outside it is refused rather than passed
// through: sending a format the backend will not read turns a picture the user attached
// into an error they cannot place.
var imageMediaTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

// base64Payload is standard base64 with optional padding. No whitespace and no URL-safe
// alphabet: the client sends neither, and accepting them here would mean re-encoding
// somebody's image on the way through.
var base64Payload = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)

// decodeImage reads one image block.
//
// Both key sets are closed. An unknown key on the block or on its source is refused
// rather than ignored, because an image carries its meaning in fields this build does not
// interpret, and quietly dropping one would send a different picture than was attached.
func decodeImage(raw json.RawMessage) (Block, error) {
	fields, err := wire.Fields(raw, []string{"type", "source", "cache_control"})
	if err != nil {
		return Block{}, refuse(CodeImageFields, "image")
	}
	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	sourceValue, present := wire.Of(fields, "source")
	if present != wire.Present {
		return Block{}, refuse(CodeImageFields, "source")
	}
	source, err := wire.Fields(sourceValue, []string{"type", "media_type", "data"})
	if err != nil {
		return Block{}, refuse(CodeImageSourceFields, "source")
	}

	var kind, mediaType, data string
	kindValue, ok := wire.Of(source, "type")
	if ok != wire.Present || json.Unmarshal(kindValue, &kind) != nil || kind != "base64" {
		return Block{}, refuse(CodeUnsupportedImage, "source.type")
	}
	typeValue, ok := wire.Of(source, "media_type")
	if ok != wire.Present || json.Unmarshal(typeValue, &mediaType) != nil || !imageMediaTypes[mediaType] {
		return Block{}, refuse(CodeUnsupportedImage, "source.media_type")
	}
	dataValue, ok := wire.Of(source, "data")
	if ok != wire.Present || json.Unmarshal(dataValue, &data) != nil || !base64Payload.MatchString(data) {
		return Block{}, refuse(CodeUnsupportedImage, "source.data")
	}
	return Block{Type: "image", Raw: raw, MediaType: mediaType, Data: data}, nil
}

// checkCacheControl validates a caching hint without acting on it.
//
// Caching is the backend's policy, so this build carries no cache behaviour — but a
// malformed hint is still a malformed request, and accepting one silently would be
// agreeing to something nobody read. The accepted shape matches the baseline.
func checkCacheControl(raw json.RawMessage) error {
	fields, err := wire.Fields(raw, []string{"type", "ttl", "scope"})
	if err != nil {
		return refuse(CodeCacheFields, "cache_control")
	}
	kindValue, present := wire.Of(fields, "type")
	var kind string
	if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil || kind != "ephemeral" {
		return refuse(CodeCacheValue, "cache_control")
	}
	if ttlValue, present := wire.Of(fields, "ttl"); present == wire.Present {
		var ttl string
		if json.Unmarshal(ttlValue, &ttl) != nil || (ttl != "5m" && ttl != "1h") {
			return refuse(CodeCacheValue, "ttl")
		}
	}
	return nil
}

func decodeOutputConfig(fields map[string]json.RawMessage, request *Request) error {
	value, presence := wire.Of(fields, "output_config")
	if presence != wire.Present {
		return nil
	}
	config, err := wire.Fields(value, []string{"effort", "format"})
	if err != nil {
		return refuse(CodeOutputConfigFields, "output_config")
	}
	if effort, present := wire.Of(config, "effort"); present == wire.Present {
		if err := json.Unmarshal(effort, &request.Effort); err != nil {
			return refuse(CodeOutputConfigFields, "effort")
		}
	}

	formatValue, present := wire.Of(config, "format")
	if present != wire.Present {
		return nil
	}
	format, err := wire.Fields(formatValue, []string{"type", "schema", "name"})
	if err != nil {
		if errors.Is(err, wire.ErrNotObject) {
			return refuse(CodeOutputFormatShape, "format")
		}
		return refuse(CodeOutputFormatFields, "format")
	}
	kindValue, present := wire.Of(format, "type")
	var kind string
	if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil || kind != "json_schema" {
		return refuse(CodeOutputFormatType, "type")
	}
	// The schema is carried through unread. JSON Schema semantics belong to the backend,
	// and a second, weaker validator here would only disagree with the one that decides.
	schemaValue, present := wire.Of(format, "schema")
	if present != wire.Present {
		return refuse(CodeOutputFormatSchema, "schema")
	}
	if _, err := wire.Fields(schemaValue, nil); err != nil {
		return refuse(CodeOutputFormatSchema, "schema")
	}
	if nameValue, present := wire.Of(format, "name"); present == wire.Present {
		var name string
		if json.Unmarshal(nameValue, &name) != nil || !identifier.MatchString(name) {
			return refuse(CodeOutputFormatName, "name")
		}
	}
	return nil
}

func decodeThinking(fields map[string]json.RawMessage) error {
	value, presence := wire.Of(fields, "thinking")
	if presence != wire.Present {
		return nil
	}
	thinking, err := wire.Fields(value, []string{"type", "display", "budget_tokens"})
	if err != nil {
		return refuse(CodeThinkingFields, "thinking")
	}
	kindValue, present := wire.Of(thinking, "type")
	var kind string
	if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil {
		return refuse(CodeThinkingType, "type")
	}
	if kind != "adaptive" && kind != "enabled" && kind != "disabled" {
		return refuse(CodeThinkingType, kind)
	}
	if budget, present := wire.Of(thinking, "budget_tokens"); present == wire.Present {
		number, err := exactInteger(budget)
		if err != nil || number <= 0 || number > maxSafeInteger {
			return refuse(CodeThinkingBudget, "budget_tokens")
		}
	}
	return nil
}

// The one context edit the baseline consumes is a semantic no-op. Anything else changes
// what the model is asked to remember, and honouring an edit this bridge has not
// implemented would quietly alter the conversation.
func decodeContextManagement(fields map[string]json.RawMessage) error {
	value, presence := wire.Of(fields, "context_management")
	if presence != wire.Present {
		return nil
	}
	management, err := wire.Fields(value, []string{"edits"})
	if err != nil {
		return refuse(CodeContextFields, "context_management")
	}
	editsValue, present := wire.Of(management, "edits")
	if present != wire.Present {
		return refuse(CodeContextFields, "edits")
	}
	var edits []json.RawMessage
	if err := json.Unmarshal(editsValue, &edits); err != nil || len(edits) != 1 {
		return refuse(CodeUnsupportedEdit, "edits")
	}
	edit, err := wire.Fields(edits[0], []string{"type", "keep"})
	if err != nil {
		return refuse(CodeUnsupportedEdit, "edits")
	}
	kind, _ := wire.Of(edit, "type")
	keep, _ := wire.Of(edit, "keep")
	if string(kind) != `"clear_thinking_20251015"` || string(keep) != `"all"` {
		return refuse(CodeUnsupportedEdit, "edits")
	}
	return nil
}

// exactInteger reads a JSON number from its literal text.
//
// Not via json.Number: unmarshalling into one accepts the JSON *string* "1024" as the
// number 1024, so a client sending a quoted value would have been read as if it had sent
// a number. Measured, not assumed — a test caught it. Parsing the literal also refuses
// 1.5, 1e100 and anything past int64, none of which are integers a caller asked for.
func exactInteger(value json.RawMessage) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
}

func (r *Request) String() string {
	return fmt.Sprintf("anthropic.Request{model:%q messages:%d tools:%d}",
		r.Model, len(r.Messages), len(r.Tools))
}
