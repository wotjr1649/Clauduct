package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// The fake native client is this test binary re-executed with a sentinel set.
//
// It is a real executable on disk taking a real Windows command line, so the argument
// vector it reports has made the full round trip through process creation. That is the
// point: comparing an in-memory slice to itself would prove nothing about spawning.
//
// Known limit: both ends are Go, so this measures Go's command-line quoting against Go's
// parsing. A native binary that parses its command line by other rules is a NATIVE_SYNTH
// question (ARG09) and is out of WP01's reach.
const (
	fakeSentinel = "CLAUDUCT_FAKE_CHILD"
	fakeReport   = "CLAUDUCT_FAKE_REPORT"
	fakeExit     = "CLAUDUCT_FAKE_EXIT"
	fakeEcho     = "CLAUDUCT_FAKE_ECHO"
	fakeDial     = "CLAUDUCT_FAKE_DIAL"
)

type childReport struct {
	Argv          []string          `json:"argv"`
	Env           map[string]string `json:"env"`
	Cwd           string            `json:"cwd"`
	ReadinessCode int               `json:"readiness_code"`
}

func TestMain(m *testing.M) {
	if os.Getenv(fakeSentinel) == "1" {
		fakeChildMain()
		return
	}
	os.Exit(m.Run())
}

func fakeChildMain() {
	report := childReport{Argv: os.Args[1:], Env: map[string]string{}}
	report.Cwd, _ = os.Getwd()
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			report.Env[key] = value
		}
	}
	if os.Getenv(fakeDial) == "1" {
		report.ReadinessCode = dialReadiness(os.Getenv("ANTHROPIC_BASE_URL"))
	}
	if path := os.Getenv(fakeReport); path != "" {
		if encoded, err := json.Marshal(report); err == nil {
			os.WriteFile(path, encoded, 0o600)
		}
	}
	if os.Getenv(fakeEcho) == "1" {
		body, _ := io.ReadAll(os.Stdin)
		os.Stdout.Write(body)
		os.Stderr.WriteString("fake-stderr")
	}
	code := 0
	if raw := os.Getenv(fakeExit); raw != "" {
		for _, r := range raw {
			code = code*10 + int(r-'0')
		}
	}
	os.Exit(code)
}

func dialReadiness(base string) int {
	if base == "" {
		return -1
	}
	req, err := http.NewRequest(http.MethodHead, base+"/api/hello", nil)
	if err != nil {
		return -2
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return -3
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// --- harness ---------------------------------------------------------------------------

type session struct {
	t      *testing.T
	report childReport
	result Result
	err    error
	stdout bytes.Buffer
	stderr bytes.Buffer
}

type sessionOptions struct {
	args     []string
	env      map[string]string
	cwd      string
	stdin    io.Reader
	exit     string
	echo     bool
	dial     bool
	notFound bool
}

func runSession(t *testing.T, so sessionOptions) *session {
	t.Helper()
	reportPath := filepath.Join(t.TempDir(), "report.json")

	env := map[string]string{}
	for k, v := range so.env {
		env[k] = v
	}
	// Windows process creation needs these; the production path inherits them from the
	// parent, and a synthetic source map has to supply them explicitly.
	for _, name := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(name); value != "" {
			env[name] = value
		}
	}
	env[fakeSentinel] = "1"
	env[fakeReport] = reportPath
	if so.exit != "" {
		env[fakeExit] = so.exit
	}
	if so.echo {
		env[fakeEcho] = "1"
	}
	if so.dial {
		env[fakeDial] = "1"
	}

	cwd := so.cwd
	if cwd == "" {
		cwd = t.TempDir()
	}
	stdin := so.stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}

	s := &session{t: t}
	s.result, s.err = Run(context.Background(), Options{
		Args:   so.args,
		Env:    env,
		Cwd:    cwd,
		Stdin:  stdin,
		Stdout: &s.stdout,
		Stderr: &s.stderr,
		ResolveClaude: func() (string, bool, error) {
			if so.notFound {
				return "C:\\nowhere\\claude.exe", false, nil
			}
			return os.Args[0], true, nil
		},
	})

	if raw, err := os.ReadFile(reportPath); err == nil {
		json.Unmarshal(raw, &s.report)
	}
	return s
}

// --- ARG: what the child actually received ----------------------------------------------

// ARG01 through a real spawn: order, count and values survive process creation.
func TestChildReceivesArgvUnchanged(t *testing.T) {
	args := []string{"--resume", "session-abc", "-p", "first", "second"}
	s := runSession(t, sessionOptions{args: args})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if !reflect.DeepEqual(s.report.Argv, args) {
		t.Fatalf("child argv\n got: %#v\nwant: %#v", s.report.Argv, args)
	}
}

