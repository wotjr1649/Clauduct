package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeEventModuleRunsWithNoNodeOnChildPATH(t *testing.T) {
	buildHook(t)
	script := newScript(
		toolStream("no_node_agent", "Agent", `{"subagent_type":"Plan","description":"native module proof","prompt":"Reply done without tools","model":"gpt-5.6-sol","effort":"high"}`),
		textStream("child", "Public child report; no unverified items."),
		textStream("parent", "Public report received."))
	out := (nativeRun{Args: []string{"-p", "Run the public child", "--allowedTools", "Agent"}, Env: map[string]string{"PATH": filepath.Join(os.Getenv("SystemRoot"), "System32")}, transport: script}).run(t)
	if out.err != nil || out.result.NativeExitCode != 0 || !out.result.Diagnostics.NativeEvents.Observed || out.result.Diagnostics.NativeEvents.Invalid != 0 {
		t.Fatalf("embedded module did not execute without Node: %v exit=%d calls=%d %+v %s", out.err, out.result.NativeExitCode, out.backendCalls, out.result.Diagnostics.NativeEvents, tail(out.output(), 2500))
	}
}

func TestNativeEventPluginPassesInstalledValidator(t *testing.T) {
	path, err := prepareNativeEvents()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, nativeAvailable(t), "plugin", "validate", path)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1", "CLAUDE_CONFIG_DIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native validation: %v %s", err, tail(string(out), 2500))
	}
	t.Logf("native plugin validator: %s", tail(string(out), 1500))
}

func TestNativeEventPublicationFailureAndCapacity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required only for the filesystem failure/capacity test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "native_events_publication_test.mjs")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native publication checks: %v\n%s", err, tail(string(output), 3000))
	}
}
