package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"net/http"
	"strings"
	"testing"
	"time"
)

func countPost(t *testing.T, g *Gateway, model, text string) *http.Response {
	t.Helper()
	content, _ := json.Marshal(map[string]any{"model": model, "messages": []any{map[string]string{"role": "user", "content": text}}})
	rq := messages(strings.NewReader(string(content)))
	rq.path = "/v1/messages/count_tokens"
	return do(t, g, rq)
}

func TestAuxiliaryCompactionTextHasSameGenerationAndCountClassification(t *testing.T) {
	g, fixture := newContextFixture(t)
	if code, body := effortRequest(t, g, "gpt-5.6-luna", "high", compactPrompt(), "auxiliary", false); code != 200 {
		t.Fatalf("generation: %d %s", code, body)
	}
	if fixture.counts.Load() != 0 {
		t.Fatal("ordinary generation added a preflight count")
	}
	if code, body := effortRequest(t, g, "gpt-5.6-luna", "high", compactPrompt(), "auxiliary", true); code != 200 {
		t.Fatalf("optional count: %d %s", code, body)
	}
	waitForActive(t, g, 0, "count diagnostic must finish")
	if fixture.Calls() != 1 || fixture.counts.Load() != 0 {
		t.Fatalf("measured usage not reused: generation=%d count=%d", fixture.Calls(), fixture.counts.Load())
	}
	for _, record := range g.Snapshot().Recent {
		if record.Path == "/v1/messages/count_tokens" && (record.RequestClass != "auxiliary" || record.CountMethod != "backend-usage" || record.CountedInputTokens == nil || *record.CountedInputTokens != fixture.tokens) {
			t.Fatal("count class or actual usage lost")
		}
	}
}

func TestCountTokensSupportsValidatedTextWithoutBackendRequests(t *testing.T) {
	f := &upstream.Fixture{}
	g := startWith(t, f)
	for _, model := range []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		rq := messages(strings.NewReader(`{"model":"` + model + `","messages":[{"role":"user","content":"Reply OK."}]}`))
		rq.path = "/v1/messages/count_tokens"
		response := do(t, g, rq)
		body := bodyText(t, response)
		var got struct {
			InputTokens int64 `json:"input_tokens"`
		}
		if response.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &got) != nil || got.InputTokens != 21 {
			t.Fatalf("count: %d %s", response.StatusCode, body)
		}
	}
	if f.Calls() != 0 {
		t.Fatal("local counting used an inference")
	}
}

type countingFixture struct {
	upstream.Fixture
	countErr  error
	countCall upstream.Call
	counts    int
}

func TestCountTokensAcceptsInterruptedTextWithEmptyCitations(t *testing.T) {
	f := &countingFixture{}
	g := startWith(t, f)
	rq := messages(strings.NewReader(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"public task"},{"role":"assistant","content":[{"type":"text","text":"partial public answer","citations":null}]},{"role":"user","content":"continue"}]}`))
	rq.path = "/v1/messages/count_tokens"
	response := do(t, g, rq)
	body := bodyText(t, response)
	if response.StatusCode != http.StatusOK || f.counts != 1 || f.Calls() != 0 || !strings.Contains(string(f.countCall.Body), "partial public answer") {
		t.Fatalf("interrupted count: status=%d counts=%d generations=%d body=%s", response.StatusCode, f.counts, f.Calls(), body)
	}
}

func (f *countingFixture) Count(_ context.Context, call upstream.Call) (int64, error) {
	f.counts++
	f.countCall = call
	return 76, f.countErr
}

func TestBackendCountUsesTheVerifiedChildSelectionAndDoesNotGenerate(t *testing.T) {
	d, scope, b := preparedDelegation(t)
	f := &countingFixture{}
	g := startWith(t, f)
	g.delegations = d
	if _, err := g.agents.register(b, time.Now()); err != nil {
		t.Fatal(err)
	}
	rq := messages(strings.NewReader(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"Reply OK."}],"tools":[{"name":"sample","input_schema":{"type":"object","properties":{}}}]}`))
	rq.path = "/v1/messages/count_tokens"
	rq.headers["X-Claude-Code-Agent-Id"] = b.ID
	rq.headers["X-Claude-Code-Session-Id"] = scope.session
	response := do(t, g, rq)
	body := bodyText(t, response)
	if response.StatusCode != 200 || body != `{"input_tokens":76}` || f.Calls() != 0 || f.counts != 1 {
		t.Fatalf("status=%d body=%s calls=%d counts=%d", response.StatusCode, body, f.Calls(), f.counts)
	}
	if f.countCall.Model != "gpt-5.6-luna" || f.countCall.Effort != "max" {
		t.Fatal("count route differs from verified child")
	}
	diagnostics := g.Diagnose()
	if len(diagnostics.Recent) != 1 || diagnostics.Recent[0].CountedInputTokens == nil || *diagnostics.Recent[0].CountedInputTokens != 76 {
		t.Fatal("count result not observable")
	}
}

