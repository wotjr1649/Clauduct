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
	"slices"
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

// selectionRefusal is errDelegationUnverified with a fixed reason label, kept in
// the request's diagnostics so the next intermittent refusal names its branch
// (#211, #215). The label never carries file content or identifiers.
type selectionRefusal string

func (selectionRefusal) Error() string        { return errDelegationUnverified.Error() }
func (selectionRefusal) Is(target error) bool { return target == errDelegationUnverified }

func refusedBecause(reason string) error { return selectionRefusal(reason) }

// refusedAs keeps err's label when it has one and otherwise names the caller's
// branch. Either way the result is errDelegationUnverified to every caller.
func refusedAs(err error, reason string) error {
	var refusal selectionRefusal
	if errors.As(err, &refusal) {
		return refusal
	}
	return refusedBecause(reason)
}

type delegationScope struct {
	workflow        bool
	session, parent string
	nativeModel     string
	route           bridge.Route
	nativeTurn      *nativeTurnReceipt
	parentWait      *parentStep
	teammate        *teammateLink
	// summary is the native step a reasoning summary is shown in, nil for none (#277).
	summary *parentStep
}
type delegationKey struct{ session, call string }
type delegatedChoice struct {
	receipt             *SelectionRecord
	parent, role, alias string
	route               bridge.Route
	inherited           bool
	custom              bool
	// omitted: the call named no subagent_type, so role is the subagent default. Native
	// 2.1.289 reports such an Agent teams teammate as "teammate" (#307).
	omitted bool
}

// omittedTeammateRole is the role native gives an Agent teams teammate whose call named no
// subagent_type (measured 2.1.289, #307).
const omittedTeammateRole = "teammate"

type resolvedChoice struct {
	receipt               *SelectionRecord
	session, parent, call string
	role, alias           string
	route                 bridge.Route
	inherited             bool
	custom                bool
	restored              bool
	// teammate: an Agent teams teammate (#269). Native delivers its reports to the lead as
	// idle notifications, so this gateway keeps no result entry for it.
	teammate bool
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
	selection bridge.Selection
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
	nativeBuiltins      func() bool
	results             agentResults
	workflowCalls       map[delegationKey]workflowOrigin
	workflows           map[delegationKey]workflowRun
	workflowSessions    map[string]string
	workflowPersistence WorkflowPersistenceReport
	workflowSourceDirs  []string
}

