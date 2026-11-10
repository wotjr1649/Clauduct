package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var errNativeProviderStatus = errors.New("NATIVE_PROVIDER_STATUS_UNVERIFIED: native provider status could not be verified; no model session was started")
var errNativeStoredGateway = errors.New("NATIVE_STORED_GATEWAY: native reports a saved Claude apps gateway login which outranks this session's endpoint; no model session was started. End Clauduct and sign out of that gateway in standalone Claude Code before retrying")
var errNativeProviderQueryDeadline = errors.New("native provider status query deadline")

const nativeProviderStatusTimeout = 30 * time.Second

type nativeProviderFailure struct {
	stage  string
	reason string
}

// Query native's opaque credential store without loading user/project settings.
// shortcut: this launch-time query does not certify later shared-state changes; stronger isolation needs a separate contract.
func checkNativeProvider(ctx context.Context, exe string, o Options, identity *childprocess.ProcessIdentity) error {
	return checkNativeProviderWithStart(ctx, exe, o, identity, startOSProcess)
}

func checkNativeProviderWithStart(parent context.Context, exe string, o Options, identity *childprocess.ProcessIdentity,
	start func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error)) error {
	entered := time.Now()
	parentDeadline, hasParentDeadline := parent.Deadline()
	settings, err := json.Marshal(map[string]any{"env": routingSettingsEnv(nil, "http://127.0.0.1:1")})
	if err != nil {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"start", "prepare_failed"}, "na", "unknown", errNativeProviderStatus)
	}
	spec := launch.Build(exe, []string{"auth", "status", "--json"}, o.Env, o.Cwd, launch.Overlay{Settings: string(settings), Enforced: map[string]string{
		"DISABLE_AUTOUPDATER": "1", "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}})
	spec.Args = append([]string{"--setting-sources", ""}, spec.Args...)
	var output backgroundOutput
	queryDeadline := time.Now().Add(nativeProviderStatusTimeout)
	ctx, cancel := context.WithDeadlineCause(parent, queryDeadline, errNativeProviderQueryDeadline)
	defer cancel()
	if ctx.Err() != nil {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"ctx", nativeProviderContextReason(parentDeadline, hasParentDeadline, queryDeadline, ctx.Err(), context.Cause(ctx))}, "na", "unknown", errNativeProviderStatus)
	}
	process, startErr := startAdmitted(start, o.AdmittedNativeSHA256, spec, nil, &output, io.Discard)
	// A synchronous start can outlive the query budget. Snapshot the first context
	// outcome before cleanup so a later cancellation cannot replace its cause.
	startContextErr, startContextCause := ctx.Err(), context.Cause(ctx)
	if startContextErr != nil {
		if process == nil {
			return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"ctx", nativeProviderContextReason(parentDeadline, hasParentDeadline, queryDeadline, startContextErr, startContextCause)}, "na", "unknown", errNativeProviderStatus)
		}
		providerProcessIdentity(process, identity)
		_, reaped := waitFor(ctx, process)
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"ctx", nativeProviderContextReason(parentDeadline, hasParentDeadline, queryDeadline, startContextErr, startContextCause)}, reapStatus(reaped), "unknown", errNativeProviderStatus)
	}
	if startErr != nil {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"start", "spawn_failed"}, "na", "unknown", errNativeProviderStatus)
	}
	if process == nil {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"start", "spawn_failed"}, "na", "unknown", errNativeProviderStatus)
	}
	providerProcessIdentity(process, identity)
	waitErr, reaped := waitFor(ctx, process)
	waitContextErr, waitContextCause := ctx.Err(), context.Cause(ctx)
	if waitContextErr != nil {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"ctx", nativeProviderContextReason(parentDeadline, hasParentDeadline, queryDeadline, waitContextErr, waitContextCause)}, reapStatus(reaped), "unknown", errNativeProviderStatus)
	}
	if !reaped {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"wait", "reap_unconfirmed"}, "false", "unknown", errNativeProviderStatus)
	}
	exit := process.ExitCode()
	exitText := strconv.Itoa(exit)
	if exit == ExitCodeUnknown {
		exitText = "unknown"
	}
	if !nativeProviderWaitError(waitErr) {
		return nativeProviderRefusal(o.Stderr, entered, nativeProviderFailure{"wait", "wait_failed"}, "true", exitText, errNativeProviderStatus)
	}
	failure := nativeProviderStatusFailure([]byte(output.text()), exit)
	if failure != nil {
		return nativeProviderRefusal(o.Stderr, entered, *failure, "true", exitText, nativeProviderStatusError(*failure))
	}
	return nil
}

func nativeProviderStatus(raw []byte, exit int) error {
	failure := nativeProviderStatusFailure(raw, exit)
	if failure == nil {
		return nil
	}
	return nativeProviderStatusError(*failure)
}

func nativeProviderStatusFailure(raw []byte, exit int) *nativeProviderFailure {
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return &nativeProviderFailure{"parse", "invalid_json"}
	}
	var provider *string
	var loggedIn *bool
	if json.Unmarshal(fields["apiProvider"], &provider) != nil || provider == nil || json.Unmarshal(fields["loggedIn"], &loggedIn) != nil || loggedIn == nil {
		return &nativeProviderFailure{"field", "invalid_field"}
	}
	if *provider == "gateway" {
		return &nativeProviderFailure{"field", "stored_gateway"}
	}
	if *provider != "firstParty" {
		return &nativeProviderFailure{"field", "unsupported_provider"}
	}
	if *loggedIn && exit != 0 || !*loggedIn && exit != 1 {
		return &nativeProviderFailure{"exit", "exit_mismatch"}
	}
	return nil
}

func nativeProviderStatusError(failure nativeProviderFailure) error {
	if failure.reason == "stored_gateway" {
		return errNativeStoredGateway
	}
	return errNativeProviderStatus
}

func nativeProviderRefusal(stderr io.Writer, entered time.Time, failure nativeProviderFailure, reaped, exit string, result error) error {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "clauduct: native-provider-status stage=%s reason=%s elapsed_ms=%d reaped=%s exit=%s\n",
			failure.stage, failure.reason, max(0, time.Since(entered).Milliseconds()), reaped, exit)
	}
	return result
}

func nativeProviderContextReason(parentDeadline time.Time, hasParentDeadline bool, queryDeadline time.Time, ctxErr, cause error) string {
	if ctxErr == context.DeadlineExceeded {
		if hasParentDeadline && !parentDeadline.After(queryDeadline) {
			return "parent_deadline"
		}
		if cause == errNativeProviderQueryDeadline {
			return "query_deadline"
		}
		return "parent_deadline"
	}
	return "parent_cancelled"
}

func providerProcessIdentity(process Process, identity *childprocess.ProcessIdentity) {
	if query, ok := process.(interface {
		Identity() (childprocess.ProcessIdentity, error)
	}); ok && identity != nil {
		*identity, _ = query.Identity()
	}
}

func reapStatus(reaped bool) string {
	if reaped {
		return "true"
	}
	return "false"
}

func nativeProviderWaitError(err error) bool {
	switch current := err.(type) {
	case nil:
		return true
	case *exec.ExitError:
		return current != nil
	case interface{ Unwrap() []error }:
		children := current.Unwrap()
		if len(children) == 0 {
			return false
		}
		sawChild := false
		for _, child := range children {
			if child == nil {
				continue
			}
			sawChild = true
			if !nativeProviderWaitError(child) {
				return false
			}
		}
		return sawChild
	case interface{ Unwrap() error }:
		child := current.Unwrap()
		return child != nil && nativeProviderWaitError(child)
	default:
		return false
	}
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
