package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Every inference answers one public marker; the count says which inputs reached it.
type clearFixture struct{ calls atomic.Int64 }

func (f *clearFixture) Count(context.Context, upstream.Call) (int64, error) { return 1000, nil }

func (f *clearFixture) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	f.calls.Add(1)
	return (&upstream.Fixture{SSE: countedFixtureReply(textStream("answer", "PUBLIC_CLEAR_REPLY"), call, 1000)}).Execute(ctx, call)
}

// /clear keeps the native process and this module's registration but starts a new
// session. Receipts written after it must name that session, or the gateway refuses
// every later request as an unverified selection.
func TestNativeClearSignsLaterReceiptsWithTheNewSession(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)
	f := &clearFixture{}
	_, cwd := workspace(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()
	in, input := io.Pipe()
	output, stdout := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { input.CloseWithError(ctx.Err()); output.CloseWithError(ctx.Err()) })
	defer stop()
	go func() {
		_, _ = Run(ctx, Options{Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "", "--strict-mcp-config"}, Env: isolatedEnv(t), Cwd: cwd, Stdin: in, Stdout: stdout, Stderr: &stderr, ResolveClaude: func() (string, bool, error) { return exe, true, nil }, StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(f)
			if e == nil {
				g.EnableContextPolicy()
			}
			return g, e
		}})
		stdout.Close()
		close(done)
	}()
	defer func() { cancel(); input.Close(); output.Close(); <-done }()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	// Each input, /clear included, ends with one result event naming its session.
	ask := func(text string) (session, answer string, failed bool) {
		t.Helper()
		if err := json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}); err != nil {
			t.Fatal(err)
		}
		for scanner.Scan() {
			var e struct {
				Type, Result string
				SessionID    string `json:"session_id"`
				IsError      bool   `json:"is_error"`
			}
			if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Type == "result" {
				return e.SessionID, e.Result, e.IsError
			}
		}
		t.Fatalf("native ended before answering %q: %v %s", text, scanner.Err(), tail(stderr.String(), 1500))
		return "", "", true
	}
	first, answer, failed := ask("Public prompt before clear")
	if failed || !strings.Contains(answer, "PUBLIC_CLEAR_REPLY") {
		t.Fatalf("first prompt: %q", answer)
	}
	cleared, _, failed := ask("/clear")
	if failed || cleared == "" || cleared == first {
		t.Fatal("/clear did not start a new native session")
	}
	second, answer, failed := ask("Public prompt after clear")
	if failed || second != cleared || !strings.Contains(answer, "PUBLIC_CLEAR_REPLY") || f.calls.Load() != 2 {
		t.Fatalf("prompt after /clear: %q calls=%d", answer, f.calls.Load())
	}
}

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
