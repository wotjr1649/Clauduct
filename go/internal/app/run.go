// Package app assembles a session and owns its lifecycle. It holds no business rule of its
// own: resolution lives in platform, the launch specification in launch, the listener in
// gateway. What it owns is order, and order is the whole safety argument here.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// ErrClaudeNotFound means the native executable was not located. Nothing was bound and
// nothing was started, so there is nothing to clean up.
var ErrClaudeNotFound = errors.New("CLAUDE_NOT_FOUND")

// ErrInterrupted means Ctrl+C arrived before the child was started, so nothing was.
var ErrInterrupted = errors.New(CategoryCancelled)

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
	// Stop ends the owned process tree. Windows binds it at creation to a Job
	// whose only handle belongs to this launcher; unrelated processes are excluded.
	Stop() error
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
	// Session is what this session tells the native child about itself, beyond the endpoint
	// and the credential. Empty leaves the child on its own defaults.
	Session map[string]string
	// Settings decides whether this launcher injects options of its own at all.
	//
	// Nil builds the session's settings blob, its delegation menu and its startup --effort.
	// A pointer sends that value as the settings blob and none of the rest, which is what a
	// caller launching something other than the native client wants -- none of them mean
	// anything to it and all would arrive as arguments it does not understand.
	Settings *string
	// Ledger records what a session spent. Zero allocates an unrestricted one.
	//
	// It is exposed so a caller can read the count rather than estimate it. A verification
	// run has to say exactly what it cost, and "roughly a few requests" is not a number.
	Ledger *upstream.Ledger
	// StartProcess spawns the child described by spec.
	StartProcess func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error)
	// ShutdownTimeout bounds the gateway drain. Zero uses a default.
	ShutdownTimeout time.Duration
	// SessionTimeout is an opt-in launcher deadline. Existing invocations have none.
	SessionTimeout time.Duration
	// DeadlineGrace bounds draining an in-flight turn after that deadline.
	DeadlineGrace time.Duration
	// Checkpoint persists metadata while the child runs; nil disables checkpoints.
	Checkpoint func(Status) error
	// BackgroundReady transfers an explicit --bg session to its resident owner.
	BackgroundReady func(BackgroundSession) error
	// interrupts replaces the console's Ctrl+C in a test. Nil subscribes to os.Interrupt.
	interrupts chan os.Signal
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
	// Category is how the session ended, as the exit report names it.
	Category string
	// HookInstalled is whether the session got a hook: since #112 the running executable.
	HookInstalled bool
	// Diagnostics is the final account after in-flight requests have drained.
	Diagnostics gateway.Diagnostics
	Lifecycle   *LifecycleFacts
}

// ExitCodeUnknown is NativeExitCode when the child was never reaped.
//
// It happens when a stopped child does not exit within the grace: the wait is still running
// somewhere, so there is no exit status yet and reading one would be both a data race and an
// answer to a question nobody can answer. Zero would read as success, which it is not.
const ExitCodeUnknown = -1

const defaultShutdownTimeout = 5 * time.Second

