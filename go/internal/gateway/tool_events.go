package gateway

import (
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
	"net/http"
	"strings"
	"sync"
)

// Native failures may occur without any further inference. Record their bounded
// identity, never the arbitrary error message, tool input or output.
type ToolFailureRecord struct {
	Session     string `json:"session"`
	Agent       string `json:"agent,omitempty"`
	Call        string `json:"call"`
	Tool        string `json:"tool"`
	Interrupted bool   `json:"interrupted"`
	Source      string `json:"source,omitempty"`
}
type ToolFailureReport struct {
	Recent           []ToolFailureRecord `json:"recent"`
	Total            int64               `json:"total"`
	Interrupted      int64               `json:"interrupted"`
	CapacityExceeded bool                `json:"capacityExceeded"`
}

// seenCall is one already-counted tool call. The sequence exists so the oldest can be
// dropped at the cap: without it the map only ever grew, and the 1025th distinct failed
// call refused every later /v1/messages request in the process. Ordinary Bash and Read
// failures land here too, so a long session reaches that number, and because the same
// conversation replays on every following request the refusal never cleared itself.
//
// Dropping the oldest can double-count a call whose hooks arrive more than 1024 distinct
// failures apart. Hooks for one call arrive together, and a miscount in a diagnostic is a
// smaller thing than a session that cannot make another request.
type seenCall struct {
	interrupted bool
	sequence    uint64
}
type toolFailures struct {
	mu       sync.Mutex
	seen     map[delegationKey]seenCall
	sequence uint64
	report   ToolFailureReport
}

func (f *toolFailures) snapshot() ToolFailureReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.report
	out.Recent = append([]ToolFailureRecord(nil), out.Recent...)
	return out
}
func (g *Gateway) handleToolFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.refuse(w, refuseMethod)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		g.refuse(w, refuseMediaType)
		return
	}
	raw, release, ok := g.readBounded(w, r, 2048)
	if !ok {
		return
	}
	defer release()
	fields, err := wire.Fields(raw, []string{"session", "agent", "call", "tool", "interrupted"})
	var event ToolFailureRecord
	validTool := false
	if err == nil && json.Unmarshal(raw, &event) == nil {
		for _, name := range []string{"Agent", "Workflow", "Read", "Write", "Edit", "Bash", "Grep", "Glob", "WebSearch", "WebFetch", "ToolSearch", "SendMessage", "TaskOutput", "TaskStop", "MCP", "other"} {
			validTool = validTool || event.Tool == name
		}
	}
	if err != nil || fields["interrupted"] == nil || !validTool || !correlationShape.MatchString(event.Session) || !correlationShape.MatchString(event.Call) || event.Agent != "" && !correlationShape.MatchString(event.Agent) {
		g.refuseCategory(w, 400, "INVALID_TOOL_FAILURE_EVENT")
		return
	}
	event.Source = "native_failure_hook"
	g.recordToolFailure(event)
	w.WriteHeader(http.StatusNoContent)
}

// No return value. Reaching the cap stopped being a refusal when the oldest counted call
// started making room, and a bool nothing can set to false leaves callers holding refusal
// branches that cannot fire -- a reader takes those for a live safeguard. CapacityExceeded
// in the report is the one signal that the cap was reached.
func (g *Gateway) recordToolFailure(event ToolFailureRecord) {
	// Controlled policy rejections already have their own counter. Keep the
	// saved original call so later requests can recover the correct history.
	if d := g.delegations; d != nil && event.Tool == "Workflow" {
		d.mu.Lock()
		rejected := d.workflowCalls[delegationKey{event.Session, event.Call}].rejected
		d.mu.Unlock()
		if rejected {
			return
		}
	}
	f := &g.toolFailures
	f.mu.Lock()
	if f.seen == nil {
		f.seen = map[delegationKey]seenCall{}
	}
	key := delegationKey{event.Session, event.Call}
	previous, seen := f.seen[key]
	interrupted := previous.interrupted
	if !seen {
		if len(f.seen) >= maxAgents {
			// Reported, because reaching the cap is still worth knowing, but no longer
			// fatal: the oldest counted call makes room for this one.
			f.report.CapacityExceeded = true
			oldest, at := delegationKey{}, ^uint64(0)
			for candidate, call := range f.seen {
				if call.sequence < at {
					oldest, at = candidate, call.sequence
				}
			}
			delete(f.seen, oldest)
		}
		f.sequence++
		f.seen[key] = seenCall{interrupted: event.Interrupted, sequence: f.sequence}
		f.report.Total++
		if event.Interrupted {
			f.report.Interrupted++
		}
		f.report.Recent = append(f.report.Recent, event)
		if len(f.report.Recent) > recentRequests {
			f.report.Recent = f.report.Recent[1:]
		}
	} else if event.Interrupted && !interrupted {
		// A later hook can prove cancellation after an error result was seen.
		// Do not count the same call twice or lose that stronger evidence.
		f.seen[key] = seenCall{interrupted: true, sequence: previous.sequence}
		f.report.Interrupted++
		for i := range f.report.Recent {
			if f.report.Recent[i].Session == event.Session && f.report.Recent[i].Call == event.Call {
				f.report.Recent[i].Interrupted = true
				f.report.Recent[i].Source = event.Source
			}
		}
	}
	f.mu.Unlock()
	if d := g.delegations; d != nil {
		d.mu.Lock()
		d.discardResume(event.Session, event.Call)
		if choice, ok := d.pending[key]; ok {
			d.selectionState(choice.receipt, "native_tool_failed")
			delete(d.pending, key)
		}
		if !d.workflowCalls[key].rejected {
			delete(d.workflowCalls, key)
		}
		d.mu.Unlock()
	}
}

// Native validation errors (for example TaskStop on a finished task) can return
// is_error without PostToolUseFailure. Match a real call/result pair; never parse
// error-looking text as a failure. SendMessage also has its success:false form.
func (g *Gateway) observeMessageFailures(req *anthropic.Request, session, agent string) {
	if !correlationShape.MatchString(session) || agent != "" && !correlationShape.MatchString(agent) {
		return
	}
	calls := map[string]string{}
	for _, m := range req.Messages {
		for _, b := range m.Blocks {
			if m.Role == "assistant" && b.Type == "tool_use" {
				calls[b.ID] = b.Name
			}
			tool := calls[b.ToolUseID]
			if m.Role != "user" || b.Type != "tool_result" || tool == "" {
				continue
			}
			if b.IsError {
				name := "other"
				for _, known := range []string{"Agent", "Workflow", "Read", "Write", "Edit", "Bash", "Grep", "Glob", "WebSearch", "WebFetch", "ToolSearch", "SendMessage", "TaskOutput", "TaskStop"} {
					if tool == known {
						name = known
					}
				}
				if strings.HasPrefix(tool, "mcp__") {
					name = "MCP"
				}
				g.recordToolFailure(ToolFailureRecord{Session: session, Agent: agent, Call: b.ToolUseID, Tool: name, Source: "tool_result"})
				continue
			}
			if tool != "SendMessage" {
				continue
			}
			for _, part := range b.Result {
				if part.Type != "text" || len(part.Text) > 64<<10 {
					continue
				}
				var result struct {
					Success *bool `json:"success"`
				}
				if json.Unmarshal([]byte(part.Text), &result) == nil && result.Success != nil && !*result.Success {
					g.recordToolFailure(ToolFailureRecord{Session: session, Agent: agent, Call: b.ToolUseID, Tool: "SendMessage", Source: "tool_result"})
				}
			}
		}
	}
}
