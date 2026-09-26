package gateway

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

var errClassifierContract = errors.New("AUTO_MODE_CLASSIFIER_UNVERIFIED")

const classifierPolicyPrefix = "You are a security monitor for autonomous AI coding agents.\n"

// classifierSelection is called only for independent, tool-less root auxiliary requests.
// Native 2.1.283 uses its Sonnet model without an effort or a dedicated classifier beta.
// Its measured block protocol gets the separately verified route; titles keep their route.
// Identification changes routing, never permission, policy text, or the classifier verdict.
func classifierSelection(request *anthropic.Request, count bool) ([]bridge.Route, error) {
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
	// ResolveRoute still validates the requested model/effort before applying this override.
	return []bridge.Route{{Model: "gpt-6-luna", Effort: "high", Source: "native-auto-mode"}}, nil
}
