package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// Files the dedicated folder derives from the user's own on every isolated launch (#379). They are
// rebuilt rather than linked because they mix what the user wrote with what ties native to a
// Claude account, and only the first may come across. The user's files are only read.

const (
	deriveSettingsBytes   = 1 << 20
	deriveClaudeJSONBytes = 8 << 20
	derivedMarker         = ".clauduct-derived.json"
)

var errIsolationDedicatedConfig = errors.New("ISOLATION_DEDICATED_CONFIG_UNREADABLE: the isolated folder's .claude.json could not be read (locked, not a file, or over 8 MiB), so it was not replaced; nothing was started. If another program holds it, try again; only if it is not a file or is over 8 MiB, run clauduct --dev --isolation-reset, which also removes the isolated jobs and state")

var errIsolationSettingsFile = errors.New("ISOLATION_SETTINGS_UNREADABLE: your settings.json could not be read as a JSON object within 1 MiB, so the isolated settings could not be built from it; nothing was started")

// settingsDropped are the top-level settings keys that sign native in, or choose how, so they stay
// behind. Native's own settings reference decides the list; a test fixes it to that reading.
var settingsDropped = []string{"apiKeyHelper", "awsAuthRefresh", "awsCredentialExport", "gcpAuthRefresh",
	"otelHeadersHelper", "forceLoginMethod", "forceLoginOrgUUID", "forceLoginGatewayUrl"}

// settingsForced are keys an isolated settings.json always carries, whatever the user's says: no
// account sync, connectors, Remote Control, or feedback.
var settingsForced = map[string]string{"syncClaudeAiSkills": "false", "syncClaudeAiPlugins": "false",
	"remoteControlAtStartup": "false", "disableClaudeAiConnectors": "true", "disableRemoteControl": "true",
	"feedbackDrafts": `"off"`, "feedbackSurveyRate": "0"}

var settingsEnvForced = map[string]string{
	nonessentialTraffic: "1", "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1",
	"CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY": "1", "ENABLE_CLAUDEAI_MCP_SERVERS": "false",
}

// settingsEnvDropped reports an `env` name that would route requests, sign native in, or turn
// traffic or telemetry back on. Forced values also replace differently cased spellings.
func settingsEnvDropped(name string) bool {
	upper := strings.ToUpper(name)
	if _, forced := settingsEnvForced[upper]; forced {
		return true
	}
	// The fast-model region name is a routing setting despite containing _MODEL.
	switch {
	case upper == "ANTHROPIC_SMALL_FAST_MODEL_AWS_REGION":
		return true
	case strings.HasPrefix(upper, "ANTHROPIC_"):
		// Model selection is the user's own (ANTHROPIC_MODEL, ANTHROPIC_DEFAULT_*_MODEL ...) and works as
		// in the default mode; endpoint, credential and header names are not.
		return !strings.Contains(upper, "_MODEL")
	case strings.HasPrefix(upper, "OTEL_"):
		return true
	case upper == "CLAUDE_CODE_OAUTH_TOKEN", upper == "CLAUDE_CODE_OAUTH_REFRESH_TOKEN", upper == "CLAUDE_CODE_OAUTH_SCOPES",
		upper == launch.HostRoutedEnv, upper == "CLAUDE_CONFIG_DIR", upper == pluginCacheDirEnv,
		upper == "CLAUDE_CODE_ENABLE_TELEMETRY", upper == "CLAUDE_CODE_ENABLE_FEEDBACK_SURVEY_FOR_OTEL",
		upper == "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA", upper == "ENABLE_ENHANCED_TELEMETRY_BETA",
		upper == "ENABLE_BETA_TRACING_DETAILED", upper == "BETA_TRACING_ENDPOINT", upper == "CLAUDE_CODE_PROPAGATE_TRACEPARENT",
		upper == "CLAUDE_CODE_SEND_FEEDBACK", upper == "FORCE_AUTOUPDATE_PLUGINS", upper == "CLAUDE_CODE_PACKAGE_MANAGER_AUTO_UPDATE",
		upper == "CLAUDE_CODE_SYNC_SKILLS", upper == "CLAUDE_CODE_SYNC_PLUGINS",
		upper == "CLAUDE_CODE_SKIP_ANTHROPIC_AWS_AUTH", upper == "CLAUDE_CODE_SKIP_ANTHROPIC_GOOGLE_CLOUD_AUTH",
		upper == "CLAUDE_CODE_SKIP_BEDROCK_AUTH", upper == "CLAUDE_CODE_SKIP_FOUNDRY_AUTH",
		upper == "CLAUDE_CODE_SKIP_MANTLE_AUTH", upper == "CLAUDE_CODE_SKIP_VERTEX_AUTH", upper == "AWS_BEARER_TOKEN_BEDROCK":
		return true
	}
	for _, name := range launch.ProviderSwitches {
		if upper == name {
			return true
		}
	}
	return false
}

