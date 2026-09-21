package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Session 36, C. Where the effort a request carries actually comes from.
//
// Two findings, both measured against the installed client on a fixture backend, with every
// effort-named variable scrubbed from the inherited environment first.
//
// One: the client states an effort on every request whatever the environment says. The
// reasoning recorded on 2026-09-17 was that removing the startup value would leave it sending
// none, SelectRoute would fall through to the catalogue, and every default session would rise
// low -> medium. It never reaches the catalogue. Unlocked and left alone the client picked
// high on its own, so the cost of getting this wrong was larger than what was written down.
//
// Two: CLAUDE_CODE_EFFORT_LEVEL beats --effort, the reverse of ANTHROPIC_MODEL against
// --model. That is why the startup effort moved onto the command line -- see
// launch.Overlay.Effort -- and why a user who sets that name still wins afterwards.
//
// The assertion is the version-independent half: an effort is always stated. The values are
// logged rather than asserted because pinning a client version is against this project's
// rules, and a test that failed when Anthropic shipped a new default would report the wrong
// thing.
func TestTheClientAlwaysStatesAnEffort(t *testing.T) {
	exe := nativeAvailable(t)

	for _, tc := range []struct {
		name    string
		args    []string
		userEnv string
	}{
		{name: "as shipped"},
		{name: "the user states their own on the command line", args: []string{"--effort", "medium"}},
		// Still honoured, and deliberately: someone who sets the pin wants the pin.
		{name: "the user pins the environment name", userEnv: "xhigh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The logged values are only a reading of the client if nothing else in this
			// terminal is naming an effort. Checked rather than assumed: the scrub is what
			// makes the numbers below mean what the comment above says they mean.
			childEnv := withoutEffortNames(isolatedEnv(t))
			for name := range childEnv {
				if upper := strings.ToUpper(name); strings.Contains(upper, "EFFORT") ||
					strings.Contains(upper, "REASONING") {
					t.Fatalf("%s reached the child; the reading would be of this terminal, not the client", name)
				}
			}
			if tc.userEnv != "" {
				childEnv[effortEnv] = tc.userEnv
			}

			_, cwd := workspace(t)
			fixture := &upstream.Fixture{SSE: measuredStream("ok")}
			var stdout, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
			defer cancel()

			result, err := Run(ctx, Options{
				Args:          append(tc.args, "-p", "hi", "--strict-mcp-config", "--bare"),
				Env:           childEnv,
				Cwd:           cwd,
				Stdout:        &stdout,
				Stderr:        &stderr,
				ResolveClaude: func() (string, bool, error) { return exe, true, nil },
				StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) },
			})
			if err != nil {
				t.Fatalf("Run: %v (stderr=%s)", err, stderr.String())
			}

			routed := 0
			for _, entry := range result.Diagnostics.Recent {
				if entry.Model == "" {
					continue
				}
				routed++
				t.Logf("effort=%-7s source=%-16s requested=%s", entry.Effort, entry.Source, entry.Requested)

				// The first finding. "+effort" is SelectRoute's word for "the request named
				// one", so its presence says the catalogue default was never consulted --
				// which is what unlocking was expected to fall back to.
				if !strings.Contains(entry.Source, "+effort") {
					t.Errorf("source=%q: the client sent no effort and the catalogue default decided this request",
						entry.Source)
				}
				if entry.Effort == "" {
					t.Errorf("record carries no effort: %+v", entry)
				}
			}
			if routed == 0 {
				t.Fatalf("no request was routed (received=%d); nothing was measured",
					result.Diagnostics.Requests.Received)
			}
		})
	}
}

// withoutEffortNames drops every inherited name that could be a second source of effort, so
// a reading of what the client does on its own is not a reading of this terminal.
func withoutEffortNames(env map[string]string) map[string]string {
	for name := range env {
		if upper := strings.ToUpper(name); strings.Contains(upper, "EFFORT") ||
			strings.Contains(upper, "REASONING") {
			delete(env, name)
		}
	}
	return env
}

// C, the fix. The startup effort has to be a default the session can move, not a lock.
//
// Measured 2026-09-18, and the reason this moved off the environment: CLAUDE_CODE_EFFORT_LEVEL
// beats --effort on the command line, which is the opposite of ANTHROPIC_MODEL's relationship
// to --model. A name that wins over an explicit flag wins over the client's own picker too,
// which is why the first real session sent effort=low on all 182 requests after the user had
// chosen high.
//
// The Node baseline had this right and the rewrite lost it: v1 passed --model and --effort on
// the command line (src/clauduct.mjs:213) and set CLAUDE_CODE_EFFORT_LEVEL in exactly one
// place -- under --verify-model-route (src/clauduct.mjs:192), the mode whose whole purpose is
// to pin the route so a verification run cannot drift. v2 made the pin the default.
func TestTheStartupEffortIsADefaultRatherThanAPin(t *testing.T) {
	if value, pinned := sessionEnvironment()[effortEnv]; pinned {
		t.Errorf("the session sets %s=%q; it beats --effort and the picker, so a session "+
			"started this way cannot change effort for as long as it runs", effortEnv, value)
	}

	spec := launchSpec(t, nil)
	effort := indexOfArg(spec.Args, "--effort")
	if effort < 0 {
		t.Fatalf("no --effort in the launch spec: %v", optionsOf(spec.Args))
	}
	if spec.Args[effort+1] != startupModel.Effort {
		t.Errorf("--effort %s, want the startup default %s", spec.Args[effort+1], startupModel.Effort)
	}
}

// Ahead of the forwarded arguments, for the same reason --agents is: the client takes the
// last one, so a user who states their own effort replaces this build's rather than fighting
// it. The same shape, so it fails the same way if the client's precedence ever changes.
func TestTheUsersOwnEffortLandsAfterThisBuilds(t *testing.T) {
	spec := launchSpec(t, []string{"--effort", "max"})
	ours, theirs := indexOfArg(spec.Args, "--effort"), lastIndexOfArg(spec.Args, "--effort")
	if ours == theirs {
		t.Fatalf("only one --effort in %v; this build's was not added", optionsOf(spec.Args))
	}
	if spec.Args[theirs+1] != "max" {
		t.Errorf("the user's --effort is not the last one: %v", optionsOf(spec.Args))
	}
}

// launchSpec is the spec this build would start the child with, without starting one.
func launchSpec(t *testing.T, args []string) launch.Spec {
	t.Helper()
	var captured launch.Spec
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()
	_, err := Run(ctx, Options{
		Args: args, Env: withoutEffortNames(isolatedEnv(t)), Cwd: t.TempDir(),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		ResolveClaude: func() (string, bool, error) { return "claude", true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(&upstream.Fixture{}) },
		StartProcess: func(spec launch.Spec, _ io.Reader, _, _ io.Writer) (Process, error) {
			captured = spec
			return nil, errors.New("not started: this test wants the spec, not a child")
		},
	})
	if err == nil {
		t.Fatal("the child was started; this test must not start one")
	}
	if captured.File == "" {
		t.Fatal("no spec was built")
	}
	return captured
}

func indexOfArg(args []string, want string) int {
	for i, arg := range args {
		if arg == want && i+1 < len(args) {
			return i
		}
	}
	return -1
}

func lastIndexOfArg(args []string, want string) int {
	found := -1
	for i, arg := range args {
		if arg == want && i+1 < len(args) {
			found = i
		}
	}
	return found
}

// optionsOf is the option names in a spec, without the blobs their values are. A failure
// that pastes the whole agent menu into the terminal buries what it is reporting.
func optionsOf(args []string) []string {
	var names []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			names = append(names, arg)
		}
	}
	return names
}
