// Command clauduct starts the installed native Claude Code with its model requests routed
// through an ephemeral loopback gateway.
//
// It parses nothing. Every argument goes to the native executable exactly as given,
// including --help and --version, so their meaning stays the native one. Clauduct's own
// build identity is a question for clauduct-dev, which is a separate binary precisely so
// that asking it can never collide with a native option or a native option's value.
//
// The name was clauduct-go until the Node implementation stopped being the installed
// product. Sharing a name before that decision would have made PATH order decide which
// implementation a user runs; now the decision has been made and the name is the answer to
// it. The Node build keeps its own name, so going back is renaming two files.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/update"
)

func main() {
	os.Exit(run())
}

func run() int {
	// An update cannot delete the binary it was running, so it renames it aside and says
	// which file is left. This process is not running that file, so it can finish the job --
	// and the next launch after an update is the first moment anything can. Silent and
	// never fatal: a leftover that stays is untidy, and a session that failed over tidying
	// would be worse than untidy.
	update.SweepLeftover()

	// The one option this launcher owns, checked before anything else happens. It updates
	// Clauduct, not the client: `clauduct update` still reaches the client's own updater,
	// because that one is a bare subcommand and this one is not.
	if wanted, _ := update.Requested(os.Args[1:]); wanted {
		ctx, cancel := context.WithTimeout(context.Background(), update.Timeout)
		defer cancel()
		return update.Run(ctx, os.Args[1:], os.Stdin, os.Stdout)
	}

	// The second option this launcher owns: what this account has spent. Recognised the
	// same way, and only as the first argument, because a prompt is an argument like any
	// other. The client defines no --usage of its own (measured on 2.1.274) and its own
	// /usage cannot answer for this backend, so nothing is being taken over.
	if asked(os.Args[1:], usageOption) {
		return app.WriteUsage("", os.Stdout)
	}

	// The third: removing this installation. It has to live in the binary and not only in
	// scripts/uninstall.ps1, because that script needs the repository and a machine that
	// installed from a release has the binary and nothing else. Recognised by the same rule
	// as --update, and it asks before it deletes anything for the same reason --update does.
	// The diagnostics directory is handed over rather than looked up there: the package that
	// writes those files owns where they live.
	if wanted, _ := update.UninstallRequested(os.Args[1:]); wanted {
		return update.Uninstall(os.Args[1:], app.StatusDir(), os.Stdin, os.Stdout)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct: CWD_UNAVAILABLE")
		return 1
	}

	env := environMap()
	resolve := platform.Resolver{}.Claude
	result, err := app.Run(context.Background(), app.Options{
		Args:          os.Args[1:],
		Env:           env,
		Cwd:           cwd,
		ResolveClaude: resolve,
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
	return result.NativeExitCode
}

func environMap() map[string]string {
	entries := os.Environ()
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if i := strings.IndexByte(entry, '='); i > 0 {
			out[entry[:i]] = entry[i+1:]
		}
	}
	return out
}

// usageOption is the name for the account view.
const usageOption = "--usage"

// asked reports whether argv is exactly this option.
//
// First and alone. Everything else this launcher sees belongs to the native client, and an
// option matched anywhere in the vector would fire on a prompt that merely mentions it --
// which for --update meant replacing binaries and for this means printing instead of
// starting a session.
func asked(args []string, option string) bool {
	return len(args) == 1 && strings.EqualFold(args[0], option)
}
