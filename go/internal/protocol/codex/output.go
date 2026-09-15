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
	// ID is the backend's handle for this item. It is what ties the streaming argument
	// events to the item they belong to, and what makes a repeated item detectable.
	ID        string
	Type      string
	CallID    string
	Name      string
	Arguments json.RawMessage
	Text      []string
}

// OutputItemEvent is one response.output_item.added or response.output_item.done payload.
//
// These are where a response's items actually come from. Measured 2026-09-15: this
// backend's response.completed carries an empty output array, on tool and text requests
// alike, so a build that reads items only from the completion reads nothing at all.
type OutputItemEvent struct {
	Index int
	Item  OutputItem
}

// DecodeOutputItem reads an output_item.added or output_item.done payload.
//
// final says which. An item being opened has an identity but need not yet have the content
// it is about to be given -- a tool call's arguments stream in afterwards, so demanding
// them in the added snapshot would refuse the ordinary case. A closing item is the
// backend's finished account and must be complete. The Node baseline draws the same line.
func DecodeOutputItem(raw []byte, final bool) (OutputItemEvent, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return OutputItemEvent{}, ErrEventShape
	}
	var event OutputItemEvent
	if err := intField(fields, "output_index", &event.Index); err != nil {
		return OutputItemEvent{}, err
	}
	value, presence := wire.Of(fields, "item")
	if presence != wire.Present {
		return OutputItemEvent{}, ErrEventShape
	}
	item, err := decodeItem(value, final)
	if err != nil {
		return OutputItemEvent{}, err
	}
	event.Item = item
	return event, nil
}

// DecodeCompletedOutput reads the output array from a response.completed payload.
//
// It is no longer where items come from. Measured 2026-09-15 against the real backend: this
// array is empty on every response, tool and text alike, so building from it built nothing.
// Items come from the output_item events; this stays as a cross-check, because a backend
// that starts filling the array and disagrees with its own stream is worth stopping for.
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

func decodeOutputItem(raw json.RawMessage) (OutputItem, error) { return decodeItem(raw, true) }

func decodeItem(raw json.RawMessage, final bool) (OutputItem, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return OutputItem{}, ErrEventShape
	}
	var item OutputItem
	if value, presence := wire.Of(fields, "id"); presence == wire.Present {
		if json.Unmarshal(value, &item.ID) != nil {
			return OutputItem{}, ErrEventShape
		}
	}
	if err := stringField(fields, "type", &item.Type); err != nil {
		return OutputItem{}, err
	}

	switch item.Type {
	case ItemFunctionCall:
		if err := optionalString(fields, "call_id", &item.CallID, final); err != nil {
			return OutputItem{}, err
		}
		if err := optionalString(fields, "name", &item.Name, final); err != nil {
			return OutputItem{}, err
		}
		// The backend sends arguments as a JSON string containing JSON. It is taken as
		// the string it is and parsed by the caller, which is where a parse failure
		// becomes a refusal rather than a half-built call.
		value, presence := wire.Of(fields, "arguments")
		if presence != wire.Present {
			if final {
				return OutputItem{}, ErrEventShape
			}
			break
		}
		var encoded string
		if json.Unmarshal(value, &encoded) != nil {
			return OutputItem{}, ErrEventShape
		}
		item.Arguments = json.RawMessage(encoded)

	case ItemMessage:
		content, presence := wire.Of(fields, "content")
		if presence != wire.Present {
			if final {
				return OutputItem{}, ErrEventShape
			}
			break
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

// optionalString reads a field that a closing item must carry and an opening one need not.
// An absent field leaves the destination empty; a present one of the wrong type is still a
// shape error, because tolerating an opening snapshot is not tolerating nonsense in it.
func optionalString(fields map[string]json.RawMessage, name string, into *string, required bool) error {
	if _, presence := wire.Of(fields, name); presence != wire.Present {
		if required {
			return ErrEventShape
		}
		return nil
	}
	return stringField(fields, name, into)
}
