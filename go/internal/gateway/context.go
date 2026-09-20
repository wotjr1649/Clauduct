package gateway

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type contextState struct {
	route            bridge.Route
	target           string
	phase            string // required, compacting, recount, failed; empty is ordinary work
	busy             bool
	usage            *contextUsageAnchor
	textEstimate     int64
	estimate         int64
	estimateSource   string
	opaqueInput      bool
	journal          string
	identity         string
	saved            string
	persistenceError bool
}

type AgentContextReport struct {
	SessionID         string              `json:"sessionId"`
	AgentID           string              `json:"agentId,omitempty"`
	Model             string              `json:"model,omitempty"`
	Effort            string              `json:"effort,omitempty"`
	PendingModel      string              `json:"pendingModel,omitempty"`
	Phase             string              `json:"phase"`
	PhaseMeaning      string              `json:"phaseMeaning"`
	LastUsage         *contextUsageAnchor `json:"lastBackendUsage,omitempty"`
	EstimatedInput    int64               `json:"estimatedInputTokens"`
	EstimateSource    string              `json:"estimateSource"`
	OpaqueInput       bool                `json:"containsUnestimatedMediaOrReasoning"`
	Persistent        bool                `json:"persistent"`
	PersistenceFailed bool                `json:"persistenceFailed,omitempty"`
}

func (g *Gateway) agentContexts() []AgentContextReport {
	if g.contexts == nil {
		return nil
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.states))
	for key := range c.states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]AgentContextReport, 0, len(keys))
	for _, key := range keys {
		s := c.states[key]
		session, agent, _ := strings.Cut(key, "/")
		phase := s.phase
		if phase == "" {
			phase = "ready"
		}
		var usage *contextUsageAnchor
		if s.usage != nil {
			copy := *s.usage
			usage = &copy
		}
		out = append(out, AgentContextReport{SessionID: session, AgentID: agent, Model: s.route.Model, Effort: s.route.Effort, PendingModel: s.target, Phase: phase, PhaseMeaning: "context_policy_state_not_turn_outcome", LastUsage: usage, EstimatedInput: s.estimate, EstimateSource: s.estimateSource, OpaqueInput: s.opaqueInput, Persistent: s.saved != "", PersistenceFailed: s.persistenceError})
	}
	return out
}

type contextGuard struct {
	mu       sync.Mutex
	states   map[string]*contextState
	tickets  map[string]compactTicket
	sessions map[string]string
}

type compactTicket struct {
	session, agent string
	at             time.Time
	trigger        string
}

var compactTicketPattern = regexp.MustCompile(`\[clauduct-compact:([A-Za-z0-9_-]{43})\]`)

// EnableContextPolicy is wired by the production launcher before starting Claude.
// Generation uses observed usage and preventive estimates, never a remote preflight.
func (g *Gateway) EnableContextPolicy() {
	g.contexts = &contextGuard{states: make(map[string]*contextState), tickets: make(map[string]compactTicket), sessions: make(map[string]string)}
}

func contextKey(session, agent string) string { return session + "/" + agent }

// Since native 2.1.273 the authenticated request class distinguishes tool-less
// conversations from title/classifier side calls. Older clients use the measured
// tool/template fallback and retain its explicitly reported limitations.
func conversationRequest(r *http.Request, request *anthropic.Request) bool {
	switch r.Header.Get("X-Claude-Code-Request-Class") {
	case "main", "subagent", "workflow", "compaction":
		return true
	case "auxiliary":
		return false
	default:
		return len(request.Tools) > 0 || bridge.IsCompaction(request)
	}
}

func policyFor(model string) (bridge.ContextPolicy, bool) {
	for _, entry := range bridge.Models {
		if model == entry.ID {
			return entry.Context, true
		}
	}
	return bridge.ContextPolicy{}, false
}

