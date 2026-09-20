package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func TestWorkflowStructuredResultArrivesAfterNativeStop(t *testing.T) {
	for _, order := range []string{"stop_first", "journal_first", "empty_terminal", "wrong_key", "duplicate", "duplicate_empty", "changed_script", "foreign_agent"} {
		t.Run(order, func(t *testing.T) {
			d, scope, binding, link := workflowProof(t)
			if err := d.linkWorkflow(link); err != nil {
				t.Fatal(err)
			}
			if _, found, err := d.findWorkflow(context.Background(), scope, binding.ID, binding); err != nil || !found {
				t.Fatal("selection", err)
			}
			g := &Gateway{delegations: d}
			e := d.results.entries[binding.ID]
			e.NativeTurn, e.NativeEndObserved, e.EndReason = "native_turn", true, "answer"
			e.State = "awaiting_native_stop"
			if order != "journal_first" && order != "empty_terminal" {
				d.stopped(binding)
			}
			g.reconcileWorkflowResults(false)
			if e.Recovered || e.State == "result_unavailable" {
				t.Fatal("early stop declared journal missing")
			}
			key := "v2:" + strings.Repeat("a", 64)
			id := binding.ID
			if order == "wrong_key" {
				key = "v2:" + strings.Repeat("b", 64)
			}
			if order == "foreign_agent" {
				id = "other_child"
			}
			var result any = map[string]int{"value": 23}
			if order == "empty_terminal" || order == "duplicate_empty" {
				result = ""
			}
			row, _ := json.Marshal(map[string]any{"type": "result", "agentId": id, "key": key, "result": result})
			p := filepath.Join(link.Directory, "journal.jsonl")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, append(row, '\n')...)
			if order == "duplicate" || order == "duplicate_empty" {
				raw = append(raw, append(row, '\n')...)
			}
			if err = os.WriteFile(p, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if order == "changed_script" {
				if err = os.WriteFile(link.Script, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			g.reconcileWorkflowResults(false)
			if order == "empty_terminal" {
				if !e.stopped || e.State != "result_unavailable" || e.Bytes != 0 {
					t.Fatal("terminal empty result remained pending", e.AgentResultRecord)
				}
				req := &anthropic.Request{}
				d.results.deliver(req, scope.session, "")(true)
				if e.State != "unavailable_reported" || len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Blocks[0].Text, "결과 미확보") {
					t.Fatal("empty terminal not reported to parent")
				}
				return
			}
			if order != "stop_first" && order != "journal_first" {
				if e.body != "" {
					t.Fatal("unverified result delivered")
				}
				g.reconcileWorkflowResults(true)
				if e.State != "result_unavailable" {
					t.Fatal("missing final result not reported")
				}
				return
			}
			if e.body != `{"value":23}` || e.Source != "workflow_journal" || !e.Recovered || e.State != "awaiting_parent" {
				t.Fatalf("structured body lost %+v", e.AgentResultRecord)
			}
			req := &anthropic.Request{}
			d.results.deliver(req, scope.session, "")(true)
			if e.State != "parent_received" || len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Blocks[0].Text, `\"value\":23`) {
				t.Fatal("structured result not delivered")
			}
		})
	}
}
