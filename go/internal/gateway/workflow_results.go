package gateway

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// StructuredOutput ends a Workflow child without an assistant text answer.
// Native persists the validated result after SubagentStop, so that earlier
// event must not attempt text recovery or declare the result lost.
// ponytail: one bounded journal scan per completed child; share scans per run
// only if large parallel Workflows show measured I/O overhead.
func (d *delegations) workflowResult(session, id string) (string, bool) {
	body, finished := d.workflowOutcome(session, id)
	return body, finished && strings.TrimSpace(body) != ""
}

// An empty persisted result proves termination, not a usable report. Keeping
// those two facts separate prevents an already-ended worker from waiting forever.
func (d *delegations) workflowOutcome(session, id string) (string, bool) {
	d.mu.Lock()
	choice, known := d.resolved[id]
	var run workflowRun
	found := false
	for _, r := range d.workflows {
		if r.Session == session && r.Call == choice.call {
			if found {
				d.mu.Unlock()
				return "", false
			}
			run, found = r, true
		}
	}
	d.mu.Unlock()
	if !known || !found || choice.session != session || !choice.isWorkflow() {
		return "", false
	}
	root, err := os.OpenRoot(d.projects)
	if err != nil {
		return "", false
	}
	defer root.Close()
	script, err := workflowRead(root, run.script, 512<<10)
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(script)
	if hex.EncodeToString(digest[:]) != run.origin.digest {
		return "", false
	}
	raw, err := workflowRead(root, filepath.Join(run.directory, "journal.jsonl"), 16<<20)
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), resultBodyLimit+16384)
	key, body := "", ""
	resultSeen := false
	rows := 0
	for scanner.Scan() {
		rows++
		if rows > 65536 {
			return "", false
		}
		var e struct {
			Type, Key, AgentID string
			Result             json.RawMessage
		}
		if json.Unmarshal(scanner.Bytes(), &e) != nil || rows == 1 && e.Type != "launched" {
			return "", false
		}
		if e.AgentID != id {
			continue
		}
		switch e.Type {
		case "started":
			if key != "" || !strings.HasPrefix(e.Key, "v2:") || len(e.Key) != 67 {
				return "", false
			}
			if _, err := hex.DecodeString(e.Key[3:]); err != nil {
				return "", false
			}
			key = e.Key
		case "result":
			if key == "" || e.Key != key || resultSeen || len(e.Result) == 0 || len(e.Result) > resultBodyLimit || string(e.Result) == "null" {
				return "", false
			}
			if json.Unmarshal(e.Result, &body) != nil {
				var compact bytes.Buffer
				if json.Compact(&compact, e.Result) != nil {
					return "", false
				}
				body = compact.String()
			}
			resultSeen = true
		default:
			return "", false
		}
	}
	return body, scanner.Err() == nil && resultSeen
}

// Read on actual parent requests and finalization, never by model polling or
// rerunning a child. A not-yet-persisted journal stays explicitly pending.
func (g *Gateway) reconcileWorkflowResults(final bool) {
	d := g.delegations
	if d == nil {
		return
	}
	r := &d.results
	r.mu.Lock()
	var pending []*agentResult
	for _, e := range r.entries {
		if (e.State == "awaiting_workflow_result" || e.State == "awaiting_native_stop" && strings.HasPrefix(e.Selection.Source, "workflow-")) && e.NativeEndObserved && e.EndReason == "answer" {
			pending = append(pending, e)
		}
	}
	r.mu.Unlock()
	for _, e := range pending {
		body, ok := d.workflowOutcome(e.Session, e.Agent)
		r.mu.Lock()
		if r.entries[e.Agent] == e && (e.State == "awaiting_workflow_result" || e.State == "awaiting_native_stop") && (ok || final) {
			e.stopped = true
			e.Recovered = true
			if ok {
				r.body(e, body, "workflow_journal")
			}
			if e.body != "" {
				r.change(e, "awaiting_parent")
			} else {
				r.change(e, "result_unavailable")
			}
		}
		r.mu.Unlock()
	}
}