// stopGrace is how long a stopped child is given to actually exit.
//
// It is not the shutdown timeout: that one bounds draining the listener, which this build
// controls. This one bounds something it does not control at all, and the answer to a child
// that will not go is to say so rather than to keep waiting.
const stopGrace = 5 * time.Second

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
	forward, userSettings, settingsSlots, err := takeUserSettings(o.Args, o.Cwd)
	if err != nil {
		return Result{}, err
	}
	o.Args = forward

	exe, found, err := o.ResolveClaude()
	if err != nil {
		return Result{}, err
	}

	if !found {
		return Result{}, ErrClaudeNotFound
	}

	// Ctrl+C reaches every process on the console, this one included. With nothing asking
	// for os.Interrupt the Go runtime leaves it to the default handler, which ends the
	// launcher at once: no report, no cleanup, and the job it holds takes the child along
	// (#86). Asked for, it is the child's to answer -- it got the same event -- and nothing
	// is forwarded, so nothing is delivered twice.
	interrupts := o.interrupts
	if interrupts == nil {
		interrupts = make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
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

	// The session values, unless the caller supplied its own set. A test that means to
	// measure one key does not want the other fifteen arriving with it.
	session := o.Session
	if session == nil {
		session = sessionEnvironment()
	}
	// The hook program: this executable, unless the platform cannot name it. Without it the settings
	// carry the picker and nothing else, which is the right answer: a hook pointing at a
	// program that is not there fails on every subagent the client starts.
	// The startup effort rides with them, and is suppressed with them: --effort means
	// nothing to a binary that is not the native client, and would arrive as an argument it
	// does not understand.
	settings, agents, effort := "", "", ""
	var background *sessionlink.Server
	nativePlugin := ""
	nativeCleanupReady := true // No child owns the directory until spawn succeeds.
	defer func() {
		if nativePlugin == "" {
			return
		}
		// Every exit above/below drains the gateway first. Retain the directory
		// if either owner may still be using it; never sweep older session dirs.
		if !nativeCleanupReady || result.CleanupErr != nil {
			result.CleanupErr = errors.Join(result.CleanupErr, errors.New("NATIVE_EVENT_CLEANUP_UNVERIFIED"))
			return
		}
		if removeErr := os.RemoveAll(nativePlugin); removeErr != nil {
			result.CleanupErr = errors.New("NATIVE_EVENT_CLEANUP_FAILED")
		}
	}()
	nativePDF := ""
	hook := ""
	if o.Settings != nil {
		settings = *o.Settings
	} else {
		effort = startupModel.Effort
		// A model named without an effort runs at that model's own default, as the Node
		// launcher did (#87); the startup effort is for the startup model. A user's --effort
		// still lands after this one and wins.
		if model, named := optionValue(o.Args, "--model"); named {
			if _, pinned := optionValue(o.Args, "--effort"); !pinned {
				// Native trims and lowercases a model name before resolving it.
				if route, err := bridge.SelectRoute(strings.ToLower(strings.TrimSpace(model)), ""); err == nil {
					effort = route.Effort
				}
			}
		}
		hook = findHook()
		result.HookInstalled = hook != ""
		if built, ok := sessionSettings(hook); ok {
			settings = built
		}
		if menu, ok := sessionAgents(); ok {
			agents = menu
		}
		if BackgroundRequested(o.Args) {
			background, err = sessionlink.Start(context.Background(), sessionlink.Connection{BaseURL: gw.BaseURL(), Token: gw.Token()})
			if err == nil {
				defer background.Close()
				settings, err = backgroundSettings(settings, hook, background.ID, gw.BaseURL(), o.Env)
			}
			if err != nil {
				result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
				return result, err
			}
		}
	}
	if userSettings != nil {
		if settings == "" {
			settings = "{}"
		}
		settings, err = mergeUserSettings(settings, userSettings)
		if err != nil {
			result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
			return result, err
		}
		for _, slot := range settingsSlots {
			o.Args[slot] = "--settings=" + settings
		}
		settings = "" // Already present at the user's original option boundaries.
	}
	if o.Settings == nil {
		gw.ConfigurePDFRenderer(hook)
		if hook != "" {
			nativePlugin, err = prepareNativeEvents()
			if err != nil {
				result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
				return result, fmt.Errorf("NATIVE_EVENT_SETUP_FAILED")
			}
			gw.ConfigureNativeEvents(filepath.Join(nativePlugin, "receipts"))
			if _, foundErr := exec.LookPath("pdftoppm.exe"); foundErr != nil {
				nativePDF, err = prepareNativePDF(nativePlugin, hook)
				if err != nil {
					result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
					return result, fmt.Errorf("NATIVE_PDF_SETUP_FAILED")
				}
			}
		}
	}
	spec := launch.Build(exe, o.Args, o.Env, o.Cwd, launch.Overlay{
		BaseURL:   gw.BaseURL(),
		AuthToken: gw.Token(),
		Session:   session,
		Effort:    effort,
		Enforced:  sessionRequirements(),
		Settings:  settings,
		Agents:    agents,
	})
	if nativePlugin != "" {
		spec.Args = append([]string{"--plugin-dir", nativePlugin}, spec.Args...)
		filtered := spec.Env[:0]
		pathAdded := false
		for _, value := range spec.Env {
			key, _, _ := strings.Cut(value, "=")
			if nativePDF != "" && strings.EqualFold(key, "PATH") {
				_, old, _ := strings.Cut(value, "=")
				filtered = append(filtered, key+"="+nativePDF+string(os.PathListSeparator)+old)
				pathAdded = true
				continue
			}
			if !strings.EqualFold(key, "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS") {
				filtered = append(filtered, value)
			}
		}
		if nativePDF != "" && !pathAdded {
			filtered = append(filtered, "PATH="+nativePDF)
		}
		spec.Env = append(filtered, "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1")
	}
	configDir := ""
	for _, entry := range spec.Env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "CLAUDE_CONFIG_DIR") {
			configDir = value
		}
	}
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".claude")
		}
	}
	if configDir != "" {
		if !filepath.IsAbs(configDir) {
			configDir = filepath.Join(o.Cwd, configDir)
		}
		gw.ConfigureDelegations(filepath.Join(configDir, "projects"))
		var workflowDirs []string
		for dir := o.Cwd; dir != "" && len(workflowDirs) < 63; dir = filepath.Dir(dir) {
			workflowDirs = append(workflowDirs, filepath.Join(dir, ".claude", "workflows"))
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
				break
			}
		}
		gw.ConfigureWorkflowSources(append(workflowDirs, filepath.Join(configDir, "workflows")))
		if nativePDF != "" {
			filtered := spec.Env[:0]
			for _, entry := range spec.Env {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.EqualFold(key, "CLAUDUCT_PDF_PROJECTS_ROOT") {
					filtered = append(filtered, entry)
				}
			}
			spec.Env = append(filtered, "CLAUDUCT_PDF_PROJECTS_ROOT="+filepath.Join(configDir, "projects"))
		}
		cli := sessionCLIRoles(o.Args, agents, o.Cwd)
		gw.ConfigureRoleDefaults(func(role string, parent bridge.Route) (bridge.Route, bool, error) {
			return sessionRoleSources(configDir, o.Cwd, cli, o.Env, role).resolve(role, parent)
		})
	}

	// Pressed before there was a child to receive it: the launch is what was cancelled, and
	// nothing is started only to be stopped again.
	select {
	case <-interrupts:
		result.Diagnostics = gw.Diagnose()
		result.Category = CategoryCancelled
		result.NativeExitCode = ExitCodeUnknown
		result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
		return result, ErrInterrupted
	default:
	}
	var dispatchOutput backgroundOutput
	stdout, stderr := o.Stdout, o.Stderr
	if background != nil {
		stdout, stderr = &dispatchOutput, &dispatchOutput
	}
	process, startErr := o.StartProcess(spec, o.Stdin, stdout, stderr)
	if startErr != nil {
		// The child never ran, so the port it was going to use must not outlive the
		// attempt. Cleanup failure here is reported alongside the start failure rather
		// than replacing it: the start failure is the cause.
		result.Diagnostics = gw.Diagnose()
		result.Category = CategoryStartFailed
		// There is no exit status: nothing ran. Leaving the field at zero printed
		// "CLIENT_START_FAILED exit=0" and wrote exitCode 0 into the account, where the
		// check for a bad exit reads != 0 and let it through. The sibling path a dozen
		// lines below already says what this build says about an answer it does not have.
		result.NativeExitCode = ExitCodeUnknown
		result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
		return result, startErr
	}
	result.NativeStarted = true
	nativeCleanupReady = false
	if background != nil {
		backgroundCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		process = &backgroundProcess{Process: process, link: background, ctx: backgroundCtx, cancel: cancel,
			exe: exe, cwd: o.Cwd, config: configDir, env: backgroundControlEnv(spec.Env), output: &dispatchOutput, ready: o.BackgroundReady, stdout: o.Stdout, stderr: o.Stderr}
	}

	waitErr, reaped, lifecycle := waitForSession(ctx, process, gw, o, result, interrupts, printMode(o.Args))
	result.Lifecycle = &lifecycle
	cleanupWaitErr := waitErr
	if joined, ok := waitErr.(interface{ Unwrap() []error }); ok && len(joined.Unwrap()) == 1 {
		cleanupWaitErr = joined.Unwrap()[0]
	}
	_, nativeExit := cleanupWaitErr.(*exec.ExitError)
	nativeCleanupReady = reaped && (waitErr == nil || nativeExit || waitErr == ctx.Err() || waitErr == context.Canceled)
	if reaped {
		result.NativeExitCode = process.ExitCode()
	} else {
		// The wait is still in flight and writing the process state as it finishes.
		// Reading the exit code here is a data race -- found by the race detector on its
		// first run in CI -- and the value would mean nothing anyway.
		result.NativeExitCode = ExitCodeUnknown
	}

	// Reap native first, then drain the gateway before taking its final account.
	// Otherwise a cancellation could still be in flight when status claimed completion.
	result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
	if reaped {
		gw.FinalizeNativeResults()
	}
	result.Diagnostics = gw.Diagnose()
	result.Lifecycle.ObservedAt = time.Now().UTC()
	result.Category = endedAs(ctx, result, ledger)
	if lifecycle.Reason == "user_interrupt" {
		result.Category = CategoryCancelled
		if errors.Is(waitErr, context.Canceled) {
			waitErr = nil // Stopped after the grace; the child's exit code is still the answer.
		}
	}
	if lifecycle.Reason == "session_deadline" {
		result.Category = CategoryDeadline
		// errors.Is, not ==. A cancellation that reached here wrapped -- which is the
		// ordinary shape once it has passed through a layer that annotates it -- kept the
		// error set and the launcher exited 1 for a session that hit its deadline, where the
		// answer is 124.
		if errors.Is(waitErr, context.Canceled) {
			waitErr = nil
		}
	}

	// A non-zero native exit is the native process's answer, not this bridge's error. Only
	// a failure to observe the child at all is returned as an error.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, waitErr
	}
	return result, nil
}

