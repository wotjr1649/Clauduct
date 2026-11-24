package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// Isolated mode (#379): Clauduct sessions run native on a dedicated config folder, so the stored
// Claude login and the shared supervisor stay out of reach of the requests native sends on its own.
const (
	isolationOff      = "off"
	isolationOn       = "on"
	pluginCacheDirEnv = "CLAUDE_CODE_PLUGIN_CACHE_DIR"
)

var errIsolationSettings = fmt.Errorf(`%w: isolation must be "off" or "on". Edit ~/.clauduct/settings.json`, errClauductSettings)

// parseIsolation reads the settings value. Anything but the two names is refused, never defaulted.
func parseIsolation(raw json.RawMessage) (string, error) {
	var mode string
	if string(raw) == "null" || json.Unmarshal(raw, &mode) != nil || mode != isolationOff && mode != isolationOn {
		return "", errIsolationSettings
	}
	return mode, nil
}

// Isolated reports whether the settings turn isolated mode on.
func (config ClauductSettings) Isolated() bool { return config.Isolation == isolationOn }

// isolationEnv is what an isolated launch forces in native's OS environment, whatever the launch
// shell says. Routing values (endpoint, token, session) are deliberately not here: the supervisor
// keeps its start environment for every job that follows, so it may only carry what is the same
// for all of them.
func isolationEnv(env map[string]string, dedicated, pluginRoot string) map[string]string {
	forced := map[string]string{
		"CLAUDE_CONFIG_DIR":       dedicated,
		pluginCacheDirEnv:         pluginRoot,
		nonessentialTraffic:       "1", // also what makes native skip its start-up quota check (2.1.321, static)
		"DISABLE_TELEMETRY":       "1",
		"DISABLE_ERROR_REPORTING": "1",
	}
	out := make(map[string]string, len(env)+len(forced))
	for name, value := range env {
		if _, replaced := forced[strings.ToUpper(name)]; !replaced {
			out[name] = value
		}
	}
	for name, value := range forced {
		out[name] = value
	}
	return out
}

func resolveIsolationPluginRoot(env map[string]string, source, cwd string, raw []byte) (string, error) {
	root, _ := windowsEnvValue(env, pluginCacheDirEnv)
	if root == "" {
		values := map[string]string{}
		var settings struct {
			Env map[string]json.RawMessage
		}
		// The same JSON BOM and env shape as deriveSettings.
		if json.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &settings) == nil {
			for name, rawValue := range settings.Env {
				if strings.EqualFold(name, pluginCacheDirEnv) {
					var value string
					if json.Unmarshal(rawValue, &value) == nil {
						values[name] = value
					}
				}
			}
		}
		root, _ = windowsEnvValue(values, pluginCacheDirEnv)
	}
	if root == "" {
		return filepath.Join(source, "plugins"), nil
	}
	return nativePluginRootPath(root, cwd)
}

