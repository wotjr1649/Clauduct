//go:build runtime_evidence

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// One fixed public HTTP request per model; no private inputs, response logging,
// alternate endpoint, retry, API key setup or profile modification.
func TestRuntimeEvidenceNonStreaming(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	if err := provider.CheckRuntime(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	if err := provider.CheckHome(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	for i, model := range bridge.Models {
		func() {
			direct := upstream.NewDirect(provider, upstream.NewLedger(upstream.Budget{Model: model.ID, Effort: "low", Limit: 1}), upstream.Fixed(version))
			defer direct.Close()
			g := startWith(t, direct)
			g.EnableContextPolicy()
			body := map[string]any{"model": model.ID, "max_tokens": 1024, "output_config": map[string]string{"effort": "low"}, "messages": []any{map[string]string{"role": "user", "content": "Public compatibility test. Reply only PUBLIC_NONSTREAM_OK."}}}
			if i%2 == 0 {
				body["stream"] = false
			}
			raw, _ := json.Marshal(body)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL()+"/v1/messages", strings.NewReader(string(raw)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Anthropic-Version", anthropicVersion)
			req.Header.Set("X-Claude-Code-Request-Class", "main")
			req.Header.Set("X-Claude-Code-Session-Id", "public_nonstream_evidence")
			req.Header.Set("Authorization", "Bearer "+g.Token())
			client := &http.Client{Transport: &http.Transport{Proxy: nil}}
			defer client.CloseIdleConnections()
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal("public JSON request failed")
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			var got struct {
				Model      string
				Content    []struct{ Type, Text string }
				Usage      map[string]int64
				StopReason string `json:"stop_reason"`
			}
			if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" || json.Unmarshal(data, &got) != nil || got.Model != model.ID || got.Usage["input_tokens"] <= 0 || got.Usage["output_tokens"] <= 0 || got.StopReason != "end_turn" {
				for _, record := range g.Diagnose().RecentFailures {
					t.Logf("stage=%s category=%s status=%d", record.Stage, record.Category, record.Status)
				}
				t.Fatal("JSON response contract failed")
			}
			answer := ""
			for _, block := range got.Content {
				if block.Type == "text" {
					answer += block.Text
				}
			}
			if strings.TrimSpace(answer) != "PUBLIC_NONSTREAM_OK" {
				t.Fatal("public answer mismatch")
			}
			t.Logf("model=%s status=200 json=true marker=true input=%d output=%d streamOmitted=%v", model.ID, got.Usage["input_tokens"], got.Usage["output_tokens"], i%2 != 0)
		}()
	}
}
