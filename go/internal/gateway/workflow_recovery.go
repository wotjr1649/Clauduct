package gateway

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var errWorkflowRecoveryUnverified = errors.New("WORKFLOW_RECOVERY_UNVERIFIED")

// Deliver a native tool denial, never the rejected operation. Keep its original
// history until session end so subsequent turns see a matched call/error pair.
func (d *delegations) rejectWorkflow(scope delegationScope, call string, raw json.RawMessage) (json.RawMessage, error) {
	adapted, _ := json.Marshal(map[string]string{"script": bridge.RejectedWorkflowScript})
	if err := d.prepareWorkflow(scope, call, adapted); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := delegationKey{scope.session, call}
	origin := d.workflowCalls[key]
	origin.rejected = true
	origin.recoveryInput = append(json.RawMessage(nil), raw...)
	d.workflowCalls[key] = origin
	return adapted, nil
}

// Recovery is a data-only Workflow, not replay of arbitrary JavaScript. Native
// still owns its approval and result delivery. Uncertain effects are never retried.
func (d *delegations) recoverWorkflow(scope delegationScope, call string, raw json.RawMessage) (json.RawMessage, error) {
	fields, err := wire.Fields(raw, []string{"resumeFromRunId"})
	var source string
	if err != nil || json.Unmarshal(fields["resumeFromRunId"], &source) != nil || !correlationShape.MatchString(source) || scope.parent != "" {
		return nil, errWorkflowRecoveryUnverified
	}
	d.mu.Lock()
	run, known := d.workflows[delegationKey{scope.session, source}]
	d.mu.Unlock()
	if !known {
		run, err = d.restoreWorkflow(scope.session, source)
		known = err == nil
		if !known {
			d.mu.Lock()
			d.workflowPersistence.Failed++
			d.mu.Unlock()
		}
	}
	if !known || run.origin.rejected {
		return nil, errWorkflowRecoveryUnverified
	}
	if run.origin.plan != nil {
		return d.resumeWorkflowPlan(scope, call, source, raw, run)
	}
	if run.origin.recoveryOf != "" {
		return nil, errWorkflowRecoveryUnverified
	}
	root, err := d.openProjects(".")
	if err != nil {
		return nil, errWorkflowRecoveryUnverified
	}
	defer root.Close()
	script, err := workflowRead(root, run.script, 512<<10)
	if err != nil {
		return nil, errWorkflowRecoveryUnverified
	}
	digest := sha256.Sum256(script)
	if hex.EncodeToString(digest[:]) != run.origin.digest {
		return nil, errWorkflowRecoveryUnverified
	}
	journal, err := workflowRead(root, filepath.Join(run.directory, "journal.jsonl"), 2<<20)
	if err != nil {
		return nil, errWorkflowRecoveryUnverified
	}
	var ids []string
	seen := map[string]bool{}
	scan := bufio.NewScanner(bytes.NewReader(journal))
	scan.Buffer(make([]byte, 4096), resultBodyLimit+16384)
	rows := 0
	for scan.Scan() {
		rows++
		var e struct{ Type, AgentID string }
		if json.Unmarshal(scan.Bytes(), &e) != nil || rows == 1 && e.Type != "launched" || rows > 65536 {
			return nil, errWorkflowRecoveryUnverified
		}
		if e.Type == "started" {
			if !correlationShape.MatchString(e.AgentID) || seen[e.AgentID] || len(ids) >= 16 {
				return nil, errWorkflowRecoveryUnverified
			}
			ids = append(ids, e.AgentID)
			seen[e.AgentID] = true
		}
	}
	if scan.Err() != nil || rows == 0 {
		return nil, errWorkflowRecoveryUnverified
	}
	type item struct {
		Agent string `json:"agentId"`
		State string `json:"state"`
		Body  string `json:"body,omitempty"`
	}
	items := make([]item, 0, len(ids))
	// ponytail: at most 16 bounded scans using the existing result verifier;
	// share a parsed journal if measured large recovery workloads justify it.
	for _, id := range ids {
		entry := item{Agent: id, State: "result_unavailable"}
		d.mu.Lock()
		choice, linked := d.resolved[id]
		d.mu.Unlock()
		d.results.mu.Lock()
		e := d.results.entries[id]
		ended := e != nil && e.Session == scope.session && e.NativeEndObserved && e.EndReason == "answer" && e.stopped
		d.results.mu.Unlock()
		if linked && choice.session == scope.session && choice.call == run.Call && ended {
			if body, ok := d.workflowResult(scope.session, id); ok {
				entry.State, entry.Body = "completed_result_reused", body
			}
		}
		items = append(items, entry)
	}
	payload, _ := json.Marshal(map[string]any{"sourceRunId": source, "mode": "completed_results_only", "originalRunState": "not_assessed", "newAgentExecutions": 0, "results": items})
	if len(payload) > resultBodyLimit {
		return nil, errWorkflowRecoveryUnverified
	}
	quoted, _ := json.Marshal(string(payload))
	adapted, _ := json.Marshal(map[string]string{"script": "export const meta = {name:'clauduct-recovered-results',description:'Return verified stored child reports; no task is re-executed'};\nreturn JSON.parse(" + string(quoted) + ");"})
	if err := d.prepareWorkflow(scope, call, adapted); err != nil {
		return nil, err
	}
	d.mu.Lock()
	origin := d.workflowCalls[delegationKey{scope.session, call}]
	origin.recoveryOf, origin.recoveryInput = source, append(json.RawMessage(nil), raw...)
	d.workflowCalls[delegationKey{scope.session, call}] = origin
	d.mu.Unlock()
	return adapted, nil
}
