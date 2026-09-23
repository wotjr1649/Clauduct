package gateway

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type workflowOrigin struct {
	scope         delegationScope
	digest        string
	created       time.Time
	adapterBytes  int
	recoveryOf    string
	recoveryInput json.RawMessage
	rejected      bool
	plan          *workflowPlan
	source        bool
}
type workflowLink struct {
	Session    string `json:"sessionId"`
	Parent     string `json:"parent,omitempty"`
	Call       string `json:"toolUseId"`
	Run        string `json:"runId"`
	Name       string `json:"workflowName"`
	Transcript string `json:"transcriptPath"`
	Directory  string `json:"transcriptDir"`
	Script     string `json:"scriptPath"`
}
type workflowRun struct {
	workflowLink
	origin            workflowOrigin
	directory, script string
	continuedBy       string
	observed          map[string]workflowObservation
	restored          bool
}

func (d *delegations) prepareWorkflow(scope delegationScope, id string, raw json.RawMessage) error {
	var input struct {
		Script, ScriptPath, Name, ResumeFromRunID string
		Remote                                    bool
	}
	if json.Unmarshal(raw, &input) != nil || len(input.Script) == 0 || len(input.Script) > 512<<10 || input.ScriptPath != "" || input.ResumeFromRunID != "" || input.Remote || scope.parent != "" || !correlationShape.MatchString(scope.session) || !correlationShape.MatchString(id) {
		return errDelegationUnverified
	}
	digest := sha256.Sum256([]byte(input.Script))
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.workflowCalls == nil {
		d.workflowCalls = map[delegationKey]workflowOrigin{}
	}
	key := delegationKey{scope.session, id}
	if _, exists := d.workflowCalls[key]; exists || len(d.workflowCalls) >= 128 {
		return errDelegationUnverified
	}
	d.workflowCalls[key] = workflowOrigin{scope: scope, digest: hex.EncodeToString(digest[:]), created: time.Now()}
	return nil
}

func workflowRead(root *os.Root, path string, limit int64) ([]byte, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errDelegationUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errDelegationUnverified
	}
	return raw, nil
}

