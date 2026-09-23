//go:build runtime_evidence

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Native gets only a public temporary project/profile and delegation tools.
// The provider checks the original process environment, not the isolated child.
type integrationLiveTransport struct {
	*upstream.Direct
	t *testing.T
}

type integrationBody struct {
	io.Reader
	io.Closer
	buffer *integrationAuditBuffer
	t      *testing.T
}

type integrationAuditBuffer struct {
	bytes.Buffer
	truncated bool
	onText    func()
}

func (b *integrationAuditBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 2<<20 - b.Len(); len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, err := b.Buffer.Write(p)
	if b.onText != nil {
		for _, line := range bytes.Split(b.Bytes(), []byte("\n")) {
			data, ok := bytes.CutPrefix(line, []byte("data: "))
			var event struct{ Type string }
			if ok && json.Unmarshal(data, &event) == nil && event.Type == "response.output_text.delta" {
				b.onText()
				b.onText = nil
				break
			}
		}
	}
	return n, err
}

func TestIntegrationAuditSignalsOnlyTextStart(t *testing.T) {
	starts := 0
	b := &integrationAuditBuffer{onText: func() { starts++ }}
	_, _ = b.Write([]byte("data: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"response.output_text.delta\"}\n"))
	_, _ = b.Write([]byte("data: {\"type\": \"response.output_text."))
	if starts != 0 {
		t.Fatal("text start signalled before a complete text event")
	}
	_, _ = b.Write([]byte("delta\",\"delta\":\"PUBLIC\"}\n"))
	_, _ = b.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"TEXT\"}\n"))
	if starts != 1 {
		t.Fatal("text start must be signalled exactly once")
	}
}

func (b *integrationBody) Close() error {
	err := b.Closer.Close()
	if b.buffer.truncated {
		b.t.Error("backend evidence exceeded audit bound")
	}
	deltas, final := 0, 0
	marker := false
	for _, line := range bytes.Split(b.buffer.Bytes(), []byte("\n")) {
		data, ok := bytes.CutPrefix(line, []byte("data: "))
		if !ok {
			continue
		}
		var event struct {
			Type     string
			Delta    string
			Response struct {
				Output []struct{ Content []struct{ Text string } }
			}
		}
		if json.Unmarshal(data, &event) != nil {
			continue
		}
		if event.Type == "response.output_text.delta" {
			deltas += len(event.Delta)
		}
		if event.Type == "response.completed" {
			for _, item := range event.Response.Output {
				for _, part := range item.Content {
					final += len(part.Text)
					marker = marker || strings.Contains(part.Text, "PUBLIC_PARENT_DONE")
				}
			}
		}
	}
	b.t.Logf("backend text_delta_bytes=%d completed_text_bytes=%d parent_marker=%v", deltas, final, marker)
	return err
}

