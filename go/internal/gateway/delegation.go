package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var errDelegationUnverified = errors.New("AGENT_SELECTION_UNVERIFIED")
var errMetadataPending = errors.New("AGENT_METADATA_PENDING")

type delegationScope struct {
	workflow        bool
	session, parent string
	nativeModel     string
	route           bridge.Route
	nativeTurn      *nativeTurnReceipt
	parentWait      *parentStep
}
type delegationKey struct{ session, call string }
type delegatedChoice struct {
	receipt             *SelectionRecord
	parent, role, alias string
	route               bridge.Route
	inherited           bool
	custom              bool
}
type resolvedChoice struct {
	receipt               *SelectionRecord
	session, parent, call string
	role, alias           string
	route                 bridge.Route
	inherited             bool
	custom                bool
}

func (c resolvedChoice) isWorkflow() bool {
	return !c.custom && bridge.CanonicalRole(c.role) == "workflow-subagent" || strings.HasPrefix(c.route.Source, "workflow-")
}

func (c resolvedChoice) isFork() bool {
	return !c.custom && !c.isWorkflow() && bridge.IsFork(c.role)
}

func roleMatches(expected, observed string, custom bool) bool {
	if custom {
		return expected == observed
	}
	return bridge.CanonicalRole(expected) == bridge.CanonicalRole(observed)
}

// Correlate a resolved Agent call with native child metadata. Full IDs and native
// aliases follow the same precedence; role prompts and tools stay with the client.
type delegations struct {
	// projectsErr is why the projects tree could not be created, for the diagnostic that
	// would otherwise report only that every delegation was refused.
	projectsErr error

	// Roles without a local route use native's observed selection. Count at
	// preparation, before first-request reconciliation resolves the actual model.
	unroutedRoles atomic.Int64

	selectionRecent     []*SelectionRecord
	selectionTotals     map[string]int64
	mu                  sync.Mutex
	projects            string
	events              string
	pending             map[delegationKey]delegatedChoice
	resolved            map[string]resolvedChoice
	resumes             map[string]*resumeBinding
	roleDefaults        func(string, bridge.Route) (bridge.Route, bool, error)
	results             agentResults
	workflowCalls       map[delegationKey]workflowOrigin
	workflows           map[delegationKey]workflowRun
	workflowSessions    map[string]string
	workflowPersistence WorkflowPersistenceReport
	workflowSourceDirs  []string
}

