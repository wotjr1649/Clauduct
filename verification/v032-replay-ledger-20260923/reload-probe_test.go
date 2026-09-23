package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

// Temporary v0.3.2 probe for #62: does /reload-plugins register the native events module
// again, and does a prompt after it still pass selection? Fixture backend only.
func TestV032ReloadPluginsProbe(t *testing.T) {
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
	ask := func(text string) (answer string, failed bool) {
		t.Helper()
		if err := json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}); err != nil {
			t.Fatal(err)
		}
		for scanner.Scan() {
			var e struct {
				Type, Result string
				IsError      bool `json:"is_error"`
			}
			if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Type == "result" {
				return e.Result, e.IsError
			}
		}
		t.Fatalf("native ended before answering %q: %v %s", text, scanner.Err(), tail(stderr.String(), 1500))
		return "", true
	}
	for i, text := range []string{"Public prompt one", "Public prompt two", "/reload-plugins", "Public prompt after reload"} {
		answer, failed := ask(text)
		if len(answer) > 160 {
			answer = answer[:160]
		}
		t.Logf("step=%d input=%q is_error=%v result=%q backend_calls=%d", i, text, failed, strings.ReplaceAll(answer, "\n", " "), f.calls.Load())
	}
}
