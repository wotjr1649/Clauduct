package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IsolationFacts is what an isolated launch tells the exit report: names and counts, never values.
type IsolationFacts struct {
	Mode      string   `json:"mode"`
	ConfigDir string   `json:"configDir"`
	Source    string   `json:"source"`
	Shared    []string `json:"shared,omitempty"`
	History   string   `json:"history,omitempty"`
	// Derived counts what was left behind when the user's files were rebuilt for the isolated folder.
	SettingsDropped int    `json:"settingsDropped,omitempty"`
	ClaudeJSONKeys  int    `json:"claudeJsonKeys,omitempty"`
	ClaudeMd        string `json:"claudeMd,omitempty"`
}

var errIsolationControl = errors.New("ISOLATION_MODE_UNREADABLE: ~/.clauduct/settings.json could not be read and isolated mode has been used on this machine, so this command was not sent to the default supervisor; fix the file and run it again")

// controlIsolation points a management command (rm, stop, logs) at the dedicated folder when
// isolated mode is on. It reads the mode leniently: those commands never needed a valid settings
// file, so a missing one (or an unreadable one on a machine that never used isolation) stays the
// default behavior. An unreadable one once isolation was used, or a key that is present and not
// "off" or "on", is refused.
func (o *Options) controlIsolation(home string) error {
	if home == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(home, ".clauduct", "settings.json"))
	var fields map[string]json.RawMessage
	if err != nil || len(raw) > clauductSettingsBytes || json.Unmarshal(raw, &fields) != nil {
		// A file that is simply absent keeps the default. One that exists but cannot be read, once
		// isolated mode has been used here, must not send the command to the default supervisor.
		if _, used := readIsolationMarker(dedicatedConfigDir(home)); used && !errors.Is(err, os.ErrNotExist) {
			return errIsolationControl
		}
		return nil
	}
	value, present := fields["isolation"]
	if !present {
		return nil
	}
	mode, err := parseIsolation(value)
	if err != nil || mode != isolationOn {
		return err
	}
	dedicated := dedicatedConfigDir(home)
	if _, _, err := isolationSource(o.Env, o.Cwd, dedicated); err != nil {
		return err
	}
	o.Env = isolationEnv(o.Env, dedicated)
	return nil
}

// applyIsolation makes a launch isolated when the settings (or, for a regenerated resident, its
// record) say so: the dedicated folder is made ready and what native reads there is shared or
// derived from the user's folder, then o.Env carries the forced environment, so every path
// below -- launch, provider check, control commands, job state -- names the dedicated folder. It
// returns the user's folder, which the product's own role and agent readers use directly, because
// a junction root is not something a directory walk descends into.
func (o *Options) applyIsolation(config *ClauductSettings, home string) (facts *IsolationFacts, source string, err error) {
	if o.Regenerate != nil {
		// The record decides, not today's settings: the job runs where it was dispatched.
		config.Isolation = isolationOff
		if o.Regenerate.Isolated {
			config.Isolation = isolationOn
		}
	}
	if !config.Isolated() {
		return nil, "", nil
	}
	dedicated := dedicatedConfigDir(home)
	var custom bool
	if o.Regenerate != nil {
		source = o.Regenerate.Source
	} else if source, custom, err = isolationSource(o.Env, o.Cwd, dedicated); err != nil {
		return nil, "", err
	}
	facts = &IsolationFacts{Mode: isolationOn, ConfigDir: dedicated, Source: source}
	// A regenerated resident starts no native: the job it serves already runs on what was built.
	if o.Regenerate == nil {
		if _, err := prepareDedicated(dedicated, source, custom, time.Now(), osIsolationHosts); err != nil {
			return nil, "", err
		}
		links, err := linkShared(dedicated, source)
		if err != nil {
			return nil, "", err
		}
		facts.Shared, facts.History = links.Shared, links.History
		derived, err := deriveFiles(dedicated, source, home, custom)
		if err != nil {
			return nil, "", err
		}
		facts.SettingsDropped, facts.ClaudeJSONKeys, facts.ClaudeMd = derived.SettingsDropped, derived.ClaudeJSONKeys, derived.ClaudeMd
		if o.Stderr != nil {
			fmt.Fprintf(o.Stderr, "clauduct: isolation on: %s (your configuration: %s)\n", dedicated, source)
			if derived.SettingsEdited {
				fmt.Fprintf(o.Stderr, "clauduct: isolation: settings changed inside the isolated session were not kept; edit %s\n", filepath.Join(source, "settings.json"))
			}
			if links.History == "unlinked" {
				fmt.Fprintln(o.Stderr, "clauduct: isolation: prompt history is not shared (the isolated history.jsonl is a separate file)")
			}
		}
	}
	o.Env = isolationEnv(o.Env, dedicated)
	return facts, source, nil
}

// isolationSourceOf is the user's config folder a resident record keeps, or "" for a launch that is not isolated.
func isolationSourceOf(facts *IsolationFacts) string {
	if facts == nil {
		return ""
	}
	return facts.Source
}

// errResumeRunningElsewhere refuses opening a session that a job in the other mode is running:
// the two natives would append to one transcript, and neither supervisor can see the other.
type errResumeRunningElsewhere struct{ Session, Job, Mode string }

func (e errResumeRunningElsewhere) Error() string {
	return fmt.Sprintf("RESUME_RUNNING_IN_OTHER_MODE: session %s is running as job %s in the %s mode; open it there (clauduct attach %s) or stop it first. A stopped session can be resumed from either mode", e.Session, e.Job, e.Mode, e.Job)
}

// resumeGuard refuses --resume <session> while a job of the other mode is running that session
// (#379). A stopped or finished job is not in the way. A jobs folder or state that cannot be read
// proves nothing, so it does not stop the user's resume; it is said on stderr instead. The default
// mode looks at the isolated folder only if isolated mode has been used on this machine.
func (o Options) resumeGuard(isolation *IsolationFacts, home, session string) error {
	if session == "" || o.Regenerate != nil || home == "" {
		return nil
	}
	other, mode := "", ""
	if isolation != nil {
		other, mode = isolation.Source, "default"
	} else if dedicated := dedicatedConfigDir(home); !sameFolder(dedicated, nativeConfigDirectory(o.Env, o.Cwd)) {
		if _, ok := readIsolationMarker(dedicated); ok {
			other, mode = dedicated, "isolated"
		}
	}
	if other == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(other, "jobs"))
	if err != nil {
		return nil // no jobs folder: nothing runs there
	}
	unreadable := 0
	for _, entry := range entries {
		if !entry.IsDir() || !residentJob.MatchString(entry.Name()) {
			continue
		}
		running, completed, err := jobRunning(other, entry.Name())
		if err != nil || running && !completed && nativeJobSession(other, entry.Name()) == "" {
			unreadable++
			continue
		}
		if running && !completed && nativeJobSession(other, entry.Name()) == session {
			return errResumeRunningElsewhere{Session: session, Job: entry.Name(), Mode: mode}
		}
	}
	if unreadable > 0 && o.Stderr != nil {
		fmt.Fprintf(o.Stderr, "clauduct: resume: %d job state(s) in the %s mode could not be read, so a running copy of this session there was not ruled out\n", unreadable, mode)
	}
	return nil
}
