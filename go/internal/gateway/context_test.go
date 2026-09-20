package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type contextFixture struct {
	upstream.Fixture
	tokens    int64
	err       error
	counts    atomic.Int64
	countBody atomic.Value
}

func (f *contextFixture) Count(_ context.Context, call upstream.Call) (int64, error) {
	f.counts.Add(1)
	f.countBody.Store(string(call.Body))
	return f.tokens, f.err
}

func TestCompactEfficiencyIsAuthorizedWithoutPreflight(t *testing.T) {
	g, f := newContextFixture(t)
	for i, compact := range []bool{false, true, false} {
		text := fmt.Sprintf("public followup %d", i)
		if compact {
			ticket := compactEvent(t, g, "PreCompact")
			text = strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1)
		}
		response := contextRequest(t, g, "gpt-5.6-sol", text)
		bodyText(t, response)
		if response.StatusCode != 200 {
			t.Fatal("request failed", response.StatusCode)
		}
		var sent bridge.Request
		if json.Unmarshal([]byte(f.LastRequest()), &sent) != nil {
			t.Fatal("request decode")
		}
		if (sent.Input[0].Role == "developer" && sent.Input[0].Content == bridge.CompactEfficiencyInstruction) != compact {
			t.Fatal("compaction guidance affected wrong request")
		}
		if f.counts.Load() != 0 {
			t.Fatal("generation performed a remote count")
		}
	}
}

func (f *contextFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.Fixture.SSE = sse(created, delta("ok"), done("ok"), strings.Replace(completed, `"input_tokens":5`, fmt.Sprintf(`"input_tokens":%d`, f.tokens), 1), "[DONE]")
	return f.Fixture.Execute(ctx, call)
}

func contextRequest(t *testing.T, g *Gateway, model, text string) *http.Response {
	t.Helper()
	content, _ := json.Marshal(text)
	rq := messages(strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":true,"max_tokens":1024,"messages":[{"role":"user","content":%s}],"tools":[{"name":"proof","input_schema":{"type":"object","properties":{}}}]}`, model, content)))
	rq.headers["X-Claude-Code-Session-Id"] = "public-session"
	return do(t, g, rq)
}

func compactEvent(t *testing.T, g *Gateway, event string) string {
	t.Helper()
	rq := messages(strings.NewReader(fmt.Sprintf(`{"sessionId":"public-session","event":%q}`, event)))
	rq.path = "/clauduct/context"
	resp := do(t, g, rq)
	body := bodyText(t, resp)
	if event == "PostCompact" {
		if resp.StatusCode != 204 {
			t.Fatal("post event failed")
		}
		return ""
	}
	var reply struct{ Ticket string }
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &reply) != nil || len(reply.Ticket) != 43 {
		t.Fatal("pre event did not return receipt")
	}
	return "[clauduct-compact:" + reply.Ticket + "]"
}

func compactPrompt() string {
	return "CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.\n\n- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.\n- You already have all the context you need in the conversation above.\n- Tool calls will be REJECTED and will waste your only turn — you will fail the task.\n- Your entire response must be plain text: an <analysis> block followed by a <summary> block.\n\nSummarize the synthetic history.\n\nREMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task."
}

func newContextFixture(t *testing.T) (*Gateway, *contextFixture) {
	f := &contextFixture{Fixture: upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}, tokens: 1000}
	g := startWith(t, f)
	g.EnableContextPolicy()
	return g, f
}

