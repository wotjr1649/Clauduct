package anthropic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A minimal request this build accepts: text only, no tools.
const textOnly = `{"model":"gpt-6-astra","max_tokens":1024,"stream":true,
  "messages":[{"role":"user","content":"ping"}]}`

func decode(t *testing.T, body string) (*Request, *RequestError) {
	t.Helper()
	request, err := DecodeRequest([]byte(body))
	if err == nil {
		return request, nil
	}
	var refusal *RequestError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a *RequestError", err)
	}
	return nil, refusal
}

func mustRefuse(t *testing.T, body, code string) {
	t.Helper()
	_, refusal := decode(t, body)
	if refusal == nil {
		t.Fatalf("accepted a request that should have been refused with %s", code)
	}
	if refusal.Code != code {
		t.Fatalf("code = %s, want %s", refusal.Code, code)
	}
}

func TestTextOnlyRequestIsAccepted(t *testing.T) {
	request, refusal := decode(t, textOnly)
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	if request.Model != "gpt-6-astra" || request.MaxTokens != 1024 {
		t.Fatalf("model=%q max_tokens=%d", request.Model, request.MaxTokens)
	}
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", request.Messages)
	}
	if len(request.Messages[0].Blocks) != 1 || request.Messages[0].Blocks[0].Text != "ping" {
		t.Fatalf("blocks = %+v", request.Messages[0].Blocks)
	}
}

// The shape the measured claude 2.1.272 actually sends. Every top-level field it uses must
// be recognised — a REQUEST_FIELDS refusal here would mean the allowlist is wrong, not that
// the client is. What it must not do is quietly succeed while ignoring the tools.
func TestTheMeasuredClientEnvelopeIsRecognisedAndToolsAreNamed(t *testing.T) {
	body := `{
	  "model":"gpt-6-astra","max_tokens":32000,"stream":true,
	  "messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}],
	  "system":[{"type":"text","text":"You are Claude Code."}],
	  "metadata":{"user_id":"abc"},
	  "output_config":{"effort":"medium"},
	  "thinking":{"type":"adaptive"},
	  "context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},
	  "tools":[{"name":"Read","description":"read a file","input_schema":{"type":"object"}}]
	}`
	_, refusal := decode(t, body)
	if refusal == nil {
		t.Fatal("accepted a request carrying tools; WP03 has no tool support and must say so")
	}
	if refusal.Code != CodeToolUseUnsupported {
		t.Fatalf("code = %s, want %s — every other field in the measured envelope must be recognised",
			refusal.Code, CodeToolUseUnsupported)
	}
}

// Dropping the tools and keeping everything else must be accepted, which is what proves
// the refusal above was about tools and not about some other field being unrecognised.
func TestTheMeasuredEnvelopeWithoutToolsIsAccepted(t *testing.T) {
	body := `{
	  "model":"gpt-6-astra","max_tokens":32000,"stream":true,
	  "messages":[{"role":"user","content":[{"type":"text","text":"ping"}]}],
	  "system":[{"type":"text","text":"You are Claude Code."}],
	  "metadata":{"user_id":"abc"},
	  "output_config":{"effort":"medium","format":{"type":"json_schema","schema":{"type":"object"}}},
	  "thinking":{"type":"adaptive","budget_tokens":4096},
	  "context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}
	}`
	request, refusal := decode(t, body)
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	if request.Effort != "medium" {
		t.Fatalf("effort = %q", request.Effort)
	}
	if request.System == nil {
		t.Fatal("system was dropped")
	}
}

