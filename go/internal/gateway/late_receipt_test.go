package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// A receipt settles a turn whose outcome is still open. Once the report has reached the
// parent there is nothing left to settle, and applying one anyway undid the delivery: the
// user presses Esc after the child already answered, native writes an aborted receipt for
// that turn, and the next parent turn was told no completed report was expected from a
// child whose report it was already holding.
func TestALateAbortReceiptDoesNotUndoADeliveredReport(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{delegations: d, agents: newAgentRegistry()}
	if _, err := g.agents.register(binding, time.Now()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	put := func(name string, receipt nativeTurnReceipt) {
		t.Helper()
		raw, _ := json.Marshal(receipt)
		if err := writeNativeTestFile(filepath.Join(dir, name), raw); err != nil {
			t.Fatal(err)
		}
	}
	put("active/child-"+binding.ID, nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Model: "gpt-5.6-luna", Effort: "high"})
	if !g.bindNativeTurn(scope.session, binding.ID) {
		t.Fatal("valid turn rejected")
	}
	d.results.handback(scope.session, binding.ID, "public report")

	// The parent's turn takes the report.
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "collect"}}}}}
	d.results.mu.Lock()
	e := d.results.entries[binding.ID]
	e.stopped = true
	d.results.change(e, "awaiting_parent")
	d.results.mu.Unlock()
	d.results.deliver(req, scope.session, "")(true)
	if got := d.results.report().Recent[0].State; got != "parent_received" {
		t.Fatalf("delivery did not settle: %s", got)
	}

	// Esc lands after the report was already handed over.
	put("end-"+binding.ID+"-first.json", nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Reason: "aborted"})
	g.reconcileNativeResults()
	if got := d.results.report().Recent[0].State; got != "parent_received" {
		t.Fatalf("a delivered report was reverted to %s", got)
	}
	next := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "continue"}}}}}
	evidence := &ParentReadiness{}
	d.results.deliver(next, scope.session, "", evidence)(false)
	if len(next.Messages) != 1 {
		t.Fatal("a cancellation notice was injected for a report the parent already holds")
	}
	if len(evidence.Unavailable) != 0 {
		t.Fatalf("delivered report counted unavailable: %v", evidence.Unavailable)
	}
}

// A journal this build cannot write is a local, permanent condition. Classified as an
// upstream failure it answered 502, which the measured client retries eight times in sixty
// seconds; the sibling path meeting the identical failure one write earlier answers 400.
func TestAnUnwritableParentDecisionIsALocalRefusal(t *testing.T) {
	if got := categoryFor(errParentWaitUnverified); got != "PARENT_WAIT_UNVERIFIED" {
		t.Fatalf("category %q", got)
	}
	if got := statusForUpstream(errParentWaitUnverified); got != http.StatusBadRequest {
		t.Fatalf("status %d, so the client retries a condition no retry can clear", got)
	}
}

// Identical payloads share one count. The owner's cancellation is its own request's, and
// handing it to a waiter answered COUNT_TOKENS_FAILED_CANCELLED for a request nobody
// cancelled -- the handler's own ctx.Err() is nil there, so nothing catches it.
func TestAnOwnersCancellationIsNotSharedWithItsWaiters(t *testing.T) {
	raw := []byte(`{"model":"gpt-6-astra","messages":[]}`)
	built := &bridge.Request{Model: "gpt-6-astra"}
	g := &Gateway{}
	key := sha256.Sum256(raw)
	flight := &countFlight{done: make(chan struct{})}
	g.counts.pending = map[[32]byte]*countFlight{key: flight}
	waiter := make(chan error, 1)
	go func() {
		_, _, _, err := g.countInput(context.Background(), built, raw, "gpt-6-astra")
		waiter <- err
	}()
	time.Sleep(20 * time.Millisecond)
	// The owner's request goes away. Its flight leaves c.pending first, exactly as the
	// owner's own defer does it.
	g.counts.mu.Lock()
	delete(g.counts.pending, key)
	flight.err = context.Canceled
	close(flight.done)
	g.counts.mu.Unlock()
	select {
	case err := <-waiter:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("a waiter inherited the owner's cancellation")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waiter never returned")
	}
}
