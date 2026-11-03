package bridge

import (
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"strings"
)

type reasoningPartKey struct {
	summary bool
	index   int
}
type reasoningPart struct {
	text                        strings.Builder
	started, textDone, partDone bool
}

func (t *Translator) acceptReasoning(event stream.Event) error {
	e, err := codex.DecodeReasoningEvent(event.Type, event.Raw)
	if err == codex.ErrResponseIncomplt {
		t.incomplete = true
		return nil
	}
	if err != nil {
		return err
	}
	h := t.held[e.OutputIndex]
	if h == nil || h.done || h.item.ID != e.ItemID || h.item.Type != codex.ItemReasoning || e.PartIndex >= maxOutputItems {
		return codex.ErrEventShape
	}
	if h.reasoning == nil {
		h.reasoning = map[reasoningPartKey]*reasoningPart{}
	}
	key := reasoningPartKey{e.Summary, e.PartIndex}
	p := h.reasoning[key]
	if event.Type == codex.ReasoningSummaryAdd {
		if p != nil || e.Text != "" {
			return ErrItemSnapshotMismatch
		}
		h.reasoning[key] = &reasoningPart{}
		return nil
	}
	if p == nil {
		if e.Summary {
			return codex.ErrEventShape
		}
		p = &reasoningPart{}
		h.reasoning[key] = p
	}
	if p.partDone {
		return ErrItemSnapshotMismatch
	}
	switch event.Type {
	case codex.ReasoningSummaryTxtD, codex.ReasoningTextDelta:
		if p.textDone {
			return ErrItemSnapshotMismatch
		}
		if len(e.Text) > stream.DefaultLimits().MaxResponseBytes-p.text.Len() {
			return anthropic.ErrResponseTooLarge
		}
		p.started = true
		p.text.WriteString(e.Text)
	case codex.ReasoningSummaryTxtF, codex.ReasoningTextDone:
		if p.textDone || p.text.String() != e.Text {
			return ErrItemSnapshotMismatch
		}
		p.textDone = true
	case codex.ReasoningSummaryDone:
		if p.text.String() != e.Text || p.started && !p.textDone {
			return ErrItemSnapshotMismatch
		}
		p.partDone = true
	}
	return nil
}

func (h *heldItem) checkReasoning(final codex.OutputItem) error {
	// Populated opening snapshots carry content too, even if no delta follows.
	for i, part := range h.item.Summary {
		if i >= len(final.Summary) || !strings.HasPrefix(final.Summary[i].Text, part.Text) {
			return ErrItemSnapshotMismatch
		}
	}
	for i, text := range h.item.ReasoningText {
		if i >= len(final.ReasoningText) || !strings.HasPrefix(final.ReasoningText[i], text) {
			return ErrItemSnapshotMismatch
		}
	}
	// The backend refreshes encrypted snapshots while reasoning progresses.
	// Preserve the final opaque value; absence is loss, different ciphertext is not.
	if h.item.Encrypted != "" && final.Encrypted == "" {
		return ErrItemSnapshotMismatch
	}
	for key, p := range h.reasoning {
		if key.summary {
			if !p.partDone || key.index >= len(final.Summary) || final.Summary[key.index].Text != p.text.String() {
				return ErrItemSnapshotMismatch
			}
		} else if !p.textDone || key.index >= len(final.ReasoningText) || final.ReasoningText[key.index] != p.text.String() {
			return ErrItemSnapshotMismatch
		}
	}
	return nil
}
