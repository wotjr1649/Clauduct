package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

const eventsOwnerFile = "foreground-owner"

type eventsOwner struct {
	PID     uint32
	Created uint64
}

func (o eventsOwner) text() string {
	return fmt.Sprintf("clauduct-foreground-owner-v1\n%d %d\n", o.PID, o.Created)
}

func currentEventsOwner() (eventsOwner, error) {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return eventsOwner{}, err
	}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return eventsOwner{}, err
	}
	return eventsOwner{uint32(os.Getpid()), uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)}, nil
}

func writeEventsOwner(dir string) error {
	owner, err := currentEventsOwner()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.OpenFile(eventsOwnerFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(f, owner.text())
	return errors.Join(writeErr, f.Close())
}

func eventsPathInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attrs, err := syscall.GetFileAttributes(name)
	if err != nil {
		return nil, err
	}
	if attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errEventsPathRefused
	}
	if info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		// Windows Lstat loads file IDs lazily; a handle Stat fixes the identity now.
		info, statErr := f.Stat()
		return info, errors.Join(statErr, f.Close())
	}
	return info, nil
}

func readEventsOwner(dir string) (eventsOwner, error) {
	info, err := eventsPathInfo(filepath.Join(dir, eventsOwnerFile))
	if err != nil || !info.Mode().IsRegular() {
		return eventsOwner{}, errors.New("EVENTS_OWNER_UNREADABLE")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return eventsOwner{}, err
	}
	defer root.Close()
	f, err := root.Open(eventsOwnerFile)
	if err != nil {
		return eventsOwner{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 128))
	if err != nil {
		return eventsOwner{}, err
	}
	var owner eventsOwner
	if n, err := fmt.Sscanf(string(raw), "clauduct-foreground-owner-v1\n%d %d\n", &owner.PID, &owner.Created); err != nil || n != 2 || owner.PID == 0 || owner.Created == 0 || string(raw) != owner.text() {
		return eventsOwner{}, errors.New("EVENTS_OWNER_INVALID")
	}
	return owner, nil
}

var eventsProcessOpen = syscall.OpenProcess

func eventsOwnerAlive(owner eventsOwner) (bool, error) {
	h, err := eventsProcessOpen(0x00100000|0x1000, false, owner.PID) // SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION
	// ERROR_INVALID_PARAMETER means there is no such PID; access failures prove nothing.
	if errors.Is(err, syscall.Errno(87)) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(h)
	state, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return false, err
	}
	if state == syscall.WAIT_OBJECT_0 {
		return false, nil
	}
	if state != syscall.WAIT_TIMEOUT {
		return false, errors.New("EVENTS_OWNER_UNKNOWN")
	}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return false, err
	}
	return uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) == owner.Created, nil
}

// An unreadable record may name any directory, so it prevents this foreground sweep.
func residentEventsRefs(home string) (map[string]bool, error) {
	refs := map[string]bool{}
	if home == "" {
		return nil, errors.New("RESIDENT_HOME_UNKNOWN")
	}
	dir := filepath.Join(home, ".clauduct", "residents")
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := eventsPathInfo(path)
		if errors.Is(err, os.ErrNotExist) {
			return refs, nil
		}
		if err != nil || !info.IsDir() {
			return nil, errors.New("RESIDENT_REFERENCES_UNKNOWN")
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := eventsPathInfo(filepath.Join(dir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("RESIDENT_REFERENCES_UNKNOWN")
		}
		f, err := root.Open(entry.Name())
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		closeErr := f.Close()
		var record struct {
			PluginDir string `json:"pluginDir"`
		}
		if readErr != nil || closeErr != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &record) != nil {
			return nil, errors.New("RESIDENT_REFERENCES_UNKNOWN")
		}
		if record.PluginDir != "" {
			refs[strings.ToLower(filepath.Clean(record.PluginDir))] = true
		}
	}
	return refs, nil
}

