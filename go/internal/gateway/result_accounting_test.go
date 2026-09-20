package gateway

import (
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// entry installs one result in a state, without driving the whole native path to reach it.
// These are accounting defects: what matters is which bucket a state lands in, not how the
// state was arrived at.
func entry(r *agentResults, key, agent, state string, stopped bool, body string) *agentResult {
	if r.entries == nil {
		r.entries = map[string]*agentResult{}
	}
	r.sequence++
	e := &agentResult{
		AgentResultRecord: AgentResultRecord{Agent: agent, Session: "s", State: state, Review: "not_assessed_by_gateway", Bytes: len(body)},
		parent:            "p", body: body, stopped: stopped, sequence: r.sequence,
	}
	r.entries[key] = e
	r.bytes += len(body)
	return e
}

func readiness(t *testing.T, r *agentResults) (*ParentReadiness, *anthropic.Request) {
	t.Helper()
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "next turn"}}}}}
	evidence := &ParentReadiness{}
	r.deliver(req, "s", "p", evidence)(false)
	return evidence, req
}

// A workflow child whose journal has not been read yet is stopped but still owes a report.
// It used to fall out of Pending (which required !stopped) and out of Unavailable (whose
// whitelist it is not in), so Eligible -- the AND of both being empty -- said the parent
// may finish while the report was still outstanding.
func TestAWorkflowChildAwaitingItsJournalHoldsCompletion(t *testing.T) {
	r := &agentResults{}
	entry(r, "child", "child", "awaiting_workflow_result", true, "")
	evidence, _ := readiness(t, r)
	if len(evidence.Pending) != 1 || evidence.Pending[0] != "child" {
		t.Fatalf("pending %v", evidence.Pending)
	}
	if evidence.Eligible {
		t.Fatal("completion declared eligible with a report still outstanding")
	}
}

// restored_evidence is historical metadata and owes nothing, so it neither holds the parent
// nor keeps its slot. Both loops that reclaim slots ask resultReported, and it was in
// neither that set nor any state they could reach.
func TestRestoredEvidenceOwesNothingAndCanBeReclaimed(t *testing.T) {
	r := &agentResults{}
	entry(r, "old", "old", "restored_evidence", true, "")
	evidence, _ := readiness(t, r)
	if len(evidence.Pending) != 0 || !evidence.Eligible {
		t.Fatalf("historical metadata held the parent: %+v", evidence)
	}
	if !resultReported("restored_evidence") {
		t.Fatal("eviction cannot reclaim a restored_evidence slot")
	}
}

// begin() archives a superseded entry under id+"/"+sequence. Delivery walked every key, so
// the previous run's undelivered report was handed to the parent beside the current one,
// under the same agent label and with a second identical receipt.
func TestASupersededRunsReportIsNotDeliveredBesideTheCurrentOne(t *testing.T) {
	r := &agentResults{}
	entry(r, "child/1", "child", "awaiting_parent", true, "stale findings from run one")
	entry(r, "child", "child", "awaiting_parent", true, "current findings from run two")
	evidence, req := readiness(t, r)
	text := ""
	for _, m := range req.Messages[1:] {
		for _, b := range m.Blocks {
			text += b.Text
		}
	}
	if strings.Contains(text, "stale findings from run one") {
		t.Fatal("a superseded run's report reached the parent")
	}
	if !strings.Contains(text, "current findings from run two") {
		t.Fatal("the current report was lost")
	}
	if len(evidence.Included) != 1 {
		t.Fatalf("one agent reported twice: %v", evidence.Included)
	}
}

// The delivery loop and the pending tally have to agree on which states can be handed over.
// They are two reads of one predicate now; this fails if a state is added to one only.
func TestDeliverableStatesAreExactlyTheDeliveredOnes(t *testing.T) {
	for _, state := range []string{"awaiting_parent", "result_unavailable", "cancelled"} {
		if !deliverable(state) {
			t.Fatalf("%s is delivered but not deliverable", state)
		}
	}
	for _, state := range []string{"running", "awaiting_children", "awaiting_native_stop", "awaiting_workflow_result"} {
		if deliverable(state) {
			t.Fatalf("%s owes a report but counts as deliverable", state)
		}
	}
}
