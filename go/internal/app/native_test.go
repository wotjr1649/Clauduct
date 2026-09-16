package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// WP06 runs the real claude.exe against a fixture backend. That is the one thing a fixture
// cannot fake: the client is the actual installed binary, so its behaviour is measured
// rather than described.
//
// Three rules are baked into the harness rather than left to each test.
//
// CLAUDE_CONFIG_DIR points at a temporary directory. Measured 2026-09-15: the client
// honours it, and everything it persists lands there. Without it a test session reads and
// writes the user's real Claude state, which is not the test's to touch.
//
// --strict-mcp-config is always passed. Without it the client would start whatever MCP
// servers the user has configured, and a test that spawns a person's Slack or database
// server is not isolated by any definition.
//
// The working directory is temporary too, so a project-level settings file cannot reach in.
//
// What is deliberately NOT defaulted is --bare. It would skip hooks, plugins and CLAUDE.md
// discovery, which is exactly what several of these tests exist to observe; defaulting to
// it would make every result unrepresentative of an ordinary session. Residual risk, stated
// rather than papered over: a system-wide managed settings file would still apply, and this
// harness does not isolate one.

// defaultNativeTimeout bounds one NATIVE_SYNTH run.
//
// A session against a fixture backend completes in about a second, so this is generous. It
// is deliberately not minutes: a launcher defect leaves the real client retrying, and a
// long bound turns every broken run into a wait rather than a failure.
const defaultNativeTimeout = 25 * time.Second

// nativeAvailable skips when the installed client is not present. These tests measure a
// real binary, and without it there is nothing to measure -- which is not the same as
// passing.
func nativeAvailable(t *testing.T) string {
	t.Helper()
	path, found, err := platform.Resolver{}.Claude()
	if err != nil {
		t.Skipf("resolving claude.exe: %v", err)
	}
	if !found {
		t.Skipf("claude.exe not installed at %s; NATIVE_SYNTH measures a real client", path)
	}
	return path
}