// Retire failed native tool calls, and retract only calls prepared by a failed
// response. Choices already linked to children stay intact.
func (d *delegations) discard(session string, calls []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, call := range calls {
		d.discardResume(session, call)
		if choice, ok := d.pending[delegationKey{session, call}]; ok {
			d.selectionState(choice.receipt, "delivery_failed")
		}
		delete(d.pending, delegationKey{session, call})
		delete(d.workflowCalls, delegationKey{session, call})
	}
}
func (d *delegations) toolFailures(session string, request *anthropic.Request) {
	var calls []string
	for _, message := range request.Messages {
		if message.Role == "user" {
			for _, b := range message.Blocks {
				if b.Type == "tool_result" && b.IsError {
					calls = append(calls, b.ToolUseID)
				}
			}
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, call := range calls {
		key := delegationKey{session, call}
		d.discardResume(session, call)
		if choice, ok := d.pending[key]; ok {
			d.selectionState(choice.receipt, "native_tool_failed")
			delete(d.pending, key)
		}
		if !d.workflowCalls[key].rejected {
			delete(d.workflowCalls, key)
		}
	}
}

// The launcher reads definition defaults; native still owns role prompts, tools,
// permission checks, loading and execution. Never substitute a request value for
// a missing definition.
func (g *Gateway) ConfigureRoleDefaults(resolve func(string, bridge.Route) (bridge.Route, bool, error)) {
	if g.delegations != nil {
		g.delegations.roleDefaults = resolve
	}
}

// ConfigureDelegations prepares native's lazily created tree before launch.
// Failure remains diagnostic; openProjects classifies each later access afresh.
func (g *Gateway) ConfigureDelegations(projects string) {
	mkdirErr := os.MkdirAll(projects, 0o700)
	g.delegations = &delegations{projects: projects, events: g.nativeEvents.directory, projectsErr: mkdirErr,
		pending: map[delegationKey]delegatedChoice{}, resolved: map[string]resolvedChoice{}}
}

// Advertise the extension on the backend's existing Agent tool only. The native
// schema, role descriptions, tool restrictions and all other tools stay intact.
func (d *delegations) describe(tools []bridge.ToolSpec, agentIDs ...string) error {
	var pinned bridge.Route
	if len(agentIDs) > 0 {
		d.mu.Lock()
		if choice, ok := d.resolved[agentIDs[0]]; ok && choice.inherited {
			pinned = choice.route
		}
		d.mu.Unlock()
	}
	for i := range tools {
		if tools[i].Name == "ToolSearch" {
			tools[i].Description += " Before declaring an Agent role unavailable, discover Agent and read the resulting role-list reminder; deferred Agent roles may not be listed until discovery completes."
		}
		if tools[i].Name == "Workflow" {
			tools[i].Description += " A successful TaskStop result is the stop acknowledgement; do not wait for an additional workflow completion notification before requesting resume. Clauduct independently verifies stopped state and child termination before it permits remaining work."
			tools[i].Description += " Clauduct: ordinary script resumeFromRunId returns verified completed child reports only, never replays arbitrary JavaScript. For resumable independent steps, use script=\"clauduct:plan-v1\" and args={steps:[{id,prompt,model?,effort?,tools?}]} (1-16 unique IDs). For a step that must use no tools, set tools:[]; this is enforced by the gateway. Omitted tools preserves native tools; a list of exact tool names (up to 64) narrows the native catalogue and callable set, never adds permissions. To continue an interrupted plan, first stop its native task with TaskStop, then pass resumeFromRunId alone. Started steps are never rerun: completed reports are reused, unavailable results reported, and only never-started steps execute. Each source run can be continued once. Model/effort and tool restrictions remain fixed from the original plan."
			tools[i].Description += " Use the native workflow-authoring contract for agent() options. Clauduct enforces tools:[] and exact tool-name allowlists for agent() as well as plan steps. Custom agentType preserves the native role's instructions, tools and maxTurns; omitted model/effort uses its verified definition defaults. scriptPath and local named .js files are read through native Read with its permissions; partial reads are refused. Same-session recovery after a reaped launcher exit revalidates saved metadata; missing or changed evidence is not replayed. A maxTurns option directly on agent() is unsupported; use the native agent definition."
		}
		if tools[i].Name == "SendMessage" {
			tools[i].Description += " Native subagents emit completion events automatically. Omit notify_when_idle for subagents; that native flag is only for peer Claude sessions. A message to a completed child starts a new task on that same agent."
		}
		if tools[i].Name != "Agent" {
			continue
		}
		tools[i].Description += " Complete delegated research with a self-contained report including findings, evidence, and unverified work. Parent must acquire and review the report, not merely a completion notice. Prefer completion events, do not periodically poll; retrieve an existing result once if absent, request only missing report sections, and report unavailable results without automatically rerunning the task."
		tools[i].Description += " Preserve task-specific constraints in every descendant's prompt, including permitted files, tool restrictions, and completion requirements."
		tools[i].Description += " When yielding for an existing child, end the turn with a brief visible waiting acknowledgment; an empty or reasoning-only response is not a deliverable answer."
		tools[i].Description += " The native UI uses compatibility aliases; Clauduct status records the original selection and effective backend route separately. Verified original model/effort arguments are restored in your tool history. Omit isolation unless worktree or remote isolation was explicitly requested."
		var schema map[string]json.RawMessage
		var properties map[string]json.RawMessage
		var model map[string]json.RawMessage
		if json.Unmarshal(tools[i].Parameters, &schema) != nil ||
			json.Unmarshal(schema["properties"], &properties) != nil ||
			json.Unmarshal(properties["model"], &model) != nil || model == nil {
			return errDelegationUnverified
		}
		var names []string
		if json.Unmarshal(model["enum"], &names) != nil {
			return errDelegationUnverified
		}
		for _, entry := range bridge.Models {
			names = append(names, entry.ID)
		}
		if pinned.Model != "" {
			names = []string{pinned.Model, "inherit"}
			tools[i].Description += " This delegated task is fixed to " + pinned.Model + "/" + pinned.Effort + ". Omit model and effort for every descendant; conflicting overrides are refused."
		}
		model["enum"], _ = json.Marshal(names)
		properties["model"], _ = json.Marshal(model)
		properties["effort"], _ = json.Marshal(struct {
			Type        string   `json:"type"`
			Enum        []string `json:"enum"`
			Description string   `json:"description"`
		}{"string", bridge.Efforts, "Model without effort uses the selected model's default effort. Effort without model uses the role's default model. Omit unrequested model and effort. A task-bound parent choice is retained by descendants; conflicting overrides are refused."})
		if pinned.Model != "" {
			properties["effort"], _ = json.Marshal(map[string]any{"type": "string", "enum": []string{pinned.Effort}, "description": "Omit effort to retain this task's verified selection."})
		}
		schema["properties"], _ = json.Marshal(properties)
		tools[i].Parameters, _ = json.Marshal(schema)
	}
	return nil
}

func (d *delegations) prepare(scope delegationScope, id, name string, raw json.RawMessage) (json.RawMessage, error) {
	if name == "SendMessage" {
		return d.prepareResume(scope, id, raw)
	}
	if name == "Workflow" {
		return d.adaptWorkflow(scope, id, raw)
	}
	if name == "SubagentHandback" {
		var report struct{ Message string }
		if json.Unmarshal(raw, &report) == nil {
			d.results.handback(scope.session, scope.parent, report.Message)
		}
	}
	if name != "Agent" {
		return raw, nil
	}
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, errDelegationUnverified
	}
	var modelID string
	_, hasEffort := fields["effort"]
	if value, present := fields["model"]; present {
		if json.Unmarshal(value, &modelID) != nil || string(value) == "null" {
			return nil, errDelegationUnverified
		}
	}
	// Native 2.1.280 makes subagent_type optional and resolves omission to
	// general-purpose. A supplied null/empty/invalid role is not omission.
	role := "general-purpose"
	if value, present := fields["subagent_type"]; present {
		if json.Unmarshal(value, &role) != nil || role == "" || string(value) == "null" || len(role) > 200 {
			return nil, delegationFailure("INVALID_ROLE")
		}
	}
	if bridge.RetiredRole(role) {
		return nil, bridge.ErrRetiredRoute
	}
	effort := ""
	if value, exists := fields["effort"]; exists {
		if json.Unmarshal(value, &effort) != nil || effort == "" {
			return nil, errDelegationUnverified
		}
	}
	explicitModel := modelID != "" && modelID != "inherit"
	requestedModel := selectionModelLabel(modelID)
	_, modelProvided := fields["model"]
	inherited := explicitModel || hasEffort
	source := "agent-call-model"
	var route bridge.Route
	d.mu.Lock()
	parent, parentKnown := d.resolved[scope.parent]
	d.mu.Unlock()
	if scope.parent != "" && !parentKnown {
		return nil, delegationFailure("PARENT_SELECTION_MISSING")
	}
	if parentKnown && parent.session != scope.session {
		return nil, delegationFailure("PARENT_SESSION_MISMATCH")
	}
	var definition bridge.Route
	custom := false
	if d.roleDefaults != nil {
		definition, custom, err = d.roleDefaults(role, scope.route)
		if err != nil && !(custom && errors.Is(err, bridge.ErrUnsupportedRoute) && (explicitModel || parentKnown && parent.inherited)) {
			if errors.Is(err, bridge.ErrUnsupportedRoute) {
				return nil, err
			}
			return nil, errDelegationUnverified
		}
	}
	if !custom {
		role = bridge.CanonicalRole(role)
		if _, present := fields["subagent_type"]; present {
			fields["subagent_type"], _ = json.Marshal(role)
		}
	}
	if strings.HasPrefix(role, bridge.MenuPrefix) && !bridge.InheritsParent(role) {
		inherited = true
	}
	if parentKnown && parent.inherited {
		// A task-bound explicit selection remains fixed in descendants. Refuse a
		// conflicting child choice instead of silently substituting either model.
		if explicitModel {
			requested, err := bridge.SelectRoute(modelID, effort)
			if errors.Is(err, bridge.ErrRetiredRoute) {
				return nil, err
			}
			if err != nil || requested.Model != parent.route.Model || hasEffort && requested.Effort != parent.route.Effort {
				return nil, delegationFailure("PARENT_OVERRIDE_CONFLICT")
			}
		} else if hasEffort && effort != parent.route.Effort {
			return nil, delegationFailure("PARENT_OVERRIDE_CONFLICT")
		}
		route = parent.route
		inherited = true
		source = "delegation-inherited"
	} else if explicitModel {
		route, err = bridge.SelectRoute(modelID, effort)
		if err != nil {
			return nil, err
		}
	} else {
		var known bool
		if modelID == "inherit" || !custom && bridge.InheritsParent(role) {
			route = scope.route
			known = route.Model != ""
			source = "parent-route"
		} else {
			source = "agent-call-role"
			if custom {
				route, known, source = definition, true, "agent-call-definition"
			}
			if !known {
				route, known = bridge.RoleRoute(role)
			}
		}
		if !known && !hasEffort && scope.route.Model != "" {
			// Pending choices block parent completion before the child has an ID.
			// Keep native's model choice, then bind its actual turn before dispatch.
			d.unroutedRoles.Add(1)
			known, inherited, source = true, false, "native-selection"
		}
		if !known {
			if hasEffort {
				return nil, bridge.ErrUnsupportedRoute
			}
			return raw, nil // Custom role defaults still require definition evidence.
		}
		if hasEffort {
			route, err = bridge.SelectRoute(route.Model, effort)
			if err != nil {
				return nil, err
			}
		}
	}
	modelID = route.Model
	// Native fork always resolves model:inherit, even if Agent receives a model
	// argument. Keep the parent model; never claim an ignored override was applied.
	if !custom && bridge.IsFork(role) && route.Model != scope.route.Model {
		return nil, bridge.ErrUnsupportedRoute
	}
	var model *bridge.Model
	for i := range bridge.Models {
		if bridge.Models[i].ID == modelID {
			model = &bridge.Models[i]
			break
		}
	}
	if model == nil && source != "native-selection" {
		if hasEffort {
			return nil, bridge.ErrUnsupportedRoute
		}
		return raw, nil
	}
	if !correlationShape.MatchString(scope.session) || !correlationShape.MatchString(id) {
		return nil, errDelegationUnverified
	}
	delete(fields, "effort")
	route.Source = source
	if hasEffort {
		route.Source += "+effort"
	}
	nativeAlias := ""
	if model != nil {
		nativeAlias = model.Alias
		alias, _ := json.Marshal(nativeAlias)
		fields["model"] = alias
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, errDelegationUnverified
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// Never expire a selection into a role fallback: a queued native child may
	// start much later. At the bound, refuse a new adapted call explicitly.
	key := delegationKey{scope.session, id}
	if _, duplicate := d.pending[key]; duplicate || len(d.pending) >= maxAgents {
		return nil, errDelegationUnverified
	}
	for _, chosen := range d.resolved {
		if chosen.session == scope.session && chosen.call == id {
			return nil, errDelegationUnverified
		}
	}
	receipt := d.noteSelection(SelectionRecord{Session: scope.session, Parent: scope.parent, Call: id, Role: role, CustomRole: custom, RequestedModel: requestedModel, RequestedEffort: effort, ModelProvided: modelProvided, EffortProvided: hasEffort, PresenceVerified: true, Model: route.Model, Effort: route.Effort, Source: route.Source, NativeModel: nativeAlias})
	d.pending[key] = delegatedChoice{parent: scope.parent, role: role, alias: nativeAlias, route: route, inherited: inherited, receipt: receipt, custom: custom}
	return encoded, nil
}

func (d *delegations) route(scope delegationScope, id string, binding agentBinding, contexts ...context.Context) (bridge.Route, bool, error) {
	d.mu.Lock()
	if chosen, found := d.resolved[id]; found {
		defer d.mu.Unlock()
		resume, err := d.resumed(scope, id, binding, chosen)
		if err != nil || chosen.session != scope.session || chosen.parent != scope.parent && resume == nil || binding.ID != id || !roleMatches(chosen.role, binding.Role, chosen.custom) || binding.SessionID != scope.session {
			return bridge.Route{}, false, errDelegationUnverified
		}
		// A SendMessage by name leaves no binding -- native owns names -- so a stopped child
		// running again got here without the check an id resume gets, and an id resume's
		// binding outlives the run it was made for. Whoever resumed it, and however often, a
		// child the user stopped stays stopped (#91).
		if d.endedRun(id) {
			if meta, err := d.metadata(binding); err != nil || meta.StoppedByUser {
				return bridge.Route{}, false, errDelegationUnverified
			}
		}
		route := chosen.route
		if resume != nil {
			route.Source = "verified-resume"
		}
		return route, true, nil
	}
	hasPending := false
	for key, choice := range d.pending {
		if key.session == scope.session && choice.parent == scope.parent {
			hasPending = true
			break
		}
	}
	d.mu.Unlock()
	if scope.workflow || !hasPending && bridge.CanonicalRole(binding.Role) == "workflow-subagent" {
		ctx := context.Background()
		if len(contexts) > 0 {
			ctx = contexts[0]
		}
		return d.workflowRoute(ctx, scope, id, binding)
	}
	if !hasPending {
		chosen, found, err := d.loadChoice(scope, id, binding)
		if err != nil || !found {
			// A forked skill's child was never prepared and keeps no journal.
			if fork, ok := d.nativeFork(scope, id, binding, contexts...); ok {
				return d.cacheFork(id, fork)
			}
			return bridge.Route{}, false, err
		}
		d.mu.Lock()
		cacheErr := d.cacheChoice(id, chosen)
		d.mu.Unlock()
		if cacheErr != nil {
			return bridge.Route{}, false, cacheErr
		}
		return chosen.route, true, nil
	}
	if binding.ID != id || binding.SessionID != scope.session {
		return bridge.Route{}, false, errDelegationUnverified
	}
	meta, err := d.metadata(binding)
	// Native 2.1.275 launches its metadata write asynchronously. Its first HTTP
	// request can arrive before the sidecar. Wait only for absence, only at first
	// resolution, and never fall back to the model from that HTTP request.
	if errors.Is(err, errMetadataPending) {
		ctx := context.Background()
		if len(contexts) != 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(contexts[0], time.Second)
			defer cancel()
		}
		for delay := 5 * time.Millisecond; errors.Is(err, errMetadataPending); delay = min(delay*2, 100*time.Millisecond) {
			// A pending sibling proves nothing about this child. Workflow metadata
			// lives in the registered run directory; ordinary Agent metadata may
			// still be arriving. Check the real Workflow proof while waiting for
			// this child's sidecar, never infer membership from the role name.
			if scope.parent == "" && bridge.CanonicalRole(binding.Role) == "workflow-subagent" {
				route, found, workflowErr := d.findWorkflow(ctx, scope, id, binding)
				if workflowErr == nil && found {
					return route, found, workflowErr
				}
				// An unrelated run's damaged evidence cannot reject an ordinary
				// Agent whose own sidecar is still arriving. Without a verified
				// alternative the bounded wait below still refuses this child.
			}
			if len(contexts) == 0 {
				break
			}
			select {
			case <-ctx.Done():
				return bridge.Route{}, false, errDelegationUnverified
			case <-time.After(delay):
				meta, err = d.metadata(binding)
			}
		}
	}
	if err != nil {
		// Metadata without a tool call: a forked skill's child, not one of the pending calls.
		if fork, ok := d.nativeFork(scope, id, binding, contexts...); ok {
			return d.cacheFork(id, fork)
		}
		return bridge.Route{}, false, errDelegationUnverified
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// A parallel request for this same child may have established the link while
	// this request read metadata. The first journal write below is serialized.
	if chosen, found := d.resolved[id]; found {
		if chosen.session != scope.session || chosen.parent != scope.parent {
			return bridge.Route{}, false, errDelegationUnverified
		}
		return chosen.route, true, nil
	}
	key := delegationKey{scope.session, meta.ToolUseID}
	choice, found := d.pending[key]
	if !found {
		chosen, found, err := d.loadChoice(scope, id, binding)
		if err != nil || !found {
			return bridge.Route{}, false, err
		}
		if err := d.cacheChoice(id, chosen); err != nil {
			return bridge.Route{}, false, err
		}
		return chosen.route, true, nil
	} // An unrelated native delegation.
	failure := ""
	choice.custom = choice.custom || strings.HasPrefix(choice.route.Source, "agent-call-definition")
	role := choice.role
	if !choice.custom {
		role = bridge.CanonicalRole(role)
	}
	if choice.route.Source == "native-selection" {
		active := scope.nativeTurn
		if active == nil || !validActiveReceipt(*active, scope.session, id) {
			return bridge.Route{}, false, errDelegationUnverified
		}
		actual, err := bridge.SelectRoute(active.Model, active.Effort)
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		actual.Source = "native-selection"
		choice.route = actual
		for _, model := range bridge.Models {
			if model.ID == actual.Model {
				choice.alias = model.Alias
				break
			}
		}
	}
	switch {
	case choice.parent != scope.parent || meta.ParentAgentID != scope.parent:
		failure = "PARENT_MISMATCH"
	case !roleMatches(role, binding.Role, choice.custom) || !roleMatches(role, meta.AgentType, choice.custom):
		failure = "ROLE_MISMATCH"
	case !metadataModelMatches(choice.role, choice.alias, meta.Model, choice.route.Source, choice.custom):
		failure = "METADATA_MODEL_MISMATCH"
	case meta.StoppedByUser:
		failure = "NATIVE_STOPPED"
	}
	if failure != "" {
		if choice.receipt != nil {
			choice.receipt.Agent, choice.receipt.Failure = id, failure
			d.selectionState(choice.receipt, "selection_failed")
		}
		return bridge.Route{}, false, errDelegationUnverified
	}
	chosen := resolvedChoice{session: scope.session, parent: scope.parent, call: meta.ToolUseID, role: role, alias: choice.alias, route: choice.route, inherited: choice.inherited, custom: choice.custom}
	chosen.receipt = choice.receipt
	if chosen.receipt != nil {
		chosen.receipt.Role = role
		chosen.receipt.Model, chosen.receipt.Effort, chosen.receipt.NativeModel = choice.route.Model, choice.route.Effort, choice.alias
	}
	// ponytail: first resolutions serialize one small journal write. Cache hits
	// perform no I/O; use per-agent locks if measured spawn throughput requires it.
	if err := d.saveChoice(binding, chosen); err != nil {
		return bridge.Route{}, false, err
	}
	if err := d.cacheChoice(id, chosen); err != nil {
		return bridge.Route{}, false, err
	}
	delete(d.pending, key)
	return choice.route, true, nil
}

// startResult opens the child's delegated-result entry. A forked skill's child gets none:
// native returns its report as the Skill tool's result (see nativeFork).
func (d *delegations) startResult(id string, choice resolvedChoice) bool {
	return choice.route.Source == "native-fork" || d.results.start(id, choice)
}

func (d *delegations) cacheChoice(id string, choice resolvedChoice) error {
	choice.custom = choice.custom || strings.HasPrefix(choice.route.Source, "agent-call-definition") || choice.receipt != nil && choice.receipt.CustomRole
	// Before start(), not after. start() installs r.entries[id] as running, and a refusal
	// below would then leave an entry with no resolved choice behind: stoppedTurn returns
	// early without one, so it never stops, never reports, and both eviction loops skip it.
	// Every refusal would burn one of those slots for the life of the process.
	//
	// Only when this id is new, the guard prepareResume already has: re-caching an agent
	// that is already resolved adds nothing to the map, so evicting for it drops an
	// unrelated agent for nothing.
	if _, held := d.resolved[id]; !held && len(d.resolved) >= maxAgents {
		// Map iteration picks an arbitrary victim, and it could be a running agent: without
		// its resolved choice, a plan child recomputes its step index one past the real one
		// and refuses from then on. Only an agent that owes nothing is dropped, and when
		// none does this refuses rather than discarding live work.
		evicted := ""
		d.results.mu.Lock()
		for key := range d.resolved {
			if e := d.results.entries[key]; e == nil || resultReported(e.State) {
				evicted = key
				break
			}
		}
		d.results.mu.Unlock()
		if evicted == "" {
			return errDelegationUnverified
		}
		// Chosen here, dropped after start() succeeds. Deleting first meant a refused
		// cacheChoice had already destroyed a victim that keeps its slot -- and start() can
		// still refuse, because r.entries fills with entries no eviction loop can reclaim.
		if !d.startResult(id, choice) {
			return errDelegationUnverified
		}
		delete(d.resolved, evicted)
	} else if !d.startResult(id, choice) {
		return errDelegationUnverified
	}
	if choice.receipt == nil {
		choice.receipt = d.noteSelection(SelectionRecord{Session: choice.session, Parent: choice.parent, Call: choice.call, Agent: id, Role: choice.role, CustomRole: choice.custom, Model: choice.route.Model, Effort: choice.route.Effort, Source: choice.route.Source, NativeModel: choice.alias, State: "restored"})
	} else if choice.receipt.State == "" {
		choice.receipt.State = "restored"
		choice.receipt = d.noteSelection(*choice.receipt)
	}
	d.resolved[id] = choice
	if choice.receipt != nil {
		choice.receipt.Agent = id
		d.selectionState(choice.receipt, "selection_verified")
	}
	return nil
}

type choiceJournal struct {
	Version    int              `json:"version"`
	Session    string           `json:"session"`
	Parent     string           `json:"parent"`
	Agent      string           `json:"agent"`
	Call       string           `json:"call"`
	Role       string           `json:"role"`
	CustomRole bool             `json:"customRole,omitempty"`
	Alias      string           `json:"alias"`
	Model      string           `json:"model"`
	Effort     string           `json:"effort"`
	Source     string           `json:"source"`
	Inherited  bool             `json:"inherited"`
	Intent     *selectionIntent `json:"intent,omitempty"`
}
type selectionIntent struct {
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	ModelProvided  bool   `json:"modelProvided"`
	EffortProvided bool   `json:"effortProvided"`
}

// Called only when the root opened but this child's journal is absent. Routed
// roles require evidence; other roles retain admission's missing-choice policy.
func (d *delegations) choiceAbsent(binding agentBinding) (resolvedChoice, bool, error) {
	var empty resolvedChoice
	if bridge.KnownRole(binding.Role) {
		return empty, false, errDelegationUnverified
	}
	return empty, false, nil
}

func (d *delegations) choicePath(binding agentBinding) (*os.Root, string, error) {
	if !correlationShape.MatchString(binding.ID) || !correlationShape.MatchString(binding.SessionID) || filepath.Base(binding.TranscriptPath) != binding.SessionID+".jsonl" {
		return nil, "", errDelegationUnverified
	}
	rel, err := filepath.Rel(d.projects, filepath.Dir(binding.TranscriptPath))
	if err != nil {
		return nil, "", errDelegationUnverified
	}
	root, err := d.openProjects(rel)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", errDelegationUnverified, err)
	}
	return root, filepath.Join(rel, binding.SessionID, "subagents", "agent-"+binding.ID+".clauduct-selection.json"), nil
}

func (d *delegations) saveChoice(binding agentBinding, c resolvedChoice) error {
	root, path, err := d.choicePath(binding)
	if err != nil {
		return err
	}
	defer root.Close()
	record := choiceJournal{Version: 2, Session: c.session, Parent: c.parent, Agent: binding.ID, Call: c.call, Role: c.role, CustomRole: c.custom, Alias: c.alias, Model: c.route.Model, Effort: c.route.Effort, Source: c.route.Source, Inherited: c.inherited}
	if c.receipt != nil && c.receipt.PresenceVerified {
		record.Intent = &selectionIntent{c.receipt.RequestedModel, c.receipt.RequestedEffort, c.receipt.ModelProvided, c.receipt.EffortProvided}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return errDelegationUnverified
	}
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errDelegationUnverified
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errDelegationUnverified
	}
	return nil
}

