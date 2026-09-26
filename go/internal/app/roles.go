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
	defaultErr   error // CLAUDE_CODE_SUBAGENT_MODEL could not be verified; matters only when used
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
	// CLAUDE_CODE_SUBAGENT_MODEL fills only a definition without a model, and its effort is
	// still the parent's (native 2.1.283: parent low gives sol/low; a definition effort wins).
	if model == "" {
		if s.defaultErr != nil {
			return bridge.Route{}, false, errRoleDefaults
		}
		if s.defaultModel != "inherit" {
			model = s.defaultModel
		}
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
	// answer (its effort, even under CLAUDE_CODE_SUBAGENT_MODEL), and a parent without one is
	// still unverified.
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
func roleCLI(args []string, injected, cwd string) (map[string]roleDefault, []string, error) {
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
	raw := []byte(selected)
	// Native 2.1.281 also takes, with --print, the path of a file that holds the object.
	// Read as the settings file is: bounded, regular, relative to the working directory.
	if selected != "" && !strings.HasPrefix(strings.TrimSpace(selected), "{") {
		path := selected
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		var err error
		if raw, err = boundedRoleFile(path); err != nil {
			return nil, nil, errRoleDefaults
		}
	}
	defs := map[string]roleDefault{}
	if len(raw) > 0 && (len(raw) > 1<<20 || json.Unmarshal(raw, &defs) != nil) {
		return nil, nil, errRoleDefaults
	}
	for name, def := range defs {
		if name == "" || len(name) > 200 || strings.HasPrefix(name, "-") || len(def.Model) > 200 || len(def.Effort) > 16 {
			return nil, nil, errRoleDefaults
		}
	}
	return defs, plugins, nil
}

// cliRoles is what argv defines, read once when the session starts, as native reads --agents
// and an --agents file: a file changed or removed later must not change or refuse routing.
type cliRoles struct {
	defs    map[string]roleDefault
	plugins []string
	err     error
	// addDirs are the --add-dir directories, absolute, in argv order.
	addDirs []string
	// sources is --setting-sources; nil when absent, which is native's default of all three.
	sources map[string]bool
	// flagModel is CLAUDE_CODE_SUBAGENT_MODEL from --settings, when flagSet.
	flagModel string
	flagSet   bool
	flagErr   error
	// subagent is CLAUDE_CODE_SUBAGENT_MODEL for the session, read from the environment and
	// settings files once, at the first delegation, like the rest of this struct.
	subagent func() (string, error)
}

func (c cliRoles) source(name string) bool { return c.sources == nil || c.sources[name] }

const subagentModelKey = "CLAUDE_CODE_SUBAGENT_MODEL"

func sessionCLIRoles(args []string, injected, cwd string) cliRoles {
	var cli cliRoles
	cli.defs, cli.plugins, cli.err = roleCLI(args, injected, cwd)
	if cli.err == nil {
		cli.err = cli.scope(args, cwd)
	}
	return cli
}

// scope reads what argv adds to or removes from the role sources: every --add-dir directory
// (variadic in native: it takes values until the next option), --setting-sources and the
// --settings subagent model. An unknown option hides where any of these after it begin.
func (c *cliRoles) scope(args []string, cwd string) error {
	sources, sourcesSet, settings := "", false, ""
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known {
			for _, arg := range args[i+1:] {
				if arg == "--" {
					break // native reads everything after it as data
				}
				if name, _, _ := strings.Cut(arg, "="); name == "--add-dir" || name == "--setting-sources" || name == "--settings" {
					return errRoleDefaults
				}
			}
			break
		}
		if end > len(args) {
			return errRoleDefaults
		}
		name, value, attached := strings.Cut(args[i], "=")
		if !attached && end == i+2 {
			value = args[i+1]
		}
		switch name {
		case "--add-dir":
			dirs := []string{value}
			for !attached && end < len(args) && !(len(args[end]) > 1 && args[end][0] == '-') {
				dirs = append(dirs, args[end])
				end++
			}
			for _, dir := range dirs {
				if !filepath.IsAbs(dir) {
					dir = filepath.Join(cwd, dir)
				}
				c.addDirs = append(c.addDirs, dir)
			}
		case "--setting-sources":
			sources, sourcesSet = value, true
		case "--settings":
			settings = value
		}
		i = end
	}
	if sourcesSet {
		c.sources = map[string]bool{}
		for _, part := range strings.Split(sources, ",") {
			switch part = strings.TrimSpace(part); part {
			case "":
			case "user", "project", "local":
				c.sources[part] = true
			default:
				return errRoleDefaults // native 2.1.283 exits: Invalid setting source
			}
		}
	}
	if settings != "" {
		raw := []byte(settings)
		if !strings.HasPrefix(strings.TrimSpace(settings), "{") {
			path := settings
			if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, path)
			}
			var err error
			if raw, err = boundedRoleFile(path); err != nil {
				c.flagErr = errRoleDefaults
				return nil
			}
		}
		c.flagModel, c.flagSet, c.flagErr = settingsSubagentModel(raw)
	}
	return nil
}

