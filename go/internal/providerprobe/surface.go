package providerprobe

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// p3Surface is one request's shape for the contract's surface comparison: derived fields only,
// never the body or a header value that could carry a credential or the prompt.
type p3Surface struct {
	Path            string   `json:"path"`
	Class           string   `json:"class"`
	Model           string   `json:"model"`
	Effort          string   `json:"effort"`
	Thinking        string   `json:"thinking"`
	ContextMgmt     string   `json:"contextManagement"`
	MaxTokens       int      `json:"maxTokens"`
	TopKeys         []string `json:"topKeys"`
	Tools           []string `json:"tools"`
	DeferredTools   int      `json:"deferredTools"`
	Betas           []string `json:"betas"`
	ClaudeHeaders   []string `json:"claudeHeaders"`
	AgentID         bool     `json:"agentId"`
	SystemBlocks    int      `json:"systemBlocks"`
	Messages        int      `json:"messages"`
	AttributionHead bool     `json:"attributionBlock"`
}

func p3SurfaceOf(in gateway.Ingress) p3Surface {
	s := p3Surface{Path: in.Path, Class: in.RequestClass}
	var body map[string]json.RawMessage
	json.Unmarshal(in.Body, &body)
	for k := range body {
		s.TopKeys = append(s.TopKeys, k)
	}
	slices.Sort(s.TopKeys)
	json.Unmarshal(body["model"], &s.Model)
	json.Unmarshal(body["max_tokens"], &s.MaxTokens)
	var messages []json.RawMessage
	json.Unmarshal(body["messages"], &messages)
	s.Messages = len(messages)
	var output struct {
		Effort string `json:"effort"`
	}
	json.Unmarshal(body["output_config"], &output)
	s.Effort = output.Effort
	var thinking struct {
		Type   string `json:"type"`
		Budget int    `json:"budget_tokens"`
	}
	if json.Unmarshal(body["thinking"], &thinking) == nil && thinking.Type != "" {
		s.Thinking = thinking.Type
		if thinking.Budget > 0 {
			s.Thinking += "/budget"
		}
	}
	if raw := body["context_management"]; raw != nil {
		var cm struct {
			Edits []struct {
				Type string `json:"type"`
			} `json:"edits"`
		}
		json.Unmarshal(raw, &cm)
		types := []string{}
		for _, e := range cm.Edits {
			types = append(types, e.Type)
		}
		s.ContextMgmt = "edits:" + strings.Join(types, ",")
	}
	var tools []struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Defer bool   `json:"defer_loading"`
	}
	json.Unmarshal(body["tools"], &tools)
	for _, t := range tools {
		name := t.Name
		if t.Type != "" && t.Type != "custom" {
			name += "(" + t.Type + ")"
		}
		s.Tools = append(s.Tools, name)
		if t.Defer {
			s.DeferredTools++
		}
	}
	slices.Sort(s.Tools)
	var system []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(body["system"], &system) == nil {
		s.SystemBlocks = len(system)
		s.AttributionHead = len(system) > 0 && strings.HasPrefix(system[0].Text, "x-anthropic-billing-header")
	}
	for _, v := range in.Header.Values("Anthropic-Beta") {
		for _, b := range strings.Split(v, ",") {
			if b = strings.TrimSpace(b); b != "" && !slices.Contains(s.Betas, b) {
				s.Betas = append(s.Betas, b)
			}
		}
	}
	slices.Sort(s.Betas)
	for name := range in.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-claude-code-") {
			s.ClaudeHeaders = append(s.ClaudeHeaders, strings.ToLower(name))
		}
	}
	slices.Sort(s.ClaudeHeaders)
	s.AgentID = in.Header.Get("X-Claude-Code-Agent-Id") != ""
	return s
}

// p3CompareSurfaces compares the main (first non-auxiliary /v1/messages) requests field by field.
func p3CompareSurfaces(baseline, treatment []p3Surface) map[string]string {
	main := func(list []p3Surface) *p3Surface {
		for i := range list {
			if list[i].Path == "/v1/messages" && len(list[i].Tools) > 0 || list[i].Path == "/v1/messages" && list[i].Class == "main" {
				return &list[i]
			}
		}
		for i := range list {
			if list[i].Path == "/v1/messages" {
				return &list[i]
			}
		}
		return nil
	}
	b, h := main(baseline), main(treatment)
	out := map[string]string{"requests": fmt.Sprintf("%d/%d", len(baseline), len(treatment))}
	if b == nil || h == nil {
		out["main"] = "NOT_OBSERVED"
		return out
	}
	cmp := func(name string, x, y any) {
		xa, _ := json.Marshal(x)
		ya, _ := json.Marshal(y)
		if string(xa) == string(ya) {
			out[name] = "SAME"
		} else {
			out[name] = "DIFFERENT " + string(xa) + " vs " + string(ya)
		}
	}
	cmp("S01 model", b.Model, h.Model)
	cmp("S02 effort", b.Effort, h.Effort)
	cmp("S03 thinking", b.Thinking, h.Thinking)
	cmp("S04 context_management", b.ContextMgmt, h.ContextMgmt)
	cmp("S05 betas", b.Betas, h.Betas)
	cmp("S07 tools", b.Tools, h.Tools)
	cmp("S07 deferred", b.DeferredTools, h.DeferredTools)
	cmp("S16 x-claude-code headers", b.ClaudeHeaders, h.ClaudeHeaders)
	cmp("S18 attribution block", b.AttributionHead, h.AttributionHead)
	cmp("S19 top keys", b.TopKeys, h.TopKeys)
	cmp("S23 max_tokens", b.MaxTokens, h.MaxTokens)
	last := func(list []p3Surface) int {
		n := 0
		for _, s := range list {
			if s.Path == "/v1/messages" && s.Class == "main" {
				n = s.Messages
			}
		}
		return n
	}
	cmp("S13 messages in the last main request", last(baseline), last(treatment))
	paths := func(list []p3Surface) []string {
		ps := []string{}
		for _, s := range list {
			ps = append(ps, s.Path+"|"+s.Class)
		}
		slices.Sort(ps)
		return ps
	}
	cmp("S20/S21 request paths and classes", paths(baseline), paths(treatment))
	return out
}
