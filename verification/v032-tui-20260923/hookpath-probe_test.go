//go:build runtime_evidence

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Temporary v0.3.2 probe: which request first carries the hook path, and does the
// live evidence guard refuse it? Local synthetic transport only.
type recordingTUI struct {
	*tuiLocalTransport
	mu     sync.Mutex
	bodies []string
}

func (r *recordingTUI) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	r.mu.Lock()
	r.bodies = append(r.bodies, string(call.Body))
	r.mu.Unlock()
	return r.tuiLocalTransport.Execute(ctx, call)
}

func TestV032HookPathAfterCompaction(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)
	env, cwd := integrationWorkspace(t)
	ledger := upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 8})
	f := &recordingTUI{tuiLocalTransport: &tuiLocalTransport{ledger}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultNativeTimeout)
	defer cancel()
	in, input := io.Pipe()
	output, stdout := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { input.CloseWithError(ctx.Err()); output.CloseWithError(ctx.Err()) })
	defer stop()
	go func() {
		_, _ = Run(ctx, Options{Args: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--model", "gpt-5.6-luna", "--effort", "low", "--tools", "", "--strict-mcp-config", "--setting-sources", ""}, Env: env, Cwd: cwd, Stdin: in, Stdout: stdout, Stderr: &stderr, Ledger: ledger,
			ResolveClaude: func() (string, bool, error) { return exe, true, nil },
			StartGateway: func() (*gateway.Gateway, error) {
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
	ask := func(text string) string {
		if err := json.NewEncoder(input).Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}); err != nil {
			t.Fatal(err)
		}
		for scanner.Scan() {
			var e struct{ Type, Result string }
			if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Type == "result" {
				return e.Result
			}
		}
		t.Fatalf("native ended: %s", tail(stderr.String(), 1500))
		return ""
	}
	for _, text := range []string{
		"Remember these public facts: code PUBLIC_TUI_47, color blue, release number 47. Use no tools. Reply exactly PUBLIC_TUI_47.",
		"/compact",
		"Use no tools. Reply with the remembered code, color and release number, followed by PUBLIC_COMPACT_DONE. Put only those four items on one line.",
	} {
		answer := ask(text)
		if len(answer) > 80 {
			answer = answer[:80]
		}
		t.Logf("input=%.40q result=%q", text, answer)
	}
	hookDir := strings.ToLower(strings.ReplaceAll(hookPath(t), `\`, "/"))
	hookDir = hookDir[:strings.LastIndex(hookDir, "/")]
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, body := range f.bodies {
		lower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(body, `\`, `\`), `\`, "/"))
		at := strings.Index(lower, "clauduct-hook")
		excerpt := ""
		if at >= 0 {
			start := max(0, at-140)
			excerpt = strings.ReplaceAll(body[start:min(len(body), at+60)], hookDir, "<HOOKDIR>")
		}
		t.Logf("request=%d bytes=%d compaction=%v hook_dir_present=%v private_guard=%v hook_excerpt=%q", i, len(body), strings.Contains(body, "CRITICAL: Respond with TEXT ONLY"), strings.Contains(lower, hookDir), privateIntegrationPath(upstream.Call{Body: []byte(body)}), excerpt)
	}
	t.Logf("hook path is next to the test binary; its directory is private=%v", privateIntegrationPath(upstream.Call{Body: []byte(hookDir)}))
}