// deriveSettings builds the isolated settings.json from the user's. Everything the user wrote is
// kept as written except what settingsDropped and settingsEnvDropped name; values are never
// logged. raw may be nil (no file): the result then carries only the forced keys.
func deriveSettings(raw []byte) (out []byte, dropped int, err error) {
	fields := map[string]json.RawMessage{}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(raw)) > 0 {
		if len(raw) > deriveSettingsBytes || json.Unmarshal(raw, &fields) != nil || fields == nil {
			return nil, 0, errIsolationSettingsFile
		}
	}
	for _, key := range settingsDropped {
		if _, present := fields[key]; present {
			delete(fields, key)
			dropped++
		}
	}
	for key, value := range settingsForced {
		fields[key] = json.RawMessage(value)
	}
	env := map[string]json.RawMessage{}
	if value, present := fields["env"]; present {
		if json.Unmarshal(value, &env) != nil || env == nil {
			env, dropped = map[string]json.RawMessage{}, dropped+1 // not an object: native would ignore it too
		}
	}
	for name := range env {
		if settingsEnvDropped(name) {
			delete(env, name)
			dropped++
		}
	}
	for key, value := range settingsEnvForced {
		env[key] = json.RawMessage(`"` + value + `"`)
	}
	fields["env"], err = json.Marshal(env)
	if err != nil {
		return nil, 0, err
	}
	out, err = json.MarshalIndent(fields, "", "  ")
	return append(out, '\n'), dropped, err
}

// claudeJSONTop and claudeJSONProject list the settings mirrored from native's global config file.
// Account fields, user and device identifiers, telemetry and per-project history stay out.
// Onboarding and trust are imported separately to keep approvals native made in the dedicated folder.
var claudeJSONTop = []string{"mcpServers", "autoConnectIde", "autoInstallIdeExtension", "claudeInChromeDefaultEnabled",
	"copyFullResponse", "copyOnSelect", "defaultToAgentsView", "diffTool", "externalEditorContext",
	"leftArrowOpensAgents", "prStatusFooterEnabled"}

var claudeJSONProject = []string{"mcpServers", "enabledMcpServers", "disabledMcpServers"}

// deriveClaudeJSON merges the user's allowlisted keys into the dedicated global config file and
// keeps native's other state. Missing keys in the lists above are removed, so deleting an MCP
// server in the user's config removes it here too. Onboarding and trust are updated only when the
// source supplies a boolean. changed is false when the result matches the dedicated file.
func deriveClaudeJSON(userRaw, dedicatedRaw []byte) (out []byte, keys int, changed bool, err error) {
	user, dedicated := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if userRaw != nil && (len(userRaw) > deriveClaudeJSONBytes || json.Unmarshal(userRaw, &user) != nil || user == nil) {
		return nil, 0, false, errors.New("ISOLATION_CLAUDE_JSON_UNREADABLE: your global config file could not be read as a JSON object; nothing was started")
	}
	if dedicatedRaw != nil && json.Unmarshal(dedicatedRaw, &dedicated) != nil || dedicated == nil {
		dedicated = map[string]json.RawMessage{} // a damaged derived file is rebuilt, native recreates its own state
	}
	for _, key := range claudeJSONTop {
		if value, present := user[key]; present {
			dedicated[key] = value
			keys++
		} else {
			delete(dedicated, key)
		}
	}
	// In the 2026-11-25 synthetic run of native 2.1.331, hasCompletedOnboarding=true
	// skipped theme and security prompts. The project boolean below records folder approval.
	switch value := user["hasCompletedOnboarding"]; string(value) {
	case "true", "false":
		dedicated["hasCompletedOnboarding"] = value
		keys++
	}
	userProjects, dedicatedProjects := map[string]map[string]json.RawMessage{}, map[string]map[string]json.RawMessage{}
	_ = json.Unmarshal(user["projects"], &userProjects)
	_ = json.Unmarshal(dedicated["projects"], &dedicatedProjects)
	if dedicatedProjects == nil {
		dedicatedProjects = map[string]map[string]json.RawMessage{}
	}
	for path, entry := range userProjects {
		if dedicatedProjects[path] == nil {
			dedicatedProjects[path] = map[string]json.RawMessage{}
		}
		switch value := entry["hasTrustDialogAccepted"]; string(value) {
		case "true", "false":
			dedicatedProjects[path]["hasTrustDialogAccepted"] = value
			keys++
		}
		for _, key := range claudeJSONProject {
			if value, present := entry[key]; present {
				dedicatedProjects[path][key] = value
				keys++
			} else {
				delete(dedicatedProjects[path], key)
			}
		}
	}
	for path, entry := range dedicatedProjects {
		if _, known := userProjects[path]; !known {
			for _, key := range claudeJSONProject {
				delete(entry, key)
			}
		}
		if len(entry) == 0 {
			delete(dedicatedProjects, path)
		}
	}
	if len(dedicatedProjects) > 0 {
		dedicated["projects"], err = json.Marshal(dedicatedProjects)
		if err != nil {
			return nil, 0, false, err
		}
	} else {
		delete(dedicated, "projects")
	}
	out, err = json.MarshalIndent(dedicated, "", "  ")
	if err != nil {
		return nil, 0, false, err
	}
	out = append(out, '\n')
	// Same content, not same bytes: native formats and orders its own file as it likes.
	var was, now any
	return out, keys, dedicatedRaw == nil || json.Unmarshal(dedicatedRaw, &was) != nil || json.Unmarshal(out, &now) != nil || !reflect.DeepEqual(was, now), nil
}

