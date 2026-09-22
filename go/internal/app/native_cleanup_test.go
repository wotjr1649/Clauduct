package app

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

func TestNativeSessionDirectoryCleanupAndFinalEvidence(t *testing.T) {
	buildHook(t)
	for _, mode := range []string{"success", "native_failure", "spawn_failure", "cancelled", "locked"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var plugin string
			var locked syscall.Handle
			// The lock intentionally makes cleanup fail. Remove only this test's
			// returned directory after releasing the real Windows handle.
			t.Cleanup(func() {
				if locked != 0 {
					if err := syscall.CloseHandle(locked); err != nil {
						t.Error(err)
					}
				}
				if plugin != "" {
					if err := os.RemoveAll(plugin); err != nil {
						t.Error(err)
					}
				}
			})
			env := map[string]string{fakeSentinel: "1", "CLAUDE_CONFIG_DIR": t.TempDir(), "SystemRoot": os.Getenv("SystemRoot")}
			if mode == "native_failure" {
				env[fakeExit] = "7"
			}
			result, err := Run(ctx, Options{
				Cwd: t.TempDir(), Env: env, Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
				ResolveClaude: func() (string, bool, error) { return os.Args[0], true, nil },
				StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(&upstream.Fixture{}) },
				StartProcess: func(spec launch.Spec, in io.Reader, out, stderr io.Writer) (Process, error) {
					if len(spec.Args) < 2 || spec.Args[0] != "--plugin-dir" {
						t.Fatal("plugin missing")
					}
					plugin = spec.Args[1]
					if _, err := os.Stat(filepath.Join(plugin, "hooks", "events.mjs")); err != nil {
						t.Fatal("plugin absent before spawn", err)
					}
					put := func(name, body string) {
						t.Helper()
						target := filepath.Join(plugin, "receipts", name)
						if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(target, []byte(body), 0600); err != nil {
							t.Fatal(err)
						}
					}
					put("active/root/1-final.json", `{"session":"public","agent":"","turn":"final"}`)
					put("active/root/1-final.ready", "")
					put("progress-root.json", `{"session":"public","agent":"","turn":"final","phase":"turn_ended","signal":"turn_answer","sequence":1,"at":1,"pendingTools":0,"permissionRequests":0}`)
					if mode == "spawn_failure" {
						return nil, errors.New("PUBLIC_SPAWN_FAILURE")
					}
					if mode == "locked" {
						path, e := syscall.UTF16PtrFromString(filepath.Join(plugin, "hooks", "events.mjs"))
						if e != nil {
							t.Fatal(e)
						}
						locked, e = syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0, 0)
						if e != nil {
							t.Fatal(e)
						}
					}
					if mode == "cancelled" {
						ping, e := exec.LookPath("ping.exe")
						if e != nil {
							t.Fatal(e)
						}
						spec.File, spec.Args = ping, []string{"-n", "120", "127.0.0.1"}
					}
					p, e := startOSProcess(spec, in, out, stderr)
					if mode == "cancelled" {
						cancel()
					}
					return p, e
				},
			})
			if mode == "spawn_failure" {
				if err == nil || result.NativeStarted {
					t.Fatal("spawn failure lost")
				}
			} else if mode == "cancelled" {
				if !errors.Is(err, context.Canceled) || result.Lifecycle == nil || !result.Lifecycle.NativeReaped {
					t.Fatal("native tree not reaped", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "locked" {
				if result.CleanupErr == nil || result.CleanupErr.Error() != "NATIVE_EVENT_CLEANUP_FAILED" {
					t.Fatal("real cleanup failure hidden", result.CleanupErr)
				}
				return
			}
			if result.CleanupErr != nil {
				t.Fatal("cleanup", result.CleanupErr)
			}
			if plugin == "" {
				t.Fatal("no owned directory")
			}
			if _, err := os.Stat(plugin); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("session directory leaked", err)
			}
			if len(result.Diagnostics.Progress.Agents) != 1 || result.Diagnostics.Progress.Agents[0].Turn != "final" {
				t.Fatal("directory removed before final diagnostics")
			}
			if mode == "native_failure" && result.NativeExitCode != 7 {
				t.Fatal("native exit lost")
			}
		})
	}
}
