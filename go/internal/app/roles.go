package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"go.yaml.in/yaml/v3"
)

var errRoleDefaults = errors.New("ROLE_DEFAULTS_UNVERIFIED")

type roleDefault struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `json:"model" yaml:"model"`
	Effort      string `json:"effort" yaml:"effort"`
	invalid     bool
}
type roleDirectory struct{ path, prefix string }
type roleSources struct {
	cli          map[string]roleDefault
	directories  []roleDirectory // highest priority first
	managed      int
	err          error
	defaultModel string
	pluginError  error
}

// Reads only routing fields. Native loads the actual role and retains its own
// source trust, prompt, tools and hook permissions; definitions are never copied
// into --agents, which would incorrectly promote project/plugin trust.
func (s roleSources) resolve(role string, parent bridge.Route) (bridge.Route, bool, error) {
	if s.err != nil {
		return bridge.Route{}, false, s.err
	}
	var def roleDefault
	found := false
	unverified := false
	// Preserve readable definitions and their precedence. An unreadable ordinary
	// definition may declare any name, so it cannot prove an unmatched role absent.
	for i, dir := range s.directories {
		if i == s.managed {
			if def, found = s.cli[role]; found {
				break
			}
		}
		if dir.prefix != "" && !strings.HasPrefix(role, dir.prefix+":") {
			continue
		}
		defs, incomplete, err := readRoleDirectory(dir)
		if err != nil {
			return bridge.Route{}, false, err
		}
		unverified = unverified || incomplete
		if def, found = defs[role]; found {
			break
		}
	}
	if !found {
		def, found = s.cli[role]
	}
	if !found {
		if unverified {
			return bridge.Route{}, false, errRoleDefaults
		}
		if strings.Contains(role, ":") && s.pluginError != nil {
			return bridge.Route{}, false, s.pluginError
		}
		return bridge.Route{}, false, nil
	}
	if def.invalid {
		return bridge.Route{}, false, errRoleDefaults
	}
	model, effort := def.Model, def.Effort
	named := model != "" && model != "inherit"
	if model == "" && s.defaultModel != "inherit" {
		model = s.defaultModel
		named = model != ""
	}
	if model == "" || model == "inherit" {
		model = parent.Model
	}
	// A definition that names a model and no effort takes that model's own default. That is
	// what SelectRoute does with an empty effort, and it is what the same choice written as
	// Agent(model:) already gets. Reading the parent's effort here routed one intent two
	// ways: a role pinned to astra ran at a max parent's effort rather than astra's own, and
	// a parent at low silently downgraded a role whose model defaults higher. Inheritance is
	// still right when the definition names no model -- then the parent's route is the whole
	// answer, and a parent without one is still unverified.
	if effort == "" && !named {
		effort = parent.Effort
	}
	if model == "" || (!named && effort == "") {
		return bridge.Route{}, false, errRoleDefaults
	}
	route, err := bridge.SelectRoute(model, effort)
	return route, true, err
}

func boundedRoleFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errRoleDefaults
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errRoleDefaults
	}
	return raw, nil
}

// Return readable definitions, filename claims and whether any names remain
// unknowable. A partial scan can answer a known role, but cannot prove absence.
func readRoleDirectory(dir roleDirectory) (map[string]roleDefault, bool, error) {
	defs := map[string]roleDefault{}
	incomplete := false
	count := 0
	err := filepath.WalkDir(dir.path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) && path == dir.path {
				return nil
			}
			// A directory this scan cannot enter holds definitions it cannot name, which is
			// the same thing an unreadable file is and gets the same answer: skip it. Failing
			// the walk here refused every role in the session over one unreadable
			// subdirectory -- the blast radius this whole mechanism exists to remove, left
			// in place for directories while the comment above it described files.
			//
			// A subdirectory, not the root. Without that distinction this swallowed the
			// root's own enumeration failure and returned an empty map with no error, so an
			// agents directory that exists and cannot be read answered "no roles here" and
			// every one of them ran on the caller's model -- silently, because the only
			// counter that moves is deliberately outside noteworthy(). Before the skip
			// existed that case refused loudly, which is the answer it gets again.
			if entry != nil && entry.IsDir() && path != dir.path {
				incomplete = true
				return nil
			}
			return errRoleDefaults
		}
		count++
		if count > 1024 {
			return errRoleDefaults
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		// Both ordinary and plugin frontmatter may name a different role. Keep
		// the filename claim and mark unknown names within this directory's
		// namespace; resolve already skips unrelated plugin namespaces.
		passedOver := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if dir.prefix != "" {
			passedOver = dir.prefix + ":" + passedOver
		}
		claim := func() error {
			incomplete = true
			if held, seen := defs[passedOver]; !seen || !held.invalid {
				defs[passedOver] = roleDefault{Name: passedOver, invalid: true}
			}
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return claim()
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			return claim()
		}
		// Only the bounded frontmatter prefix is needed, never the role prompt.
		raw, err := io.ReadAll(io.LimitReader(file, 65544))
		file.Close()
		if err != nil {
			return claim()
		}
		def, ok, err := parseRoleFile(raw)
		if err != nil {
			return claim()
		}
		if dir.prefix != "" && !ok {
			def.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			ok = true
		}
		if !ok {
			return nil
		} // Native skips malformed non-plugin definitions.
		if dir.prefix != "" {
			def.Name = dir.prefix + ":" + def.Name
		}
		if _, duplicate := defs[def.Name]; duplicate {
			def.invalid = true
		}
		defs[def.Name] = def
		return nil
	})
	return defs, incomplete, err
}