func (d *delegations) loadChoice(scope delegationScope, id string, binding agentBinding) (resolvedChoice, bool, error) {
	var empty resolvedChoice
	if binding.ID == "" || binding.SessionID == "" || binding.TranscriptPath == "" {
		return empty, false, nil
	}
	root, path, err := d.choicePath(binding)
	if err != nil {
		return empty, false, err
	}
	defer root.Close()
	file, err := root.Open(path)
	if os.IsNotExist(err) {
		return d.choiceAbsent(binding)
	}
	if err != nil {
		return empty, false, errDelegationUnverified
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return empty, false, errDelegationUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return empty, false, errDelegationUnverified
	}
	fields, err := wire.Fields(raw, []string{"version", "session", "parent", "agent", "call", "role", "customRole", "alias", "model", "effort", "source", "inherited", "intent"})
	if err != nil {
		return empty, false, errDelegationUnverified
	}
	var saved choiceJournal
	decodeErr := json.Unmarshal(raw, &saved)
	saved.CustomRole = saved.CustomRole || strings.HasPrefix(saved.Source, "agent-call-definition")
	if decodeErr != nil || (saved.Version != 1 && saved.Version != 2) || saved.Session != scope.session || saved.Parent != scope.parent || saved.Agent != id || !roleMatches(saved.Role, binding.Role, saved.CustomRole) {
		return empty, false, errDelegationUnverified
	}
	meta, err := d.metadata(binding)
	if err != nil || meta.ToolUseID != saved.Call || meta.ParentAgentID != saved.Parent || !roleMatches(saved.Role, meta.AgentType, saved.CustomRole) || !metadataModelMatches(saved.Role, saved.Alias, meta.Model, saved.Source, saved.CustomRole) || meta.StoppedByUser {
		return empty, false, errDelegationUnverified
	}
	// A child started on a route v0.3.4 retired is refused by name, not resumed elsewhere.
	if bridge.RetiredRole(saved.Role) {
		return empty, false, bridge.ErrRetiredRoute
	}
	route, err := bridge.SelectRoute(saved.Model, saved.Effort)
	if errors.Is(err, bridge.ErrRetiredRoute) {
		return empty, false, err
	}
	if err != nil {
		return empty, false, errDelegationUnverified
	}
	model, ok := bridge.ForAlias(saved.Alias)
	if !ok || model.ID != route.Model {
		return empty, false, errDelegationUnverified
	}
	switch saved.Source {
	case "agent-call-model", "agent-call-model+effort", "agent-call-role", "agent-call-role+effort", "agent-call-definition", "agent-call-definition+effort", "parent-route", "parent-route+effort", "delegation-inherited", "delegation-inherited+effort", "native-selection":
	default:
		return empty, false, errDelegationUnverified
	}
	route.Source = saved.Source
	choice := resolvedChoice{session: saved.Session, parent: saved.Parent, call: saved.Call, role: saved.Role, alias: saved.Alias, route: route, inherited: saved.Inherited, custom: saved.CustomRole}
	if saved.Intent != nil {
		intent := saved.Intent
		if saved.Version != 2 {
			return empty, false, errDelegationUnverified
		}
		if _, err := wire.Fields(fields["intent"], []string{"model", "effort", "modelProvided", "effortProvided"}); err != nil {
			return empty, false, errDelegationUnverified
		}
		modelOK := intent.Model == "model-family" || selectionModelLabel(intent.Model) == intent.Model
		effortOK := intent.Effort == ""
		for _, effort := range bridge.Efforts {
			effortOK = effortOK || intent.Effort == effort
		}
		if !modelOK || !effortOK || !intent.ModelProvided && intent.Model != "" || intent.EffortProvided != (intent.Effort != "") {
			return empty, false, errDelegationUnverified
		}
		choice.receipt = &SelectionRecord{Session: saved.Session, Parent: saved.Parent, Call: saved.Call, Role: saved.Role, CustomRole: saved.CustomRole, RequestedModel: intent.Model, RequestedEffort: intent.Effort, ModelProvided: intent.ModelProvided, EffortProvided: intent.EffortProvided, PresenceVerified: true, Model: route.Model, Effort: route.Effort, Source: route.Source, NativeModel: saved.Alias}
	}
	return choice, true, nil
}

