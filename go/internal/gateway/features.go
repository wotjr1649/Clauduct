package gateway

import (
	"slices"
	"strings"
)

// A completed request proves only the checks it actually reached. Historical
// observations never grant admission to the next request or assert TUI acceptance.
type FeatureEvidence struct {
	Name                 string   `json:"name"`
	Enforcement          string   `json:"enforcement"`
	Required             []string `json:"requiredChecks"`
	Requests             int64    `json:"requests"`
	Unconfirmed          int64    `json:"requestsWithoutRequiredChecks"`
	Observed             int64    `json:"requestsWithRequiredChecks"`
	Completed            int64    `json:"successfulResponses"`
	Failed               int64    `json:"failedAfterChecks"`
	Cancelled            int64    `json:"cancelledAfterChecks"`
	LastRequest          int64    `json:"lastRequest,omitempty"`
	LastCompletedRequest int64    `json:"lastCompletedRequest,omitempty"`
	RejectedToolCalls    int64    `json:"rejectedToolCalls,omitempty"`
	Evidence             string   `json:"runtimeEvidence"`
	Acceptance           string   `json:"acceptance"`
}

var featureRequirements = []FeatureEvidence{
	{Name: "generation", Required: []string{"input", "route", "context_policy"}},
	{Name: "native_agent", Required: []string{"input", "selection", "native_turn", "route", "context_policy"}},
	{Name: "agent_resume", Required: []string{"input", "selection", "native_turn", "continuation", "route", "context_policy"}},
	{Name: "workflow_agent", Required: []string{"input", "workflow_selection", "native_turn", "route", "context_policy"}},
	{Name: "workflow_tool_policy", Required: []string{"input", "selection", "workflow_tool_policy", "route"}},
	{Name: "count_tokens", Required: []string{"input", "route", "count_completed"}},
	{Name: "web_search", Required: []string{"input", "route", "search_available"}},
	{Name: "compaction", Required: []string{"input", "route", "compaction_authorized", "context_policy"}},
	{Name: "workflow_result_reuse", Required: []string{"input", "route", "context_policy", "workflow_recovery_request", "workflow_recovery"}},
	{Name: "parent_wait", Required: []string{"input", "route", "context_policy", "parent_input_snapshot", "native_wait_step", "native_wait_control"}},
	{Name: "empty_reply_wait", Required: []string{"input", "route", "context_policy", "parent_input_snapshot", "native_wait_step", "native_wait_control"}},
	{Name: "workflow_resume", Required: []string{"input", "route", "context_policy", "workflow_recovery_request", "workflow_plan_resume"}},
	{Name: "workflow_restore", Required: []string{"input", "route", "context_policy", "workflow_recovery_request", "workflow_checkpoint"}},
}

func (r *record) checked(name string) {
	if r == nil {
		return
	}
	known := false
	for _, feature := range featureRequirements {
		known = known || slices.Contains(feature.Required, name)
	}
	if !known {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.Contains(r.data.VerifiedChecks, name) {
		r.data.VerifiedChecks = append(r.data.VerifiedChecks, name)
	}
}

func (g *ring) countFeatures(r RequestRecord) {
	// Caller owns ring.mu; fixed catalogue, no request-supplied dictionary keys.
	if g.features == nil {
		g.features = make(map[string]FeatureEvidence)
	}
	for _, definition := range featureRequirements {
		if !featureApplies(definition.Name, r) {
			continue
		}
		e := g.features[definition.Name]
		e.Requests++
		e.LastRequest = max(e.LastRequest, r.Seq)
		e.LastCompletedRequest = r.Seq
		e.RejectedToolCalls += r.RejectedWorkflowCalls
		if !slices.ContainsFunc(definition.Required, func(check string) bool { return !slices.Contains(r.VerifiedChecks, check) }) {
			e.Observed++
			if r.Category == "CANCELLED" {
				e.Cancelled++
			} else if r.Outcome == outcomeOK {
				e.Completed++
			} else {
				e.Failed++
			}
		} else {
			e.Unconfirmed++
		}
		g.features[definition.Name] = e
	}
}

func featureApplies(name string, r RequestRecord) bool {
	switch name {
	case "generation":
		return r.Kind == "generation"
	case "native_agent":
		return r.Kind == "generation" && r.AgentID != "" && r.RequestClass != "auxiliary"
	case "workflow_agent":
		return r.Kind == "generation" && (strings.HasPrefix(r.Source, "workflow-") || slices.Contains(r.VerifiedChecks, "workflow_selection") || r.RequestClass == "workflow") && r.RequestClass != "auxiliary"
	case "workflow_tool_policy":
		return r.ToolPolicy == "workflow_step_none" || r.ToolPolicy == "workflow_allowlist"
	case "agent_resume":
		return r.Kind == "generation" && r.RequestClass != "auxiliary" && slices.Contains(r.VerifiedChecks, "continuation")
	case "workflow_result_reuse":
		return r.Kind == "generation" && slices.Contains(r.VerifiedChecks, "workflow_recovery_request")
	case "workflow_resume":
		return r.Kind == "generation" && slices.Contains(r.VerifiedChecks, "workflow_plan_resume")
	case "workflow_restore":
		return r.Kind == "generation" && slices.Contains(r.VerifiedChecks, "workflow_checkpoint")
	case "parent_wait":
		return r.Kind == "generation" && r.ParentReadiness != nil && r.ParentReadiness.Withheld
	case "empty_reply_wait":
		return r.Kind == "generation" && r.ParentReadiness != nil && r.ParentReadiness.Withheld && r.ParentReadiness.Empty
	default:
		return r.Kind == name
	}
}

func (g *ring) featureReport() []FeatureEvidence {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := make([]FeatureEvidence, 0, len(featureRequirements)+2)
	for _, definition := range featureRequirements {
		e := g.features[definition.Name]
		e.Name, e.Required = definition.Name, append([]string(nil), definition.Required...)
		e.Enforcement, e.Acceptance, e.Evidence = "per_request", "not_assessed", "not_observed"
		if e.Observed > 0 {
			e.Evidence = "required_checks_observed"
		}
		if e.Unconfirmed > 0 {
			e.Evidence = "conditions_not_confirmed_for_all_requests"
		}
		result = append(result, e)
	}
	return result
}
