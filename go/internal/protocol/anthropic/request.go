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
	"encoding/base64"
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
	CodeTextCitations        = "UNSUPPORTED_TEXT_CITATIONS"
	CodeCacheFields          = "CACHE_FIELDS"
	CodeCacheValue           = "CACHE_VALUE"
	CodeUnsupportedContent   = "UNSUPPORTED_CONTENT"
	CodeImageFields          = "IMAGE_FIELDS"
	CodeImageSourceFields    = "IMAGE_SOURCE_FIELDS"
	CodeUnsupportedImage     = "UNSUPPORTED_IMAGE"
	CodeImageRole            = "IMAGE_ROLE"
	CodeDocumentFields       = "DOCUMENT_FIELDS"
	CodeDocumentSourceFields = "DOCUMENT_SOURCE_FIELDS"
	CodeUnsupportedDocument  = "UNSUPPORTED_DOCUMENT"
	CodeDocumentRole         = "DOCUMENT_ROLE"
	CodeRedactedFields       = "REDACTED_FIELDS"
	CodeRedactedRole         = "REDACTED_ROLE"
	CodeReasoningFields      = "REASONING_FIELDS"
	CodeUnsupportedThinking  = "UNSUPPORTED_THINKING"
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
	// Field names the offending member when one is identifiable: a name this package chose,
	// or one the request used -- a key, a type, a tool name.
	Field string
	// Unknown says Field is something this build does not know -- a key outside the
	// allowlist, or a block or thinking type -- which is what a client update adds. A known
	// member with a bad value, or a repeated key, is not Unknown (#127).
	Unknown bool
}

func (e *RequestError) Error() string {
	if e.Field != "" {
		return e.Code + " " + e.Field
	}
	return e.Code
}

func refuse(code, field string) error { return &RequestError{Code: code, Field: field} }

// refuseFields refuses a closed key set, naming the key that broke it when there is one.
//
// An unknown key is what a client update adds, and a refusal that names only the object
// it sat in leaves the fix -- one more allowed name -- for someone to rediscover (#127).
func refuseFields(code, object string, err error) error {
	var fieldErr *wire.FieldError
	if errors.As(err, &fieldErr) {
		return &RequestError{Code: code, Field: fieldErr.Field, Unknown: errors.Is(err, wire.ErrUnknownField)}
	}
	return refuse(code, object)
}

// refuseUnknown refuses a type this build does not know, naming it.
func refuseUnknown(code, kind string) error { return &RequestError{Code: code, Field: kind, Unknown: true} }

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

	// redacted_thinking
	Reasoning *Reasoning
}

// Reasoning is the model's own chain of thought, recorded in the client's transcript so it
// can be handed back on the next turn.
//
// The content is encrypted and this build never reads it. What it does is keep it intact:
// the request asks the backend for reasoning.encrypted_content, and throwing away what
// comes back would mean asking for something and discarding it, leaving the model to start
// its thinking again every turn.
type Reasoning struct {
	ID        string
	Summary   []SummaryText
	Encrypted string
}

// SummaryText is one part of the visible summary that travels beside the encrypted content.
type SummaryText struct {
	Type string
	Text string
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
	NonStreaming bool
	Model        string
	MaxTokens    int64
	Messages     []Message
	System       json.RawMessage
	Effort       string
	Fields       map[string]json.RawMessage

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

	// OutputFormat is the JSON Schema the answer must obey, when the request named one.
	//
	// Carried rather than only validated. Validating a schema and then not sending it is
	// the worst of both: the request is accepted, so the client believes the constraint
	// holds, and the model never hears about it -- the answer comes back as prose and
	// whatever asked for structure fails somewhere further away from the cause.
	OutputFormat *OutputFormat
}

// OutputFormat is a structured output request, in the baseline's shape.
type OutputFormat struct {
	// Name is what the backend calls the schema. The baseline defaults it rather than
	// omitting it, so a request that named none still produces the same wire shape.
	Name string
	// Schema is carried through unread. JSON Schema semantics belong to the backend, and
	// a second, weaker validator here would only disagree with the one that decides.
	Schema json.RawMessage
}

// HostedSearch is a server-side search tool definition.
type HostedSearch struct {
	Type string
	Name string
	// Allowed and Blocked are the domain filters, already bounded. Nil means the request
	// named none, which is different from naming an empty list.
	Allowed []string
	Blocked []string
	// Location is the caller's approximate location when the request named one, validated
	// and carried. Where it goes on the search wire is not yet measured -- see
	// bridge.BuildSearchRequest.
	Location map[string]string
}