// Compaction is authorized by the client's event on the authenticated loopback
// channel, scoped to its session and agent. Prompt wording alone grants nothing.
func (g *Gateway) handleContextEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.refuse(w, refuseMethod)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		g.refuse(w, refuseMediaType)
		return
	}
	raw, ok := g.readBounded(w, r, maxBindingBytes)
	if !ok {
		return
	}
	fields, err := wire.Fields(raw, []string{"event", "sessionId", "agentId", "transcriptPath", "trigger"})
	var event struct {
		Event      string `json:"event"`
		Session    string `json:"sessionId"`
		Agent      string `json:"agentId"`
		Transcript string `json:"transcriptPath"`
		Trigger    string `json:"trigger"`
	}
	if err != nil || len(fields) < 2 || json.Unmarshal(raw, &event) != nil ||
		!correlationShape.MatchString(event.Session) || (event.Agent != "" && !correlationShape.MatchString(event.Agent)) ||
		(event.Trigger != "" && event.Trigger != "manual" && event.Trigger != "auto") ||
		(event.Event != "PreCompact" && event.Event != "PostCompact" && event.Event != "SessionStart") {
		g.refuseCategory(w, 400, "INVALID_CONTEXT_EVENT")
		return
	}
	if g.contexts == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	if event.Event == "SessionStart" {
		if err := g.contextSession(event.Session, event.Transcript); err != nil {
			g.refuseCategory(w, 400, "CONTEXT_JOURNAL_UNVERIFIED")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if event.Event == "PostCompact" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for ticket, receipt := range c.tickets {
		if time.Since(receipt.at) > 5*time.Minute {
			delete(c.tickets, ticket)
		}
	}
	if len(c.tickets) >= 128 {
		g.refuseCategory(w, 400, "CONTEXT_EVENT_LIMIT")
		return
	}
	ticket, err := newToken()
	if err != nil {
		g.refuseCategory(w, 400, "CONTEXT_EVENT_FAILED")
		return
	}
	c.tickets[ticket] = compactTicket{session: event.Session, agent: event.Agent, at: time.Now(), trigger: event.Trigger}
	// Claude 2.1.275 omits agent_id from PreCompact, even in children. Its
	// stdout custom-instruction channel carries this one-use receipt back in
	// the actual compaction request, whose headers identify the correct child.
	// No FIFO or shared process environment decides the association.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"ticket": ticket})
}

// beginContext pins a compaction to the preceding route, including effort. No
// lock is held over counting, inference or delivery; independent agents proceed.
func (g *Gateway) beginContext(r *http.Request, request *anthropic.Request, entry *record, override []bridge.Route) ([]bridge.Route, func(), string) {
	if g.contexts == nil {
		return override, func() {}, ""
	}
	if r.Header.Get("X-Claude-Code-Request-Class") == "" {
		return override, func() {}, "CONTEXT_REQUEST_CLASS_UNVERIFIED"
	}
	nativeCompact := r.Header.Get("X-Claude-Code-Request-Class") == "compaction"
	compact := nativeCompact || bridge.IsCompaction(request)
	if !conversationRequest(r, request) {
		return override, func() {}, ""
	}
	session, agent := r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id")
	if session == "" {
		return override, func() {}, "CONTEXT_IDENTITY_UNVERIFIED"
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	if g.delegations != nil && c.sessions[session] == "" {
		return override, func() {}, "CONTEXT_SESSION_UNVERIFIED"
	}
	key := contextKey(session, agent)
	s := c.states[key]
	if s == nil {
		if len(c.states) >= maxAgents {
			for old, state := range c.states {
				// A state whose journal cannot be written is still restorable from the
				// journal it last wrote, so it is no less evictable than any other. Excluding
				// it meant a poisoned entry held one of these slots permanently, and a
				// machine with a sticky write error eventually refused every new agent.
				if !state.busy && state.saved != "" {
					delete(c.states, old)
					break
				}
			}
			if len(c.states) >= maxAgents {
				return override, func() {}, "CONTEXT_STATE_LIMIT"
			}
		}
		s = &contextState{}
		if err := g.restoreContext(session, agent, s); err != nil {
			return override, func() {}, "CONTEXT_JOURNAL_UNVERIFIED"
		}
		c.states[key] = s
	}
	if s.persistenceError {
		// Memory ahead of disk is worth stopping for, but this was a one-way latch: the only
		// other saveContext caller runs after this check, so nothing could ever clear the
		// flag. One transient failure -- a temp file an antivirus still holds across the
		// rename, a momentary ACL error -- ended that session and agent for the life of the
		// process. Retrying the write here is the recovery the flag always needed; a failure
		// that is not transient still refuses, but only this request.
		if g.saveContext(s) != nil {
			return override, func() {}, "CONTEXT_JOURNAL_FAILED"
		}
	}
	if s.busy {
		return override, func() {}, "CONTEXT_REQUEST_CONFLICT"
	}
	if s.phase == "failed" && !compact {
		return override, func() {}, "CONTEXT_COMPACTION_FAILED"
	}
	if compact {
		ticket := ""
		for _, message := range request.Messages {
			if message.Role != "user" {
				continue
			}
			for _, block := range message.Blocks {
				if block.Type == "text" {
					if match := compactTicketPattern.FindStringSubmatch(block.Text); len(match) == 2 {
						ticket = match[1]
					}
				}
			}
		}
		receipt, found := c.tickets[ticket]
		validReceipt := found && receipt.session == session && (receipt.agent == "" || receipt.agent == agent) && time.Since(receipt.at) <= 5*time.Minute
		// Only an explicit native /compact event can retry a failed compaction.
		// An automatic event or a new generation request cannot restart the work.
		if (s.phase == "failed" || s.phase == "recount") && validReceipt && receipt.trigger == "manual" {
			s.phase = "required"
		}
		if (!nativeCompact && !validReceipt) || (s.phase != "" && s.phase != "required") {
			return override, func() {}, "CONTEXT_COMPACTION_UNVERIFIED"
		}
		delete(c.tickets, ticket)
		// Remove local correlation data before counting or contacting the backend.
		for i := range request.Messages {
			for j := range request.Messages[i].Blocks {
				block := &request.Messages[i].Blocks[j]
				if block.Type == "text" {
					block.Text = strings.ReplaceAll(block.Text, "[clauduct-compact:"+ticket+"]", "")
				}
			}
		}
		if s.route.Model != "" {
			override = []bridge.Route{s.route}
		}
		s.phase = "compacting"
		if g.saveContext(s) != nil {
			return override, func() {}, "CONTEXT_JOURNAL_FAILED"
		}
		entry.kind("compaction")
	} else if s.phase != "" && s.phase != "recount" {
		return override, func() {}, "CONTEXT_COMPACTION_REQUIRED"
	}
	s.busy = true
	return override, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		s.busy = false
		result := entry.snapshot()
		if compact {
			if result.Status == 200 && result.Category == "" {
				s.phase = "recount"
				// The old full-history usage is not the size of the new summary.
				s.usage = nil
			} else {
				s.phase = "failed"
			}
		} else if result.InputTokens != nil && result.OutputTokens != nil {
			s.usage = &contextUsageAnchor{Model: result.Model, Effort: result.Effort, Input: *result.InputTokens, Output: *result.OutputTokens, TextEstimate: s.textEstimate}
			if result.Category == "" && result.Status == 200 && s.phase == "recount" {
				s.phase, s.target = "", ""
			}
		}
		if g.saveContext(s) != nil && entry.snapshot().Category == "" {
			g.broken.Add(1)
			entry.brokeAfterCommitting("CONTEXT_JOURNAL_FAILED")
		}
	}, ""
}

func (g *Gateway) checkContext(w http.ResponseWriter, r *http.Request, request *anthropic.Request, built *bridge.Request) bool {
	if g.contexts == nil {
		return true
	}
	entry := recordOf(w)
	policy, known := policyFor(built.Model)
	if !known {
		g.refuseCategory(w, 400, "CONTEXT_POLICY_UNVERIFIED")
		return false
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.states[contextKey(r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"))]
	if !conversationRequest(r, request) {
		s = nil // A title/classifier shares identity headers, never conversation usage.
	}
	text, opaque := estimateTextInput(built)
	tokens, source := text, "text_estimate"
	if s != nil && s.usage != nil {
		// Growth is deliberately conservative: generated output is reserved in
		// full, even when some visible output also appears in the text delta.
		// It is an estimate, not a token count or a hard admission guarantee.
		tokens = max(text, s.usage.Input+s.usage.Output+text-s.usage.TextEstimate)
		source = "backend_usage_plus_text_estimate"
		if s.usage.Model != built.Model {
			source = "previous_model_usage_plus_text_estimate"
		}
	}
	entry.contextEstimate(tokens, source, opaque)
	entry.checked("context_policy")
	// Save route/phase transitions, never conversation content. A checkpoint
	// failure stops this request before generation rather than losing switch state.
	checkpoint := func() bool {
		if s != nil && g.saveContext(s) != nil {
			g.refuseCategory(w, 400, "CONTEXT_JOURNAL_FAILED")
			return false
		}
		return true
	}
	compact := entry.snapshot().Kind == "compaction"
	if s != nil && s.busy && conversationRequest(r, request) {
		s.textEstimate, s.estimate, s.estimateSource, s.opaqueInput = text, tokens, source, opaque
	}
	if compact {
		if s != nil && s.route.Model == "" {
			s.route = bridge.Route{Model: built.Model, Effort: built.Effort.Effort, Source: built.Source}
		}
		// A configured window is a management target, not the backend's hard
		// limit. An estimate cannot forbid the very compaction needed to recover.
		return checkpoint()
	}
	if !conversationRequest(r, request) {
		if tokens >= policy.CompactAt {
			g.refuseCategory(w, 400, "CONTEXT_COMPACTION_UNAVAILABLE")
			return false
		}
		return true
	}
	switching := s != nil && s.route.Model != "" && s.route.Model != built.Model && s.phase != "recount"
	// A changed name alone never compacts history. Reassess against the
	// destination target; cross-model estimates remain explicitly unverified.
	if tokens >= policy.CompactAt {
		if s != nil && s.phase == "recount" {
			s.phase = "failed"
			g.refuseCategory(w, 400, "CONTEXT_COMPACTION_INSUFFICIENT")
			return false
		}
		if s == nil || !s.busy {
			g.refuseCategory(w, 400, "CONTEXT_COMPACTION_UNAVAILABLE")
			return false
		}
		if s.route.Model == "" {
			s.route = bridge.Route{Model: built.Model, Effort: built.Effort.Effort, Source: built.Source}
		}
		s.target, s.phase = built.Model, "required"
		if !checkpoint() {
			return false
		}
		// Native's numeric overflow parser truncates history; this fixed signal
		// enters native compaction without inventing usage or deleting messages.
		reason := "CONTEXT_THRESHOLD_COMPACTION_REQUIRED"
		if switching {
			reason = "MODEL_SWITCH_COMPACTION_REQUIRED"
		}
		entry.control(reason)
		g.refuseCategory(w, 400, "prompt is too long")
		return false
	}
	if s != nil && s.busy {
		if s.phase == "recount" && s.target != "" && s.target != built.Model {
			g.refuseCategory(w, 400, "CONTEXT_SWITCH_UNVERIFIED")
			return false
		}
		s.route = bridge.Route{Model: built.Model, Effort: built.Effort.Effort, Source: built.Source}
		if s.phase != "recount" {
			s.phase, s.target = "", ""
		}
	}
	return checkpoint()
}

// Only a structured backend overflow, before any delivered operation, reaches
// this path. One native compaction is allowed; an insufficient summary stops.
func (g *Gateway) recoverContextOverflow(w http.ResponseWriter, session, agent, model string) bool {
	if g.contexts == nil {
		return false
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.states[contextKey(session, agent)]
	if s == nil || !s.busy {
		return false
	}
	entry := recordOf(w)
	if s.phase == "compacting" || s.phase == "recount" || s.phase == "failed" || entry.snapshot().Kind == "compaction" {
		s.phase = "failed"
		if g.saveContext(s) != nil {
			g.refuseCategory(w, 400, "CONTEXT_JOURNAL_FAILED")
		} else {
			g.refuseCategory(w, 400, "CONTEXT_COMPACTION_INSUFFICIENT")
		}
		return true
	}
	if s.usage != nil {
		s.route = bridge.Route{Model: s.usage.Model, Effort: s.usage.Effort, Source: "usage-before-overflow"}
	}
	s.target, s.phase = model, "required"
	if g.saveContext(s) != nil {
		g.refuseCategory(w, 400, "CONTEXT_JOURNAL_FAILED")
		return true
	}
	entry.control("BACKEND_CONTEXT_COMPACTION_REQUIRED")
	g.refuseCategory(w, 400, "prompt is too long")
	return true
}
