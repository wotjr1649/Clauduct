package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestAgentResultsRequireBodyAndSuccessfulParentDelivery(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, found, err := d.route(scope, binding.ID, binding); err != nil || !found {
		t.Fatal(err)
	}
	binding.Stop = true
	binding.Result = "Findings: proof. Evidence: checked. Unverified: none."
	d.stopped(binding)
	for _, session := range []string{"wrong", scope.session} {
		req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Child completed"}}}}}
		finish := d.results.deliver(req, session, "")
		if (len(req.Messages) > 1) != (session == scope.session) {
			t.Fatal("cross-session result or lost recovery")
		}
		finish(false)
	}
	if got := d.results.report(); got.Totals["parent_received"] != 0 || got.Totals["awaiting_parent"] != 1 {
		t.Fatal(got)
	}
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "<task-id>" + binding.ID + "</task-id>\n<result>" + binding.Result + "</result>"}}}}}
	d.results.deliver(req, scope.session, "")(true)
	if len(req.Messages) != 2 || strings.Contains(req.Messages[1].Blocks[0].Text, binding.Result) || !strings.Contains(req.Messages[1].Blocks[0].Text, `"agentId":"`+binding.ID+`"`) {
		t.Fatal("available body duplicated or verified identity receipt missing")
	}
	before := len(req.Messages)
	d.results.deliver(req, scope.session, "")(true)
	if len(req.Messages) != before {
		t.Fatal("completed receipt was delivered twice")
	}
	got := d.results.report()
	if got.Totals["parent_received"] != 1 || got.Recent[0].Review != "not_assessed_by_gateway" {
		t.Fatal(got)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), binding.Result) {
		t.Fatal("result leaked into status")
	}
	if d.results.bytes != 0 {
		t.Fatal("body retained after delivery")
	}
}

func TestAgentResultWordInOriginalPromptIsNotACompletedReport(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	binding.Stop, binding.Result = true, "probe"
	d.stopped(binding)
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply with the single word: probe"}}}}}
	d.results.deliver(req, scope.session, "")(true)
	if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Blocks[0].Text, `Report: "probe"`) {
		t.Fatal("original prompt falsely acknowledged as child result")
	}
}

func TestAgentResultExistingTranscriptRecoveredOnceWithoutRerun(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing", false: "missing"}[exists], func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			if _, found, err := d.route(scope, binding.ID, binding); err != nil || !found {
				t.Fatal(err)
			}
			path := filepath.Join(filepath.Dir(binding.TranscriptPath), binding.SessionID, "subagents", "agent-"+binding.ID+".jsonl")
			if exists {
				if err := os.WriteFile(path, []byte(`{"type":"assistant","agentId":"proof_child","sessionId":"proof_session","timestamp":"`+time.Now().UTC().Format(time.RFC3339Nano)+`","message":{"content":[{"type":"tool_use","name":"SubagentHandback","input":{"message":"PUBLIC_COMPLETE_REPORT"}}]}}`+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			binding.Stop = true
			d.stopped(binding)
			// A second completion event cannot cause a second transcript read.
			if err := os.WriteFile(path, []byte("invalid later transcript"), 0600); err != nil {
				t.Fatal(err)
			}
			d.stopped(binding)
			req := &anthropic.Request{}
			d.results.deliver(req, scope.session, "")(true)
			text := req.Messages[0].Blocks[0].Text
			if exists && !strings.Contains(text, "PUBLIC_COMPLETE_REPORT") {
				t.Fatal("existing report not recovered")
			}
			if !exists && !strings.Contains(text, "결과 미확보") {
				t.Fatal("missing report called successful")
			}
			if !d.results.report().Recent[0].Recovered {
				t.Fatal("recovery not recorded")
			}
			if len(d.pending) != 0 {
				t.Fatal("recovery scheduled a delegation")
			}
		})
	}
}

func TestResumedResultDoesNotReplaceUnacknowledgedReport(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	binding.Stop = true
	binding.Result = "first public report"
	d.stopped(binding)
	first := d.results.deliver(&anthropic.Request{}, scope.session, "")
	if !d.results.begin(binding.ID) {
		t.Fatal("resume refused")
	}
	binding.Result = "second public report"
	d.stopped(binding)
	first(true)
	req := &anthropic.Request{}
	second := d.results.deliver(req, scope.session, "")
	if len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Blocks[0].Text, "second public report") {
		t.Fatal("acknowledging old report erased new report")
	}
	second(true)
	if d.results.report().Totals["parent_received"] != 2 || d.results.bytes != 0 {
		t.Fatal("result accounting mismatch")
	}
}

