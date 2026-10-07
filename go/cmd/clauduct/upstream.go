package main

import (
	"context"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// productUpstream is the one place a session gets the real transport (G7): every inference in
// a session started by this binary reaches the user's Codex subscription. app.Run builds no
// upstream of its own (#319); the probe and tests pass synthetic ones.
//
// Nothing is read or spawned here. The credential provider opens auth.json on the first
// request, and the client version runs codex --version on the first request, so
// `clauduct --version` still costs neither (ARG07).
//
// The budget is unrestricted, and deliberately: a route is what the client asked for, and a
// count cap would stop a long session partway through. The verification budget is a separate
// thing and lives in clauduct --dev --probe.
func productUpstream() (*upstream.Ledger, func() (*gateway.Gateway, error), func(context.Context, string) (*bridge.Catalogue, app.ModelListFacts, error)) {
	ledger := upstream.NewLedger(upstream.Unlimited())
	direct := upstream.NewDirect(&auth.Provider{}, ledger, upstream.InstalledVersion())
	start := func() (*gateway.Gateway, error) { return app.StartProductGateway(direct) }
	// The same Direct: the list and every request bind the same account.
	list := func(ctx context.Context, home string) (*bridge.Catalogue, app.ModelListFacts, error) {
		return app.AccountModelList(ctx, direct, home)
	}
	return ledger, start, list
}