func parseRoleFile(raw []byte) (roleDefault, bool, error) {
	var def roleDefault
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return def, false, nil
	}
	end := bytes.Index(raw[4:], []byte("\n---\n"))
	if end < 0 && bytes.HasSuffix(raw, []byte("\n---")) {
		end = len(raw) - 8
	}
	if end < 0 {
		return def, false, errRoleDefaults
	}
	front := raw[4 : 4+end]
	if len(front) > 65536 {
		return def, false, errRoleDefaults
	}
	var node yaml.Node
	if yaml.Unmarshal(front, &node) != nil {
		return def, false, errRoleDefaults
	}
	// Bound aliases before decoding, including cyclic/expanding merge keys.
	budget := 4096
	var visit func(*yaml.Node, int) bool
	visit = func(n *yaml.Node, depth int) bool {
		budget--
		if budget < 0 || depth > 32 {
			return false
		}
		if n.Alias != nil && !visit(n.Alias, depth+1) {
			return false
		}
		for _, child := range n.Content {
			if !visit(child, depth+1) {
				return false
			}
		}
		return true
	}
	if !visit(&node, 0) || node.Decode(&def) != nil {
		return def, false, errRoleDefaults
	}
	if def.Name == "" || def.Description == "" || strings.HasPrefix(def.Name, "-") || strings.Contains(def.Name, ":") {
		return def, false, nil
	}
	if len(def.Name) > 200 || len(def.Model) > 200 || len(def.Effort) > 16 {
		return def, false, errRoleDefaults
	}
	return def, true, nil
}

// Required-value options consume their next token even if it begins with '--'.
// This read-only scan never changes forwarded argv. Unknown options make --agents
// discovery unverified rather than misreading a prompt value as a routing policy.
func roleCLI(args []string, injected string) (map[string]roleDefault, []string, error) {
	hasCLI := false
	for _, arg := range args {
		if arg == "--agents" || strings.HasPrefix(arg, "--agents=") || strings.HasPrefix(arg, "--plugin-dir") {
			hasCLI = true
		}
	}
	var plugins []string
	selected := injected
	for i := 0; i < len(args); {
		if args[i] == "--" {
			break
		}
		end, known := nativeArgEnd(args, i)
		if end > len(args) || hasCLI && !known {
			return nil, nil, errRoleDefaults
		}
		arg, value, attached := strings.Cut(args[i], "=")
		if !attached && end == i+2 {
			value = args[i+1]
		}
		if arg == "--agents" {
			selected = value
		}
		if arg == "--plugin-dir" || arg == "--plugin-dir-no-mcp" {
			plugins = append(plugins, value)
		}
		i = end
	}
	defs := map[string]roleDefault{}
	if selected != "" && (len(selected) > 1<<20 || json.Unmarshal([]byte(selected), &defs) != nil) {
		return nil, nil, errRoleDefaults
	}
	for name, def := range defs {
		if name == "" || len(name) > 200 || strings.HasPrefix(name, "-") || len(def.Model) > 200 || len(def.Effort) > 16 {
			return nil, nil, errRoleDefaults
		}
	}
	return defs, plugins, nil
}

