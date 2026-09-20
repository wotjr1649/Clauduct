package gateway

// Native can omit the creation parent when a completed background child wakes
// its waiting coordinator. Only a new, independently recorded native turn and
// the original identity permit that continuation; an HTTP header is not proof.
func (g *Gateway) continuationScope(scope delegationScope, id string, binding agentBinding) (delegationScope, bool) {
	if scope.parent != "" || g.delegations == nil {
		return scope, false
	}
	d := g.delegations
	d.mu.Lock()
	choice, known := d.resolved[id]
	explicitResume := d.resumes[id] != nil
	d.mu.Unlock()
	if !known || explicitResume || choice.parent == "" || choice.session != scope.session || binding.ID != id || binding.SessionID != scope.session || binding.Role != choice.role {
		return scope, false
	}
	var active nativeTurnReceipt
	found, err := g.readNativeReceipt("active-"+id+".json", &active)
	if !found || err != nil || !validActiveReceipt(active, scope.session, id) || active.Model != choice.route.Model {
		return scope, false
	}
	meta, err := d.metadata(binding)
	if err != nil || meta.ToolUseID != choice.call || meta.ParentAgentID != choice.parent || meta.AgentType != choice.role || !metadataModelMatches(choice.role, choice.alias, meta.Model) || meta.StoppedByUser {
		return scope, false
	}
	r := &d.results
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil || e.stopped || e.Session != scope.session {
		return scope, false
	}
	// Subsequent requests in this same native turn retain the verified creation
	// parent. A completed turn cannot authorize a later or replayed execution.
	if e.continuationTurn == active.Turn && e.NativeTurn == active.Turn && !e.NativeEndObserved && e.State == "running" {
		scope.parent = choice.parent
		return scope, true
	}
	if e.State != "awaiting_children" || !e.NativeEndObserved || e.EndReason != "answer" || e.NativeTurn == "" || e.NativeTurn == active.Turn {
		return scope, false
	}
	for _, child := range r.entries {
		if child.Session == scope.session && child.parent == id && child.stopped && !resultReported(child.State) {
			e.continuationTurn = active.Turn
			scope.parent = choice.parent
			return scope, true
		}
	}
	return scope, false
}
