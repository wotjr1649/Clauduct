//go:build policy_evidence && windows

package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func TestPolicyEvidenceWebSocketWarmupCount(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	model := "gpt-5.6-sol"
	ledger := NewLedger(Budget{Model: model, Effort: "low", Limit: 1})
	if err := ledger.Reserve(Attempt{Model: model, Effort: "low", Source: "websocket-warmup-evidence"}); err != nil {
		t.Fatal("budget refused")
	}
	provider := &auth.Provider{}
	credential, err := provider.Credential()
	if err != nil || credential.Synthetic {
		t.Fatal("credential unavailable")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	request, err := bridge.BuildRequest(&anthropic.Request{Model: model, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply OK."}}}}})
	if err != nil {
		t.Fatal("build failed")
	}
	if count := warmupCount(t, request, credential, version); count != 21 {
		t.Fatalf("warmup count=%d want=21", count)
	}
}

func TestPolicyEvidenceGoBackendCount(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	ledger := NewLedger(Budget{Model: "gpt-5.6-sol", Effort: "low", Limit: 1})
	request, err := bridge.BuildRequest(&anthropic.Request{Model: "gpt-5.6-sol", Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply OK."}}}}})
	if err != nil {
		t.Fatal("build failed")
	}
	raw, _ := json.Marshal(request)
	start := time.Now()
	count, err := NewDirect(provider, ledger, Fixed(version)).Count(context.Background(), Call{Model: request.Model, Effort: "low", Body: raw})
	if err != nil {
		t.Fatalf("count failed category=%v", err)
	}
	t.Logf("backend_count=%d elapsed_ms=%d", count, time.Since(start).Milliseconds())
	if count != 21 {
		t.Fatal("count mismatch")
	}
}

// Four count-only requests at the selected boundaries. No large inference is
// generated. This checks the counter at scale, not native compaction quality.
func TestPolicyEvidenceBackendCountAtSelectedBoundaries(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range bridge.Models[:2] {
		for _, offset := range []int64{-1, 1} {
			label := "below"
			if offset > 0 {
				label = "above"
			}
			t.Run(model.Key+"/"+label, func(t *testing.T) {
				want := model.Context.CompactAt + offset
				text := strings.Repeat("public sample ", int((want-21)/2)) + "Reply OK."
				request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: text}}}}})
				if err != nil {
					t.Fatal("build failed")
				}
				local, err := bridge.CountInput(request)
				if err != nil || local != want {
					t.Fatalf("boundary construction local=%d target=%d err=%v", local, want, err)
				}
				raw, _ := json.Marshal(request)
				direct := NewDirect(provider, NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 1}), Fixed(version))
				start := time.Now()
				actual, err := direct.Count(context.Background(), Call{Model: model.ID, Effort: "low", Body: raw})
				if err != nil {
					t.Fatalf("count failed category=%v", err)
				}
				t.Logf("local=%d backend=%d bytes=%d elapsed_ms=%d generated=false", local, actual, len(raw), time.Since(start).Milliseconds())
				if actual != want {
					t.Fatal("counter mismatch at selected boundary")
				}
			})
		}
	}
}

