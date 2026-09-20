package gateway

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkflowRuntimeOptionsAreBoundBeforeDispatch(t *testing.T) {
	for _, tc := range []struct{ name, model, effort, wantModel, wantEffort string }{
		{"both", "gpt-5.6-terra", "medium", "gpt-5.6-terra", "medium"},
		{"model_only", "gpt-5.6-luna", "", "gpt-5.6-luna", "max"},
		{"effort_only", "", "high", "gpt-6-astra", "high"},
		{"inherit", "", "", "gpt-6-astra", "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, scope, binding, link := workflowProof(t)
			delete(d.workflowCalls, delegationKey{scope.session, link.Call})
			original := `export const meta = {name:'proof',description:'public'}; return await agent('public',{});`
			raw, _ := json.Marshal(map[string]string{"script": original})
			adapted, err := d.adaptWorkflow(scope, link.Call, raw)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]string
			if json.Unmarshal(adapted, &fields) != nil {
				t.Fatal("script")
			}
			for _, r := range fields["script"] {
				if r < 32 && r != '\n' && r != '\t' {
					t.Fatal("hidden control character in native approval input")
				}
			}
			if err = os.WriteFile(link.Script, []byte(fields["script"]), 0600); err != nil {
				t.Fatal(err)
			}
			if err = d.linkWorkflow(link); err != nil {
				t.Fatal(err)
			}
			first, _ := json.Marshal(map[string]any{"type": "user", "sessionId": scope.session, "agentId": binding.ID, "timestamp": time.Now().UTC()})
			if err = os.WriteFile(filepath.Join(link.Directory, "agent-"+binding.ID+".jsonl"), append(first, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			var m, e any
			if tc.model != "" {
				m = tc.model
			}
			if tc.effort != "" {
				e = tc.effort
			}
			parts, _ := json.Marshal([]any{link.Call, m, e})
			label := "proof [clauduct:" + string(parts) + "]"
			journal, _ := json.Marshal(map[string]string{"type": "started", "key": "v2:" + strings.Repeat("a", 64), "agentId": binding.ID, "label": label})
			meta, _ := json.Marshal(map[string]any{"agentType": binding.Role, "description": label, "spawnDepth": 1, "model": tc.wantModel})
			for name, data := range map[string][]byte{"journal.jsonl": append([]byte("{\"type\":\"launched\"}\n"), append(journal, '\n')...), "agent-" + binding.ID + ".meta.json": meta} {
				if err = os.WriteFile(filepath.Join(link.Directory, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "native_turn", Model: tc.wantModel, Effort: tc.wantEffort}
			scope.nativeTurn = &active
			route, found, err := d.findWorkflow(context.Background(), scope, binding.ID, binding)
			if err != nil || !found || route.Model != tc.wantModel || route.Effort != tc.wantEffort || route.Source != "workflow-selection" {
				t.Fatalf("route %+v found=%v error=%v", route, found, err)
			}
			choice := d.resolved[binding.ID]
			if !choice.receipt.PresenceVerified || choice.receipt.ModelProvided != (tc.model != "") || choice.receipt.EffortProvided != (tc.effort != "") {
				t.Fatal("presence not proven")
			}
			restored := d.restoreWorkflowScript(scope.session, link.Call, adapted)
			if json.Unmarshal(restored, &fields) != nil || fields["script"] != original {
				t.Fatal("adapter entered model history")
			}
			for _, bad := range []string{"missing", "session", "agent", "model", "effort", "call"} {
				wrong := active
				badLabel := label
				switch bad {
				case "session":
					wrong.Session = "other"
				case "agent":
					wrong.Agent = "other"
				case "model":
					wrong.Model = "unlisted"
				case "effort":
					wrong.Effort = "unlisted"
				case "call":
					badLabel = strings.Replace(label, link.Call, "other", 1)
				}
				proof := &wrong
				if bad == "missing" {
					proof = nil
				}
				if _, _, err := workflowLabelSelection(badLabel, d.workflows[delegationKey{scope.session, link.Run}], binding.ID, proof); err == nil {
					t.Fatal("invalid evidence accepted", bad)
				}
			}
		})
	}
}

func TestWorkflowWrapperRunsOriginalNativeGlobalWithExactOptions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for the isolated native-VM compatibility check")
	}
	d, scope, _, link := workflowProof(t)
	delete(d.workflowCalls, delegationKey{scope.session, link.Call})
	raw := json.RawMessage(`{"script":"export const meta = {};"}`)
	adapted, err := d.adaptWorkflow(scope, link.Call, raw)
	if err != nil {
		t.Fatal(err)
	}
	var input struct{ Script string }
	_ = json.Unmarshal(adapted, &input)
	wrapper := strings.TrimPrefix(input.Script, "export const meta = {};")
	source, _ := json.Marshal(wrapper)
	harness := `const vm=require('node:vm'),assert=require('node:assert/strict');let calls=[];const context=vm.createContext({agent:(prompt,opts)=>{calls.push({prompt,opts});return 'native-result'}});const wrap=` + string(source) + `;function run(options){return vm.runInContext('(function(){return agent("public",'+JSON.stringify(options)+');'+wrap+'})()',context,{timeout:1000});}for(const [opts,model,effort] of [[{},'gpt-6-astra','low'],[{model:'luna'},'gpt-5.6-luna','max'],[{effort:'high'},'gpt-6-astra','high'],[{agentType:'custom'},undefined,undefined],[{tools:['Read']},'gpt-6-astra','low'],[{model:'gpt-5.6-terra',effort:'medium',label:'public',phase:'public'},'gpt-5.6-terra','medium']]){assert.equal(run(opts),'native-result');const last=calls.at(-1);assert.equal(last.opts.model,model);assert.equal(last.opts.effort,effort);assert.equal(last.prompt,'public');if(opts.phase)assert.equal(last.opts.phase,'public');}let count=calls.length;for(const opts of [{model:null},{effort:null},{model:'__proto__'},{model:'unknown'},{effort:'invalid'},{agentType:null},{tools:['*']},{tools:['Read','Read']},{maxTurns:2},{tools:null},{maxTurns:null}])assert.throws(()=>run(opts));assert.equal(calls.length,count);console.log('WORKFLOW_WRAPPER_CHECK_OK');`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "-e", harness)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "WORKFLOW_WRAPPER_CHECK_OK") {
		t.Fatalf("wrapper: %v %s", err, out)
	}
}
