package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

func TestNativeContinuationRequiresCurrentTurnAndOriginalLineage(t *testing.T) {
	for _, mutation := range []string{"none", "missing_receipt", "stale_turn", "wrong_session", "wrong_agent", "wrong_model", "wrong_metadata", "wrong_parent_header", "live_child", "stopped", "no_wait"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			scope.parent = "proof_parent"
			key := delegationKey{scope.session, "proof_call"}
			choice := d.pending[key]
			choice.parent, choice.receipt.Parent = scope.parent, scope.parent
			d.pending[key] = choice
			metaPath := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
			meta := delegationMetadata{ToolUseID: "proof_call", ParentAgentID: scope.parent, AgentType: binding.Role, Model: "haiku"}
			put := func(name string, value any) {
				t.Helper()
				raw, _ := json.Marshal(value)
				if err := os.WriteFile(name, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			put(metaPath, meta)
			if _, _, err := d.route(scope, binding.ID, binding); err != nil {
				t.Fatal(err)
			}
			g := &Gateway{delegations: d, agents: newAgentRegistry()}
			if _, err := g.agents.register(binding, time.Now()); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			g.ConfigureNativeEvents(dir)
			e := d.results.entries[binding.ID]
			e.NativeTurn, e.NativeEndObserved, e.EndReason, e.State = "old_turn", true, "answer", "awaiting_children"
			d.results.start("leaf", resolvedChoice{session: scope.session, parent: binding.ID, call: "leaf_call"})
			leaf := d.results.entries["leaf"]
			leaf.stopped, leaf.State = true, "awaiting_parent"
			active := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "new_turn", Model: "gpt-5.6-luna", Effort: "low"}
			req := httptest.NewRequest("POST", "/v1/messages", nil)
			req.Header.Set("X-Claude-Code-Session-Id", scope.session)
			req.Header.Set("X-Claude-Code-Agent-Id", binding.ID)
			switch mutation {
			case "stale_turn":
				active.Turn = "old_turn"
			case "wrong_session":
				active.Session = "other"
			case "wrong_agent":
				active.Agent = "other"
			case "wrong_model":
				active.Model = "gpt-6-astra"
			case "wrong_metadata":
				meta.ParentAgentID = "other"
				put(metaPath, meta)
			case "wrong_parent_header":
				req.Header.Set("X-Claude-Code-Parent-Agent-Id", "other")
			case "live_child":
				leaf.stopped = false
			case "stopped":
				e.stopped = true
			case "no_wait":
				e.State = "running"
			}
			if mutation != "missing_receipt" {
				put(filepath.Join(dir, "active-"+binding.ID+".json"), active)
			}
			record := &record{}
			routes, release, err := g.agentSelection(req, &anthropic.Request{Model: "gpt-5.6-luna"}, record)
			release()
			if mutation != "none" {
				if err == nil || len(routes) != 0 {
					t.Fatal("unverified continuation accepted")
				}
				return
			}
			if err != nil || len(routes) != 1 || routes[0].Source != "verified-continuation" || routes[0].Effort != "max" || record.data.ParentAgentID != "" || record.data.VerifiedParentAgentID != scope.parent {
				t.Fatalf("continuation: %v %+v %+v", err, routes, record.data)
			}
			if !d.results.begin(binding.ID) || !g.bindNativeTurn(scope.session, binding.ID) {
				t.Fatal("new turn not bound")
			}
			routes, release, err = g.agentSelection(req, &anthropic.Request{Model: "gpt-5.6-luna"}, record)
			release()
			if err != nil || len(routes) != 1 {
				t.Fatal("same live continuation rejected", err)
			}
			e.NativeEndObserved, e.EndReason, e.State = true, "answer", "awaiting_children"
			_, release, err = g.agentSelection(req, &anthropic.Request{Model: "gpt-5.6-luna"}, record)
			release()
			if err == nil {
				t.Fatal("ended turn reused")
			}
		})
	}
}
