package app

import (
	"context"
	"encoding/json"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReleaseScopedWritePermission(t *testing.T) {
	for _, placement := range []string{"direct_settings", "settings", "cli"} {
		t.Run(placement, func(t *testing.T) {
			_, cwd := workspace(t)
			file := filepath.Join(cwd, "public-file.txt")
			input, _ := json.Marshal(map[string]string{"file_path": file, "content": "PUBLIC_FILE_53"})
			args := []string{"-p", "public scoped write proof", "--tools", "Write", "--model", "gpt-5.6-luna", "--effort", "low", "--permission-mode", "dontAsk", "--setting-sources", "", "--strict-mcp-config"}
			if placement != "cli" {
				args = append(args, "--settings", `{"permissions":{"allow":["Edit(/public-file.txt)"]}}`)
			} else {
				args = append(args, "--allowedTools", "Edit(/public-file.txt)")
			}
			outside := filepath.Join(cwd, "outside-approved-file.txt")
			wrong, _ := json.Marshal(map[string]string{"file_path": outside, "content": "PUBLIC_DENIED"})
			script := newScript(toolStream("public_write", "Write", string(input)), toolStream("denied_write", "Write", string(wrong)), textStream("public_done", "PUBLIC_DONE"))
			exitCode := 0
			if placement == "direct_settings" {
				g, err := gateway.Start(script)
				if err != nil {
					t.Fatal(err)
				}
				defer g.Close(context.Background())
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, nativeAvailable(t), args...)
				cmd.Dir = cwd
				env := isolatedEnv(t)
				env["ANTHROPIC_BASE_URL"] = g.BaseURL()
				env["ANTHROPIC_AUTH_TOKEN"] = g.Token()
				for k, v := range env {
					cmd.Env = append(cmd.Env, k+"="+v)
				}
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				if err := cmd.Run(); err != nil {
					t.Fatal("direct native failed")
				}
			} else {
				out := (nativeRun{Cwd: cwd, Args: args, transport: script}).run(t)
				exitCode = out.result.NativeExitCode
				if out.err != nil || exitCode != 0 {
					t.Fatal("native run failed")
				}
			}
			raw, err := os.ReadFile(file)
			if (err == nil) != (placement == "cli") {
				t.Fatal("unexpected scoped permission result")
			}
			if _, err := os.Stat(outside); !os.IsNotExist(err) {
				t.Fatal("permission scope widened")
			}
			t.Logf("placement=%s written=%t public_bytes=%t exit=%d outside_file_absent=true", placement, err == nil, string(raw) == "PUBLIC_FILE_53", exitCode)
		})
	}
}