// An unknown top-level field is refused, not ignored. Ignoring it would mean acting on a
// request while discarding part of what it asked for.
func TestUnknownTopLevelFieldIsRefusedAndNamed(t *testing.T) {
	_, refusal := decode(t, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],"surprise":1}`)
	if refusal == nil || refusal.Code != CodeRequestFields {
		t.Fatalf("refusal = %v, want REQUEST_FIELDS", refusal)
	}
	if refusal.Field != "surprise" {
		t.Fatalf("Field = %q, want the offending name", refusal.Field)
	}
}

// WIRE01 at the request layer: stream has three distinguishable states and each gets its
// own category, because "you forgot it" and "you asked for non-streaming" are different.
func TestStreamHasThreeDistinctRefusals(t *testing.T) {
	base := `{"model":"m","max_tokens":1,"messages":[{"role":"user","content":"x"}]`
	for name, tc := range map[string]struct{ suffix, code string }{
		"missing": {`}`, CodeStreamMissing},
		"false":   {`,"stream":false}`, CodeStreamFalse},
		"null":    {`,"stream":null}`, CodeStreamInvalid},
		"string":  {`,"stream":"true"}`, CodeStreamInvalid},
		"number":  {`,"stream":1}`, CodeStreamInvalid},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, base+tc.suffix, tc.code) })
	}
}

// Sampling controls the backend does not honour. Accepting and ignoring them would hand
// back output that silently disobeyed the request — including an explicit null, which is a
// present field, not an absent one.
func TestSamplingControlsAreRefusedIncludingNull(t *testing.T) {
	for _, extra := range []string{
		`,"temperature":0`, `,"temperature":null`,
		`,"top_p":0.9`, `,"top_p":null`,
		`,"stop_sequences":[]`, `,"stop_sequences":null`,
	} {
		body := `{"model":"m","max_tokens":1,"stream":true,
		  "messages":[{"role":"user","content":"x"}]` + extra + `}`
		_, refusal := decode(t, body)
		if refusal == nil || refusal.Code != CodeUnsupportedSample {
			t.Errorf("%s: refusal = %v, want UNSUPPORTED_SAMPLING", extra, refusal)
		}
	}
}

func TestMessagesValidation(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":`
	for name, tc := range map[string]struct{ messages, code string }{
		"not an array":    {`{}`, CodeMessagesInvalid},
		"string":          {`"x"`, CodeMessagesInvalid},
		"null":            {`null`, CodeMessagesInvalid},
		"empty":           {`[]`, CodeMessagesEmpty},
		"unknown role":    {`[{"role":"system","content":"x"}]`, CodeMessageRole},
		"missing role":    {`[{"content":"x"}]`, CodeMessageRole},
		"missing content": {`[{"role":"user"}]`, CodeMessageFields},
		"extra member":    {`[{"role":"user","content":"x","extra":1}]`, CodeMessageFields},
		"duplicate role":  {`[{"role":"user","role":"assistant","content":"x"}]`, CodeMessageFields},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, head+tc.messages+`}`, tc.code) })
	}
}

// Content blocks this build has not implemented are named, not skipped. A request that
// carried an image and came back answered as if it had not is a wrong answer.
func TestUnimplementedContentBlocksAreNamed(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":[`
	for name, tc := range map[string]struct{ block, code string }{
		"image":        {`{"type":"image","source":{}}`, CodeUnsupportedContent},
		"document":     {`{"type":"document","source":{}}`, CodeUnsupportedContent},
		"thinking":     {`{"type":"thinking","thinking":"x"}`, CodeUnsupportedContent},
		"tool_use":     {`{"type":"tool_use","id":"t","name":"Read","input":{}}`, CodeToolUseUnsupported},
		"tool_result":  {`{"type":"tool_result","tool_use_id":"t","content":"x"}`, CodeToolUseUnsupported},
		"no type":      {`{"text":"x"}`, CodeUnsupportedContent},
		"text no text": {`{"type":"text"}`, CodeUnsupportedContent},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, head+tc.block+`]}]}`, tc.code) })
	}
}

// tool_choice without tools is still a request for tool use.
func TestToolChoiceAloneIsRefused(t *testing.T) {
	mustRefuse(t, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],"tool_choice":{"type":"auto"}}`, CodeToolUseUnsupported)
}

// A malformed tools array is a malformed request, not an unimplemented capability. They
// have different fixes and must not share a category.
func TestMalformedToolsIsShapeNotCapability(t *testing.T) {
	mustRefuse(t, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],"tools":{}}`, CodeToolsShape)
}

