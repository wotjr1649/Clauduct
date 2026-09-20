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
		name, session                  string
		eligible, pending, want, error bool
	}{
		{"pending", scope.session, true, true, true, false},
		{"explicit input", scope.session, false, true, false, false},
		{"completed", scope.session, true, false, false, false},
		{"foreign session", "wrong", true, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry.data.ParentReadiness.Pending = nil
			if tc.pending {
				entry.data.ParentReadiness.Pending = []string{"public-child"}
			}
			raw, _ := json.Marshal(parentStep{Session: tc.session, Turn: "native-turn", Index: 1, Eligible: tc.eligible, Mode: "native_tui"})
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
			}
		})
	}
}