func TestContextPolicyRequiresClassAndNativeSessionEvidence(t *testing.T) {
	for _, missing := range []string{"class", "session"} {
		t.Run(missing, func(t *testing.T) {
			g, f := newContextFixture(t)
			rq := messages(strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"public"}]}`))
			rq.headers["X-Claude-Code-Session-Id"] = "public-session"
			want := "CONTEXT_REQUEST_CLASS_UNVERIFIED"
			if missing == "class" {
				delete(rq.headers, "X-Claude-Code-Request-Class")
			} else {
				g.ConfigureDelegations(t.TempDir())
				want = "CONTEXT_SESSION_UNVERIFIED"
			}
			response := do(t, g, rq)
			if response.StatusCode != 400 || !strings.Contains(bodyText(t, response), want) || f.Calls() != 0 || f.counts.Load() != 0 {
				t.Fatal("unverified policy performed upstream work")
			}
		})
	}
}

func TestContextObservedBoundaries(t *testing.T) {
	for _, model := range bridge.Models {
		for _, offset := range []int64{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", model.Key, offset), func(t *testing.T) {
				g, f := newContextFixture(t)
				f.tokens = model.Context.CompactAt + offset - 2
				first := contextRequest(t, g, model.ID, "public boundary")
				bodyText(t, first)
				if first.StatusCode != 200 || f.counts.Load() != 0 {
					t.Fatal("unseen payload treated as exact preflight")
				}
				next := contextRequest(t, g, model.ID, "public boundary")
				body := bodyText(t, next)
				if offset < 0 {
					if next.StatusCode != 200 || f.Calls() != 2 {
						t.Fatal("below target refused")
					}
				} else if next.StatusCode != 400 || f.Calls() != 1 || !strings.Contains(body, "prompt is too long") {
					t.Fatal("observed target not compacted before next generation")
				}
			})
		}
	}
}

func TestGenerationDoesNotDependOnCountAvailability(t *testing.T) {
	g, f := newContextFixture(t)
	f.err = errors.New("private detail")
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-terra"} {
		response := contextRequest(t, g, model, "same")
		body := bodyText(t, response)
		if response.StatusCode != 200 || strings.Contains(body, "private") {
			t.Fatal("count failure blocked generation")
		}
	}
	if f.counts.Load() != 0 || f.Calls() != 3 {
		t.Fatal("additional backend calls")
	}
	if d := status(t, g); d.Totals.MeasuredRequests != 3 || d.Totals.InputTokens != 3000 || d.Totals.OutputTokens != 6 {
		t.Fatal("actual usage totals incorrect", d.Totals)
	}
}

func TestContextCompactionNeedsAuthorizationAndReassessment(t *testing.T) {
	for _, outcome := range []string{"ok", "still_large", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			g, f := newContextFixture(t)
			r := contextRequest(t, g, "gpt-5.6-sol", compactPrompt())
			bodyText(t, r)
			if r.StatusCode != 400 || f.Calls() != 0 {
				t.Fatal("prompt alone authorized compaction")
			}
			f.tokens = 239000
			bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "large"))
			bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "large"))
			ticket := compactEvent(t, g, "PreCompact")
			if outcome == "failed" {
				f.Err = upstream.ErrNoTransport
			}
			r = contextRequest(t, g, "gpt-5.6-sol", strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1))
			bodyText(t, r)
			if outcome == "failed" {
				f.Err = nil
				r = contextRequest(t, g, "gpt-5.6-sol", "after")
				bodyText(t, r)
				if r.StatusCode != 400 || f.Calls() != 2 {
					t.Fatal("failed compaction resumed")
				}
				return
			}
			if r.StatusCode != 200 || g.agentContexts()[0].LastUsage != nil {
				t.Fatal("old full-history usage retained after compaction")
			}
			text := "small summary"
			if outcome == "still_large" {
				text = strings.Repeat("x", 239000*3)
			}
			f.tokens = 1000
			r = contextRequest(t, g, "gpt-5.6-sol", text)
			body := bodyText(t, r)
			if outcome == "ok" {
				if r.StatusCode != 200 || f.Calls() != 3 {
					t.Fatal("compaction did not resume")
				}
			} else if r.StatusCode != 400 || f.Calls() != 2 || strings.Contains(body, "prompt is too long") {
				t.Fatal("oversized summary looped")
			}
		})
	}
}

func TestContextSmallerSwitchCompactsWithPreviousRoute(t *testing.T) {
	g, f := newContextFixture(t)
	f.tokens = 239001
	bodyText(t, contextRequest(t, g, "gpt-6-astra", "old conversation"))
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "switch"))
	ticket := compactEvent(t, g, "PreCompact")
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1)))
	if strings.Contains(f.LastRequest(), ticket) {
		t.Fatal("local receipt reached backend")
	}
	var sent struct {
		Model     string
		Reasoning struct{ Effort string }
	}
	if json.Unmarshal([]byte(f.LastRequest()), &sent) != nil || sent.Model != "gpt-6-astra" || sent.Reasoning.Effort != "medium" {
		t.Fatal("switch compacted with new model/effort")
	}
	compactEvent(t, g, "PostCompact")
	f.tokens = 1000
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "small summary"))
	if json.Unmarshal([]byte(f.LastRequest()), &sent) != nil || sent.Model != "gpt-5.6-sol" {
		t.Fatal("verified switch not applied")
	}
}

func TestContextSwitchUsesDestinationTargetWithoutClaimingExactness(t *testing.T) {
	for _, from := range bridge.Models {
		for _, to := range bridge.Models {
			if from.ID == to.ID {
				continue
			}
			for _, used := range []int64{39162, to.Context.CompactAt - 1, to.Context.CompactAt} {
				t.Run(fmt.Sprintf("%s-to-%s/%d", from.Key, to.Key, used), func(t *testing.T) {
					g, f := newContextFixture(t)
					f.tokens = used - 2
					bodyText(t, contextRequest(t, g, from.ID, "PUBLIC_UNSUMMARIZED_HISTORY"))
					r := contextRequest(t, g, to.ID, "PUBLIC_UNSUMMARIZED_HISTORY")
					bodyText(t, r)
					if used >= to.Context.CompactAt {
						if r.StatusCode != 400 || f.Calls() != 1 {
							t.Fatal("destination target ignored")
						}
						return
					}
					state := g.agentContexts()[0]
					if r.StatusCode != 200 || f.Calls() != 2 || f.counts.Load() != 0 || state.Model != to.ID || state.Phase != "ready" || state.LastUsage.Model != to.ID || !strings.Contains(f.LastRequest(), "PUBLIC_UNSUMMARIZED_HISTORY") {
						t.Fatal("switch lost history or route")
					}
				})
			}
		}
	}
}

func TestContextNativeClassSupportsToollessCompactionWithoutPromptTemplate(t *testing.T) {
	g, f := newContextFixture(t)
	// Long unbroken text is outside the local tokenizer's validated subset.
	body := strings.Replace(validRequest, "ping", strings.Repeat("x", 513), 1)
	send := func(class string) *http.Response {
		rq := messages(strings.NewReader(body))
		rq.headers["X-Claude-Code-Session-Id"] = "public-session"
		rq.headers["X-Claude-Code-Request-Class"] = class
		return do(t, g, rq)
	}
	f.tokens = 450000
	bodyText(t, send("main"))
	response := send("main")
	if result := bodyText(t, response); response.StatusCode != 400 || !strings.Contains(result, "prompt is too long") {
		t.Fatal("tool-less conversation not compactable")
	}
	// No hook receipt or English prompt match is needed when native identifies
	// the compaction over the authenticated local request boundary.
	response = send("compaction")
	bodyText(t, response)
	if response.StatusCode != 200 || f.Calls() != 2 {
		t.Fatal("native-class compaction rejected")
	}
	f.tokens = 1000
	body = strings.Replace(body, strings.Repeat("x", 513), strings.Repeat("y", 513), 1)
	response = send("main")
	bodyText(t, response)
	if response.StatusCode != 200 || f.Calls() != 3 {
		t.Fatal("tool-less compaction did not resume")
	}
}

func TestContextUnknownChildFailsBeforeCountingOrGeneration(t *testing.T) {
	g, f := newContextFixture(t)
	rq := messages(strings.NewReader(validRequest))
	rq.headers["X-Claude-Code-Agent-Id"] = "unknown"
	resp := do(t, g, rq)
	bodyText(t, resp)
	if resp.StatusCode != 400 || f.counts.Load() != 0 || f.Calls() != 0 {
		t.Fatal("unknown child ran")
	}
}

func TestContextFailedCompactionNeedsExplicitManualRetry(t *testing.T) {
	for _, trigger := range []string{"auto", "manual"} {
		g, f := newContextFixture(t)
		bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "old"))
		g.contexts.states[contextKey("public-session", "")].phase = "failed"
		rq := messages(strings.NewReader(`{"event":"PreCompact","sessionId":"public-session","trigger":"` + trigger + `"}`))
		rq.path = "/clauduct/context"
		var receipt struct{ Ticket string }
		if json.Unmarshal([]byte(bodyText(t, do(t, g, rq))), &receipt) != nil {
			t.Fatal("receipt")
		}
		prompt := strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. [clauduct-compact:"+receipt.Ticket+"]", 1)
		response := contextRequest(t, g, "gpt-5.6-sol", prompt)
		bodyText(t, response)
		if trigger == "auto" {
			if response.StatusCode != 400 || f.Calls() != 1 {
				t.Fatal("automatic failure retry executed")
			}
		} else if response.StatusCode != 200 || f.Calls() != 2 {
			t.Fatal("explicit manual recovery refused")
		}
	}
}

func TestContextReceiptRejectsReplayWrongScopeAndExpiry(t *testing.T) {
	for _, failure := range []string{"expired", "wrong_session", "wrong_agent", "replay"} {
		t.Run(failure, func(t *testing.T) {
			g, f := newContextFixture(t)
			ticket := compactEvent(t, g, "PreCompact")
			key := compactTicketPattern.FindStringSubmatch(ticket)[1]
			g.contexts.mu.Lock()
			receipt := g.contexts.tickets[key]
			switch failure {
			case "expired":
				receipt.at = receipt.at.Add(-6 * time.Minute)
			case "wrong_session":
				receipt.session = "different"
			case "wrong_agent":
				receipt.agent = "different"
			case "replay":
				delete(g.contexts.tickets, key)
			}
			if failure != "replay" {
				g.contexts.tickets[key] = receipt
			}
			g.contexts.mu.Unlock()
			resp := contextRequest(t, g, "gpt-5.6-sol", strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1))
			bodyText(t, resp)
			if resp.StatusCode != 400 || f.Calls() != 0 {
				t.Fatal("invalid receipt or over-window summary executed")
			}
		})
	}
}

func TestContextStatusSeparatesUsageEstimatesAndTargets(t *testing.T) {
	g, f := newContextFixture(t)
	f.tokens = 239000
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "large"))
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "large"))
	d := status(t, g)
	if len(d.AgentContexts) != 1 || d.AgentContexts[0].Phase != "required" || d.AgentContexts[0].LastUsage.Input != 239000 {
		t.Fatal("pending context not observable")
	}
	for _, model := range d.ModelContexts {
		if model.Application != "gateway_usage_preventive_compaction" || model.Observed.Preflights != 0 {
			t.Fatal("policy misreported")
		}
		if model.Model == "gpt-5.6-sol" {
			if model.Verification != "backend_usage_observed" || model.Observed.Requests != 1 {
				t.Fatal("usage missing")
			}
		} else if model.Verification != "not_observed" {
			t.Fatal("unrun model reported verified")
		}
	}
}