func TestMissingResumedResultNeverRecoversAnOlderAnswer(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(binding.TranscriptPath), binding.SessionID, "subagents", "agent-"+binding.ID+".jsonl")
	old := `{"type":"assistant","agentId":"proof_child","sessionId":"proof_session","timestamp":"2000-01-01T00:00:00Z","message":{"content":[{"type":"text","text":"STALE_PUBLIC_REPORT"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	binding.Stop = true
	d.stopped(binding)
	if d.results.report().Recent[0].State != "result_unavailable" {
		t.Fatal("older result substituted for missing current result")
	}
}

func TestNativeStopAndStreamCompletionOrderingDoesNotLoseAnswer(t *testing.T) {
	for _, stopFirst := range []bool{true, false} {
		for _, delivered := range []bool{true, false} {
			t.Run(fmt.Sprintf("stopFirst=%v/delivered=%v", stopFirst, delivered), func(t *testing.T) {
				d, scope, binding := preparedDelegation(t)
				if _, _, err := d.route(scope, binding.ID, binding); err != nil {
					t.Fatal(err)
				}
				finish := d.beginAnswer(scope.session, binding.ID)
				if stopFirst {
					d.stopped(binding)
				}
				// No transcript exists, exactly as when native's disk flush lags.
				finish("PUBLIC_DELIVERED_REPORT", delivered)
				if !stopFirst {
					d.stopped(binding)
				}
				req := &anthropic.Request{}
				d.results.deliver(req, scope.session, "")(true)
				got := d.results.report().Recent[0]
				if delivered {
					if got.State != "parent_received" || got.Source != "delivered_response" || got.Recovered || !strings.Contains(req.Messages[0].Blocks[0].Text, "PUBLIC_DELIVERED_REPORT") {
						t.Fatalf("lost answer: %+v", got)
					}
				} else if got.State != "unavailable_reported" || strings.Contains(req.Messages[0].Blocks[0].Text, "PUBLIC_DELIVERED_REPORT") {
					t.Fatalf("failed delivery accepted: %+v", got)
				}
				if d.results.bytes != 0 || len(d.pending) != 0 {
					t.Fatal("retained answer or reran work")
				}
			})
		}
	}
}

func TestAuxiliaryRequestCannotRestartOrReplaceChildResult(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	g := startWith(t, &upstream.Fixture{SSE: sse(created, delta("title"), done("title"), completed, "[DONE]")})
	g.delegations = d
	if _, err := g.agents.register(binding, time.Now()); err != nil {
		t.Fatal(err)
	}
	e := d.results.entries[binding.ID]
	d.results.body(e, "PUBLIC_PENDING_REPORT", "delivered_response")
	d.results.change(e, "awaiting_native_stop")
	rq := messages(strings.NewReader(strings.Replace(validRequest, "gpt-6-astra", "gpt-5.6-luna", 1)))
	rq.headers["X-Claude-Code-Request-Class"] = "auxiliary"
	rq.headers["X-Claude-Code-Session-Id"] = scope.session
	rq.headers["X-Claude-Code-Agent-Id"] = binding.ID
	resp := do(t, g, rq)
	bodyText(t, resp)
	if resp.StatusCode != 200 || d.results.entries[binding.ID] != e || e.State != "awaiting_native_stop" || e.body != "PUBLIC_PENDING_REPORT" || len(d.results.entries) != 1 {
		t.Fatal("auxiliary turn changed delegated result")
	}
}

func TestOldStreamCompletionCannotSettleResumedResult(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(fmt.Sprint(delivered), func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			if _, _, err := d.route(scope, binding.ID, binding); err != nil {
				t.Fatal(err)
			}
			r := &d.results
			first := r.entries[binding.ID]
			first.NativeTurn = "first"
			finish := d.beginAnswer(scope.session, binding.ID)
			binding.Result = "OLD_PUBLIC_STOP"
			d.stopped(binding)
			if first.stopBinding == nil {
				t.Fatal("stop was not deferred until stream completion")
			}
			r.change(first, "awaiting_children")
			if !r.begin(binding.ID) {
				t.Fatal("resume refused")
			}
			next := r.entries[binding.ID]
			next.NativeTurn = "second"
			r.body(next, "NEW_PUBLIC_REPORT", "native_handback")
			before, beforeBytes := next.AgentResultRecord, r.bytes
			finish("OLD_PUBLIC_STREAM", delivered)
			if r.entries[binding.ID] != next || next.stopped || next.AgentResultRecord != before || next.body != "NEW_PUBLIC_REPORT" || r.bytes != beforeBytes {
				t.Fatal("old stream completed the resumed result or changed its accounting")
			}
			d.stopped(binding)
			parent := &anthropic.Request{}
			r.deliver(parent, scope.session, "")(true)
			if len(parent.Messages) != 1 || !strings.Contains(parent.Messages[0].Blocks[0].Text, "NEW_PUBLIC_REPORT") || strings.Contains(parent.Messages[0].Blocks[0].Text, "OLD_PUBLIC") || r.bytes != 0 {
				t.Fatal("new result was not delivered exactly once")
			}
		})
	}
}

func TestStreamCompletionRechecksResultAfterWaitingForStop(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	r := &d.results
	first := r.entries[binding.ID]
	finish := d.beginAnswer(scope.session, binding.ID)
	binding.Result = "OLD_PUBLIC_STOP"
	d.stopped(binding)
	if first.stopBinding == nil {
		t.Fatal("stop was not deferred")
	}
	// Pause the stop after completion released results.mu, then resume the child.
	d.mu.Lock()
	locked := true
	done := make(chan struct{})
	defer func() {
		if locked {
			d.mu.Unlock()
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("completion did not finish")
		}
	}()
	go func() { finish("OLD_PUBLIC_STREAM", true); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		r.mu.Lock()
		waiting := !first.streaming
		r.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("completion did not reach the stop boundary")
		}
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	r.change(first, "awaiting_children")
	r.mu.Unlock()
	if !r.begin(binding.ID) {
		t.Fatal("resume refused")
	}
	d.mu.Unlock()
	locked = false
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not finish")
	}
	if next := r.entries[binding.ID]; next == first || next.stopped || next.State != "running" || next.body != "" || r.bytes != 0 {
		t.Fatal("a stop that waited for the lock settled the new result")
	}
}