func TestPolicyEvidenceEncryptedReasoningCount(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			t.Parallel()
			ledger := NewLedger(Budget{Model: model.ID, Effort: "high", Limit: 3})
			direct := NewDirect(provider, ledger, Fixed(version))
			request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "high", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Public synthetic arithmetic test: find the smallest positive integer n for which n modulo 7 is 3, n modulo 11 is 5, and n modulo 13 is 7. Do not use tools. Reply with the resulting integer only."}}}}})
			if err != nil {
				t.Fatal("build failed")
			}
			raw, _ := json.Marshal(request)
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			generate := func(body []byte) (int64, []json.RawMessage) {
				t.Helper()
				response, err := direct.Execute(ctx, Call{Model: model.ID, Effort: "high", Body: body, Source: "encrypted-count-evidence"})
				if err != nil {
					t.Fatal("generation refused")
				}
				defer response.Body.Close()
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				if err != nil {
					t.Fatal("generation read failed")
				}
				var items []json.RawMessage
				for _, line := range bytes.Split(data, []byte("\n")) {
					if !bytes.HasPrefix(line, []byte("data: ")) {
						continue
					}
					var event struct {
						Type     string          `json:"type"`
						Item     json.RawMessage `json:"item"`
						Response struct {
							Output []json.RawMessage `json:"output"`
							Usage  struct {
								Input *int64 `json:"input_tokens"`
							} `json:"usage"`
						} `json:"response"`
					}
					if json.Unmarshal(line[6:], &event) != nil {
						continue
					}
					if event.Type == "response.output_item.done" {
						items = append(items, event.Item)
					}
					if event.Type == "response.completed" && event.Response.Usage.Input != nil {
						if len(items) == 0 {
							items = event.Response.Output
						}
						return *event.Response.Usage.Input, items
					}
				}
				t.Fatal("completed usage absent")
				return 0, nil
			}
			_, output := generate(raw)
			found := false
			var answer []bridge.InputPart
			for _, item := range output {
				var reasoning struct {
					Type      string                 `json:"type"`
					ID        string                 `json:"id"`
					Encrypted string                 `json:"encrypted_content"`
					Summary   []bridge.ReasoningPart `json:"summary"`
					Content   []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}
				if json.Unmarshal(item, &reasoning) == nil && reasoning.Type == "reasoning" && reasoning.Encrypted != "" {
					request.Input = append(request.Input, bridge.InputEntry{Reasoning: &bridge.Reasoning{ID: reasoning.ID, Encrypted: reasoning.Encrypted, Summary: reasoning.Summary}})
					found = true
				}
				if reasoning.Type == "message" {
					for _, part := range reasoning.Content {
						if part.Type == "output_text" {
							answer = append(answer, bridge.InputPart{Type: "output_text", Text: part.Text})
						}
					}
				}
			}
			if !found {
				t.Fatal("backend did not provide encrypted reasoning; case remains unverified")
			}
			if len(answer) == 0 {
				t.Fatal("public synthetic answer absent")
			}
			request.Input = append(request.Input, bridge.InputEntry{Role: "assistant", Content: answer}, bridge.InputEntry{Role: "user", Content: []bridge.InputPart{{Type: "input_text", Text: "Reply OK."}}})
			raw, _ = json.Marshal(request)
			start := time.Now()
			count, err := direct.Count(ctx, Call{Model: model.ID, Effort: "high", Body: raw})
			if err != nil {
				t.Fatalf("count failed category=%v", err)
			}
			elapsed := time.Since(start).Milliseconds()
			actual, _ := generate(raw)
			t.Logf("warmup_input=%d generation_input=%d count_ms=%d", count, actual, elapsed)
			if count != actual {
				t.Fatal("reasoning count mismatch")
			}
		})
	}
}

func warmupCount(t *testing.T, request *bridge.Request, credential auth.Credential, version string) int64 {
	t.Helper()
	raw, _ := json.Marshal(request)
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(raw, &payload)
	delete(payload, "stream")
	payload["type"] = json.RawMessage(`"response.create"`)
	payload["generate"] = json.RawMessage(`false`)
	headers, _ := http.NewRequest(http.MethodGet, Endpoint, nil)
	applyHeaders(headers, credential, version, 0)
	input, _ := json.Marshal(struct {
		Headers http.Header `json:"headers"`
		Payload any         `json:"payload"`
	}{headers.Header, payload})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	script, _ := filepath.Abs("../../../verification/policy-evidence-20260918/websocket-warmup.ps1")
	cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-File", script)
	cmd.Stdin = bytes.NewReader(input)
	output, runErr := cmd.Output() // stderr may contain diagnostics: deliberately never print it.
	var result struct {
		Stage     string `json:"stage"`
		Completed bool   `json:"completed"`
		Input     *int64 `json:"input_tokens"`
		Output    *int64 `json:"output_tokens"`
		Items     int    `json:"output_items"`
	}
	if json.Unmarshal(bytes.TrimSpace(output), &result) != nil {
		t.Fatal("warmup probe failed without a usable count")
	}
	if runErr != nil {
		t.Fatalf("warmup failed stage=%s", result.Stage)
	}
	if result.Input == nil || result.Output == nil {
		t.Fatalf("completed=%v usage_absent=true output_items=%d", result.Completed, result.Items)
	}
	t.Logf("completed=%v input_tokens=%d output_tokens=%d output_items=%d", result.Completed, *result.Input, *result.Output, result.Items)
	if !result.Completed || *result.Input < 1 || *result.Output != 0 || result.Items != 0 {
		t.Fatal("warmup returned an unusable count or generated output")
	}
	return *result.Input
}