// Used only by tests to change ownership or references before the final recheck.
var foregroundEventsRecheck func()

func reclaimForegroundEvents(home, dir string, refs map[string]bool) string {
	if !filepath.IsAbs(dir) || !eventsDirValid(dir) {
		return "KEPT_PATH_REFUSED"
	}
	info, err := eventsPathInfo(dir)
	if err != nil || !info.IsDir() {
		return "KEPT_PATH_REFUSED"
	}
	key := strings.ToLower(filepath.Clean(dir))
	if refs[key] {
		return "KEPT_RESIDENT"
	}
	owner, err := readEventsOwner(dir)
	if err != nil {
		return "KEPT_OWNER_INVALID"
	}
	if alive, err := eventsOwnerAlive(owner); err != nil {
		return "KEPT_OWNER_UNKNOWN"
	} else if alive {
		return "KEPT_ACTIVE"
	}
	refused := ""
	err = removeEventsDirChecked(dir, func() error {
		if foregroundEventsRecheck != nil {
			foregroundEventsRecheck()
		}
		refused = recheckForegroundEvents(home, dir, info, owner)
		if refused != "" {
			return errors.New("FOREGROUND_EVENTS_RECHECK_FAILED")
		}
		return nil
	})
	if refused != "" {
		return refused
	}
	if errors.Is(err, errEventsPathRefused) {
		return "KEPT_PATH_REFUSED"
	} else if err != nil {
		return "KEPT_DELETE_FAILED"
	}
	return "RECLAIMED"
}

func recheckForegroundEvents(home, dir string, info os.FileInfo, owner eventsOwner) string {
	refs, err := residentEventsRefs(home)
	if err != nil {
		return "SWEEP_SKIPPED"
	}
	if refs[strings.ToLower(filepath.Clean(dir))] {
		return "KEPT_RESIDENT"
	}
	current, err := eventsPathInfo(dir)
	if err != nil || !current.IsDir() || !os.SameFile(info, current) {
		return "KEPT_PATH_REFUSED"
	}
	verified, err := readEventsOwner(dir)
	if err != nil || verified != owner {
		return "KEPT_OWNER_INVALID"
	}
	if alive, err := eventsOwnerAlive(verified); err != nil {
		return "KEPT_OWNER_UNKNOWN"
	} else if alive {
		return "KEPT_ACTIVE"
	}
	return ""
}

func collectForegroundEvents(home string) []reclaimOutcome {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return []reclaimOutcome{{Category: "SWEEP_SKIPPED"}}
	}
	var outcomes []reclaimOutcome
	refs, err := residentEventsRefs(home)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), nativeEventsPrefix) {
			continue
		}
		category := "SWEEP_SKIPPED"
		if err == nil {
			category = reclaimForegroundEvents(home, filepath.Join(os.TempDir(), entry.Name()), refs)
		}
		outcomes = append(outcomes, reclaimOutcome{Category: category})
	}
	return outcomes
}

func reportForegroundEventsCleanup(out io.Writer, outcomes []reclaimOutcome) {
	counts := map[string]int{}
	for _, outcome := range outcomes {
		// R3 D2 leaves legacy ownerless resources to manual cleanup; reporting them on every exit is noise.
		switch outcome.Category {
		case "KEPT_PATH_REFUSED", "KEPT_DELETE_FAILED", "KEPT_OWNER_UNKNOWN", "SWEEP_SKIPPED":
			counts[outcome.Category]++
		}
	}
	var categories []string
	for category, count := range counts {
		categories = append(categories, fmt.Sprintf("%s=%d", category, count))
	}
	if len(categories) > 0 {
		slices.Sort(categories)
		fmt.Fprintf(out, "clauduct: FOREGROUND_EVENT_CLEANUP_INCOMPLETE: %s; retained resources will be checked at the next exit\n", strings.Join(categories, ", "))
	}
}
