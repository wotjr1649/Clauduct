package providerprobe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3BashOnce answers the turn's first main request that offers the Bash tool with a Bash call
// that writes the marker file, and records, per run, whether Bash was offered at all and the
// text native returned for that call (admission r2 foregroundConditions PM00–PM03).
type p3BashOnce struct {
	mu      sync.Mutex
	marker  string
	markers []string // every run's marker, in run order (control, then treatment)
	offered bool
	results []string
}

// reset starts a run: the marker is in that run's workspace.
func (b *p3BashOnce) reset(marker string) {
	b.mu.Lock()
	b.marker, b.offered, b.results = marker, false, nil
	b.markers = append(b.markers, marker)
	b.mu.Unlock()
}

// executedInEveryRun reports whether the Bash call actually ran (its marker file exists) in
// each of the n runs this case made, control and treatment both.
func (b *p3BashOnce) executedInEveryRun(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.markers) != n {
		return false
	}
	for _, marker := range b.markers {
		if _, err := os.Stat(marker); err != nil {
			return false
		}
	}
	return true
}

// p3ManagedFixtureVerdict judges PM03 when its managed-file marker reached no receiver in
// control or treatment (admission r3 dispositions PM03): FIXTURE_NOT_APPLIED only when Bash also
// executed in both runs, which shows the managed deny did not apply; otherwise INCONCLUSIVE.
func p3ManagedFixtureVerdict(bash *p3BashOnce) (verdict, reason string) {
	if !bash.executedInEveryRun(2) {
		return providercert.Inconclusive, "managed file marker reached no receiver, but Bash did not execute (no marker file) in both control and treatment, so the managed deny is not shown unapplied"
	}
	return VerdictFixtureNotApplied, "managed file marker reached no receiver in control or treatment, and Bash executed in both"
}

func (b *p3BashOnce) script(body string, _ []string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !strings.Contains(body, `"name":"Bash"`) {
		return "" // auxiliary request: no tools
	}
	b.offered = true
	if strings.Contains(body, "p3-pm-marker") {
		// The call is already in this request's history: its result is what native returned.
		b.results = append(b.results, p3ToolOutput(body))
		return ""
	}
	args, _ := json.Marshal(map[string]any{"command": "echo p3 > '" + filepath.ToSlash(b.marker) + "'", "description": "p3 permission marker"})
	return toolStream("call_p3pm", "Bash", string(args))
}

// p3ToolOutput is the output native returned for call_p3pm, from the backend request body.
func p3ToolOutput(body string) string {
	var request struct {
		Input []struct {
			Type   string          `json:"type"`
			CallID string          `json:"call_id"`
			Output json.RawMessage `json:"output"`
		} `json:"input"`
	}
	if json.Unmarshal([]byte(body), &request) != nil {
		return "unparsed"
	}
	if i := strings.Index(body, "function_call_output"); i >= 0 && len(request.Input) == 0 {
		end := min(len(body), i+300)
		return body[i:end]
	}
	for _, item := range request.Input {
		if item.Type == "function_call_output" {
			text := string(item.Output)
			if len(text) > 300 {
				text = text[:300]
			}
			return text
		}
	}
	return "absent"
}

func p3WrongBytes(obs *p3Observed) int64 {
	n := int64(0)
	for _, w := range obs.Wrong {
		n += w.Bytes
	}
	return n
}

// p3PermissionDenied is a tool result native returns for a call its permission rules deny.
func p3PermissionDenied(result string) bool {
	return strings.Contains(strings.ToLower(result), "denied") || strings.Contains(strings.ToLower(result), "permission")
}

// p3ApprovalRequired is a tool result native returns in -p for a call that needs approval.
func p3ApprovalRequired(result string) bool {
	lower := strings.ToLower(result)
	return strings.Contains(lower, "requested permission") || strings.Contains(lower, "haven't granted") || strings.Contains(lower, "approval")
}
