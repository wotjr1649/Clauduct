package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The previous native enum rejected these IDs before a child existed. The
// production translator now adapts only the model spelling, keeping Plan intact.
func TestNativeAgentAcceptsFullModelIDWithModelDefaultEffort(t *testing.T) {
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			models, stderr := delegationAs(t, "Plan", model.ID)
			if !ran(models, model.ID+"/"+model.Effort) {
				t.Fatalf("full ID did not reach backend with model default: %v; %s", models, tail(stderr, 800))
			}
		})
	}
}

// The malformed call is supplied as backend input to the real translator and
// native client. The rejection itself is never mocked or left to model judgment.
func TestNativeUnsupportedEffortReachesGatewayValidation(t *testing.T) {
	buildHook(t)
	script, output, result := scripted(t, []string{"-p", "delegate once", "--allowedTools", "Agent"},
		toolStream("unsupported_effort", "Agent", `{"subagent_type":"Plan","description":"negative proof","prompt":"reply ok","model":"gpt-5.6-luna","effort":"turbo"}`))
	if len(script.Conversations()) != 1 || result.NativeExitCode == 0 || result.Diagnostics.Totals.Failures["UNSUPPORTED_MODEL_OR_EFFORT"] != 1 || len(result.Diagnostics.AgentSelections.Recent) != 1 {
		t.Fatalf("invalid choice did not fail before a child: exit=%d conversations=%d failures=%v output=%s", result.NativeExitCode, len(script.Conversations()), result.Diagnostics.Totals.Failures, tail(output, 800))
	}
	selection := result.Diagnostics.AgentSelections.Recent[0]
	if selection.State != "prepare_refused" || selection.Failure != "UNSUPPORTED_MODEL_OR_EFFORT" || selection.Agent != "" || selection.BackendResponses != 0 || selection.RequestedEffort != "unlisted" {
		t.Fatalf("refused child evidence: %+v", selection)
	}
}

