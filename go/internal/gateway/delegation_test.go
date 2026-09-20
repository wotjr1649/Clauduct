package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestBackendHistoryRestoresOnlyVerifiedOriginalSelection(t *testing.T) {
	for _, tc := range []struct{ fields, model, effort string }{
		{`"model":"gpt-5.6-terra","effort":"medium",`, `"gpt-5.6-terra"`, `"medium"`},
		{`"model":"gpt-5.6-luna",`, `"gpt-5.6-luna"`, ""},
		{`"effort":"high",`, "", `"high"`},
		{"", "", ""},
	} {
		d := &delegations{pending: map[delegationKey]delegatedChoice{}, resolved: map[string]resolvedChoice{}}
		scope := delegationScope{session: "history_session", parent: ""}
		adapted, err := d.prepare(scope, "history_call", "Agent", []byte(`{`+tc.fields+`"subagent_type":"Plan","prompt":"keep this exact prompt","description":"proof","isolation":null}`))
		if err != nil {
			t.Fatal(err)
		}
		request := func() *anthropic.Request {
			return &anthropic.Request{Messages: []anthropic.Message{{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", Name: "Agent", ID: "history_call", Input: adapted}}}}}
		}
		for _, wrong := range []bool{true, false} {
			req := request()
			session := scope.session
			if wrong {
				session = "different_session"
			}
			d.restoreSelectionHistory(req, session, "")
			got := req.Messages[0].Blocks[0].Input
			if wrong {
				if string(got) != string(adapted) {
					t.Fatal("cross-session history changed")
				}
				continue
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(got, &fields) != nil || string(fields["model"]) != tc.model || string(fields["effort"]) != tc.effort || string(fields["isolation"]) != "null" || string(fields["prompt"]) != `"keep this exact prompt"` {
				t.Fatalf("history: %s", got)
			}
			built, err := bridge.BuildRequest(&anthropic.Request{Model: "gpt-6-astra", Messages: req.Messages})
			if err != nil || len(built.Input) != 1 || built.Input[0].Arguments != string(got) {
				t.Fatal("restored arguments not used on wire")
			}
		}
	}
}

func TestDelegationWaitsForNativeAsynchronousMetadataOnly(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		d, scope, binding := preparedDelegation(t)
		path := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
		body, err := os.ReadFile(path)
		if err != nil || os.Remove(path) != nil {
			t.Fatal("fixture setup")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if cancelled {
			cancel()
		} else {
			defer cancel()
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			time.Sleep(40 * time.Millisecond)
			if os.WriteFile(path, body, 0600) != nil {
				t.Error("fixture publication")
			}
		}()
		route, found, err := d.route(scope, binding.ID, binding, ctx)
		<-done
		if cancelled {
			if err == nil || found {
				t.Fatal("cancelled selection executed")
			}
		} else if err != nil || !found || route.Model != "gpt-5.6-luna" {
			t.Fatal("valid delayed metadata rejected")
		}
	}
}

func TestNativeBuiltinRoleCaseKeepsVerifiedRoute(t *testing.T) {
	for _, definition := range []bool{false, true} {
		d, scope, binding := preparedDelegation(t)
		key := delegationKey{scope.session, "proof_call"}
		choice := d.pending[key]
		choice.role = "plan"
		if definition {
			choice.route.Source = "agent-call-definition"
		}
		d.pending[key] = choice
		route, found, err := d.route(scope, binding.ID, binding)
		if definition {
			if err == nil || found {
				t.Fatal("custom role identity changed")
			}
			continue
		}
		if err != nil || !found || route.Model != "gpt-5.6-luna" || route.Effort != "max" {
			t.Fatal("native built-in rejected", err)
		}
		if d.resolved[binding.ID].role != "Plan" {
			t.Fatal("canonical role not retained")
		}
		if _, _, err := d.route(scope, binding.ID, binding); err != nil {
			t.Fatal("subsequent request rejected", err)
		}
		delete(d.resolved, binding.ID)
		if _, found, err := d.loadChoice(scope, binding.ID, binding); err != nil || !found {
			t.Fatal("saved role not restorable", err)
		}
	}
}

func TestOmittedNativeRoleKeepsGeneralPurposePolicyAndProof(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	delete(d.pending, delegationKey{scope.session, "proof_call"})
	raw, err := d.prepare(scope, "proof_call", "Agent", json.RawMessage(`{"prompt":"public","description":"default role"}`))
	if err != nil || strings.Contains(string(raw), "subagent_type") {
		t.Fatal("native omission not retained", err)
	}
	binding.Role = "general-purpose"
	meta := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
	if err = os.WriteFile(meta, []byte(`{"toolUseId":"proof_call","agentType":"general-purpose","model":"haiku"}`), 0600); err != nil {
		t.Fatal(err)
	}
	route, found, err := d.route(scope, binding.ID, binding)
	if err != nil || !found || route.Model != "gpt-5.6-luna" || route.Effort != "max" {
		t.Fatal("default role not independently verified", err)
	}
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", Name: "Agent", ID: "proof_call", Input: raw}}}}}
	d.restoreSelectionHistory(req, scope.session, "")
	if strings.Contains(string(req.Messages[0].Blocks[0].Input), `"model"`) {
		t.Fatal("model omission lost in history")
	}
	for _, value := range []string{`null`, `""`, `42`, `{}`} {
		if _, err = d.prepare(scope, "invalid_role", "Agent", json.RawMessage(`{"subagent_type":`+value+`,"prompt":"public"}`)); err == nil {
			t.Fatal("invalid role accepted", value)
		}
	}
}

func preparedDelegation(t testing.TB) (*delegations, delegationScope, agentBinding) {
	t.Helper()
	root := t.TempDir()
	d := &delegations{projects: root, pending: map[delegationKey]delegatedChoice{}, resolved: map[string]resolvedChoice{}}
	scope := delegationScope{session: "proof_session"}
	binding := agentBinding{ID: "proof_child", Role: "Plan", SessionID: scope.session, TranscriptPath: filepath.Join(root, "project", scope.session+".jsonl")}
	raw, err := d.prepare(scope, "proof_call", "Agent", json.RawMessage(`{"subagent_type":"Plan","model":"gpt-5.6-luna","prompt":"synthetic","description":"proof"}`))
	if err != nil || !strings.Contains(string(raw), `"model":"haiku"`) || !strings.Contains(string(raw), `"subagent_type":"Plan"`) {
		t.Fatalf("prepare: %s %v", raw, err)
	}
	dir := filepath.Join(root, "project", scope.session, "subagents")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-proof_child.meta.json"), []byte(`{"toolUseId":"proof_call","agentType":"Plan","model":"haiku"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return d, scope, binding
}

func TestUnverifiedAdaptedChildNeverReachesBackend(t *testing.T) {
	d, scope, b := preparedDelegation(t)
	f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, f)
	g.delegations = d
	b.TranscriptPath = filepath.Join(d.projects, "missing", scope.session+".jsonl")
	if _, err := g.agents.register(b, time.Now()); err != nil {
		t.Fatal(err)
	}
	req := messages(strings.NewReader(`{"model":"gpt-5.6-luna","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"synthetic"}]}`))
	req.headers["X-Claude-Code-Agent-Id"] = b.ID
	req.headers["X-Claude-Code-Session-Id"] = scope.session
	response := do(t, g, req)
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(bodyText(t, response), "AGENT_SELECTION_UNVERIFIED") {
		t.Fatal("missing metadata was not refused")
	}
	if f.Calls() != 0 {
		t.Fatal("unverified child reached backend")
	}
}

func TestAdaptedSelectionRejectsNativeModelMismatch(t *testing.T) {
	d, scope, b := preparedDelegation(t)
	f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
	g := startWith(t, f)
	g.delegations = d
	if _, err := g.agents.register(b, time.Now()); err != nil {
		t.Fatal(err)
	}
	req := messages(strings.NewReader(`{"model":"gpt-6-astra","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"synthetic"}]}`))
	req.headers["X-Claude-Code-Agent-Id"] = b.ID
	req.headers["X-Claude-Code-Session-Id"] = scope.session
	response := do(t, g, req)
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(bodyText(t, response), "AGENT_SELECTION_UNVERIFIED") || f.Calls() != 0 {
		t.Fatal("native model mismatch was silently overridden")
	}
}

func TestForkSelectionChecksIdentityAndRestores(t *testing.T) {
	for _, changed := range []string{"", "parent", "call", "role", "model", "stopped", "request_model", "resume_ok", "resume_missing", "resume_header", "resume_model", "resume_discarded"} {
		t.Run(changed, func(t *testing.T) {
			d, scope, parent := preparedDelegation(t)
			parentRoute, _, err := d.route(scope, parent.ID, parent)
			if err != nil {
				t.Fatal(err)
			}
			scope.parent, scope.route = parent.ID, parentRoute
			_, err = d.prepare(scope, "fork_call", "Agent", []byte(`{"subagent_type":"fork","description":"public","prompt":"say ok"}`))
			if err != nil {
				t.Fatal(err)
			}
			b := parent
			b.ID, b.Role = "fork_child", "fork"
			meta := map[string]any{"toolUseId": "fork_call", "parentAgentId": parent.ID, "agentType": "fork", "model": "inherit"}
			switch changed {
			case "parent":
				meta["parentAgentId"] = "wrong"
			case "call":
				meta["toolUseId"] = "wrong"
			case "role":
				meta["agentType"] = "Plan"
			case "model":
				meta["model"] = "haiku"
			case "stopped":
				meta["stoppedByUser"] = true
			}
			raw, _ := json.Marshal(meta)
			if err := os.WriteFile(filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+b.ID+".meta.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			f := &upstream.Fixture{SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]")}
			g := startWith(t, f)
			g.delegations = d
			if _, err := g.agents.register(b, time.Now()); err != nil {
				t.Fatal(err)
			}
			model := parentRoute.Model
			if changed == "request_model" {
				model = "gpt-6-astra"
			}
			req := messages(strings.NewReader(`{"model":"` + model + `","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"public"}]}`))
			req.headers["X-Claude-Code-Agent-Id"], req.headers["X-Claude-Code-Parent-Agent-Id"], req.headers["X-Claude-Code-Session-Id"] = b.ID, parent.ID, scope.session
			if strings.HasPrefix(changed, "resume_") {
				if _, found, err := d.route(scope, b.ID, b); err != nil || !found {
					t.Fatal("initial fork", err)
				}
				b.Result = "FIRST_PUBLIC_REPORT"
				d.stopped(b)
				sender := delegationScope{session: scope.session, nativeModel: "gpt-6-astra", route: bridge.Route{Model: "gpt-6-astra", Effort: "low"}}
				if changed != "resume_missing" {
					adapted, err := d.prepare(sender, "resume_call", "SendMessage", []byte(`{"to":"fork_child","message":"Continue public proof","notify_when_idle":true}`))
					if err != nil {
						t.Fatal(err)
					}
					var message struct{ To, Message string }
					if json.Unmarshal(adapted, &message) != nil || message.To != "fork_child" || !strings.Contains(message.Message, "Your previous task has completed.") || !strings.HasSuffix(message.Message, "Continue public proof") {
						t.Fatal("fork resume did not frame the new directive")
					}
					if strings.Contains(string(adapted), "notify_when_idle") {
						t.Fatal("native child notification redundantly requested with a peer-session flag")
					}
				}
				if changed == "resume_discarded" {
					d.discard(scope.session, []string{"resume_call"})
				}
				req = messages(strings.NewReader(`{"model":"gpt-6-astra","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"resume public"}]}`))
				if changed == "resume_model" {
					req = messages(strings.NewReader(`{"model":"gpt-5.6-terra","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"resume public"}]}`))
				}
				req.headers["X-Claude-Code-Agent-Id"], req.headers["X-Claude-Code-Session-Id"] = b.ID, scope.session
				if changed == "resume_header" {
					req.headers["X-Claude-Code-Parent-Agent-Id"] = "wrong"
				}
			}
			resp := do(t, g, req)
			bodyText(t, resp)
			if changed != "" && changed != "resume_ok" {
				if resp.StatusCode != 400 || f.Calls() != 0 {
					t.Fatal("unverified fork dispatched")
				}
				return
			}
			if resp.StatusCode != 200 || f.Calls() != 1 {
				t.Fatal("verified fork refused")
			}
			if changed == "resume_ok" {
				e := d.results.entries[b.ID]
				if e.parent != "" || e.Call != "resume_call" || e.stopped {
					t.Fatal("resume retained stale recipient/result")
				}
			}
			restarted := &delegations{projects: d.projects, pending: map[delegationKey]delegatedChoice{}, resolved: map[string]resolvedChoice{}}
			got, found, err := restarted.route(scope, b.ID, b)
			if err != nil || !found || got.Model != parentRoute.Model || got.Effort != parentRoute.Effort {
				t.Fatal("fork resume lost selection", got, err)
			}
		})
	}
}

func TestDelegationConcurrentFirstRequestsKeepOneChoice(t *testing.T) {
	d, scope, b := preparedDelegation(t)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			route, found, err := d.route(scope, b.ID, b)
			if err != nil || !found || route.Model != "gpt-5.6-luna" || route.Effort != "max" {
				t.Errorf("concurrent route: %+v %v %v", route, found, err)
			}
		})
	}
	wg.Wait()
}

