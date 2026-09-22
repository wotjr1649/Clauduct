package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The native client and product hook are real. The relay faults only the first
// registration (or all registrations); the gateway remains reachable throughout.
func TestNativeSessionRegistrationRecoversBeforePrompt(t *testing.T) {
	native := nativeAvailable(t)
	buildHook(t)
	for _, permanent := range []bool{false, true} {
		name := "first_post_lost"
		if permanent {
			name = "persistent_failure"
		}
		t.Run(name, func(t *testing.T) {
			config := t.TempDir()
			_, cwd := workspace(t)
			fixture := &upstream.Fixture{SSE: measuredStream("PUBLIC_REGISTRATION_OK")}
			g, err := gateway.Start(fixture)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := g.Close(ctx); err != nil {
					t.Error(err)
				}
			}()
			g.EnableContextPolicy()
			g.ConfigureDelegations(filepath.Join(config, "projects"))
			var registrations atomic.Int64
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/clauduct/context" {
					raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
					if err != nil {
						w.WriteHeader(400)
						return
					}
					var event struct{ Event string }
					if json.Unmarshal(raw, &event) != nil {
						w.WriteHeader(400)
						return
					}
					if event.Event == "SessionStart" && (registrations.Add(1) == 1 || permanent) {
						w.WriteHeader(503)
						return
					}
					r.Body = io.NopCloser(bytes.NewReader(raw))
				}
				request := r.Clone(r.Context())
				request.RequestURI = ""
				request.URL.Scheme, request.URL.Host, request.Host = "http", g.Addr(), g.Addr()
				response, err := client.Do(request)
				if err != nil {
					w.WriteHeader(502)
					return
				}
				defer response.Body.Close()
				for key, values := range response.Header {
					w.Header()[key] = values
				}
				w.WriteHeader(response.StatusCode)
				io.Copy(w, response.Body)
			}))
			defer relay.Close()
			env := map[string]string{}
			for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATH", "PATHEXT"} {
				env[key] = os.Getenv(key)
			}
			for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "CLAUDE_CONFIG_DIR"} {
				env[key] = config
			}
			for _, key := range []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "DISABLE_AUTOUPDATER", "DISABLE_TELEMETRY", "DISABLE_ERROR_REPORTING"} {
				env[key] = "1"
			}
			settings, ok := sessionSettings(hookPath(t))
			if !ok {
				t.Fatal("session settings unavailable")
			}
			spec := launch.Build(native, []string{"--strict-mcp-config", "--model", "gpt-5.6-luna", "--effort", "low", "-p", "public registration proof"}, env, cwd, launch.Overlay{BaseURL: relay.URL, AuthToken: g.Token(), Settings: settings, Session: sessionEnvironment(), Enforced: sessionRequirements()})
			ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, spec.File, spec.Args...)
			cmd.Env, cmd.Dir, cmd.Stdin = spec.Env, spec.Dir, strings.NewReader("")
			cmd.WaitDelay = time.Second
			output, runErr := cmd.CombinedOutput()
			if registrations.Load() < 2 {
				t.Fatalf("registration was not retried: posts=%d refusals=%v", registrations.Load(), g.Snapshot().Requests.RefusedBy)
			}
			if permanent {
				if fixture.Calls() != 0 || !strings.Contains(string(output), "CLAUDUCT_CONTEXT_SESSION_UNVERIFIED") {
					t.Fatalf("prompt was not visibly blocked: calls=%d output=%s", fixture.Calls(), tail(string(output), 800))
				}
				return
			}
			if runErr != nil || !strings.Contains(string(output), "PUBLIC_REGISTRATION_OK") || fixture.Calls() != 1 {
				t.Fatalf("recovery failed: err=%v calls=%d output=%s", runErr, fixture.Calls(), tail(string(output), 800))
			}
			report := g.Snapshot()
			if len(report.AgentContexts) != 1 || !report.AgentContexts[0].Persistent || report.AgentContexts[0].PersistenceFailed {
				t.Fatal("registration recovery lost durable context")
			}
		})
	}
}
