//go:build policy_evidence

package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Two fixed same-service count-route probes, no retries, no generation, no private
// payload. Existing credential/runtime/destination protections remain in place.
func TestPolicyEvidenceSubscriptionCountEndpoint(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live test switch absent")
	}
	provider := &auth.Provider{}
	credential, err := provider.Credential()
	if err != nil {
		t.Fatalf("credential guard: %s", auth.CategoryOf(err))
	}
	if credential.Synthetic {
		t.Fatal("synthetic credential refused")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	payload := []byte(`{"model":"gpt-5.6-luna","input":[{"role":"user","content":"Count this public synthetic sentence."}]}`)
	for _, suffix := range []string{"/input_tokens", "/count_tokens"} {
		t.Run(suffix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint+suffix, bytes.NewReader(payload))
			if err != nil {
				t.Fatal("request construction")
			}
			applyHeaders(req, credential, version, len(payload))
			req.Header.Set("Accept", "application/json")
			response, err := newClient().Do(req)
			if err != nil {
				t.Fatalf("transport category=%T", err)
			}
			defer response.Body.Close()
			raw, err := io.ReadAll(io.LimitReader(response.Body, 65536))
			if err != nil {
				t.Fatal("response read failed")
			}
			var value struct {
				InputTokens *int64 `json:"input_tokens"`
			}
			_ = json.Unmarshal(raw, &value)
			if value.InputTokens != nil {
				t.Logf("status=%d input_tokens=%d", response.StatusCode, *value.InputTokens)
			} else {
				t.Logf("status=%d input_tokens_absent=true", response.StatusCode)
			}
			if response.StatusCode != 200 || value.InputTokens == nil {
				t.Error("candidate endpoint did not provide a usable count; this does not exclude other undocumented endpoints")
			}
		})
	}
}

// A bounded public-text compatibility matrix: one attempt per pair, no retries.
// A successful response proves acceptance, not the provider's physical model identity.
func TestPolicyEvidenceLiveModelEffortMatrix(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live test switch absent")
	}
	provider := &auth.Provider{}
	if err := provider.CheckRuntime(); err != nil {
		t.Fatalf("runtime guard: %s", auth.CategoryOf(err))
	}
	if err := provider.CheckHome(); err != nil {
		t.Fatalf("home guard: %s", auth.CategoryOf(err))
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				t.Parallel()
				rq := &anthropic.Request{Model: model, Effort: effort, Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply with exactly OK. This is a public synthetic compatibility test."}}}}}
				built, err := bridge.BuildRequest(rq)
				if err != nil {
					t.Fatal("build rejected")
				}
				payload, err := json.Marshal(built)
				if err != nil {
					t.Fatal("encode failed")
				}
				ledger := NewLedger(Budget{Model: model, Effort: effort, Limit: 1})
				direct := NewDirect(provider, ledger, Fixed(version))
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				response, err := direct.Execute(ctx, Call{Requested: model, Model: model, Effort: effort, Source: "policy-evidence", Body: payload})
				if err != nil {
					t.Fatalf("route=%s/%s rejected category=%v", model, effort, err)
				}
				defer response.Body.Close()
				raw, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
				if err != nil {
					t.Fatal("bounded response read failed")
				}
				// Only inspect event JSON. Generated content and encrypted reasoning are not logged.
				completed := false
				var input, output int64
				for _, line := range bytes.Split(raw, []byte("\n")) {
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
						completed = true
						input = event.Response.Usage.Input
						output = event.Response.Usage.Output
					}
				}
				t.Logf("accepted=%v input_tokens=%d output_tokens=%d", completed, input, output)
				if !completed {
					t.Error("HTTP accepted but no completed response; pair remains unverified")
				}
			})
		}
	}
}
