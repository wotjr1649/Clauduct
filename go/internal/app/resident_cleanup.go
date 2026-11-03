package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// M1 determines whether two separate executions must observe absence.
const residentRecheckAcrossRuns = false

type reclaimOutcome struct {
	Link     string
	Category string
}

// Used only by the crash test, after directory removal and before record removal.
var reclaimInterrupt func()

func collectResidents(home string) []reclaimOutcome {
	if home == "" {
		return []reclaimOutcome{{Category: "SWEEP_SKIPPED"}}
	}
	dir := filepath.Join(home, ".clauduct", "residents")
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil // A home with no resident records has nothing to reclaim.
		}
		if err != nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return []reclaimOutcome{{Category: "SWEEP_SKIPPED"}}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []reclaimOutcome{{Category: "SWEEP_SKIPPED"}}
	}
	var links []string
	refs := map[string]int{}
	for _, entry := range entries {
		link := strings.TrimSuffix(entry.Name(), ".json")
		if entry.Name() != link+".json" || !residentLink.MatchString(link) {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		links = append(links, link)
		// An otherwise invalid record may still claim this directory; preserve that claim.
		if rec, _ := LoadResident(home, link); eventsDirValid(rec.PluginDir) {
			refs[strings.ToLower(filepath.Clean(rec.PluginDir))]++
		}
	}
	var outcomes []reclaimOutcome
	for _, link := range links {
		release, err := lockResident(home, link, 0)
		category := "KEPT_LOCK_FAILED"
		if errors.Is(err, errResidentBusy) {
			category = "KEPT_ACTIVE"
		} else if err == nil {
			category = reclaimResident(home, link, refs)
			release()
		}
		outcomes = append(outcomes, reclaimOutcome{link, category})
	}
	return outcomes
}

func reclaimResident(home, link string, refs map[string]int) string {
	info, err := os.Lstat(residentPath(home, link))
	if err != nil || !info.Mode().IsRegular() {
		return "KEPT_RECORD_INVALID"
	}
	rec, err := LoadResident(home, link)
	if err != nil {
		return "KEPT_RECORD_INVALID"
	}
	if rec.Config == "" {
		return "KEPT_NO_CONFIG"
	}
	jobs := append([]string{rec.Job}, rec.Jobs...)
	listed, known := jobListing(rec.Config, jobs)
	if listed || !known {
		if residentRecheckAcrossRuns && rec.AbsentObserved {
			rec.AbsentObserved = false
			if writeResident(home, rec) != nil {
				return "KEPT_RECORD_INVALID"
			}
		}
		if listed {
			return "KEPT_LISTED"
		}
		return "KEPT_LISTING_UNKNOWN"
	}
	if rec.PluginDir != "" && refs[strings.ToLower(filepath.Clean(rec.PluginDir))] != 1 {
		return "KEPT_SHARED_DIR"
	}
	if residentRecheckAcrossRuns && !rec.AbsentObserved {
		rec.AbsentObserved = true
		if writeResident(home, rec) != nil {
			return "KEPT_RECORD_INVALID"
		}
		return "KEPT_RECHECK_PENDING"
	}
	listed, known = jobListing(rec.Config, jobs)
	if listed || !known {
		if residentRecheckAcrossRuns && rec.AbsentObserved {
			rec.AbsentObserved = false
			if writeResident(home, rec) != nil {
				return "KEPT_RECORD_INVALID"
			}
		}
		if listed {
			return "KEPT_LISTED"
		}
		return "KEPT_LISTING_UNKNOWN"
	}
	if rec.PluginDir != "" {
		if err := removeEventsDir(rec.PluginDir); errors.Is(err, errEventsPathRefused) {
			return "KEPT_PATH_REFUSED"
		} else if err != nil {
			return "KEPT_DELETE_FAILED"
		}
	}
	if reclaimInterrupt != nil {
		reclaimInterrupt()
	}
	if removeResident(home, link) != nil {
		return "RECORD_DELETE_FAILED"
	}
	return "RECLAIMED"
}

func reportResidentCleanup(out io.Writer, outcomes []reclaimOutcome) {
	n := 0
	for _, outcome := range outcomes {
		switch outcome.Category {
		case "KEPT_PATH_REFUSED", "KEPT_DELETE_FAILED", "RECORD_DELETE_FAILED", "KEPT_LOCK_FAILED", "SWEEP_SKIPPED":
			n++
		}
	}
	if n > 0 {
		fmt.Fprintf(out, "clauduct: RESIDENT_CLEANUP_INCOMPLETE: %d retained background job resources could not be removed; the next start tries again\n", n)
	}
}
