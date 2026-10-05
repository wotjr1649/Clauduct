package codex

import "github.com/wotjr1649/Clauduct/go/internal/wire"

// ReasoningEvent carries content even when it produces no native display frame.
// Private reasoning and its final snapshot must agree before a response succeeds.
type ReasoningEvent struct {
	ItemID                 string
	OutputIndex, PartIndex int
	Summary                bool
	Text                   string
}

func DecodeReasoningEvent(kind string, raw []byte) (ReasoningEvent, error) {
	var out ReasoningEvent
	index, content := "summary_index", "text"
	out.Summary = true
	switch kind {
	case ReasoningSummaryAdd, ReasoningSummaryDone:
		content = "part"
	case ReasoningSummaryTxtD:
		content = "delta"
	case ReasoningSummaryTxtF:
	case ReasoningTextDelta:
		index, content, out.Summary = "content_index", "delta", false
	case ReasoningTextDone:
		index, out.Summary = "content_index", false
	default:
		return out, ErrUnsupportedEvent
	}
	allowed := []string{"type", "item_id", "output_index", index, content, "sequence_number"}
	if content == "delta" {
		allowed = append(allowed, "obfuscation")
	}
	if kind == ReasoningSummaryDone {
		allowed = append(allowed, "status")
	}
	fields, err := wire.Fields(raw, allowed)
	if err != nil {
		return out, ErrEventShape
	}
	var actual string
	if stringField(fields, "type", &actual) != nil || actual != kind || stringField(fields, "item_id", &out.ItemID) != nil || out.ItemID == "" || intField(fields, "output_index", &out.OutputIndex) != nil || out.OutputIndex < 0 || intField(fields, index, &out.PartIndex) != nil || out.PartIndex < 0 {
		return out, ErrEventShape
	}
	if _, present := fields["sequence_number"]; present {
		var sequence int
		if intField(fields, "sequence_number", &sequence) != nil || sequence < 0 {
			return out, ErrEventShape
		}
	}
	// Official stream padding mitigates payload-size side channels. Its bytes
	// carry no reasoning semantic; validate the field without retaining them.
	if _, present := fields["obfuscation"]; present {
		var padding string
		if stringField(fields, "obfuscation", &padding) != nil {
			return out, ErrEventShape
		}
	}
	if _, present := fields["status"]; present {
		var status string
		if stringField(fields, "status", &status) != nil || status != "incomplete" {
			return out, ErrEventShape
		}
		return out, ErrResponseIncomplt
	}
	if content == "part" {
		out.Text, err = decodeTextPart(fields[content], "summary_text")
	} else {
		err = stringField(fields, content, &out.Text)
	}
	return out, err
}
