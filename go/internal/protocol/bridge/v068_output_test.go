package bridge

import (
	"encoding/json"
	"strings"
	"testing"
)

// #252: a tool_result with no content still sends function_call_output.output, as "". omitempty
// dropped the key (HTTP 400); an empty list made the model call the tool again. An empty
// failure gets no invented failure text either (#144).
func TestEmptyToolResultKeepsItsOutput(t *testing.T) {
	for name, result := range map[string]string{
		"absent":         `{"type":"tool_result","tool_use_id":"t1"}`,
		"empty list":     `{"type":"tool_result","tool_use_id":"t1","content":[]}`,
		"absent failure": `{"type":"tool_result","tool_use_id":"t1","is_error":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := BuildRequest(decodeRequest(t, `{"model":"sonnet","max_tokens":1,"stream":true,
			  "tools":[{"name":"Read","input_schema":{"type":"object"}}],
			  "messages":[{"role":"user","content":"read it"},
			    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]},
			    {"role":"user","content":[`+result+`]}]}`))
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			body, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `{"type":"function_call_output","call_id":"t1","output":""}`) {
				t.Fatalf("output lost or changed: %s", body)
			}
		})
	}
}

// A result with content is written as before: the same keys in the same order.
func TestToolResultWithContentIsUnchanged(t *testing.T) {
	out, err := BuildRequest(decodeRequest(t, `{"model":"sonnet","max_tokens":1,"stream":true,
	  "tools":[{"name":"Read","input_schema":{"type":"object"}}],
	  "messages":[{"role":"user","content":"read it"},
	    {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"public text"}]}]}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	body, _ := json.Marshal(out)
	if !strings.Contains(string(body), `{"type":"function_call_output","call_id":"t1","output":[{"type":"input_text","text":"public text"}]}`) {
		t.Fatalf("a result with content changed shape: %s", body)
	}
}
