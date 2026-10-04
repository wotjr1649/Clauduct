package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// The backend's reasoning summary, shown on screen by the session plugin and nowhere else
// (#277). It never becomes a thinking block or a message: the plugin hands it to native's
// $.ui.log, which draws a line the model is not sent.
const (
	maxSummaryParts = 8
	maxSummaryRunes = 240
)

// summaryStep is the native step a summary would be shown in, or nil when this request
// asked for none or no step can be matched. A summary is asked for only when it is non-nil,
// since asking delays the first text. Only the main turn's own request (#286): native does
// not write a subagent's or teammate's thinking into the main transcript either, and a side
// question in the same step would replace the step's summary with its own. Only in the TUI:
// in -p and the SDK a $.ui.log line would add to output that must stay as it was.
func (g *Gateway) summaryStep(r *http.Request, request *anthropic.Request, session string) *parentStep {
	if request.ThinkingDisplay != "summarized" || g.nativeEvents.directory == "" ||
		r.Header.Get("X-Claude-Code-Request-Class") != "main" || r.Header.Get("X-Claude-Code-Agent-Id") != "" {
		return nil
	}
	step, found, err := g.readNativeStep(session, "")
	if err != nil || !found || step.Mode != "native_tui" {
		return nil
	}
	return &step
}

// writeSummary publishes the parts finished so far for the plugin. Best effort: a summary
// that cannot be shown is not a reason to fail the answer it came with.
func (g *Gateway) writeSummary(step *parentStep, parts []string) {
	shown := displaySummary(parts)
	if len(shown) == 0 {
		return
	}
	name := "root"
	if step.Agent != "" {
		name = "child-" + step.Agent
	}
	body, _ := json.Marshal(struct {
		Session string   `json:"session"`
		Agent   string   `json:"agent"`
		Turn    string   `json:"turn"`
		Index   int      `json:"index"`
		Parts   []string `json:"parts"`
	}{step.Session, step.Agent, step.Turn, step.Index, shown})
	_ = g.writeNativeControl("summary-"+name+".json", body)
}

// displaySummary makes backend text safe for one terminal line each: no control or format
// characters (escape sequences, bidi overrides, newlines), no markdown bold markers,
// whitespace collapsed, and bounded in count and length.
func displaySummary(parts []string) []string {
	out := []string{}
	for _, part := range parts {
		if len(out) == maxSummaryParts {
			break
		}
		cleaned := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
				return ' '
			}
			return r
		}, strings.ReplaceAll(part, "**", ""))
		line := []rune(strings.Join(strings.Fields(cleaned), " "))
		if len(line) == 0 {
			continue
		}
		if len(line) > maxSummaryRunes {
			line = append(line[:maxSummaryRunes-1], '…')
		}
		out = append(out, string(line))
	}
	return out
}
