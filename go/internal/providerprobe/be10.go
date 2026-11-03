package providerprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

const (
	p3BE10Task  = "p3-be10-task: reply with exactly the word plum"
	p3BE10Call  = "call_p3be10"
	p3BE10Limit = 128
)

// Only enums, booleans and digests leave the classifier, never bodies, IDs or tool output.
type p3BE10Request struct {
	ChildCallSHA256                                                     string
	RequestClass, RequestKind, ChildCallLink, NativeSessionSHA256       string
	Stage, Role, RouteSource, SessionLink, TaskMarkerSource, ToolResult string
	SessionSHA256, AgentSHA256, ParentSHA256, CallSHA256                string
	CallMatch                                                           string
	AgentAvailable, OwnToolResult                                       bool
}

type p3BE10Evidence struct {
	ChildDelivered, ChildEnded, ChildCompleted, ChildFailed bool
	ChildEndCategory                                        string
	// ChildConflict is any evidence that the child binding is not one session, agent, call and turn.
	ResultConflict, ChildConflict          bool
	Issued, Returned, Subagent, Incomplete bool
	ToolResult                             string
	Requests                               []p3BE10Request
}

type p3AgentBackgroundOnce struct {
	childTurns                         map[string]string
	childAgents                        map[string]string
	delivered                          map[string]bool
	pending                            map[[2]int64]*p3BE10Scope
	resultSnapshot                     func() []gateway.AgentResultRecord
	boundGateway                       *gateway.Gateway
	childCalls                         map[string]bool
	mu                                 sync.Mutex
	facts                              p3BE10Evidence
	parentSession, nativeParentSession string
	nativeResult                       string
	nativeCallID, backendCallID        string
}

type p3BE10ScopeKey struct{}
type p3BE10Scope struct {
	results                             func() []gateway.AgentResultRecord
	seq, generation                     int64
	childCall                           string
	owner                               *p3AgentBackgroundOnce
	session, agent, parent, class, path string
	selection                           func() []gateway.SelectionRecord
}

func (a *p3AgentBackgroundOnce) bindGateway(g *gateway.Gateway) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.boundGateway != nil && a.boundGateway != g {
		a.facts.Incomplete = true
		return
	}
	a.boundGateway = g
	a.resultSnapshot = func() []gateway.AgentResultRecord { return g.Diagnose().AgentResults.Recent }
}

// The request context, rather than route Source, binds classification to the actual HTTP request.
func (a *p3AgentBackgroundOnce) bindRequest(r *http.Request, g *gateway.Gateway) {
	scope := &p3BE10Scope{owner: a, session: r.Header.Get("X-Claude-Code-Session-Id"), agent: r.Header.Get("X-Claude-Code-Agent-Id"), parent: r.Header.Get("X-Claude-Code-Parent-Agent-Id"), class: r.Header.Get("X-Claude-Code-Request-Class"), path: r.URL.Path}
	scope.selection = func() []gateway.SelectionRecord { return g.Diagnose().AgentSelections.Recent }
	scope.results = func() []gateway.AgentResultRecord { return g.Diagnose().AgentResults.Recent }
	*r = *r.WithContext(context.WithValue(r.Context(), p3BE10ScopeKey{}, scope))
}

func p3BE10Class(class string) string {
	switch class {
	case "main", "subagent", "auxiliary", "compaction", "workflow":
		return class
	default:
		return "unknown"
	}
}
func p3BE10Kind(path string) string {
	switch path {
	case "/v1/messages":
		return "generation"
	case "/v1/messages/count_tokens":
		return "count"
	default:
		return "other"
	}
}

func (a *p3AgentBackgroundOnce) refreshChild() {
	a.facts.Subagent = a.nativeCallID != "" && a.childCalls[a.nativeCallID]
}

type p3BE10Item struct {
	Type, Role, Name           string
	CallID                     string `json:"call_id"`
	Arguments, Content, Output json.RawMessage
}

type p3BE10Wire struct {
	Tools           []struct{ Name string }
	Input, Messages []p3BE10Item
	agentCallID     string
	hasResults      bool
}

