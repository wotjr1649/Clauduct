package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func TestParentReadinessIsTheDeliveredInputSnapshot(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	_, prepared := d.deliverWithReadiness(&anthropic.Request{}, scope.session, "")
	if prepared.Eligible || len(prepared.Pending) != 1 {
		t.Fatal("unstarted prepared child was omitted")
	}
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	finish, waiting := d.deliverWithReadiness(&anthropic.Request{}, scope.session, "")
	if waiting.Eligible || len(waiting.Pending) != 1 {
		t.Fatal("running child was omitted")
	}
	binding.Stop = true
	binding.Result = "PUBLIC_FINAL_REPORT"
	d.stopped(binding)
	finish(true)
	if waiting.Eligible || len(waiting.Included) != 0 {
		t.Fatal("late completion changed previous request evidence")
	}
	request := &anthropic.Request{}
	ack, ready := d.deliverWithReadiness(request, scope.session, "")
	if !ready.Eligible || len(ready.Included) != 1 || len(ready.Pending) != 0 || len(request.Messages) != 1 {
		t.Fatal("completed report admission lost")
	}
	ack(true)
	_, again := d.deliverWithReadiness(&anthropic.Request{}, scope.session, "")
	if !again.Eligible || len(again.Included) != 0 {
		t.Fatal("already acknowledged result redelivered")
	}
}

func TestParentWaitNeedsMatchingNativeStepAndPendingChild(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	g := &Gateway{delegations: d}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-Claude-Code-Session-Id", scope.session)
	req.Header.Set("X-Claude-Code-Request-Class", "main")
	entry := &record{data: RequestRecord{ParentReadiness: &ParentReadiness{Pending: []string{"public-child"}}}}
	request := &anthropic.Request{}
	for _, tc := range []struct {
		name, session, mode            string
		eligible, pending, want, error bool
	}{
		{"pending", scope.session, "native_tui", true, true, true, false},
		{"explicit input", scope.session, "native_tui", false, true, false, false},
		{"completed", scope.session, "native_tui", true, false, false, false},
		{"foreign session", "wrong", "native_tui", true, true, false, true},
		{"SDK pending", scope.session, "sdk", true, true, true, false},
		{"SDK explicit input", scope.session, "sdk", false, true, false, false},
		{"SDK completed", scope.session, "sdk", true, false, false, false},
		{"unclassified wait", scope.session, "unclassified", true, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry.data.ParentReadiness.Pending = nil
			if tc.pending {
				entry.data.ParentReadiness.Pending = []string{"public-child"}
			}
			raw, _ := json.Marshal(parentStep{Session: tc.session, Turn: "native-turn", Index: 1, Eligible: tc.eligible, Mode: tc.mode})
			if err := os.WriteFile(filepath.Join(dir, "step-root.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			step, err := g.prepareParentWait(req, request, entry)
			if (err != nil) != tc.error || (step != nil) != tc.want {
				t.Fatalf("step=%v err=%v", step, err)
			}
			if tc.error {
				return
			}
			var decision struct {
				Turn  string
				Index int
				Hold  bool
			}
			raw, err = os.ReadFile(filepath.Join(dir, "decision-root.json"))
			if err != nil {
				t.Fatal(err)
			}
			if json.Unmarshal(raw, &decision) != nil || decision.Hold {
				t.Fatal("admission withheld a response before completion")
			}
			if step != nil {
				if err := g.writeParentDecision(step, true); err != nil {
					t.Fatal(err)
				}
				raw, err = os.ReadFile(filepath.Join(dir, "decision-root.json"))
				var held struct{ Hold, Wait bool }
				if err != nil || json.Unmarshal(raw, &held) != nil || !held.Hold || !held.Wait {
					t.Fatal("pending child wait lost")
				}
			}
		})
	}
}

func TestOnlyVerifiedRootNotificationAllowsEmptyWithoutPendingChildren(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	g := &Gateway{delegations: d}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	for _, tc := range []struct {
		name, agent    string
		index          int
		eligible, want bool
	}{
		{"notification", "", 0, true, true},
		{"explicit input", "", 0, false, false},
		{"ordinary later step", "", 1, true, false},
		{"unverified child wake", "child", 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "root"
			if tc.agent != "" {
				name = "child-" + tc.agent
			}
			putCancellationReceipt(t, dir, "step-"+name+".json", parentStep{Session: scope.session, Agent: tc.agent, Turn: "public-turn", Index: tc.index, Eligible: tc.eligible, Mode: "sdk"})
			r := httptest.NewRequest("POST", "/v1/messages", nil)
			r.Header.Set("X-Claude-Code-Session-Id", scope.session)
			r.Header.Set("X-Claude-Code-Agent-Id", tc.agent)
			r.Header.Set("X-Claude-Code-Request-Class", "main")
			e := &record{data: RequestRecord{ParentReadiness: &ParentReadiness{Eligible: true}}}
			step, err := g.prepareParentWait(r, &anthropic.Request{}, e)
			if err != nil || (step != nil) != tc.want {
				t.Fatal("notification boundary", step, err)
			}
			if step != nil {
				if err := g.writeParentDecision(step, true); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(filepath.Join(dir, "decision-"+name+".json"))
				var held struct{ Hold, Wait bool }
				if err != nil || json.Unmarshal(raw, &held) != nil || !held.Hold || held.Wait {
					t.Fatal("completed notification created a pending child")
				}
			}
		})
	}
}

