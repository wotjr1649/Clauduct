package app

import (
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
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
