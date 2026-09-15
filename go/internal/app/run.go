// Package app assembles a session and owns its lifecycle. It holds no business rule of its
// own: resolution lives in platform, the launch specification in launch, the listener in
// gateway. What it owns is order, and order is the whole safety argument here.
package app

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// ErrClaudeNotFound means the native executable was not located. Nothing was bound and
// nothing was started, so there is nothing to clean up.
var ErrClaudeNotFound = errors.New("CLAUDE_NOT_FOUND")

// RefusedOptionError names a native option this launcher will not forward. Only the
// permission-bypass options qualify; see internal/launch for why the list is two entries
// and not the baseline's thirty.
type RefusedOptionError struct{ Option string }

func (e *RefusedOptionError) Error() string { return "OPTION_REFUSED " + e.Option }

// Process is the part of a running child this package uses. The interface exists so a test
// can supply a child that fails in a chosen way at a chosen moment.
type Process interface {
	Wait() error
	ExitCode() int
}

// Options are the session inputs. Every external dependency is injectable, which is what
// lets the lifecycle tests run without a real claude.exe.
type Options struct {
	Args   []string
	Env    map[string]string
	Cwd    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// ResolveClaude returns the native executable and whether it was found.
	ResolveClaude func() (string, bool, error)
	// StartGateway brings up the loopback listener. Zero uses the real one, with the real
	// transport behind it; a test substitutes a fixture, or a failing one to prove the
	// child is never started without a gateway.
	StartGateway func() (*gateway.Gateway, error)
	// Ledger records what a session spent. Zero allocates an unrestricted one.
	//
	// It is exposed so a caller can read the count rather than estimate it. A verification
	// run has to say exactly what it cost, and "roughly a few requests" is not a number.
	Ledger *upstream.Ledger
	// StartProcess spawns the child described by spec.
	StartProcess func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error)
	// ShutdownTimeout bounds the gateway drain. Zero uses a default.
	ShutdownTimeout time.Duration
}

// Result separates what the native process did from what cleanup did.
//
// These must never collapse into one number. A clean native exit with a failed socket
// release is a different situation from a native crash, and reporting exit code 0 for a
// session that leaked a listener would hide exactly the defect this bridge has to prove it
// does not have.
type Result struct {
	NativeStarted  bool
	NativeExitCode int
	GatewayAddr    string
	CleanupErr     error
	// Attempts and Inferences are what the session spent upstream. One inference retried
	// twice is one inference and three attempts, and a claim about cost needs the unit it
	// was measured in.
	Attempts   int
	Inferences int
}

const defaultShutdownTimeout = 5 * time.Second

// Run starts one session and returns when the native process has exited and everything
// this run owns has been released.
//
// The order is the contract:
//
//	resolve  — a missing executable must not cost a bound port
//	bind     — if the gateway cannot come up, the child is never started (LIFE01)
//	spawn    — if the child cannot start, the listener is released (LIFE02)
//	wait     — the child owns the console for as long as it lives
//	release  — owned listener and connections, and only those (LIFE03)
func Run(ctx context.Context, o Options) (result Result, err error) {
	o = o.withDefaults()

	// First, before anything is resolved or bound. A refused session must not cost an
	// executable lookup or a port, and must not leave either to be cleaned up.
	if option, refused := launch.Refused(o.Args); refused {
		return Result{}, &RefusedOptionError{Option: option}
	}

	exe, found, err := o.ResolveClaude()
	if err != nil {
		return Result{}, err
	}

	if !found {
		return Result{}, ErrClaudeNotFound
	}

	gw, err := o.StartGateway()
	if err != nil {
		// No listener means no endpoint to hand the child. Starting it anyway would
		// point it at the user's real Anthropic endpoint or at nothing at all, so the
		// session ends here and the child is never spawned.
		return Result{}, err
	}
	result = Result{GatewayAddr: gw.Addr()}
	ledger := o.Ledger
	// Named return values, and deliberately: a deferred write to an unnamed one is
	// discarded, so the count would always have been zero.
	defer func() {
		// Read at the end whatever happened, including a failed start: a request that was
		// sent before something went wrong still cost what it cost.
		result.Attempts, result.Inferences, _ = ledger.Spent()
	}()

	spec := launch.Build(exe, o.Args, o.Env, o.Cwd, launch.Overlay{
		BaseURL:   gw.BaseURL(),
		AuthToken: gw.Token(),
	})

	process, startErr := o.StartProcess(spec, o.Stdin, o.Stdout, o.Stderr)
	if startErr != nil {
		// The child never ran, so the port it was going to use must not outlive the
		// attempt. Cleanup failure here is reported alongside the start failure rather
		// than replacing it: the start failure is the cause.
		result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
		return result, startErr
	}
	result.NativeStarted = true

	waitErr := process.Wait()
	result.NativeExitCode = process.ExitCode()

	result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)

	// A non-zero native exit is the native process's answer, not this bridge's error. Only
	// a failure to observe the child at all is returned as an error.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, waitErr
	}
	return result, nil
}

func closeGateway(gw *gateway.Gateway, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return gw.Close(ctx)
}

func (o Options) withDefaults() Options {
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Ledger == nil {
		o.Ledger = upstream.NewLedger(upstream.Unlimited())
	}
	if o.StartGateway == nil {
		// G7: the real transport. Every inference in a session started by this binary now
		// reaches the user's Codex subscription.
		//
		// Nothing is read or spawned here. The credential provider opens auth.json on the
		// first request, and the client version runs codex --version on the first request,
		// so `clauduct-go --version` still costs neither -- which is ARG07 and would break
		// if either were resolved eagerly.
		//
		// The budget is unrestricted, and deliberately: a route is what the client asked
		// for, and a count cap would stop a long session partway through. The verification
		// budget is a separate thing and lives in clauduct-dev probe.
		ledger := o.Ledger
		o.StartGateway = func() (*gateway.Gateway, error) {
			return gateway.Start(upstream.NewDirect(
				&auth.Provider{}, ledger, upstream.InstalledVersion(), "", ""))
		}
	}
	if o.StartProcess == nil {
		o.StartProcess = startOSProcess
	}
	if o.ShutdownTimeout == 0 {
		o.ShutdownTimeout = defaultShutdownTimeout
	}
	return o
}

type osProcess struct{ cmd *exec.Cmd }

func (p *osProcess) Wait() error { return p.cmd.Wait() }

func (p *osProcess) ExitCode() int { return p.cmd.ProcessState.ExitCode() }

// startOSProcess runs the real executable.
//
// No shell. exec.Command is handed an argument slice, never a command line assembled by
// string concatenation, so a value containing & | < > ^ % or a quote is an argument and
// cannot become a command. When stdout is the real *os.File the child inherits that handle
// directly rather than through a pipe, which is what keeps the native TUI, its key
// handling and its Ctrl+C behaviour identical to running claude by hand.
func startOSProcess(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.Command(spec.File, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// SysProcAttr is deliberately left alone. CREATE_NEW_PROCESS_GROUP would stop Ctrl+C
	// from reaching the child, and HideWindow would hide the console the TUI needs.
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osProcess{cmd: cmd}, nil
}