// measuredStream is the reply shape the real backend was observed to send on 2026-09-15.
//
// It matters that this is the measured shape and not a plausible one. WP04 was built on a
// fixture that put items in the completion's output array; the real backend leaves that
// array empty and delivers items through output_item events. A fixture agrees with whoever
// wrote it, so this one is written from what arrived.
func measuredStream(text string) string {
	encoded, _ := json.Marshal(text)
	item := `{"id":"msg_synth","type":"message","content":[{"type":"output_text","text":` +
		string(encoded) + `}]}`
	frames := []string{
		`{"type":"response.created","response":{"id":"resp_synth"}}`,
		`{"type":"response.in_progress"}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_synth","type":"message"}}`,
		`{"type":"response.content_part.added"}`,
		`{"type":"response.output_text.delta","item_id":"msg_synth","content_index":0,"delta":` + string(encoded) + `}`,
		`{"type":"response.output_text.done","item_id":"msg_synth","content_index":0,"text":` + string(encoded) + `}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`,
		`{"type":"response.completed","response":{"id":"resp_synth",` +
			`"usage":{"input_tokens":9,"output_tokens":2,"total_tokens":11},"output":[]}}`,
	}
	var b strings.Builder
	for _, frame := range frames {
		b.WriteString("data: " + frame + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// nativeRun describes one NATIVE_SYNTH run.
type nativeRun struct {
	// Args are the native options under test. The harness adds its own isolation flags.
	Args []string
	// Reply is what the fixture backend answers. Empty means the backend is never asked.
	Reply string
	// Env adds to the parent environment the child inherits.
	Env map[string]string
	// ConfigDir overrides the synthetic CLAUDE_CONFIG_DIR. Empty allocates one.
	ConfigDir string
	// Bare passes --bare, which skips hooks, plugins, keychain reads and CLAUDE.md.
	Bare bool
	// Timeout bounds the whole run.
	Timeout time.Duration
	// fixture lets a test keep the backend handle and read what was actually sent. Zero
	// allocates one from Reply.
	fixture *upstream.Fixture
	// transport replaces the fixture entirely, for a test that needs to see the calls
	// rather than the bytes.
	transport upstream.Transport
}

// workspace is a working directory with a parent nobody else writes to.
//
// The wrapper's predecessor wrote a status directory *beside* the project, so answering
// "did anything appear next to the working directory" needs a parent this run owns. A
// t.TempDir is shared with every other t.TempDir in the test.
func workspace(t *testing.T) (parent, cwd string) {
	t.Helper()
	parent = filepath.Join(t.TempDir(), "workspace")
	cwd = filepath.Join(parent, "project")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	return parent, cwd
}

type nativeOutcome struct {
	result         Result
	err            error
	stdout, stderr string
	backendCalls   int64
	// received is every request the gateway answered, which is more than the inferences
	// when the client asks for anything else -- the model list, for instance.
	received int64
	// modelLists is how many times the client asked for the model list, which is the only
	// thing visible from here that says whether gateway discovery is on.
	modelLists int64
	configDir  string
	cwd        string
	// parentBefore and parentAfter bracket the directory containing the working directory,
	// so anything the wrapper leaves beside a project is visible by comparison rather than
	// by guessing at its name.
	parent       string
	parentBefore []string
	parentAfter  []string
}

func (o nativeOutcome) output() string { return o.stdout + o.stderr }

func (s nativeRun) run(t *testing.T) nativeOutcome {
	t.Helper()
	exe := nativeAvailable(t)

	configDir := s.ConfigDir
	if configDir == "" {
		configDir = t.TempDir()
	}
	parent, cwd := workspace(t)
	before := tree(t, parent)

	env := map[string]string{}
	for _, entry := range os.Environ() {
		if i := strings.IndexByte(entry, '='); i > 0 {
			env[entry[:i]] = entry[i+1:]
		}
	}
	// The isolation, applied after the inherited environment so nothing can undo it.
	env["CLAUDE_CONFIG_DIR"] = configDir
	for name, value := range s.Env {
		env[name] = value
	}

	args := append([]string{}, s.Args...)
	if s.Bare {
		args = append(args, "--bare")
	}
	// Never optional. The user's MCP servers are not this test's to start.
	args = append(args, "--strict-mcp-config")

	fixture := s.fixture
	if fixture == nil {
		fixture = &upstream.Fixture{SSE: s.Reply}
	}
	var transport upstream.Transport = fixture
	if s.transport != nil {
		transport = s.transport
	}
	var started *gateway.Gateway
	var stdout, stderr bytes.Buffer

	timeout := s.Timeout
	if timeout == 0 {
		timeout = defaultNativeTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args:          args,
		Env:           env,
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			g, err := gateway.Start(transport)
			started = g
			return g, err
		},
	})
	return nativeOutcome{
		result: result, err: err,
		stdout: stdout.String(), stderr: stderr.String(),
		backendCalls: fixture.Calls(), received: receivedBy(started),
		modelLists: modelListsBy(started),
		configDir:  configDir, cwd: cwd,
		parent: parent, parentBefore: before, parentAfter: tree(t, parent),
	}
}

// tree lists a directory's contents relative to it, so two snapshots can be compared.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	sort.Strings(found)
	return found
}

// ARG07: asking the client what version it is must not need a credential, and must not
// reach a backend. A wrapper that made --version cost an inference would be charging for a
// question it could answer itself.
func TestNativeVersionAndHelpNeedNoBackend(t *testing.T) {
	for name, args := range map[string][]string{
		"version": {"--version"},
		"help":    {"--help"},
	} {
		t.Run(name, func(t *testing.T) {
			got := nativeRun{Args: args, Timeout: defaultNativeTimeout}.run(t)
			if got.err != nil {
				t.Fatalf("Run: %v", got.err)
			}
			if got.result.NativeExitCode != 0 {
				t.Fatalf("exit = %d: %s", got.result.NativeExitCode, got.output())
			}
			if got.backendCalls != 0 {
				t.Fatalf("%s reached the backend %d times, want 0", name, got.backendCalls)
			}
			if got.result.CleanupErr != nil {
				t.Fatalf("cleanup: %v", got.result.CleanupErr)
			}
		})
	}
}

