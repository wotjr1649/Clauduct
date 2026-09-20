//go:build policy_evidence && windows

package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Public synthetic data only; emitted calls are inspected, never executed.
// Four catalogue models, four requests each, no retry and no native child.
func TestPolicyEvidenceOptionalArguments(t *testing.T) {
	if os.Getenv("CLAUDUCT_OPTIONAL_EVIDENCE") != "1" {
		t.Skip("explicit bounded live switch absent")
	}
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("version unavailable")
	}
	for _, model := range bridge.Models {
		t.Run(model.Key, func(t *testing.T) {
			t.Parallel()
			direct := NewDirect(&auth.Provider{}, NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 4}), Fixed(version))
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			for round := 0; round < 2; round++ {
				for _, explicit := range []bool{false, true} {
					request, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Call Agent exactly once with description=probe, prompt=Reply probe, subagent_type=Explore. Omit model, effort and isolation entirely; these are optional and not requested. Do not execute the returned call."}}}}})
					if err != nil {
						t.Fatal("build failed")
					}
					raw, _ := json.Marshal(request)
					var body map[string]any
					if json.Unmarshal(raw, &body) != nil {
						t.Fatal("decode failed")
					}
					tool := map[string]any{"type": "function", "name": "Agent", "description": "Public synthetic argument probe. No tool is executed.", "parameters": map[string]any{"type": "object", "properties": map[string]any{
						"description": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"},
						"subagent_type": map[string]any{"type": "string"},
						"model":         map[string]any{"type": "string", "enum": []string{"sonnet", "opus", "haiku", "fable", "gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}},
						"effort":        map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "xhigh", "max"}},
						"isolation":     map[string]any{"type": "string", "enum": []string{"worktree", "remote"}},
					}, "required": []string{"description", "prompt"}, "additionalProperties": false}}
					if explicit {
						tool["strict"] = false
					}
					body["tools"] = []any{tool}
					body["tool_choice"] = map[string]string{"type": "function", "name": "Agent"}
					raw, _ = json.Marshal(body)
					response, err := direct.Execute(ctx, Call{Model: model.ID, Effort: "low", Body: raw, Source: "public-optional-probe"})
					if err != nil {
						t.Fatal("probe transport failed")
					}
					data, readErr := io.ReadAll(io.LimitReader(response.Body, 512<<10))
					response.Body.Close()
					if readErr != nil || len(data) >= 512<<10 {
						t.Fatal("probe stream failed")
					}
					completed := false
					var arguments map[string]json.RawMessage
					for _, line := range bytes.Split(data, []byte("\n")) {
						if !bytes.HasPrefix(line, []byte("data: ")) {
							continue
						}
						var event struct {
							Type     string
							Item     struct{ Type, Name, Arguments string }
							Response struct {
								Output []struct{ Type, Name, Arguments string }
							}
						}
						if json.Unmarshal(line[6:], &event) != nil {
							continue
						}
						if event.Type == "response.output_item.done" && event.Item.Type == "function_call" && event.Item.Name == "Agent" {
							if arguments != nil || json.Unmarshal([]byte(event.Item.Arguments), &arguments) != nil {
								t.Fatal("ambiguous probe item")
							}
						}
						if event.Type != "response.completed" {
							continue
						}
						completed = true
						if arguments != nil {
							continue
						}
						for _, item := range event.Response.Output {
							if item.Type == "function_call" && item.Name == "Agent" {
								if arguments != nil || json.Unmarshal([]byte(item.Arguments), &arguments) != nil {
									t.Fatal("ambiguous probe call")
								}
							}
						}
					}
					if !completed || arguments == nil {
						t.Fatal("completed probe call absent")
					}
					keys := make([]string, 0, len(arguments))
					for k := range arguments {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					_, hasModel := arguments["model"]
					_, hasEffort := arguments["effort"]
					_, hasIsolation := arguments["isolation"]
					t.Logf("round=%d explicit_non_strict=%t optional_model=%t optional_effort=%t optional_isolation=%t keys=%v", round, explicit, hasModel, hasEffort, hasIsolation, keys)
					if explicit && (hasModel || hasEffort || hasIsolation) {
						t.Fatal("optional omission violated; no retry")
					}
				}
			}
		})
	}
}
