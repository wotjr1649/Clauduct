package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var errNativeProviderStatus = errors.New("NATIVE_PROVIDER_STATUS_UNVERIFIED: native provider status could not be verified; no model session was started")
var errNativeStoredGateway = errors.New("NATIVE_STORED_GATEWAY: native reports a saved Claude apps gateway login which outranks this session's endpoint; no model session was started. End Clauduct and sign out of that gateway in standalone Claude Code before retrying")

// Query native's opaque credential store without loading user/project settings.
// shortcut: this launch-time query does not certify later shared-state changes; stronger isolation needs a separate contract.
func checkNativeProvider(ctx context.Context, exe string, o Options, identity *childprocess.ProcessIdentity) error {
	settings, err := json.Marshal(map[string]any{"env": routingSettingsEnv(nil, "http://127.0.0.1:1")})
	if err != nil {
		return errNativeProviderStatus
	}
	spec := launch.Build(exe, []string{"auth", "status", "--json"}, o.Env, o.Cwd, launch.Overlay{Settings: string(settings), Enforced: map[string]string{
		"DISABLE_AUTOUPDATER": "1", "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}})
	spec.Args = append([]string{"--setting-sources", ""}, spec.Args...)
	var output backgroundOutput
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	process, err := startAdmitted(startOSProcess, o.AdmittedNativeSHA256, spec, nil, &output, io.Discard)
	if err != nil {
		return errNativeProviderStatus
	}
	if query, ok := process.(interface {
		Identity() (childprocess.ProcessIdentity, error)
	}); ok {
		*identity, _ = query.Identity()
	}
	waitErr, reaped := waitFor(ctx, process)
	var exitErr *exec.ExitError
	if !reaped || ctx.Err() != nil || waitErr != nil && !errors.As(waitErr, &exitErr) {
		return errNativeProviderStatus
	}
	return nativeProviderStatus([]byte(output.text()), process.ExitCode())
}

func nativeProviderStatus(raw []byte, exit int) error {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return errNativeProviderStatus
	}
	var provider string
	var loggedIn *bool
	if json.Unmarshal(fields["apiProvider"], &provider) != nil || json.Unmarshal(fields["loggedIn"], &loggedIn) != nil || loggedIn == nil {
		return errNativeProviderStatus
	}
	if provider == "gateway" {
		return errNativeStoredGateway
	}
	if provider != "firstParty" || *loggedIn && exit != 0 || !*loggedIn && exit != 1 {
		return errNativeProviderStatus
	}
	return nil
}

// Keep native's parent policy intact, but refuse routing that can replace the bridge.
func checkParentRouting(args []string) error {
	value, supplied := "", false
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known {
			for _, arg := range args[i+1:] {
				if name, _, _ := strings.Cut(arg, "="); name == "--managed-settings" {
					return errUserSettings
				}
			}
			break
		}
		name, attached, hasValue := strings.Cut(args[i], "=")
		if end > len(args) {
			if name == "--managed-settings" {
				return errUserSettings
			}
			break
		}
		if name == "--managed-settings" {
			value, supplied = attached, true
			if !hasValue {
				value = args[i+1]
			}
		}
		i = end
	}
	if !supplied {
		return nil
	}
	if len(value) > settingsBytes {
		return errUserSettings
	}
	fields, err := wire.Fields([]byte(value), nil)
	if err != nil {
		return errUserSettings
	}
	delete(fields, "allowManagedPermissionRulesOnly")
	raw, err := json.Marshal(fields)
	if err != nil {
		return errUserSettings
	}
	if key, valid := routingKey(string(raw)); !valid {
		return errUserSettings
	} else if key != "" {
		return errSettingsConflict
	}
	return nil
}
