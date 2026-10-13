// Command clauduct starts the installed native Claude Code with its model requests routed
// through an ephemeral loopback gateway.
//
// It parses nothing. Every argument goes to the native executable exactly as given,
// including --help and --version, so their meaning stays the native one. The exceptions
// are a handful of first arguments no native option is: --update, --usage, --uninstall,
// --dev for Clauduct's own commands (clauduct --dev --version is this build's identity),
// and the hook and PDF renderer roles the session runs this same file for. Since v0.4.0
// that is the whole installation (#112); a copy named clauduct-hook.exe or clauduct-dev.exe
// takes the role its name says.
//
// The name was clauduct-go until the Node implementation stopped being the installed
// product. Sharing a name before that decision would have made PATH order decide which
// implementation a user runs; now the decision has been made and the name is the answer to
// it. The Node build keeps its own name, so going back is renaming two files.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/devcmd"
	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"github.com/wotjr1649/Clauduct/go/internal/update"
)

// probeSHA256 is the SHA-256 of the clauduct-probe.exe built from this source with
// -trimpath -buildvcs=false (go/README.md): the only probe this build's diagnostics run or
// trust. A launch never needs it.
//
//go:embed probe.sha256
var probeSHA256 string

func main() {
	app.ExpectedProbeSHA256 = strings.TrimSpace(probeSHA256)
	os.Exit(run())
}