// ARG02, ARG03, ARG10 through a real spawn. If Windows quoting were wrong, these are the
// shapes that would come back merged, split, unescaped or executed.
func TestChildReceivesHostileArgvUnchanged(t *testing.T) {
	args := []string{
		" spaced ",
		`quoted "inner" text`,
		`C:\dir\with\trailing\`,
		`mixed"\backslash\"quote`,
		"&", "|", ">", "<", "^", "%PATH%", "!DELAYED!", ";",
		"$(whoami)",
		"한국어 인자와 공백",
		"emoji 🙂 arg",
		`{"a":"b\\c"}`,
	}
	s := runSession(t, sessionOptions{args: args})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if !reflect.DeepEqual(s.report.Argv, args) {
		for i := range args {
			if i >= len(s.report.Argv) {
				t.Errorf("arg %d missing: %q", i, args[i])
				continue
			}
			if s.report.Argv[i] != args[i] {
				t.Errorf("arg %d\n got: %q\nwant: %q", i, s.report.Argv[i], args[i])
			}
		}
		t.Fatalf("argv count got %d want %d", len(s.report.Argv), len(args))
	}
}

// ARG05 through a real spawn: a value that reads like an option is still a value.
func TestChildReceivesOptionLookalikeValues(t *testing.T) {
	args := []string{"--append-system-prompt", "--model is a description", "--", "--help", "-p"}
	s := runSession(t, sessionOptions{args: args})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if !reflect.DeepEqual(s.report.Argv, args) {
		t.Fatalf("child argv\n got: %#v\nwant: %#v", s.report.Argv, args)
	}
}

// ARG10: an empty argument is a real argument. Windows quoting historically loses it.
func TestEmptyArgumentSurvives(t *testing.T) {
	args := []string{"before", "", "after"}
	s := runSession(t, sessionOptions{args: args})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if !reflect.DeepEqual(s.report.Argv, args) {
		t.Fatalf("child argv\n got: %#v\nwant: %#v", s.report.Argv, args)
	}
}

// ARG08: stdin, stdout, stderr, cwd and exit code all cross the boundary.
func TestStdioCwdAndExitCodeAreCarried(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	s := runSession(t, sessionOptions{
		cwd:   dir,
		stdin: strings.NewReader("piped stdin payload"),
		echo:  true,
		exit:  "7",
	})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if s.result.NativeExitCode != 7 {
		t.Errorf("exit code = %d, want 7", s.result.NativeExitCode)
	}
	if got := s.stdout.String(); got != "piped stdin payload" {
		t.Errorf("stdout = %q, want the stdin payload echoed back", got)
	}
	if got := s.stderr.String(); got != "fake-stderr" {
		t.Errorf("stderr = %q, want fake-stderr", got)
	}
	childCwd, err := filepath.EvalSymlinks(s.report.Cwd)
	if err != nil {
		childCwd = s.report.Cwd
	}
	if !strings.EqualFold(childCwd, resolved) {
		t.Errorf("child cwd = %q, want %q", childCwd, resolved)
	}
}

// A non-zero native exit is the native process's answer, not a bridge failure.
func TestNonZeroNativeExitIsNotABridgeError(t *testing.T) {
	s := runSession(t, sessionOptions{exit: "3"})
	if s.err != nil {
		t.Fatalf("Run returned an error for a non-zero child exit: %v", s.err)
	}
	if s.result.NativeExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", s.result.NativeExitCode)
	}
}

// --- ENV: what the child can see ---------------------------------------------------------

// ENV01 and ENV02 together, observed in the child's own environment rather than in a spec.
func TestChildEnvironmentMatchesTheAcceptedRule(t *testing.T) {
	const leak = "PARENT-ANTHROPIC-CREDENTIAL"
	s := runSession(t, sessionOptions{
		dial: true,
		env: map[string]string{
			"ANTHROPIC_API_KEY":       leak,
			"ANTHROPIC_AUTH_TOKEN":    leak,
			"CLAUDE_CODE_OAUTH_TOKEN": leak,
			// Names outside the overlay's five keys. The overlay overwrites those five
			// unconditionally, so without these the check passes even with the prefix
			// rule deleted. A mutation run caught exactly that.
			"ANTHROPIC_API_URL":             leak,
			"ANTHROPIC_BEDROCK_BASE_URL":    leak,
			"ANTHROPIC_DEFAULT_OPUS_MODEL":  leak,
			"GITHUB_TOKEN":                  "gh-dummy",
			"SLACK_BOT_TOKEN":               "slack-dummy",
			"AWS_SECRET_ACCESS_KEY":         "aws-dummy",
			"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32000",
			"CLAUDE_CONFIG_DIR":             "C:\\Users\\dev\\.claude",
		},
	})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}

	for name, value := range s.report.Env {
		if strings.Contains(value, leak) {
			t.Fatalf("parent Anthropic credential reached the child in %s", name)
		}
	}
	for name, want := range map[string]string{
		"GITHUB_TOKEN":                  "gh-dummy",
		"SLACK_BOT_TOKEN":               "slack-dummy",
		"AWS_SECRET_ACCESS_KEY":         "aws-dummy",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32000",
		"CLAUDE_CONFIG_DIR":             "C:\\Users\\dev\\.claude",
	} {
		if got := s.report.Env[name]; got != want {
			t.Errorf("child %s = %q, want %q", name, got, want)
		}
	}
	if got := s.report.Env["ANTHROPIC_BASE_URL"]; got != "http://"+s.result.GatewayAddr {
		t.Errorf("child ANTHROPIC_BASE_URL = %q, want the session gateway", got)
	}
	if s.report.Env["ANTHROPIC_AUTH_TOKEN"] == "" {
		t.Error("child has no session token")
	}
	// The address handed to the child is one the child can actually reach. Binding and
	// telling are two different things and only this asserts the second.
	if s.report.ReadinessCode != http.StatusOK {
		t.Errorf("child readiness probe = %d, want 200", s.report.ReadinessCode)
	}
}

