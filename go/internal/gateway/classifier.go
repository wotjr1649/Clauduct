package gateway

import (
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

var errClassifierContract = errors.New("AUTO_MODE_CLASSIFIER_UNVERIFIED")
var errClassifierModel = errors.New("AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED")

//go:embed classifier-models.json
var classifierModelDocument string

// Classifier support scope, not a fallback or a user preference. Each model's
// existing effort catalogue and the configured auxiliary cap still apply.
var classifierModels = func() []string {
	var models []string
	if json.Unmarshal([]byte(classifierModelDocument), &models) != nil || len(models) == 0 {
		panic("invalid embedded classifier models")
	}
	for i, model := range models {
		route, err := bridge.SelectRoute(model, "")
		if err != nil || route.Model != model || slices.Contains(models[:i], model) {
			panic("invalid embedded classifier model")
		}
	}
	return models
}()

const classifierPolicyPrefix = "You are a security monitor for autonomous AI coding agents.\n"

// ConfigureAuxiliaryEffortCap installs the validated global limit before native starts.
func (g *Gateway) ConfigureAuxiliaryEffortCap(cap string) { g.auxiliaryEffortCap = cap }

func (g *Gateway) auxiliarySelection(request *anthropic.Request, count bool) ([]bridge.Route, error) {
	routes, err := g.classifierSelection(request, count)
	if err != nil {
		return nil, err
	}
	classifier := len(routes) != 0
	if len(routes) == 0 {
		route, err := g.selection.SelectRoute(request.Model, request.Effort)
		if err != nil {
			return nil, nil
		} // Preserve BuildRequest's explicit route refusal.
		routes = []bridge.Route{route}
	}
	if slices.Index(bridge.Efforts, routes[0].Effort) > slices.Index(bridge.Efforts, g.auxiliaryEffortCap) {
		routes[0].Effort = g.auxiliaryEffortCap
		routes[0].Source += "+auxiliary-cap"
	}
	if classifier && !slices.Contains(classifierModels, routes[0].Model) {
		return nil, errClassifierModel
	}
	return routes, nil
}

// classifierSelection is called only for independent, tool-less root auxiliary requests.
// Native 2.1.283 uses its Sonnet model without an effort or a dedicated classifier beta.
// Its measured block protocol uses the session's model mapping and effort defaults.
// Identification changes routing, never permission, policy text, or the classifier verdict.
func (g *Gateway) classifierSelection(request *anthropic.Request, count bool) ([]bridge.Route, error) {
	// A changed policy heading must not quietly send the known block envelope back
	// to Sonnet. These independent markers also catch that form of native drift.
	envelope := slices.Contains(request.StopSequences, "</block>")
	if len(request.Messages) == 1 {
		blocks := request.Messages[0].Blocks
		envelope = envelope || len(blocks) >= 4 && blocks[0].Text == "<transcript>\n" &&
			blocks[len(blocks)-2].Text == "</transcript>\n" && strings.Contains(blocks[len(blocks)-1].Text, "<block>")
	}
	var system []struct{ Text string }
	if json.Unmarshal(request.System, &system) != nil {
		var text string
		if envelope || json.Unmarshal(request.System, &text) == nil && strings.HasPrefix(text, classifierPolicyPrefix) {
			return nil, errClassifierContract
		}
		return nil, nil
	}
	found := false
	for _, block := range system {
		found = found || strings.HasPrefix(block.Text, classifierPolicyPrefix)
	}
	if !found {
		if envelope {
			return nil, errClassifierContract
		}
		return nil, nil
	}
	if len(system) < 2 || !strings.HasPrefix(system[0].Text, "x-anthropic-billing-header: ") ||
		!strings.HasPrefix(system[1].Text, classifierPolicyPrefix) || len(request.Messages) != 1 ||
		request.Messages[0].Role != "user" || request.OutputFormat != nil || len(request.Fields["thinking"]) != 0 {
		return nil, errClassifierContract
	}
	blocks := request.Messages[0].Blocks
	if len(blocks) < 4 || blocks[0].Text != "<transcript>\n" || blocks[len(blocks)-2].Text != "</transcript>\n" {
		return nil, errClassifierContract
	}
	for _, block := range blocks {
		if block.Type != "text" {
			return nil, errClassifierContract
		}
	}
	// Transcript cache chunks and native meta lines vary; only the protocol envelope
	// is inspected. Unknown envelopes fail before dispatch instead of using Sonnet's default.
	suffix := blocks[len(blocks)-1].Text
	first := strings.HasPrefix(suffix, "\nErr on the side of blocking. Stage 1 does NOT apply user intent or ALLOW exceptions") && strings.Contains(suffix, "<block>")
	second := strings.HasPrefix(suffix, "\nReview the classification process and follow it carefully,") && strings.Contains(suffix, "before responding with <block>")
	if !first && !second || !count && (!request.NonStreaming ||
		first && (request.MaxTokens != 2112 || len(request.StopSequences) != 1 || request.StopSequences[0] != "</block>") ||
		second && (request.MaxTokens != 10240 || len(request.StopSequences) != 0)) {
		return nil, errClassifierContract
	}
	route, err := g.selection.SelectRoute(request.Model, request.Effort)
	if err != nil {
		// BuildRequest rejects the unchanged request with the generation/count
		// route error it used before. Do not substitute a valid fallback route.
		return nil, nil
	}
	route.Source = "native-auto-mode"
	return []bridge.Route{route}, nil
}
