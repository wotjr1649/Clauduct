package gateway

import (
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"testing"
)

func TestInterimParentTurnNeverBecomesFinalNestedReport(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	child := resolvedChoice{session: scope.session, parent: binding.ID, call: "nested_call"}
	if !d.results.start("nested_child", child) {
		t.Fatal("child start")
	}
	binding.Result = "INTERIM_NOT_A_FINAL_REPORT"
	d.stopped(binding)
	if got := d.results.entries[binding.ID]; got.stopped || got.State != "awaiting_children" || got.body != "" {
		t.Fatal("premature completion")
	}
	parentRequest := &anthropic.Request{}
	d.results.deliver(parentRequest, scope.session, "")(true)
	if len(parentRequest.Messages) != 0 {
		t.Fatal("interim report leaked to parent")
	}
	nested := d.results.entries["nested_child"]
	d.results.mu.Lock()
	nested.stopped = true
	d.results.body(nested, "CHILD_FINAL", "native_stop")
	d.results.change(nested, "awaiting_parent")
	d.results.mu.Unlock()
	// Its completed child's body arrives in the delegated parent's next turn.
	if !d.results.begin(binding.ID) {
		t.Fatal("parent continuation")
	}
	d.results.deliver(&anthropic.Request{}, scope.session, binding.ID)(true)
	binding.Result = "FINAL_SELF_CONTAINED_REPORT"
	d.stopped(binding)
	parentRequest = &anthropic.Request{}
	d.results.deliver(parentRequest, scope.session, "")(true)
	if len(parentRequest.Messages) != 1 || d.results.entries[binding.ID].Bytes != len(binding.Result) || d.results.report().Totals["awaiting_parent"] != 2 {
		t.Fatal("final result omitted or interim counted")
	}
}