// ConfigureSelection installs this launch's immutable routing defaults. It must
// follow ConfigureDelegations and precede starting the native child.
func (g *Gateway) ConfigureSelection(selection bridge.Selection) {
	g.selection = selection.Snapshot()
	if g.delegations != nil {
		g.delegations.selection = g.selection
	}
	g.ring.mu.Lock()
	g.ring.models = g.ring.models[:0]
	for _, model := range g.selection.Catalogue().Models() {
		g.ring.models = append(g.ring.models, model.ID)
	}
	g.ring.mu.Unlock()
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

// ConfigureRoleDefaults installs the launcher's definition reader; native still owns role prompts, tools,
// permission checks, loading and execution. Never substitute a request value for
// a missing definition.
func (g *Gateway) ConfigureRoleDefaults(resolve func(string, bridge.Route) (bridge.Route, bool, error)) {
	if g.delegations != nil {
		g.delegations.roleDefaults = resolve
	}
}

// ConfigureNativeBuiltinRoles reports whether the user set CLAUDE_CODE_SUBAGENT_MODEL (or
// could not be shown not to). Then a built-in role called with neither model nor effort
// runs on native's own choice rather than the role table (#145): native gives
// general-purpose the env model and keeps Explore's and Plan's own, all at the parent's
// effort (2.1.283, measured).
func (g *Gateway) ConfigureNativeBuiltinRoles(set func() bool) {
	if g.delegations != nil {
		g.delegations.nativeBuiltins = set
	}
}

// ConfigureDelegations prepares native's lazily created tree before launch.
// Failure remains diagnostic; openProjects classifies each later access afresh.
func (g *Gateway) ConfigureDelegations(projects string) {
	mkdirErr := os.MkdirAll(projects, 0o700)
	g.delegations = &delegations{selection: g.selection, projects: projects, events: g.nativeEvents.directory, projectsErr: mkdirErr,
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
		if tools[i].Name == "Workflow" {
			tools[i].Description += " Clauduct: resumeFromRunId alone returns verified completed child reports without replaying an ordinary script. To explicitly replay that run, pass resumeFromRunId together with script, scriptPath or a supported local name. Native owns replay and approval: completed agents return cached results; interrupted or failed agents and agents after a failure or changed prompt can run again, including their effects. Stop an active run and wait for its children to end first. For resumable independent steps, use script=\"clauduct:plan-v1\" and args={steps:[{id,prompt,model?,effort?,tools?}]} (1-16 unique IDs). For a step that must use no tools, set tools:[]; this is enforced by the gateway. Omitted tools preserves native tools; a list of exact tool names (up to 64) narrows the native catalogue and callable set, never adds permissions. To continue an interrupted plan, first stop its native task with TaskStop, then pass resumeFromRunId alone. Started plan steps are never rerun: completed reports are reused, unavailable results reported, and only never-started steps execute. Each plan source run can be continued once. Model/effort and tool restrictions remain fixed from the original plan."
			tools[i].Description += " Use the native workflow-authoring contract for agent() options. Clauduct enforces tools:[] and exact tool-name allowlists for agent() as well as plan steps. Custom agentType preserves the native role's instructions, tools and maxTurns; omitted model/effort uses its verified definition defaults. scriptPath and local named .js files are read through native Read with its permissions; partial reads are refused. Same-session recovery after a reaped launcher exit revalidates saved journal, child and termination evidence; missing or altered cached evidence is refused. An explicitly supplied replay script may be edited. A maxTurns option directly on agent() is unsupported; use the native agent definition."
		}
		if tools[i].Name == "SendMessage" {
			// Kept by #144: without it a parent sent a second, empty "notify when done"
			// message to a child it had already resumed (2 of 5 runs).
			tools[i].Description += " Native subagents emit completion events automatically. Omit notify_when_idle for subagents; that native flag is only for peer Claude sessions. A message to a completed child starts a new task on that same agent."
		}
		if tools[i].Name != "Agent" {
			continue
		}
		// Kept by #144 with the inherit entry's sentence: removing both let a parent pass an
		// unrequested model (1 of 5 runs). Native shows the gateway's alias in tool history.
		tools[i].Description += " The native UI uses compatibility aliases; Clauduct status records the original selection and effective backend route separately. Verified original model/effort arguments are restored in your tool history. Omit isolation unless worktree or remote isolation was explicitly requested."
		tools[i].Description += " After an accepted asynchronous launch, you may end the current assistant turn while the agent is still running. To wait, emit a final_answer text message such as 'Waiting for the agent.' with NO function calls. This ends only your current turn, not the user's task; native will resume you when the completion notification arrives. Never call Agent to wait, poll, or retry an accepted task."
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
		// Only what the account lists in its picker is offered; a hidden model's full ID is
		// still accepted when passed (the enum is advisory: the tool is not strict).
		models := d.selection.Catalogue().Models()
		for _, entry := range models {
			if entry.Visible {
				names = append(names, entry.ID)
			}
		}
		// Likewise the efforts: one only a hidden model takes is not offered.
		var efforts []string
		for _, effort := range d.selection.Catalogue().Efforts() {
			if slices.ContainsFunc(models, func(m bridge.Model) bool { return m.Visible && slices.Contains(m.Efforts, effort) }) {
				efforts = append(efforts, effort)
			}
		}
		if len(efforts) == 0 {
			efforts = d.selection.Catalogue().Efforts() // an account that lists nothing in its picker
		}
		if pinned.Model != "" {
			names = []string{pinned.Model, "inherit"}
		}
		model["enum"], _ = json.Marshal(names)
		properties["model"], _ = json.Marshal(model)
		properties["effort"], _ = json.Marshal(struct {
			Type        string   `json:"type"`
			Enum        []string `json:"enum"`
			Description string   `json:"description"`
		}{"string", efforts, "Model without effort uses the selected model's default effort. Effort without model uses the role's default model. Omit unrequested model and effort. A task-bound parent choice is retained by descendants; conflicting overrides are refused."})
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
	_, roleNamed := fields["subagent_type"]
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
		if err != nil && (!custom || !errors.Is(err, bridge.ErrUnsupportedRoute) || !explicitModel && (!parentKnown || !parent.inherited)) {
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
			requested, err := d.selection.SelectRoute(modelID, effort)
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
		route, err = d.selection.SelectRoute(modelID, effort)
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
			if definition.Model != "" {
				route, known = definition, true
				if custom {
					source = "agent-call-definition"
				}
			}
			if !known && (hasEffort || !bridge.BuiltinRole(role) || d.nativeBuiltins == nil || !d.nativeBuiltins()) {
				route, known = d.selection.RoleRoute(role)
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
			route, err = d.selection.SelectRoute(route.Model, effort)
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
	if offered, ok := d.selection.ModelByID(modelID); ok {
		model = &offered
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
		nativeAlias = model.AgentAlias
		if nativeAlias != "" {
			alias, _ := json.Marshal(nativeAlias)
			fields["model"] = alias
		} else if d.events == "" {
			// A model without a legacy tier can only be pinned by the spawn event.
			return nil, bridge.ErrUnsupportedRoute
		} else {
			// Native's argument takes only its tier names, and no tier is guessed for an
			// account model without one. The spawn event below pins the full ID.
			delete(fields, "model")
		}
		// The tool schema still takes Claude aliases. The native spawn event
		// accepts a full ID and pins the call identity, so duplicate alias mappings
		// no longer make a backend model unreachable.
		if d.events != "" {
			nativeAlias = model.ID
		}
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
	if model != nil && d.events != "" {
		if err := d.writeNativeSelection(scope, id, role, !roleNamed, route); err != nil {
			return nil, err
		}
	}
	receipt := d.noteSelection(SelectionRecord{Session: scope.session, Parent: scope.parent, Call: id, Role: role, CustomRole: custom, RequestedModel: requestedModel, RequestedEffort: effort, ModelProvided: modelProvided, EffortProvided: hasEffort, PresenceVerified: true, Model: route.Model, Effort: route.Effort, Source: route.Source, NativeModel: nativeAlias})
	if scope.nativeTurn != nil && scope.nativeTurn.Session == scope.session && scope.nativeTurn.Agent == scope.parent {
		receipt.turn = scope.nativeTurn.Turn
		receipt.arguments, _ = agentArguments(raw)
	}
	d.pending[key] = delegatedChoice{parent: scope.parent, role: role, alias: nativeAlias, route: route, inherited: inherited, receipt: receipt, custom: custom, omitted: !roleNamed}
	return encoded, nil
}

// Caller holds mu. Every cache hit retains the same identity and native-stop checks.
func (d *delegations) cachedRoute(scope delegationScope, id string, binding agentBinding) (bridge.Route, bool, error) {
	if chosen, found := d.resolved[id]; found {
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
	return bridge.Route{}, false, nil
}

func (d *delegations) route(scope delegationScope, id string, binding agentBinding, contexts ...context.Context) (bridge.Route, bool, error) {
	// A teammate's binding role is its name, not a role: it never takes the cached check below.
	if scope.teammate != nil {
		return d.teammateRoute(scope, id, binding, contexts...)
	}
	d.mu.Lock()
	// A teammate's requests carry its address. Its loop id arriving bare would skip the
	// teammate checks (task, address, stop) that only the address path makes.
	if chosen, held := d.resolved[id]; held && chosen.teammate {
		d.mu.Unlock()
		return bridge.Route{}, false, errDelegationUnverified
	}
	if route, found, err := d.cachedRoute(scope, id, binding); found || err != nil {
		d.mu.Unlock()
		return route, found, err
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
				return bridge.Route{}, false, refusedBecause("metadata_wait_expired")
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
		return bridge.Route{}, false, refusedAs(err, "metadata_unreadable")
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
		if active == nil || !validActiveReceipt(d.selection, *active, scope.session, id) {
			return bridge.Route{}, false, errDelegationUnverified
		}
		actual, err := d.selection.SelectRoute(active.Model, active.Effort)
		if err != nil {
			return bridge.Route{}, false, errDelegationUnverified
		}
		actual.Source = "native-selection"
		choice.route = actual
		if model, ok := d.selection.ModelByID(actual.Model); ok {
			choice.alias = model.AgentAlias
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

// teammateRoute binds an Agent teams teammate (#269) to the Agent call that started it.
// Its native metadata names no tool call or parent: the call and role come from the spawn
// the session plugin saw. Every request checks that the metadata is still a teammate task
// at the address the request carried, with no tool call or parent and not stopped; the first
// also checks the model against the call's choice (or native's receipt) and consumes the
// lead's pending choice for that call, as an ordinary child's is.
func (d *delegations) teammateRoute(scope delegationScope, id string, binding agentBinding, contexts ...context.Context) (bridge.Route, bool, error) {
	link := scope.teammate
	if binding.ID != id || binding.SessionID != scope.session || link.Agent != id || link.Session != scope.session {
		return bridge.Route{}, false, errDelegationUnverified
	}
	meta, err := d.readMetadata(binding)
	if errors.Is(err, errMetadataPending) && len(contexts) != 0 {
		ctx, cancel := context.WithTimeout(contexts[0], time.Second)
		defer cancel()
		for delay := 5 * time.Millisecond; errors.Is(err, errMetadataPending); delay = min(delay*2, 100*time.Millisecond) {
			select {
			case <-ctx.Done():
				return bridge.Route{}, false, refusedBecause("metadata_wait_expired")
			case <-time.After(delay):
				meta, err = d.readMetadata(binding)
			}
		}
	}
	if err != nil {
		return bridge.Route{}, false, refusedAs(err, "metadata_unreadable")
	}
	// Every request, not only the first: the record must still be this teammate's, and a
	// teammate the user stopped stays stopped.
	identity := meta.TaskKind == "in_process_teammate" && meta.Name+"@"+meta.TeamName == link.Address && meta.ToolUseID == "" &&
		meta.ParentAgentID == scope.parent && binding.Role == meta.Name && !meta.StoppedByUser
	d.mu.Lock()
	defer d.mu.Unlock()
	if !identity {
		// Before its first request binds, this teammate will never bind: release the call so
		// the lead's completion check is not held open by a choice nothing can consume.
		if _, bound := d.resolved[id]; !bound {
			delete(d.pending, delegationKey{scope.session, link.Call})
		}
		return bridge.Route{}, false, errDelegationUnverified
	}
	if chosen, found := d.resolved[id]; found {
		if chosen.session != scope.session || chosen.parent != scope.parent || chosen.call != link.Call || !roleMatches(chosen.role, link.Role, chosen.custom) {
			return bridge.Route{}, false, errDelegationUnverified
		}
		return chosen.route, true, nil
	}
	key := delegationKey{scope.session, link.Call}
	choice, found := d.pending[key]
	if !found {
		return bridge.Route{}, false, errDelegationUnverified
	}
	custom := choice.custom || strings.HasPrefix(choice.route.Source, "agent-call-definition")
	role := choice.role
	if !custom {
		role = bridge.CanonicalRole(role)
	}
	failure := ""
	// Where native chose the model, its receipt for this teammate's turn names it, as for
	// an ordinary child; the metadata then records what native actually ran.
	if choice.route.Source == "native-selection" {
		active := scope.nativeTurn
		if active == nil || !validActiveReceipt(d.selection, *active, scope.session, id) {
			failure = "NATIVE_RECEIPT_UNVERIFIED"
		} else if actual, err := d.selection.SelectRoute(active.Model, active.Effort); err != nil {
			failure = "NATIVE_RECEIPT_UNVERIFIED"
		} else {
			actual.Source = "native-selection"
			choice.route = actual
			if model, ok := d.selection.ModelByID(actual.Model); ok {
				choice.alias = model.AgentAlias
			}
		}
	}
	switch {
	case failure != "":
	case choice.parent != scope.parent:
		failure = "PARENT_MISMATCH"
	case !roleMatches(role, link.Role, custom) && !(choice.omitted && link.Role == omittedTeammateRole):
		failure = "ROLE_MISMATCH"
	case meta.Model != choice.route.Model && !(choice.route.Source == "native-selection" && meta.Model == ""):
		failure = "METADATA_MODEL_MISMATCH"
	}
	if failure != "" {
		if choice.receipt != nil {
			choice.receipt.Agent, choice.receipt.Failure = id, failure
			d.selectionState(choice.receipt, "selection_failed")
		}
		// The spawn succeeded, so no tool failure will clear this choice; left pending it
		// would hold the lead's completion check open for the rest of the session.
		delete(d.pending, key)
		return bridge.Route{}, false, errDelegationUnverified
	}
	// Later requests compare against what native reports for this teammate. The receipt keeps
	// the call's own role: the lead's Agent history is restored by it (selection_records.go).
	boundRole := role
	if choice.omitted && link.Role == omittedTeammateRole {
		boundRole = link.Role
	}
	chosen := resolvedChoice{session: scope.session, parent: scope.parent, call: link.Call, role: boundRole, alias: choice.alias, route: choice.route, inherited: choice.inherited, custom: custom, teammate: true}
	chosen.receipt = choice.receipt
	if chosen.receipt != nil {
		chosen.receipt.Role = role
		chosen.receipt.Model, chosen.receipt.Effort, chosen.receipt.NativeModel = choice.route.Model, choice.route.Effort, choice.alias
	}
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
	return choice.route.Source == "native-fork" || choice.teammate || d.results.start(id, choice)
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
		for key, held := range d.resolved {
			if held.teammate {
				continue // owes no result, but its choice cannot be rebuilt once dropped
			}
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
	if c.alias == c.route.Model {
		record.Version = 3 // exact native selector, independent of configurable aliases
	}
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
		// Routed roles require evidence; other roles retain admission's missing-choice policy.
		if d.selection.KnownRole(binding.Role) {
			return empty, false, errDelegationUnverified
		}
		return empty, false, nil
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
	if decodeErr != nil || (saved.Version != 1 && saved.Version != 2 && saved.Version != 3) || saved.Session != scope.session || saved.Parent != scope.parent || saved.Agent != id || !roleMatches(saved.Role, binding.Role, saved.CustomRole) {
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
	// The saved pair must still be offered by this session's account list.
	if !d.selection.ValidPair(bridge.Pair{Model: saved.Model, Effort: saved.Effort}) {
		if d.selection.Retires(saved.Model) {
			return empty, false, bridge.ErrRetiredRoute
		}
		return empty, false, errDelegationUnverified
	}
	route := bridge.Route{Model: saved.Model, Effort: saved.Effort}
	model, ok := d.selection.ForAlias(saved.Alias)
	if saved.Version == 3 {
		model, ok = d.selection.ModelByID(saved.Alias)
	} else if !ok || model.ID != route.Model {
		// The tier alias moved (v0.6.3: opus is GPT-6.1 Sol) or the model has none: accept
		// only the model the journal names, and only when its own Agent tier is the alias.
		if byID, known := d.selection.ModelByID(route.Model); known && byID.Alias == "" && byID.AgentAlias != "" {
			model, ok = byID, byID.AgentAlias == saved.Alias
		}
	}
	if !ok || model.ID != route.Model {
		return empty, false, errDelegationUnverified
	}
	switch saved.Source {
	case "agent-call-model", "agent-call-model+effort", "agent-call-role", "agent-call-role+effort", "agent-call-definition", "agent-call-definition+effort", "parent-route", "parent-route+effort", "delegation-inherited", "delegation-inherited+effort", "native-selection":
	default:
		return empty, false, errDelegationUnverified
	}
	route.Source = saved.Source
	choice := resolvedChoice{session: saved.Session, parent: saved.Parent, call: saved.Call, role: saved.Role, alias: saved.Alias, route: route, inherited: saved.Inherited, custom: saved.CustomRole, restored: true}
	if saved.Intent != nil {
		intent := saved.Intent
		if saved.Version < 2 {
			return empty, false, errDelegationUnverified
		}
		if _, err := wire.Fields(fields["intent"], []string{"model", "effort", "modelProvided", "effortProvided"}); err != nil {
			return empty, false, errDelegationUnverified
		}
		modelOK := intent.Model == "model-family" || selectionModelLabel(intent.Model) == intent.Model
		effortOK := intent.Effort == "" || bridge.TransmittableEffort(intent.Effort)
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
	// An Agent teams teammate records these instead of a tool call and parent (native 2.1.289).
	TaskKind string `json:"taskKind"`
	Name     string `json:"name"`
	TeamName string `json:"teamName"`
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
	// Native writes the sidecar in place: an empty or cut-off document is still
	// arriving, like an absent one, and gets the same bounded first wait.
	if partialJSON(raw) {
		return meta, errMetadataPending
	}
	if _, err = wire.Fields(raw, nil); err != nil || json.Unmarshal(raw, &meta) != nil {
		return meta, refusedBecause("metadata_invalid")
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
// general-purpose child with no tool call one level below the conversation that ran the
// skill (the root at depth 1, or a verified subagent of this session at its depth + 1), and a
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
	if binding.ID != id || binding.SessionID != scope.session || bridge.CanonicalRole(binding.Role) != "general-purpose" {
		return none, false
	}
	// A fork started inside a subagent sits one level below a child this gateway already
	// verified in the same session, and its metadata names that child (measured on 2.1.283:
	// spawnDepth 2, parentAgentId the subagent). The parent's own sidecar gives its depth.
	depth := 1
	if scope.parent != "" {
		d.mu.Lock()
		parent, verified := d.resolved[scope.parent]
		d.mu.Unlock()
		if !verified || parent.session != scope.session {
			return none, false
		}
		parentMeta, err := d.readMetadata(agentBinding{ID: scope.parent, SessionID: scope.session, TranscriptPath: binding.TranscriptPath})
		if err != nil || parentMeta.SpawnDepth < 1 || parentMeta.StoppedByUser {
			return none, false
		}
		depth = parentMeta.SpawnDepth + 1
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
	if err != nil || meta.ToolUseID != "" || meta.ParentAgentID != scope.parent || meta.SpawnDepth != depth || meta.StoppedByUser || !roleMatches("general-purpose", meta.AgentType, false) {
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
	if active == nil || !validActiveReceipt(d.selection, *active, scope.session, id) {
		return none, false
	}
	route, err := d.selection.SelectRoute(active.Model, active.Effort)
	if err != nil {
		return none, false
	}
	route.Source = "native-fork"
	choice := resolvedChoice{session: scope.session, parent: scope.parent, role: "general-purpose", route: route}
	if model, ok := d.selection.ModelByID(route.Model); ok {
		choice.alias = model.AgentAlias
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
	first, _, ended := bytes.Cut(raw, []byte("\n"))
	if len(bytes.TrimSpace(first)) == 0 || !ended && partialJSON(first) {
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
	choice.receipt = d.noteSelection(SelectionRecord{Session: choice.session, Parent: choice.parent, Agent: id, Role: choice.role, Model: choice.route.Model, Effort: choice.route.Effort, Source: choice.route.Source, NativeModel: choice.alias})
	if err := d.cacheChoice(id, choice); err != nil {
		return bridge.Route{}, false, err
	}
	return choice.route, true, nil
}
