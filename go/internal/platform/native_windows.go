//go:build windows

// Package platform holds the OS boundary. The file suffix is the contract: this resolution
// exists only for Windows, because the Node baseline it ports from only ever had a Windows
// path (src/runtime-paths.mjs at 1b1c5e1 splits PATH on ';' and looks for .exe under AppData). A second
// OS is new design under requirement V2-04, not a port, so there is deliberately no
// native_other.go stub that would let this package compile into a lie elsewhere.
package platform

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// The baseline reads at most this many raw PATH entries before any filtering. Keeping the
// same order of operations matters: a machine with a 400-entry PATH must resolve to the same
// executable under both implementations or the comparison is measuring the wrong thing.
const maxPathEntries = 64

// Resolver finds the installed native executable. Every input is injectable so a test can
// describe a machine it does not have, and so no test needs the real filesystem.
type Resolver struct {
	// Home is the OS user's profile directory. Leave empty to look it up.
	Home string
	// Env is the environment to search. Leave nil to read the process environment.
	Env map[string]string
	// Cwd is the working directory to exclude from PATH. Leave empty to read it.
	Cwd string
	// IsFile reports whether a path names an existing regular file. Leave nil for the real one.
	IsFile func(string) bool
}

// OSUserHome returns the home directory of the OS user that owns this process.
//
// It deliberately does not call os.UserHomeDir. That function returns %USERPROFILE%
// verbatim, so any caller who can set one environment variable can redirect the search for
// claude.exe to a directory they control. Measured on go1.27.0 windows/amd64: with
// USERPROFILE poisoned, os.UserHomeDir returned the poisoned value while user.Current
// returned the real profile. The Node baseline defends the same way with os.userInfo().
func OSUserHome() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (r Resolver) resolved() (Resolver, error) {
	if r.IsFile == nil {
		r.IsFile = isRegularFile
	}
	if r.Env == nil {
		r.Env = environMap(os.Environ())
	}
	if r.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return r, err
		}
		r.Cwd = cwd
	}
	if r.Home == "" {
		home, err := OSUserHome()
		if err != nil {
			return r, err
		}
		r.Home = home
	}
	return r, nil
}

func environMap(entries []string) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if i := strings.IndexByte(entry, '='); i > 0 {
			out[entry[:i]] = entry[i+1:]
		}
	}
	return out
}

// pathValue finds PATH without assuming its case. Windows environment names are
// case-insensitive, so a process can just as well be handed "Path".
func pathValue(env map[string]string) string {
	for name, value := range env {
		if strings.EqualFold(name, "path") {
			return value
		}
	}
	return ""
}

// searchDirs applies the baseline's five defenses in the baseline's order: cap the raw
// entries first, then unquote, then keep absolute paths only, then drop the working
// directory, then deduplicate. Dropping cwd is what stops a program that merely happens to
// sit next to the user's files from being treated as an installed tool.
func searchDirs(pathText, cwd string) []string {
	raw := strings.Split(pathText, ";")
	if len(raw) > maxPathEntries {
		raw = raw[:maxPathEntries]
	}
	cwdKey := comparableDir(cwd)
	seen := make(map[string]bool, len(raw))
	dirs := make([]string, 0, len(raw))
	for _, entry := range raw {
		dir := strings.TrimSpace(entry)
		if len(dir) >= 2 && dir[0] == '"' && dir[len(dir)-1] == '"' {
			dir = dir[1 : len(dir)-1]
		}
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		key := comparableDir(dir)
		if key == cwdKey || seen[key] {
			continue
		}
		seen[key] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

func comparableDir(dir string) string {
	if dir == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(dir))
}

// Claude reports the native executable to run and whether it was actually found.
//
// When it is not found the returned path is still the standalone install location. That is
// the address of the problem a user has to fix, and returning it lets a caller report where
// it looked without a second lookup. A caller must not spawn a path whose found is false.
func (r Resolver) Claude() (path string, found bool, err error) {
	r, err = r.resolved()
	if err != nil {
		return "", false, err
	}
	return r.first("claude.exe", nil, nil)
}

// Codex reports the Codex CLI, found the same way and with the same defences. Its version
// is what a backend request identifies itself as, so reading it from a directory an
// attacker can prepend to PATH would let them choose what this bridge claims to be.
//
// Two places more than claude.exe, both the baseline's (runtime-paths.mjs at 1b1c5e1): the
// Codex app's standalone install before PATH, and an npm install last. The baseline ran
// npm's codex.js through Node, which this build may not; the package carries the native
// executable codex.js would have started, so that is what is found (#88).
func (r Resolver) Codex() (path string, found bool, err error) {
	r, err = r.resolved()
	if err != nil {
		return "", false, err
	}
	app := filepath.Join(r.Home, "AppData", "Local", "Programs", "OpenAI", "Codex", "bin", "codex.exe")
	npm := []string{filepath.Join(r.Home, "AppData", "Roaming", "npm", npmCodex)}
	for _, dir := range searchDirs(pathValue(r.Env), r.Cwd) {
		npm = append(npm, filepath.Join(dir, npmCodex))
	}
	return r.first("codex.exe", []string{app}, npm)
}

// npmCodex is where a global npm install of @openai/codex keeps its native executable,
// relative to the npm prefix: npm nests the platform package under the one depending on it.
// ponytail: npm's nested x64 layout only (the only one shipped for); pnpm, bun and hoisted
// layouts are not searched -- add one when someone has it.
var npmCodex = filepath.Join("node_modules", "@openai", "codex", "node_modules", "@openai",
	"codex-win32-x64", "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe")

// first tries ~\.local\bin, then before, then PATH, then after. Not found still reports
// ~\.local\bin, the standalone location.
func (r Resolver) first(name string, before, after []string) (path string, found bool, err error) {
	standalone := filepath.Join(r.Home, ".local", "bin", name)
	candidates := append([]string{standalone}, before...)
	for _, dir := range searchDirs(pathValue(r.Env), r.Cwd) {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	for _, candidate := range append(candidates, after...) {
		if r.IsFile(candidate) {
			return candidate, true, nil
		}
	}
	return standalone, false, nil
}
