package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestAdGuardCandidateFixture(t *testing.T) {
	const marker = "LOCAL_FILTER_BACKEND_OK"
	buildHook(t)
	got := nativeRun{
		Args: []string{
			"-p", "Reply with exactly " + marker + ". Do not use tools.",
			"--model", "gpt-5.6-luna", "--effort", "low", "--tools", "", "--allowedTools", "",
			"--max-turns", "1", "--output-format", "json",
		},
		Reply: measuredStream(marker),
		ContextPolicy: true,
	}.run(t)
	var response struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	decodeErr := json.Unmarshal([]byte(got.stdout), &response)
	if got.err != nil || got.backendCalls == 0 || decodeErr != nil || strings.TrimSpace(response.Result) != marker || got.result.NativeExitCode != 0 || got.result.CleanupErr != nil {
		var failureCategories []string
		for _, request := range got.result.Diagnostics.RecentFailures {
			failureCategories = append(failureCategories, request.Category)
		}
		t.Fatalf("fixture failed: gateway_calls=%d received=%d refused=%d exit=%d parsed=%t exact_reply=%t stderr_present=%t failure_categories=%v",
			got.backendCalls, got.result.Diagnostics.Requests.Received, got.result.Diagnostics.Requests.Refused,
			got.result.NativeExitCode, decodeErr == nil, strings.TrimSpace(response.Result) == marker, got.stderr != "", failureCategories)
	}
	t.Logf("fixture passed: gateway_calls=%d", got.backendCalls)
}

func TestAdGuardCandidateLive(t *testing.T) {
	const marker = "LOCAL_FILTER_BACKEND_OK"
	exe := nativeAvailable(t)
	buildHook(t)
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 1})
	var stdout, stderr strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, runErr := Run(ctx, Options{
		Args: []string{
			"-p", "Reply with exactly " + marker + ". Do not use tools.",
			"--model", "gpt-5.6-luna", "--effort", "low", "--strict-mcp-config",
			"--tools", "", "--allowedTools", "", "--max-turns", "1",
			"--output-format", "json",
		},
		Env:           isolatedEnv(t),
		Cwd:           t.TempDir(),
		Stdout:        &stdout,
		Stderr:        &stderr,
		Ledger:        ledger,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
	})
	var response struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	decodeErr := json.Unmarshal([]byte(stdout.String()), &response)
	attempts, inferences, refused := ledger.Spent()
	var failureCategories []string
	for _, request := range result.Diagnostics.RecentFailures {
		failureCategories = append(failureCategories, request.Category)
	}
	t.Logf("live: attempts=%d inferences=%d refused=%d exit=%d cleanup_failed=%t parsed=%t exact_reply=%t stderr_present=%t run_error=%t native_error=%t error_subtype=%t gateway_received=%d gateway_refused=%d gateway_broken=%d failure_categories=%v",
		attempts, inferences, refused, result.NativeExitCode, result.CleanupErr != nil,
		decodeErr == nil, strings.TrimSpace(response.Result) == marker, stderr.Len() > 0,
		runErr != nil, response.IsError, response.Subtype == "error_during_execution",
		result.Diagnostics.Requests.Received, result.Diagnostics.Requests.Refused, result.Diagnostics.Requests.Broken, failureCategories)
	if runErr != nil || attempts != 1 || inferences != 1 || refused != 0 || result.NativeExitCode != 0 || result.CleanupErr != nil ||
		decodeErr != nil || response.Type != "result" || response.Subtype != "success" || response.IsError || strings.TrimSpace(response.Result) != marker {
		t.Fatal("live backend response was not fully verified")
	}
}