// Paired observations: identical public synthetic payload, first without model
// generation and then through the actual HTTP generation path. No tool executes.
func TestPolicyEvidenceWebSocketCountFormats(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	provider := &auth.Provider{}
	credential, err := provider.Credential()
	if err != nil || credential.Synthetic {
		t.Fatal("credential unavailable")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	var pngData bytes.Buffer
	if png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 64, 64))) != nil {
		t.Fatal("synthetic image failed")
	}
	for _, model := range bridge.Models {
		for _, name := range []string{"tools", "tool_result", "image", "schema", "pdf"} {
			t.Run(model.Key+"/"+name, func(t *testing.T) {
				t.Parallel()
				request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Reply OK. If JSON is required, return {\"answer\":\"OK\"}. Do not call tools."}}}}})
				if err != nil {
					t.Fatal("build failed")
				}
				switch name {
				case "tools", "tool_result":
					request.Tools = []bridge.ToolSpec{{Type: "function", Name: "public_sample", Description: "A synthetic tool that is never executed.", Parameters: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)}}
					request.ToolChoice = "none"
					if name == "tool_result" {
						request.Input = append(request.Input,
							bridge.InputEntry{Type: "function_call", CallID: "call_public_count", Name: "public_sample", Arguments: `{"value":"sample"}`},
							bridge.InputEntry{Type: "function_call_output", CallID: "call_public_count", Output: []bridge.InputPart{{Type: "input_text", Text: "public synthetic result"}}},
							bridge.InputEntry{Role: "user", Content: []bridge.InputPart{{Type: "input_text", Text: "Reply OK."}}})
					}
				case "image":
					request.Input[0].Content = append(request.Input[0].Content.([]bridge.InputPart), bridge.InputPart{Type: "input_image", ImageURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes())})
				case "pdf":
					// Same public single-page fixture already used by document_test.go.
					const pdf = "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUl0gL0NvdW50IDEgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA2MTIgNzkyXSAvUmVzb3VyY2VzIDw8IC9Gb250IDw8IC9GMSA0IDAgUiA+PiA+PiAvQ29udGVudHMgNSAwIFIgPj4KZW5kb2JqCjQgMCBvYmoKPDwgL1R5cGUgL0ZvbnQgL1N1YnR5cGUgL1R5cGUxIC9CYXNlRm9udCAvSGVsdmV0aWNhID4+CmVuZG9iago1IDAgb2JqCjw8IC9MZW5ndGggNDggPj4Kc3RyZWFtCkJUIC9GMSAyNCBUZiA3MiA3MDAgVGQgKENMQVVEVUNULVBERi03UTRNKSBUaiBFVAplbmRzdHJlYW0KZW5kb2JqCnhyZWYKMCA2CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwOSAwMDAwMCBuIAowMDAwMDAwMDU4IDAwMDAwIG4gCjAwMDAwMDAxMTUgMDAwMDAgbiAKMDAwMDAwMDI0MSAwMDAwMCBuIAowMDAwMDAwMzExIDAwMDAwIG4gCnRyYWlsZXIKPDwgL1NpemUgNiAvUm9vdCAxIDAgUiA+PgpzdGFydHhyZWYKNDA5CiUlRU9GCg=="
					request.Input[0].Content = append(request.Input[0].Content.([]bridge.InputPart), bridge.InputPart{Type: "input_file", Filename: bridge.DocumentFilename, FileData: "data:application/pdf;base64," + pdf})
				case "schema":
					request.Text = &bridge.TextParam{Format: bridge.SchemaFormat{Type: "json_schema", Name: "public_reply", Strict: true, Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`)}}
				}
				ledger := NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 2})
				if ledger.Reserve(Attempt{Model: model.ID, Effort: "low", Source: "warmup-format-evidence"}) != nil {
					t.Fatal("budget refused")
				}
				count := warmupCount(t, request, credential, version)
				raw, _ := json.Marshal(request)
				ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
				defer cancel()
				response, err := NewDirect(provider, ledger, Fixed(version)).Execute(ctx, Call{Requested: model.ID, Model: model.ID, Effort: "low", Source: "warmup-format-generation", Body: raw})
				if err != nil {
					t.Fatal("generation failed")
				}
				defer response.Body.Close()
				data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				if err != nil {
					t.Fatal("read failed")
				}
				for _, line := range bytes.Split(data, []byte("\n")) {
					if !bytes.HasPrefix(line, []byte("data: ")) {
						continue
					}
					var event struct {
						Type     string `json:"type"`
						Response struct {
							Usage struct {
								Input *int64 `json:"input_tokens"`
							} `json:"usage"`
						} `json:"response"`
					}
					if json.Unmarshal(line[6:], &event) == nil && event.Type == "response.completed" && event.Response.Usage.Input != nil {
						t.Logf("warmup_input=%d generation_input=%d", count, *event.Response.Usage.Input)
						if count != *event.Response.Usage.Input {
							t.Error("exact count mismatch")
						}
						return
					}
				}
				t.Fatal("generation usage absent")
			})
		}
	}
}
