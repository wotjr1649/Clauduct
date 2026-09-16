package anthropic

import (
	"encoding/json"
	"regexp"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Tool refusal categories, matching the baseline's vocabulary.
const (
	CodeToolFields        = "TOOL_FIELDS"
	CodeUnsupportedTools  = "UNSUPPORTED_TOOLS"
	CodeToolChoiceFields  = "TOOL_CHOICE_FIELDS"
	CodeInvalidToolCall   = "INVALID_TOOL_CALL"
	CodeInvalidToolResult = "INVALID_TOOL_RESULT"
	CodeToolResultShape   = "TOOL_RESULT_SHAPE"
	CodeToolRefFields     = "TOOL_REFERENCE_FIELDS"
	CodeInvalidToolRef    = "INVALID_TOOL_REFERENCE"
	CodeUnsupportedChange = "UNSUPPORTED_TOOL_CHANGE"
	CodeHostedToolUnsupp  = "HOSTED_TOOL_UNSUPPORTED"
)

// A server-side search tool. It is executed by the backend rather than by the client, so it
// is a different capability with its own request budget and result semantics — not a
// callable definition. This build does not carry it, and says so rather than dropping it
// from the tool list and answering as though it had never been asked for.
var hostedSearchTool = regexp.MustCompile(`^web_search_20\d{6}$`)

// Tool is one callable definition. InputSchema is carried unread: JSON Schema semantics
// belong to the backend, and a second, weaker validator here would only disagree with the
// one that decides.
type Tool struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	DeferLoading bool
}

// ToolChoice is how the caller constrained tool selection.
type ToolChoice struct {
	Present                bool
	Type                   string
	Name                   string
	DisableParallelToolUse bool
}

// decodeTools validates the tool definitions and records which names are callable.
//
// Two name sets come out of this and they are not the same. Active is what may be called
// next; a call naming anything else is refused. Discovered additionally holds names seen in
// the conversation's history, so a transcript recorded under a different tool set still
// decodes — completed history survives a changed tool or permission set on resume, while
// every new call is still checked against what is callable now.
func decodeTools(fields map[string]json.RawMessage, request *Request) error {
	value, presence := wire.Of(fields, "tools")
	if presence == wire.Absent {
		return nil
	}
	if presence == wire.Null {
		return refuse(CodeToolsShape, "tools")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(value, &raw); err != nil {
		return refuse(CodeToolsShape, "tools")
	}

	seen := make(map[string]bool, len(raw))
	for _, entry := range raw {
		loose, err := wire.Fields(entry, nil)
		if err != nil {
			return refuse(CodeToolFields, "tools")
		}
		if kindValue, present := wire.Of(loose, "type"); present == wire.Present {
			var kind string
			if json.Unmarshal(kindValue, &kind) == nil && hostedSearchTool.MatchString(kind) {
				hosted, err := decodeHostedSearch(entry, kind)
				if err != nil {
					return err
				}
				if request.HostedSearch != nil {
					return refuse(CodeUnsupportedTools, kind)
				}
				request.HostedSearch = hosted
				continue
			}
		}

		tool, err := decodeTool(entry)
		if err != nil {
			return err
		}
		if seen[tool.Name] {
			return refuse(CodeUnsupportedTools, tool.Name)
		}
		seen[tool.Name] = true
		request.Tools = append(request.Tools, tool)
	}
	return nil
}

func decodeTool(raw json.RawMessage) (Tool, error) {
	fields, err := wire.Fields(raw, []string{"name", "description", "input_schema", "cache_control", "defer_loading"})
	if err != nil {
		return Tool{}, refuse(CodeToolFields, "tools")
	}

	var tool Tool
	nameValue, present := wire.Of(fields, "name")
	if present != wire.Present || json.Unmarshal(nameValue, &tool.Name) != nil || !identifier.MatchString(tool.Name) {
		return Tool{}, refuse(CodeUnsupportedTools, "name")
	}
	schema, present := wire.Of(fields, "input_schema")
	if present != wire.Present {
		return Tool{}, refuse(CodeUnsupportedTools, "input_schema")
	}
	schemaFields, err := wire.Fields(schema, nil)
	if err != nil {
		return Tool{}, refuse(CodeUnsupportedTools, "input_schema")
	}
	if kind, present := wire.Of(schemaFields, "type"); present != wire.Present || string(kind) != `"object"` {
		return Tool{}, refuse(CodeUnsupportedTools, "input_schema")
	}
	tool.InputSchema = schema

	if value, present := wire.Of(fields, "description"); present == wire.Present {
		if json.Unmarshal(value, &tool.Description) != nil {
			return Tool{}, refuse(CodeUnsupportedTools, "description")
		}
	}
	if value, present := wire.Of(fields, "defer_loading"); present == wire.Present {
		if json.Unmarshal(value, &tool.DeferLoading) != nil {
			return Tool{}, refuse(CodeUnsupportedTools, "defer_loading")
		}
	}
	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Tool{}, err
		}
	}
	return tool, nil
}