func TestChildCountAndGenerationUseTheSameToolContract(t *testing.T) {
	d, scope, child := preparedDelegation(t)
	f := &countingFixture{Fixture: upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}}
	g := startWith(t, f)
	g.delegations = d
	if _, err := g.agents.register(child, time.Now()); err != nil {
		t.Fatal(err)
	}
	base := `"model":"gpt-5.6-luna","messages":[{"role":"user","content":"PUBLIC"}],"tools":[{"name":"Agent","input_schema":{"type":"object","properties":{"model":{"type":"string","enum":["opus","sonnet","haiku"]}}}}]`
	for _, count := range []bool{true, false} {
		body := "{" + base + "}"
		if !count {
			body = `{"stream":true,"max_tokens":128,` + base + "}"
		}
		rq := messages(strings.NewReader(body))
		if count {
			rq.path = "/v1/messages/count_tokens"
		}
		rq.headers["X-Claude-Code-Agent-Id"] = child.ID
		rq.headers["X-Claude-Code-Session-Id"] = scope.session
		rq.headers["X-Claude-Code-Request-Class"] = "auxiliary"
		response := do(t, g, rq)
		bodyText(t, response)
		if response.StatusCode != 200 {
			t.Fatal("request refused", count, response.StatusCode)
		}
	}
	var counted, generated map[string]json.RawMessage
	if json.Unmarshal(f.countCall.Body, &counted) != nil || json.Unmarshal([]byte(f.LastRequest()), &generated) != nil {
		t.Fatal("missing backend payload")
	}
	if string(counted["tools"]) != string(generated["tools"]) || string(counted["input"]) != string(generated["input"]) {
		t.Fatal("counted payload differs from generated input/tool contract")
	}
}

func TestBackendCountFailureDoesNotPoisonGeneration(t *testing.T) {
	f := &countingFixture{Fixture: upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}, countErr: errors.New("private provider error must not appear")}
	g := startWith(t, f)
	rq := messages(strings.NewReader(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"Reply OK."}],"tools":[{"name":"sample","input_schema":{"type":"object","properties":{}}}]}`))
	rq.path = "/v1/messages/count_tokens"
	response := do(t, g, rq)
	body := bodyText(t, response)
	if response.StatusCode != 400 || !strings.Contains(body, "COUNT_TOKENS_FAILED_") || strings.Contains(body, "private") {
		t.Fatal("count failure was hidden or leaked provider text")
	}
	response = post(t, g, validRequest)
	bodyText(t, response)
	if response.StatusCode != 200 || f.Calls() != 1 || f.counts != 1 {
		t.Fatal("failed counting retried or contaminated generation")
	}
}

func TestCountBudgetRefusalsKeepTheirCategory(t *testing.T) {
	for _, failure := range []error{upstream.ErrBudgetExhausted, upstream.ErrRouteNotAuthorised} {
		t.Run(failure.Error(), func(t *testing.T) {
			f := &countingFixture{countErr: failure}
			g := startWith(t, f)
			response := countPost(t, g, "gpt-5.6-luna", "public count")
			body := bodyText(t, response)
			category := "COUNT_TOKENS_FAILED_" + failure.Error()
			if response.StatusCode != 400 || !strings.Contains(body, category) || f.counts != 1 || f.Calls() != 0 || g.Diagnose().Totals.Failures[category] != 1 {
				t.Fatalf("status=%d counts=%d generations=%d category=%s", response.StatusCode, f.counts, f.Calls(), body)
			}
		})
	}
}

func TestCountTokensUnsupportedScopeLeavesGenerationWorking(t *testing.T) {
	f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, f)
	rq := messages(strings.NewReader(`{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"Reply OK."}],"tools":[{"name":"example","input_schema":{"type":"object","properties":{}}}]}`))
	rq.path = "/v1/messages/count_tokens"
	response := do(t, g, rq)
	if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), "COUNT_TOKENS_UNSUPPORTED") {
		t.Fatal("unvalidated tool count accepted")
	}
	response = post(t, g, validRequest)
	bodyText(t, response)
	if response.StatusCode != 200 || f.Calls() != 1 {
		t.Fatal("count failure contaminated generation")
	}
}
