package app

import (
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// LIFE04 and LIFE05: how a session ends when something other than the model ends it.
//
// Nothing here generates a console control event. On Windows a Ctrl+C or a close event goes
// to every process attached to the console, not to a chosen one, so a test that raised one
// would reach whatever else the machine has running -- including the user's own sessions.
// The condition that makes Ctrl+C work is asserted structurally instead, and the part that
// cannot be isolated is recorded as not run rather than quietly skipped.

// LIFE04: Ctrl+C reaches the child because nothing puts it in a group of its own.
//
// CREATE_NEW_PROCESS_GROUP is precisely the flag that would stop a console Ctrl+C from
// reaching the child, and startOSProcess leaves SysProcAttr alone for that reason. This
// asserts the built command rather than the source, so it fails on the flag arriving by any
// route.
//
// What it does not do is press Ctrl+C. Raising one is not addressable to a single process
// on this platform, and the machine this runs on has other sessions on it.
func TestTheChildIsNotPutInItsOwnProcessGroup(t *testing.T) {
	spec := launch.Build("cmd", []string{"/c", "exit", "0"}, map[string]string{},
		os.TempDir(), launch.Overlay{})

	process, err := startOSProcess(spec, nil, io.Discard, io.Discard)
	if err != nil {
		t.Skipf("starting a test process: %v", err)
	}
	defer func() { _ = process.Wait() }()

	p, ok := process.(*osProcess)
	if !ok {
		t.Fatalf("startOSProcess returned %T", process)
	}
	if p.cmd.SysProcAttr == nil {
		return // No attributes at all: the child shares the console and its group.
	}
	if flags := p.cmd.SysProcAttr.CreationFlags; flags&syscall.CREATE_NEW_PROCESS_GROUP != 0 {
		t.Fatalf("the child is started with CREATE_NEW_PROCESS_GROUP (flags %#x). A console "+
			"Ctrl+C would no longer reach it, and this launcher has no signal handling of "+
			"its own to forward one.", flags)
	}
}

// LIFE05: stdin ending is the client's answer, not this launcher's error.
//
// A headless client with nothing on stdin and no prompt says so and exits. That is a
// finished session with a non-zero exit code -- not a failure of the bridge -- and Run has
// to report it that way or a user sees a wrapper error for a thing the wrapper did not do.
func TestStdinEndingIsTheClientsAnswerNotAnError(t *testing.T) {
	exe := nativeAvailable(t)
	fixture := &upstream.Fixture{SSE: measuredStream("unused")}

	var stdout, stderr strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		// No prompt. With stdin already at end the client has nothing to work from.
		Args:          []string{"--strict-mcp-config"},
		Env:           isolatedEnv(t),
		Cwd:           t.TempDir(),
		Stdin:         strings.NewReader(""),
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) },
	})

	// The distinction LIFE05 asks for. A session the client ended returns no error from Run
	// and carries the client's own exit code; a session the caller ended returns one.
	if err != nil {
		t.Fatalf("Run reported an error for a session the client ended by itself: %v", err)
	}
	if result.NativeExitCode == 0 {
		t.Fatalf("exit = 0 with no input and no prompt: %s%s", stdout.String(), stderr.String())
	}
	if fixture.Calls() != 0 {
		t.Fatalf("a session with no input asked the backend %d times", fixture.Calls())
	}
	if result.CleanupErr != nil {
		t.Fatalf("cleanup: %v", result.CleanupErr)
	}

	// And the message the user sees is the client's.
	if combined := stdout.String() + stderr.String(); !strings.Contains(combined, "stdin") {
		t.Fatalf("the client's own explanation did not reach the user: %q", combined)
	}
}

// The other half of the distinction, stated against the same shape: a session the caller
// ended returns an error, and it is not an exit code.
//
// The two live together because the pair is the assertion. Either one alone would pass a
// build that reported everything the same way.
func TestACancelledSessionIsDistinguishableFromOneThatEnded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// ping directly rather than through cmd. A subject with a child of its own leaves that
	// child holding the pipes it inherited, so Wait does not return when the subject is
	// killed and the session ends in the stop grace instead of the deadline -- which this
	// test could not tell apart, because the grace message wraps the deadline and so
	// contains its text. It passed for that reason until a mutation showed it.
	_, err := runOwningCommand(ctx, t, "ping", []string{"-n", "120", "127.0.0.1"}, nil)
	if err == nil {
		t.Fatal("a cancelled session reported no error, so it is indistinguishable from a " +
			"session the client finished")
	}
	// Exactly the deadline. Not a wrapped one: the grace path means the child did not go,
	// which is a different answer and has to read as one.
	if err.Error() != context.DeadlineExceeded.Error() {
		t.Fatalf("err = %q, want exactly %q. Anything else means the session ended some "+
			"other way and this assertion is not testing the cancellation path.",
			err, context.DeadlineExceeded)
	}
}

// And the other shape, measured rather than assumed: a child with a child of its own does
// not finish when it is killed, because the grandchild holds the pipes it inherited. The
// session ends in the stop grace, and says so.
//
// This is the same limit LIFE11 records, arriving through the wait rather than through the
// process table. A cancelled session with a grandchild costs the full grace.
func TestACancelledSessionWithAGrandchildEndsInTheStopGrace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := time.Now()
	_, err := runOwning(ctx, t, nil)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("a cancelled session reported no error")
	}
	if !strings.Contains(err.Error(), "did not exit") {
		t.Skipf("this child went promptly (%v): %v. The grandchild did not hold the pipes, "+
			"so there is no grace to measure.", elapsed, err)
	}
	if elapsed < stopGrace {
		t.Fatalf("the grace was reported after only %v", elapsed)
	}
	t.Logf("measured: a cancelled session whose child has a child of its own ends after %v "+
		"-- the deadline plus the full %v grace, because Wait holds until the inherited "+
		"pipes close.", elapsed.Round(time.Millisecond), stopGrace)
}

// LIFE05's remaining leg, recorded rather than skipped.
//
// Closing the console sends CTRL_CLOSE_EVENT to every process attached to it. There is no
// way to address one process with it, so a test that raised one on this machine would reach
// the user's other sessions -- which is the failure LIFE12 exists to prevent, arriving
// through a different door.
//
// It stays NOT_RUN. Not run is not passed, and the condition for running it is a machine
// with nothing else on it, or a child started in a console of its own -- which would itself
// change the Ctrl+C behaviour LIFE04 depends on.
func TestConsoleCloseIsNotRunHere(t *testing.T) {
	t.Skip("console close cannot be addressed to one process; raising one would reach " +
		"every session on this machine. NOT_RUN, not passed.")
}