// ToolCount reports how many definitions were supplied.
func (r *Request) ToolCount() int { return len(r.Tools) }

// maxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER. The baseline validates against
// it, and a client that round-trips a larger value through a JSON number cannot be relied
// on to have sent what it meant.
const maxSafeInteger = int64(1)<<53 - 1

// DecodeRequest validates an inference request and returns what this build understands.
//
// It refuses rather than repairs. A stream flag with the wrong type, a sampling parameter this
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

	// The Messages API defaults an omitted stream flag to a single JSON response.
	switch value, presence := wire.Of(fields, "stream"); {
	case string(value) == "true":
		// streaming delivery
	case presence == wire.Absent || string(value) == "false":
		request.NonStreaming = true
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
		//
		// The hosted search tool counts as nameable even though it is not callable. It is
		// kept out of the tool list on purpose -- it is not a definition the client can be
		// told to call, it is a request for this gateway to search -- but the client may
		// still point tool_choice at it, and the baseline accepts that
		// (native-protocol.mjs:334). Refusing it here made the branch that reads that shape
		// unreachable: a side query naming its own tool was answered 400 before anything
		// looked at what it was.
		hosted := request.HostedSearch != nil && request.HostedSearch.Name == request.ToolChoice.Name
		if !hosted && !request.CallableNames()[request.ToolChoice.Name] {
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
		if err := checkSystem(value); err != nil {
			return nil, err
		}
		request.System = value
	}
	return request, nil
}

// checkSystem holds the system prompt to the shapes it may take: a string, or text blocks
// read by the rules a message's are. A non-text block or an unknown key used to be dropped
// on the way to the backend, and a system prompt short by a part is a request nobody sent
// (#91).
func checkSystem(raw json.RawMessage) error {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return nil
	}
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return refuse(CodeTextValue, "system")
	}
	for _, block := range blocks {
		if _, err := decodeBlock(block); err != nil {
			return err
		}
	}
	return nil
}

