package codex

import (
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var ErrContextLimit = errors.New("CONTEXT_LENGTH_EXCEEDED")

// Inspect only a structured code. Wording, generic 400s and transport errors
// never authorize compaction or replay. Duplicate fields are rejected.
func ContextLimit(raw []byte) bool {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return false
	}
	if response := fields["response"]; response != nil {
		fields, err = wire.Fields(response, nil)
		if err != nil {
			return false
		}
	}
	failure, err := wire.Fields(fields["error"], nil)
	if err != nil {
		return false
	}
	var code string
	return json.Unmarshal(failure["code"], &code) == nil && code == "context_length_exceeded"
}