// waitFor waits for the child, or stops it when the caller gives up.
//
// Run took a context from the first version of this package and ignored it: the child was
// waited on unconditionally, so a caller with a deadline had no way to end a session. In
// production nothing noticed, because main passes context.Background. It surfaced when a
// fixture was mutated into answering the same tool call forever and the client looped --
// every test timeout in this package had been decorative until then.
func waitFor(ctx context.Context, process Process) (err error, reaped bool) {
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()

	select {
	case waitErr := <-done:
		return waitErr, true
	case <-ctx.Done():
		stopErr := process.Stop()

		// Reaped so the handles are really released, but not waited on forever. A Stop that
		// did not work would otherwise block here for as long as the child chose to live,
		// which defeats the deadline that got us into this branch -- found by mutating Stop
		// into a no-op and watching the suite hang instead of fail.
		//
		// The wait error from a process that was just killed is expected and says nothing.
		// It is discarded rather than returned because reporting it would make a session
		// the caller ended look like one the child ended: Run treats an ExitError as the
		// native process's own answer.
		select {
		case <-done:
		case <-time.After(stopGrace):
			return fmt.Errorf("the child did not exit within %v of being stopped after %w",
				stopGrace, ctx.Err()), false
		}

		if stopErr != nil {
			return fmt.Errorf("stopping the child after %w: %v", ctx.Err(), stopErr), true
		}
		return ctx.Err(), true
	}
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
		// so `clauduct --version` still costs neither -- which is ARG07 and would break
		// if either were resolved eagerly.
		//
		// The budget is unrestricted, and deliberately: a route is what the client asked
		// for, and a count cap would stop a long session partway through. The verification
		// budget is a separate thing and lives in clauduct --dev --probe.
		ledger := o.Ledger
		o.StartGateway = func() (*gateway.Gateway, error) {
			g, err := gateway.Start(upstream.NewDirect(
				&auth.Provider{}, ledger, upstream.InstalledVersion()))
			if err == nil {
				g.EnableContextPolicy()
			}
			return g, err
		}
	}
	if o.StartProcess == nil {
		o.StartProcess = startOSProcess
	}
	if o.ShutdownTimeout == 0 {
		o.ShutdownTimeout = defaultShutdownTimeout
	}
	if o.DeadlineGrace == 0 {
		o.DeadlineGrace = 3 * time.Minute
	}
	return o
}