func translateFieldError(err error) error {
	var fieldErr *wire.FieldError
	if errors.As(err, &fieldErr) {
		return refuseFields(CodeRequestFields, "", err)
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
		return Message{}, refuseFields(CodeMessageFields, "messages", err)
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
			return Message{}, refuseFields(CodeOutputConfigFields, "output_config", err)
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
	case "document":
		// Same rule as an image and for the same reason: a file attached to an assistant
		// turn is a transcript that has been edited, not a request to honour.
		if role != "user" {
			return Block{}, refuse(CodeDocumentRole, "document")
		}
		return decodeDocument(raw)
	case "tool_use":
		return decodeToolUse(raw, role, state)
	case "tool_result":
		return decodeToolResult(raw, role, state)
	case "tool_addition", "tool_removal":
		return decodeToolChange(raw, kind, role, state)
	case "redacted_thinking":
		// Only an assistant turn has a chain of thought. One attached to a user turn is a
		// transcript that has been edited, not a request to honour.
		if role != "assistant" {
			return Block{}, refuse(CodeRedactedRole, "redacted_thinking")
		}
		return decodeRedactedThinking(raw)
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
		return Block{}, refuseUnknown(CodeUnsupportedContent, kind)
	}

	fields, err := wire.Fields(raw, []string{"type", "text", "cache_control", "citations"})
	if err != nil {
		return Block{}, refuseFields(CodeTextFields, "content", err)
	}
	block := Block{Type: kind, Raw: raw}
	textValue, presence := wire.Of(fields, "text")
	if presence != wire.Present || json.Unmarshal(textValue, &block.Text) != nil {
		return Block{}, refuse(CodeTextValue, "text")
	}
	// Native retains interrupted text with citations:null. Empty citations carry
	// no references to translate; populated/invalid values must not be discarded.
	if value, present := fields["citations"]; present {
		var citations []json.RawMessage
		if json.Unmarshal(value, &citations) != nil || len(citations) != 0 {
			return Block{}, refuse(CodeTextCitations, "citations")
		}
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
		return Block{}, refuseFields(CodeImageFields, "image", err)
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
		return Block{}, refuseFields(CodeImageSourceFields, "source", err)
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

// documentMediaTypes is what an attached file may be, and the list is one entry because
// one entry is what has been measured.
//
// The backend reads a base64 PDF sent as an input_file data URL -- asked directly
// (`clauduct-dev probe file --send`), and the model returned a token that existed only
// inside the PDF. Nothing establishes any other type, and a type this build forwards
// without evidence turns an attachment the user made into an answer about nothing.
var documentMediaTypes = map[string]bool{"application/pdf": true}

// decodeDocument reads one document block.
//
// The shape is the client's own, read out of claude 2.1.274 rather than from a
// specification: {type:"document", source:{type:"base64", media_type:"application/pdf",
// data}}. It arrives two ways -- attached to a user turn, and inside a tool_result when
// Read opens a PDF -- and the second is the common one.
//
// Both key sets are closed, as for an image: a document carries its meaning in fields this
// build does not interpret, and quietly dropping one would send a different file than was
// attached.
func decodeDocument(raw json.RawMessage) (Block, error) {
	fields, err := wire.Fields(raw, []string{"type", "source", "cache_control"})
	if err != nil {
		return Block{}, refuseFields(CodeDocumentFields, "document", err)
	}
	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	sourceValue, present := wire.Of(fields, "source")
	if present != wire.Present {
		return Block{}, refuse(CodeDocumentFields, "source")
	}
	source, err := wire.Fields(sourceValue, []string{"type", "media_type", "data"})
	if err != nil {
		return Block{}, refuseFields(CodeDocumentSourceFields, "source", err)
	}

	var kind, mediaType, data string
	kindValue, ok := wire.Of(source, "type")
	if ok != wire.Present || json.Unmarshal(kindValue, &kind) != nil || kind != "base64" {
		// url and file sources name something this build would have to fetch or look up,
		// and it has neither the Files API nor any business fetching a URL for the model.
		return Block{}, refuse(CodeUnsupportedDocument, "source.type")
	}
	typeValue, ok := wire.Of(source, "media_type")
	if ok != wire.Present || json.Unmarshal(typeValue, &mediaType) != nil || !documentMediaTypes[mediaType] {
		return Block{}, refuse(CodeUnsupportedDocument, "source.media_type")
	}
	dataValue, ok := wire.Of(source, "data")
	if ok != wire.Present || json.Unmarshal(dataValue, &data) != nil || !base64Payload.MatchString(data) {
		return Block{}, refuse(CodeUnsupportedDocument, "source.data")
	}
	return Block{Type: "document", Raw: raw, MediaType: mediaType, Data: data}, nil
}

// checkCacheControl validates a caching hint without acting on it.
//
// Caching is the backend's policy, so this build carries no cache behaviour — but a
// malformed hint is still a malformed request, and accepting one silently would be
// agreeing to something nobody read. The accepted shape matches the baseline.
func checkCacheControl(raw json.RawMessage) error {
	fields, err := wire.Fields(raw, []string{"type", "ttl", "scope"})
	if err != nil {
		return refuseFields(CodeCacheFields, "cache_control", err)
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
		return refuseFields(CodeOutputConfigFields, "output_config", err)
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
		return refuseFields(CodeOutputFormatFields, "format", err)
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
	name := DefaultSchemaName
	if nameValue, present := wire.Of(format, "name"); present == wire.Present {
		if json.Unmarshal(nameValue, &name) != nil || !identifier.MatchString(name) {
			return refuse(CodeOutputFormatName, "name")
		}
	}
	request.OutputFormat = &OutputFormat{Name: name, Schema: schemaValue}
	return nil
}

// DefaultSchemaName is what an unnamed schema is called upstream. The baseline's value:
// the backend requires a name, and inventing a different one here would make the same
// request from the same client look like two schemas depending on which build served it.
const DefaultSchemaName = "structured_output"

func decodeThinking(fields map[string]json.RawMessage) error {
	value, presence := wire.Of(fields, "thinking")
	if presence != wire.Present {
		return nil
	}
	thinking, err := wire.Fields(value, []string{"type", "display", "budget_tokens"})
	if err != nil {
		return refuseFields(CodeThinkingFields, "thinking", err)
	}
	kindValue, present := wire.Of(thinking, "type")
	var kind string
	if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil {
		return refuse(CodeThinkingType, "type")
	}
	if kind != "adaptive" && kind != "enabled" && kind != "disabled" {
		return refuseUnknown(CodeThinkingType, kind)
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
		return refuseFields(CodeContextFields, "context_management", err)
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
		return refuseFields(CodeUnsupportedEdit, "edits", err)
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

// ReasoningPrefix marks a redacted_thinking block as this bridge's own envelope.
//
// The exact string the Node baseline uses, and that is an interop contract rather than a
// detail: a transcript recorded under one implementation is resumed under the other, and a
// different prefix would make every recorded thought unreadable at exactly the moment it
// was needed.
const ReasoningPrefix = "clauduct-reasoning-v1:"

// DecodeRecordedThought reads back an envelope this bridge wrote.
//
// Exported so the side that writes one can read it straight back and find out immediately
// if the two halves have drifted, rather than a session later when a transcript is resumed.
func DecodeRecordedThought(data string) (*Reasoning, error) {
	block, err := decodeRedactedThinking(json.RawMessage(
		`{"type":"redacted_thinking","data":` + quoteJSON(data) + `}`))
	if err != nil {
		return nil, err
	}
	return block.Reasoning, nil
}

func quoteJSON(value string) string {
	out, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(out)
}

// decodeRedactedThinking reads back a chain of thought this bridge recorded.
//
// Everything about the envelope is checked before anything is believed. What it carries is
// opaque and stays opaque -- the encrypted content is never read here, only kept whole --
// but the envelope around it is this build's own, so a malformed one is a transcript that
// has been tampered with or truncated rather than something to pass on to the backend.
func decodeRedactedThinking(raw json.RawMessage) (Block, error) {
	fields, err := wire.Fields(raw, []string{"type", "data", "cache_control"})
	if err != nil {
		return Block{}, refuseFields(CodeRedactedFields, "redacted_thinking", err)
	}
	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	var data string
	value, present := wire.Of(fields, "data")
	if present != wire.Present || json.Unmarshal(value, &data) != nil ||
		!strings.HasPrefix(data, ReasoningPrefix) {
		return Block{}, refuse(CodeUnsupportedThinking, "data")
	}
	// base64url without padding, which is what Node's 'base64url' encoding produces. The
	// padding is tolerated on the way in because a transcript is written by something else.
	decoded, err := base64.RawURLEncoding.DecodeString(
		strings.TrimRight(data[len(ReasoningPrefix):], "="))
	if err != nil {
		return Block{}, refuse(CodeUnsupportedThinking, "data")
	}

	saved, err := wire.Fields(decoded, []string{"type", "id", "summary", "encrypted_content"})
	if err != nil {
		return Block{}, refuseFields(CodeReasoningFields, "reasoning", err)
	}
	var kind, id, encrypted string
	kindValue, ok := wire.Of(saved, "type")
	if ok != wire.Present || json.Unmarshal(kindValue, &kind) != nil || kind != "reasoning" {
		return Block{}, refuse(CodeUnsupportedThinking, "type")
	}
	idValue, ok := wire.Of(saved, "id")
	if ok != wire.Present || json.Unmarshal(idValue, &id) != nil || !identifier.MatchString(id) {
		return Block{}, refuse(CodeUnsupportedThinking, "id")
	}
	encryptedValue, ok := wire.Of(saved, "encrypted_content")
	if ok != wire.Present || json.Unmarshal(encryptedValue, &encrypted) != nil {
		return Block{}, refuse(CodeUnsupportedThinking, "encrypted_content")
	}

	// Always a list, never absent and never null: the backend is handed back exactly the
	// shape it produced, and an empty summary is a summary with nothing in it.
	summary := []SummaryText{}
	summaryValue, ok := wire.Of(saved, "summary")
	if ok != wire.Present {
		return Block{}, refuse(CodeUnsupportedThinking, "summary")
	}
	var parts []json.RawMessage
	if json.Unmarshal(summaryValue, &parts) != nil {
		return Block{}, refuse(CodeUnsupportedThinking, "summary")
	}
	for _, entry := range parts {
		part, err := wire.Fields(entry, []string{"type", "text"})
		if err != nil {
			return Block{}, refuseFields(CodeUnsupportedThinking, "summary", err)
		}
		var partType, text string
		typeValue, has := wire.Of(part, "type")
		textValue, hasText := wire.Of(part, "text")
		if has != wire.Present || json.Unmarshal(typeValue, &partType) != nil || partType != "summary_text" ||
			hasText != wire.Present || json.Unmarshal(textValue, &text) != nil {
			return Block{}, refuse(CodeUnsupportedThinking, "summary")
		}
		summary = append(summary, SummaryText{Type: partType, Text: text})
	}

	return Block{Type: "redacted_thinking", Raw: raw,
		Reasoning: &Reasoning{ID: id, Summary: summary, Encrypted: encrypted}}, nil
}
