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
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"github.com/wotjr1649/Clauduct/go/internal/settingsfile"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// ErrClaudeNotFound means the native executable was not located. Nothing was bound and
// nothing was started, so there is nothing to clean up.
var ErrClaudeNotFound = errors.New("CLAUDE_NOT_FOUND")

// ErrNoUpstream means Run was given no StartGateway or no ModelList. It is raised before
// anything is read, resolved or bound.
var ErrNoUpstream = errors.New("NO_UPSTREAM: Options.StartGateway and Options.ModelList are required")

// ErrInterrupted means Ctrl+C arrived before the child was started, so nothing was.
var ErrInterrupted = errors.New(CategoryCancelled)

// Desktop dispatch leaves the terminal bridge and uses a separately configured backend.
var ErrDesktopUnsupported = errors.New("DESKTOP_UNSUPPORTED: --desktop opens a separate app outside this bridge; use a native terminal session")
var ErrDesktopBoundary = errors.New("DESKTOP_OPTION_UNVERIFIED: unknown native option hides whether --desktop is an option or a value; use unambiguous terminal arguments")

// ErrBackgroundResumeTarget refuses a background resume whose target is not a session UUID.
var ErrBackgroundResumeTarget = errors.New("BACKGROUND_RESUME_TARGET_UNVERIFIED: --resume with --bg needs the session's full id (as claude agents --json prints it); native copies the conversation for any other value and that copy never reaches this session's gateway. Nothing was started")

// ErrBackgroundResumeNeedsFork refuses --resume <id> --bg without --fork-session.
var ErrBackgroundResumeNeedsFork = errors.New("BACKGROUND_RESUME_IS_A_COPY: a background resume through Clauduct starts a copy of the session, not the session itself (native copies whenever flags are passed, and Clauduct passes them); add --fork-session to start that copy as a new session, leaving the original unchanged. Nothing was started")

// ErrBackgroundContinue refuses --continue with --bg.
var ErrBackgroundContinue = errors.New("BACKGROUND_CONTINUE_UNVERIFIED: --continue with --bg starts a copy of a session this launcher cannot name, which its gateway refuses; use --resume <session id> --bg. Nothing was started")

// ErrAccountCommand refuses a native Claude account subcommand before anything starts.
var ErrAccountCommand = errors.New("ACCOUNT_COMMAND_REFUSED: auth login, auth logout and setup-token manage a Claude account, which Clauduct does not sign in to or out of; run them in standalone Claude Code. Nothing was started")

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
	// ClauductHome overrides the home directory for isolated local checks.
	ClauductHome string

	// ResolveClaude returns the native executable and whether it was found.
	ResolveClaude func() (string, bool, error)
	// StartGateway brings up the loopback listener with the transport behind it. Required:
	// Run builds no transport of its own, so zero is ErrNoUpstream. The product passes the
	// real one (cmd/clauduct); the probe and tests pass synthetic ones.
	StartGateway func() (*gateway.Gateway, error)
	// ModelList fixes the session's account model list before native starts. Required for the
	// same reason; the product's reads the account through the transport StartGateway
	// serves and falls back to that account's last good list (AccountModelList).
	ModelList func(ctx context.Context, home string) (*bridge.Catalogue, ModelListFacts, error)
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
	// ManagedPolicy reads the machine's managed settings sources. Zero reads the real ones.
	ManagedPolicy func() ([]policyDocument, error)
	// BackgroundReady transfers an explicit --bg session to its resident owner.
	BackgroundReady func(BackgroundSession) error
	// Regenerate serves an existing background job again (#319 decision 18c): no native
	// process starts, and StartGateway must bind the record's port.
	Regenerate *ResidentRecord
	// Handover is offered the native exit code when an agent view session ends with jobs still
	// on its link (#319 decision 18h). True means the terminal has been given back; even if the
	// launcher is gone, this process keeps the gateway and link until those jobs end.
	Handover func(nativeExit int) bool
	// ProviderStrategy is how native is held to this session's gateway (#320); empty is the
	// launch without one. An unknown name is refused.
	ProviderStrategy string
	// AdmittedNativeSHA256 is the native the admission gate admitted (admission r2 cache.use):
	// the session starts only that file, held against change until it is running.
	AdmittedNativeSHA256 string
	// backgroundUnderStrategy lets a probe run the unverified background path under a strategy.
	backgroundUnderStrategy bool
	// foregroundCheckpoint persists newly dispatched jobs before their first gateway request.
	foregroundCheckpoint func() error
	// interrupts replaces the console's Ctrl+C in a test. Nil subscribes to os.Interrupt.
	interrupts chan os.Signal
	// afterNative, set only by a test, runs once native has ended and before the gateway closes;
	// the session link is still open. Decision 18 experiments (X2, X3) hold a foreground session's
	// gateway here to see what native's background path does while it lives. The product never
	// sets it.
	afterNative func()
	// nativeIdentityOff, set only by a test that relays native's requests through its own
	// recorder, leaves native identity unrequired: the relay, not native, opens the connections.
	nativeIdentityOff bool
	// backgroundCrash, set only by a test, makes a background resident end as if its process
	// died: the native job is not stopped (decision 18c X7).
	backgroundCrash <-chan struct{}
}

