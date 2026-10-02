package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// bypassLaunch reports whether the user starts this session in native bypassPermissions
// mode by their own choice. Then Clauduct adds no required ask rules, which is what native
// bypass means (decided 2026-10-02); every other mode keeps them. The start mode decides:
// a later in-session mode change does not add or remove the rules.
//
// The launcher refuses --dangerously-skip-permissions before this runs (launch/refuse.go),
// so a bypass start is --permission-mode bypassPermissions or a settings defaultMode.
// Precedence follows native: an explicit CLI mode, else permissions.defaultMode from
// user < project < local settings (as --setting-sources loads them) < --settings.
//
// Any doubt keeps the rules: an argument list this launcher cannot scan to the end, any
// managed policy source (managed-settings.json naming a mode or disabling bypass, a
// managed-settings.d drop-in, the registry policy key), a loaded settings file that
// disables bypass mode, or a settings file it cannot read.
func bypassLaunch(args []string, user map[string]json.RawMessage, config, cwd string) bool {
	if !argumentsScanned(args) || managedPolicy() {
		return false
	}
	sources := map[string]bool{"user": true, "project": true, "local": true}
	if value, set := optionValue(args, "--setting-sources"); set {
		sources = map[string]bool{}
		for _, part := range strings.Split(value, ",") {
			sources[strings.TrimSpace(part)] = true
		}
	}
	mode, disabled := "", false
	for _, file := range []struct{ source, path string }{
		{"user", filepath.Join(config, "settings.json")},
		{"project", filepath.Join(cwd, ".claude", "settings.json")},
		{"local", filepath.Join(cwd, ".claude", "settings.local.json")},
	} {
		if !sources[file.source] {
			continue
		}
		raw, err := boundedRoleFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		value, decided, disables, ok := permissionMode(raw, err)
		if !ok {
			return false
		}
		if decided {
			mode = value
		}
		disabled = disabled || disables
	}
	if permissions, present := user["permissions"]; present {
		value, decided, disables, ok := permissionMode([]byte(`{"permissions":`+string(permissions)+`}`), nil)
		if !ok {
			return false
		}
		if decided {
			mode = value
		}
		disabled = disabled || disables
	}
	if disabled {
		return false
	}
	if value, set := optionValue(args, "--permission-mode"); set {
		return value == "bypassPermissions"
	}
	return mode == "bypassPermissions"
}

// managedPolicy reports any managed policy that could set the mode or forbid bypass. A
// variable so tests can stand in for the machine's policy sources.
var managedPolicy = func() bool {
	root := managedRoot()
	if raw, err := boundedRoleFile(filepath.Join(root, "managed-settings.json")); !os.IsNotExist(err) {
		_, decided, disables, ok := permissionMode(raw, err)
		if !ok || decided || disables {
			return true
		}
	}
	if entries, err := os.ReadDir(filepath.Join(root, "managed-settings.d")); err == nil && len(entries) > 0 || err != nil && !os.IsNotExist(err) {
		return true
	}
	return platform.ClaudePolicyKeyPresent()
}

// argumentsScanned reports whether every argument before "--" is an option this launcher
// knows the shape of; past an unknown one, a later --permission-mode could be a value.
func argumentsScanned(args []string) bool {
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			return false
		}
		i = end
	}
	return true
}

// permissionMode reads permissions.defaultMode and whether disableBypassPermissionsMode is
// present at all; ok is false for a document it cannot read.
func permissionMode(raw []byte, readErr error) (mode string, decided, disables, ok bool) {
	if readErr != nil {
		return "", false, false, false
	}
	var settings struct {
		Permissions map[string]json.RawMessage `json:"permissions"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return "", false, false, false
	}
	_, disables = settings.Permissions["disableBypassPermissionsMode"]
	value, present := settings.Permissions["defaultMode"]
	if !present {
		return "", false, disables, true
	}
	if json.Unmarshal(value, &mode) != nil {
		return "", false, false, false
	}
	return mode, true, disables, true
}
