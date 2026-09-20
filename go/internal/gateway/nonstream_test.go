package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNonStreamingDeliveryUsesOneExecutionAndWholeResponse(t *testing.T) {
	for _, failure := range []bool{false, true} {
		stream := sse(created, delta("PUBLIC"), done("PUBLIC"), completed, "[DONE]")
		if failure {
			stream = sse(created, delta("PUBLIC"))
		}
		fixture := &upstream.Fixture{SSE: stream}
		g := startWith(t, fixture)
		resp := post(t, g, strings.Replace(validRequest, `"stream":true`, `"stream":false`, 1))
		body := bodyText(t, resp)
		if fixture.Calls() != 1 {
			t.Fatal("generation was replayed")
		}
		if resp.Header.Get("Content-Type") != "application/json" {
			t.Fatal("not JSON", resp.Header)
		}
		if failure {
			if resp.StatusCode == 200 || strings.Contains(body, "PUBLIC") {
				t.Fatal("partial response became success", body)
			}
			continue
		}
		var got struct {
			Content    []struct{ Text string }
			Usage      map[string]int
			StopReason string `json:"stop_reason"`
		}
		if json.Unmarshal([]byte(body), &got) != nil || resp.StatusCode != 200 || len(got.Content) != 1 || got.Content[0].Text != "PUBLIC" || got.Usage["input_tokens"] != 5 || got.Usage["output_tokens"] != 2 || got.StopReason != "end_turn" {
			t.Fatal("nonstream message", body)
		}
	}
}

func TestNonStreamingSearchPreservesServerToolResults(t *testing.T) {
	fixture := &upstream.Fixture{SearchJSON: searchAnswer}
	g := startWith(t, fixture)
	resp := post(t, g, strings.Replace(sideQuery("public release"), `"stream":true`, `"stream":false`, 1))
	body := bodyText(t, resp)
	var got struct {
		Content []struct {
			Type, ID  string
			ToolUseID string `json:"tool_use_id"`
		}
	}
	if json.Unmarshal([]byte(body), &got) != nil || resp.StatusCode != 200 || len(got.Content) != 3 {
		t.Fatal("search JSON", body)
	}
	if got.Content[0].Type != "server_tool_use" || got.Content[1].ToolUseID != got.Content[0].ID || got.Content[1].Type != "web_search_tool_result" || fixture.Searches() != 1 || fixture.Calls() != 0 {
		t.Fatal("search content or execution changed")
	}
}
