package anthropic

import (
	"encoding/json"
	"strings"
)

// ResponseMessage collects our validated frames for an explicit non-streaming request.
// It never interprets backend events or releases a partial tool call.
type ResponseMessage struct {
	fields map[string]json.RawMessage
	// Pointers, not values. strings.Builder records the address it was first written at and
	// panics if it is later used from a different one, and append moves every element when it
	// grows. A delta for block 0 that arrives after block 1 opened -- which the ordering
	// guards below accept, since start never requires the previous block closed and delta
	// never requires the newest index -- then wrote through a Builder that had been copied,
	// and the process took a panic where every other malformed shape returns ErrStreamOrder.
	// go vet's copylocks does not see strings.Builder and the race detector cannot either.
	blocks          []*messageBlock
	bytes           int
	finished, delta bool
}

type messageBlock struct {
	fields map[string]json.RawMessage
	data   strings.Builder
	field  string
	closed bool
}

func (m *ResponseMessage) Add(frames []Frame) error {
	for _, frame := range frames {
		if m.finished {
			return ErrStreamOrder
		}
		var e struct {
			Message      map[string]json.RawMessage
			ContentBlock map[string]json.RawMessage `json:"content_block"`
			Delta        map[string]json.RawMessage
			Usage        map[string]json.RawMessage
			Index        int
		}
		if json.Unmarshal(frame.Data, &e) != nil {
			return ErrStreamOrder
		}
		switch frame.Type {
		case "message_start":
			if m.fields != nil || e.Message == nil {
				return ErrStreamOrder
			}
			m.fields = e.Message
		case "content_block_start":
			if m.fields == nil || e.Index != len(m.blocks) || e.ContentBlock == nil || m.delta {
				return ErrStreamOrder
			}
			if len(m.blocks) >= maxTextParts {
				return ErrResponseTooLarge
			}
			m.blocks = append(m.blocks, &messageBlock{fields: e.ContentBlock})
			m.bytes += len(frame.Data)
		case "content_block_delta", "content_block_stop":
			if e.Index < 0 || e.Index >= len(m.blocks) || m.blocks[e.Index].closed || m.delta {
				return ErrStreamOrder
			}
			b := m.blocks[e.Index]
			if frame.Type == "content_block_stop" {
				b.closed = true
				break
			}
			var kind, value, field string
			if json.Unmarshal(e.Delta["type"], &kind) != nil {
				return ErrStreamOrder
			}
			switch kind {
			case "text_delta":
				field = "text"
			case "input_json_delta":
				field = "partial_json"
			default:
				return ErrStreamOrder
			}
			if json.Unmarshal(e.Delta[field], &value) != nil || b.field != "" && b.field != field {
				return ErrStreamOrder
			}
			b.field = field
			b.data.WriteString(value)
			m.bytes += len(value)
		case "message_delta":
			if m.fields == nil || m.delta || e.Delta == nil || e.Usage == nil {
				return ErrStreamOrder
			}
			for _, b := range m.blocks {
				if !b.closed {
					return ErrStreamOrder
				}
			}
			for key, value := range e.Delta {
				m.fields[key] = value
			}
			// Initial usage is provisional. Unknown final counts stay unknown.
			m.fields["usage"], _ = json.Marshal(e.Usage)
			m.delta = true
		case "message_stop":
			if !m.delta {
				return ErrStreamOrder
			}
			m.finished = true
		default:
			return ErrStreamOrder
		}
		if m.bytes > maxResponseBytes {
			return ErrResponseTooLarge
		}
	}
	return nil
}

func (m *ResponseMessage) JSON() ([]byte, error) {
	if !m.finished {
		return nil, ErrStreamOrder
	}
	content := make([]map[string]json.RawMessage, 0, len(m.blocks))
	for _, b := range m.blocks {
		switch b.field {
		case "text":
			b.fields["text"], _ = json.Marshal(b.data.String())
		case "partial_json":
			raw := []byte(b.data.String())
			if !isJSONObject(raw) {
				return nil, ErrInvalidToolCall
			}
			b.fields["input"] = raw
		}
		content = append(content, b.fields)
	}
	m.fields["content"], _ = json.Marshal(content)
	raw, err := json.Marshal(m.fields)
	if len(raw) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	return raw, err
}
