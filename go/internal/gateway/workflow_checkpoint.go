package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Saved only after native has been reaped. Native owns scripts and result bodies;
// this record contains routing/termination evidence and offsets into that script.
// It is not a signature against an attacker able to replace all same-user files.
type workflowCheckpoint struct {
	Version                              int
	Link                                 workflowLink
	Model, Effort, Digest, JournalDigest string
	Created                              time.Time
	AdapterBytes                         int
	RecoveryOf, ContinuedBy              string
	Plan                                 *[]workflowStepOffset
	Children                             []workflowChildProof
}
type workflowStepOffset struct {
	ID, Model, Effort string
	Tools             *[]string
	Start, Length     int
}
type workflowChildProof struct {
	Agent, Role, Model, Effort, Source, Turn, Reason, MetaDigest string
	Key                                                          string
	Ended                                                        bool
}
type WorkflowPersistenceReport struct {
	Saved    int64 `json:"saved"`
	Restored int64 `json:"restored"`
	Failed   int64 `json:"failed"`
}

func workflowDigest(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// No prompt is duplicated in the checkpoint. Rebuild only our canonical plan
// compiler output, never evaluate JavaScript found on disk.
func checkpointPlan(script string, plan *workflowPlan) (*[]workflowStepOffset, error) {
	if plan == nil {
		return nil, nil
	}
	if !strings.HasPrefix(script, plan.script()) {
		return nil, errWorkflowRecoveryUnverified
	}
	steps := make([]workflowStepOffset, 0, len(plan.Steps))
	for i, s := range plan.Steps {
		prefix := "\nconst result" + strconv.Itoa(i) + " = await agent("
		start := strings.Index(script, prefix)
		text, _ := json.Marshal(s.Prompt)
		if start < 0 {
			return nil, errWorkflowRecoveryUnverified
		}
		start += len(prefix)
		if !strings.HasPrefix(script[start:], string(text)) {
			return nil, errWorkflowRecoveryUnverified
		}
		steps = append(steps, workflowStepOffset{s.ID, s.Model, s.Effort, s.Tools, start, len(text)})
	}
	return &steps, nil
}

func restoreCheckpointPlan(script string, offsets *[]workflowStepOffset) (*workflowPlan, error) {
	if offsets == nil {
		return nil, nil
	}
	if len(*offsets) > 16 {
		return nil, errWorkflowRecoveryUnverified
	}
	p := &workflowPlan{}
	_, rest, found := strings.Cut(script, "\nconst results = ")
	if !found {
		return nil, errWorkflowRecoveryUnverified
	}
	prior, _, found := strings.Cut(rest, ";\n")
	if !found || json.Unmarshal([]byte(prior), &p.Prior) != nil || len(p.Prior) > 16 {
		return nil, errWorkflowRecoveryUnverified
	}
	seen := map[string]bool{}
	for _, s := range *offsets {
		if !correlationShape.MatchString(s.ID) || seen[s.ID] || s.Start < 0 || s.Length < 2 || s.Length > len(script) || s.Start > len(script)-s.Length || s.Tools != nil && !validWorkflowTools(s.Tools) {
			return nil, errWorkflowRecoveryUnverified
		}
		seen[s.ID] = true
		step := workflowPlanStep{ID: s.ID, Model: s.Model, Effort: s.Effort, Tools: s.Tools}
		if json.Unmarshal([]byte(script[s.Start:s.Start+s.Length]), &step.Prompt) != nil {
			return nil, errWorkflowRecoveryUnverified
		}
		route, err := bridge.SelectRoute(s.Model, s.Effort)
		if err != nil || route.Model != s.Model || s.Effort == "" {
			return nil, errWorkflowRecoveryUnverified
		}
		p.Steps = append(p.Steps, step)
	}
	if p.script() != script {
		return nil, errWorkflowRecoveryUnverified
	}
	return p, nil
}

func (d *delegations) saveWorkflowCheckpoints() {
	d.mu.Lock()
	defer d.mu.Unlock()
	root, err := os.OpenRoot(d.projects)
	if err != nil {
		if len(d.workflows) > 0 {
			d.workflowPersistence.Failed++
		}
		return
	}
	defer root.Close()
	for _, run := range d.workflows {
		if run.origin.rejected {
			continue
		}
		if err := d.saveWorkflowCheckpoint(root, run); err != nil {
			d.workflowPersistence.Failed++
		} else {
			d.workflowPersistence.Saved++
		}
	}
}

// Caller holds d.mu, after native exit and gateway drain.
func (d *delegations) saveWorkflowCheckpoint(root *os.Root, run workflowRun) error {
	script, err := workflowRead(root, run.script, 512<<10)
	if err != nil || workflowDigest(script) != run.origin.digest {
		return errWorkflowRecoveryUnverified
	}
	journal, err := workflowRead(root, filepath.Join(run.directory, "journal.jsonl"), 2<<20)
	if err != nil {
		return errWorkflowRecoveryUnverified
	}
	plan, err := checkpointPlan(string(script), run.origin.plan)
	if err != nil {
		return err
	}
	saved := workflowCheckpoint{Version: 1, Link: run.workflowLink, Model: run.origin.scope.route.Model, Effort: run.origin.scope.route.Effort, Digest: run.origin.digest, JournalDigest: workflowDigest(journal), Created: run.origin.created, AdapterBytes: run.origin.adapterBytes, RecoveryOf: run.origin.recoveryOf, ContinuedBy: run.continuedBy, Plan: plan}
	for id, observation := range run.observed {
		choice, ok := d.resolved[id]
		if !ok || choice.session != run.Session || choice.call != run.Call {
			return errWorkflowRecoveryUnverified
		}
		meta, err := workflowRead(root, filepath.Join(run.directory, "agent-"+id+".meta.json"), 16384)
		if err != nil {
			return errWorkflowRecoveryUnverified
		}
		child := workflowChildProof{Agent: id, Role: choice.role, Model: choice.route.Model, Effort: choice.route.Effort, Source: choice.route.Source, Key: observation.Key, MetaDigest: workflowDigest(meta)}
		d.results.mu.Lock()
		if e := d.results.entries[id]; e != nil && e.Session == run.Session && e.stopped {
			child.Turn, child.Reason, child.Ended = e.NativeTurn, e.EndReason, e.NativeEndObserved
		}
		d.results.mu.Unlock()
		saved.Children = append(saved.Children, child)
	}
	raw, err := json.Marshal(saved)
	if err != nil || len(raw) > 1<<20 {
		return errWorkflowRecoveryUnverified
	}
	path := run.script + ".clauduct-workflow.json"
	nonce, err := newToken()
	if err != nil {
		return err
	}
	temp := path + "." + nonce + ".tmp"
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errWorkflowRecoveryUnverified
	}
	return root.Rename(temp, path)
}