type p3BE10NativePart struct {
	Type, ID, Name string
	Content, Input json.RawMessage
	IsError        bool   `json:"is_error"`
	ToolID         string `json:"tool_use_id"`
}

func p3BE10TaskArguments(raw json.RawMessage) bool {
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = []byte(encoded)
	}
	var args struct {
		Prompt     string
		Background *bool `json:"run_in_background"`
	}
	return json.Unmarshal(raw, &args) == nil && args.Prompt == p3BE10Task && args.Background != nil && *args.Background
}

// IDs are opaque and differ between native and backend histories when confirmations are on.
func p3BE10CallResult(wire p3BE10Wire, native bool) (id, match, result string) {
	var calls []string
	type output struct {
		id, category string
		at           int
	}
	var outputs []output
	position, callAt := 0, -1
	if native {
		for _, item := range wire.Messages {
			var parts []p3BE10NativePart
			if json.Unmarshal(item.Content, &parts) != nil {
				continue
			}
			for _, part := range parts {
				position++
				if item.Role == "assistant" && part.Type == "tool_use" && part.Name == "Agent" && p3BE10TaskArguments(part.Input) {
					calls = append(calls, part.ID)
					callAt = position
				}
				if item.Role == "user" && part.Type == "tool_result" {
					category := "ok"
					if part.IsError {
						category = "error"
					}
					outputs = append(outputs, output{part.ToolID, category, position})
				}
			}
		}
	} else {
		for _, item := range wire.Input {
			position++
			if item.Type == "function_call" && item.Name == "Agent" && p3BE10TaskArguments(item.Arguments) {
				calls = append(calls, item.CallID)
				callAt = position
			}
			if item.Type == "function_call_output" {
				outputs = append(outputs, output{item.CallID, "unknown", position})
			}
		}
	}
	if len(calls) == 0 {
		return "", "absent", "absent"
	}
	if len(calls) != 1 || calls[0] == "" {
		return "", "ambiguous", "absent"
	}
	id, match, result = calls[0], "unique", "absent"
	for _, out := range outputs {
		if out.id != id {
			continue
		}
		if out.at <= callAt {
			result = "conflict"
			continue
		}
		if result != "absent" {
			result = "conflict"
		} else {
			result = out.category
		}
	}
	return
}

