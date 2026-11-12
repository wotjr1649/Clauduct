package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// IsolationReport says, for clauduct --dev --doctor, what isolated mode is doing on this machine:
// the mode, where the dedicated folder is and what it was built from, which names are shared, and
// whether a supervisor is running there. It reads; it changes nothing.
func IsolationReport(out io.Writer, home string) {
	if home == "" {
		return
	}
	mode := isolationOff
	if raw, err := os.ReadFile(filepath.Join(home, ".clauduct", "settings.json")); err == nil && len(raw) <= clauductSettingsBytes {
		if config, err := parseClauductSettings(raw); err == nil {
			mode = config.Isolation
		} else if strings.Contains(string(raw), `"isolation"`) {
			mode = "unreadable (settings.json is invalid)"
		}
	}
	dedicated := dedicatedConfigDir(home)
	marker, haveMarker := readIsolationMarker(dedicated)
	if mode == isolationOff && !haveMarker {
		return
	}
	fmt.Fprintf(out, "isolation    %s\n", mode)
	if !haveMarker {
		fmt.Fprintln(out, "isolation    the isolated folder is not made yet (the first isolated launch makes it)")
		return
	}
	fmt.Fprintf(out, "isolation    folder %s, built from %s since %s\n", dedicated, marker.Source, marker.CreatedAt)
	var linked, wrong []string
	for _, entry := range sharedEntries {
		if entry.kind != sharedJunction {
			continue
		}
		to := filepath.Join(dedicated, entry.name)
		if target, ok := isJunction(to); ok && sameFolder(target, filepath.Join(marker.Source, entry.name)) {
			linked = append(linked, entry.name)
		} else if _, err := os.Lstat(to); err == nil {
			wrong = append(wrong, entry.name)
		}
	}
	fmt.Fprintf(out, "isolation    shared %d folders", len(linked))
	if len(wrong) > 0 {
		fmt.Fprintf(out, "; NOT the expected link: %s", strings.Join(wrong, ", "))
	}
	fmt.Fprintln(out)
	if pid := osIsolationHosts.lockOwner(dedicated); pid != 0 {
		fmt.Fprintf(out, "isolation    a native supervisor is running there (pid %d)\n", pid)
	}
}

// IsolationReset removes the dedicated folder: the links first, so that nothing reaches the
// user's own folder, then what is left. It refuses while a supervisor runs there, and without
// --yes it only says what it would remove. A shared name that is a real directory (not the link
// isolated mode makes) is left alone, and the folder is then kept.
func IsolationReset(args []string, out, errOut io.Writer, home string) int {
	yes := len(args) == 1 && args[0] == "--yes"
	if len(args) > 1 || len(args) == 1 && !yes {
		fmt.Fprintln(errOut, "usage: clauduct --dev --isolation-reset [--yes]")
		return 2
	}
	dedicated := dedicatedConfigDir(home)
	if home == "" {
		fmt.Fprintln(errOut, "clauduct: ISOLATION_RESET_UNAVAILABLE: the home folder is unknown")
		return 1
	}
	if _, err := os.Lstat(dedicated); err != nil {
		fmt.Fprintln(out, "nothing to reset: the isolated folder does not exist")
		return 0
	}
	// The same guard a launch uses: a link above or at the folder would point the removal elsewhere.
	for _, dir := range []string{filepath.Dir(filepath.Dir(dedicated)), filepath.Dir(dedicated), dedicated} {
		if _, err := plainDirectory(dir); err != nil {
			fmt.Fprintf(errOut, "clauduct: ISOLATION_RESET_REFUSED: %v\n", err)
			return 1
		}
	}
	if pid := osIsolationHosts.lockOwner(dedicated); pid != 0 {
		fmt.Fprintf(errOut, "clauduct: ISOLATION_RESET_REFUSED: a native supervisor (pid %d) is running for the isolated folder; stop it first (claude daemon stop --any with CLAUDE_CONFIG_DIR=%s)\n", pid, dedicated)
		return 1
	}
	fmt.Fprintf(out, "remove %s: its jobs, state and derived files; the links to your own folders are taken off first and your own files are not touched\n", dedicated)
	if !yes {
		fmt.Fprintln(out, "nothing was removed; run again with --yes")
		return 1
	}
	left, err := removeSharedLinks(dedicated)
	if err != nil {
		fmt.Fprintf(errOut, "clauduct: ISOLATION_RESET_FAILED: %v\n", err)
		return 1
	}
	if len(left) > 0 {
		fmt.Fprintf(errOut, "clauduct: ISOLATION_RESET_REFUSED: %s in the isolated folder is a real directory, not a link; it may hold your data, so nothing else was removed\n", strings.Join(left, ", "))
		return 1
	}
	if err := os.RemoveAll(dedicated); err != nil {
		fmt.Fprintf(errOut, "clauduct: ISOLATION_RESET_FAILED: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "removed")
	return 0
}

// isolationField is the exit line's note for an isolated session; empty otherwise.
func isolationField(status Status) string {
	if status.Session.Isolation == nil {
		return ""
	}
	return " isolation=on"
}