// mergeGlobalConfig brings the dedicated global config file up to date with the user's, touching
// only the imported keys, and returns how many it imported. A running isolated native rewrites this
// file with its own state, so the merge is done from what is on disk right before the replace: if the
// file changed since it was read, it is read and merged again (up to three times; the last merge is
// written without a further check). That narrows the window in which an update of native's own keys
// could be lost to the moment between the last read and the rename. It cannot close it: native does
// not take a lock this code could share, and Windows has no compare-and-swap for a file. beforeWrite
// is a test hook, called after a merge that needs writing and before the file is read again.
func mergeGlobalConfig(path string, userRaw []byte, beforeWrite func()) (keys int, err error) {
	const attempts = 3
	for attempt := 0; ; attempt++ {
		current, readErr := readIfExists(path, deriveClaudeJSONBytes)
		if readErr != nil {
			// A read that failed is not an absent file: replacing it would drop native's own state
			// (trust approvals) on a transient error. Refused; a retry or --isolation-reset recovers.
			return 0, errIsolationDedicatedConfig
		}
		out, n, changed, deriveErr := deriveClaudeJSON(userRaw, current)
		if deriveErr != nil {
			return 0, deriveErr
		}
		if !changed {
			return n, nil
		}
		if beforeWrite != nil {
			beforeWrite()
		}
		if attempt == attempts {
			return n, writeFileAtomic(path, out)
		}
		// A re-read that fails is not "unchanged" (nil equals an absent first read): go round, and
		// the next first read refuses it if it still fails.
		if again, againErr := readIfExists(path, deriveClaudeJSONBytes); againErr == nil && bytes.Equal(again, current) {
			return n, writeFileAtomic(path, out)
		}
	}
}

// claudeMdStub is the first line of the dedicated CLAUDE.md: an import of the user's own file.
// Native loads user-scope imports without an approval dialog, and relative imports inside the
// user's file resolve from that file. A space in the path takes a backslash, so the path is
// written with slashes.
func claudeMdStub(source string) string {
	return "@" + strings.ReplaceAll(filepath.ToSlash(filepath.Join(source, "CLAUDE.md")), " ", `\ `)
}

// deriveClaudeMd returns the dedicated CLAUDE.md with its first line managed: the import when the
// user has a CLAUDE.md, no import when not. What follows the first line belongs to whoever wrote it
// (a /memory edit inside an isolated session lands there) and is kept.
func deriveClaudeMd(current []byte, haveUser bool, source string) []byte {
	stub := claudeMdStub(source)
	text := string(current)
	first, rest, _ := strings.Cut(text, "\n")
	// Ours is exactly the import this folder's source gives, in any letter case (Windows paths) and
	// with either line ending. Any other first line, an import the user wrote included, is theirs.
	line := strings.TrimRight(first, "\r")
	ours := strings.EqualFold(line, stub)
	switch {
	case haveUser && line == stub:
		return current
	case haveUser && ours:
		return []byte(stub + "\n" + rest)
	case haveUser && text == "":
		return []byte(stub + "\n")
	case haveUser:
		return []byte(stub + "\n" + text)
	case ours:
		return []byte(rest)
	}
	return current
}

// deriveReport says what an isolated launch rebuilt, by count and name only.
type deriveReport struct {
	SettingsDropped int
	ClaudeJSONKeys  int
	ClaudeMd        string // "imported" or "absent"
	Keybindings     bool
	// SettingsEdited is set when the dedicated settings.json changed since the last launch wrote it:
	// a /config change made inside an isolated session. It is replaced, not copied back.
	SettingsEdited bool
}

