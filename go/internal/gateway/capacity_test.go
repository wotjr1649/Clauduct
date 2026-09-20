package gateway

import (
	"fmt"
	"testing"
)

// Every errored tool_result lands here, ordinary Bash and Read failures included, so a long
// session reaches the cap. It had no eviction at all: the 1025th distinct failed call
// refused every later /v1/messages request, and because the same conversation replays on
// each following request the refusal could never clear itself.
func TestToolFailureTrackingSurvivesItsOwnCap(t *testing.T) {
	g := &Gateway{}
	for i := 0; i < maxAgents; i++ {
		if !g.recordToolFailure(ToolFailureRecord{Session: "s", Call: fmt.Sprintf("call_%d", i)}) {
			t.Fatalf("refused at %d, below the cap", i)
		}
	}
	if !g.recordToolFailure(ToolFailureRecord{Session: "s", Call: "call_over_the_cap"}) {
		t.Fatal("the first call past the cap refused the request, and nothing would ever clear it")
	}
	if len(g.toolFailures.seen) > maxAgents {
		t.Fatalf("the map grew past its own bound: %d", len(g.toolFailures.seen))
	}
	if !g.toolFailures.snapshot().CapacityExceeded {
		t.Fatal("reaching the cap stopped being visible")
	}
	// The newest call must still be the one retained, so a repeat of it is not counted twice.
	before := g.toolFailures.snapshot().Total
	g.recordToolFailure(ToolFailureRecord{Session: "s", Call: "call_over_the_cap"})
	if g.toolFailures.snapshot().Total != before {
		t.Fatal("the call just recorded was counted again")
	}
}

// A state whose journal cannot be written is still restorable from the journal it last
// wrote, so it is no less evictable than any other. Excluding it held a slot permanently.
func TestAStateWithAFailedJournalDoesNotHoldItsSlot(t *testing.T) {
	c := &contextGuard{states: map[string]*contextState{}}
	for i := 0; i < maxAgents; i++ {
		c.states[fmt.Sprintf("s|agent_%d", i)] = &contextState{saved: "{}", persistenceError: i == 0}
	}
	evictable := 0
	for _, state := range c.states {
		if !state.busy && state.saved != "" {
			evictable++
		}
	}
	if evictable != maxAgents {
		t.Fatalf("%d of %d states could be reclaimed", evictable, maxAgents)
	}
}
