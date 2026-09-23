package gateway

import (
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeTerminalReceiptsBindOneExecution(t *testing.T) {
	for _, reason := range []string{"answer", "aborted", "error", "refusal"} {
		t.Run(reason, func(t *testing.T) {
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
			active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Model: "gpt-5.6-luna", Effort: "high"}
			put("active/child-"+binding.ID, active)
			if !g.bindNativeTurn(scope.session, binding.ID) {
				t.Fatal("valid turn rejected")
			}
			d.results.handback(scope.session, binding.ID, "public report")
			end := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "wrong", Reason: reason}
			put("end-"+binding.ID+"-first.json", end)
			g.reconcileNativeResults()
			if d.results.report().Recent[0].State != "running" {
				t.Fatal("wrong turn ended execution")
			}
			end.Turn = "first"
			put("end-"+binding.ID+"-first.json", end)
			g.reconcileNativeResults()
			if reason == "answer" {
				if d.results.report().Recent[0].State != "awaiting_native_stop" {
					t.Fatal("turn alone completed a task")
				}
				d.stopped(binding)
			}
			got := d.results.report().Recent[0]
			if got.NativeEffort != "high" || got.Selection.Model != "gpt-5.6-luna" || got.Selection.Effort != "max" || !got.Selection.ModelProvided || got.Selection.EffortProvided || !got.Selection.PresenceVerified {
				t.Fatalf("native and effective selection conflated: %+v", got)
			}
			want := "result_unavailable"
			if reason == "answer" {
				want = "awaiting_parent"
			} else if reason == "aborted" {
				want = "cancelled"
			}
			if got.State != want || got.EndReason != reason || !got.NativeEndObserved {
				t.Fatalf("terminal receipt: %+v", got)
			}
			if reason != "answer" && d.results.bytes != 0 {
				t.Fatal("partial report retained as final")
			}
			if reason == "aborted" {
				req := &anthropic.Request{}
				d.results.deliver(req, "wrong-session", "")(true)
				if len(req.Messages) != 0 {
					t.Fatal("cross-session cancellation leaked")
				}
				d.results.deliver(req, scope.session, "")(true)
				text := req.Messages[0].Blocks[0].Text
				report := d.results.report()
				if !strings.Contains(text, "was aborted") || strings.Contains(text, "recovery") || report.Recent[0].State != "cancellation_reported" || report.Current["running"] != 0 || report.Totals["cancelled"] != 1 || report.TotalsMeaning != "state_entry_counts" {
					t.Fatalf("cancellation mislabeled: %+v %s", report, text)
				}
			}
			if reason == "error" || reason == "refusal" {
				d.results.entries[binding.ID].RequestFailure = "EMPTY_REPLY"
				req := &anthropic.Request{}
				d.results.deliver(req, scope.session, "")(true)
				text := req.Messages[0].Blocks[0].Text
				if !strings.Contains(text, "EMPTY_REPLY") || !strings.Contains(text, "No completed report was produced") || strings.Contains(text, "after one existing-result recovery") {
					t.Fatal("failure misrepresented", text)
				}
			}
			if !d.results.begin(binding.ID) {
				t.Fatal("resume failed")
			}
			active.Turn = "second"
			put("active/child-"+binding.ID, active)
			if !g.bindNativeTurn(scope.session, binding.ID) {
				t.Fatal("resume turn rejected")
			}
			put("end-"+binding.ID+"-first.json", end)
			g.reconcileNativeResults()
			if d.results.report().Recent[0].State != "running" {
				t.Fatal("old receipt ended resumed child")
			}
			d.results.handback(scope.session, binding.ID, "unfinished")
			g.FinalizeNativeResults()
			got = d.results.report().Recent[0]
			if got.State != "result_unavailable" || got.EndReason != "session_ended_unverified" || d.results.bytes != len("public report") && reason == "answer" || d.results.bytes != 0 && reason != "answer" {
				t.Fatal("unverified finalization", got)
			}
		})
	}
}

func TestMalformedActiveNativeReceiptFailsClosed(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{delegations: d}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	for _, raw := range []string{`{`, `{"session":"proof_session","agent":"proof_child","turn":"../bad","model":"gpt-5.6-luna","effort":"high"}`, `{"session":"proof_session","agent":"proof_child","turn":"good","model":"unknown private value","effort":"high"}`} {
		if err := writeNativeTestFile(filepath.Join(dir, "active/child-"+binding.ID), []byte(raw)); err != nil {
			t.Fatal(err)
		}
		if g.bindNativeTurn(scope.session, binding.ID) {
			t.Fatal("malformed receipt accepted")
		}
	}
}

func TestRejectedContinuationRecordsItsOwnNativeTerminal(t *testing.T) {
	for _, wrongSession := range []bool{false, true} {
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
		put := func(name string, v any) {
			t.Helper()
			raw, _ := json.Marshal(v)
			if err := writeNativeTestFile(filepath.Join(dir, name), raw); err != nil {
				t.Fatal(err)
			}
		}
		e := d.results.entries[binding.ID]
		e.State, e.NativeTurn, e.NativeEndObserved, e.EndReason = "awaiting_children", "old_turn", true, "answer"
		active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "new_turn", Model: "gpt-5.6-luna", Effort: "low"}
		if wrongSession {
			active.Session = "other"
		}
		put("active/child-"+binding.ID, active)
		req := httptest.NewRequest("POST", "/v1/messages", nil)
		req.Header.Set("X-Claude-Code-Session-Id", scope.session)
		req.Header.Set("X-Claude-Code-Agent-Id", binding.ID)
		r := &record{}
		_, release, _ := g.agentSelection(req, &anthropic.Request{Model: active.Model}, r)
		release()
		r.data.Category = "AGENT_SELECTION_UNVERIFIED"
		g.recordFailedAgentRequest(scope.session, binding.ID, r)
		if wrongSession {
			if e.NativeTurn != "old_turn" || e.RequestFailure != "" {
				t.Fatal("foreign receipt mutated lifecycle")
			}
			continue
		}
		e = d.results.entries[binding.ID]
		if e.NativeTurn != "new_turn" || e.NativeEndObserved || e.RequestFailure != "AGENT_SELECTION_UNVERIFIED" {
			t.Fatal("failure bound to old turn")
		}
		put("end-"+binding.ID+"-new_turn.json", nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "new_turn", Reason: "error"})
		g.reconcileNativeResults()
		if !e.NativeEndObserved || e.EndReason != "error" || e.State != "result_unavailable" {
			t.Fatal("terminal failure lost")
		}
	}
}
