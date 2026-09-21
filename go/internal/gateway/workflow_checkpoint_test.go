package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkpointProof(t *testing.T) (*delegations, delegationScope, workflowLink) {
	t.Helper()
	d, scope, link := planProof(t)
	c := d.resolved["workflow_child"]
	c.route.Source = "workflow-selection"
	d.resolved["workflow_child"] = c
	d.results.entries["workflow_child"].NativeTurn = "public_turn"
	meta, _ := json.Marshal(map[string]any{"agentType": "workflow-subagent", "description": d.workflows[delegationKey{scope.session, link.Run}].observed["workflow_child"].Label, "model": scope.route.Model, "spawnDepth": 1})
	if err := os.WriteFile(filepath.Join(link.Directory, "agent-workflow_child.meta.json"), meta, 0600); err != nil {
		t.Fatal(err)
	}
	d.saveWorkflowCheckpoints()
	if d.workflowPersistence.Saved != 1 || d.workflowPersistence.Failed != 0 {
		t.Fatal("checkpoint not saved", d.workflowPersistence)
	}
	rel, _ := filepath.Rel(d.projects, link.Transcript)
	restart := &delegations{projects: d.projects, events: t.TempDir(), workflowSessions: map[string]string{scope.session: rel}, resolved: map[string]resolvedChoice{}, pending: map[delegationKey]delegatedChoice{}}
	return restart, scope, link
}

func TestWorkflowCheckpointRestoresOnlyVerifiedNeverStartedWork(t *testing.T) {
	for _, mutation := range []string{"none", "script", "journal", "metadata", "missing", "foreign_session", "bad_offsets", "bad_tools", "unfinished", "duplicate_json"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, link := checkpointProof(t)
			path := link.Script + ".clauduct-workflow.json"
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "PUBLIC_COMPLETED") || strings.Contains(string(raw), "PUBLIC_NOT_STARTED") || strings.Contains(string(raw), "PUBLIC_REPORT") {
				t.Fatal("prompt/result duplicated in metadata")
			}
			var saved workflowCheckpoint
			_ = json.Unmarshal(raw, &saved)
			switch mutation {
			case "script":
				err = os.WriteFile(link.Script, []byte("changed"), 0600)
			case "journal":
				err = os.WriteFile(filepath.Join(link.Directory, "journal.jsonl"), []byte("changed"), 0600)
			case "metadata":
				err = os.WriteFile(filepath.Join(link.Directory, "agent-workflow_child.meta.json"), []byte("changed"), 0600)
			case "missing":
				err = os.Remove(path)
			case "foreign_session":
				scope.session = "foreign"
			case "bad_offsets":
				(*saved.Plan)[0].Start++
				raw, _ = json.Marshal(saved)
				err = os.WriteFile(path, raw, 0600)
			case "bad_tools":
				tools := []string{"Bash"}
				(*saved.Plan)[1].Tools = &tools
				raw, _ = json.Marshal(saved)
				err = os.WriteFile(path, raw, 0600)
			case "unfinished":
				saved.Children[0].Ended = false
				raw, _ = json.Marshal(saved)
				err = os.WriteFile(path, raw, 0600)
			case "duplicate_json":
				err = os.WriteFile(path, append(raw, []byte("{}")...), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.adaptWorkflow(scope, "restart_continue", json.RawMessage(`{"resumeFromRunId":"wf_public01"}`))
			if mutation != "none" {
				if err == nil {
					t.Fatal("unverified checkpoint resumed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			plan := d.workflowCalls[delegationKey{scope.session, "restart_continue"}].plan
			if len(plan.Steps) != 1 || plan.Steps[0].ID != "B" || plan.Steps[0].Prompt != "PUBLIC_NOT_STARTED" || plan.Steps[0].Tools == nil || len(*plan.Steps[0].Tools) != 0 || len(plan.Prior) != 1 || plan.Prior[0].Body != "PUBLIC_REPORT" {
				t.Fatal("lost result or changed remaining work")
			}
			if d.workflowPersistence.Restored != 1 {
				t.Fatal("restore not observed")
			}
		})
	}
}

func TestWorkflowCheckpointClaimSurvivesSeparateGateways(t *testing.T) {
	d, scope, link := checkpointProof(t)
	rel, _ := filepath.Rel(d.projects, link.Transcript)
	second := &delegations{projects: d.projects, events: t.TempDir(), workflowSessions: map[string]string{scope.session: rel}, resolved: map[string]resolvedChoice{}, pending: map[delegationKey]delegatedChoice{}}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i, g := range []*delegations{d, second} {
		wg.Go(func() {
			call := []string{"first", "second"}[i]
			if _, err := g.adaptWorkflow(scope, call, json.RawMessage(`{"resumeFromRunId":"wf_public01"}`)); err == nil {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("cross-gateway duplicate continuation", accepted.Load())
	}
}

func TestWorkflowCheckpointWriteFailureIsObservable(t *testing.T) {
	d, _, link := planProof(t)
	if err := os.Mkdir(link.Script+".clauduct-workflow.json", 0700); err != nil {
		t.Fatal(err)
	}
	d.saveWorkflowCheckpoints()
	if d.workflowPersistence.Failed != 1 || d.workflowPersistence.Saved != 0 {
		t.Fatal("failed persistence claimed success")
	}
}

func TestWorkflowCheckpointPromptOffsetsIgnoreQuotedCodeInPriorResult(t *testing.T) {
	p := &workflowPlan{Prior: []workflowPlanResult{{Step: "old", State: "completed_result_reused", Body: "const result0 = await agent(\"not a prompt\");\n"}}, Steps: []workflowPlanStep{{ID: "next", Prompt: "PUBLIC", Model: "gpt-5.6-luna", Effort: "max"}}}
	offsets, err := checkpointPlan(p.script(), p)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoreCheckpointPlan(p.script(), offsets)
	if err != nil || restored.script() != p.script() {
		t.Fatal("quoted report confused source offsets", err)
	}
}