// An empty tools array asks for nothing, so there is nothing to refuse.
func TestEmptyToolsArrayIsAccepted(t *testing.T) {
	request, refusal := decode(t, `{"model":"m","max_tokens":1,"stream":true,
	  "messages":[{"role":"user","content":"x"}],"tools":[]}`)
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	if request.ToolCount != 0 {
		t.Fatalf("ToolCount = %d", request.ToolCount)
	}
}

// WIRE02: max_tokens is read from its literal text. Through float64, 1e100 reads as an
// integer and a value past the safe range rounds into a neighbour.
func TestMaxTokensIsExact(t *testing.T) {
	head := `{"model":"m","stream":true,"messages":[{"role":"user","content":"x"}],"max_tokens":`
	for name, tc := range map[string]struct {
		value  string
		reject bool
		want   int64
	}{
		"ordinary":      {"32000", false, 32000},
		"max safe":      {"9007199254740991", false, 9007199254740991},
		"past max safe": {"9007199254740993", true, 0},
		"zero":          {"0", true, 0},
		"negative":      {"-1", true, 0},
		"float":         {"1.5", true, 0},
		"exponent":      {"1e100", true, 0},
		"string":        {`"1024"`, true, 0},
		"null":          {"null", true, 0},
		"huge":          {"999999999999999999999999", true, 0},
	} {
		t.Run(name, func(t *testing.T) {
			request, refusal := decode(t, head+tc.value+`}`)
			if tc.reject {
				if refusal == nil || refusal.Code != CodeInvalidOutputLimit {
					t.Fatalf("refusal = %v, want INVALID_OUTPUT_LIMIT", refusal)
				}
				return
			}
			if refusal != nil {
				t.Fatalf("refused: %v", refusal)
			}
			if request.MaxTokens != tc.want {
				t.Fatalf("MaxTokens = %d, want %d", request.MaxTokens, tc.want)
			}
		})
	}
}

func TestThinkingValidation(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],"thinking":`
	for name, tc := range map[string]struct{ thinking, code string }{
		"unknown type":   {`{"type":"maximal"}`, CodeThinkingType},
		"missing type":   {`{"budget_tokens":1}`, CodeThinkingType},
		"unknown member": {`{"type":"adaptive","extra":1}`, CodeThinkingFields},
		"zero budget":    {`{"type":"adaptive","budget_tokens":0}`, CodeThinkingBudget},
		"float budget":   {`{"type":"adaptive","budget_tokens":1.5}`, CodeThinkingBudget},
		"not an object":  {`"adaptive"`, CodeThinkingFields},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, head+tc.thinking+`}`, tc.code) })
	}
	for _, kind := range []string{"adaptive", "enabled", "disabled"} {
		if _, refusal := decode(t, head+`{"type":"`+kind+`"}}`); refusal != nil {
			t.Errorf("%s refused: %v", kind, refusal)
		}
	}
}

// Only the one semantic no-op is consumed. Any other edit changes what the model is asked
// to remember, and honouring an unimplemented one would quietly alter the conversation.
func TestOnlyTheNoOpContextEditIsAccepted(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],"context_management":`
	if _, refusal := decode(t, head+`{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}}`); refusal != nil {
		t.Fatalf("the no-op edit was refused: %v", refusal)
	}
	for name, tc := range map[string]struct{ management, code string }{
		"different type": {`{"edits":[{"type":"clear_tool_uses_20250919","keep":"all"}]}`, CodeUnsupportedEdit},
		"different keep": {`{"edits":[{"type":"clear_thinking_20251015","keep":"none"}]}`, CodeUnsupportedEdit},
		"two edits":      {`{"edits":[{"type":"clear_thinking_20251015","keep":"all"},{"type":"x","keep":"all"}]}`, CodeUnsupportedEdit},
		"empty edits":    {`{"edits":[]}`, CodeUnsupportedEdit},
		"unknown member": {`{"edits":[],"extra":1}`, CodeContextFields},
		"missing edits":  {`{}`, CodeContextFields},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, head+tc.management+`}`, tc.code) })
	}
}

