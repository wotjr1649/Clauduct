//go:build policy_evidence

package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Public synthetic inputs only. One bounded inference per cell; no retries, tools
// are never executed, generated content and credentials are never printed.
func TestPolicyEvidenceLocalTokenAccounting(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	if err := provider.CheckRuntime(); err != nil {
		t.Fatalf("runtime: %s", auth.CategoryOf(err))
	}
	if err := provider.CheckHome(); err != nil {
		t.Fatalf("home: %s", auth.CategoryOf(err))
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	cases := []struct {
		name, text, system string
		multi              bool
	}{
		{"short", "Reply OK.", "", false},
		{"english", "Public synthetic token test: the quick brown fox jumps over the lazy dog. Reply OK.", "", false},
		{"korean", "공개 합성 테스트입니다. 모델별 입력 토큰 수를 확인합니다. OK만 답하세요.", "", false},
		{"unicode", "Public sample: café naïve 東京 😀. Reply OK.", "", false},
		{"code", "Public code sample: func add(a, b int) int { return a + b }\nReply OK.", "", false},
		{"system", "Reply OK.", "This is a public synthetic tokenizer test. Answer briefly.", false},
		{"multi", "Reply OK.", "", true},
		{"edge_space", "  Reply OK.  ", "", false},
		{"edge_lines", "\n\nReply\tOK.\r\n", "", false},
		{"edge_contractions", "We're testing; don't explain. Reply OK.", "", false},
		{"edge_combining", "Public sample: e\u0301 👨‍👩‍👧‍👦. Reply OK.", "", false},
		{"edge_empty", "", "Reply OK.", false},
		{"edge_repeat", strings.Repeat("public sample ", 512) + "Reply OK.", "", false},
		{"edge_parts", "Public sample.", "", false},
		{"edge_systemparts", "Reply OK.", "Public system sample.", false},
		{"edge_effort", "Reply OK.", "", false},
		{"holdout_markup", "Public synthetic text: **bold**, `x = 3.14159`, https://example.com/a?q=one&n=42\n한국어와 العربية and 👩🏽‍💻. Reply OK.", "", false},
		{"holdout_conversation", "Public independent holdout. Reply OK.", "Public test system: keep code `[]{}()` and Unicode Ω intact.", true},
	}
	for _, model := range bridge.Models {
		for _, c := range cases {
			t.Run(model.Key+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				rq := &anthropic.Request{Model: model.ID, Effort: "low"}
				if c.name == "edge_effort" {
					rq.Effort = "max"
				}
				if c.system != "" {
					rq.System, _ = json.Marshal(c.system)
				}
				if c.multi {
					rq.Messages = append(rq.Messages, anthropic.Message{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Hello."}}}, anthropic.Message{Role: "assistant", Blocks: []anthropic.Block{{Type: "text", Text: "Hello."}}})
				}
				rq.Messages = append(rq.Messages, anthropic.Message{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: c.text}}})
				if c.name == "edge_parts" {
					rq.Messages[len(rq.Messages)-1].Blocks = append(rq.Messages[len(rq.Messages)-1].Blocks, anthropic.Block{Type: "text", Text: "Reply OK."})
				}
				if c.name == "edge_systemparts" {
					rq.System = json.RawMessage(`[{"type":"text","text":"Public system sample."},{"type":"text","text":"Answer briefly."}]`)
				}
				request, err := bridge.BuildRequest(rq)
				if err != nil {
					t.Fatal("build rejected")
				}
				predicted, err := bridge.CountInput(request)
				if err != nil {
					t.Fatal("local counter refused validation input")
				}
				raw, err := json.Marshal(request)
				if err != nil {
					t.Fatal("encode failed")
				}
				direct := NewDirect(provider, NewLedger(Budget{Model: model.ID, Effort: rq.Effort, Limit: 1}), Fixed(version))
				ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
				defer cancel()
				response, err := direct.Execute(ctx, Call{Requested: model.ID, Model: model.ID, Effort: rq.Effort, Source: "token-accounting-evidence", Body: raw})
				if err != nil {
					t.Fatalf("request refused: %v", err)
				}
				defer response.Body.Close()
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				if err != nil {
					t.Fatal("response read failed")
				}
				for _, line := range bytes.Split(data, []byte("\n")) {
					if !bytes.HasPrefix(line, []byte("data: ")) {
						continue
					}
					var event struct {
						Type     string `json:"type"`
						Response struct {
							Usage struct {
								Input  int64 `json:"input_tokens"`
								Output int64 `json:"output_tokens"`
							} `json:"usage"`
						} `json:"response"`
					}
					if json.Unmarshal(line[6:], &event) == nil && event.Type == "response.completed" {
						t.Logf("input_tokens=%d output_tokens=%d", event.Response.Usage.Input, event.Response.Usage.Output)
						if predicted != event.Response.Usage.Input {
							t.Errorf("local_prediction=%d backend=%d", predicted, event.Response.Usage.Input)
						}
						return
					}
				}
				t.Fatal("no completed usage")
			})
		}
	}
}
