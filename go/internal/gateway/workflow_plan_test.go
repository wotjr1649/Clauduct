package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

func TestWorkflowStepToolRestriction(t *testing.T) {
	for _, variant := range []string{"none", "omitted", "forced_tool", "forced_any", "hosted_search", "missing_proof", "wrong_label", "missing_run", "ordinary", "inline"} {
		t.Run(variant, func(t *testing.T) {
			d, scope, link := planProof(t)
			key := delegationKey{scope.session, link.Run}
			run := d.workflows[key]
			noTools := []string{}
			run.origin.plan.Steps[0].Tools = &noTools
			req := &anthropic.Request{Model: scope.route.Model, MaxTokens: 100, Tools: []anthropic.Tool{{Name: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
			wantOK, wantRestricted := true, true
			switch variant {
			case "omitted":
				run.origin.plan.Steps[0].Tools = nil
				wantRestricted = false
			case "forced_tool":
				req.ToolChoice = anthropic.ToolChoice{Type: "tool", Name: "Read"}
				wantOK = false
			case "forced_any":
				req.ToolChoice.Type = "any"
				wantOK = false
			case "hosted_search":
				req.HostedSearch = &anthropic.HostedSearch{}
				wantOK = false
			case "missing_proof":
				run.observed = nil
				wantOK = false
			case "wrong_label":
				run.observed["workflow_child"] = workflowObservation{Label: "clauduct-step:UNKNOWN [clauduct:"}
				wantOK = false
			case "missing_run":
				wantOK = false
			case "ordinary":
				choice := d.resolved["workflow_child"]
				choice.role = "general-purpose"
				d.resolved["workflow_child"] = choice
				wantRestricted = false
			case "inline":
				run.origin.plan = nil
				wantRestricted = false
			}
			d.workflows[key] = run
			if variant == "missing_run" {
				delete(d.workflows, key)
			}
			entry := &record{}
			if got := d.restrictWorkflowTools(req, scope.session, "workflow_child", entry); got != wantOK {
				t.Fatalf("restriction accepted=%v want=%v", got, wantOK)
			}
			proof := entry.snapshot()
			if slices.Contains(proof.VerifiedChecks, "workflow_tool_policy") != (wantOK && wantRestricted) {
				t.Fatal("tool policy check was lost or recorded before enforcement")
			}
			if !wantOK {
				return
			}
			if (len(req.Tools) == 0) != wantRestricted || req.CallableNames()["Read"] == wantRestricted {
				t.Fatal("backend catalogue and callable set disagree with step restriction")
			}
			if !wantRestricted {
				return
			}
			built, err := bridge.BuildRequest(req)
			if err != nil || len(built.Tools) != 0 {
				t.Fatal("restricted backend request", err)
			}
			translator := bridge.NewTranslatorFor(req, scope.route.Model)
			for _, e := range []stream.Event{
				{Type: "response.output_item.added", Raw: []byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc","type":"function_call","call_id":"call_public","name":"Read","arguments":""}}`)},
				{Type: codex.OutputItemDone, Raw: []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc","type":"function_call","call_id":"call_public","name":"Read","arguments":"{}"}}`)},
				{Type: codex.Completed, Raw: []byte(`{"type":"response.completed","response":{"id":"resp_public","status":"completed","usage":{"input_tokens":10,"output_tokens":2},"output":[]}}`)},
			} {
				frames, acceptErr := translator.Accept(e)
				for _, frame := range frames {
					if strings.Contains(string(frame.Data), `"tool_use"`) {
						t.Fatal("prohibited tool reached native client")
					}
				}
				if acceptErr != nil {
					err = acceptErr
					break
				}
			}
			if !errors.Is(err, anthropic.ErrUnsupportedToolCall) {
				t.Fatalf("prohibited call error=%v", err)
			}
		})
	}
}

func TestWorkflowPlanCompletionRequiresEveryResultBody(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for the isolated native-VM compatibility check")
	}
	p := &workflowPlan{Steps: []workflowPlanStep{{ID: "A", Prompt: "PUBLIC", Model: "gpt-6-astra", Effort: "low"}}}
	_, script, _ := strings.Cut(p.script(), "\n")
	source, _ := json.Marshal(script)
	harness := `const vm=require('node:vm'),assert=require('node:assert/strict');const script=` + string(source) + `;(async()=>{for(const value of ['PUBLIC_BODY',null,'','  ',{}]){let calls=0;const c=vm.createContext({agent:async()=>{calls++;return value}});const out=await vm.runInContext('(async()=>{'+script+'})()',c,{timeout:1000});assert.equal(calls,1);assert.equal(out.complete,value==='PUBLIC_BODY');assert.equal(out.completeMeaning,'all_step_results_present_not_task_success');assert.equal(out.results[0].state,value==='PUBLIC_BODY'?'completed':'result_unavailable');}console.log('PLAN_RESULT_CHECK_OK');})().catch(e=>{console.error(e);process.exitCode=1});`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "-e", harness).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PLAN_RESULT_CHECK_OK") {
		t.Fatalf("plan result: %v %s", err, out)
	}
}

func TestWorkflowStepRoleRequiresVerifiedPlanAndChild(t *testing.T) {
	for _, mutation := range []string{"none", "foreign_session", "unknown_child", "ordinary_agent", "inline_script", "unobserved"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, link := planProof(t)
			id := "workflow_child"
			run := d.workflows[delegationKey{scope.session, link.Run}]
			switch mutation {
			case "foreign_session":
				scope.session = "foreign"
			case "unknown_child":
				id = "unknown"
			case "ordinary_agent":
				choice := d.resolved[id]
				choice.role = "general-purpose"
				d.resolved[id] = choice
			case "inline_script":
				run.origin.plan = nil
			case "unobserved":
				run.observed = nil
			}
			d.workflows[delegationKey{link.Session, link.Run}] = run
			req := &bridge.Request{Input: []bridge.InputEntry{
				{Role: "developer", Content: "PUBLIC_NATIVE_WORKER_INSTRUCTION"},
				{Role: "user", Content: "PUBLIC_USER_RESTRICTION"},
				{Role: "user", Content: "PUBLIC_COMPUTED_TASK_CANNOT_EXTEND_USER_SCOPE"},
			}}
			d.describeWorkflowStep(req, scope.session, id)
			want := 3
			if mutation == "none" || mutation == "inline_script" {
				want = 4
			}
			if len(req.Input) != want || req.Input[0].Content != "PUBLIC_NATIVE_WORKER_INSTRUCTION" || req.Input[want-2].Role != "user" || req.Input[want-2].Content != "PUBLIC_USER_RESTRICTION" || req.Input[want-1].Role != "user" || req.Input[want-1].Content != "PUBLIC_COMPUTED_TASK_CANNOT_EXTEND_USER_SCOPE" {
				t.Fatal("worker role escaped the verified plan or changed user content")
			}
			if want == 4 && (req.Input[1].Role != "developer" || req.Input[1].Content != workflowStepInstruction) {
				t.Fatal("worker instruction missing")
			}
		})
	}
}

