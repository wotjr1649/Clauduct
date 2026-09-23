package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Temporary v0.3.2 probe: write /plugin-types from a chosen native binary.
func TestV032PluginTypesProbe(t *testing.T) {
	exe, out := os.Getenv("V032_NATIVE"), os.Getenv("V032_TYPES_OUT")
	if exe == "" || out == "" {
		t.Skip("probe inputs absent")
	}
	_, cwd := workspace(t)
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if i := strings.IndexByte(entry, '='); i > 0 {
			env[entry[:i]] = entry[i+1:]
		}
	}
	env["CLAUDE_CONFIG_DIR"] = t.TempDir()
	env["CLAUDE_CODE_ENABLE_FUNCTION_HOOKS"] = "1"
	fixture := &upstream.Fixture{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	result, err := Run(ctx, Options{Args: []string{"-p", "/plugin-types", "--strict-mcp-config"}, Env: env, Cwd: cwd, Stdout: &stdout, Stderr: &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) }})
	if err != nil || result.NativeExitCode != 0 || fixture.Calls() != 0 {
		t.Fatalf("exit=%d calls=%d err=%v", result.NativeExitCode, fixture.Calls(), err)
	}
	raw, err := os.ReadFile(filepath.Join(cwd, ".claude", "types", "claude-code.d.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("native=%s bytes=%d sha256=%x calls=0", filepath.Base(exe), len(raw), sha256.Sum256(raw))
}
