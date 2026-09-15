package codex

import (
	"encoding/json"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Output item kinds this build reads from a completed response.
const (
	ItemFunctionCall = "function_call"
	ItemMessage      = "message"
	ItemReasoning    = "reasoning"
	ItemWebSearch    = "web_search_call"
)

// OutputItem is one entry of a completed response's output array.
//
// Arguments stays as the bytes that arrived. They are a tool call's input, and re-encoding
// them would collapse an omitted optional, an explicit null and a chosen value into fewer
// readings than the model wrote.
type OutputItem struct {
	Type      string
	CallID    string
	Name      string
	Arguments json.RawMessage
	Text      []string
}

// DecodeCompletedOutput reads the output array from a response.completed payload.
//
// This is where tool calls come from, and that is the delivery barrier rather than an
// implementation detail. Nothing tool-shaped is built from the streaming
// function_call_arguments events: a call exists only once the backend has said the
// response completed, so a stream that fails midway cannot have handed the client
// something to execute.
//
// present is false when the payload carries no output at all, which is not an error — the
// caller decides whether a response with nothing in it is acceptable.
func DecodeCompletedOutput(raw []byte) (items []OutputItem, present bool, err error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, false, ErrEventShape
	}
	value, presence := wire.Of(fields, "response")
	if presence != wire.Present {
		return nil, false, nil
	}
	response, err := wire.Fields(value, nil)
	if err != nil {
		return nil, false, ErrEventShape
	}
	outputValue, presence := wire.Of(response, "output")
	if presence != wire.Present {
		return nil, false, nil
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(outputValue, &entries); err != nil {
		return nil, false, ErrEventShape
	}
	items = make([]OutputItem, 0, len(entries))
	for _, entry := range entries {
		item, err := decodeOutputItem(entry)
		if err != nil {
			return nil, true, err
		}
		items = append(items, item)
	}
	return items, true, nil
}

func decodeOutputItem(raw json.RawMessage) (OutputItem, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return OutputItem{}, ErrEventShape
	}
	var item OutputItem
	if err := stringField(fields, "type", &item.Type); err != nil {
		return OutputItem{}, err
	}

	switch item.Type {
	case ItemFunctionCall:
		if err := stringField(fields, "call_id", &item.CallID); err != nil {
			return OutputItem{}, err
		}
		if err := stringField(fields, "name", &item.Name); err != nil {
			return OutputItem{}, err
		}
		// The backend sends arguments as a JSON string containing JSON. It is taken as
		// the string it is and parsed by the caller, which is where a parse failure
		// becomes a refusal rather than a half-built call.
		value, presence := wire.Of(fields, "arguments")
		if presence != wire.Present {
			return OutputItem{}, ErrEventShape
		}
		var encoded string
		if json.Unmarshal(value, &encoded) != nil {
			return OutputItem{}, ErrEventShape
		}
		item.Arguments = json.RawMessage(encoded)

	case ItemMessage:
		content, presence := wire.Of(fields, "content")
		if presence != wire.Present {
			return OutputItem{}, ErrEventShape
		}
		var parts []json.RawMessage
		if json.Unmarshal(content, &parts) != nil {
			return OutputItem{}, ErrEventShape
		}
		for _, part := range parts {
			partFields, err := wire.Fields(part, nil)
			if err != nil {
				return OutputItem{}, ErrEventShape
			}
			var text string
			if err := stringField(partFields, "text", &text); err != nil {
				return OutputItem{}, err
			}
			item.Text = append(item.Text, text)
		}
	}
	return item, nil
}
