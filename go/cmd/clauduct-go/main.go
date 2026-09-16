// Command clauduct-go starts the installed native Claude Code with its model requests
// routed through an ephemeral loopback gateway.
//
// It parses nothing. Every argument goes to the native executable exactly as given,
// including --help and --version, so their meaning stays the native one. Clauduct's own
// build identity is a question for clauduct-dev, which is a separate binary precisely so
// that asking it can never collide with a native option or a native option's value.
//
// The name is clauduct-go, not clauduct, for as long as the Node implementation is the
// installed product. Sharing a name before a promotion decision would make PATH order
// decide which implementation a user runs.
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
)

func main() {
	os.Exit(run())
}

func run() int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "clauduct: CWD_UNAVAILABLE")
		return 1
	}

	resolve := platform.Resolver{}.Claude
	result, err := app.Run(context.Background(), app.Options{
		Args:          os.Args[1:],
		Env:           environMap(),
		Cwd:           cwd,
		ResolveClaude: resolve,
	})

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
