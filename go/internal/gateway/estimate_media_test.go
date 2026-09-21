package gateway

import (
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Opaque and Media answer different questions. Encrypted reasoning is opaque and this build's
// own replies carry it, so Opaque is set on nearly every turn of an ordinary conversation; a
// counter built on it would measure "there was a conversation". Only Media isolates input the
// estimate could not see whose size is why a length refusal arrived.
func TestReasoningIsOpaqueButIsNotMedia(t *testing.T) {
	reasoning := &bridge.Request{Input: []bridge.InputEntry{{Reasoning: &bridge.Reasoning{}}}}
	if _, opaque, media := estimateTextInput(reasoning); !opaque || media {
		t.Fatalf("reasoning: opaque=%v media=%v", opaque, media)
	}
	image := &bridge.Request{Input: []bridge.InputEntry{{Content: []bridge.InputPart{{Type: "input_image"}}}}}
	if _, opaque, media := estimateTextInput(image); !opaque || !media {
		t.Fatalf("image: opaque=%v media=%v", opaque, media)
	}
	text := &bridge.Request{Input: []bridge.InputEntry{{Content: []bridge.InputPart{{Type: "input_text", Text: "hello"}}}}}
	if _, opaque, media := estimateTextInput(text); opaque || media {
		t.Fatalf("text: opaque=%v media=%v", opaque, media)
	}
	// The line this exists for: a part kind outside the named set is unestimated, so opaque,
	// but it is not media and must not reach the counter. Inferring media from "not text"
	// made every future part kind media by default, which is the defect the named set
	// removed -- and nothing failed when it was inferred, because no test used one.
	unknown := &bridge.Request{Input: []bridge.InputEntry{{Content: []bridge.InputPart{{Type: "input_audio"}}}}}
	if _, opaque, media := estimateTextInput(unknown); !opaque || media {
		t.Fatalf("an unnamed part kind: opaque=%v media=%v", opaque, media)
	}
}

// A journal whose content is already on disk has nothing unwritten, so the flag that says
// otherwise has to come off. Returning early without clearing it latched a state as
// unwritable for the life of the process: the retry that exists to clear it reaches that
// early return and never gets to the line that clears.
func TestAJournalThatMatchesDiskClearsTheUnwritableFlag(t *testing.T) {
	g := &Gateway{delegations: &delegations{projects: t.TempDir()}}
	g.EnableContextPolicy()
	state := &contextState{journal: "proj/s.clauduct-context.json", identity: "s|", route: bridge.Route{Model: "gpt-6-astra", Effort: "low", Source: "catalogue"}}
	if err := g.saveContext(state); err != nil {
		t.Fatal(err)
	}
	state.persistenceError = true
	if err := g.saveContext(state); err != nil {
		t.Fatal(err)
	}
	if state.persistenceError {
		t.Fatal("a state whose journal is already on disk stayed latched as unwritable")
	}
}

// The counter reads Media, not Opaque. Reading Opaque made it a duplicate of the control
// count, because this build's own replies carry encrypted reasoning and the client replays
// it: an ordinary conversation sets Opaque on nearly every request. Reasoning-only overflows
// are deliberately not counted here; they are a compaction control with no media on it.
func TestTheOverflowCounterReadsMediaNotOpaque(t *testing.T) {
	overflow := func(opaque, media bool) int64 {
		r := newRing()
		r.count(RequestRecord{
			Model:           "gpt-6-astra",
			Control:         "BACKEND_CONTEXT_COMPACTION_REQUIRED",
			ContextEstimate: &ContextEstimate{Input: 40000, Source: "text_estimate", Opaque: opaque, Media: media},
		})
		return r.contextUsage["gpt-6-astra"].MediaOverflows
	}
	if n := overflow(true, false); n != 0 {
		t.Fatalf("an overflow on reasoning-only opacity counted %d; the window is media", n)
	}
	if n := overflow(true, true); n != 1 {
		t.Fatalf("an overflow carrying media counted %d", n)
	}
}