func run() int {
	// The roles this file plays for a running session come first: they are not launches,
	// and a hook has the client's timeout to answer in.
	if len(os.Args) == 3 && os.Args[1] == "--clauduct-background-key" {
		// A background job whose resident died asks for its key first when it wakes: a dead
		// resident's record lets this start one again on the same port and link (#319 decision 18c).
		if _, err := sessionlink.Read(os.Args[2]); err != nil {
			app.RegenerateResident(os.Args[2], 30*time.Second)
		}
	}
	hookcmd.Regenerable = app.RegenerableJob
	hookcmd.ReconnectContext = app.ReconnectResidentContext
	if code, ok := hookcmd.Dispatch(os.Args); ok {
		return code
	}
	if len(os.Args) == 3 && os.Args[1] == app.ResidentRestoreRole {
		return regenerate(os.Args[2])
	}
	if args, ok := devArgs(os.Args); ok {
		return devcmd.Run(args, os.Stdout, os.Stderr)
	}
	args := os.Args[1:]
	resident := len(args) > 0 && args[0] == app.BackgroundRole
	ready := false
	if resident {
		args = args[1:]
		defer func() {
			if !ready {
				_ = json.NewEncoder(os.Stdout).Encode(app.BackgroundReply{Error: "BACKGROUND_START_FAILED"})
			}
		}()
		if !app.BackgroundRequested(args) {
			return 1
		}
	}
	if len(args) == 2 && args[0] == "--background-stop" {
		if err := sessionlink.StopAndWait(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	// An update cannot delete the binary it was running, so it renames it aside and says
	// which file is left. This process is not running that file, so it can finish the job --
	// and the next launch after an update is the first moment anything can. Silent and
	// never fatal: a leftover that stays is untidy, and a session that failed over tidying
	// would be worse than untidy.
	update.SweepLeftover()

	// The one option this launcher owns, checked before anything else happens. It updates
	// Clauduct, not the client: `clauduct update` still reaches the client's own updater,
	// because that one is a bare subcommand and this one is not.
	if wanted, _ := update.Requested(args); wanted {
		ctx, cancel := context.WithTimeout(context.Background(), update.Timeout)
		defer cancel()
		return update.Run(ctx, args, os.Stdin, os.Stdout)
	}

	// The second option this launcher owns: what this account has spent. Recognised the
	// same way, and only as the first argument, because a prompt is an argument like any
	// other. The client defines no --usage of its own (measured on 2.1.274) and its own
	// /usage cannot answer for this backend, so nothing is being taken over.
	if asked(args, usageOption) {
		return app.WriteUsage("", os.Stdout)
	}

	// The third: removing this installation. It has to live in the binary and not only in
	// scripts/uninstall.ps1, because that script needs the repository and a machine that
	// installed from a release has the binary and nothing else. Recognised by the same rule
	// as --update, and it asks before it deletes anything for the same reason --update does.
	// The diagnostics directory is handed over rather than looked up there: the package that
	// writes those files owns where they live.
	if wanted, _ := update.UninstallRequested(args); wanted {
		return update.Uninstall(args, app.StatusDir(), os.Stdin, os.Stdout)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct: CWD_UNAVAILABLE")
		return 1
	}

	handover := app.HandoverFromEnv()
	if handover == nil && !resident && app.HandoverLaunch(args) {
		return app.LaunchHandover(args)
	}
	var handoverTo func(int) bool
	if handover != nil {
		handoverTo = func(nativeExit int) bool {
			fmt.Fprintln(os.Stderr, "clauduct: agent view jobs are still running; this session's gateway stays with them in the background")
			return handover(nativeExit)
		}
	}
	env := platform.Environment(os.Environ())
	sessionDuration, err := app.SessionDuration(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct: INVALID_SESSION_TIMEOUT")
		return 1
	}
	resolve := platform.Resolver{}.Claude
	// No provider strategy and no admission gate (#319 decision 17): every launch path runs on
	// the launch's own endpoint, the settings routing pin and the hooks' start-up handshake.
	// The provider-control probe stays a diagnostic (clauduct --dev --probe provider-control).
	if !resident && app.BackgroundRequested(args) {
		if err := app.StartBackground(context.Background(), args, cwd, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "clauduct:", err)
			return 1
		}
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var backgroundReady func(app.BackgroundSession) error
	if resident {
		bootstrap := time.AfterFunc(60*time.Second, cancel)
		defer bootstrap.Stop()
		backgroundReady = func(session app.BackgroundSession) error {
			bootstrap.Stop()
			if err := json.NewEncoder(os.Stdout).Encode(app.BackgroundReply{Session: &session}); err != nil {
				return err
			}
			ready = true
			os.Stdout.Close()
			os.Stderr.Close()
			return nil
		}
	}
	ledger, startGateway, modelList := productUpstream(0)
	result, err := app.Run(ctx, app.Options{
		Ledger:          ledger,
		StartGateway:    startGateway,
		ModelList:       modelList,
		Args:            args,
		Env:             env,
		Cwd:             cwd,
		ResolveClaude:   resolve,
		SessionTimeout:  sessionDuration,
		Checkpoint:      app.WriteCheckpoint,
		BackgroundReady: backgroundReady,
		Handover:        handoverTo,
	})

	// What the session did, said once, at the end. The native client owns the terminal
	// while it runs, so anything written during a session lands in the prompt box -- and
	// nothing goes to stdout, which belongs to the answer `claude -p` was asked for.
	if result.NativeStarted || result.Category != "" {
		app.Report(result, os.Stderr, env)
	}

	if err != nil {
		var refused *app.RefusedOptionError
		if errors.As(err, &refused) {
			// Name the option and say why. A refusal the user cannot act on reads as a
			// bug, and this one is a deliberate policy with a short list behind it.
			fmt.Fprintf(os.Stderr,
				"clauduct: %s is not forwarded, because %s.\n"+
					"          Every other native option is passed through unchanged.\n",
				refused.Option, launch.Reason(refused.Option))
			return 1
		}
		if errors.Is(err, app.ErrClaudeNotFound) {
			// Name where it looked. A user who installed elsewhere can act on this;
			// "not found" alone sends them guessing.
			path, _, _ := resolve()
			fmt.Fprintf(os.Stderr, "clauduct: CLAUDE_NOT_FOUND (expected %s or a PATH entry)\n", path)
			return 1
		}
		fmt.Fprintf(os.Stderr, "clauduct: %v\n", err)
		return 1
	}

	// Cleanup trouble is reported without overwriting the native result. The native exit
	// code is the answer to what the user asked for; a leaked resource is a separate fact
	// and stderr is where it belongs, never mixed into a headless stdout.
	if result.CleanupErr != nil {
		fmt.Fprintf(os.Stderr, "clauduct: CLEANUP_FAILED %v\n", result.CleanupErr)
	}
	if result.Category == app.CategoryDeadline {
		return 124
	}
	return result.NativeExitCode
}

// usageOption is the name for the account view.
const usageOption = "--usage"

// devOption is where Clauduct's own commands start: clauduct --dev --version.
const devOption = "--dev"

// devArgs is the command for devcmd when argv asks for one: --dev first, the way --update
// is recognised, or the name clauduct-dev.exe that a copy from a 0.3.x updater carries.
//
// The name is compared without its .exe: cmd.exe hands a program the command line as typed,
// so `clauduct-dev version` there arrives as argv[0] "clauduct-dev" -- and a miss would send
// "version" to the client as a prompt, which is a billed session.
func devArgs(argv []string) ([]string, bool) {
	if len(argv) > 0 && hookcmd.Named(argv[0], "clauduct-dev") {
		return argv[1:], true
	}
	if len(argv) > 1 && strings.EqualFold(argv[1], devOption) {
		return argv[2:], true
	}
	return nil, false
}

// asked reports whether argv is exactly this option.
//
// First and alone. Everything else this launcher sees belongs to the native client, and an
// option matched anywhere in the vector would fire on a prompt that merely mentions it --
// which for --update meant replacing binaries and for this means printing instead of
// starting a session.
func asked(args []string, option string) bool {
	return len(args) == 1 && strings.EqualFold(args[0], option)
}

// regenerate is the resident a background key helper starts when its link is gone (#319
// decision 18c). The dead resident's record names the job, port, link and options; the
// gateway is new, so is its token, which native's key helper reads from the link.
func regenerate(link string) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "CLAUDUCT_RESIDENT_RESTORE_FAILED: RESIDENT_RECORD_UNAVAILABLE")
		return 1
	}
	record, err := app.LoadResident(home, link)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CLAUDUCT_RESIDENT_RESTORE_FAILED: RESIDENT_RECORD_UNAVAILABLE")
		return 1
	}
	env := platform.Environment(os.Environ())
	sessionDuration, err := app.SessionDuration(env)
	if err != nil {
		return 1
	}
	ledger, startGateway, modelList := productUpstream(record.Port)
	result, err := app.Run(context.Background(), app.Options{
		Ledger:         ledger,
		StartGateway:   startGateway,
		ModelList:      modelList,
		Args:           record.Args,
		Env:            env,
		Cwd:            record.Cwd,
		ResolveClaude:  platform.Resolver{}.Claude,
		SessionTimeout: sessionDuration,
		Checkpoint:     app.WriteCheckpoint,
		Regenerate:     &record,
		Stderr:         io.Discard,
	})
	return finishRegeneration(result, err, os.Stderr)
}

func finishRegeneration(result app.Result, runErr error, errOut io.Writer) int {
	// The status file says how this resident ended, as run() writes it; without this the last
	// checkpoint (RUNNING) stayed. No summary line: this stderr reaches native's key helper.
	if result.NativeStarted || result.Category != "" {
		if err := app.WriteFinal(result); err != nil {
			fmt.Fprintln(errOut, "clauduct: CLAUDUCT_REQUEST_STATUS_UNAVAILABLE")
		}
	}
	if runErr != nil || result.CleanupErr != nil {
		category := "RESIDENT_START_FAILED"
		if runErr != nil {
			if known := hookcmd.DiagnosticCategory(runErr.Error()); known != "" {
				category = known
			}
		}
		fmt.Fprintln(errOut, "CLAUDUCT_RESIDENT_RESTORE_FAILED: "+category)
		return 1
	}
	return 0
}
