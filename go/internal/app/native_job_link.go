package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Only the job's current helper identifies the gateway its next worker can reach.
func nativeJobsWithLink(config, link string) []string {
	var jobs []string
	if config == "" || !residentLink.MatchString(link) {
		return jobs
	}
	root, err := os.OpenRoot(filepath.Join(config, "jobs"))
	if err != nil {
		return jobs
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return jobs
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil || closeErr != nil {
		return jobs
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	for _, entry := range entries {
		job := entry.Name()
		if !entry.IsDir() || !residentJob.MatchString(job) {
			continue
		}
		file, err := root.Open(filepath.Join(job, "state.json"))
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > settingsBytes {
			file.Close()
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, settingsBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(raw) > settingsBytes || !jobHasLink(raw, link) {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func jobHasLink(raw []byte, link string) bool {
	if len(raw) > settingsBytes || !residentLink.MatchString(link) {
		return false
	}
	state, err := wire.Fields(raw, nil)
	if err != nil {
		return false
	}
	var flags []string
	if json.Unmarshal(state["respawnFlags"], &flags) != nil {
		return false
	}
	seen := map[string]bool{}
	var settings string
	for i := 0; i < len(flags); {
		end, known := nativeArgEnd(flags, i)
		name, value, attached := strings.Cut(flags[i], "=")
		if !known || end > len(flags) || !strings.HasPrefix(name, "-") || name == "-" || name == "--" || seen[name] {
			return false
		}
		seen[name] = true
		if name == "--settings" {
			if !attached {
				value = flags[i+1]
			}
			settings = value
		}
		i = end
	}
	fields, err := wire.Fields([]byte(settings), nil)
	if err != nil {
		return false
	}
	var helper string
	if json.Unmarshal(fields["apiKeyHelper"], &helper) != nil {
		return false
	}
	prefix, linked, found := strings.Cut(helper, " --clauduct-background-key ")
	if !found || linked != link || len(prefix) < 3 || prefix[0] != '"' || prefix[len(prefix)-1] != '"' {
		return false
	}
	exe := prefix[1 : len(prefix)-1]
	return filepath.IsAbs(exe) && !strings.ContainsRune(exe, '"')
}