// ARG06: an option this launcher does not recognise is the native's business. The error the
// user sees has to be the native's own, because a launcher error would describe a problem
// the native never had.
func TestUnknownNativeOptionKeepsTheNativeError(t *testing.T) {
	got := nativeRun{Args: []string{"--clauduct-made-this-up"}, Timeout: defaultNativeTimeout}.run(t)

	// Not a launcher refusal. The launcher owns no option, so it cannot have an opinion.
	var refused *RefusedOptionError
	if got.err != nil {
		if errors.As(got.err, &refused) {
			t.Fatalf("the launcher refused an option it does not own: %v", got.err)
		}
		t.Fatalf("Run: %v", got.err)
	}
	if got.result.NativeExitCode == 0 {
		t.Fatalf("an unknown option exited 0: %s", got.output())
	}
	if !strings.Contains(got.output(), "clauduct-made-this-up") {
		t.Fatalf("the native's own error did not survive: %q", got.output())
	}
	if got.backendCalls != 0 {
		t.Fatalf("a rejected option reached the backend %d times", got.backendCalls)
	}
}

// ENV04: the user's own CLAUDE_CONFIG_DIR is their choice and must reach the child intact.
// Measured 2026-09-15: the client honours it, which is also what makes this harness safe.
func TestTheUsersConfigDirChoiceIsPreserved(t *testing.T) {
	got := nativeRun{
		Args:    []string{"-p", "say ok"},
		Reply:   measuredStream("ok"),
		Timeout: defaultNativeTimeout,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	// The proof is that the client's own state landed there. A variable that was dropped
	// would have sent all of this to the user's real profile.
	if entries := tree(t, got.configDir); len(entries) == 0 {
		t.Fatal("the synthetic config dir is empty; CLAUDE_CONFIG_DIR did not reach the child")
	}
}

// ENV10 and CAP04: the native's own persistent writes are its business. This wrapper's are
// not, and there are none.
//
// V1 wrote a status directory beside the project and injected a hooks entry into settings.
// Neither survives here, and the check is what the filesystem says rather than what the
// code claims.
func TestTheWrapperWritesNothingOfItsOwn(t *testing.T) {
	got := nativeRun{
		Args:    []string{"-p", "say ok"},
		Reply:   measuredStream("ok"),
		Timeout: defaultNativeTimeout,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}

	// Everything, not just entries with a telling name. Looking for "clauduct" would pass
	// for a status file called anything else, which is how a check comes to assert nothing.
	//
	// Beside the project, not inside it. ENV10 asks for the native's ordinary persistence
	// to be told apart from the wrapper's, and anything the client writes into the working
	// directory is the client's own doing.
	var beside []string
	for _, entry := range added(got.parentBefore, got.parentAfter) {
		if entry == "project" || strings.HasPrefix(entry, "project/") {
			continue
		}
		beside = append(beside, entry)
	}
	if len(beside) != 0 {
		t.Fatalf("a session left %v beside the working directory. The native's own state "+
			"belongs in CLAUDE_CONFIG_DIR and the wrapper's nowhere.", beside)
	}

	// The native's own state did land, which is what makes the check above meaningful: the
	// session really ran rather than failing before it could write anything.
	if entries := tree(t, got.configDir); len(entries) == 0 {
		t.Fatal("the session wrote nothing at all; it cannot have run")
	}

	// CAP04: nothing of the wrapper's inside the native's state either. V1 injected a hooks
	// entry pointing at its own script; a path into this build would show up as one.
	for _, entry := range tree(t, got.configDir) {
		if strings.Contains(strings.ToLower(entry), "clauduct") {
			t.Fatalf("the wrapper wrote %q into the native's config directory", entry)
		}
	}
	settings := filepath.Join(got.configDir, ".claude.json")
	if raw, err := os.ReadFile(settings); err == nil {
		if strings.Contains(strings.ToLower(string(raw)), "clauduct") {
			t.Fatal("the native's own settings mention this wrapper; something was injected")
		}
	}
}

// added reports entries present after a run that were not present before.
func added(before, after []string) []string {
	had := make(map[string]bool, len(before))
	for _, entry := range before {
		had[entry] = true
	}
	var appeared []string
	for _, entry := range after {
		if !had[entry] {
			appeared = append(appeared, entry)
		}
	}
	return appeared
}

// CAP10: --bare skips hooks, plugins, keychain reads and CLAUDE.md discovery, and states
// that Anthropic auth is then strictly ANTHROPIC_API_KEY. This overlay sets
// ANTHROPIC_AUTH_TOKEN and blanks ANTHROPIC_API_KEY, so reading that sentence alone
// predicts a failure.
//
// Measured 2026-09-15: it connects anyway. The prediction was wrong, which is the reason
// this is a test and not a paragraph.
func TestBareModeStillConnects(t *testing.T) {
	got := nativeRun{
		Args:    []string{"-p", "say ok"},
		Reply:   measuredStream("ok"),
		Bare:    true,
		Timeout: defaultNativeTimeout,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	if got.backendCalls == 0 {
		t.Fatalf("--bare never reached the backend; the session did not authenticate: %s",
			got.output())
	}
}

// The measured reply shape drives a real client end to end. If the client cannot read what
// this bridge emits, that shows up here rather than in a user's session.
func TestAMeasuredReplyReachesTheClient(t *testing.T) {
	got := nativeRun{
		Args:    []string{"-p", "say the word ok"},
		Reply:   measuredStream("ok"),
		Timeout: defaultNativeTimeout,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	if got.backendCalls == 0 {
		t.Fatalf("the backend was never asked: %s", got.output())
	}
	if !strings.Contains(got.stdout, "ok") {
		t.Fatalf("the reply did not reach the client's output: stdout=%q stderr=%q",
			got.stdout, got.stderr)
	}
	if got.result.NativeExitCode != 0 {
		t.Fatalf("exit = %d: %s", got.result.NativeExitCode, got.output())
	}
	if got.result.CleanupErr != nil {
		t.Fatalf("cleanup: %v", got.result.CleanupErr)
	}
}

// ENV08 and ENV02 end to end: a name the user set that this launcher drops must not reach
// the child.
//
// The five overlay names cannot test this. They are overwritten unconditionally, so a
// forwarded value loses to the overlay anyway and the check passes with the denylist
// deleted -- the exact false green WP01 hit. ANTHROPIC_MODEL is dropped by the prefix rule
// and replaced by nothing, so if it leaked the client would act on it, and what it acts on
// reaches the backend request where this can read it.
func TestADroppedAnthropicNameNeverReachesTheChild(t *testing.T) {
	const poison = "clauduct-leak-canary-model"
	fixture := &upstream.Fixture{SSE: measuredStream("ok")}
	got := nativeRun{
		Args:    []string{"-p", "say ok"},
		Reply:   measuredStream("ok"),
		Env:     map[string]string{"ANTHROPIC_MODEL": poison, "ANTHROPIC_SMALL_FAST_MODEL": poison},
		fixture: fixture,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	if got.backendCalls == 0 {
		t.Fatalf("the backend was never asked, so nothing was observed: %s", got.output())
	}
	if strings.Contains(fixture.LastRequest(), poison) {
		t.Fatalf("a dropped ANTHROPIC_ name reached the backend request:\n%s",
			fixture.LastRequest())
	}
}

// ENV06: the user's own agents and hooks are theirs. This wrapper installs none and
// rewrites none, and the check is the bytes on disk rather than the claim.
//
// V1 wrote a hooks entry into settings pointing at its own script. Nothing here does, and a
// change that started would show up as an altered file.
func TestUserAgentsAndSettingsAreLeftAlone(t *testing.T) {
	configDir := t.TempDir()
	agents := filepath.Join(configDir, "agents")
	if err := os.MkdirAll(agents, 0o700); err != nil {
		t.Fatalf("agents dir: %v", err)
	}
	files := map[string]string{
		filepath.Join(agents, "reviewer.md"): "---\nname: reviewer\n---\nThe user's own agent.\n",
		filepath.Join(configDir, "settings.json"): `{"hooks":{"PreToolUse":` +
			`[{"matcher":"Bash","hooks":[{"type":"command","command":"echo the user's own hook"}]}]}}`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got := nativeRun{
		Args:      []string{"-p", "say ok"},
		Reply:     measuredStream("ok"),
		ConfigDir: configDir,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}

	for path, want := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("the session removed %s: %v", filepath.Base(path), err)
		}
		if string(raw) != want {
			t.Fatalf("%s was rewritten.\n want: %s\n got:  %s", filepath.Base(path), want, raw)
		}
	}
}

// ENV07, and an honest gap rather than a pass.
//
// A user who has pointed ANTHROPIC_BASE_URL at another provider has said something, and this
// build overwrites it without a word. That is the right endpoint to use -- a session must
// reach this gateway -- but silently is not the same as diagnosed, and ENV07 asks for a
// diagnosis. This test pins the behaviour that exists so the gap is a recorded decision
// rather than an oversight.
func TestAConflictingBaseURLIsOverriddenButNotYetDiagnosed(t *testing.T) {
	got := nativeRun{
		Args:  []string{"-p", "say ok"},
		Reply: measuredStream("ok"),
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://another-provider.example/v1",
		},
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	// The session reached this gateway, so the user's value did not win.
	if got.backendCalls == 0 {
		t.Fatalf("the session did not reach this gateway: %s", got.output())
	}
	// And nothing said so. When ENV07 is implemented this assertion inverts; until then it
	// records what a user actually experiences.
	if strings.Contains(strings.ToLower(got.output()), "another-provider") {
		t.Fatal("a diagnosis appeared. ENV07 is now implemented and this test must be " +
			"rewritten to require it rather than to record its absence.")
	}
}

// countingTransport reserves against a ledger and then replays, which is what the real
// transport does either side of the network. It exists so the recorded spend can be checked
// without spending anything.
type countingTransport struct {
	ledger *upstream.Ledger
	inner  *upstream.Fixture
}

func (c countingTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	if err := c.ledger.Reserve(upstream.Attempt{Requested: "any", Model: "any", Effort: "any", Source: "test"}); err != nil {
		return nil, err
	}
	return c.inner.Execute(ctx, call)
}

// What a session cost is reported, not estimated.
//
// Added after a mutation run: deleting the ledger read left the suite green, because the
// only thing checking it was the live test and that is skipped unless money is authorised.
// The live run had already caught the same field reading zero -- a deferred write to an
// unnamed return value goes nowhere -- and nothing offline would have.
func TestASessionReportsWhatItSpent(t *testing.T) {
	exe := nativeAvailable(t)

	ledger := upstream.NewLedger(upstream.Unlimited())
	fixture := &upstream.Fixture{SSE: measuredStream("ok")}
	parent, cwd := workspace(t)
	_ = parent

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args:          []string{"-p", "say ok", "--strict-mcp-config"},
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		Ledger:        ledger,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			return gateway.Start(countingTransport{ledger: ledger, inner: fixture})
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	attempts, inferences, _ := ledger.Spent()
	if attempts == 0 {
		t.Fatalf("the session sent nothing: %s%s", stdout.String(), stderr.String())
	}
	if result.Attempts != attempts || result.Inferences != inferences {
		t.Fatalf("Result reported %d attempts / %d inferences against the ledger's %d / %d",
			result.Attempts, result.Inferences, attempts, inferences)
	}
}