type delegationMetadata struct {
	ToolUseID     string `json:"toolUseId"`
	ParentAgentID string `json:"parentAgentId"`
	AgentType     string `json:"agentType"`
	Model         string `json:"model"`
	SpawnDepth    int    `json:"spawnDepth"`
	StoppedByUser bool   `json:"stoppedByUser"`
}

// 2.1.276 records inherit for native fork, rather than Agent's compatibility
// alias. Call, parent, role and session are checked by both callers; dispatch
// separately verifies the native request's resolved model against the choice.
func metadataModelMatches(role, alias, model, source string, custom bool) bool {
	if source == "native-selection" {
		return model == ""
	}
	if !custom && bridge.IsFork(role) {
		return model == "inherit"
	}
	return model == alias
}

func (d *delegations) metadata(binding agentBinding) (delegationMetadata, error) {
	meta, err := d.readMetadata(binding)
	if err == nil && !correlationShape.MatchString(meta.ToolUseID) {
		err = errDelegationUnverified
	}
	return meta, err
}

// readMetadata is metadata without the tool call an Agent-created child records. The child
// of a forked skill has none.
func (d *delegations) readMetadata(binding agentBinding) (delegationMetadata, error) {
	var meta delegationMetadata
	raw, err := d.subagentFile(binding, "meta.json", 16384)
	if err != nil {
		return meta, err
	}
	if _, err = wire.Fields(raw, nil); err != nil || json.Unmarshal(raw, &meta) != nil {
		return meta, errDelegationUnverified
	}
	return meta, nil
}

