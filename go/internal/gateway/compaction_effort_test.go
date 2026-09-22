package gateway

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func effortRequest(t *testing.T, g *Gateway, model, effort, text, class string, count bool) (int, string) {
	t.Helper()
	fields := map[string]any{"model": model, "output_config": map[string]string{"effort": effort}, "messages": []any{map[string]string{"role": "user", "content": text}}}
	if !count {
		fields["stream"], fields["max_tokens"] = true, 1024
	}
	raw, _ := json.Marshal(fields)
	rq := messages(strings.NewReader(string(raw)))
	rq.headers["X-Claude-Code-Session-Id"] = "public-session"
	rq.headers["X-Claude-Code-Request-Class"] = class
	if count {
		rq.path = "/v1/messages/count_tokens"
	}
	r := do(t, g, rq)
	return r.StatusCode, bodyText(t, r)
}

func TestCompactionCountRestoresJournalWithoutChangingIt(t *testing.T) {
	projects := t.TempDir()
	g := journalGateway(t, projects)
	if code, _ := effortRequest(t, g, "gpt-6-astra", "max", "before", "main", false); code != 200 {
		t.Fatal("initial request failed")
	}
	state := g.contexts.states[contextKey("public-session", "")]
	state.phase, state.target = "required", "gpt-5.6-sol"
	if err := g.saveContext(state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projects, state.journal)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := journalGateway(t, projects)
	prompt := compactPrompt() + triggeredCompactEvent(t, restarted, "auto")
	if code, _ := effortRequest(t, restarted, "gpt-5.6-sol", "high", prompt, "compaction", true); code != 200 {
		t.Fatal("count failed")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || len(restarted.contexts.states) != 0 || len(restarted.contexts.tickets) != 1 {
		t.Fatal("count mutated journal, state or receipt")
	}
	f := restarted.transport.(*contextFixture)
	counted := f.countBody.Load().(string)
	var built bridge.Request
	if json.Unmarshal([]byte(counted), &built) != nil || built.Model != "gpt-6-astra" || built.Effort.Effort != "medium" {
		t.Fatal("count did not restore and cap the persisted route")
	}
	if code, _ := effortRequest(t, restarted, "gpt-5.6-sol", "high", prompt, "compaction", false); code != 200 || f.LastRequest() != counted {
		t.Fatal("restored generation differed from count")
	}
	if state := restarted.contexts.states[contextKey("public-session", "")]; state.route.Model != "gpt-6-astra" || state.route.Effort != "max" || state.phase != "recount" {
		t.Fatal("persisted original route changed")
	}
}

func TestCompactionRequiresVerifiedAutomaticReceiptForCap(t *testing.T) {
	for _, kind := range []string{"missing", "expired", "other-session", "other-agent"} {
		t.Run(kind, func(t *testing.T) {
			g, f := newContextFixture(t)
			marker := ""
			if kind != "missing" {
				marker = triggeredCompactEvent(t, g, "auto")
				for key, receipt := range g.contexts.tickets {
					switch kind {
					case "expired":
						receipt.at = time.Now().Add(-6 * time.Minute)
					case "other-session":
						receipt.session = "other-session"
					case "other-agent":
						receipt.agent = "other-agent"
					}
					g.contexts.tickets[key] = receipt
				}
			}
			prompt := compactPrompt() + marker
			for _, count := range []bool{true, false} {
				if code, _ := effortRequest(t, g, "gpt-5.6-luna", "high", prompt, "compaction", count); code != 200 {
					t.Fatal("native class compaction failed")
				}
				body := f.LastRequest()
				if count {
					body = f.countBody.Load().(string)
				}
				var built bridge.Request
				if json.Unmarshal([]byte(body), &built) != nil || built.Effort.Effort != "high" || strings.Contains(body, "clauduct-compact:") {
					t.Fatal("unverified receipt lowered effort or leaked")
				}
				if kind != "missing" && len(g.contexts.tickets) != 1 {
					t.Fatal("unverified request consumed another receipt")
				}
			}
		})
	}
}

func TestCompactionUnknownRouteDoesNotConsumeManualRetry(t *testing.T) {
	for _, route := range [][2]string{{"unknown-model", "high"}, {"gpt-5.6-luna", "unknown-effort"}} {
		g, f := newContextFixture(t)
		if code, _ := effortRequest(t, g, "gpt-5.6-luna", "max", "before", "main", false); code != 200 {
			t.Fatal("initial request failed")
		}
		state := g.contexts.states[contextKey("public-session", "")]
		state.phase = "failed"
		prompt := compactPrompt() + triggeredCompactEvent(t, g, "manual")
		for _, count := range []bool{true, false} {
			if code, _ := effortRequest(t, g, route[0], route[1], prompt, "compaction", count); code != 400 || f.Calls() != 1 || f.counts.Load() != 0 {
				t.Fatal("unknown model or effort reached backend")
			}
			if state.phase != "failed" || state.route.Effort != "max" || len(g.contexts.tickets) != 1 {
				t.Fatal("invalid request consumed the manual recovery")
			}
		}
		if code, _ := effortRequest(t, g, "gpt-5.6-luna", "max", prompt, "compaction", false); code != 200 || state.phase != "recount" {
			t.Fatal("manual recovery failed")
		}
		var built bridge.Request
		if json.Unmarshal([]byte(f.LastRequest()), &built) != nil || built.Effort.Effort != "max" {
			t.Fatal("manual recovery lost original effort")
		}
	}
}

func triggeredCompactEvent(t *testing.T, g *Gateway, trigger string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"event": "PreCompact", "sessionId": "public-session", "trigger": trigger})
	rq := messages(strings.NewReader(string(raw)))
	rq.path = "/clauduct/context"
	r := do(t, g, rq)
	var reply struct{ Ticket string }
	if r.StatusCode != 200 || json.Unmarshal([]byte(bodyText(t, r)), &reply) != nil || reply.Ticket == "" {
		t.Fatal("compaction event failed")
	}
	return "[clauduct-compact:" + reply.Ticket + "]"
}

