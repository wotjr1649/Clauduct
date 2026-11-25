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
	walkNativeJobs(config, link, func(job string, _ []byte) bool {
		jobs = append(jobs, job)
		return true
	})
	return jobs
}

func walkNativeJobs(config, link string, visit func(job string, raw []byte) bool) {
	if config == "" || !residentLink.MatchString(link) {
		return
	}
	root, err := os.OpenRoot(filepath.Join(config, "jobs"))
	if err != nil {
		return
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil || closeErr != nil {
		return
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
		if !visit(job, raw) {
			return
		}
	}
}

func jobHasLink(raw []byte, link string) bool {
	return link != "" && nativeJobLink(raw) == link
}

func nativeJobLink(raw []byte) string {
	if len(raw) > settingsBytes {
		return ""
	}
	state, err := wire.Fields(raw, nil)
	if err != nil {
		return ""
	}
	var flags []string
	if json.Unmarshal(state["respawnFlags"], &flags) != nil {
		return ""
	}
	seen := map[string]bool{}
	var settings string
	for i := 0; i < len(flags); {
		end, known := nativeArgEnd(flags, i)
		name, value, attached := strings.Cut(flags[i], "=")
		if !known || end > len(flags) || !strings.HasPrefix(name, "-") || name == "-" || name == "--" || seen[name] {
			return ""
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
		return ""
	}
	var helper string
	if json.Unmarshal(fields["apiKeyHelper"], &helper) != nil {
		return ""
	}
	prefix, linked, found := strings.Cut(helper, " --clauduct-background-key ")
	if !found || !residentLink.MatchString(linked) || len(prefix) < 3 || prefix[0] != '"' || prefix[len(prefix)-1] != '"' {
		return ""
	}
	exe := prefix[1 : len(prefix)-1]
	if !filepath.IsAbs(exe) || strings.ContainsRune(exe, '"') {
		return ""
	}
	return linked
}