func TestNativeFullModelSelectionsStaySeparateInMixedParallelDelegations(t *testing.T) {
	buildHook(t)
	events := []string{`{"type":"response.created","response":{"id":"resp_parallel"}}`}
	args := []string{
		`{"subagent_type":"Plan","description":"sol","prompt":"say ok","model":"gpt-5.6-sol","effort":"high"}`,
		`{"subagent_type":"Plan","description":"luna","prompt":"say ok","model":"gpt-5.6-luna"}`,
		`{"subagent_type":"clauduct-terra-medium","description":"legacy","prompt":"say ok"}`,
	}
	for i, raw := range args {
		encoded, _ := json.Marshal(raw)
		id := fmt.Sprintf("fc_%d", i)
		events = append(events,
			fmt.Sprintf(`{"type":"response.output_item.added","output_index":%d,"item":{"id":%q,"type":"function_call"}}`, i, id),
			fmt.Sprintf(`{"type":"response.function_call_arguments.delta","item_id":%q,"delta":%s}`, id, encoded),
			fmt.Sprintf(`{"type":"response.function_call_arguments.done","item_id":%q,"arguments":%s}`, id, encoded),
			fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":{"id":%q,"type":"function_call","call_id":"call_parallel_%d","name":"Agent","arguments":%s}}`, i, id, i, encoded))
	}
	events = append(events, `{"type":"response.completed","response":{"id":"resp_parallel","usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13},"output":[]}}`)
	script, _, result := scripted(t, []string{"-p", "delegate in parallel", "--allowedTools", "Agent"},
		frames(events...), textStream("child1", "ok"), textStream("child2", "ok"), textStream("child3", "ok"), textStream("parent", "done"))
	if result.NativeExitCode != 0 {
		t.Fatalf("native exit: %d", result.NativeExitCode)
	}
	seen := map[string]int{}
	for _, raw := range script.Conversations() {
		var req struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		seen[req.Model+"/"+req.Reasoning.Effort]++
	}
	for _, route := range []string{"gpt-5.6-sol/high", "gpt-5.6-luna/max", "gpt-5.6-terra/medium"} {
		if seen[route] != 1 {
			t.Fatalf("parallel route counts: %v; want one %s", seen, route)
		}
	}
}

func TestNativeFullModelSelectionPreservesRoleToolsAndExplicitEffort(t *testing.T) {
	buildHook(t)
	const role = `{"restricted":{"description":"Synthetic read-only role","prompt":"ROLE_RESTRICTION_PROOF: inspect only","tools":["Read"],"model":"opus","effort":"high"}}`
	script, _, result := scripted(t, []string{"-p", "delegate", "--allowedTools", "Agent", "--agents", role},
		toolStream("call_explicit", "Agent", `{"subagent_type":"restricted","description":"check","prompt":"say ok","model":"gpt-5.6-luna","effort":"medium"}`),
		textStream("child", "ok"), textStream("parent", "done"))
	if result.NativeExitCode != 0 {
		t.Fatalf("native exit: %d", result.NativeExitCode)
	}
	found := false
	for _, raw := range script.Conversations() {
		var req struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "gpt-5.6-luna" {
			continue
		}
		found = true
		if req.Reasoning.Effort != "medium" || !strings.Contains(raw, "ROLE_RESTRICTION_PROOF") {
			t.Fatalf("role prompt or explicit effort lost: %s/%s", req.Model, req.Reasoning.Effort)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "Read" {
			t.Fatalf("role tools changed: %+v", req.Tools)
		}
	}
	if !found {
		t.Fatal("selected child never reached backend")
	}
}

func TestNativeEffortOnlyKeepsBuiltInRoleModel(t *testing.T) {
	buildHook(t)
	script, _, result := scripted(t, []string{"-p", "delegate", "--allowedTools", "Agent"},
		toolStream("call_effort_only", "Agent", `{"subagent_type":"Plan","description":"plan","prompt":"say ok","effort":"high"}`),
		textStream("child", "ok"), textStream("parent", "done"))
	if result.NativeExitCode != 0 {
		t.Fatalf("native exit: %d", result.NativeExitCode)
	}
	found := false
	for _, raw := range script.Conversations() {
		var req struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		if req.Model == "gpt-6-astra" && req.Reasoning.Effort == "high" {
			found = true
		}
	}
	if !found {
		t.Fatal("Plan did not use its Astra default with explicit high effort")
	}
}

func TestNativeNestedAgentKeepsDelegationModelAndEffort(t *testing.T) {
	buildHook(t)
	// Native starts background children asynchronously. Do not let a fixture's
	// premature parent answer close the process before the descendant can run.
	script := &nestedSelectionFixture{inner: make(chan struct{})}
	out := (nativeRun{Args: []string{"-p", "delegate nested task", "--allowedTools", "Agent"}, transport: script}).run(t)
	result := out.result
	if result.NativeExitCode != 0 {
		t.Fatalf("native exit: %d outer=%d root=%d records=%+v output=%s", result.NativeExitCode, script.outer.Load(), script.root.Load(), result.Diagnostics.Recent, tail(out.output(), 3000))
	}
	found := false
	for _, entry := range result.Diagnostics.Recent {
		if entry.Source == "delegation-inherited" {
			found = true
			if entry.AgentID == "" || entry.ParentAgentID == "" || entry.AgentRole != "Plan" || entry.SelectionVerified == nil || !*entry.SelectionVerified {
				t.Fatalf("nested provenance absent: %+v", entry)
			}
			if entry.Model != "gpt-5.6-sol" || entry.Effort != "high" {
				t.Fatalf("nested route changed: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatalf("no verified nested selection reached backend: %+v", result.Diagnostics.Recent)
	}
}

type nestedSelectionFixture struct {
	innerRole   string
	omitRole    bool
	root, outer atomic.Int64
	once        sync.Once
	inner       chan struct{}
}

func (s *nestedSelectionFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	reply := textStream("side", "done")
	if call.Source == "delegation-inherited" {
		s.once.Do(func() { close(s.inner) })
		reply = textStream("inner", "ok")
	} else if call.Model == "gpt-5.6-sol" {
		switch s.outer.Add(1) {
		case 1:
			reply = toolStream("discover_inner", "ToolSearch", `{"query":"select:Agent","max_results":1}`)
		case 2:
			role := s.innerRole
			if role == "" {
				role = "Plan"
			}
			reply = toolStream("call_inner", "Agent", fmt.Sprintf(`{"subagent_type":%q,"description":"inner","prompt":"say ok"}`, role))
			if s.omitRole {
				reply = toolStream("call_inner", "Agent", `{"description":"inner","prompt":"say ok"}`)
			}
		default:
			select {
			case <-s.inner:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			reply = textStream("outer", "done")
		}
	} else if upstream.Conversation(string(call.Body)) {
		if s.root.Add(1) == 1 {
			reply = toolStream("call_outer", "Agent", `{"subagent_type":"clauduct-inherit","description":"outer","prompt":"delegate a plan","model":"gpt-5.6-sol","effort":"high"}`)
		} else {
			select {
			case <-s.inner:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return (&upstream.Fixture{SSE: reply}).Execute(ctx, call)
}

func TestNativeOmittedChildRoleRetainsVerifiedInheritance(t *testing.T) {
	buildHook(t)
	script := &nestedSelectionFixture{omitRole: true, inner: make(chan struct{})}
	out := (nativeRun{Args: []string{"-p", "delegate nested public task", "--allowedTools", "Agent"}, transport: script}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("native omitted role: %v exit=%d", out.err, out.result.NativeExitCode)
	}
	found := false
	for _, r := range out.result.Diagnostics.Recent {
		if r.Source == "delegation-inherited" {
			found = r.AgentRole == "general-purpose" && r.Model == "gpt-5.6-sol" && r.Effort == "high" && r.SelectionVerified != nil && *r.SelectionVerified
		}
	}
	if !found {
		t.Fatal("native default role/inheritance not observed")
	}
}

func TestNativeForkKeepsVerifiedParentSelection(t *testing.T) {
	buildHook(t)
	script := &nestedSelectionFixture{innerRole: "fork", inner: make(chan struct{})}
	// Native enables fork by default in TUI, but disables it in -p unless this
	// documented flag is set. Use the same feature in this isolated native check.
	out := (nativeRun{Args: []string{"-p", "delegate nested public task", "--allowedTools", "Agent"}, Env: map[string]string{"CLAUDE_CODE_FORK_SUBAGENT": "1"}, transport: script}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("fork failed: exit=%d err=%v records=%+v output=%s", out.result.NativeExitCode, out.err, out.result.Diagnostics.RecentFailures, tail(out.output(), 800))
	}
	found := false
	for _, r := range out.result.Diagnostics.Recent {
		if r.AgentRole == "fork" && r.Source == "delegation-inherited" {
			found = r.Model == "gpt-5.6-sol" && r.Effort == "high" && r.SelectionVerified != nil && *r.SelectionVerified
		}
	}
	if !found {
		t.Fatal("fork never reached backend with its verified parent selection")
	}
}
