package gateway

import (
	"errors"
	"net/http"
	"slices"
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

func compactRoute(route bridge.Route, automatic bool) bridge.Route {
	if automatic && slices.Index(bridge.Efforts, route.Effort) > slices.Index(bridge.Efforts, "medium") {
		route.Effort, route.Source = "medium", route.Source+"+auto-compact"
	}
	return route // A copy: the session's route and subsequent generation stay intact.
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
	_, receipt, valid := g.compactReceipt(session, agent, request)
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
	if s.route.Model != "" {
		override = []bridge.Route{s.route}
	}
	route, err := bridge.ResolveRoute(request, override...)
	if err != nil {
		return nil, false, routeCategory(err)
	}
	stripCompactReceipts(request)
	return []bridge.Route{compactRoute(route, valid && receipt.trigger == "auto")}, true, ""
}