// subagentFile reads one of native's files for this child, bounded and inside the projects
// tree. Absence is errMetadataPending: native writes these asynchronously.
func (d *delegations) subagentFile(binding agentBinding, suffix string, limit int64) ([]byte, error) {
	if filepath.Base(binding.TranscriptPath) != binding.SessionID+".jsonl" {
		return nil, errDelegationUnverified
	}
	rel, err := filepath.Rel(d.projects, filepath.Dir(binding.TranscriptPath))
	if err != nil {
		return nil, errDelegationUnverified
	}
	root, err := d.openProjects(rel)
	if errors.Is(err, errProjectsAbsent) {
		return nil, errMetadataPending
	}
	if err != nil {
		return nil, errDelegationUnverified
	}
	defer root.Close()
	// Root.Open prevents symlink/reparse traversal outside the launcher-owned
	// projects directory. The hook cannot make us read an arbitrary file.
	raw, err := workflowRead(root, filepath.Join(rel, binding.SessionID, "subagents", "agent-"+binding.ID+"."+suffix), limit)
	if os.IsNotExist(err) {
		return nil, errMetadataPending
	}
	if err != nil {
		return nil, errDelegationUnverified
	}
	return raw, nil
}

// nativeFork verifies the child of a forked skill -- frontmatter `context: fork`, including
// built-in commands such as /code-review -- and returns the route native's own receipt
// records for this turn (#81).
//
// Native starts that child without an Agent call, so nothing was prepared for it and its
// metadata carries no toolUseId: on 2.1.280 it is agentType, spawnDepth and two request
// flags. What identifies it is the combination of a live SubagentStart registration, a
// general-purpose child at depth 1 under the root conversation with no tool call, and a
// transcript that opens with the skill body as a meta user message. The body is not
// matched: a skill from a file starts "Base directory for this skill: …", a built-in one
// such as code-review starts with its own prompt (both measured on 2.1.280). The Node
// baseline recognised the same case from two sidecar files that native no longer writes.
//
// Native chooses the model, as it does under native-selection, and dispatch checks the
// request against the same receipt. The child's report reaches the parent as the Skill
// tool's own result, so the child is kept out of the delegated-result relay; relaying it
// as well would hand the parent the report twice. No journal is written either: a restart
// verifies the child again from the same evidence.
func (d *delegations) nativeFork(scope delegationScope, id string, binding agentBinding, contexts ...context.Context) (resolvedChoice, bool) {
	var none resolvedChoice
	if scope.parent != "" || binding.ID != id || binding.SessionID != scope.session || bridge.CanonicalRole(binding.Role) != "general-purpose" {
		return none, false
	}
	meta, first, err := d.forkEvidence(binding)
	if errors.Is(err, errMetadataPending) && len(contexts) > 0 {
		ctx, cancel := context.WithTimeout(contexts[0], time.Second)
		defer cancel()
		for delay := 5 * time.Millisecond; errors.Is(err, errMetadataPending); delay = min(delay*2, 100*time.Millisecond) {
			select {
			case <-ctx.Done():
				return none, false
			case <-time.After(delay):
				meta, first, err = d.forkEvidence(binding)
			}
		}
	}
	if err != nil || meta.ToolUseID != "" || meta.ParentAgentID != "" || meta.SpawnDepth != 1 || meta.StoppedByUser || !roleMatches("general-purpose", meta.AgentType, false) {
		return none, false
	}
	var entry struct {
		Type, AgentID, SessionID string
		IsSidechain, IsMeta      bool
		Message                  struct{ Role, Content string }
	}
	if json.Unmarshal(first, &entry) != nil || entry.Type != "user" || entry.AgentID != id || entry.SessionID != scope.session ||
		!entry.IsSidechain || !entry.IsMeta || entry.Message.Role != "user" || entry.Message.Content == "" {
		return none, false
	}
	active := scope.nativeTurn
	if active == nil || !validActiveReceipt(*active, scope.session, id) {
		return none, false
	}
	route, err := bridge.SelectRoute(active.Model, active.Effort)
	if err != nil {
		return none, false
	}
	route.Source = "native-fork"
	choice := resolvedChoice{session: scope.session, role: "general-purpose", route: route}
	for _, model := range bridge.Models {
		if model.ID == route.Model {
			choice.alias = model.Alias
		}
	}
	return choice, true
}