func readIfExists(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("ISOLATION_SOURCE_FILE_UNREADABLE")
	}
	raw, err := os.ReadFile(path)
	if raw == nil && err == nil {
		raw = []byte{}
	}
	return raw, err
}

// writeFileAtomic replaces path with data in one step, so native never reads half a file.
func writeFileAtomic(path string, data []byte) error {
	var nonce [6]byte
	rand.Read(nonce[:])
	tmp := path + ".tmp-" + hex.EncodeToString(nonce[:])
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func writeIfChanged(path string, data []byte) error {
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, data) {
		return nil
	}
	return writeFileAtomic(path, data)
}

// sourceGlobalConfig is where the user's native keeps its global config file: inside the config
// folder when CLAUDE_CONFIG_DIR is set, at ~\.claude.json when it is not.
func sourceGlobalConfig(source, userHome string, custom bool) string {
	if custom {
		return filepath.Join(source, ".claude.json")
	}
	return filepath.Join(userHome, ".claude.json")
}

func readIsolationSettings(source string) ([]byte, error) {
	raw, err := readIfExists(filepath.Join(source, "settings.json"), deriveSettingsBytes)
	if err != nil {
		return nil, errIsolationSettingsFile
	}
	return raw, nil
}

// deriveFiles rebuilds the dedicated files from the same settings snapshot used for the plugin root.
func deriveFiles(dedicated, source, userHome string, custom bool, userSettings []byte) (deriveReport, error) {
	var report deriveReport
	settings, dropped, err := deriveSettings(userSettings)
	if err != nil {
		return report, err
	}
	report.SettingsDropped = dropped
	userJSON, err := readIfExists(sourceGlobalConfig(source, userHome, custom), deriveClaudeJSONBytes)
	if err != nil {
		return report, errors.New("ISOLATION_CLAUDE_JSON_UNREADABLE: your global config file could not be read; nothing was started")
	}
	// The global config file is written last of the derived files that native may be using, and only
	// when an imported key changed: see mergeGlobalConfig.
	keys, err := mergeGlobalConfig(filepath.Join(dedicated, ".claude.json"), userJSON, nil)
	if err != nil {
		return report, err
	}
	report.ClaudeJSONKeys = keys

	// A /config change made inside an isolated session: the file is not what the last launch wrote.
	path := filepath.Join(dedicated, "settings.json")
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	var state struct {
		Settings string `json:"settings"`
	}
	if raw, rerr := os.ReadFile(filepath.Join(dedicated, derivedMarker)); rerr == nil {
		_ = json.Unmarshal(raw, &state)
	}
	if current, rerr := os.ReadFile(path); rerr == nil && state.Settings != "" && sum(current) != state.Settings {
		report.SettingsEdited = true
	}
	if err := writeIfChanged(path, settings); err != nil {
		return report, err
	}
	if marker, merr := json.Marshal(map[string]string{"settings": sum(settings)}); merr == nil {
		if err := writeIfChanged(filepath.Join(dedicated, derivedMarker), marker); err != nil {
			return report, err
		}
	}

	userMd, err := readIfExists(filepath.Join(source, "CLAUDE.md"), 4<<20)
	if err != nil {
		return report, fmt.Errorf("ISOLATION_SOURCE_FILE_UNREADABLE: CLAUDE.md")
	}
	haveMd := userMd != nil
	currentMd, _ := readIfExists(filepath.Join(dedicated, "CLAUDE.md"), 4<<20)
	if next := deriveClaudeMd(currentMd, haveMd, source); !bytes.Equal(next, currentMd) {
		if len(next) == 0 {
			os.Remove(filepath.Join(dedicated, "CLAUDE.md"))
		} else if err := writeFileAtomic(filepath.Join(dedicated, "CLAUDE.md"), next); err != nil {
			return report, err
		}
	}
	report.ClaudeMd = "absent"
	if haveMd {
		report.ClaudeMd = "imported"
	}

	// keybindings.json is a user file native does not write back: a plain copy, removed when the
	// user's is.
	keys2, err := readIfExists(filepath.Join(source, "keybindings.json"), deriveSettingsBytes)
	if err != nil {
		return report, errors.New("ISOLATION_SOURCE_FILE_UNREADABLE: keybindings.json")
	}
	if keys2 != nil {
		report.Keybindings = true
		return report, writeIfChanged(filepath.Join(dedicated, "keybindings.json"), keys2)
	}
	if err := os.Remove(filepath.Join(dedicated, "keybindings.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	return report, nil
}
