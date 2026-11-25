package app

import (
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

var errIsolationControl = errors.New("ISOLATION_MODE_UNREADABLE: ~/.clauduct/settings.json could not be read as valid v1 settings, so the command was not sent to a supervisor; fix the file and run it again")

// controlIsolation points a management command (rm, stop, logs) at the dedicated folder when
// isolated mode is on. A missing file keeps the default. A file we cannot validate gives us no
// authority to choose a supervisor.
func (o *Options) controlIsolation(home string) error {
	if home == "" {
		return nil
	}
	settingsPath := filepath.Join(home, ".clauduct", "settings.json")
	raw, err := readIfExists(settingsPath, clauductSettingsBytes)
	if err == nil && raw == nil {
		if _, err := os.Lstat(settingsPath); errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil || len(raw) > clauductSettingsBytes {
		return errIsolationControl
	}
	selected, err := parseClauductSettings(raw)
	if err != nil {
		return errIsolationControl
	}
	if !selected.Isolated() {
		return nil
	}
	dedicated := dedicatedConfigDir(home)
	source, _, err := isolationSource(o.Env, o.Cwd, dedicated)
	if err != nil {
		return err
	}
	var settings []byte
	if shell, _ := windowsEnvValue(o.Env, pluginCacheDirEnv); shell == "" {
		if settings, err = readIsolationSettings(source); err != nil {
			return err
		}
	}
	root, err := resolveIsolationPluginRoot(o.Env, source, o.Cwd, settings)
	if err != nil {
		return err
	}
	if o.ManagedPolicy != nil {
		docs, err := o.ManagedPolicy()
		if err != nil {
			return err
		}
		if err := managedIsolationPluginRoot(docs, root, o.Cwd); err != nil {
			return err
		}
	}
	config := defaultClauductSettings()
	config.Isolation, config.pluginCacheRoot = isolationOn, root
	if _, _, _, err := config.takeUserSettings(o.Args, o.Cwd); err != nil {
		return err
	}
	o.Env = isolationEnv(o.Env, dedicated, root)
	return nil
}

// applyIsolation makes a launch isolated when the settings (or, for a regenerated resident, its
// record) say so: the dedicated folder is made ready and what native reads there is shared or
// derived from the user's folder, then o.Env carries the forced environment, so every path
// below -- launch, provider check, control commands, job state -- names the dedicated folder. It
// returns the user's folder, which the product's own role and agent readers use directly, because
// a junction root is not something a directory walk descends into.
func (o *Options) applyIsolation(config *ClauductSettings, home string, policy []policyDocument) (facts *IsolationFacts, source string, err error) {
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
	var settings []byte
	config.pluginCacheRoot = ""
	if o.Regenerate != nil {
		source = o.Regenerate.Source
		config.pluginCacheRoot = o.Regenerate.PluginCacheRoot
	} else if source, custom, err = isolationSource(o.Env, o.Cwd, dedicated); err != nil {
		return nil, "", err
	}
	if config.pluginCacheRoot == "" {
		if shell, _ := windowsEnvValue(o.Env, pluginCacheDirEnv); o.Regenerate == nil || shell == "" {
			if settings, err = readIsolationSettings(source); err != nil {
				return nil, "", err
			}
		}
		if config.pluginCacheRoot, err = resolveIsolationPluginRoot(o.Env, source, o.Cwd, settings); err != nil {
			return nil, "", err
		}
	}
	if err := managedIsolationPluginRoot(policy, config.pluginCacheRoot, o.Cwd); err != nil {
		return nil, "", err
	}
	facts = &IsolationFacts{Mode: isolationOn, ConfigDir: dedicated, Source: source}
	// A regenerated resident starts no native: the job it serves already runs on what was built.
	if o.Regenerate == nil {
		if _, err := prepareDedicated(dedicated, source, custom, time.Now(), osIsolationHosts); err != nil {
			return nil, "", err
		}
		if err := removeLegacyPluginJunction(dedicated, source); err != nil {
			return nil, "", err
		}
		links, err := linkShared(dedicated, source)
		if err != nil {
			return nil, "", err
		}
		facts.Shared, facts.History = links.Shared, links.History
		derived, err := deriveFiles(dedicated, source, home, custom, settings)
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
	o.Env = isolationEnv(o.Env, dedicated, config.pluginCacheRoot)
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
		if isolationMarkerPresent(dedicated) {
			other, mode = dedicated, "isolated"
		}
	}
	if other == "" {
		return nil
	}
	jobs := filepath.Join(other, "jobs")
	info, err := os.Stat(jobs)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var entries []os.DirEntry
	// On Windows, ReadDir can report an ordinary file as an empty directory.
	if err == nil && info.IsDir() {
		entries, err = os.ReadDir(jobs)
	}
	if err != nil || !info.IsDir() {
		if o.Stderr != nil {
			fmt.Fprintf(o.Stderr, "clauduct: resume: the jobs folder in the %s mode could not be read, so a running copy of this session there was not ruled out\n", mode)
		}
		return nil
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