func (g *Gateway) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isJSON(r.Header.Get("Content-Type")) || g.delegations == nil {
		g.refuseCategory(w, 400, "AGENT_SELECTION_UNVERIFIED")
		return
	}
	raw, ok := g.readBounded(w, r, 8192)
	var link workflowLink
	if !ok {
		return
	}
	_, err := wire.Fields(raw, []string{"sessionId", "parent", "toolUseId", "runId", "workflowName", "transcriptPath", "transcriptDir", "scriptPath"})
	if err != nil || json.Unmarshal(raw, &link) != nil || g.delegations.linkWorkflow(link) != nil {
		g.refuseCategory(w, 400, "AGENT_SELECTION_UNVERIFIED")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (d *delegations) linkWorkflow(link workflowLink) error {
	if !correlationShape.MatchString(link.Session) || !correlationShape.MatchString(link.Call) || !correlationShape.MatchString(link.Run) || !strings.HasPrefix(link.Run, "wf_") || !correlationShape.MatchString(link.Name) || link.Parent != "" || filepath.Base(link.Transcript) != link.Session+".jsonl" {
		return errDelegationUnverified
	}
	base := filepath.Join(filepath.Dir(link.Transcript), link.Session)
	dir := filepath.Join(base, "subagents", "workflows", link.Run)
	script := filepath.Join(base, "workflows", "scripts", link.Name+"-"+link.Run+".js")
	if filepath.Clean(link.Directory) != dir || filepath.Clean(link.Script) != script {
		return errDelegationUnverified
	}
	relDir, err := filepath.Rel(d.projects, dir)
	if err != nil || !filepath.IsLocal(relDir) {
		return errDelegationUnverified
	}
	relScript, err := filepath.Rel(d.projects, script)
	if err != nil || !filepath.IsLocal(relScript) {
		return errDelegationUnverified
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	origin, ok := d.workflowCalls[delegationKey{link.Session, link.Call}]
	if !ok || origin.scope.parent != link.Parent {
		return errDelegationUnverified
	}
	if d.workflows == nil {
		d.workflows = map[delegationKey]workflowRun{}
	}
	key := delegationKey{link.Session, link.Run}
	if _, exists := d.workflows[key]; exists || len(d.workflows) >= 128 {
		return errDelegationUnverified
	}
	root, err := d.openProjects(".")
	if err != nil {
		return errDelegationUnverified
	}
	defer root.Close()
	text, err := workflowRead(root, relScript, 512<<10)
	if err != nil {
		return errDelegationUnverified
	}
	digest := sha256.Sum256(text)
	if origin.source {
		if !d.verifyWorkflowSource(link, origin, text) {
			return errDelegationUnverified
		}
		origin.digest = hex.EncodeToString(digest[:])
	}
	if hex.EncodeToString(digest[:]) != origin.digest {
		return errDelegationUnverified
	}
	d.workflows[key] = workflowRun{workflowLink: link, origin: origin, directory: relDir, script: relScript}
	delete(d.workflowCalls, delegationKey{link.Session, link.Call})
	return nil
}

func (d *delegations) workflowRoute(ctx context.Context, scope delegationScope, id string, binding agentBinding) (bridge.Route, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for {
		route, found, err := d.findWorkflow(ctx, scope, id, binding)
		if err != nil || found {
			return route, found, err
		}
		select {
		case <-ctx.Done():
			return bridge.Route{}, false, errDelegationUnverified
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func (d *delegations) findWorkflow(ctx context.Context, scope delegationScope, id string, binding agentBinding) (bridge.Route, bool, error) {
	if scope.parent != "" || binding.SessionID != scope.session || binding.ID != id {
		return bridge.Route{}, false, errDelegationUnverified
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	root, err := d.openProjects(".")
	if err != nil {
		return bridge.Route{}, false, errDelegationUnverified
	}
	defer root.Close()
	var matched *workflowRun
	var selected bridge.Route
	var receipt *SelectionRecord
	var observation workflowObservation
	bytesLeft := int64(16 << 20)
	for _, run := range d.workflows {
		if ctx.Err() != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		if run.Session != scope.session || run.Transcript != binding.TranscriptPath {
			continue
		}
		raw, err := workflowRead(root, filepath.Join(run.directory, "journal.jsonl"), bytesLeft)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		bytesLeft -= int64(len(raw))
		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		// The journal legitimately holds result lines up to resultBodyLimit, so a reader
		// bounded below it refuses valid input: one child report over 128 KiB made every
		// later child of that run fail selection, and no retry could recover it. The three
		// other readers of this same file already use this bound.
		scanner.Buffer(make([]byte, 4096), resultBodyLimit+16384)
		found := false
		label := ""
		rows := 0
		for scanner.Scan() {
			if ctx.Err() != nil {
				return bridge.Route{}, false, errDelegationUnverified
			}
			rows++
			if rows > 65536 {
				return bridge.Route{}, false, errDelegationUnverified
			}
			var entry struct{ Type, AgentID, Key, Label string }
			if json.Unmarshal(scanner.Bytes(), &entry) != nil {
				return bridge.Route{}, false, errDelegationUnverified
			}
			if rows == 1 && entry.Type != "launched" {
				return bridge.Route{}, false, errDelegationUnverified
			}
			if entry.AgentID != id {
				continue
			}
			if found || entry.Type != "started" || !strings.HasPrefix(entry.Key, "v2:") || len(entry.Key) != 67 {
				return bridge.Route{}, false, errDelegationUnverified
			}
			found, label = true, entry.Label
			observation = workflowObservation{Key: entry.Key, Label: entry.Label}
		}
		if scanner.Err() != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		if !found {
			continue
		}
		if matched != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		text, err := workflowRead(root, run.script, 512<<10)
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		digest := sha256.Sum256(text)
		if hex.EncodeToString(digest[:]) != run.origin.digest {
			return bridge.Route{}, false, errDelegationUnverified
		}
		metaRaw, err := workflowRead(root, filepath.Join(run.directory, "agent-"+id+".meta.json"), 16384)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		var meta struct {
			AgentType, Description, ToolUseID, ParentAgentID, Model string
			SpawnDepth                                              int
			StoppedByUser                                           bool
		}
		if json.Unmarshal(metaRaw, &meta) != nil || meta.AgentType != binding.Role || meta.Description != label || meta.ToolUseID != "" || meta.ParentAgentID != "" || meta.SpawnDepth != 1 || meta.StoppedByUser {
			return bridge.Route{}, false, errDelegationUnverified
		}
		if run.origin.adapterBytes != 0 {
			var err error
			selected, receipt, err = workflowLabelSelection(label, run, id, scope.nativeTurn, d.roleDefaults)
			// Native stores the invocation's optional model in this file, not the
			// custom role's resolved default. The active turn independently proves
			// the actual model/effort, and the definition resolver proves its source.
			roleDefault := err == nil && receipt.CustomRole && !receipt.ModelProvided && meta.Model == ""
			if err != nil || meta.Model != selected.Model && !roleDefault || receipt.Role != meta.AgentType {
				return bridge.Route{}, false, errDelegationUnverified
			}
		} else if bridge.CanonicalRole(meta.AgentType) != "workflow-subagent" {
			return bridge.Route{}, false, errDelegationUnverified
		} else if meta.Model != "" {
			selected, err := bridge.SelectRoute(meta.Model, "")
			if err != nil || selected.Model != run.origin.scope.route.Model {
				return bridge.Route{}, false, errDelegationUnverified
			}
		}
		transcript, err := workflowRead(root, filepath.Join(run.directory, "agent-"+id+".jsonl"), 1<<20)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		line, _, _ := strings.Cut(string(transcript), "\n")
		var first struct {
			Type, AgentID, SessionID string
			Timestamp                time.Time
		}
		if json.Unmarshal([]byte(line), &first) != nil || first.Type != "user" || first.AgentID != id || first.SessionID != scope.session || first.Timestamp.Before(run.origin.created) || first.Timestamp.After(time.Now()) {
			return bridge.Route{}, false, errDelegationUnverified
		}
		matched = &run
	}
	if matched == nil {
		return bridge.Route{}, false, nil
	}
	route := matched.origin.scope.route
	route.Source = "workflow-parent"
	if receipt != nil {
		route = selected
	}
	choice := resolvedChoice{session: scope.session, parent: scope.parent, call: matched.Call, role: binding.Role, route: route, inherited: true, receipt: receipt}
	if receipt != nil {
		choice.custom = receipt.CustomRole
	}
	if matched.origin.plan != nil {
		key := delegationKey{matched.Session, matched.Run}
		run := d.workflows[key]
		index := len(run.observed)
		if index >= len(run.origin.plan.Steps) {
			return bridge.Route{}, false, errDelegationUnverified
		}
		step := run.origin.plan.Steps[index]
		if !strings.HasPrefix(observation.Label, "clauduct-step:"+step.ID+" [clauduct:") || route.Model != step.Model || route.Effort != step.Effort {
			return bridge.Route{}, false, errDelegationUnverified
		}
	}
	key := delegationKey{matched.Session, matched.Run}
	run := d.workflows[key]
	if len(run.observed) >= 4096 {
		return bridge.Route{}, false, errDelegationUnverified
	}
	if run.observed == nil {
		run.observed = map[string]workflowObservation{}
	}
	run.observed[id] = observation
	d.workflows[key] = run
	if err := d.cacheChoice(id, choice); err != nil {
		return bridge.Route{}, false, err
	}
	return route, true, nil
}
