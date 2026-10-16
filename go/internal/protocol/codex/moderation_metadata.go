package codex

import "github.com/wotjr1649/Clauduct/go/internal/wire"

// ModerationMetadata recognises only the first-party presentation envelope observed on
// 2026-10-16. OpenAI codex-api/src/common.rs and sse/responses.rs carry its value as opaque
// TurnModerationMetadata, separately from safety buffering, routing and terminal outcomes.
// The presentation payload has no Anthropic equivalent; validate its container without
// retaining or forwarding its fields. Other metadata remains unsupported.
func ModerationMetadata(raw []byte) (bool, error) {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return false, ErrEventShape
	}
	metadata, err := wire.Fields(fields["metadata"], nil)
	if err != nil {
		return false, nil
	}
	value, present := metadata["openai_chatgpt_moderation_metadata"]
	if !present {
		return false, nil
	}
	if len(metadata) != 1 {
		return false, ErrEventShape
	}
	if _, err := wire.Fields(raw, []string{"type", "metadata", "sequence_number", "response_id"}); err != nil {
		return false, ErrEventShape
	}
	var kind string
	if stringField(fields, "type", &kind) != nil || kind != MetadataEvent {
		return false, ErrEventShape
	}
	if _, present := fields["response_id"]; present {
		var id string
		if stringField(fields, "response_id", &id) != nil || id == "" {
			return false, ErrEventShape
		}
	}
	if _, present := fields["sequence_number"]; present {
		var sequence int
		if intField(fields, "sequence_number", &sequence) != nil || sequence < 0 {
			return false, ErrEventShape
		}
	}
	if _, err := wire.Fields(value, nil); err != nil {
		return false, ErrEventShape
	}
	return true, nil
}