func TestOutputConfigValidation(t *testing.T) {
	head := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],"output_config":`
	for name, tc := range map[string]struct{ config, code string }{
		"unknown member":     {`{"effort":"low","extra":1}`, CodeOutputConfigFields},
		"format not object":  {`{"format":"json"}`, CodeOutputFormatShape},
		"format unknown key": {`{"format":{"type":"json_schema","schema":{},"extra":1}}`, CodeOutputFormatFields},
		"format wrong type":  {`{"format":{"type":"text","schema":{}}}`, CodeOutputFormatType},
		"format no schema":   {`{"format":{"type":"json_schema"}}`, CodeOutputFormatSchema},
		"schema not object":  {`{"format":{"type":"json_schema","schema":[]}}`, CodeOutputFormatSchema},
		"bad name":           {`{"format":{"type":"json_schema","schema":{},"name":"has space"}}`, CodeOutputFormatName},
	} {
		t.Run(name, func(t *testing.T) { mustRefuse(t, head+tc.config+`}`, tc.code) })
	}
}

// The schema is carried through unread. A second, weaker validator here would only
// disagree with the one that actually decides.
func TestOutputSchemaIsNotInterpreted(t *testing.T) {
	body := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],
	  "output_config":{"format":{"type":"json_schema","name":"structured_output",
	    "schema":{"type":"object","properties":{"n":{"type":"integer","maximum":9007199254740993}},
	    "$ref":"#/definitions/x","unevaluatedProperties":false}}}}`
	if _, refusal := decode(t, body); refusal != nil {
		t.Fatalf("a schema this bridge does not evaluate was refused: %v", refusal)
	}
}

// WIRE03 at the request layer: a duplicate top-level key means the document has two
// readings, and a decoder would pick the last one silently.
func TestDuplicateAndTrailingAreRefused(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate model":  `{"model":"a","model":"b","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}]}`,
		"duplicate stream": `{"model":"a","stream":false,"stream":true,"max_tokens":1,"messages":[{"role":"user","content":"x"}]}`,
		"trailing object":  textOnly + `{"model":"evil"}`,
		"array body":       `[]`,
		"not json":         `nope`,
	} {
		t.Run(name, func(t *testing.T) {
			_, refusal := decode(t, body)
			if refusal == nil {
				t.Fatal("accepted an ambiguous or malformed body")
			}
			if refusal.Code != CodeRequestFields && refusal.Code != CodeRequestShape {
				t.Fatalf("code = %s", refusal.Code)
			}
		})
	}
}

// A refusal must not carry request content back out. Field names come from the allowlist
// or from this package; values never do.
func TestRefusalDoesNotEchoRequestValues(t *testing.T) {
	const marker = "SENSITIVE-PROMPT-CONTENT"
	body := `{"model":"m","max_tokens":1,"stream":false,
	  "messages":[{"role":"user","content":"` + marker + `"}]}`
	_, refusal := decode(t, body)
	if refusal == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(refusal.Error(), marker) {
		t.Fatalf("refusal echoed request content: %s", refusal.Error())
	}
}

// Fields keeps every member unparsed, so a later package can read one without this one
// having had to guess what it meant.
func TestUnparsedFieldsAreKeptVerbatim(t *testing.T) {
	body := `{"model":"m","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"x"}],
	  "metadata":{"user_id":"u_01ABCdef","big":9007199254740993}}`
	request, refusal := decode(t, body)
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	metadata, presence := fieldOf(request, "metadata")
	if presence != 2 {
		t.Fatalf("metadata presence = %v", presence)
	}
	if !strings.Contains(string(metadata), "9007199254740993") {
		t.Fatalf("metadata was rewritten: %s", metadata)
	}
}

func fieldOf(request *Request, name string) (json.RawMessage, int) {
	value, ok := request.Fields[name]
	if !ok {
		return nil, 0
	}
	if strings.TrimSpace(string(value)) == "null" {
		return value, 1
	}
	return value, 2
}
