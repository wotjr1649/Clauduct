//go:build runtime_evidence

package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// At most twenty fixed public requests; no caller payload, raw response logging,
// private file input, alternate destination, retry or configuration mutation.
func TestRuntimeEvidenceSamplingParameters(t *testing.T) {
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
	version, err := InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	for _, model := range bridge.Models {
		direct := NewDirect(provider, NewLedger(Budget{Model: model.ID, Effort: "low", Limit: 5}), Fixed(version))
		for _, parameter := range []struct {
			name  string
			value any
		}{{"baseline", nil}, {"temperature", 0.2}, {"top_p", 0.8}, {"max_output_tokens", 32}, {"stop", []string{"PUBLIC_STOP"}}} {
			req, err := bridge.BuildRequest(&anthropic.Request{Model: model.ID, Effort: "low", Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "Public compatibility test. Reply with only OK."}}}}})
			if err != nil {
				t.Fatal("public request build failed")
			}
			raw, _ := json.Marshal(req)
			var body map[string]json.RawMessage
			_ = json.Unmarshal(raw, &body)
			if parameter.name != "baseline" {
				body[parameter.name], _ = json.Marshal(parameter.value)
			}
			raw, _ = json.Marshal(body)
			bounded, stop := context.WithTimeout(ctx, 25*time.Second)
			response, callErr := direct.Execute(bounded, Call{Body: raw, Requested: model.ID, Model: model.ID, Effort: "low", Source: "parameter-evidence"})
			if callErr != nil {
				stop()
				var failure Failure
				if errors.As(callErr, &failure) {
					t.Logf("model=%s parameter=%s status=%d category=%s", model.ID, parameter.name, failure.Status, failure.Category)
					if parameter.name != "baseline" && failure.Status == 400 {
						continue
					}
				}
				direct.Close()
				t.Fatal("probe stopped: baseline, quota or transport not available")
			}
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			stop()
			parser := stream.NewParser(stream.DefaultLimits())
			parser.IsTerminal = codex.Terminal
			events, parseErr := parser.Push(data)
			terminal := false
			echoed := false
			for _, event := range events {
				if event.Type != codex.Completed {
					continue
				}
				terminal = true
				var envelope struct{ Response map[string]json.RawMessage }
				_ = json.Unmarshal(event.Raw, &envelope)
				if value, ok := envelope.Response[parameter.name]; ok && parameter.name != "baseline" {
					want, _ := json.Marshal(parameter.value)
					echoed = string(value) == string(want)
				}
			}
			if readErr != nil || parseErr != nil || parser.Finish(true) != nil || !terminal {
				direct.Close()
				t.Fatal("probe stopped: incomplete response")
			}
			t.Logf("model=%s parameter=%s status=200 completed=true echoed=%v semantics=not_proven_by_acceptance", model.ID, parameter.name, echoed)
		}
		direct.Close()
	}
}