func nativePluginRootPath(root, cwd string) (string, error) {
	if root == "~" || strings.HasPrefix(root, "~/") || strings.HasPrefix(root, `~\`) {
		home, err := platform.OSUserHome()
		if err != nil {
			return "", errors.New("ISOLATION_PLUGIN_HOME_UNAVAILABLE: the OS home could not be resolved for the plugin cache")
		}
		return filepath.Join(home, strings.TrimLeft(root[1:], `/\`)), nil
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), nil
	}
	volume := filepath.VolumeName(root)
	if volume != "" && !strings.EqualFold(volume, filepath.VolumeName(cwd)) {
		return filepath.Abs(root)
	}
	relative := root[len(volume):]
	if relative != "" && os.IsPathSeparator(relative[0]) {
		return filepath.Clean(filepath.VolumeName(cwd) + relative), nil
	}
	return filepath.Join(cwd, relative), nil
}

const isolationMarker = ".clauduct-isolation"

// dedicatedConfigDir is native's configuration folder for isolated sessions. It lives under the
// Clauduct home, outlives every session, and is never created by --help, --version or the
// management commands.
func dedicatedConfigDir(home string) string {
	return filepath.Join(home, ".clauduct", "isolated", "claude")
}

var (
	errIsolationConflict = errors.New("ISOLATION_CONFIG_CONFLICT: CLAUDE_CONFIG_DIR names a folder inside or around Clauduct's isolated configuration folder; nothing was started")
	errIsolationSource   = errors.New("ISOLATION_SOURCE_UNKNOWN: CLAUDE_CONFIG_DIR is Clauduct's isolated folder and its marker does not name your own configuration folder; start clauduct from a shell without CLAUDE_CONFIG_DIR set to it")
	errIsolationChanged  = errors.New("ISOLATION_SOURCE_CHANGED: your configuration folder is not the one the isolated folder was built from; run clauduct --dev --isolation-reset, then start again")
	errIsolationUnsafe   = errors.New("ISOLATION_CONFIG_UNSAFE: the isolated configuration folder or a folder above it is a link or is not a directory; nothing was changed")
	errIsolationMarker   = errors.New("ISOLATION_MARKER_UNREADABLE: the isolated configuration marker is invalid or could not be read; nothing was started. Check its contents and access, or run clauduct --dev --isolation-reset to rebuild the folder")
	errIsolationActive   = errors.New("ISOLATION_SUPERVISOR_UNVERIFIED: a native background supervisor that was not started under isolated mode is running for the isolated folder; stop it (claude daemon stop --any with that CLAUDE_CONFIG_DIR) and start again")
)

func sameFolder(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

func insideFolder(parent, child string) bool {
	rel, err := filepath.Rel(strings.ToLower(filepath.Clean(parent)), strings.ToLower(filepath.Clean(child)))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isolationMarkerFile is what the dedicated folder remembers: when isolated mode first used it,
// and the user's configuration folder it is built from.
type isolationMarkerFile struct {
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
	Source    string `json:"source"`
	// Custom is whether the source folder came from CLAUDE_CONFIG_DIR: native keeps its global
	// config file inside that folder then, and at ~\.claude.json otherwise.
	Custom bool `json:"custom,omitempty"`
}

func readIsolationMarker(dedicated string) (isolationMarkerFile, bool) {
	raw, err := os.ReadFile(filepath.Join(dedicated, isolationMarker))
	var m isolationMarkerFile
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &m) != nil || m.Version != 1 || !filepath.IsAbs(m.Source) {
		return isolationMarkerFile{}, false
	}
	return m, true
}

// isolationSource is the user's configuration folder an isolated launch is built from: what
// CLAUDE_CONFIG_DIR (or ~\.claude) names, unless that is the isolated folder itself -- a clauduct
// started inside an isolated session inherits it -- and then the folder's marker says.
func isolationSource(env map[string]string, cwd, dedicated string) (source string, custom bool, err error) {
	configured := nativeConfigDirectory(env, cwd)
	switch {
	case configured == "":
		return "", false, errIsolationSource
	case sameFolder(configured, dedicated):
		m, ok := readIsolationMarker(dedicated)
		if !ok {
			return "", false, errIsolationSource
		}
		return m.Source, m.Custom, nil
	case insideFolder(dedicated, configured) || insideFolder(configured, dedicated):
		return "", false, errIsolationConflict
	}
	// Refused before anything is made: a junction cannot point at a network share.
	if _, err := junctionTarget(configured); err != nil {
		return "", false, err
	}
	for name, value := range env {
		if strings.EqualFold(name, "CLAUDE_CONFIG_DIR") && value != "" {
			custom = true
		}
	}
	return configured, custom, nil
}

// plainDirectory is a real directory when it exists: not a link, junction or other reparse point.
func plainDirectory(path string) (exists bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return true, errIsolationUnsafe
	}
	return true, nil
}

// isolationHosts are the process lookups the supervisor rule needs; a test replaces them.
type isolationHosts struct {
	lockOwner func(configDir string) int
	created   func(pid int) (time.Time, bool)
}

var osIsolationHosts = isolationHosts{lockOwner: supervisorLockOwner, created: isolationProcessCreated}

// prepareDedicated makes the dedicated folder ready for a native start and returns its marker. It
// never changes a folder that is a link, and it refuses a supervisor that predates isolated mode:
// that one was started without the OS environment isolated mode forces, and native keeps a
// supervisor's start environment for every session after it.
func prepareDedicated(dedicated, source string, custom bool, now time.Time, host isolationHosts) (isolationMarkerFile, error) {
	// Every folder from the Clauduct home's .clauduct down must be a plain directory when it exists.
	for _, dir := range []string{filepath.Dir(filepath.Dir(dedicated)), filepath.Dir(dedicated), dedicated} {
		if _, err := plainDirectory(dir); err != nil {
			return isolationMarkerFile{}, err
		}
	}
	if err := os.MkdirAll(dedicated, 0o700); err != nil {
		return isolationMarkerFile{}, err
	}
	marker, ok := readIsolationMarker(dedicated)
	if !ok {
		if host.lockOwner(dedicated) != 0 {
			return isolationMarkerFile{}, errIsolationActive
		}
		marker = isolationMarkerFile{Version: 1, CreatedAt: now.UTC().Format(time.RFC3339Nano), Source: filepath.Clean(source), Custom: custom}
		if err := publishIsolationMarker(dedicated, marker); err == nil {
			return marker, nil
		} else if !errors.Is(err, os.ErrExist) {
			return isolationMarkerFile{}, err
		}
		if marker, ok = readIsolationMarker(dedicated); !ok {
			return isolationMarkerFile{}, errIsolationMarker
		}
	}
	if !sameFolder(marker.Source, source) {
		return isolationMarkerFile{}, errIsolationChanged
	}
	created, err := time.Parse(time.RFC3339Nano, marker.CreatedAt)
	if err != nil {
		return isolationMarkerFile{}, errIsolationMarker
	}
	if pid := host.lockOwner(dedicated); pid != 0 {
		if at, known := host.created(pid); !known || at.Before(created.Add(-time.Second)) {
			return isolationMarkerFile{}, errIsolationActive
		}
	}
	return marker, nil
}

func publishIsolationMarker(dedicated string, marker isolationMarkerFile) error {
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dedicated, isolationMarker+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	from, err := syscall.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(filepath.Join(dedicated, isolationMarker))
	if err != nil {
		return err
	}
	// MoveFile publishes the complete marker without replacing another launch's marker.
	if err := syscall.MoveFile(from, to); err != nil {
		return &os.LinkError{Op: "publish isolation marker", Old: file.Name(), New: filepath.Join(dedicated, isolationMarker), Err: err}
	}
	return nil
}

// isolationProcessCreated is when pid started, from the OS.
func isolationProcessCreated(pid int) (time.Time, bool) {
	h, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return time.Time{}, false
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return time.Time{}, false
	}
	return time.Unix(0, created.Nanoseconds()), true
}