// settingsSubagentModel reads CLAUDE_CODE_SUBAGENT_MODEL from one settings object's env.
func settingsSubagentModel(raw []byte) (string, bool, error) {
	var values struct {
		Env map[string]json.RawMessage `json:"env"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return "", false, errRoleDefaults
	}
	value, found := values.Env[subagentModelKey]
	var model string
	if found && json.Unmarshal(value, &model) != nil {
		return "", false, errRoleDefaults
	}
	return model, found, nil
}

// subagentModel is CLAUDE_CODE_SUBAGENT_MODEL as native 2.1.283 applies it (measured):
// settings env overrides the process environment, user < project < local < --settings,
// each file only when its source is loaded, project and local from the session directory
// only (not the git root). The managed settings rank is unmeasured, so a managed value is
// unverified rather than placed. Native trims and lowercases a model name before resolving it.
func subagentModel(config, cwd, managed string, cli cliRoles, env map[string]string) (string, error) {
	model := env[subagentModelKey]
	for _, file := range []struct{ source, path string }{
		{"user", filepath.Join(config, "settings.json")},
		{"project", filepath.Join(cwd, ".claude", "settings.json")},
		{"local", filepath.Join(cwd, ".claude", "settings.local.json")},
	} {
		if !cli.source(file.source) {
			continue
		}
		raw, err := boundedRoleFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", errRoleDefaults
		}
		value, found, err := settingsSubagentModel(raw)
		if err != nil {
			return "", err
		}
		if found {
			model = value
		}
	}
	if cli.flagErr != nil {
		return "", cli.flagErr
	}
	if cli.flagSet {
		model = cli.flagModel
	}
	if raw, err := boundedRoleFile(filepath.Join(managed, "managed-settings.json")); !os.IsNotExist(err) {
		if err != nil {
			return "", errRoleDefaults
		}
		if _, found, err := settingsSubagentModel(raw); err != nil || found {
			return "", errRoleDefaults
		}
	}
	return strings.ToLower(strings.TrimSpace(model)), nil
}

// managedRoot is native's platform directory for managed settings and agents.
func managedRoot() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "windows":
		return `C:\Program Files\ClaudeCode`
	}
	return "/etc/claude-code"
}

func sessionRoleSources(config, cwd string, cli cliRoles, env map[string]string, role string) roleSources {
	plugins := append([]string(nil), cli.plugins...) // appended to below; cli is shared across calls
	s := roleSources{cli: cli.defs, err: cli.err}
	// Native's platform directories; no invented environment override.
	managed := managedRoot()
	if cli.subagent != nil {
		s.defaultModel, s.defaultErr = cli.subagent()
	} else {
		s.defaultModel, s.defaultErr = subagentModel(config, cwd, managed, cli, env)
	}
	if managed != "" {
		s.directories = append(s.directories, roleDirectory{path: filepath.Join(managed, ".claude", "agents")})
		s.managed = 1
	}
	// Native 2.1.283 (measured): CLI > project > --add-dir > user, the later --add-dir first;
	// project and --add-dir definitions load only with the project source, user ones only
	// with the user source. An --add-dir is read at its own .claude/agents, not walked.
	var projectDirs []string
	for dir := cwd; dir != ""; dir = filepath.Dir(dir) {
		projectDirs = append(projectDirs, dir)
		if cli.source("project") {
			s.directories = append(s.directories, roleDirectory{path: filepath.Join(dir, ".claude", "agents")})
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
			break
		}
	}
	if cli.source("project") {
		for i := len(cli.addDirs) - 1; i >= 0; i-- {
			s.directories = append(s.directories, roleDirectory{path: filepath.Join(cli.addDirs[i], ".claude", "agents")})
		}
	}
	if cli.source("user") {
		s.directories = append(s.directories, roleDirectory{path: filepath.Join(config, "agents")})
	}
	// Plugin roles are namespaced. Ordinary role resolution needs no plugin
	// registry/manifest I/O, even in a workspace with many installed plugins.
	if !strings.Contains(role, ":") {
		return s
	}
	installed, err := installedRolePlugins(config, projectDirs, managed, cli.source)
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
	// Native 2.1.283 (measured, #148) also takes a .zip, and a folder whose children are
	// plugins. A zip is not unpacked here: its roles resolve as unknown and run on native's
	// own choice, as a --plugin-url plugin's already do. A folder of plugins is read child
	// by child.
	if info, err := os.Stat(dir); err == nil && info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(dir), ".zip") {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "plugin.json")); errors.Is(err, fs.ErrNotExist) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 128 {
			return nil, errRoleDefaults
		}
		var out []roleDirectory
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), ".claude-plugin", "plugin.json")); err != nil {
				continue
			}
			directories, err := pluginManifestDirectories(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, directories...)
		}
		if len(out) == 0 {
			return nil, errRoleDefaults
		}
		return out, nil
	}
	return pluginManifestDirectories(dir)
}

func pluginManifestDirectories(dir string) ([]roleDirectory, error) {
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
// Settings of a source --setting-sources excludes enable nothing (measured on 2.1.283 for
// user and project).
func installedRolePlugins(config string, projects []string, managed string, source func(string) bool) ([]string, error) {
	var settings []string
	if source("user") {
		settings = append(settings, filepath.Join(config, "settings.json"))
	}
	for i := len(projects) - 1; i >= 0; i-- {
		if source("project") {
			settings = append(settings, filepath.Join(projects[i], ".claude", "settings.json"))
		}
		if source("local") {
			settings = append(settings, filepath.Join(projects[i], ".claude", "settings.local.json"))
		}
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
