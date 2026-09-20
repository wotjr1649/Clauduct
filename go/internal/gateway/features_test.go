package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFeatureEvidenceRequiresChecksOnSameRequest(t *testing.T) {
	r := newRing()
	for _, check := range []string{"input", "route", "context_policy"} {
		e := r.open("POST", "/v1/messages")
		e.checked(check)
		e.finish()
	}
	if r.featureReport()[0].Observed != 0 {
		t.Fatal("different requests fabricated one complete proof")
	}
	for range 20 {
		e := r.open("POST", "/v1/messages")
		for _, check := range []string{"input", "route", "context_policy"} {
			e.checked(check)
			e.checked(check)
		}
		e.checked("untrusted private value")
		e.finish()
	}
	failed := r.open("POST", "/v1/messages")
	for _, check := range []string{"input", "route", "context_policy"} {
		failed.checked(check)
	}
	failed.refusedWith(502, "UPSTREAM_FAILURE")
	failed.finish()
	e := r.featureReport()[0]
	if e.Observed != 21 || e.Completed != 20 || e.Failed != 1 || e.Acceptance != "not_assessed" {
		t.Fatalf("incorrect evidence: %+v", e)
	}
	if len(r.recent()) != 16 || len(r.recent()[0].VerifiedChecks) != 3 {
		t.Fatal("ring limit or bounded checks changed")
	}
}

func TestFeatureEvidenceDoesNotTreatAuxiliaryAsNativeTurn(t *testing.T) {
	r := RequestRecord{Kind: "generation", AgentID: "agent", AgentRole: "workflow-subagent", RequestClass: "auxiliary", VerifiedChecks: []string{"continuation"}}
	for _, name := range []string{"native_agent", "workflow_agent", "agent_resume"} {
		if featureApplies(name, r) {
			t.Fatalf("auxiliary falsely classified as %s", name)
		}
	}
	if !featureApplies("generation", r) {
		t.Fatal("auxiliary generation disappeared")
	}
}

func TestFeatureEvidenceRetainsRejectedRecovery(t *testing.T) {
	r := newRing()
	e := r.open("POST", "/v1/messages")
	e.checked("workflow_recovery_request")
	e.refusedWith(400, "WORKFLOW_RECOVERY_UNVERIFIED")
	e.finish()
	for _, f := range r.featureReport() {
		if f.Name == "workflow_result_reuse" && (f.Requests != 1 || f.Unconfirmed != 1 || f.Observed != 0) {
			t.Fatal("rejected attempt disappeared", f)
		}
	}
}

func TestFeatureEvidenceSeparatesRejectedToolAndOutOfOrderCompletion(t *testing.T) {
	r := newRing()
	first, second := r.open("POST", "/v1/messages"), r.open("POST", "/v1/messages")
	second.checked("workflow_recovery_request")
	second.rejectedWorkflow()
	second.finish()
	first.finish()
	for _, f := range r.featureReport() {
		if f.Name == "generation" && (f.LastRequest != 2 || f.LastCompletedRequest != 1 || f.RejectedToolCalls != 1) {
			t.Fatal(f)
		}
		if f.Name == "workflow_result_reuse" && (f.Requests != 1 || f.Unconfirmed != 1 || f.Completed != 0 || f.RejectedToolCalls != 1) {
			t.Fatal("denial became successful recovery", f)
		}
	}
	if r.counts().RejectedWorkflowCalls != 1 || len(r.counts().Failures) != 0 {
		t.Fatal("tool denial confused with API failure")
	}
}

func TestNativeProgressRejectsForeignAndIncompleteReceipts(t *testing.T) {
	g := &Gateway{}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	put := func(name string, value any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("active-main.json", nativeTurnReceipt{Session: "session", Turn: "current"})
	if out := g.nativeProgressReport(); out.Unreadable != 1 || len(out.Agents) != 0 {
		t.Fatal("missing progress became idle proof")
	}
	p := NativeProgress{Session: "session", Turn: "current", Phase: "tool_pending", Signal: "permission_requested", Sequence: 3, At: time.Now().Add(-time.Hour).UnixMilli(), PendingTools: 1, PermissionRequests: 1}
	// Wire receipts contain no derived age. Unknown fields cannot enter status.
	wire := map[string]any{"session": p.Session, "agent": "", "turn": p.Turn, "phase": p.Phase, "signal": p.Signal, "sequence": p.Sequence, "at": p.At, "pendingTools": p.PendingTools, "permissionRequests": p.PermissionRequests}
	put("progress-root.json", wire)
	out := g.nativeProgressReport()
	if len(out.Agents) != 1 || out.Agents[0].AgeMs < 3500000 || out.State != "observed_events_not_liveness_proof" {
		t.Fatalf("old event became termination or liveness proof: %+v", out)
	}
	wire["turn"] = "old"
	put("progress-root.json", wire)
	if out = g.nativeProgressReport(); len(out.Agents) != 0 || out.Unreadable != 1 {
		t.Fatal("old turn accepted")
	}
	wire["turn"] = "current"
	wire["session"] = "foreign"
	put("progress-root.json", wire)
	if len(g.nativeProgressReport().Agents) != 0 {
		t.Fatal("foreign session accepted")
	}
	wire["session"] = "session"
	wire["command"] = "must never be retained"
	put("progress-root.json", wire)
	if len(g.nativeProgressReport().Agents) != 0 {
		t.Fatal("private extra field accepted")
	}
}

func TestConfiguredNativeTurnMustExistBeforeDispatch(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	g := &Gateway{delegations: d}
	g.ConfigureNativeEvents(t.TempDir())
	if g.bindNativeTurn(scope.session, binding.ID) {
		t.Fatal("missing required receipt accepted")
	}
}

func TestBackendProgressCountsArgumentsWithoutPayload(t *testing.T) {
	r := newRing().open("POST", "/v1/messages")
	r.backendEvent("response.function_call_arguments.delta")
	r.backendEvent("response.output_text.delta")
	r.backendEvent("unknown private event")
	p := r.snapshot().BackendProgress
	if p.Events != 3 || p.ToolArgumentDeltas != 1 || p.TextDeltas != 1 {
		t.Fatal(p)
	}
}
