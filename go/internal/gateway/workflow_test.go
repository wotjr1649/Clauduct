package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func workflowProof(t *testing.T) (*delegations, delegationScope, agentBinding, workflowLink) {
	t.Helper()
	root := t.TempDir()
	d := &delegations{projects: root, resolved: map[string]resolvedChoice{}, pending: map[delegationKey]delegatedChoice{}}
	scope := delegationScope{session: "workflow_session", route: bridge.Route{Model: "gpt-6-astra", Effort: "low"}}
	base := filepath.Join(root, "project", scope.session)
	link := workflowLink{Session: scope.session, Call: "workflow_call", Run: "wf_public01", Name: "proof", Transcript: base + ".jsonl", Directory: filepath.Join(base, "subagents", "workflows", "wf_public01"), Script: filepath.Join(base, "workflows", "scripts", "proof-wf_public01.js")}
	for _, p := range []string{link.Directory, filepath.Dir(link.Script)} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	const script = "public fixture, never executed"
	raw, _ := json.Marshal(map[string]string{"script": script})
	if err := d.prepareWorkflow(scope, link.Call, raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link.Script, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	child := agentBinding{ID: "workflow_child", Role: "workflow-subagent", SessionID: scope.session, TranscriptPath: link.Transcript}
	files := map[string]string{
		"journal.jsonl":                  `{"type":"launched"}` + "\n" + `{"type":"started","key":"v2:` + strings.Repeat("a", 64) + `","agentId":"workflow_child","label":"proof"}` + "\n",
		"agent-workflow_child.meta.json": `{"agentType":"workflow-subagent","description":"proof","spawnDepth":1}`,
		"agent-workflow_child.jsonl":     `{"type":"user","sessionId":"workflow_session","agentId":"workflow_child","timestamp":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}` + "\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(link.Directory, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return d, scope, child, link
}

func TestWorkflowSelectionRequiresOriginAndUnmodifiedNativeEvidence(t *testing.T) {
	for _, mutation := range []string{"none", "script", "call", "directory", "duplicate", "model", "session", "stopped"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, binding, link := workflowProof(t)
			if mutation == "call" {
				link.Call = "other"
			}
			if mutation == "directory" {
				link.Directory = t.TempDir()
			}
			err := d.linkWorkflow(link)
			if mutation == "call" || mutation == "directory" {
				if err == nil {
					t.Fatal("uncorrelated link accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "script":
				err = os.WriteFile(link.Script, []byte("changed"), 0600)
			case "duplicate":
				path := filepath.Join(link.Directory, "journal.jsonl")
				raw, _ := os.ReadFile(path)
				err = os.WriteFile(path, append(raw, []byte(`{"type":"started","agentId":"workflow_child"}`+"\n")...), 0600)
			case "model":
				err = os.WriteFile(filepath.Join(link.Directory, "agent-workflow_child.meta.json"), []byte(`{"agentType":"workflow-subagent","description":"proof","spawnDepth":1,"model":"gpt-5.6-sol"}`), 0600)
			case "session":
				binding.SessionID = "other"
			case "stopped":
				err = os.WriteFile(filepath.Join(link.Directory, "agent-workflow_child.meta.json"), []byte(`{"agentType":"workflow-subagent","description":"proof","spawnDepth":1,"stoppedByUser":true}`), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			route, found, err := d.workflowRoute(context.Background(), scope, binding.ID, binding)
			if mutation == "none" {
				if err != nil || !found || route.Model != scope.route.Model || route.Effort != scope.route.Effort || route.Source != "workflow-parent" {
					t.Fatal("verified parent not retained")
				}
			} else if err == nil || found {
				t.Fatal("altered evidence accepted")
			}
		})
	}
}
