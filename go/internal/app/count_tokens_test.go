package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type displayFixture struct {
	nativeCountFixture
	leaked atomic.Int64
}

func (f *displayFixture) inspect(call upstream.Call) {
	var req struct {
		Input []struct{ Content json.RawMessage }
	}
	if json.Unmarshal(call.Body, &req) != nil {
		f.leaked.Add(1)
		return
	}
	for _, m := range req.Input {
		var parts []struct{ Text string }
		if err := json.Unmarshal(m.Content, &parts); err != nil {
			var plain string
			if json.Unmarshal(m.Content, &plain) != nil {
				f.leaked.Add(1)
				continue
			}
			parts = append(parts, struct{ Text string }{plain})
		}
		for _, p := range parts {
			if strings.Contains(p.Text, "## Context Usage") || strings.Contains(p.Text, "<local-command-stdout>") || strings.Contains(p.Text, "<command-name>/context</command-name>") {
				f.leaked.Add(1)
			}
		}
	}
}
func (f *displayFixture) Count(ctx context.Context, call upstream.Call) (int64, error) {
	f.inspect(call)
	return f.nativeCountFixture.Count(ctx, call)
}
func (f *displayFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.inspect(call)
	reply := fixtureInputCount.ReplaceAllString(textStream("public", "DISPLAY_PROBE_OK"), `"input_tokens":701`)
	return (&upstream.Fixture{SSE: reply}).Execute(ctx, call)
}

func TestNativeContextHistoryIsDisplayOnly(t *testing.T) {
	buildHook(t)
	f := &displayFixture{nativeCountFixture: nativeCountFixture{Fixture: upstream.Fixture{SSE: textStream("public", "DISPLAY_PROBE_OK")}}}
	profile := t.TempDir()
	_, cwd := workspace(t)
	const session = "f6877c63-524f-4e66-a60c-86c233f73ead"
	for i, prompt := range []string{"/context all", "/context all", "Say DISPLAY_PROBE_OK without tools."} {
		args := []string{"-p", prompt, "--model", "gpt-6-astra"}
		if i == 0 {
			args = append(args, "--session-id", session)
		} else {
			args = append(args, "--resume", session)
		}
		out := (nativeRun{Args: args, ConfigDir: profile, Cwd: cwd, transport: f, ContextPolicy: true}).run(t)
		if out.err != nil || out.result.NativeExitCode != 0 {
			t.Fatalf("step=%d exit=%d err=%v output=%s", i, out.result.NativeExitCode, out.err, tail(out.output(), 700))
		}
		t.Logf("step=%d display=%+v", i, out.result.Diagnostics.ContextDisplay)
		if i > 0 && out.result.Diagnostics.ContextDisplay.RemovedBlocks == 0 {
			t.Errorf("step=%d no proven displays removed", i)
		}
	}
	if f.leaked.Load() != 0 {
		t.Fatalf("diagnostic blocks reached backend %d times", f.leaked.Load())
	}
}

type nativeCountFixture struct {
	upstream.Fixture
	counts atomic.Int64
}

func (f *nativeCountFixture) Count(context.Context, upstream.Call) (int64, error) {
	f.counts.Add(1)
	return 701, nil
}

// This verifies native /context protocol integration, not tokenizer accuracy.
// Backend-count accuracy is measured separately with paired live requests.
func TestNativeContextAllConsumesSupportedCounts(t *testing.T) {
	f := &nativeCountFixture{Fixture: upstream.Fixture{SSE: textStream("unexpected", "ok")}}
	out := (nativeRun{Args: []string{"-p", "/context all"}, transport: f}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("context failed: %v exit=%d", out.err, out.result.NativeExitCode)
	}
	succeeded := int64(0)
	for _, record := range out.result.Diagnostics.Recent {
		if record.Kind == "count_tokens" && record.Outcome == "ok" {
			succeeded++
		}
	}
	t.Logf("native_exit=%d backend_count_requests=%d successful_counts_in_recent=%d generation_requests=%d", out.result.NativeExitCode, f.counts.Load(), succeeded, f.Calls())
	if f.counts.Load() == 0 || succeeded == 0 || f.Calls() != 0 {
		t.Fatalf("native did not consume count responses: %+v", out.result.Diagnostics.Recent)
	}
}

func TestNativeContextKeepsOriginalRendererAndGatewayPolicy(t *testing.T) {
	buildHook(t)
	f := &nativeCountFixture{Fixture: upstream.Fixture{SSE: textStream("unexpected", "ok")}}
	out := (nativeRun{Args: []string{"-p", "/context all", "--model", "gpt-5.6-sol"}, transport: f, ContextPolicy: true}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 {
		t.Fatalf("context failed: err=%v exit=%d", out.err, out.result.NativeExitCode)
	}
	if !strings.Contains(strings.Join(strings.Fields(strings.ToLower(out.stdout)), ""), "/500k") || strings.Contains(out.stdout, "clauduct-native-events:") {
		t.Fatalf("native renderer replaced: %s", tail(out.stdout, 1500))
	}
	if f.Calls() != 0 || f.counts.Load() == 0 {
		t.Fatal("context must count without generating an answer")
	}
	for _, model := range out.result.Diagnostics.ModelContexts {
		if model.Application != "gateway_usage_preventive_compaction" || model.Model == "gpt-5.6-sol" && (model.Target.Window != 272000 || model.Target.CompactAt != 239000) {
			t.Fatal("production guard not enabled")
		}
	}
	t.Log("native /context retains its renderer; Sol 272000/239000 remains in gateway status")
}

func TestNativeEventPluginDoesNotInterceptCommands(t *testing.T) {
	for _, forbidden := range []string{"command.run", "command.describe", "session.usage", "CONTEXT_POLICIES"} {
		if strings.Contains(nativeEventModule, forbidden) {
			t.Fatalf("native command interception remains: %s", forbidden)
		}
	}
}
