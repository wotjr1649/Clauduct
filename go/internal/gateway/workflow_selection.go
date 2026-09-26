package gateway

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

//go:embed workflow-agent.js
var workflowAgentWrapper string

var workflowWrapperFirstLine, _, _ = strings.Cut(strings.ReplaceAll(workflowAgentWrapper, "\r\n", "\n"), "\n")

// A hoisted local function wraps native's existing VM global; native still
// validates and executes the script. No JS parser, extra process or model call.
func (d *delegations) adaptWorkflow(scope delegationScope, id string, raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	var script string
	if json.Unmarshal(raw, &fields) != nil {
		return nil, errDelegationUnverified
	}
	if _, present := fields["resumeFromRunId"]; present {
		return d.recoverWorkflow(scope, id, raw)
	}
	// Native's schema offers name for "a predefined workflow", and models put the plan
	// marker there rather than in script (v0.5.1 G3: every such call was refused).
	var name string
	if fields["script"] == nil && fields["scriptPath"] == nil && json.Unmarshal(fields["name"], &name) == nil && name == bridge.WorkflowPlanMarker {
		fields["script"] = fields["name"]
		delete(fields, "name")
		raw, _ = json.Marshal(fields)
	}
	if fields["scriptPath"] != nil || fields["script"] == nil && fields["name"] != nil {
		return d.prepareWorkflowSource(scope, id, raw)
	}
	if json.Unmarshal(fields["script"], &script) != nil || len(script) > 500<<10 || strings.Contains(script, "__CLAUDUCT_") {
		return nil, errDelegationUnverified
	}
	// After a restart the history still shows the wrapper appended last time (the restore
	// record is per process), and a model that resubmits that script sends it back; native
	// then refuses the second copy (v0.5.1 G3). The wrapper always ends the script, so cut
	// from its first line; this only ever removes code.
	if at := strings.Index(script, "\n"+workflowWrapperFirstLine); at >= 0 {
		script = script[:at]
	}
	var plan *workflowPlan
	if script == bridge.WorkflowPlanMarker {
		var err error
		plan, err = parseWorkflowPlan(raw, scope.route)
		if err != nil {
			return nil, err
		}
		script = plan.script()
		fields = map[string]json.RawMessage{}
	}
	return d.adaptWorkflowBody(scope, id, fields, script, plan, raw)
}

// Called with either validated user JavaScript or a plan compiled from bounded
// data. Compiler output is not reinterpreted as untrusted source input.
func (d *delegations) adaptWorkflowBody(scope delegationScope, id string, fields map[string]json.RawMessage, script string, plan *workflowPlan, raw json.RawMessage) (json.RawMessage, error) {
	trailer := workflowTrailer(scope, id)
	fields["script"], _ = json.Marshal(script + trailer)
	encoded, _ := json.Marshal(fields)
	if err := d.prepareWorkflow(scope, id, encoded); err != nil {
		return nil, err
	}
	d.mu.Lock()
	origin := d.workflowCalls[delegationKey{scope.session, id}]
	origin.adapterBytes = len(trailer)
	if plan != nil {
		origin.plan = plan
		origin.recoveryInput = append(json.RawMessage(nil), raw...)
	}
	d.workflowCalls[delegationKey{scope.session, id}] = origin
	d.mu.Unlock()
	return encoded, nil
}

func workflowTrailer(scope delegationScope, id string) string {
	// [id, default effort, accepted efforts], so the wrapper refuses before native starts a child.
	catalogue := map[string][]any{}
	parentEntry := []any{scope.route.Model, scope.route.Effort, []string{}}
	for _, m := range bridge.Models {
		for _, name := range []string{m.ID, m.Key, m.Alias} {
			catalogue[name] = []any{m.ID, m.Effort, m.Efforts}
		}
		if m.ID == scope.route.Model {
			parentEntry[2] = m.Efforts
		}
	}
	models, _ := json.Marshal(catalogue)
	parent, _ := json.Marshal(parentEntry)
	call, _ := json.Marshal(id)
	// Git's Windows checkout can give the embedded helper CRLF. Native rejects CR
	// in its approval dialog; normalize only our helper, never the user's script.
	template := strings.ReplaceAll(workflowAgentWrapper, "\r\n", "\n")
	return "\n" + strings.NewReplacer("__CLAUDUCT_CATALOGUE__", string(models), "__CLAUDUCT_PARENT__", string(parent), "__CLAUDUCT_CALL__", string(call)).Replace(template)
}

