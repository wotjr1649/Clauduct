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
		g.recordToolFailure(ToolFailureRecord{Session: "s", Call: fmt.Sprintf("call_%d", i)})
	}
	g.recordToolFailure(ToolFailureRecord{Session: "s", Call: "call_over_the_cap"})
	if g.toolFailures.snapshot().Total != int64(maxAgents)+1 {
		t.Fatalf("the call past the cap was dropped instead of counted: total=%d", g.toolFailures.snapshot().Total)
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

// Two properties of a full choice table.
//
// A refusal leaves nothing behind. results.start installs the entry as running, so refusing
// after it would leave an entry with no resolved choice -- stoppedTurn returns early without
// one, so it never stops, never reports, and both eviction loops skip anything not reported.
// Today start's own cap refuses first and the leak is unreachable (a full resolved table of
// live agents means r.entries is full too), so this half of the assertion does not
// discriminate; the ordering is what keeps it unreachable if either cap ever moves.
//
// Re-caching an already resolved agent must not evict. That one is reachable and was: the
// map does not grow for an id it already holds, so the eviction dropped an unrelated agent
// for nothing. prepareResume has carried this guard all along.
func TestAFullChoiceTableRefusesCleanly(t *testing.T) {
	d := &delegations{resolved: map[string]resolvedChoice{}}
	live := resolvedChoice{session: "s", parent: "p", call: "c"}
	for i := 0; i < maxAgents; i++ {
		id := fmt.Sprintf("agent_%d", i)
		d.resolved[id] = live
		if !d.results.start(id, live) {
			t.Fatalf("setup refused at %d", i)
		}
	}
	entries := len(d.results.entries)
	if err := d.cacheChoice("agent_new", live); err == nil {
		t.Fatal("a full table of running agents accepted another")
	}
	if _, held := d.results.entries["agent_new"]; held {
		t.Fatal("the refusal left a result entry nothing can ever reclaim")
	}
	if len(d.results.entries) != entries {
		t.Fatalf("result entries changed on a refusal: %d -> %d", entries, len(d.results.entries))
	}
	// Re-caching an agent already resolved adds nothing to the map, so it must not evict.
	if err := d.cacheChoice("agent_0", live); err != nil {
		t.Fatalf("re-caching a resolved agent was refused: %v", err)
	}
	if len(d.resolved) != maxAgents {
		t.Fatalf("re-caching evicted an unrelated agent: %d", len(d.resolved))
	}
}