func TestDelegationRestoresSelectionAfterGatewayRestart(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	want, found, err := d.route(scope, binding.ID, binding)
	if err != nil || !found {
		t.Fatal("initial selection not verified", err)
	}
	restarted := &delegations{projects: d.projects, pending: map[delegationKey]delegatedChoice{}, resolved: map[string]resolvedChoice{}}
	got, found, err := restarted.route(scope, binding.ID, binding)
	if err != nil || !found || got != want {
		t.Fatalf("resume: %+v found=%v err=%v", got, found, err)
	}
	receipt := restarted.selectionReport().Recent[0]
	if !receipt.PresenceVerified || !receipt.ModelProvided || receipt.EffortProvided || receipt.RequestedModel != "gpt-5.6-luna" || receipt.Effort != "max" || receipt.Agent != binding.ID {
		t.Fatal("restart lost original presence/effective selection", receipt)
	}
	// Cache eviction re-verifies the journal and native metadata, not a role default.
	delete(restarted.resolved, binding.ID)
	if err := os.WriteFile(filepath.Join(d.projects, "project", scope.session, "subagents", "agent-proof_child.meta.json"), []byte(`{"toolUseId":"different","agentType":"Plan","model":"haiku"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := restarted.route(scope, binding.ID, binding); err == nil {
		t.Fatal("restored a journal belonging to a different call")
	}
}

func TestDelegationInheritanceRejectsConflictingDescendantChoice(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	child := delegationScope{session: scope.session, parent: binding.ID}
	raw, err := d.prepare(child, "grandchild_call", "Agent", json.RawMessage(`{"subagent_type":"Plan","prompt":"synthetic"}`))
	if err != nil || !strings.Contains(string(raw), `"model":"haiku"`) {
		t.Fatalf("inherit: %s %v", raw, err)
	}
	if got := d.pending[delegationKey{scope.session, "grandchild_call"}].route; got.Model != "gpt-5.6-luna" || got.Effort != "max" {
		t.Fatalf("changed inheritance: %+v", got)
	}
	if _, err := d.prepare(child, "conflict_call", "Agent", json.RawMessage(`{"subagent_type":"Plan","model":"opus"}`)); err == nil {
		t.Fatal("silently changed inherited model")
	}
}

func TestDelegationEffortOnlyAndInvalidSelections(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	got, err := d.prepare(scope, "effort_only", "Agent", json.RawMessage(`{"subagent_type":"Plan","effort":"high","prompt":"synthetic"}`))
	if err != nil || !strings.Contains(string(got), `"model":"fable"`) || strings.Contains(string(got), `"effort"`) {
		t.Fatalf("effort-only adaptation: %s %v", got, err)
	}
	if chosen := d.pending[delegationKey{scope.session, "effort_only"}].route; chosen.Model != "gpt-6-astra" || chosen.Effort != "high" {
		t.Fatalf("effort-only route: %+v", chosen)
	}
	for _, raw := range []string{
		`{"subagent_type":"Plan","model":"gpt-5.6-luna","effort":"ultra"}`,
		`{"subagent_type":"unknown","effort":"high"}`,
		`{"subagent_type":"Plan","model":42,"effort":"high"}`,
		`{"subagent_type":"Plan","model":null,"effort":"high"}`,
		`{"subagent_type":"Plan","model":"gpt-5.6-luna","effort":null}`,
	} {
		if _, err := d.prepare(scope, "invalid_call", "Agent", json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid: %s", raw)
		}
	}
	if categoryFor(bridge.ErrUnsupportedRoute) != "UNSUPPORTED_MODEL_OR_EFFORT" || statusForUpstream(bridge.ErrUnsupportedRoute) != http.StatusBadRequest {
		t.Fatal("unsupported selection classified as retryable upstream failure")
	}
}

func TestDelegationDescribesOnlyTheExistingAgentSchema(t *testing.T) {
	d, _, _ := preparedDelegation(t)
	original := json.RawMessage(`{"type":"object","properties":{"model":{"type":"string","enum":["opus","sonnet","haiku","fable"]},"subagent_type":{"type":"string"}},"required":["subagent_type"]}`)
	tools := []bridge.ToolSpec{{Name: "Agent", Parameters: original}, {Name: "Read", Parameters: original}, {Name: "ToolSearch", Parameters: original}, {Name: "Workflow", Parameters: original}}
	if err := d.describe(tools); err != nil {
		t.Fatal(err)
	}
	if string(tools[1].Parameters) != string(original) {
		t.Fatal("changed unrelated tool")
	}
	if !strings.Contains(tools[2].Description, "discover Agent") || !strings.Contains(tools[3].Description, "exact tool-name allowlists") || !strings.Contains(tools[3].Description, "omitted model/effort uses its verified definition defaults") || !strings.Contains(tools[3].Description, "read through native Read with its permissions") || string(tools[2].Parameters) != string(original) || string(tools[3].Parameters) != string(original) {
		t.Fatal("discovery/option contract missing or native schema changed")
	}
	for _, model := range bridge.Models {
		if !strings.Contains(string(tools[0].Parameters), model.ID) {
			t.Fatalf("missing %s", model.ID)
		}
	}
	if !strings.Contains(string(tools[0].Parameters), `"effort"`) {
		t.Fatal("effort not discoverable")
	}
	t.Logf("Agent schema extension: %d additional bytes", len(tools[0].Parameters)-len(original))
}

func TestPinnedDelegationAdvertisesOnlyCompatibleOverrides(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	tools := []bridge.ToolSpec{{Name: "Agent", Parameters: json.RawMessage(`{"type":"object","properties":{"model":{"type":"string","enum":["opus","sonnet","haiku"]}}}`)}}
	if err := d.describe(tools, binding.ID); err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct{ Enum []string }
	}
	if json.Unmarshal(tools[0].Parameters, &schema) != nil {
		t.Fatal("schema")
	}
	if strings.Join(schema.Properties["model"].Enum, ",") != "gpt-5.6-luna,inherit" || strings.Join(schema.Properties["effort"].Enum, ",") != "max" {
		t.Fatal("conflicting overrides advertised")
	}
	child := delegationScope{session: scope.session, parent: binding.ID}
	raw := json.RawMessage(`{"subagent_type":"general-purpose","model":"gpt-5.6-sol","prompt":"PRIVATE_FIXTURE_MUST_NOT_LOG"}`)
	_, err := d.prepare(child, "conflict", "Agent", raw)
	if err == nil {
		t.Fatal("guard weakened")
	}
	d.rejectedSelection(child, "conflict", raw, err)
	report := d.selectionReport()
	if report.Recent[0].Failure != "PARENT_OVERRIDE_CONFLICT" || report.Recent[0].RequestedModel != "gpt-5.6-sol" {
		t.Fatal("missing diagnostic")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_FIXTURE") {
		t.Fatal("prompt exposed")
	}
}

func TestDelegationBindsByCallIDAndCachesVerifiedSelection(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	route, found, err := d.route(scope, binding.ID, binding)
	if err != nil || !found || route.Model != "gpt-5.6-luna" || route.Effort != "max" {
		t.Fatalf("route: %+v %v %v", route, found, err)
	}
	// Once linked, normal requests do not read metadata again.
	d.projects = filepath.Join(d.projects, "does-not-exist")
	if got, found, err := d.route(scope, binding.ID, binding); err != nil || !found || got != route {
		t.Fatalf("cache: %+v %v %v", got, found, err)
	}
	if _, _, err := d.route(delegationScope{session: "other"}, binding.ID, binding); err == nil {
		t.Fatal("cross-session reuse accepted")
	}
}

func TestDelegationRejectsMissingWrongOrEscapingMetadata(t *testing.T) {
	for _, kind := range []string{"missing", "parent", "role", "model", "duplicate", "escape"} {
		t.Run(kind, func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			file := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-proof_child.meta.json")
			switch kind {
			case "missing":
				binding.ID = "absent"
			case "parent":
				scope.parent = "wrong"
				d.pending[delegationKey{scope.session, "proof_call"}] = delegatedChoice{parent: "wrong", role: "Plan", alias: "haiku"}
			case "role":
				binding.Role = "Explore"
			case "model":
				if err := os.WriteFile(file, []byte(`{"toolUseId":"proof_call","agentType":"Plan","model":"opus"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				if err := os.WriteFile(file, []byte(`{"toolUseId":"proof_call","toolUseId":"other"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "escape":
				binding.TranscriptPath = filepath.Join(filepath.Dir(d.projects), scope.session+".jsonl")
			}
			if _, _, err := d.route(scope, binding.ID, binding); err == nil {
				t.Fatal("unverified metadata accepted")
			}
		})
	}
}

func BenchmarkDelegationCachedRoute(b *testing.B) {
	d, scope, binding := preparedDelegation(b)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := d.route(scope, binding.ID, binding); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDelegationMetadataRead(b *testing.B) {
	d, _, binding := preparedDelegation(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := d.metadata(binding); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDelegationDescribe(b *testing.B) {
	d, _, _ := preparedDelegation(b)
	raw := json.RawMessage(`{"type":"object","properties":{"model":{"type":"string","enum":["opus","sonnet","haiku","fable"]},"subagent_type":{"type":"string"}},"required":["subagent_type"]}`)
	tools := []bridge.ToolSpec{{Name: "Agent", Parameters: raw}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tools[0].Parameters = raw
		if err := d.describe(tools); err != nil {
			b.Fatal(err)
		}
	}
}