// --- LIFE: order and release --------------------------------------------------------------

// A missing executable must not cost a bound port.
func TestMissingExecutableBindsNothing(t *testing.T) {
	s := runSession(t, sessionOptions{notFound: true})
	if !errors.Is(s.err, ErrClaudeNotFound) {
		t.Fatalf("err = %v, want ErrClaudeNotFound", s.err)
	}
	if s.result.GatewayAddr != "" {
		t.Fatalf("a port was bound before resolution succeeded: %s", s.result.GatewayAddr)
	}
	if s.result.NativeStarted {
		t.Fatal("a child was started without an executable")
	}
}

// LIFE01: if the gateway cannot come up, the child is never spawned.
func TestGatewayFailureStopsBeforeSpawn(t *testing.T) {
	sentinel := errors.New("BIND_REFUSED")
	spawned := false

	result, err := Run(context.Background(), Options{
		Cwd:           t.TempDir(),
		Stdin:         strings.NewReader(""),
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		ResolveClaude: func() (string, bool, error) { return os.Args[0], true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return nil, sentinel },
		StartProcess: func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error) {
			spawned = true
			return nil, nil
		},
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the bind failure", err)
	}
	if spawned {
		t.Fatal("the child was started without a gateway to talk to")
	}
	if result.NativeStarted {
		t.Fatal("result claims the native process started")
	}
}

// LIFE02: a child that fails to start must not leave its port bound.
func TestSpawnFailureReleasesThePort(t *testing.T) {
	sentinel := errors.New("SPAWN_REFUSED")
	var addr string

	result, err := Run(context.Background(), Options{
		Cwd:           t.TempDir(),
		Stdin:         strings.NewReader(""),
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		ResolveClaude: func() (string, bool, error) { return os.Args[0], true, nil },
		StartProcess: func(spec launch.Spec, _ io.Reader, _, _ io.Writer) (Process, error) {
			return nil, sentinel
		},
	})
	addr = result.GatewayAddr

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the spawn failure", err)
	}
	if result.CleanupErr != nil {
		t.Errorf("cleanup reported %v", result.CleanupErr)
	}
	if addr == "" {
		t.Fatal("no address recorded, so the release cannot be checked")
	}
	if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
		conn.Close()
		t.Fatalf("%s still accepting after a failed spawn", addr)
	}
}

// LIFE03: a normal session releases what it owned.
func TestNormalExitReleasesThePort(t *testing.T) {
	s := runSession(t, sessionOptions{})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	if s.result.CleanupErr != nil {
		t.Errorf("cleanup reported %v", s.result.CleanupErr)
	}
	if conn, err := net.DialTimeout("tcp", s.result.GatewayAddr, 2*time.Second); err == nil {
		conn.Close()
		t.Fatalf("%s still accepting after the session ended", s.result.GatewayAddr)
	}
}

// LIFE14 in miniature: repeated sessions must not accumulate listeners.
func TestRepeatedSessionsDoNotAccumulate(t *testing.T) {
	addrs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		s := runSession(t, sessionOptions{})
		if s.err != nil {
			t.Fatalf("session %d: %v", i, s.err)
		}
		addrs = append(addrs, s.result.GatewayAddr)
	}
	for _, addr := range addrs {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			conn.Close()
			t.Errorf("%s survived its session", addr)
		}
	}
}

// ENV09: the session token is a secret the moment it exists. It must not reach any stream
// a user or a log would read.
func TestSessionTokenIsNotWrittenToStdioOrError(t *testing.T) {
	s := runSession(t, sessionOptions{args: []string{"--version"}})
	if s.err != nil {
		t.Fatalf("Run: %v", s.err)
	}
	token := s.report.Env["ANTHROPIC_AUTH_TOKEN"]
	if token == "" {
		t.Fatal("no token to check")
	}
	for name, stream := range map[string]string{"stdout": s.stdout.String(), "stderr": s.stderr.String()} {
		if strings.Contains(stream, token) {
			t.Errorf("session token appeared on %s", name)
		}
	}
	if s.result.CleanupErr != nil && strings.Contains(s.result.CleanupErr.Error(), token) {
		t.Error("session token appeared in a cleanup error")
	}
}
