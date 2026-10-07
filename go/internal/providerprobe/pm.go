package providerprobe

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// p3BashOnce answers the turn's first main request (one that offers the Bash tool) with a
// Bash call that writes marker, and lets every later call get the default reply. results
// collects the text native returned for that call.
func p3BashOnce(marker *string, results *[]string, mu *sync.Mutex) func(body string, canaries []string) string {
	return func(body string, canaries []string) string {
		if !strings.Contains(body, `"name":"Bash"`) {
			return "" // auxiliary request: no tools
		}
		if strings.Contains(body, "p3-pm-marker") {
			// The call is already in this request's history: its result is what native returned.
			mu.Lock()
			*results = append(*results, p3ToolOutput(body))
			mu.Unlock()
			return ""
		}
		args, _ := json.Marshal(map[string]any{"command": "echo p3 > '" + filepath.ToSlash(*marker) + "'", "description": "p3 permission marker"})
		return toolStream("call_p3pm", "Bash", string(args))
	}
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