func workflowLabelParts(label string) ([]json.RawMessage, error) {
	const prefix = " [clauduct:"
	start := strings.LastIndex(label, prefix)
	if start < 0 || !strings.HasSuffix(label, "]") {
		return nil, errDelegationUnverified
	}
	var parts []json.RawMessage
	if json.Unmarshal([]byte(label[start+len(prefix):len(label)-1]), &parts) != nil || len(parts) < 3 || len(parts) > 4 {
		return nil, errDelegationUnverified
	}
	if _, err := workflowLabelOptions(parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func workflowLabelSelection(label string, run workflowRun, agent string, active *nativeTurnReceipt, defaults ...func(string, bridge.Route) (bridge.Route, bool, error)) (bridge.Route, *SelectionRecord, error) {
	parts, err := workflowLabelParts(label)
	if err != nil || active == nil || !validActiveReceipt(*active, run.Session, agent) {
		return bridge.Route{}, nil, errDelegationUnverified
	}
	var call, model, effort string
	if json.Unmarshal(parts[0], &call) != nil || call != run.Call {
		return bridge.Route{}, nil, errDelegationUnverified
	}
	hasModel, hasEffort := string(parts[1]) != "null", string(parts[2]) != "null"
	if hasModel && (json.Unmarshal(parts[1], &model) != nil || model == "") || hasEffort && (json.Unmarshal(parts[2], &effort) != nil || effort == "") {
		return bridge.Route{}, nil, errDelegationUnverified
	}
	if selectionModelLabel(model) == "model-family" {
		return bridge.Route{}, nil, errDelegationUnverified
	}
	selectedModel, selectedEffort := model, effort
	options, _ := workflowLabelOptions(parts)
	if !hasModel || model == "inherit" {
		selectedModel = run.origin.scope.route.Model
		if !hasEffort {
			selectedEffort = run.origin.scope.route.Effort
		}
	}
	if options.AgentType != "" && !hasModel {
		if len(defaults) != 1 || defaults[0] == nil {
			return bridge.Route{}, nil, errDelegationUnverified
		}
		roleRoute, found, err := defaults[0](options.AgentType, run.origin.scope.route)
		if err != nil || !found {
			return bridge.Route{}, nil, errDelegationUnverified
		}
		selectedModel = roleRoute.Model
		if !hasEffort {
			selectedEffort = roleRoute.Effort
		}
	}
	route, err := bridge.SelectRoute(selectedModel, selectedEffort)
	if err != nil || active.Model != route.Model || active.Effort != route.Effort {
		return bridge.Route{}, nil, errDelegationUnverified
	}
	route.Source = "workflow-selection"
	r := &SelectionRecord{Session: run.Session, Call: run.Call, Role: "workflow-subagent", RequestedModel: selectionModelLabel(model), RequestedEffort: effort, ModelProvided: hasModel, EffortProvided: hasEffort, PresenceVerified: true, Model: route.Model, Effort: route.Effort, Source: route.Source}
	if options.AgentType != "" {
		r.Role = options.AgentType
		r.CustomRole = true
	}
	return route, r, nil
}

// Restore only our own proven trailer before counting/sending model history.
func (d *delegations) restoreWorkflowScript(session, call string, raw json.RawMessage) json.RawMessage {
	origin, ok := d.workflowCalls[delegationKey{session, call}]
	if !ok {
		for _, run := range d.workflows {
			if run.Session == session && run.Call == call {
				origin, ok = run.origin, true
				break
			}
		}
	}
	if !ok || origin.adapterBytes == 0 && origin.recoveryOf == "" && !origin.rejected {
		return raw
	}
	var fields map[string]json.RawMessage
	var script string
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields["script"], &script) != nil || len(script) < origin.adapterBytes {
		return raw
	}
	digest := sha256.Sum256([]byte(script))
	if hex.EncodeToString(digest[:]) != origin.digest {
		return raw
	}
	if (origin.recoveryOf != "" || origin.rejected || origin.plan != nil || origin.source) && len(origin.recoveryInput) != 0 {
		return append(json.RawMessage(nil), origin.recoveryInput...)
	}
	fields["script"], _ = json.Marshal(script[:len(script)-origin.adapterBytes])
	encoded, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return encoded
}