// SessionStart supplies the only directory eligible for lookup. No home-wide
// search, inferred UUID, old process receipt directory or arbitrary path fallback.
func (d *delegations) restoreWorkflow(session, source string) (workflowRun, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var zero workflowRun
	if run, ok := d.workflows[delegationKey{session, source}]; ok {
		return run, nil
	}
	transcript := d.workflowSessions[session]
	if transcript == "" || !correlationShape.MatchString(source) || !strings.HasPrefix(source, "wf_") {
		return zero, errWorkflowRecoveryUnverified
	}
	root, err := os.OpenRoot(d.projects)
	if err != nil {
		return zero, errWorkflowRecoveryUnverified
	}
	defer root.Close()
	dir := filepath.Join(strings.TrimSuffix(transcript, ".jsonl"), "workflows", "scripts")
	f, err := root.Open(dir)
	if err != nil {
		return zero, errWorkflowRecoveryUnverified
	}
	entries, readErr := f.ReadDir(513)
	f.Close()
	if readErr != nil && readErr != io.EOF || len(entries) > 512 {
		return zero, errWorkflowRecoveryUnverified
	}
	var raw []byte
	var scriptPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-"+source+".js.clauduct-workflow.json") {
			if raw != nil {
				return zero, errWorkflowRecoveryUnverified
			}
			scriptPath = filepath.Join(dir, strings.TrimSuffix(e.Name(), ".clauduct-workflow.json"))
			raw, err = workflowRead(root, filepath.Join(dir, e.Name()), 1<<20)
			if err != nil {
				return zero, errWorkflowRecoveryUnverified
			}
		}
	}
	var saved workflowCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&saved) != nil || saved.Version != 1 || saved.Link.Session != session || saved.Link.Run != source || saved.Link.Parent != "" || !correlationShape.MatchString(saved.Link.Call) || !correlationShape.MatchString(saved.Link.Name) || len(saved.Children) > maxAgents || saved.AdapterBytes < 0 || saved.Created.IsZero() || saved.Created.After(time.Now()) {
		return zero, errWorkflowRecoveryUnverified
	}
	if decoder.Decode(new(any)) != io.EOF {
		return zero, errWorkflowRecoveryUnverified
	}
	canonical, _ := json.Marshal(saved)
	if !bytes.Equal(raw, canonical) {
		return zero, errWorkflowRecoveryUnverified
	}
	link := saved.Link
	expectedTranscript := filepath.Join(d.projects, transcript)
	expectedScript := filepath.Join(d.projects, dir, link.Name+"-"+source+".js")
	expectedDirectory := filepath.Join(strings.TrimSuffix(expectedTranscript, ".jsonl"), "subagents", "workflows", source)
	if link.Transcript != expectedTranscript || link.Script != expectedScript || link.Directory != expectedDirectory || filepath.Join(d.projects, scriptPath) != expectedScript {
		return zero, errWorkflowRecoveryUnverified
	}
	route, err := bridge.SelectRoute(saved.Model, saved.Effort)
	if err != nil || route.Model != saved.Model || saved.Effort == "" {
		return zero, errWorkflowRecoveryUnverified
	}
	script, err := workflowRead(root, scriptPath, 512<<10)
	if err != nil || saved.AdapterBytes > len(script) || workflowDigest(script) != saved.Digest {
		return zero, errWorkflowRecoveryUnverified
	}
	directory, _ := filepath.Rel(d.projects, expectedDirectory)
	journal, err := workflowRead(root, filepath.Join(directory, "journal.jsonl"), 2<<20)
	if err != nil || workflowDigest(journal) != saved.JournalDigest {
		return zero, errWorkflowRecoveryUnverified
	}
	plan, err := restoreCheckpointPlan(string(script[:len(script)-saved.AdapterBytes]), saved.Plan)
	if err != nil {
		return zero, err
	}
	run := workflowRun{workflowLink: link, directory: directory, script: scriptPath, continuedBy: saved.ContinuedBy, restored: true, observed: map[string]workflowObservation{}, origin: workflowOrigin{scope: delegationScope{session: session, route: route}, digest: saved.Digest, created: saved.Created, adapterBytes: saved.AdapterBytes, recoveryOf: saved.RecoveryOf, plan: plan}}
	choices := map[string]resolvedChoice{}
	for _, child := range saved.Children {
		if !correlationShape.MatchString(child.Agent) || !correlationShape.MatchString(strings.ReplaceAll(child.Role, ":", "_")) || choices[child.Agent].session != "" || (child.Source != "workflow-parent" && child.Source != "workflow-selection") {
			return zero, errWorkflowRecoveryUnverified
		}
		selected, err := bridge.SelectRoute(child.Model, child.Effort)
		if err != nil || selected.Model != child.Model || child.Effort == "" {
			return zero, errWorkflowRecoveryUnverified
		}
		selected.Source = child.Source
		meta, err := workflowRead(root, filepath.Join(directory, "agent-"+child.Agent+".meta.json"), 16384)
		if err != nil || workflowDigest(meta) != child.MetaDigest {
			return zero, errWorkflowRecoveryUnverified
		}
		var native struct {
			AgentType, Description, Model string
			SpawnDepth                    int
		}
		if json.Unmarshal(meta, &native) != nil || native.AgentType != child.Role || native.SpawnDepth != 1 {
			return zero, errWorkflowRecoveryUnverified
		}
		if child.Ended && child.Reason != "answer" && child.Reason != "aborted" && child.Reason != "error" && child.Reason != "refusal" {
			return zero, errWorkflowRecoveryUnverified
		}
		if child.Ended && !correlationShape.MatchString(child.Turn) {
			return zero, errWorkflowRecoveryUnverified
		}
		run.observed[child.Agent] = workflowObservation{Key: child.Key, Label: native.Description}
		choice := resolvedChoice{session: session, call: link.Call, role: child.Role, route: selected, inherited: true}
		if parts, err := workflowLabelParts(native.Description); err == nil {
			var originalCall, model, effort string
			_ = json.Unmarshal(parts[0], &originalCall)
			_ = json.Unmarshal(parts[1], &model)
			_ = json.Unmarshal(parts[2], &effort)
			if originalCall != link.Call {
				return zero, errWorkflowRecoveryUnverified
			}
			choice.receipt = &SelectionRecord{Session: session, Call: link.Call, Agent: child.Agent, Role: child.Role, RequestedModel: selectionModelLabel(model), RequestedEffort: effort, ModelProvided: string(parts[1]) != "null", EffortProvided: string(parts[2]) != "null", PresenceVerified: true, Model: child.Model, Effort: child.Effort, Source: child.Source}
		}
		choices[child.Agent] = choice
		if _, exists := d.resolved[child.Agent]; exists {
			return zero, errWorkflowRecoveryUnverified
		}
	}
	if len(d.workflows) >= 128 || len(d.resolved)+len(choices) > maxAgents {
		return zero, errWorkflowRecoveryUnverified
	}
	for _, child := range saved.Children {
		if err := d.cacheChoice(child.Agent, choices[child.Agent]); err != nil {
			return zero, err
		}
		d.results.mu.Lock()
		e := d.results.entries[child.Agent]
		e.NativeTurn, e.NativeEndObserved, e.EndReason, e.stopped = child.Turn, child.Ended, child.Reason, true
		d.results.change(e, "restored_evidence") // historical metadata, not a new task or an acquired report
		d.results.mu.Unlock()
	}
	if d.workflows == nil {
		d.workflows = map[delegationKey]workflowRun{}
	}
	d.workflows[delegationKey{session, source}] = run
	d.workflowPersistence.Restored++
	return run, nil
}

// A durable exclusive claim precedes every continuation, even before the first
// checkpoint. Crash/denial after claiming stays uncertain and cannot duplicate work.
func (d *delegations) claimWorkflow(run workflowRun, call string) error {
	root, err := os.OpenRoot(d.projects)
	if err != nil {
		return errWorkflowRecoveryUnverified
	}
	defer root.Close()
	f, err := root.OpenFile(run.script+".clauduct-claim", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errWorkflowRecoveryUnverified
	}
	_, writeErr := f.WriteString(call)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errWorkflowRecoveryUnverified
	}
	return nil
}

func (g *Gateway) workflowPersistenceReport() WorkflowPersistenceReport {
	if g.delegations == nil {
		return WorkflowPersistenceReport{}
	}
	g.delegations.mu.Lock()
	defer g.delegations.mu.Unlock()
	return g.delegations.workflowPersistence
}
