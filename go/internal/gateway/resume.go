package gateway

import (
	"encoding/json"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// A SendMessage can resume a stopped descendant from a different native caller.
// Keep its creation identity immutable; separately bind this execution to the
// actual delivered tool call. No HTTP model or parent header creates this receipt.
type resumeBinding struct {
	session, parent, call, nativeModel string
	verified, started                  bool
}

func (d *delegations) prepareResume(scope delegationScope, call string, raw json.RawMessage) (json.RawMessage, error) {
	var input struct{ To, Recipient, Message string }
	if json.Unmarshal(raw, &input) != nil {
		return nil, errDelegationUnverified
	}
	id := input.To
	if id == "" {
		id = input.Recipient
	}
	if input.To != "" && input.Recipient != "" && input.To != input.Recipient {
		return nil, errDelegationUnverified
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	choice, known := d.resolved[id]
	if !known {
		return raw, nil
	} // Native still owns names, live messages and unknown recipients.
	if choice.session != scope.session || !correlationShape.MatchString(call) {
		return nil, errDelegationUnverified
	}
	if scope.parent != "" && d.resolved[scope.parent].session != scope.session {
		return nil, errDelegationUnverified
	}
	// Native reserves notify_when_idle for peer Claude sessions. A native child
	// already emits the completion event; requesting that same notification must
	// not prevent message delivery. Unknown recipients retain native semantics.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, errDelegationUnverified
	}
	if flag, found := fields["notify_when_idle"]; found {
		var enabled bool
		if json.Unmarshal(flag, &enabled) != nil {
			return nil, errDelegationUnverified
		}
		delete(fields, "notify_when_idle")
		var err error
		raw, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	}
	d.results.mu.Lock()
	e := d.results.entries[id]
	stopped := e != nil && e.stopped
	d.results.mu.Unlock()
	if !stopped {
		return raw, nil
	}
	if d.resumes == nil {
		d.resumes = map[string]*resumeBinding{}
	}
	if prior := d.resumes[id]; prior != nil && !prior.started {
		return nil, errDelegationUnverified
	}
	if len(d.resumes) >= maxAgents && d.resumes[id] == nil {
		// discardResume only removes bindings that never started, so started ones accumulated
		// and the 1025th distinct agent could never be resumed again. A binding whose agent
		// has reported owes nothing and is the one to reclaim; if none has, this still
		// refuses rather than cutting a live resume loose.
		evicted := ""
		d.results.mu.Lock()
		for key, binding := range d.resumes {
			if !binding.started {
				continue
			}
			if e := d.results.entries[key]; e == nil || resultReported(e.State) {
				evicted = key
				break
			}
		}
		d.results.mu.Unlock()
		if evicted == "" {
			return nil, errDelegationUnverified
		}
		delete(d.resumes, evicted)
	}
	native, err := bridge.SelectRoute(scope.nativeModel, "")
	if err != nil {
		return nil, errDelegationUnverified
	}
	if choice.isFork() {
		if input.Message == "" {
			return nil, errDelegationUnverified
		}
		fields["message"], _ = json.Marshal("Your previous task has completed. This is a new task on the same agent, not an interruption of the previous task. Preserve the existing constraints and answer the new coordinator request below. Do not repeat the prior answer unless this request asks for it.\n\n" + input.Message)
		raw, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	}
	d.resumes[id] = &resumeBinding{session: scope.session, parent: scope.parent, call: call, nativeModel: native.Model}
	return raw, nil
}

// Caller holds d.mu. Original metadata must still prove this is the same child.
func (d *delegations) resumed(scope delegationScope, id string, binding agentBinding, choice resolvedChoice) (*resumeBinding, error) {
	r := d.resumes[id]
	if r == nil {
		return nil, nil
	}
	if r.session != scope.session || r.parent != scope.parent {
		return nil, errDelegationUnverified
	}
	if !r.verified {
		meta, err := d.metadata(binding)
		if err != nil || meta.ToolUseID != choice.call || meta.ParentAgentID != choice.parent || !roleMatches(choice.role, meta.AgentType, choice.custom) || !metadataModelMatches(choice.role, choice.alias, meta.Model, choice.route.Source, choice.custom) || meta.StoppedByUser {
			return nil, errDelegationUnverified
		}
		r.verified = true
	}
	return r, nil
}

// Caller holds d.mu. A rejected/undelivered tool call is not resume authority.
func (d *delegations) discardResume(session, call string) {
	for id, r := range d.resumes {
		if r.session == session && r.call == call && !r.started {
			delete(d.resumes, id)
		}
	}
}

func (d *delegations) resumeModel(scope delegationScope, id, model string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.resumes[id]
	return d.resolved[id].isFork() && r != nil && r.verified && r.session == scope.session && r.parent == scope.parent && r.nativeModel == model
}

func (d *delegations) beginResult(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.results.begin(id) {
		return false
	}
	if r := d.resumes[id]; r != nil && r.verified {
		d.results.mu.Lock()
		if e := d.results.entries[id]; e != nil {
			e.parent, e.Call = r.parent, r.call
		}
		d.results.mu.Unlock()
		r.started = true
	}
	return true
}
