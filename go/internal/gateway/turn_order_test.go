package gateway

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// beginResult is destructive: it clears NativeTurn, NativeEndObserved and EndReason for an
// awaiting_children parent and drops the body of an awaiting_native_stop child. Everything
// about the receipt that can refuse the request has to be decided before that, because a
// refusal afterwards leaves the request unrun and the evidence needed to recover it gone --
// continuation wants exactly the fields beginResult wiped, and reconcileNativeResults only
// revisits entries that still carry a NativeTurn.
func TestReadingTheActiveTurnChangesNothing(t *testing.T) {
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
	write := func(receipt nativeTurnReceipt) {
		t.Helper()
		raw, _ := json.Marshal(receipt)
		if err := writeNativeTestFile(filepath.Join(dir, "active/child-"+binding.ID), raw); err != nil {
			t.Fatal(err)
		}
	}
	write(nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Model: "gpt-5.6-luna", Effort: "high"})
	if !g.bindNativeTurn(scope.session, binding.ID) {
		t.Fatal("valid turn rejected")
	}
	d.results.mu.Lock()
	e := d.results.entries[binding.ID]
	e.NativeEndObserved, e.EndReason = true, "answer"
	d.results.change(e, "awaiting_children")
	d.results.mu.Unlock()

	// A receipt the gateway will not accept: a model outside the catalogue.
	write(nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "second", Model: "not-a-model", Effort: "high"})
	if _, _, ok := g.readActiveTurn(scope.session, binding.ID); ok {
		t.Fatal("an invalid receipt was accepted")
	}
	d.results.mu.Lock()
	defer d.results.mu.Unlock()
	if e.NativeTurn != "first" || !e.NativeEndObserved || e.EndReason != "answer" || e.State != "awaiting_children" {
		t.Fatalf("the refusal cost the turn its evidence: turn=%q observed=%v reason=%q state=%q",
			e.NativeTurn, e.NativeEndObserved, e.EndReason, e.State)
	}
}
