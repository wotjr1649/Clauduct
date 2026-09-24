package gateway

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Native stores the compatibility alias, not the model's original call. Restore
// only the two adapted fields before the backend reads its own history. Scope,
// call ID, role and alias must agree with a verified original receipt. The native
// transcript, prompts and every other tool argument remain untouched.
func (d *delegations) restoreSelectionHistory(request *anthropic.Request, session, parent string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	byCall := map[string]*SelectionRecord{}
	add := func(r *SelectionRecord) {
		if r != nil && r.PresenceVerified && r.Session == session && r.Parent == parent && r.RequestedModel != "model-family" {
			byCall[r.Call] = r
		}
	}
	for _, c := range d.pending {
		add(c.receipt)
	}
	for _, c := range d.resolved {
		add(c.receipt)
	}
	for _, r := range d.selectionRecent {
		add(r)
	}
	for i := range request.Messages {
		m := &request.Messages[i]
		if m.Role != "assistant" {
			continue
		}
		for j := range m.Blocks {
			b := &m.Blocks[j]
			if b.Type == "tool_use" && b.Name == "Workflow" {
				b.Input = d.restoreWorkflowScript(session, b.ID, b.Input)
				continue
			}
			r := byCall[b.ID]
			if b.Type != "tool_use" || b.Name != "Agent" || r == nil {
				continue
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(b.Input, &fields) != nil {
				continue
			}
			var alias, role string
			_ = json.Unmarshal(fields["model"], &alias)
			_ = json.Unmarshal(fields["subagent_type"], &role)
			matches := roleMatches(r.Role, role, r.CustomRole)
			if fields["subagent_type"] == nil && r.Role == "general-purpose" {
				matches = true
			}
			if alias != r.NativeModel || !matches || fields["effort"] != nil {
				continue
			}
			delete(fields, "model")
			if r.ModelProvided {
				fields["model"], _ = json.Marshal(r.RequestedModel)
			}
			if r.EffortProvided {
				fields["effort"], _ = json.Marshal(r.RequestedEffort)
			}
			b.Input, _ = json.Marshal(fields)
		}
	}
}

type delegationFailure string

func (e delegationFailure) Error() string { return errDelegationUnverified.Error() }
func (e delegationFailure) Unwrap() error { return errDelegationUnverified }

// Retain only closed routing fields when a generated delegation is refused.
// The prompt, description, arbitrary role, and arbitrary error are never logged.
func (d *delegations) rejectedSelection(scope delegationScope, call string, raw json.RawMessage, err error) {
	if !correlationShape.MatchString(scope.session) || !correlationShape.MatchString(call) {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	var model, effort, role string
	_ = json.Unmarshal(fields["model"], &model)
	_ = json.Unmarshal(fields["effort"], &effort)
	_ = json.Unmarshal(fields["subagent_type"], &role)
	if !bridge.KnownRole(role) {
		role = "unlisted"
	}
	if effort != "" && !slices.Contains(bridge.Efforts, effort) {
		effort = "unlisted"
	}
	failure := "PREPARE_UNVERIFIED"
	var reason delegationFailure
	if errors.As(err, &reason) {
		failure = string(reason)
	}
	if errors.Is(err, bridge.ErrUnsupportedRoute) {
		failure = routeCategory(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.noteSelection(SelectionRecord{Session: scope.session, Parent: scope.parent, Call: call, Role: role, RequestedModel: selectionModelLabel(model), RequestedEffort: effort, ModelProvided: fields["model"] != nil, EffortProvided: fields["effort"] != nil, PresenceVerified: true, State: "prepare_refused", Failure: failure})
}

type SelectionRecord struct {
	Session            string `json:"session"`
	Parent             string `json:"parent,omitempty"`
	Call               string `json:"call"`
	Agent              string `json:"agent,omitempty"`
	Role               string `json:"role"`
	CustomRole         bool   `json:"customRole,omitempty"`
	RequestedModel     string `json:"requestedModel,omitempty"`
	RequestedEffort    string `json:"requestedEffort,omitempty"`
	ModelProvided      bool   `json:"modelProvided"`
	EffortProvided     bool   `json:"effortProvided"`
	PresenceVerified   bool   `json:"requestPresenceVerified"`
	Model              string `json:"effectiveModel"`
	Effort             string `json:"effectiveEffort"`
	Source             string `json:"source"`
	NativeModel        string `json:"nativeModelAlias"`
	State              string `json:"state"`
	Failure            string `json:"failure,omitempty"` // closed validation reason, never native text
	BackendResponses   int64  `json:"backendResponses"`
	BackendCompletions int64  `json:"backendCompletions"`
}
type SelectionReport struct {
	Recent []SelectionRecord `json:"recent"`
	Totals map[string]int64  `json:"totals"`
}

func selectionModelLabel(value string) string {
	if value == "" || value == "inherit" {
		return value
	}
	for _, m := range bridge.Models {
		if value == m.ID || value == m.Alias || value == m.Key {
			return value
		}
	}
	return "model-family"
}

// All mutations are under delegations.mu. Records evicted from the detail ring
// remain counted; pending/resolved calls retain their own bounded pointer.
func (d *delegations) selectionState(r *SelectionRecord, state string) {
	if r == nil || r.State == state {
		return
	}
	if d.selectionTotals == nil {
		d.selectionTotals = map[string]int64{}
	}
	d.selectionTotals[state]++
	r.State = state
}
func (d *delegations) noteSelection(r SelectionRecord) *SelectionRecord {
	state := r.State
	if state == "" {
		state = "prepared"
	}
	r.State = ""
	p := &r
	d.selectionRecent = append(d.selectionRecent, p)
	if len(d.selectionRecent) > recentRequests {
		d.selectionRecent = d.selectionRecent[1:]
	}
	d.selectionState(p, state)
	return p
}

func (d *delegations) observeBackend(id string, completed bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if receipt := d.resolved[id].receipt; receipt != nil {
		if completed {
			receipt.BackendCompletions++
		} else {
			receipt.BackendResponses++
		}
	}
}
func (d *delegations) selectionReport() SelectionReport {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := SelectionReport{Totals: map[string]int64{}}
	for k, v := range d.selectionTotals {
		out.Totals[k] = v
	}
	for i := len(d.selectionRecent) - 1; i >= 0; i-- {
		out.Recent = append(out.Recent, *d.selectionRecent[i])
	}
	return out
}
