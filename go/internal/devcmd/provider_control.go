package devcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
)

// providerControl is clauduct --dev --probe provider-control [--scope S] [--long]: the manual
// pre-warm of admission r2's writer. It runs the same probe the launch gate runs, for one
// scope or all of them in order; --long then runs the RP01 check of each scope that needs it.
func providerControl(args []string, errOut io.Writer) int {
	scopes, long := app.AdmissionScopes, false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--long":
			long = true
		case args[i] == "--scope" && i+1 < len(args) && slices.Contains(app.AdmissionScopes, args[i+1]):
			scopes = []string{args[i+1]}
			i++
		default:
			fmt.Fprintln(errOut, "usage: clauduct --dev --probe provider-control [--scope "+fmt.Sprint(app.AdmissionScopes)+"] [--long]")
			return 2
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(errOut, "clauduct:", err)
		return 1
	}
	ctx := context.Background()
	for _, scope := range scopes {
		if err := app.RunProbe(ctx, app.ProbeCall{Scope: scope, Home: home, Stderr: errOut}); err != nil {
			fmt.Fprintf(errOut, "clauduct: provider-control probe %s: %v\n", scope, err)
			return 1
		}
	}
	if long {
		for _, scope := range scopes {
			if !admissioncontract.NeedsRP01(scope) {
				continue
			}
			if err := app.RunProbe(ctx, app.ProbeCall{Scope: scope, Long: true, Home: home, Stderr: errOut}); err != nil {
				fmt.Fprintf(errOut, "clauduct: provider-control RP01 %s: %v\n", scope, err)
				return 1
			}
		}
	}
	return 0
}