func decodeToolChoice(fields map[string]json.RawMessage, request *Request) error {
	value, presence := wire.Of(fields, "tool_choice")
	if presence == wire.Absent {
		return nil
	}
	choice, err := wire.Fields(value, []string{"type", "name", "disable_parallel_tool_use"})
	if err != nil {
		return refuse(CodeToolChoiceFields, "tool_choice")
	}

	request.ToolChoice.Present = true
	kindValue, present := wire.Of(choice, "type")
	if present != wire.Present || json.Unmarshal(kindValue, &request.ToolChoice.Type) != nil {
		return refuse(CodeUnsupportedTools, "tool_choice")
	}
	switch request.ToolChoice.Type {
	case "auto", "any", "none", "tool":
	default:
		return refuse(CodeUnsupportedTools, "tool_choice")
	}
	if nameValue, present := wire.Of(choice, "name"); present == wire.Present {
		if json.Unmarshal(nameValue, &request.ToolChoice.Name) != nil || !identifier.MatchString(request.ToolChoice.Name) {
			return refuse(CodeUnsupportedTools, "tool_choice")
		}
	}
	if value, present := wire.Of(choice, "disable_parallel_tool_use"); present == wire.Present {
		if json.Unmarshal(value, &request.ToolChoice.DisableParallelToolUse) != nil {
			return refuse(CodeUnsupportedTools, "tool_choice")
		}
	}
	return nil
}

// decodeToolUse reads an assistant's recorded call.
//
// Input stays as the bytes that arrived. That is what keeps {} , {"isolation":null} and
// {"isolation":"worktree"} three different instructions: re-encoding a decoded map would
// drop the distinction between an omitted optional and one explicitly cleared, and a
// "default tidy-up" is how a deliberate choice disappears.
func decodeToolUse(raw json.RawMessage, role string, state *toolState) (Block, error) {
	fields, err := wire.Fields(raw, []string{"type", "id", "name", "input", "cache_control"})
	if err != nil {
		return Block{}, refuse(CodeToolUseFieldsCode, "tool_use")
	}
	if role != "assistant" {
		return Block{}, refuse(CodeInvalidToolCall, "role")
	}

	block := Block{Type: "tool_use", Raw: raw}
	idValue, present := wire.Of(fields, "id")
	if present != wire.Present || json.Unmarshal(idValue, &block.ID) != nil || !identifier.MatchString(block.ID) {
		return Block{}, refuse(CodeInvalidToolCall, "id")
	}
	nameValue, present := wire.Of(fields, "name")
	if present != wire.Present || json.Unmarshal(nameValue, &block.Name) != nil || !identifier.MatchString(block.Name) {
		return Block{}, refuse(CodeInvalidToolCall, "name")
	}
	inputValue, present := wire.Of(fields, "input")
	if present != wire.Present {
		return Block{}, refuse(CodeInvalidToolCall, "input")
	}
	if _, err := wire.Fields(inputValue, nil); err != nil {
		return Block{}, refuse(CodeInvalidToolCall, "input")
	}
	block.Input = inputValue

	// One identifier, one call. A repeated id makes two calls indistinguishable, and a
	// result addressed to it could be matched to either.
	if state.used[block.ID] {
		return Block{}, refuse(CodeInvalidToolCall, "id")
	}
	state.used[block.ID] = true
	state.pending[block.ID] = true
	// A name seen in history is discovered, not activated. It lets a transcript recorded
	// under a different tool set decode; it does not make that tool callable again.
	state.discovered[block.Name] = true

	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	return block, nil
}

// decodeToolResult reads the client's answer to a recorded call.
func decodeToolResult(raw json.RawMessage, role string, state *toolState) (Block, error) {
	fields, err := wire.Fields(raw, []string{"type", "tool_use_id", "content", "is_error", "cache_control"})
	if err != nil {
		return Block{}, refuse(CodeToolResultFieldsCode, "tool_result")
	}
	if role != "user" {
		return Block{}, refuse(CodeInvalidToolResult, "role")
	}

	block := Block{Type: "tool_result", Raw: raw}
	idValue, present := wire.Of(fields, "tool_use_id")
	if present != wire.Present || json.Unmarshal(idValue, &block.ToolUseID) != nil || !identifier.MatchString(block.ToolUseID) {
		return Block{}, refuse(CodeInvalidToolResult, "tool_use_id")
	}
	// A result must answer a call that was made and has not been answered. An unmatched
	// result is a claim about work nobody asked for.
	if !state.pending[block.ToolUseID] {
		return Block{}, refuse(CodeInvalidToolResult, "tool_use_id")
	}
	delete(state.pending, block.ToolUseID)

	if value, present := wire.Of(fields, "is_error"); present == wire.Present {
		if json.Unmarshal(value, &block.IsError) != nil {
			return Block{}, refuse(CodeInvalidToolResult, "is_error")
		}
	}

	contentValue, present := wire.Of(fields, "content")
	switch {
	case present == wire.Absent:
		// An answer with no content is still an answer.
	default:
		parts, err := decodeResultParts(contentValue, state)
		if err != nil {
			return Block{}, err
		}
		block.Result = parts
	}

	if control, present := wire.Of(fields, "cache_control"); present != wire.Absent {
		if err := checkCacheControl(control); err != nil {
			return Block{}, err
		}
	}
	return block, nil
}

