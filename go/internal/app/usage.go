package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Reading back what sessions wrote.
//
// The quota this reports is the backend's, and it is the number a subscription user
// actually needs: how much of the weekly window is gone. The client cannot show it --
// measured, its own /usage asks an Anthropic account endpoint and skips the request
// entirely against a gateway -- but every session here already reads it out of the response
// headers and writes it down. What was missing was somewhere to read it from.
//
// Nothing here makes a request. A cached reading is the honest thing to have, as long as it
// says when it was taken; a number presented as current when it is an hour old is worse
// than no number.

// Reading is one session's account with the time it was written.
type Reading struct {
	Status Status
	// Taken is the file's modification time, which is when the session ended.
	Taken time.Time
	// Path is where it was read from, so a reader can go and look.
	Path string
}

// Age is how old the reading is.
func (r Reading) Age() time.Duration { return time.Since(r.Taken) }

// Readings returns the accounts sessions have left behind, newest first.
//
// dir empty means the directory sessions write to. A file that does not decode is skipped
// rather than reported: the directory is a temporary one and anything may be in it, and a
// half-written file from a session that is still running is expected rather than wrong.
func Readings(dir string, limit int) []Reading {
	if dir == "" {
		dir = statusDir()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	readings := make([]Reading, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "status-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var status Status
		if json.Unmarshal(raw, &status) != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		readings = append(readings, Reading{Status: status, Taken: info.ModTime(), Path: path})
	}

	sort.Slice(readings, func(i, j int) bool { return readings[i].Taken.After(readings[j].Taken) })
	if limit > 0 && len(readings) > limit {
		readings = readings[:limit]
	}
	return readings
}

// LatestQuota returns the newest reading that actually carries a figure.
//
// Newest-with-one rather than newest: a session that asked nothing of the backend has no
// headers to report, and taking its empty account would hide a reading from ten minutes ago
// behind one from ten seconds ago that says nothing.
//
// A figure, not a report. The observation is recorded even when the response carried only
// an active-limit name, or another family, or a field this build could not read -- so
// testing the report for existence accepted accounts with no percentage in them, printed a
// state and nothing else, and buried the last real reading. The same predicate the exit
// line uses, which had it right.
func LatestQuota(dir string) (Reading, bool) {
	for _, reading := range Readings(dir, 0) {
		if hasFigure(reading.Status) {
			return reading, true
		}
	}
	return Reading{}, false
}

// hasFigure reports whether an account carries a usable percentage.
func hasFigure(status Status) bool {
	limits := status.Gateway.Limits
	return limits != nil && limits.Primary != nil && limits.Primary.UsedPercent != nil
}