func sessionRoleSources(config, cwd, injected string, args []string, env map[string]string, role string) roleSources {
	cli, plugins, err := roleCLI(args, injected)
	s := roleSources{cli: cli, err: err, defaultModel: env["CLAUDE_CODE_SUBAGENT_MODEL"]}
	// Native's platform directories; no invented environment override.
	managed := "/etc/claude-code"
	if runtime.GOOS == "darwin" {
		managed = "/Library/Application Support/ClaudeCode"
	}
	if runtime.GOOS == "windows" {
		managed = `C:\Program Files\ClaudeCode`
	}
	if managed != "" {
		s.directories = append(s.directories, roleDirectory{path: filepath.Join(managed, ".claude", "agents")})
		s.managed = 1
	}
	var projectDirs []string
	for dir := cwd; dir != ""; dir = filepath.Dir(dir) {
		projectDirs = append(projectDirs, dir)
		s.directories = append(s.directories, roleDirectory{path: filepath.Join(dir, ".claude", "agents")})
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
			break
		}
	}
	s.directories = append(s.directories, roleDirectory{path: filepath.Join(config, "agents")})
	// Plugin roles are namespaced. Ordinary role resolution needs no plugin
	// registry/manifest I/O, even in a workspace with many installed plugins.
	if !strings.Contains(role, ":") {
		return s
	}
	installed, err := installedRolePlugins(config, projectDirs, managed)
	if err != nil {
		s.pluginError = err
	}
	plugins = append(plugins, installed...)
	for _, dir := range plugins {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		directories, err := pluginRoleDirectories(dir)
		if err != nil {
			s.pluginError = errRoleDefaults
			continue
		}
		s.directories = append(s.directories, directories...)
	}
	return s
}

func pluginRoleDirectories(dir string) ([]roleDirectory, error) {
	var manifest struct {
		Name   string
		Agents json.RawMessage
	}
	raw, err := boundedRoleFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Name == "" || len(manifest.Name) > 128 || strings.ContainsAny(manifest.Name, ":/\\") {
		return nil, errRoleDefaults
	}
	paths := []string{"agents"}
	if len(manifest.Agents) > 0 {
		var single string
		var extra []string
		if json.Unmarshal(manifest.Agents, &single) == nil {
			extra = []string{single}
		} else if json.Unmarshal(manifest.Agents, &extra) != nil {
			return nil, errRoleDefaults
		}
		paths = append(paths, extra...)
	}
	if len(paths) > 128 {
		return nil, errRoleDefaults
	}
	out := []roleDirectory{}
	seen := map[string]bool{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsLocal(path) {
			return nil, errRoleDefaults
		}
		if !seen[path] {
			out = append(out, roleDirectory{path: filepath.Join(dir, path), prefix: manifest.Name})
			seen[path] = true
		}
	}
	return out, nil
}

// Only enabled installed plugins are inspected. No plugin code, hooks, commands
// or MCP configuration is run by this loader. Native remains the execution owner.
func installedRolePlugins(config string, projects []string, managed string) ([]string, error) {
	settings := []string{filepath.Join(config, "settings.json")}
	for i := len(projects) - 1; i >= 0; i-- {
		settings = append(settings, filepath.Join(projects[i], ".claude", "settings.json"), filepath.Join(projects[i], ".claude", "settings.local.json"))
	}
	settings = append(settings, filepath.Join(managed, "managed-settings.json"))
	enabled := map[string]bool{}
	for _, path := range settings {
		raw, err := boundedRoleFile(path)
		if os.IsNotExist(err) {
			continue
		}
		var values struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}
		if err != nil || json.Unmarshal(raw, &values) != nil {
			return nil, errRoleDefaults
		}
		for name, on := range values.EnabledPlugins {
			enabled[name] = on
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	raw, err := boundedRoleFile(filepath.Join(config, "plugins", "installed_plugins.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	var installed struct {
		Version int
		Plugins map[string][]struct{ Scope, InstallPath, ProjectPath string }
	}
	if err != nil || json.Unmarshal(raw, &installed) != nil || installed.Version != 2 || len(installed.Plugins) > 512 {
		return nil, errRoleDefaults
	}
	names := make([]string, 0, len(enabled))
	for name, on := range enabled {
		if on {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var paths []string
	for _, name := range names {
		best, rank := "", 0
		for _, entry := range installed.Plugins[name] {
			n := 0
			switch entry.Scope {
			case "user":
				n = 1
			case "project", "local":
				for _, project := range projects {
					if sameRolePath(project, entry.ProjectPath) {
						n = 2
						if entry.Scope == "local" {
							n = 3
						}
						break
					}
				}
			case "managed":
				n = 4
			}
			if n > rank {
				best, rank = entry.InstallPath, n
			} else if n == rank && n > 0 && best != entry.InstallPath {
				return nil, errRoleDefaults
			}
		}
		if best != "" {
			if !filepath.IsAbs(best) {
				return nil, errRoleDefaults
			}
			paths = append(paths, best)
		}
	}
	return paths, nil
}

func sameRolePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