// ResultPart is one piece of a tool result.
type ResultPart struct {
	Type string
	Text string
	Name string // tool_reference only

	// image only
	MediaType string
	Data      string
}

func decodeResultParts(raw json.RawMessage, state *toolState) ([]ResultPart, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []ResultPart{{Type: "text", Text: text}}, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, refuse(CodeToolResultShape, "content")
	}

	parts := make([]ResultPart, 0, len(entries))
	for _, entry := range entries {
		loose, err := wire.Fields(entry, nil)
		if err != nil {
			return nil, refuse(CodeToolResultShape, "content")
		}
		kindValue, present := wire.Of(loose, "type")
		var kind string
		if present != wire.Present || json.Unmarshal(kindValue, &kind) != nil {
			return nil, refuse(CodeToolResultShape, "content")
		}

		switch kind {
		case "text":
			block, err := decodeBlock(entry)
			if err != nil {
				return nil, err
			}
			parts = append(parts, ResultPart{Type: "text", Text: block.Text})
		case "image":
			// No role check here: a tool result is already required to be a user turn, so
			// the question the block-level check answers has been answered.
			block, err := decodeImage(entry)
			if err != nil {
				return nil, err
			}
			parts = append(parts, ResultPart{Type: "image", MediaType: block.MediaType, Data: block.Data})
		case "tool_reference":
			reference, err := wire.Fields(entry, []string{"type", "tool_name", "cache_control"})
			if err != nil {
				return nil, refuse(CodeToolRefFields, "tool_reference")
			}
			nameValue, present := wire.Of(reference, "tool_name")
			var name string
			if present != wire.Present || json.Unmarshal(nameValue, &name) != nil || !identifier.MatchString(name) {
				return nil, refuse(CodeInvalidToolRef, "tool_name")
			}
			// A historical reference is data, not a definition that can reactivate a tool.
			state.discovered[name] = true
			parts = append(parts, ResultPart{Type: "tool_reference", Name: name})
		default:
			return nil, refuse(CodeUnsupportedContent, kind)
		}
	}
	return parts, nil
}

// toolState tracks identifiers and names across a whole conversation.
type toolState struct {
	used       map[string]bool
	pending    map[string]bool
	discovered map[string]bool
}

func newToolState() *toolState {
	return &toolState{
		used:       make(map[string]bool, 8),
		pending:    make(map[string]bool, 8),
		discovered: make(map[string]bool, 8),
	}
}

// ActiveTools reports the definitions that go upstream, and therefore the only names a new
// call may use.
//
// A deferred tool is held back until something in the conversation has named it. That is
// the point of deferring: a tool nobody has mentioned costs schema on every request.
func (r *Request) ActiveTools() []Tool {
	active := make([]Tool, 0, len(r.Tools))
	for _, tool := range r.Tools {
		if tool.DeferLoading && !r.Discovered[tool.Name] {
			continue
		}
		active = append(active, tool)
	}
	return active
}

// CallableNames is the set a new call is checked against.
func (r *Request) CallableNames() map[string]bool {
	names := make(map[string]bool, len(r.Tools))
	for _, tool := range r.ActiveTools() {
		names[tool.Name] = true
	}
	return names
}

// searchDomainLimits bound a filter list. The baseline's numbers: at most thirty-two
// domains, each at most the length a DNS name can be.
const (
	maxSearchDomains    = 32
	maxSearchDomainName = 253
)

// decodeHostedSearch reads the server-side search tool.
//
// The domain filters are bounded here rather than where they are used, because they travel
// to the backend and a list the client did not bound is a list this build would be sending
// on its behalf. A malformed filter drops to nil rather than failing the request: the
// baseline treats an unusable filter as no filter, and refusing the whole search over one
// would take away the feature to protect a narrowing nobody can act on.
func decodeHostedSearch(raw json.RawMessage, kind string) (*HostedSearch, error) {
	fields, err := wire.Fields(raw, []string{"type", "name", "allowed_domains", "blocked_domains",
		"max_uses", "cache_control"})
	if err != nil {
		return nil, refuse(CodeToolFields, "tools")
	}
	var name string
	nameValue, present := wire.Of(fields, "name")
	if present != wire.Present || json.Unmarshal(nameValue, &name) != nil || name == "" {
		return nil, refuse(CodeToolFields, "name")
	}
	return &HostedSearch{
		Type:    kind,
		Name:    name,
		Allowed: searchDomains(fields, "allowed_domains"),
		Blocked: searchDomains(fields, "blocked_domains"),
	}, nil
}

func searchDomains(fields map[string]json.RawMessage, key string) []string {
	value, present := wire.Of(fields, key)
	if present != wire.Present {
		return nil
	}
	var list []string
	if json.Unmarshal(value, &list) != nil || len(list) == 0 {
		return nil
	}
	for _, name := range list {
		if name == "" || len(name) > maxSearchDomainName {
			return nil
		}
	}
	if len(list) > maxSearchDomains {
		list = list[:maxSearchDomains]
	}
	return list
}
