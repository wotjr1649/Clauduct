package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func TestRejectedWorkflowCannotExecuteOriginalAndRetainsErrorHistory(t *testing.T) {
	d, scope, _, _ := workflowProof(t)
	raw := json.RawMessage(`{"resumeFromRunId":"wf_unknown","script":"agent('DO_NOT_EXECUTE')"}`)
	adapted, err := d.rejectWorkflow(scope, "rejected", raw)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if json.Unmarshal(adapted, &fields) != nil || len(fields) != 1 || fields["script"] != bridge.RejectedWorkflowScript {
		t.Fatal("original operation reached native")
	}
	r := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "tool_result", ToolUseID: "rejected", IsError: true}}}}}
	d.toolFailures(scope.session, r)
	if string(d.restoreWorkflowScript(scope.session, "rejected", adapted)) != string(raw) {
		t.Fatal("denial lost original tool-call history")
	}
	if len(d.resolved) != 0 || len(d.pending) != 0 {
		t.Fatal("rejected operation created an agent")
	}
}

func TestWorkflowRecoveryDoesNotReexecuteUncertainTasks(t *testing.T) {
	for _, mutation := range []string{"none", "missing_result", "unfinished", "wrong_session", "changed_script", "new_script", "path", "unknown_run", "forged_body", "foreign_selection"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, child, link := workflowProof(t)
			if err := d.linkWorkflow(link); err != nil {
				t.Fatal(err)
			}
			if _, found, err := d.findWorkflow(context.Background(), scope, child.ID, child); err != nil || !found {
				t.Fatal(err)
			}
			e := d.results.entries[child.ID]
			e.NativeTurn, e.NativeEndObserved, e.EndReason, e.stopped = "turn", true, "answer", true
			body := `{"public":23}`
			if mutation == "forged_body" {
				body = `"'); agent('DO_NOT_EXECUTE'); //"`
			}
			p := filepath.Join(link.Directory, "journal.jsonl")
			before, _ := os.ReadFile(p)
			row := `{"type":"result","key":"v2:` + strings.Repeat("a", 64) + `","agentId":"workflow_child","result":` + body + `}` + "\n"
			if mutation != "missing_result" {
				if err := os.WriteFile(p, append(before, []byte(row)...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			input := map[string]any{"resumeFromRunId": link.Run}
			switch mutation {
			case "unfinished":
				e.NativeEndObserved = false
			case "wrong_session":
				scope.session = "other"
			case "changed_script":
				if err := os.WriteFile(link.Script, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "new_script":
				input["script"] = "return agent('must not run')"
			case "path":
				input["scriptPath"] = link.Script
			case "unknown_run":
				input["resumeFromRunId"] = "wf_unknown"
			case "foreign_selection":
				c := d.resolved[child.ID]
				c.call = "other"
				d.resolved[child.ID] = c
			}
			raw, _ := json.Marshal(input)
			adapted, err := d.adaptWorkflow(scope, "recover_call", raw)
			if mutation == "wrong_session" || mutation == "changed_script" || mutation == "new_script" || mutation == "path" || mutation == "unknown_run" {
				if err == nil {
					t.Fatal("unverified recovery accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var out struct{ Script string }
			if json.Unmarshal(adapted, &out) != nil {
				t.Fatal("script")
			}
			prefix := "export const meta = {name:'clauduct-recovered-results',description:'Return verified stored child reports; no task is re-executed'};\nreturn JSON.parse("
			if !strings.HasPrefix(out.Script, prefix) || !strings.HasSuffix(out.Script, ");") {
				t.Fatal("not data-only")
			}
			var payload string
			if json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(out.Script, prefix), ");")), &payload) != nil {
				t.Fatal("body escaped literal")
			}
			var report struct {
				NewAgentExecutions int
				Results            []struct{ State, Body string }
			}
			if json.Unmarshal([]byte(payload), &report) != nil || report.NewAgentExecutions != 0 || len(report.Results) != 1 {
				t.Fatal("report")
			}
			want := "completed_result_reused"
			if mutation == "missing_result" || mutation == "unfinished" || mutation == "foreign_selection" {
				want = "result_unavailable"
			}
			if report.Results[0].State != want {
				t.Fatal(report)
			}
			if string(d.restoreWorkflowScript(scope.session, "recover_call", adapted)) != string(raw) {
				t.Fatal("generated recovery data entered tool input history")
			}
			if len(d.resolved) != 1 || len(d.pending) != 0 {
				t.Fatal("recovery started a child")
			}
		})
	}
}
