package gateway

import (
	"errors"
	"net/http"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Caller holds contexts.mu. Reading a receipt does not consume it: count_tokens
// must prepare the same request without advancing the compaction state machine.
func (g *Gateway) compactReceipt(session, agent string, request *anthropic.Request) (string, compactTicket, bool) {
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
	receipt, found := g.contexts.tickets[ticket]
	valid := found && receipt.session == session && (receipt.agent == "" || receipt.agent == agent) && time.Since(receipt.at) <= 5*time.Minute
	return ticket, receipt, valid
}

// compactionRoute is the current selection of the session or Agent being compacted,
// automatic and manual alike: the Agent's own route, or the model and effort native states
// on the request itself. The route of an earlier request is not reused, a one-shot turn
// effort does not apply, and no cap lowers the effort.
func (g *Gateway) compactionRoute(request *anthropic.Request, override []bridge.Route) (bridge.Route, error) {
	if len(override) > 0 {
		return g.selection.ResolveRoute(request, override[0])
	}
	route, err := g.selection.SelectRoute(request.Model, request.Effort)
	if err != nil {
		return bridge.Route{}, err
	}
	return g.selection.ResolveRoute(request, route)
}

func stripCompactReceipts(request *anthropic.Request) {
	for i := range request.Messages {
		for j := range request.Messages[i].Blocks {
			block := &request.Messages[i].Blocks[j]
			if block.Type == "text" {
				block.Text = compactTicketPattern.ReplaceAllString(block.Text, "")
			}
		}
	}
}

func addCompactGuidance(built *bridge.Request) {
	built.Input = append([]bridge.InputEntry{{Role: "developer", Content: bridge.CompactEfficiencyInstruction}}, built.Input...)
	built.ToolChoice = "none" // Native summaries cannot execute tools; count uses the same payload.
}

// Read-only preview. Generation still validates phase, claims the state, consumes
// the receipt and saves the journal in beginContext. No count authorizes a turn.
func (g *Gateway) previewCompaction(r *http.Request, request *anthropic.Request, override []bridge.Route) ([]bridge.Route, bool, string) {
	nativeCompact := r.Header.Get("X-Claude-Code-Request-Class") == "compaction"
	if g.contexts == nil || !conversationRequest(r, request) || !nativeCompact && !bridge.IsCompaction(request) {
		return override, false, ""
	}
	session, agent := r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id")
	if session == "" {
		return nil, false, "CONTEXT_IDENTITY_UNVERIFIED"
	}
	c := g.contexts
	c.mu.Lock()
	defer c.mu.Unlock()
	if g.delegations != nil && c.sessions[session] == "" {
		return nil, false, "CONTEXT_SESSION_UNVERIFIED"
	}
	_, _, valid := g.compactReceipt(session, agent, request)
	if !nativeCompact && !valid {
		return nil, false, "CONTEXT_COMPACTION_UNVERIFIED"
	}
	s := c.states[contextKey(session, agent)]
	if s == nil {
		s = &contextState{}
		if err := g.restoreContext(session, agent, s); errors.Is(err, bridge.ErrRetiredRoute) {
			return nil, false, "MODEL_RETIRED"
		} else if err != nil {
			return nil, false, "CONTEXT_JOURNAL_UNVERIFIED"
		}
	}
	route, err := g.compactionRoute(request, override)
	if err != nil {
		return nil, false, routeCategory(err)
	}
	stripCompactReceipts(request)
	return []bridge.Route{route}, true, ""
}
