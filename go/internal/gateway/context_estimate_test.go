package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestMediaEstimateNeverCountsBase64AsText(t *testing.T) {
	r := &bridge.Request{Model: "gpt-6-astra", Instruction: bridge.Instruction, Input: []bridge.InputEntry{{Type: "function_call_output", Output: []bridge.InputPart{{Type: "input_image", ImageURL: "small"}}}}}
	small, opaque, _ := estimateTextInput(r)
	r.Input[0].Output[0].ImageURL = strings.Repeat("A", 2<<20)
	large, _, _ := estimateTextInput(r)
	if !opaque || small != large || bridge.BackendCountSupported(r) {
		t.Fatal("media bytes became a token count or unsupported counter was enabled")
	}
	g, f := newContextFixture(t)
	raw, _ := json.Marshal(r)
	if _, _, _, err := g.countInput(context.Background(), r, raw, r.Model); err == nil || f.counts.Load() != 0 {
		t.Fatal("unverified tool media reached exact counter")
	}
}

func TestUsageMismatchDoesNotAbortAnswerAndRepairsExactInputCache(t *testing.T) {
	f := &countingFixture{Fixture: upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}}
	g := startWith(t, f)
	g.EnableContextPolicy()
	bodyText(t, countPost(t, g, "gpt-6-astra", "ping")) // Deliberate 76 versus backend 5.
	r := messages(strings.NewReader(validRequest))
	r.headers["X-Claude-Code-Session-Id"] = "public-session"
	response := do(t, g, r)
	body := bodyText(t, response)
	if response.StatusCode != 200 || !strings.Contains(body, "message_stop") || strings.Contains(body, "event: error") {
		t.Fatal("optional counter invalidated valid generation")
	}
	d := status(t, g)
	var measured RequestRecord
	for _, r := range d.Recent {
		if r.Kind == "generation" {
			measured = r
		}
	}
	if measured.CountAgreement != "mismatched" || measured.UsageSource != "backend" || *measured.InputTokens != 5 || d.Totals.Failures["COUNT_INPUT_MISMATCH"] != 0 {
		t.Fatal("mismatch was hidden or promoted to an API failure", measured)
	}
	response = countPost(t, g, "gpt-6-astra", "ping")
	if b := bodyText(t, response); response.StatusCode != 200 || b != `{"input_tokens":5}` || f.counts != 1 {
		t.Fatal("exact same input did not reuse measured usage", b)
	}
}

func TestMissingUsageIsNotZero(t *testing.T) {
	g, f := newContextFixture(t)
	f.Err = upstream.Failure{Category: "PUBLIC_ABORT"}
	response := contextRequest(t, g, "gpt-6-astra", "public")
	bodyText(t, response)
	d := status(t, g)
	if d.AgentContexts[0].LastUsage != nil || d.Totals.MeasuredRequests != 0 || d.Totals.UnmeasuredRequests != 1 {
		t.Fatal("unknown usage reported as measured")
	}
}

func TestAuxiliaryAndOtherSessionDoNotBorrowConversationUsage(t *testing.T) {
	g, f := newContextFixture(t)
	f.tokens = 450001
	bodyText(t, contextRequest(t, g, "gpt-6-astra", "public"))
	f.tokens = 100
	for _, scope := range []struct{ session, class string }{{"public-session", "auxiliary"}, {"different-session", "main"}} {
		r := messages(strings.NewReader(validRequest))
		r.headers["X-Claude-Code-Session-Id"] = scope.session
		r.headers["X-Claude-Code-Request-Class"] = scope.class
		response := do(t, g, r)
		bodyText(t, response)
		if response.StatusCode != 200 {
			t.Fatal("independent request borrowed parent usage")
		}
	}
	for _, report := range g.agentContexts() {
		if report.SessionID == "public-session" && report.LastUsage.Input != 450001 {
			t.Fatal("auxiliary overwrote conversation usage")
		}
	}
}

func TestBackendOverflowRecoveryIsBoundedAndKeepsPriorModel(t *testing.T) {
	g, f := newContextFixture(t)
	bodyText(t, contextRequest(t, g, "gpt-6-astra", "public"))
	f.Err = upstream.Failure{Category: "CONTEXT_LENGTH_EXCEEDED", Status: 400}
	r := contextRequest(t, g, "gpt-5.6-sol", "public")
	body := bodyText(t, r)
	if r.StatusCode != 400 || !strings.Contains(body, "prompt is too long") {
		t.Fatal("confirmed overflow did not request compaction")
	}
	f.Err = nil
	ticket := compactEvent(t, g, "PreCompact")
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1)))
	if g.agentContexts()[0].Model != "gpt-6-astra" {
		t.Fatal("overflow discarded prior model")
	}
	f.Err = upstream.Failure{Category: "CONTEXT_LENGTH_EXCEEDED", Status: 400}
	r = contextRequest(t, g, "gpt-5.6-sol", "summary")
	body = bodyText(t, r)
	if r.StatusCode != 400 || strings.Contains(body, "prompt is too long") || !strings.Contains(body, "CONTEXT_COMPACTION_INSUFFICIENT") {
		t.Fatal("overflow recovery looped")
	}
	calls := f.Calls()
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "again"))
	if f.Calls() != calls {
		t.Fatal("failed operation replayed")
	}
}