func TestWorkflowLaunchWaitPrecedesItsFirstChild(t *testing.T) {
	d, scope, _, link := workflowProof(t)
	g := &Gateway{delegations: d}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	putCancellationReceipt(t, dir, "step-root.json", parentStep{Session: scope.session, Turn: "public-launch", Index: 2, Eligible: true, Mode: "native_tui"})
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	r.Header.Set("X-Claude-Code-Session-Id", scope.session)
	r.Header.Set("X-Claude-Code-Request-Class", "main")
	request := &anthropic.Request{Messages: []anthropic.Message{{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", Name: "Workflow", ID: link.Call}}}, {Role: "user", Blocks: []anthropic.Block{{Type: "tool_result", ToolUseID: link.Call}}}, {Role: "system", Blocks: []anthropic.Block{{Type: "text", Text: "public native reminder"}}}}}
	for _, linked := range []bool{false, true} {
		if linked {
			if err := d.linkWorkflow(link); err != nil {
				t.Fatal(err)
			}
		}
		if !d.returnedWorkflowLaunch(request, scope.session, "") {
			t.Fatal("verified launch lost before first child")
		}
		entry := &record{data: RequestRecord{ParentReadiness: &ParentReadiness{Eligible: true}}}
		step, err := g.prepareParentWait(r, request, entry)
		if err != nil || step == nil || step.waiting {
			t.Fatal("Workflow control fabricated a pending child", step, err)
		}
		if d.returnedWorkflowLaunch(request, "foreign", "") || d.returnedWorkflowLaunch(request, scope.session, "child") {
			t.Fatal("foreign launch accepted")
		}
		request.Messages[1].Blocks[0].IsError = true
		if d.returnedWorkflowLaunch(request, scope.session, "") {
			t.Fatal("failed launch accepted")
		}
		request.Messages[1].Blocks[0].IsError = false
	}
	request.Messages = append(request.Messages, anthropic.Message{Role: "assistant", Blocks: []anthropic.Block{{Type: "text", Text: "completed answer"}}})
	if d.returnedWorkflowLaunch(request, scope.session, "") {
		t.Fatal("old tool result admitted a new input")
	}
}

func TestWithheldChildIsPendingRatherThanMissingACompletedResult(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	if !d.results.start("grandchild", resolvedChoice{session: scope.session, parent: binding.ID, call: "nested"}) {
		t.Fatal("child start")
	}
	finish := d.beginAnswer(scope.session, binding.ID)
	finish("", true) // Successful control delivery is not native task completion.
	d.stopped(binding)
	request := &anthropic.Request{}
	_, ready := d.deliverWithReadiness(request, scope.session, "")
	if len(ready.Pending) != 1 || len(ready.Unavailable) != 0 || len(request.Messages) != 0 || d.results.entries[binding.ID].State != "awaiting_children" {
		t.Fatal("waiting child was promoted to unavailable/completed", ready)
	}
}