// Result separates what the native process did from what cleanup did.
//
// These must never collapse into one number. A clean native exit with a failed socket
// release is a different situation from a native crash, and reporting exit code 0 for a
// session that leaked a listener would hide exactly the defect this bridge has to prove it
// does not have.
type Result struct {
	Startup               bridge.Pair
	StartupModelSource    string
	StartupEffortSource   string
	Context               ContextFacts
	ClassifierModel       bridge.Pair
	ClassifierModelSource string
	ClassifierFinding     string
	ModelList             ModelListFacts
	DeprecatedSettings    []string
	// MenuEntries counts the delegation menu, the inherit entry included.
	MenuEntries    int
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
	// NativeReplaced is whether claude.exe changed on disk while this session ran:
	// an updater ran meanwhile, from this native or another process (#229).
	NativeReplaced bool
	// HandedOver names the jobs an agent view session's gateway stayed for after native ended.
	HandedOver []string
	// Diagnostic identity of the native auth query; never an inference identity.
	nativeProviderQuery childprocess.ProcessIdentity
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
	// First of all: a session without an upstream it was given has none. Run never builds a
	// credential provider or a Direct transport itself (#319), so what a caller forgot to
	// pass cannot become the user's real account.
	if o.StartGateway == nil || o.ModelList == nil {
		return Result{}, ErrNoUpstream
	}
	o = o.withDefaults()
	launchArgs := o.Args // as given: a background resident records them for its regeneration

	// First, before anything is resolved or bound. A refused session must not cost an
	// executable lookup or a port, and must not leave either to be cleaned up.
	if option, refused := launch.Refused(o.Args); refused {
		return Result{}, &RefusedOptionError{Option: option}
	}
	if err := nativeDesktop(o.Args); err != nil {
		return Result{}, err
	}
	if err := accountCommand(o.Args); err != nil {
		return Result{}, err
	}
	providerFlag, err := strategyFlag(o.ProviderStrategy)
	if err != nil {
		return Result{}, err
	}
	if providerFlag != "" && !o.backgroundUnderStrategy {
		// Under a strategy only the verified launch forms start (admission scopes, #320);
		// background entry is one of the refusals.
		if _, err := admissionScope(o.Args); err != nil {
			return Result{}, err
		}
	}
	clauductHome := o.ClauductHome
	information := nativeInformation(o.Args)
	if clauductHome == "" && !information {
		clauductHome, _ = os.UserHomeDir()
	}
	if clauductHome != "" && o.Settings == nil && !information {
		defer func() { reportForegroundEventsCleanup(o.Stderr, collectForegroundEvents(clauductHome)) }()
	}
	if command := nativeControl(o.Args); command != "" && o.Regenerate == nil {
		return runNativeControl(ctx, o, command)
	}
	config := defaultClauductSettings()
	// Before anything is fetched or bound: a machine policy that can move model requests off
	// the gateway is one this launcher cannot outrank (#295). Help and version send none.
	if !information {
		docs, readErr := o.ManagedPolicy()
		if err := managedRouting(docs, readErr); err != nil {
			return Result{}, err
		}
		if providerFlag != "" || !BackgroundRequested(o.Args) && !o.agentView() {
			// The foreground's agent-view restriction holds against machine policy too.
			if err := managedAgentViewOverride(docs); err != nil {
				return Result{}, err
			}
		}
	}
	if o.Settings == nil && !information {
		if clauductHome == "" {
			return Result{}, errClauductSettings
		}
		// An updater older than this build never runs its settings sync, so the first
		// normal start adds the top-level keys this build's defaults have and the file
		// lacks. A file that is invalid is left exactly as it is and refused below.
		// A sync that cannot write (a held file, a busy lock, a disk error) is reported, never
		// shown as done, and does not stop the session: an omitted key still takes its
		// factory value at run time, and the next start tries again.
		synced, syncErr := settingsfile.Sync(clauductHome)
		if syncErr != nil && !errors.Is(syncErr, settingsfile.ErrSyncInvalid) && !errors.Is(syncErr, settingsfile.ErrSyncVersion) {
			fmt.Fprintf(o.Stderr, "clauduct: CLAUDUCT_SETTINGS_SYNC_FAILED: %v; continuing with the file as it is, and the next start tries again\n", syncErr)
		}
		if len(synced.Added) > 0 {
			fmt.Fprintf(o.Stderr, "clauduct: settings: added %s; previous file kept as %s\n", strings.Join(synced.Added, ", "), synced.Backup)
		}
		// Context is global launch configuration, including UUID resume. Only
		// model/effort preferences are subsequently restored from the snapshot.
		config, err = loadClauductSettings(clauductHome)
		if err != nil {
			return Result{}, err
		}
		if len(config.Deprecated) > 0 {
			fmt.Fprintf(o.Stderr, "clauduct: settings: %s no longer applied since v0.6.4; the keys stay in the file and may be removed\n", strings.Join(config.Deprecated, ", "))
		}
		if config, err = config.strictReady(o.Env); err != nil {
			return Result{}, err
		}
	}
	forward, userSettings, settingsSlots, err := config.takeUserSettings(o.Args, o.Cwd)
	if err != nil {
		return Result{}, err
	}
	if !information {
		if err := checkParentRouting(forward); err != nil {
			return Result{}, err
		}
	}
	if providerFlag != "" || !BackgroundRequested(o.Args) && !o.agentView() {
		// The foreground's agent-view restriction is this launch's, as its routing is: a
		// settings env that turns agent view back on would reopen native's supervisor path.
		if err := userKeepsAgentViewOff(userSettings); err != nil {
			return Result{}, err
		}
	}
	forward, resumeID := prepareSessionArgs(forward)
	forkedFrom := ""
	releaseResident := func() {}
	residentLocked := false
	defer func() { releaseResident() }()
	if clauductHome != "" && o.Settings == nil && !information {
		defer func() {
			outcomes := collectResidents(clauductHome)
			if !BackgroundRequested(launchArgs) && len(result.HandedOver) == 0 {
				reportResidentCleanup(o.Stderr, outcomes)
			}
		}()
	}
	if o.Regenerate != nil {
		// The job's own session comes back as a resume, never a fork of it.
		if resumeID, err = regenerateCheck(o, clauductHome); err != nil {
			return Result{}, err
		}
		if releaseResident, err = lockResident(clauductHome, o.Regenerate.Link, 2*time.Second); err != nil {
			releaseResident = func() {}
			return Result{}, err
		}
		residentLocked = true
		current, readErr := LoadResident(clauductHome, o.Regenerate.Link)
		if readErr != nil || current.Job != o.Regenerate.Job || current.PluginDir != o.Regenerate.PluginDir {
			return Result{}, errors.New("RESIDENT_RECORD_INVALID")
		}
		o.Regenerate = &current
		resumeID = current.Session
	} else if BackgroundRequested(forward) {
		if forkedFrom, err = backgroundResume(forward, resumeID); err != nil {
			return Result{}, err
		}
	}
	o.Args = forward
	// One account list per session, fixed before native starts and shared by display,
	// routing, Agent schemas and native hooks. Help and version never fetch one.
	var catalogue *bridge.Catalogue
	var classifierFinding string // printed only for a pair the account offers; status says the same
	var listFacts ModelListFacts
	if !information {
		catalogue, listFacts, err = o.ModelList(ctx, clauductHome)
		if err != nil {
			return Result{}, err
		}
		config.Selection = config.Selection.WithCatalogue(catalogue)
		listFacts.Problems = config.Selection.Problems()
		if config.ClassifierModel != (bridge.Pair{}) && !config.Selection.ValidPair(config.ClassifierModel) {
			listFacts.Problems = append(listFacts.Problems, "classifier_model: "+config.ClassifierModel.Model+"/"+config.ClassifierModel.Effort+" is not offered by the account")
		} else if classifierFinding = classifierFindingOf(config.ClassifierModel); classifierFinding != "" {
			fmt.Fprintf(o.Stderr, "clauduct: %s\n", classifierFinding)
		}
		if listFacts.Source == "last-good" {
			fmt.Fprintf(o.Stderr, "clauduct: the account model list could not be fetched (%s); using this account's list from %s\n", listFacts.FetchFailure, listFacts.FetchedAt)
		}
		for _, problem := range listFacts.Problems {
			fmt.Fprintf(o.Stderr, "clauduct: settings: %s; kept as written, refused only when selected\n", problem)
		}
	}

	exe, found, err := o.ResolveClaude()
	if err != nil {
		return Result{}, err
	}

	if !found {
		return Result{}, ErrClaudeNotFound
	}
	nativeBefore, _ := os.Stat(exe)
	defer func() { result.NativeReplaced = nativeReplaced(exe, nativeBefore) }()
	if !information && o.Settings == nil && o.Regenerate == nil && providerFlag == "" && nativeBefore != nil && strings.EqualFold(filepath.Base(exe), "claude.exe") {
		var query childprocess.ProcessIdentity
		defer func() { result.nativeProviderQuery = query }()
		if err := checkNativeProvider(ctx, exe, o, &query); err != nil {
			return Result{Category: CategoryRouting, NativeExitCode: ExitCodeUnknown}, err
		}
	}
	var profiles *gateway.SessionProfiles
	var resumed *gateway.SessionProfile
	_, forkSession := optionValue(o.Args, "--fork-session")
	forkSession = forkSession && o.Regenerate == nil
	_, ephemeral := optionValue(o.Args, "--no-session-persistence")
	if clauductHome != "" && !nativeInformation(o.Args) && o.Settings == nil && (!ephemeral || resumeID != "") {
		profiles, err = gateway.OpenSessionProfiles(filepath.Join(clauductHome, ".clauduct", "sessions"))
		if err != nil {
			return Result{}, err
		}
		defer func() { result.CleanupErr = errors.Join(result.CleanupErr, profiles.Close()) }()
		if resumeID != "" {
			saved, loadErr := profiles.Load(resumeID)
			switch {
			case errors.Is(loadErr, os.ErrNotExist):
				// No snapshot: retain today's settings, including the startup pair.
			case loadErr != nil:
				return Result{}, loadErr
			default:
				saved.Selection = saved.Selection.WithCatalogue(catalogue)
				if err := profiles.Refresh(&saved, filepath.Join(nativeConfigDirectory(o.Env, o.Cwd), "projects"), true, saved.Selection); err != nil {
					return Result{}, err
				}
				config.Selection, config.Startup, resumed = saved.Selection, saved.Last, &saved
				config.StartupSource, config.SelectionSource = "session-snapshot.last", "session-snapshot"
			}
		}
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
	if o.Regenerate != nil && residentPort(gw.Addr()) != o.Regenerate.Port {
		// The job's settings name the recorded port; any other serves nobody.
		return Result{}, errors.Join(errors.New("RESIDENT_PORT_MISMATCH"), closeGateway(gw, o.ShutdownTimeout))
	}
	if o.Settings == nil {
		if image := filepath.Base(exe); strings.EqualFold(image, "claude.exe") && !o.nativeIdentityOff {
			gw.RequireNativeIdentity(image)
		}
	}
	gw.ConfigureContextPolicy(config.ContextPolicy)
	gw.ConfigureClassifierModel(config.ClassifierModel)
	gw.ExpectTools(expectedTools(o.Args, upstream.ReferenceCoreTools))
	result = Result{GatewayAddr: gw.Addr(), Startup: config.Startup, Context: config.contextFacts(),
		ClassifierModel: config.ClassifierModel, ClassifierModelSource: config.ClassifierModelSource, ClassifierFinding: classifierFinding,
		ModelList: listFacts, DeprecatedSettings: config.Deprecated}
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
		session = config.sessionEnvironment()
	}
	// The hook program: this executable, unless the platform cannot name it. Without it the settings
	// carry the picker and nothing else, which is the right answer: a hook pointing at a
	// program that is not there fails on every subagent the client starts.
	// The startup effort rides with them, and is suppressed with them: --effort means
	// nothing to a binary that is not the native client, and would arrive as an argument it
	// does not understand.
	settings, agents, effort := "", "", ""
	var background, foreground *sessionlink.Server
	nativePlugin := ""
	nativeCleanupReady := true // No child owns the directory until spawn succeeds.
	// A regeneration serves a running job's directory: until it is serving, no exit removes it.
	retainPlugin := o.Regenerate != nil
	var resident *backgroundProcess
	defer func() {
		if retainPlugin || nativePlugin == "" && (resident == nil || resident.record == nil) {
			return
		}
		// Every exit above/below drains the gateway first. Retain the directory
		// if either owner may still be using it; older directories have a separate owner check.
		if !nativeCleanupReady || result.CleanupErr != nil {
			result.CleanupErr = errors.Join(result.CleanupErr, errors.New("NATIVE_EVENT_CLEANUP_UNVERIFIED"))
			return
		}
		if resident != nil && resident.recordStays() && !resident.unlisted {
			return
		}
		if nativePlugin != "" {
			if removeErr := removeEventsDir(nativePlugin); removeErr != nil {
				result.CleanupErr = errors.New("NATIVE_EVENT_CLEANUP_FAILED")
				return
			}
		}
		if resident != nil && resident.unlisted && resident.record != nil {
			if reclaimInterrupt != nil {
				reclaimInterrupt()
			}
			if removeResident(resident.home, resident.record.Link) != nil {
				result.CleanupErr = errors.New("NATIVE_EVENT_CLEANUP_FAILED")
			}
		}
	}()
	nativePDF := ""
	hook := ""
	if o.Settings != nil {
		settings = *o.Settings
	} else {
		effort = config.Startup.Effort
		// A model named without an effort runs at that model's own default, as the Node
		// launcher did (#87); the startup effort is for the startup model. A user's --effort
		// still lands after this one and wins.
		if model, named := optionValue(o.Args, "--model"); named {
			if _, pinned := optionValue(o.Args, "--effort"); !pinned {
				// Native trims and lowercases a model name before resolving it.
				// No effort determinable for it: pass none rather than the startup effort,
				// so the start is refused below instead of running on an effort nobody chose.
				effort = ""
				if route, err := config.Selection.SelectRoute(strings.ToLower(strings.TrimSpace(model)), ""); err == nil {
					effort = route.Effort
				}
			}
		}
		hook = findHook()
		result.HookInstalled = hook != ""
		if built, ok := config.sessionSettings(hook); ok {
			settings = built
		}
		if settings != "" && !BackgroundRequested(o.Args) {
			env := routingSettingsEnv(o.Env, gw.BaseURL())
			for name, value := range config.sessionRequirements() {
				env[name] = value
			}
			if providerFlag != "" {
				env[providerFlag] = "1"
			}
			env[agentViewOff] = "1"
			// The hooks and the confirmation helper read this session's connection from its
			// link, held until native and its hooks are done: the deferred close runs after
			// the native process tree has ended.
			if hook != "" {
				foreground, err = sessionlink.StartForeground(context.Background(), sessionlink.Connection{BaseURL: gw.BaseURL(), Token: gw.Token()})
				if err != nil {
					err = errors.New("SESSION_LINK_UNAVAILABLE: the session's hook connection could not be prepared")
				} else {
					defer foreground.Close()
					link := foreground.ID
					if providerFlag != "" {
						// The hook may then rely on native's provider control (hookcmd).
						link += " " + hookcmd.ProviderControlledArg
					}
					if o.agentView() {
						settings, err = config.backgroundSettings(settings, hook, foreground.ID, gw.BaseURL(), o, providerFlag)
					} else {
						settings, err = foregroundSettings(settings, link, env)
					}
				}
			} else {
				if o.agentView() {
					err = errors.New("SESSION_LINK_UNAVAILABLE: the session's key helper could not be prepared")
				} else {
					settings, err = foregroundSettings(settings, "", env)
				}
			}
			if err != nil {
				result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
				return result, err
			}
		}
		if menu, ok := config.sessionAgents(); ok {
			agents = menu
			result.MenuEntries = len(config.agentDefinitions())
		}
		if BackgroundRequested(o.Args) {
			connection := sessionlink.Connection{BaseURL: gw.BaseURL(), Token: gw.Token()}
			if o.Regenerate != nil {
				err = restoreResidentProof(gw, o.Regenerate.Link, *o.Regenerate)
				if err == nil {
					background, err = sessionlink.Restart(context.Background(), o.Regenerate.Link, connection)
				}
			} else {
				background, err = sessionlink.Start(context.Background(), connection)
			}
			if err == nil {
				defer background.Close()
				if !residentLocked && clauductHome != "" {
					if release, lockErr := lockResident(clauductHome, background.ID, 0); lockErr == nil {
						releaseResident, residentLocked = release, true
					}
				}
				settings, err = config.backgroundSettings(settings, hook, background.ID, gw.BaseURL(), o, providerFlag)
			}
			if err != nil {
				result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
				return result, err
			}
		}
	}
	if o.Settings == nil {
		gw.RequireNativeConfirmations()
		gw.ConfigurePDFRenderer(hook)
		if hook != "" {
			link := ""
			if background != nil {
				link = background.ID
			} else if foreground != nil {
				link = foreground.ID
			}
			at := ""
			if o.Regenerate != nil {
				at = o.Regenerate.PluginDir
			}
			nativePlugin, err = prepareNativeEventsAt(at, link, config.Selection.Catalogue())
			if err == nil && o.Regenerate == nil && !BackgroundRequested(launchArgs) {
				err = writeEventsOwner(nativePlugin)
			}
			if err != nil {
				result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
				return result, fmt.Errorf("NATIVE_EVENT_SETUP_FAILED")
			}
			gw.ConfigureNativeEvents(filepath.Join(nativePlugin, "receipts"))
			// The alias goes on a new native's PATH; the job a regenerated resident serves has its own.
			if _, foundErr := exec.LookPath("pdftoppm.exe"); foundErr != nil && o.Regenerate == nil {
				nativePDF, err = prepareNativePDF(nativePlugin, hook)
				if err != nil {
					result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
					return result, fmt.Errorf("NATIVE_PDF_SETUP_FAILED")
				}
			}
		}
	}
	configDir := nativeConfigDirectory(o.Env, o.Cwd)
	if userSettings != nil {
		if settings == "" {
			settings = "{}"
		}
		settings, err = mergeUserSettings(settings, userSettings)
		if err != nil {
			result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
			return result, err
		}
	}
	if o.agentView() && nativePlugin != "" {
		settings, err = config.nativeEventSettings(settings, nativePDF, configDir, o, gw.BaseURL())
		if err != nil {
			result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
			return result, err
		}
	}
	finalSettings := settings
	roleArgs := append([]string(nil), o.Args...)
	if userSettings != nil {
		for _, slot := range settingsSlots {
			o.Args[slot] = "--settings=" + settings
		}
		settings = "" // Already present at the user's original option boundaries.
	}
	overlay := launch.Overlay{
		BaseURL:   gw.BaseURL(),
		AuthToken: gw.Token(),
		Session:   session,
		Effort:    effort,
		Enforced:  config.sessionRequirements(),
		Settings:  settings,
		Agents:    agents,
	}
	if o.agentView() {
		overlay.BaseURL, overlay.AuthToken = "", ""
		overlay.Session, overlay.Enforced = nil, nil
	}
	spec := launch.Build(exe, o.Args, o.Env, o.Cwd, overlay)
	if nativePlugin != "" {
		spec.Args = append([]string{"--plugin-dir", nativePlugin}, spec.Args...)
	}
	if nativePlugin != "" && !o.agentView() {
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
	if configDir != "" {
		gw.ConfigureDelegations(filepath.Join(configDir, "projects"))
		var workflowDirs []string
		for dir := o.Cwd; dir != "" && len(workflowDirs) < 63; dir = filepath.Dir(dir) {
			workflowDirs = append(workflowDirs, filepath.Join(dir, ".claude", "workflows"))
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
				break
			}
		}
		gw.ConfigureWorkflowSources(append(workflowDirs, filepath.Join(configDir, "workflows")))
		if nativePDF != "" && !o.agentView() {
			filtered := spec.Env[:0]
			for _, entry := range spec.Env {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.EqualFold(key, "CLAUDUCT_PDF_PROJECTS_ROOT") {
					filtered = append(filtered, entry)
				}
			}
			spec.Env = append(filtered, "CLAUDUCT_PDF_PROJECTS_ROOT="+filepath.Join(configDir, "projects"))
		}
		cli := sessionCLIRoles(roleArgs, agents, o.Cwd)
		if o.agentView() {
			cli.readFlagEnv([]byte(finalSettings))
		}
		cli.subagent = sync.OnceValues(func() (string, error) { return subagentModel(configDir, o.Cwd, managedRoot(), cli, o.Env) })
		gw.ConfigureRoleDefaults(func(role string, parent bridge.Route) (bridge.Route, bool, error) {
			sources := sessionRoleSources(configDir, o.Cwd, cli, o.Env, role)
			sources.selection = config.Selection
			return sources.resolve(role, parent)
		})
		// An env this cannot read counts as set: native's own choice is never the wrong
		// answer for what native runs.
		gw.ConfigureNativeBuiltinRoles(func() bool {
			model, err := cli.subagent()
			return model != "" || err != nil
		})
	}
	gw.ConfigureSelection(config.Selection)
	startup, modelSource, effortSource, startupErr := config.effectiveStartup(spec, roleArgs, finalSettings)
	result.Startup = startup
	result.StartupModelSource, result.StartupEffortSource = modelSource, effortSource
	if profiles != nil && startupErr != nil {
		result.NativeExitCode = ExitCodeUnknown
		result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
		return result, startupErr
	}
	if !information && userSettings != nil {
		if err := config.validateUserEffortSettings(userSettings, startup.Model); err != nil {
			result.NativeExitCode = ExitCodeUnknown
			result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
			return result, err
		}
	}
	if ephemeral {
		// Restore provenance without enabling persistent writes for this run.
		gw.ConfigureSessionProfiles(nil, result.Startup, resumed, forkSession, background != nil)
	} else {
		gw.ConfigureSessionProfiles(profiles, result.Startup, resumed, forkSession, background != nil)
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
	spec.Env = withProcessFlag(spec.Env, providerFlag)
	if background == nil && !o.agentView() {
		spec.Env = withProcessFlag(spec.Env, agentViewOff)
	}
	var dispatchOutput backgroundOutput
	stdout, stderr := o.Stdout, o.Stderr
	if background != nil {
		stdout, stderr = &dispatchOutput, &dispatchOutput
	}
	if background != nil && providerFlag != "" {
		// Under a strategy the supervisor is this launch's own, started with its provider
		// environment before the dispatch looks for one.
		supervisor, err := startSupervisor(o.StartProcess, exe, o.Cwd, configDir, spec.Env, 15*time.Second)
		if err != nil {
			result.NativeExitCode = ExitCodeUnknown
			result.Category = CategoryStartFailed
			result.CleanupErr = closeGateway(gw, o.ShutdownTimeout)
			return result, err
		}
		defer func() { supervisor.Stop(); supervisor.Wait() }()
	}
	// A regenerated resident starts no native process: the job it serves already runs.
	var process Process = noDispatch{}
	var startErr error
	if o.Regenerate == nil {
		process, startErr = startAdmitted(o.StartProcess, o.AdmittedNativeSHA256, spec, o.Stdin, stdout, stderr)
	}
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
		var long *childprocess.CommandLineTooLong
		if errors.As(startErr, &long) {
			if nativePDF != "" && o.agentView() {
				startErr = fmt.Errorf("%w. PDF alias lookup carries PATH in --settings alongside the delegation menu (%d entries) and the /model list; their combined launch arguments exceed the Windows limit. Nothing was started and no model was left out", startErr, result.MenuEntries)
			} else {
				startErr = fmt.Errorf("%w. The delegation menu (%d entries) and the /model list are passed as launch arguments, one entry per model the account lists in its picker. Nothing was started and no model was left out", startErr, result.MenuEntries)
			}
		}
		return result, startErr
	}
	result.NativeStarted = true
	nativeCleanupReady = false
	nativeStatus := func() (registered, notExited int, active int64) {
		natives := gw.NativeProcesses()
		for _, native := range natives {
			if providercert.ProcessStatus(native.PID, native.Created) != providercert.ProcessExited {
				notExited++
			}
		}
		_, _, active = gw.Stats()
		return len(natives), notExited, active
	}
	if background != nil {
		backgroundCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		bp := &backgroundProcess{Process: process, link: background, ctx: backgroundCtx, cancel: cancel,
			exe: exe, cwd: o.Cwd, config: configDir, env: backgroundControlEnv(spec.Env), output: &dispatchOutput, ready: o.BackgroundReady, stdout: o.Stdout, stderr: o.Stderr, forkedFrom: forkedFrom, crash: o.backgroundCrash}
		if o.Regenerate != nil {
			record := *o.Regenerate
			bp.job, bp.record = record.Job, &record
		} else if clauductHome != "" && o.Settings == nil && residentLocked {
			bp.record = &ResidentRecord{Link: background.ID, Port: residentPort(gw.Addr()), PluginDir: nativePlugin, Config: configDir, Cwd: o.Cwd, Args: residentArgs(launchArgs)}
		}
		bp.home = clauductHome
		bp.proved = gw.ProvedNativeProcesses
		bp.nativeStatus = nativeStatus
		bp.releaseProfiles = gw.ReleaseIdleSessionProfiles
		if providerFlag != "" {
			// Native's control commands read the provider settings too: without them a stored
			// login decides where they connect (#320 R08). They get the launch's own process
			// environment and start where every native process of this launch starts.
			bp.env, bp.start = spec.Env, o.StartProcess
		}
		process, resident = bp, bp
		retainPlugin = false // serving: the record decides, as for any resident
	}
	var foregroundRecordErr error
	checkpointForeground := func() error {
		if foreground == nil || !o.agentView() || clauductHome == "" || nativePlugin == "" {
			return nil
		}
		if (resident == nil || resident.record.Job == "") && len(nativeJobsWithLink(configDir, foreground.ID)) == 0 {
			retainPlugin = false
			return nil
		}
		if !residentLocked {
			release, lockErr := lockResident(clauductHome, foreground.ID, 0)
			if lockErr != nil {
				retainPlugin = true
				return errors.New("FOREGROUND_RESIDENT_LOCK_FAILED")
			}
			releaseResident, residentLocked = release, true
		}
		if resident == nil {
			resident = &backgroundProcess{config: configDir, home: clauductHome, proved: gw.ProvedNativeProcesses,
				record: &ResidentRecord{Link: foreground.ID, Port: residentPort(gw.Addr()), PluginDir: nativePlugin, Config: configDir, Cwd: o.Cwd, Args: append(residentArgs(launchArgs), "--bg")}}
		}
		err := resident.checkpointForeground()
		retainPlugin = err != nil
		return err
	}
	if foreground != nil && o.agentView() {
		o.foregroundCheckpoint = func() error {
			foregroundRecordErr = checkpointForeground()
			return foregroundRecordErr
		}
		checkpoint := o.Checkpoint
		o.Checkpoint = func(account Status) error {
			err := o.foregroundCheckpoint()
			if checkpoint != nil {
				err = errors.Join(err, checkpoint(account))
			}
			return err
		}
	}

	reaped, lifecycle, waitErr := waitForSession(ctx, process, gw, o, result, interrupts, printMode(o.Args))
	// A hook that blocked the prompt may have ended a short session before the next checkpoint
	// saw its report; the report still decides how the session ended (#295).
	if gw.RoutingMismatch() && lifecycle.Reason != "managed_routing_policy" {
		lifecycle.Reason, lifecycle.Detail = "routing_mismatch", routingMismatchDetail
	}
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

	if o.afterNative != nil {
		o.afterNative()
	}
	foregroundRecordErr = checkpointForeground()
	if reaped && foreground != nil && o.agentView() && o.Handover != nil && clauductHome != "" && nativePlugin != "" {
		// Native keeps an agent view session and the jobs dispatched from it running after its
		// terminal closes; they all use this gateway. Its state stays: the same token, routing
		// proofs and session bindings, so nothing they have proved has to be proved again.
		jobs := nativeJobsWithLink(configDir, foreground.ID)
		for i := 0; len(jobs) == 0 && i < 30; i++ {
			time.Sleep(100 * time.Millisecond) // native lists the closed session as a job as it exits
			jobs = nativeJobsWithLink(configDir, foreground.ID)
		}
		if len(jobs) > 0 {
			foregroundRecordErr = checkpointForeground()
			// Persist before releasing the launcher; a failed pipe write still leaves jobs to serve.
			o.Handover(result.NativeExitCode)
			handoverCtx, cancel := context.WithCancel(context.Background())
			var record *ResidentRecord
			if resident != nil && resident.record.Job != "" {
				record = resident.record
			}
			bp := &backgroundProcess{Process: noDispatch{}, link: foreground, ctx: handoverCtx, cancel: cancel, exe: exe, cwd: o.Cwd, config: configDir,
				env: backgroundControlEnv(spec.Env), output: &backgroundOutput{}, stdout: io.Discard, stderr: io.Discard,
				job: jobs[0], record: record, home: clauductHome, proved: gw.ProvedNativeProcesses, nativeStatus: nativeStatus, releaseProfiles: gw.ReleaseIdleSessionProfiles}
			if record != nil {
				bp.job = record.Job
			}
			if providerFlag != "" {
				bp.env, bp.start = spec.Env, o.StartProcess
			}
			result.HandedOver = jobs
			resident = bp
			foregroundRecordErr = errors.Join(foregroundRecordErr, bp.Wait())
		}
	}
	if foreground != nil && resident != nil && resident.record != nil && resident.record.Job != "" {
		listed, known := jobListing(configDir, append([]string{resident.record.Job}, resident.record.Jobs...))
		resident.unlisted = known && !listed
	}
	// Reap native first, then drain the gateway before taking its final account.
	// Otherwise a cancellation could still be in flight when status claimed completion.
	result.CleanupErr = errors.Join(closeGateway(gw, o.ShutdownTimeout), foregroundRecordErr)
	if reaped {
		gw.FinalizeNativeResults()
		result.CleanupErr = errors.Join(result.CleanupErr, gw.CheckpointSessionProfiles(true))
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

	if lifecycle.Reason == "routing_mismatch" || lifecycle.Reason == "managed_routing_policy" {
		result.Category = CategoryRouting
		return result, fmt.Errorf("ROUTING_UNVERIFIED: %s; the session was ended so no further model request could leave this session's gateway", lifecycle.Detail)
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
	if o.ManagedPolicy == nil {
		env := o.Env
		o.ManagedPolicy = func() ([]policyDocument, error) { return readManagedPolicy(env) }
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

// nativeReplaced compares claude.exe with what was resolved at launch. Size and
// modification time, not a hash: this runs on every session and an updater
// rewrites both. A file gone or unreadable afterwards also counts as replaced.
func nativeReplaced(exe string, before os.FileInfo) bool {
	if before == nil {
		return false
	}
	after, err := os.Stat(exe)
	return err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime())
}

// startProductGateway starts the gateway every session runs: the transport given, under the
// product's admission policy. The provider-control probe starts its gateway here too, with its
// synthetic transport, so the boundaries it measures are the product's own.
func startProductGateway(transport upstream.Transport) (*gateway.Gateway, error) {
	return StartProductGatewayAt(transport, 0)
}

// StartProductGatewayAt is the product gateway on a given loopback port, 0 for any: a
// regenerated background resident binds the port its job was dispatched with (#319 decision 18c).
func StartProductGatewayAt(transport upstream.Transport, port int) (*gateway.Gateway, error) {
	g, err := gateway.StartAt(transport, port)
	if err == nil {
		g.EnableContextPolicy()
	}
	return g, err
}

// Provider strategies retain their separate supervisor boundary.
func (o Options) agentView() bool { return o.ProviderStrategy == "" }
