package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

func nativeConfigDirectory(env map[string]string, cwd string) string {
	configDir := ""
	for key, value := range env {
		if strings.EqualFold(key, "CLAUDE_CONFIG_DIR") {
			configDir = value
		}
	}
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".claude")
		}
	}
	if configDir != "" && !filepath.IsAbs(configDir) {
		configDir = filepath.Join(cwd, configDir)
	}
	return configDir
}

// UUIDs can be loaded before launch; titles and pickers remain native-owned.
// Native preserves the spelling of a supplied UUID. Normalize it once so the
// transcript, headers and our Windows case-insensitive record share one key.
func prepareSessionArgs(args []string) ([]string, string) {
	args = slices.Clone(args)
	result := ""
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			return args, ""
		}
		name, value, attached := strings.Cut(args[i], "=")
		prefix := name + "="
		resume := name == "--resume" || name == "-r"
		if len(name) > 2 && name[0] == '-' && name[1] != '-' {
			for j := 1; j < len(name); j++ {
				if name[j] == 'r' {
					resume = true
					if j+1 < len(name) {
						value, attached = name[j+1:], true
						prefix = name[:j+1]
					}
					break
				}
				if !strings.ContainsRune("cpvh", rune(name[j])) {
					break
				}
			}
		}
		if resume || name == "--session-id" {
			if !attached && end == i+2 {
				value = args[i+1]
			}
			id := strings.ToLower(value)
			if gateway.IsSessionUUID(id) {
				if attached {
					args[i] = prefix + id
				} else if end == i+2 {
					args[i+1] = id
				}
			} else {
				id = ""
			}
			if resume {
				result = id
			}
		}
		i = end
	}
	return args, result
}

const clauductSettingsBytes = 1 << 20

var errClauductSettings = errors.New("CLAUDUCT_SETTINGS_INVALID")

// ClauductSettings contains validated launch preferences. SessionProfile owns
// the separately versioned persistent format.
type ClauductSettings struct {
	Startup         bridge.Pair
	Selection       bridge.Selection
	StartupSource   string
	SelectionSource string
}

func defaultClauductSettings() ClauductSettings {
	return ClauductSettings{Startup: startupModel, StartupSource: "factory.startup", SelectionSource: "factory"}
}

func (config ClauductSettings) effectiveStartup(spec launch.Spec, requested []string) (bridge.Pair, string, string, error) {
	model, named := optionValue(spec.Args, "--model")
	effort, _ := optionValue(spec.Args, "--effort")
	modelSource, effortSource := config.StartupSource, config.StartupSource
	_, explicitModel := optionValue(requested, "--model")
	_, explicitEffort := optionValue(requested, "--effort")
	if explicitModel {
		modelSource = "cli.model"
	}
	if explicitEffort {
		effortSource = "cli.effort"
	}
	env := map[string]string{}
	for _, entry := range spec.Env {
		key, value, _ := strings.Cut(entry, "=")
		if key = strings.ToUpper(key); key == "ANTHROPIC_MODEL" || key == effortEnv || key == "CLAUDE_CONFIG_DIR" {
			env[key] = value
		}
	}
	cli := cliRoles{}
	if err := cli.scope(spec.Args, spec.Dir); err != nil {
		return bridge.Pair{}, "", "", err
	}
	configDir := nativeConfigDirectory(env, spec.Dir)
	if !named {
		value, source, err := nativeEnvironment(configDir, spec.Dir, managedRoot(), "ANTHROPIC_MODEL", cli, env)
		if err != nil {
			return bridge.Pair{}, "", "", err
		}
		model = value
		if strings.HasPrefix(source, "native.") {
			modelSource = source
		}
	}
	value, source, err := nativeEnvironment(configDir, spec.Dir, managedRoot(), effortEnv, cli, env)
	if err != nil {
		return bridge.Pair{}, "", "", err
	}
	environmentEffort := source != "" && value != ""
	if environmentEffort {
		effort, effortSource = value, source
	}
	model = strings.ToLower(strings.TrimSpace(model))
	route, err := config.Selection.SelectRoute(model, effort)
	if err != nil {
		if replacement, retired := bridge.Retired[model]; retired {
			return bridge.Pair{}, "", "", fmt.Errorf("%w: %s -> %s", err, model, replacement)
		}
		return bridge.Pair{}, "", "", err
	}
	if explicitModel && !explicitEffort && !environmentEffort {
		effortSource = "factory.modelDefaults"
		if _, configured := config.Selection.ModelDefaults[route.Model]; configured {
			effortSource = config.SelectionSource + ".modelDefaults"
		}
	}
	if strings.HasPrefix(route.Source, "alias") || strings.HasPrefix(route.Source, "family") {
		for _, candidate := range bridge.Models {
			if model == candidate.Alias || candidate.Family != "" && strings.HasPrefix(model, candidate.Family) {
				mappingSource := "factory.modelMapping"
				if _, configured := config.Selection.ModelMapping[candidate.Alias]; configured {
					mappingSource = config.SelectionSource + ".modelMapping"
				}
				modelSource += "+" + mappingSource
				break
			}
		}
	}
	return bridge.Pair{Model: route.Model, Effort: route.Effort}, modelSource, effortSource, nil
}

// loadClauductSettings reads only the Clauduct-owned file under the given home.
// A missing file uses built-in defaults; a present malformed file fails closed.
func loadClauductSettings(home string) (ClauductSettings, error) {
	if !filepath.IsAbs(home) {
		return ClauductSettings{}, errClauductSettings
	}
	file, err := os.Open(filepath.Join(home, ".clauduct", "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return defaultClauductSettings(), nil
	}
	if err != nil {
		return ClauductSettings{}, errClauductSettings
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > clauductSettingsBytes {
		return ClauductSettings{}, errClauductSettings
	}
	raw, err := io.ReadAll(io.LimitReader(file, clauductSettingsBytes+1))
	if err != nil || len(raw) > clauductSettingsBytes {
		return ClauductSettings{}, errClauductSettings
	}
	return parseClauductSettings(raw)
}

func parseClauductSettings(raw []byte) (ClauductSettings, error) {
	bad := func() (ClauductSettings, error) { return ClauductSettings{}, errClauductSettings }
	fields, err := wire.Fields(raw, []string{"version", "startup", "modelDefaults", "modelMapping", "agents"})
	if err != nil {
		return bad()
	}
	settings := defaultClauductSettings()
	var version int
	if json.Unmarshal(fields["version"], &version) != nil || version != 1 {
		return bad()
	}
	if value, ok := fields["startup"]; ok {
		settings.Startup, err = bridge.ParsePair(value)
		if err != nil {
			return bad()
		}
		settings.StartupSource = "settings.startup"
	}
	delete(fields, "version")
	delete(fields, "startup")
	preferences, err := json.Marshal(fields)
	if err != nil {
		return bad()
	}
	settings.Selection, err = bridge.ParseSelection(preferences)
	if err != nil {
		return bad()
	}
	settings.SelectionSource = "settings"
	return settings, nil
}
