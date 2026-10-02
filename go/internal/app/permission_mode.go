package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// bypassLaunch reports whether the user starts this session in native bypassPermissions
// mode by their own choice. Then Clauduct adds no required ask rules, which is what native
// bypass means (decided 2026-10-02); every other mode keeps them. The start mode decides:
// a later in-session mode change does not add or remove the rules.
//
// The launcher refuses --dangerously-skip-permissions before this runs (launch/refuse.go),
// so a bypass start is --permission-mode bypassPermissions or a settings defaultMode.
// Precedence follows native: an explicit CLI mode, else permissions.defaultMode from
// user < project < local settings (as --setting-sources loads them) < --settings. Managed
// policy that names a mode or disables bypass keeps the rules, and anything unreadable
// keeps them too: a doubt resolves toward confirmation, never away from it.
func bypassLaunch(args []string, user map[string]json.RawMessage, config, cwd string) bool {
	if mode, set := optionValue(args, "--permission-mode"); set {
		return mode == "bypassPermissions"
	}
	if raw, err := boundedRoleFile(filepath.Join(managedRoot(), "managed-settings.json")); !os.IsNotExist(err) {
		_, decided, ok := permissionMode(raw, err)
		var managed struct {
			Permissions map[string]json.RawMessage `json:"permissions"`
		}
		if !ok || decided || json.Unmarshal(raw, &managed) != nil || managed.Permissions["disableBypassPermissionsMode"] != nil {
			return false
		}
	}
	sources := map[string]bool{"user": true, "project": true, "local": true}
	if value, set := optionValue(args, "--setting-sources"); set {
		sources = map[string]bool{}
		for _, part := range strings.Split(value, ",") {
			sources[strings.TrimSpace(part)] = true
		}
	}
	mode := ""
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
		value, decided, ok := permissionMode(raw, err)
		if !ok {
			return false
		}
		if decided {
			mode = value
		}
	}
	if permissions, present := user["permissions"]; present {
		value, decided, ok := permissionMode([]byte(`{"permissions":`+string(permissions)+`}`), nil)
		if !ok {
			return false
		}
		if decided {
			mode = value
		}
	}
	return mode == "bypassPermissions"
}

// permissionMode reads permissions.defaultMode; ok is false for a document it cannot read.
func permissionMode(raw []byte, readErr error) (mode string, decided, ok bool) {
	if readErr != nil {
		return "", false, false
	}
	var settings struct {
		Permissions map[string]json.RawMessage `json:"permissions"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return "", false, false
	}
	value, present := settings.Permissions["defaultMode"]
	if !present {
		return "", false, true
	}
	if json.Unmarshal(value, &mode) != nil {
		return "", false, false
	}
	return mode, true, true
}
