package app

import (
	"encoding/json"
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

// controlIsolation points a management command (rm, stop, logs) at the dedicated folder when
// isolated mode is on. It reads the mode leniently: those commands never needed a valid settings
// file, so a missing or unreadable one stays the default behavior; only a key that is present and
// not "off" or "on" is refused.
func (o *Options) controlIsolation(home string) error {
	if home == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(home, ".clauduct", "settings.json"))
	var fields map[string]json.RawMessage
	if err != nil || len(raw) > clauductSettingsBytes || json.Unmarshal(raw, &fields) != nil {
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
		if !BackgroundRequested(o.Args) || o.Stderr != nil {
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