func privateIntegrationPath(call upstream.Call) bool {
	text := strings.ToLower(strings.ReplaceAll(string(call.Body), `\\`, `\`))
	return strings.Contains(text, `c:\users\`) || strings.Contains(text, "c:/users/")
}

func TestIntegrationPrivatePathsStopBeforeTransport(t *testing.T) {
	// A nil Direct transport would panic if either guard reached credentials or
	// the network. All inputs are synthetic, including the private-shaped path.
	d := integrationLiveTransport{}
	for _, path := range []string{`C:\Users\PublicSynthetic\task`, `c:/users/PublicSynthetic/task`, `C:\\Users\\PublicSynthetic\\task`} {
		call := upstream.Call{Body: []byte(path)}
		_, executeErr := d.Execute(context.Background(), call)
		_, countErr := d.Count(context.Background(), call)
		for _, err := range []error{executeErr, countErr} {
			var failure upstream.Failure
			if !errors.As(err, &failure) || failure.Category != "EVIDENCE_PRIVATE_PATH" {
				t.Fatal("private path crossed the live transport boundary")
			}
		}
	}
	if privateIntegrationPath(upstream.Call{Body: []byte(`D:/public-synthetic/task`)}) {
		t.Fatal("public task path rejected")
	}
	t.Setenv("TMP", `C:\Users\PublicSynthetic\Temp`)
	t.Setenv("TEMP", `C:\Users\PublicSynthetic\Temp`)
	if integrationPathsPrivate(t) == "" {
		t.Fatal("a private temporary root was accepted before the first request")
	}
	t.Setenv("TMP", `D:\public-synthetic`)
	t.Setenv("TEMP", `D:\public-synthetic`)
	if got, hook := integrationPathsPrivate(t), privateIntegrationPath(upstream.Call{Body: []byte(hookPath(t))}); (got == "hook") != hook || !hook && got != "" {
		t.Fatalf("hook location check: got %q, private hook %v", got, hook)
	}
}

// The transport guard refuses a request that names a private path, but only after the
// requests before it were billed. Native sends the working directory, under the temporary
// root, with the first request, and the PreCompact hook's command path, next to this test
// binary, in the request after /compact (measured on 2.1.280). Refuse before anything is sent.
func integrationPathsPrivate(t *testing.T) string {
	switch {
	case privateIntegrationPath(upstream.Call{Body: []byte(os.TempDir())}):
		return "temporary root"
	case privateIntegrationPath(upstream.Call{Body: []byte(hookPath(t))}):
		return "hook"
	}
	return ""
}

func (d integrationLiveTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if privateIntegrationPath(call) {
		return nil, upstream.Failure{Category: "EVIDENCE_PRIVATE_PATH", Status: 400}
	}
	response, err := d.Direct.Execute(ctx, call)
	if err == nil {
		buffer := &integrationAuditBuffer{onText: func() { d.t.Log("backend text_stream_started") }}
		response.Body = &integrationBody{Reader: io.TeeReader(response.Body, buffer), Closer: response.Body, buffer: buffer, t: d.t}
	}
	return response, err
}

func (d integrationLiveTransport) Count(ctx context.Context, call upstream.Call) (int64, error) {
	if privateIntegrationPath(call) {
		return 0, upstream.Failure{Category: "EVIDENCE_PRIVATE_PATH", Status: 400}
	}
	return d.Direct.Count(ctx, call)
}

func integrationLive(t *testing.T) (integrationLiveTransport, map[string]string, string) {
	t.Helper()
	if os.Getenv("CLAUDUCT_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit live switch absent")
	}
	if where := integrationPathsPrivate(t); where != "" {
		t.Fatalf("EVIDENCE_PRIVATE_PATH before any request: the %s is under C:\\Users; build the test binary and set TEMP/TMP elsewhere", where)
	}
	p := &auth.Provider{}
	if err := p.CheckRuntime(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	if err := p.CheckHome(); err != nil {
		t.Fatal(auth.CategoryOf(err))
	}
	for _, dir := range []string{`C:\Program Files\ClaudeCode`, `C:\ProgramData\ClaudeCode`} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("managed native configuration requires review")
		}
	}
	version, err := upstream.InstalledVersion()()
	if err != nil {
		t.Fatal("client version unavailable")
	}
	d := upstream.NewDirect(p, upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 8}), upstream.Fixed(version))
	d.Client.Timeout = 90 * time.Second
	t.Cleanup(func() {
		_ = d.Close()
		a, i, r := d.Ledger.Spent()
		t.Logf("live attempts=%d inferences=%d refused=%d", a, i, r)
	})
	env, cwd := integrationWorkspace(t)
	return integrationLiveTransport{d, t}, env, cwd
}

func integrationWorkspace(t *testing.T) (map[string]string, string) {
	t.Helper()
	root := t.TempDir()
	config, cwd := filepath.Join(root, "profile"), filepath.Join(root, "public-project")
	for _, dir := range []string{config, cwd} {
		if os.MkdirAll(dir, 0700) != nil {
			t.Fatal("public workspace creation")
		}
	}
	env := map[string]string{}
	for _, name := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "PATH", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "OS", "TEMP", "TMP"} {
		if value := os.Getenv(name); value != "" {
			env[name] = value
		}
	}
	for _, name := range []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA", "CLAUDE_CONFIG_DIR"} {
		env[name] = config
	}
	env["CLAUDE_CODE_FORK_SUBAGENT"] = "1"
	return env, cwd
}

const publicIntegrationRoles = `{"Fork":{"description":"Public synthetic role named Fork","prompt":"You are a public test worker. Use no tools. Reply with exactly the identifier requested in your task.","model":"gpt-5.6-luna","effort":"low","tools":[]},"public-peer":{"description":"Public synthetic peer","prompt":"You are a public test worker. Use no tools. Reply with exactly the identifier requested in your task.","model":"gpt-5.6-luna","effort":"low","tools":[]}}`

func TestRuntimeEvidenceNativeRolesAndRestart(t *testing.T) {
	d, env, cwd := integrationLive(t)
	buildHook(t)
	exe := nativeAvailable(t)
	const session = "603cfe81-fdb7-43b2-bbb6-abfde8156393"
	for i, prompt := range []string{
		"Public synthetic integration test. Use Agent to run exactly two children in parallel, one subagent_type Fork and one subagent_type public-peer, both model gpt-5.6-luna and effort low. Their tasks are respectively: reply exactly PUBLIC_FORK_17; reply exactly PUBLIC_PEER_29. Do not use any filesystem or shell tools. Wait for both results. Your final answer must be exactly: PUBLIC_FORK_17 PUBLIC_PEER_29 PUBLIC_PARENT_DONE PUBLIC_RESTART_43. Do not end your turn before both children finish.",
		"Use no tools. Copy verbatim the four PUBLIC_ identifiers from your previous final answer, not the agent IDs. Finish with PUBLIC_RESTART_DONE.",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		var out, stderr bytes.Buffer
		args := []string{"-p", prompt, "--model", "gpt-5.6-luna", "--effort", "low", "--tools", "Agent,ToolSearch,SendMessage", "--allowedTools", "Agent,ToolSearch,SendMessage", "--strict-mcp-config", "--setting-sources", "", "--agents", publicIntegrationRoles}
		if i == 0 {
			args = append(args, "--session-id", session)
		} else {
			args = append(args, "--resume", session)
		}
		result, err := Run(ctx, Options{Args: args, Env: env, Cwd: cwd, Stdout: &out, Stderr: &stderr, Ledger: d.Ledger,
			ResolveClaude: func() (string, bool, error) { return exe, true, nil },
			StartGateway: func() (*gateway.Gateway, error) {
				g, e := gateway.Start(d)
				if e == nil {
					g.EnableContextPolicy()
				}
				return g, e
			},
		})
		cancel()
		t.Logf("phase=%d stdout_bytes=%d fork_marker=%v peer_marker=%v parent_marker=%v restart_marker=%v", i, out.Len(), strings.Contains(out.String(), "PUBLIC_FORK_17"), strings.Contains(out.String(), "PUBLIC_PEER_29"), strings.Contains(out.String(), "PUBLIC_PARENT_DONE"), strings.Contains(out.String(), "PUBLIC_RESTART_43"))
		t.Logf("phase=%d exit=%d category=%s cleanup_ok=%v parent_received=%d", i, result.NativeExitCode, result.Category, result.CleanupErr == nil, result.Diagnostics.AgentResults.Totals["parent_received"])
		for _, record := range result.Diagnostics.Recent {
			if record.Model != "" || record.Category != "" {
				t.Logf("phase=%d class=%s role=%s model=%s effort=%s status=%d category=%s verified=%v", i, record.RequestClass, record.AgentRole, record.Model, record.Effort, record.Status, record.Category, record.SelectionVerified != nil && *record.SelectionVerified)
			}
		}
		if err != nil || result.NativeExitCode != 0 || result.CleanupErr != nil {
			t.Fatal("native integration did not complete")
		}
		for _, marker := range []string{"PUBLIC_FORK_17", "PUBLIC_PEER_29"} {
			if !strings.Contains(out.String(), marker) {
				t.Fatal("child result did not reach native output")
			}
		}
		if i == 0 && result.Diagnostics.AgentResults.Totals["parent_received"] != 2 {
			t.Fatal("both child completions were not tracked")
		}
		if i == 1 && (!strings.Contains(out.String(), "PUBLIC_RESTART_43") || !strings.Contains(out.String(), "PUBLIC_RESTART_DONE")) {
			t.Fatal("native restart lost public history")
		}
	}
}

// Run only in an actual PTY. A human/terminal driver supplies public prompts,
// /compact, Esc and /exit; the product owns the native process and its cleanup.
func TestRuntimeEvidenceNativeTUI(t *testing.T) {
	if os.Getenv("CLAUDUCT_EVIDENCE_TUI") != "1" {
		t.Skip("interactive live switch absent")
	}
	d, env, cwd := integrationLive(t)
	d.Ledger = upstream.NewLedger(upstream.Budget{Model: "gpt-5.6-luna", Effort: "low", Limit: 5})
	runIntegrationTUI(t, d, d.Ledger, env, cwd)
}

func runIntegrationTUI(t *testing.T, transport upstream.Transport, ledger *upstream.Ledger, env map[string]string, cwd string) {
	t.Helper()
	buildHook(t)
	exe := nativeAvailable(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := Run(ctx, Options{Args: []string{"--model", "gpt-5.6-luna", "--effort", "low", "--tools", "", "--strict-mcp-config", "--setting-sources", ""}, Env: env, Cwd: cwd, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Ledger: ledger,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			g, e := gateway.Start(transport)
			if e == nil {
				g.EnableContextPolicy()
			}
			return g, e
		},
	})
	for _, r := range result.Diagnostics.Recent {
		t.Logf("tui class=%s kind=%s outcome=%s category=%s cancellation=%s", r.RequestClass, r.Kind, r.Outcome, r.Category, r.CancellationSource)
	}
	if err != nil || !result.NativeStarted || result.NativeExitCode != 0 || result.CleanupErr != nil {
		t.Fatal("TUI lifecycle incomplete")
	}
	if err := assessTUIDiagnostics(result.Diagnostics); err != nil {
		t.Fatal(err)
	}
	if err := assessTUIProject(env["CLAUDE_CONFIG_DIR"]); err != nil {
		t.Fatal(err)
	}
	t.Log("TUI acceptance: generation=3 compaction=1 cancellation=1; native transcript facts and order verified; exit=0 cleanup_ok=true")
}
