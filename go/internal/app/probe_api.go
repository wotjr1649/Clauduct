package app

import (
	"context"
	"errors"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The provider-control probe (go/cmd/clauduct-probe, #320) runs sessions through Run in its
// own process. These are the only parts of this package it needs beyond Run and Options.

// UseProductExecutable makes the sessions this process runs give their hooks and native
// confirmation helper path instead of this process's own executable. The probe calls it once
// at start with the clauduct.exe it certifies, so the hooks measured are that binary's.
func UseProductExecutable(path string) {
	executable = func() (string, error) { return path, nil }
}

// StartProductGateway starts the gateway every session runs, on transport: the probe's
// synthetic backend behind the product's own admission policy.
func StartProductGateway(transport upstream.Transport) (*gateway.Gateway, error) {
	return startProductGateway(transport)
}

// ResolvedNativeSHA256 is the SHA-256 of the claude.exe a session would resolve now.
func ResolvedNativeSHA256() (string, error) {
	path, found, err := platform.Resolver{}.Claude()
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrClaudeNotFound
	}
	id, err := providercert.Observe(path)
	if err != nil {
		return "", err
	}
	return id.SHA256, nil
}

// NativeEventsModule is the native-events plugin's hook module relative to the plugin
// directory Run puts first on native's command line (the probe breaks its proof there).
func NativeEventsModule() string { return nativeEventsModule }

// ErrProbeUpstream means ProbeOptions was given no synthetic gateway or no synthetic model
// list. The probe has no real upstream to fall back to, so it has no default either.
var ErrProbeUpstream = errors.New("PROBE_UPSTREAM_REQUIRED: the probe must pass its synthetic gateway and model list")

// ProbeOptions completes base with the probe's synthetic upstream: the gateway start and the
// account list it measures against. Both are required, so a probe session cannot be built
// without them and cannot reach a credential provider through an omission; Run itself
// refuses an Options that lacks either (ErrNoUpstream). base's own StartGateway and
// ModelList are replaced.
func ProbeOptions(base Options, startGateway func() (*gateway.Gateway, error), modelList func(ctx context.Context, home string) (*bridge.Catalogue, ModelListFacts, error)) (Options, error) {
	if startGateway == nil || modelList == nil {
		return Options{}, ErrProbeUpstream
	}
	base.StartGateway = startGateway
	base.ModelList = modelList
	return base, nil
}