// forkEvidence reads the metadata and the first transcript entry together, so a caller
// waiting for either waits for both.
func (d *delegations) forkEvidence(binding agentBinding) (delegationMetadata, []byte, error) {
	meta, err := d.readMetadata(binding)
	if err != nil {
		return meta, nil, err
	}
	raw, err := d.subagentFile(binding, "jsonl", 1<<20)
	if err != nil {
		return meta, nil, err
	}
	first, _, _ := bytes.Cut(raw, []byte("\n"))
	if len(bytes.TrimSpace(first)) == 0 {
		return meta, nil, errMetadataPending
	}
	return meta, first, nil
}

// cacheFork records a verified fork child. A parallel first request may have done so first.
func (d *delegations) cacheFork(id string, choice resolvedChoice) (bridge.Route, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if chosen, found := d.resolved[id]; found {
		if chosen.session != choice.session || chosen.parent != choice.parent || chosen.route != choice.route {
			return bridge.Route{}, false, errDelegationUnverified
		}
		return chosen.route, true, nil
	}
	choice.receipt = d.noteSelection(SelectionRecord{Session: choice.session, Agent: id, Role: choice.role, Model: choice.route.Model, Effort: choice.route.Effort, Source: choice.route.Source, NativeModel: choice.alias})
	if err := d.cacheChoice(id, choice); err != nil {
		return bridge.Route{}, false, err
	}
	return choice.route, true, nil
}
