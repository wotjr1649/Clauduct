//go:build policy_evidence && windows

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Public synthetic bytes only. At most three subscription attempts: one count
// and two tiny generations. No native prompts, paths, tools or files travel.
func TestPolicyEvidenceLivePreflightAndCache(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			ledger := upstream.NewLedger(upstream.Budget{Model: model.ID, Effort: "low", Limit: 3})
			g := startWith(t, upstream.NewDirect(&auth.Provider{}, ledger, upstream.Fixed(version)))
			g.EnableContextPolicy()
			body := `{"model":"` + model.ID + `","output_config":{"effort":"low"},"max_tokens":256,"stream":true,"messages":[{"role":"user","content":"Reply with exactly OK. Do not call tools."}],"tools":[{"name":"public_echo","description":"Echo public text","input_schema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}}]}`
			for i := 0; i < 2; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				rq, _ := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL()+"/v1/messages", strings.NewReader(body))
				rq.Header.Set("Authorization", "Bearer "+g.Token())
				rq.Header.Set("Content-Type", "application/json")
				rq.Header.Set("Anthropic-Version", anthropicVersion)
				rq.Header.Set("X-Claude-Code-Request-Class", "main")
				rq.Header.Set("X-Claude-Code-Session-Id", "public-count-proof")
				response, err := http.DefaultClient.Do(rq)
				if err != nil {
					cancel()
					t.Fatal("gateway request failed")
				}
				bodyText(t, response)
				response.Body.Close()
				cancel()
				account := status(t, g)
				var record RequestRecord
				for _, r := range account.Recent {
					if r.Path == "/v1/messages" {
						record = r
					}
				}
				if response.StatusCode != 200 || record.Outcome != "ok" || record.InputTokens == nil || record.CountedInputTokens == nil || *record.InputTokens != *record.CountedInputTokens || record.CountAgreement != "matched" {
					t.Fatalf("preflight did not match usage: status=%d category=%s", response.StatusCode, record.Category)
				}
				if i == 1 && record.CountSource != "exact-count-cache" {
					t.Fatal("identical input not reused")
				}
				metrics, _ := json.Marshal(map[string]any{"round": i + 1, "countSource": record.CountSource, "countMs": record.CountMs, "countedInputTokens": record.CountedInputTokens, "backendInputTokens": record.InputTokens, "firstByteMs": record.FirstByteMs, "startedMs": record.StartedMs})
				t.Log(string(metrics))
			}
			attempts, inferences, refused := ledger.Spent()
			if attempts != 3 || inferences != 2 || refused != 0 {
				t.Fatalf("unexpected spending: attempts=%d inference=%d refused=%d", attempts, inferences, refused)
			}
		})
	}
}

func TestPolicyEvidenceLiveOversizeStopsBeforeGeneration(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 1})
	g := startWith(t, upstream.NewDirect(&auth.Provider{}, ledger, upstream.Fixed(version)))
	g.EnableContextPolicy()
	content := strings.Repeat("apple ", 239000)
	body, _ := json.Marshal(map[string]any{"model": "gpt-5.6-luna", "output_config": map[string]string{"effort": "low"}, "max_tokens": 256, "stream": true, "messages": []any{map[string]any{"role": "user", "content": content}}, "tools": []any{map[string]any{"name": "public_echo", "input_schema": map[string]any{"type": "object", "properties": map[string]any{}}}}})
	rq := messages(strings.NewReader(string(body)))
	rq.headers["X-Claude-Code-Session-Id"] = "public-boundary-proof"
	resp := do(t, g, rq)
	result := bodyText(t, resp)
	account := status(t, g)
	var record RequestRecord
	for _, r := range account.Recent {
		if r.Path == "/v1/messages" {
			record = r
		}
	}
	attempts, inferences, refused := ledger.Spent()
	if resp.StatusCode != 400 || !strings.Contains(result, "prompt is too long") || record.CountedInputTokens == nil || *record.CountedInputTokens < 239000 || attempts != 1 || inferences != 0 || refused != 0 {
		t.Fatalf("real boundary guard failed: status=%d category=%s attempts=%d generations=%d", resp.StatusCode, record.Category, attempts, inferences)
	}
	metrics, _ := json.Marshal(map[string]any{"model": record.Model, "countedInputTokens": record.CountedInputTokens, "compactAt": 239000, "countMs": record.CountMs, "generations": inferences, "refusal": record.Category})
	t.Log(string(metrics))
}
