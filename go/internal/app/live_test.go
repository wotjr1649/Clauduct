package app

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// G7 wired the real transport into the product build, so these tests exercise the default
// path rather than an injected fixture.
//
// Two of them cost nothing and always run: whatever the transport is, asking the client its
// version must not read a credential or spawn anything. That property is the one most
// likely to break when a transport is added, because the obvious way to add one resolves
// everything at startup.
//
// The third spends real money and does not run unless asked. It is a test rather than a
// shell command so that what it cost is read from the ledger instead of estimated.

// liveRun describes a session against the real backend.
type liveRun struct {
	Args    []string
	Timeout time.Duration
}

// ARG07 under a live transport. The session ends without a credential read or a subprocess,
// because neither happens until a request needs it -- and --version sends none.
//
// This uses the product default, not an injected fixture. An eager credential read or an
// eager codex --version would make this fail on a machine with no Codex installed, and
// would make it cost a subprocess on every machine.
func TestVersionCostsNothingWithTheRealTransportWired(t *testing.T) {
	exe := nativeAvailable(t)

	for name, args := range map[string][]string{
		"version": {"--version"},
		"help":    {"--help"},
	} {
		t.Run(name, func(t *testing.T) {
			ledger := upstream.NewLedger(upstream.Unlimited())
			var stdout, stderr strings.Builder

			ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
			defer cancel()

			// No StartGateway: the real one, with the real transport behind it.
			result, err := Run(ctx, Options{
				Args:          append(args, "--strict-mcp-config"),
				Env:           isolatedEnv(t),
				Cwd:           t.TempDir(),
				Stdout:        &stdout,
				Stderr:        &stderr,
				Ledger:        ledger,
				ResolveClaude: func() (string, bool, error) { return exe, true, nil },
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.NativeExitCode != 0 {
				t.Fatalf("exit = %d: %s%s", result.NativeExitCode, stdout.String(), stderr.String())
			}
			if result.Attempts != 0 || result.Inferences != 0 {
				t.Fatalf("%s spent %d attempts and %d inferences upstream, want none",
					name, result.Attempts, result.Inferences)
			}
		})
	}
}

// A real session against the user's real Codex subscription.
//
// Skipped unless CLAUDUCT_LIVE=1. Money is not something to spend because someone ran the
// test suite, and a gate that has to be typed is the difference between a decision and an
// accident.
func TestALiveSessionCompletes(t *testing.T) {
	if os.Getenv("CLAUDUCT_LIVE") != "1" {
		t.Skip("live session: set CLAUDUCT_LIVE=1 to spend real requests")
	}
	exe := nativeAvailable(t)

	ledger := upstream.NewLedger(upstream.Unlimited())
	var stdout, stderr strings.Builder

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := Run(ctx, Options{
		// One turn, no tools, a reply short enough to recognise. --strict-mcp-config for
		// the same reason as every other run here: the user's MCP servers are not this
		// test's to start.
		Args: []string{"-p", "Reply with exactly the word: pineapple", "--strict-mcp-config"},
		Env:  isolatedEnv(t),
		Cwd:  t.TempDir(),

		Stdout:        &stdout,
		Stderr:        &stderr,
		Ledger:        ledger,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
	})

	attempts, inferences, refused := ledger.Spent()
	t.Logf("live session: %d attempts, %d inferences, %d refused", attempts, inferences, refused)
	t.Logf("stdout: %q", stdout.String())
	if stderr.Len() > 0 {
		t.Logf("stderr: %q", stderr.String())
	}

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if attempts == 0 {
		t.Fatalf("nothing was sent upstream; the session did not reach the backend")
	}
	if result.NativeExitCode != 0 {
		t.Fatalf("exit = %d", result.NativeExitCode)
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "pineapple") {
		t.Fatalf("the model's answer did not reach the client")
	}
	if result.CleanupErr != nil {
		t.Fatalf("cleanup: %v", result.CleanupErr)
	}
	// The count belongs in the record, not in a guess. A verification run has to say what
	// it cost in the unit it was measured in.
	if result.Attempts != attempts || result.Inferences != inferences {
		t.Fatalf("Result reported %d/%d against the ledger's %d/%d",
			result.Attempts, result.Inferences, attempts, inferences)
	}
}

// isolatedEnv is the parent environment with the Claude config dir redirected, so a live
// run writes its session state somewhere temporary rather than into the user's profile.
func isolatedEnv(t *testing.T) map[string]string {
	t.Helper()
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if i := strings.IndexByte(entry, '='); i > 0 {
			env[entry[:i]] = entry[i+1:]
		}
	}
	env["CLAUDE_CONFIG_DIR"] = t.TempDir()
	return env
}