func TestAutomaticCompactionCapsOnlyItsRequest(t *testing.T) {
	for _, trigger := range []string{"auto", "manual", ""} {
		for _, effort := range bridge.Efforts {
			t.Run(trigger+"/"+effort, func(t *testing.T) {
				g, f := newContextFixture(t)
				if code, _ := effortRequest(t, g, "gpt-5.6-luna", effort, "before", "main", false); code != 200 {
					t.Fatal("initial request failed")
				}
				ticket := triggeredCompactEvent(t, g, trigger)
				if code, _ := effortRequest(t, g, "gpt-5.6-luna", effort, compactPrompt()+ticket, "compaction", false); code != 200 {
					t.Fatal("compaction failed")
				}
				var built bridge.Request
				if json.Unmarshal([]byte(f.LastRequest()), &built) != nil {
					t.Fatal("request decode")
				}
				want := effort
				if trigger == "auto" && effort != "low" && effort != "medium" {
					want = "medium"
				}
				if built.Model != "gpt-5.6-luna" || built.Effort.Effort != want {
					t.Fatalf("compaction route=%s/%s want=%s", built.Model, built.Effort.Effort, want)
				}
				if state := g.contexts.states[contextKey("public-session", "")]; state.route.Effort != effort || state.phase != "recount" {
					t.Fatal("compaction overwrote the session route")
				}
				if code, _ := effortRequest(t, g, "gpt-5.6-luna", effort, "after", "main", false); code != 200 {
					t.Fatal("followup failed")
				}
				if json.Unmarshal([]byte(f.LastRequest()), &built) != nil || built.Effort.Effort != effort {
					t.Fatal("followup did not restore effort")
				}
			})
		}
	}
}

func TestCompactionCountMatchesGenerationWithoutConsumingReceipt(t *testing.T) {
	g, f := newContextFixture(t)
	if code, _ := effortRequest(t, g, "gpt-6-astra", "max", "before", "main", false); code != 200 {
		t.Fatal("initial request failed")
	}
	ticket := triggeredCompactEvent(t, g, "auto")
	text := compactPrompt() + ticket
	state := g.contexts.states[contextKey("public-session", "")]
	if code, _ := effortRequest(t, g, "gpt-5.6-sol", "xhigh", text, "compaction", true); code != 200 {
		t.Fatal("count failed")
	}
	if state.phase != "" || state.busy || len(g.contexts.tickets) != 1 || f.Calls() != 1 {
		t.Fatal("count consumed compaction state")
	}
	counted := f.countBody.Load().(string)
	if !strings.Contains(counted, bridge.CompactEfficiencyInstruction) || strings.Contains(counted, "clauduct-compact:") {
		t.Fatal("count omitted guidance or included local ticket")
	}
	if code, _ := effortRequest(t, g, "gpt-5.6-sol", "xhigh", text, "compaction", false); code != 200 {
		t.Fatal("generation failed")
	}
	if f.LastRequest() != counted {
		t.Fatal("count and generation payloads differ")
	}
	if len(g.contexts.tickets) != 0 {
		t.Fatal("generation did not consume its verified receipt")
	}
	var got bridge.Request
	if json.Unmarshal([]byte(counted), &got) != nil || got.Model != "gpt-6-astra" || got.Effort.Effort != "medium" {
		t.Fatal("count lost the pinned model or cap")
	}
	recent := g.Snapshot().Recent
	last := recent[len(recent)-1]
	if last.CountSource != "prior-count-cache" || last.CountAgreement != "matched" || f.counts.Load() != 1 {
		t.Fatalf("prior count not matched: source=%s agreement=%s", last.CountSource, last.CountAgreement)
	}
}