func p3BE10Digest(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Text locations are parsed rather than searched in serialized JSON (history can quote tools).
func p3BE10Text(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct{ Type, Text string }
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" || p.Type == "input_text" || p.Type == "output_text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func p3BE10Parse(body []byte, native bool) (p3BE10Wire, p3BE10Request, bool) {
	var wire p3BE10Wire
	e := p3BE10Request{TaskMarkerSource: "absent", ToolResult: "absent"}
	if len(body) > 2<<20 || json.Unmarshal(body, &wire) != nil || len(wire.Tools) > 256 || len(wire.Input) > 256 || len(wire.Messages) > 256 {
		return wire, e, false
	}
	wire.agentCallID, e.CallMatch, e.ToolResult = p3BE10CallResult(wire, native)
	e.CallSHA256 = p3BE10Digest(wire.agentCallID)
	e.OwnToolResult = e.CallMatch == "unique" && e.ToolResult != "absent"
	for _, t := range wire.Tools {
		e.AgentAvailable = e.AgentAvailable || t.Name == "Agent"
	}
	items := wire.Input
	if native {
		items = wire.Messages
	}
	lastUser := -1
	for i, item := range items {
		if item.Role == "user" {
			lastUser = i
		}
	}
	user, history, other := false, false, false
	for i, item := range items {
		text := p3BE10Text(item.Content)
		if strings.Contains(text, "p3-be10-task") {
			if i == lastUser && item.Role == "user" && strings.Contains(text, p3BE10Task) {
				user = true
			} else {
				other = true
			}
		}
		if strings.Contains(string(item.Arguments), "p3-be10-task") || strings.Contains(string(item.Output), "p3-be10-task") {
			history = true
		}
		if item.Type == "function_call_output" {
			wire.hasResults = true
		}
		if native {
			var parts []p3BE10NativePart
			if json.Unmarshal(item.Content, &parts) == nil {
				for _, part := range parts {
					if part.Type == "tool_use" && strings.Contains(string(part.Input), "p3-be10-task") {
						history = true
					}
					if part.Type != "tool_result" {
						continue
					}
					wire.hasResults = true
					if strings.Contains(string(part.Content), "p3-be10-task") {
						history = true
					}
				}
			}
		}
	}
	switch {
	case user && (history || other):
		e.TaskMarkerSource = "mixed"
	case user:
		e.TaskMarkerSource = "user_task"
	case history:
		e.TaskMarkerSource = "tool_history"
	case other:
		e.TaskMarkerSource = "other"
	}
	return wire, e, true
}

// Source is assigned by gateway selection/proof validation, not by a body or a tool's presence.
func p3BE10Role(source string) string {
	base, _, _ := strings.Cut(source, "+")
	switch base {
	case "agent-call-model", "agent-call-role", "agent-call-definition", "parent-route", "delegation-inherited", "native-selection":
		return "child"
	case "catalogue", "alias", "family", "direct", "role":
		return "parent"
	case "native-auto-mode":
		return "auxiliary"
	default:
		return "unknown"
	}
}

func p3BE10Link(session, parent string) string {
	if session == "" {
		return "absent"
	}
	if parent == "" {
		return "unbound"
	}
	if session == parent {
		return "parent_session"
	}
	return "different_session"
}

func (a *p3AgentBackgroundOnce) record(e p3BE10Request) {
	if len(a.facts.Requests) == p3BE10Limit {
		a.facts.Incomplete = true
		return
	}
	a.facts.Requests = append(a.facts.Requests, e)
}

func (a *p3AgentBackgroundOnce) observeIngress(in gateway.Ingress) {
	a.observeIngressAt(1, in)
}

func (a *p3AgentBackgroundOnce) observeIngressAt(generation int64, in gateway.Ingress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if in.Context != nil {
		if scope, ok := in.Context.Value(p3BE10ScopeKey{}).(*p3BE10Scope); ok && scope.owner == a && in.Seq > 0 {
			if a.pending == nil {
				a.pending = map[[2]int64]*p3BE10Scope{}
			}
			key := [2]int64{generation, in.Seq}
			if len(a.pending) >= p3BE10Limit && a.pending[key] == nil {
				a.facts.Incomplete = true
			} else {
				scope.seq, scope.generation = in.Seq, generation
				a.pending[key] = scope
			}
		}
	}
	wire, e, ok := p3BE10Parse(in.Body, true)
	e.Stage = "ingress"
	e.RouteSource = "ingress"
	e.RequestClass, e.RequestKind = p3BE10Class(in.RequestClass), p3BE10Kind(in.Path)
	session, agent, parent := in.Header.Get("X-Claude-Code-Session-Id"), in.Header.Get("X-Claude-Code-Agent-Id"), in.Header.Get("X-Claude-Code-Parent-Agent-Id")
	e.SessionSHA256, e.AgentSHA256, e.ParentSHA256 = p3BE10Digest(session), p3BE10Digest(agent), p3BE10Digest(parent)
	e.Role = "unknown"
	if in.RequestClass == "auxiliary" || in.RequestClass == "compaction" || e.RequestKind == "count" {
		e.Role = "auxiliary"
	}
	if e.RequestKind == "generation" && (in.RequestClass == "main" || in.RequestClass == "subagent") {
		e.Role = "parent"
		if agent != "" {
			e.Role = "child"
		}
	}
	if !ok || e.CallMatch == "ambiguous" || e.ToolResult == "conflict" {
		a.facts.Incomplete = true
	}
	if a.facts.Issued && e.Role == "parent" && wire.hasResults && e.CallMatch != "unique" {
		a.facts.Incomplete = true
	}
	if e.Role == "parent" && e.AgentAvailable && !a.facts.Issued && session != "" {
		if a.nativeParentSession != "" && a.nativeParentSession != session {
			a.facts.Incomplete, a.facts.ChildConflict = true, true
		} else {
			a.nativeParentSession = session
		}
	}
	e.SessionLink = p3BE10Link(session, a.nativeParentSession)
	if a.facts.Issued && e.Role == "parent" && e.SessionLink == "parent_session" && wire.hasResults && (e.CallMatch != "unique" || e.ToolResult == "conflict") {
		a.facts.ResultConflict = true
	}
	// is_error is lost in the backend protocol: keep its category at authenticated ingress.
	if a.facts.Issued && e.Role == "parent" && e.SessionLink == "parent_session" && e.OwnToolResult {
		if a.nativeCallID != "" && a.nativeCallID != wire.agentCallID {
			a.facts.Incomplete = true
			a.facts.ResultConflict = true
		}
		a.nativeCallID = wire.agentCallID
		a.refreshChild()
		if a.nativeResult != "" && a.nativeResult != e.ToolResult {
			a.nativeResult = "conflict"
		} else {
			a.nativeResult = e.ToolResult
		}
		if a.facts.Returned {
			a.facts.ToolResult = a.nativeResult
		}
	}
	a.record(e)
}

func (a *p3AgentBackgroundOnce) observeOutcome(generation int64, r gateway.RequestRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := [2]int64{generation, r.Seq}
	scope := a.pending[key]
	delete(a.pending, key)
	if scope == nil || scope.childCall == "" {
		return
	}
	if r.Path != scope.path || r.RequestClass != scope.class || r.AgentID != scope.agent || r.SelectionVerified == nil || !*r.SelectionVerified || r.Kind != "generation" {
		a.facts.Incomplete, a.facts.ChildConflict = true, true
		return
	}
	if a.delivered == nil {
		a.delivered = map[string]bool{}
	}
	a.delivered[scope.childCall] = r.ResponseDelivered && r.Outcome == "ok" && r.Status == http.StatusOK && r.StreamEnd != nil && r.StreamEnd.TerminalObserved && r.StreamEnd.ClientContext == "none"
}

func (a *p3AgentBackgroundOnce) script(ctx context.Context, call upstream.Call, canaries []string) string {
	scope, bound := ctx.Value(p3BE10ScopeKey{}).(*p3BE10Scope)
	bound = bound && scope.owner == a
	selectedCall, matches := "", 0
	selectedTurn, turnMatches := "", 0
	// No live evidence justifies treating a child claiming class main as subagent generation.
	if bound && p3BE10Role(call.Source) == "child" && scope.path == "/v1/messages" && scope.class == "subagent" && scope.agent != "" && scope.selection != nil {
		for _, rec := range scope.selection() {
			if rec.Session == scope.session && rec.Agent == scope.agent && rec.Parent == scope.parent && rec.Role == "general-purpose" && rec.PresenceVerified && rec.State == "selection_verified" && rec.Call != "" {
				selectedCall = rec.Call
				matches++
			}
		}
	}
	if matches == 1 && scope.results != nil {
		for _, r := range scope.results() {
			if r.Session == scope.session && r.Agent == scope.agent && r.Call == selectedCall && r.NativeTurn != "" {
				selectedTurn = r.NativeTurn
				turnMatches++
			}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	wire, e, ok := p3BE10Parse(call.Body, false)
	e.Stage, e.Role = "backend", p3BE10Role(call.Source)
	e.RouteSource, _, _ = strings.Cut(call.Source, "+")
	if e.Role == "unknown" {
		e.RouteSource = "unknown"
	}
	e.SessionSHA256 = p3BE10Digest(call.Session)
	e.RequestClass, e.RequestKind, e.ChildCallLink = "unknown", "other", "absent"
	if bound {
		e.RequestClass, e.RequestKind = p3BE10Class(scope.class), p3BE10Kind(scope.path)
		e.NativeSessionSHA256, e.AgentSHA256, e.ParentSHA256 = p3BE10Digest(scope.session), p3BE10Digest(scope.agent), p3BE10Digest(scope.parent)
		if scope.class == "auxiliary" || scope.class == "compaction" || e.RequestKind == "count" {
			e.Role = "auxiliary"
		}
	}
	parent := bound && scope.path == "/v1/messages" && scope.class == "main" && scope.agent == "" && scope.session == a.nativeParentSession && scope.session != ""
	if !ok || e.Role == "unknown" || e.CallMatch == "ambiguous" || e.ToolResult == "conflict" {
		a.facts.Incomplete = true
	}
	if !bound {
		a.facts.Incomplete = true
	}
	if a.facts.Issued && e.Role == "parent" && wire.hasResults && e.CallMatch != "unique" {
		a.facts.Incomplete = true
	}
	if parent && a.facts.Issued && wire.hasResults && (e.CallMatch != "unique" || e.ToolResult == "conflict") {
		a.facts.ResultConflict = true
	}
	stream := ""
	switch {
	case parent && e.Role == "parent" && !a.facts.Issued && e.AgentAvailable && e.TaskMarkerSource == "absent" && call.Session != "" && slices.Contains(canaries, "turn1"):
		a.parentSession, a.facts.Issued = call.Session, true
		args, _ := json.Marshal(map[string]any{"description": "p3 background subagent", "prompt": p3BE10Task, "subagent_type": "general-purpose", "run_in_background": true})
		stream = toolStream(p3BE10Call, "Agent", string(args))
	case parent && e.Role == "parent" && a.facts.Issued && call.Session == a.parentSession && e.OwnToolResult:
		if a.backendCallID != "" && a.backendCallID != wire.agentCallID {
			a.facts.Incomplete = true
			a.facts.ResultConflict = true
		}
		a.backendCallID = wire.agentCallID
		a.facts.Returned = true
		a.facts.ToolResult = a.nativeResult
		if a.facts.ToolResult == "" {
			a.facts.ToolResult = "unknown"
		}
		e.ToolResult = a.facts.ToolResult
	case e.Role == "child" && a.facts.Issued && call.Session == a.parentSession && e.TaskMarkerSource == "user_task" && bound && scope.session == a.nativeParentSession && scope.agent != "" && scope.parent == "" && e.RequestKind == "generation" && e.RequestClass == "subagent" && matches == 1 && turnMatches == 1 && scope.seq > 0 && scope.generation > 0:
		if a.childCalls == nil {
			a.childCalls = map[string]bool{}
		}
		if len(a.childCalls) >= p3BE10Limit {
			a.facts.Incomplete = true
		} else {
			a.childCalls[selectedCall] = true
			// One Agent call is one child: another agent or native turn for it is an identity conflict.
			if agent, seen := a.childAgents[selectedCall]; seen && (agent != scope.agent || a.childTurns[selectedCall] != selectedTurn) {
				a.facts.Incomplete, a.facts.ChildConflict = true, true
			}
			if a.childAgents == nil {
				a.childAgents = map[string]string{}
			}
			a.childAgents[selectedCall] = scope.agent
			if a.childTurns == nil {
				a.childTurns = map[string]string{}
			}
			a.childTurns[selectedCall] = selectedTurn
			scope.childCall = selectedCall
		}
		a.refreshChild()
		e.ChildCallLink = "selection_verified"
		e.ChildCallSHA256 = p3BE10Digest(selectedCall)
	}
	e.SessionLink = p3BE10Link(call.Session, a.parentSession)
	if e.Role == "parent" && !e.AgentAvailable && !e.OwnToolResult {
		e.Role = "auxiliary"
	}
	if e.Role == "child" && e.ChildCallLink != "selection_verified" {
		a.facts.Incomplete = true
	}
	if matches > 1 || turnMatches > 1 {
		a.facts.Incomplete, a.facts.ChildConflict = true, true
	}
	a.record(e)
	return stream
}

func (a *p3AgentBackgroundOnce) snapshot() p3BE10Evidence {
	a.mu.Lock()
	query := a.resultSnapshot
	a.mu.Unlock()
	var results []gateway.AgentResultRecord
	if query != nil {
		results = query()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	call, agent := a.nativeCallID, a.childAgents[a.nativeCallID]
	a.facts.ChildDelivered = a.delivered[call]
	a.facts.ChildCompleted, a.facts.ChildEnded, a.facts.ChildFailed = false, false, false
	a.facts.ChildEndCategory = "unobserved"
	matches := 0
	for _, r := range results {
		if call == "" || agent == "" || r.Call != call || r.Agent != agent || r.Session != a.nativeParentSession || r.NativeTurn == "" || r.NativeTurn != a.childTurns[call] {
			continue
		}
		matches++
		a.facts.ChildEnded = r.NativeEndObserved
		if r.NativeEndObserved {
			switch r.EndReason {
			case "answer", "aborted", "error", "refusal":
				a.facts.ChildEndCategory = r.EndReason
			default:
				a.facts.ChildEndCategory = "unknown"
			}
		}
		a.facts.ChildFailed = r.NativeEndObserved && (r.EndReason == "aborted" || r.EndReason == "error" || r.EndReason == "refusal") || r.RequestFailure != ""
		a.facts.ChildCompleted = a.facts.ChildDelivered && r.NativeEndObserved && r.EndReason == "answer" && r.RequestFailure == "" && (r.State == "awaiting_parent" || r.State == "parent_received")
	}
	if matches > 1 {
		a.facts.Incomplete, a.facts.ChildConflict = true, true
		a.facts.ChildCompleted = false
		a.facts.ChildEndCategory = "conflict"
	}
	facts := a.facts
	facts.Requests = slices.Clone(facts.Requests)
	return facts
}

func (a *p3AgentBackgroundOnce) judge(treatment *p3Observed) CaseResult {
	facts := a.snapshot()
	// Case consistency is local; scope.go's p3ScopePinnedIdentityChecked also binds checkpoints to its start pin.
	identityValid := !treatment.IdentityError && treatment.NativeStarts > 0 && len(treatment.Identity) >= 2+treatment.NativeStarts
	if identityValid {
		first := treatment.Identity[0]
		identityValid = first.SHA256 != "" && first.Path != "" && !slices.ContainsFunc(treatment.Identity, func(id providercert.Identity) bool { return id != first })
	}
	switch {
	case p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0:
		return CaseResult{Verdict: VerdictBackgroundEntryReached}
	case !identityValid:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"BE10 native identity changed or a checkpoint is missing"}}
	case treatment.CleanupFailed:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{providercert.ReasonCleanupFailed}}
	case treatment.TimedOut || treatment.SpawnFailed || treatment.IdentityError || treatment.Dropped != 0 || len(treatment.RunErrors) != 0 || len(treatment.ExitCodes) == 0 || slices.ContainsFunc(treatment.ExitCodes, func(code int) bool { return code != 0 }):
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"BE10 harness execution or evidence is incomplete"}}
	case facts.Issued && facts.Returned && facts.ToolResult == "error" && !facts.ResultConflict:
		return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"the background Agent tool returned an error"}}
	case facts.ResultConflict || facts.ChildConflict:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"BE10 result or child identity evidence conflicts"}}
	// A failure bound to this one child's session, agent, call and native turn outranks unrelated classifier gaps.
	case facts.ChildFailed && facts.Issued && facts.Returned && facts.ToolResult == "ok":
		return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"the bound child ended unsuccessfully or its gateway request failed"}}
	case facts.Incomplete:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"BE10 execution or classifier evidence is incomplete"}}
	case !facts.Issued || !facts.Returned:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{fmt.Sprintf("the background Agent call was not shown returned (issued=%t returned=%t)", facts.Issued, facts.Returned)}}
	case facts.ToolResult != "ok":
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"the background Agent tool result category is unknown or conflicting"}}
	case !facts.Subagent:
		return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"the background subagent's request did not reach the backend through the gateway"}}
	case !facts.ChildCompleted:
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"the bound child's gateway delivery and successful native completion are unproved"}}
	}
	return CaseResult{Verdict: providercert.Pass}
}