func planProof(t *testing.T) (*delegations, delegationScope, workflowLink) {
	t.Helper()
	d, scope, child, link := workflowProof(t)
	delete(d.workflowCalls, delegationKey{scope.session, link.Call})
	input := json.RawMessage(`{"script":"clauduct:plan-v1","args":{"steps":[{"id":"A","prompt":"PUBLIC_COMPLETED"},{"id":"B","prompt":"PUBLIC_NOT_STARTED","tools":[]}]}}`)
	adapted, err := d.adaptWorkflow(scope, link.Call, input)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Script string }
	_ = json.Unmarshal(adapted, &out)
	if err = os.WriteFile(link.Script, []byte(out.Script), 0600); err != nil {
		t.Fatal(err)
	}
	if err = d.linkWorkflow(link); err != nil {
		t.Fatal(err)
	}
	if string(d.restoreWorkflowScript(scope.session, link.Call, adapted)) != string(input) {
		t.Fatal("generated plan entered model history")
	}
	label := `clauduct-step:A [clauduct:["workflow_call","gpt-6-astra","low"]]`
	started, _ := json.Marshal(map[string]string{"type": "started", "key": "v2:" + strings.Repeat("a", 64), "agentId": child.ID, "label": label})
	result, _ := json.Marshal(map[string]any{"type": "result", "key": "v2:" + strings.Repeat("a", 64), "agentId": child.ID, "result": "PUBLIC_REPORT"})
	journal := append([]byte("{\"type\":\"launched\"}\n"), started...)
	journal = append(journal, '\n')
	journal = append(journal, result...)
	journal = append(journal, '\n')
	if err = os.WriteFile(filepath.Join(link.Directory, "journal.jsonl"), journal, 0600); err != nil {
		t.Fatal(err)
	}
	d.resolved[child.ID] = resolvedChoice{session: scope.session, call: link.Call, role: child.Role, route: scope.route}
	run := d.workflows[delegationKey{scope.session, link.Run}]
	run.observed = map[string]workflowObservation{child.ID: {Key: "v2:" + strings.Repeat("a", 64), Label: label}}
	d.workflows[delegationKey{scope.session, link.Run}] = run
	d.results.entries = map[string]*agentResult{child.ID: {AgentResultRecord: AgentResultRecord{Session: scope.session, Agent: child.ID, NativeEndObserved: true, EndReason: "answer"}, stopped: true}}
	d.events = t.TempDir()
	for name, value := range map[string]any{"workflow-" + link.Run + ".json": map[string]string{"session": scope.session, "run": link.Run, "task": "native_task", "call": link.Call}, "stopped-native_task.json": map[string]string{"session": scope.session, "task": "native_task", "call": "stop_call", "type": "local_workflow"}} {
		raw, _ := json.Marshal(value)
		if err = os.WriteFile(filepath.Join(d.events, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return d, scope, link
}

func TestWorkflowPlanResumeOnlyRunsNeverStartedSteps(t *testing.T) {
	for _, mutation := range []string{"none", "missing_stop", "foreign_stop", "script_changed", "child_live", "missing_result", "duplicate_start", "extra_argument", "removed_started"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, link := planProof(t)
			input := json.RawMessage(`{"resumeFromRunId":"wf_public01"}`)
			switch mutation {
			case "missing_stop":
				d.events = t.TempDir()
			case "removed_started":
				_ = os.WriteFile(filepath.Join(link.Directory, "journal.jsonl"), []byte("{\"type\":\"launched\"}\n"), 0600)
			case "foreign_stop":
				_ = os.WriteFile(filepath.Join(d.events, "stopped-native_task.json"), []byte(`{"session":"foreign","task":"native_task","call":"stop_call","type":"local_workflow"}`), 0600)
			case "script_changed":
				_ = os.WriteFile(link.Script, []byte("altered"), 0600)
			case "child_live":
				d.results.entries["workflow_child"].stopped = false
			case "missing_result":
				p := filepath.Join(link.Directory, "journal.jsonl")
				raw, _ := os.ReadFile(p)
				rows := strings.Split(string(raw), "\n")
				_ = os.WriteFile(p, []byte(strings.Join(rows[:2], "\n")+"\n"), 0600)
				d.results.entries["workflow_child"].EndReason = "aborted"
			case "duplicate_start":
				p := filepath.Join(link.Directory, "journal.jsonl")
				raw, _ := os.ReadFile(p)
				rows := strings.Split(string(raw), "\n")
				_ = os.WriteFile(p, append(raw, []byte(rows[1]+"\n")...), 0600)
			case "extra_argument":
				input = json.RawMessage(`{"resumeFromRunId":"wf_public01","script":"must not run"}`)
			}
			adapted, err := d.adaptWorkflow(scope, "continue_call", input)
			if mutation != "none" && mutation != "missing_result" {
				if err == nil {
					t.Fatal("unverified continuation accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			origin := d.workflowCalls[delegationKey{scope.session, "continue_call"}]
			if len(origin.plan.Steps) != 1 || origin.plan.Steps[0].ID != "B" || origin.plan.Steps[0].Model != scope.route.Model || len(origin.plan.Prior) != 1 {
				t.Fatal("wrong remaining work")
			}
			if origin.plan.Steps[0].Tools == nil || len(*origin.plan.Steps[0].Tools) != 0 {
				t.Fatal("resume lost the immutable tool restriction")
			}
			want := "completed_result_reused"
			if mutation == "missing_result" {
				want = "started_not_reexecuted"
			}
			if origin.plan.Prior[0].State != want {
				t.Fatal("started work was recreated")
			}
			if string(d.restoreWorkflowScript(scope.session, "continue_call", adapted)) != string(input) {
				t.Fatal("resume history changed")
			}
			if _, err = d.adaptWorkflow(scope, "again", input); err == nil {
				t.Fatal("source run consumed twice")
			}
		})
	}
}

func TestWorkflowPlanConcurrentResumesCannotDuplicateWork(t *testing.T) {
	d, scope, _ := planProof(t)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for _, call := range []string{"one", "two"} {
		wg.Go(func() {
			if _, err := d.adaptWorkflow(scope, call, json.RawMessage(`{"resumeFromRunId":"wf_public01"}`)); err == nil {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("non-atomic source claim", accepted.Load())
	}
}

func TestWorkflowPlanDataValidationAndDefaultEffort(t *testing.T) {
	parent := bridge.Route{Model: "gpt-6-astra", Effort: "low"}
	for _, bad := range []string{
		`{"id":"A","prompt":"public","model":null}`, `{"id":"A","prompt":"public","effort":null}`, `{"id":"A","prompt":"public","tools":null}`, `{"id":"A","prompt":"public","tools":["Read","Read"]}`, `{"id":"A","prompt":"public","tools":"none"}`, `{"id":"A","prompt":"public","model":"unknown"}`, `{"id":"../A","prompt":"public"}`, `{"id":"A","prompt":""}`,
	} {
		if _, err := parseWorkflowPlan([]byte(`{"script":"clauduct:plan-v1","args":{"steps":[`+bad+`]}}`), parent); err == nil {
			t.Fatal("invalid step accepted")
		}
	}
	raw := []byte(`{"script":"clauduct:plan-v1","args":{"steps":[{"id":"A","prompt":"'); throw Error('inert'); //","model":"gpt-5.6-luna"}]}}`)
	plan, err := parseWorkflowPlan(raw, parent)
	if err != nil || plan.Steps[0].Effort != "max" {
		t.Fatal("model default lost")
	}
	if !strings.Contains(plan.script(), `agent("'); throw Error('inert'); //",`) {
		t.Fatal("prompt escaped its JSON value")
	}
}

func TestWorkflowAllowlistNarrowsCatalogueAndNewCalls(t *testing.T) {
	for _, inline := range []bool{false, true} {
		d, scope, link := planProof(t)
		key := delegationKey{scope.session, link.Run}
		run := d.workflows[key]
		tools := []string{"Read"}
		if inline {
			run.origin.plan = nil
			run.observed["workflow_child"] = workflowObservation{Label: `proof [clauduct:["workflow_call",null,null,{"tools":["Read"]}]]`}
		} else {
			run.origin.plan.Steps[0].Tools = &tools
		}
		d.workflows[key] = run
		req := &anthropic.Request{Tools: []anthropic.Tool{{Name: "Read"}, {Name: "Write"}}, Discovered: map[string]bool{"Write": true}, ToolChoice: anthropic.ToolChoice{Type: "tool", Name: "Read"}}
		if !d.restrictWorkflowTools(req, scope.session, "workflow_child", &record{}) || len(req.Tools) != 1 || !req.CallableNames()["Read"] || req.CallableNames()["Write"] {
			t.Fatal("history or omitted tools escaped allowlist")
		}
		req.ToolChoice.Name = "Write"
		if d.restrictWorkflowTools(req, scope.session, "workflow_child", &record{}) {
			t.Fatal("forced excluded call accepted")
		}
	}
}
