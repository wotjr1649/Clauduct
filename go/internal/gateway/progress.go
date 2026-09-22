package gateway

import (
	"sort"
	"time"
)

// Fixed event counters identify cancellation phases without retaining payloads.
type BackendProgress struct {
	Events             int64 `json:"events"`
	TextDeltas         int64 `json:"textDeltas"`
	ToolArgumentDeltas int64 `json:"toolArgumentDeltas"`
	ReasoningDeltas    int64 `json:"reasoningDeltas"`
}

func (r *record) backendEvent(kind string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.LastObservedMs = time.Since(r.epoch).Milliseconds()
	r.data.BackendProgress.Events++
	switch kind {
	case "response.output_text.delta":
		r.data.BackendProgress.TextDeltas++
	case "response.function_call_arguments.delta":
		r.data.BackendProgress.ToolArgumentDeltas++
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		r.data.BackendProgress.ReasoningDeltas++
	}
}

type NativeProgress struct {
	Session            string `json:"session"`
	Agent              string `json:"agent"`
	Turn               string `json:"turn"`
	Phase              string `json:"phase"`
	Signal             string `json:"signal"`
	Sequence           int64  `json:"sequence"`
	At                 int64  `json:"at"`
	PendingTools       int    `json:"pendingTools"`
	PermissionRequests int    `json:"permissionRequests"`
	AgeMs              int64  `json:"ageMs"`
}

type NativeProgressReport struct {
	Source           string           `json:"source"`
	State            string           `json:"state"`
	Unreadable       int              `json:"unreadable"`
	CapacityExceeded bool             `json:"capacityExceeded"`
	Agents           []NativeProgress `json:"agents"`
}

func (g *Gateway) nativeProgressReport() NativeProgressReport {
	out := NativeProgressReport{Source: "native_event_receipts", State: "not_observed", Agents: []NativeProgress{}}
	if g.nativeEvents.directory == "" {
		return out
	}
	type target struct{ session, agent, turn string }
	targets := []target{}
	if main, found, err := g.readCurrentNativeTurn(""); err != nil {
		out.Unreadable++
	} else if found && correlationShape.MatchString(main.Session) && correlationShape.MatchString(main.Turn) && main.Agent == "" {
		targets = append(targets, target{main.Session, "", main.Turn})
	} else if found {
		out.Unreadable++
	}
	if g.delegations != nil {
		r := &g.delegations.results
		r.mu.Lock()
		for _, e := range r.entries {
			if !e.stopped && correlationShape.MatchString(e.Session) && correlationShape.MatchString(e.Agent) && correlationShape.MatchString(e.NativeTurn) {
				targets = append(targets, target{e.Session, e.Agent, e.NativeTurn})
			}
		}
		r.mu.Unlock()
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].agent < targets[j].agent })
	// ponytail: bound checkpoint I/O; report saturation instead of hiding active work.
	if len(targets) > 128 {
		targets = targets[:128]
		out.CapacityExceeded = true
	}
	for _, t := range targets {
		name := "progress-root.json"
		if t.agent != "" {
			name = "progress-child-" + t.agent + ".json"
		}
		var p NativeProgress
		found, err := g.readNativeJSON(name, []string{"session", "agent", "turn", "phase", "signal", "sequence", "at", "pendingTools", "permissionRequests"}, &p)
		if !found && err == nil {
			out.Unreadable++
			continue
		}
		if err != nil || !validProgress(p, t.session, t.agent, t.turn, time.Now().UnixMilli()) {
			out.Unreadable++
			continue
		}
		p.AgeMs = max(0, time.Now().UnixMilli()-p.At)
		out.Agents = append(out.Agents, p)
	}
	if len(out.Agents) > 0 {
		out.State = "observed_events_not_liveness_proof"
	}
	return out
}

func validProgress(p NativeProgress, session, agent, turn string, now int64) bool {
	if p.Session != session || p.Agent != agent || p.Turn != turn || p.At <= 0 || p.At > now+60000 || p.Sequence <= 0 || p.PendingTools < 0 || p.PendingTools > 1024 || p.PermissionRequests < 0 {
		return false
	}
	switch p.Phase {
	case "request", "tool_pending", "progress_unconfirmed", "turn_ended", "awaiting_children":
	default:
		return false
	}
	switch p.Signal {
	case "request_started", "request_returned", "tool_started", "tool_returned", "permission_requested", "turn_answer", "turn_aborted", "turn_error", "turn_refusal", "children_wait":
		return true
	}
	return false
}
