package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/settingsfile"
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

var errBoundarySettings = fmt.Errorf(`%w: boundary_profile must be "default" or "strict". Edit ~/.clauduct/settings.json`, errClauductSettings)

// errClassifierSettings explains the one shape change v0.6.4 made to an existing key. The
// file is never converted: the user edits it, and nothing runs on a guessed effort.
var errClassifierSettings = fmt.Errorf(`%w: classifier_model must be an object with both model and effort, for example "classifier_model": {"model": "gpt-5.6-terra", "effort": "low"}; the v0.6.3 string form is no longer accepted. Edit ~/.clauduct/settings.json`, errClauductSettings)

// deprecatedSettings are keys v0.6.4 stopped applying. They are accepted, left in the file
// and reported; every other unknown key is still refused.
var deprecatedSettings = []string{"auto_compact_effort_cap", "auxiliary_effort_cap"}

// ClauductSettings contains validated launch preferences. SessionProfile owns
// the separately versioned persistent format.
type ClauductSettings struct {
	Startup                 bridge.Pair
	Selection               bridge.Selection
	StartupSource           string
	SelectionSource         string
	Context                 bridge.ContextSettings
	ContextRequestedPercent json.Number
	ContextPolicy           bridge.ContextPolicy
	ContextWindowSource     string
	ContextPercentSource    string
	ClassifierModel         bridge.Pair
	ClassifierModelSource   string
	// BoundaryProfile is "default" or "strict": which Anthropic-hosted paths native keeps
	// (#301, boundary_profile in settings.json).
	BoundaryProfile string
	// Deprecated names keys present in the file that this build ignores.
	Deprecated []string
}

// classifierMeasurements is what Clauduct's auto-mode classifier gate observed per pair on
// the real backend (core and independent public corpora): "" passed; any other value is the
// finding a user choosing that pair is told at start. A pair not listed was never measured.
// v0.6.4 lets classifier_model name any pair the account offers (#238); this does not refuse
// one, it says what is known about it (v0.6.7).
var classifierMeasurements = map[bridge.Pair]string{
	{Model: "gpt-5.6-terra", Effort: "low"}:    "",
	{Model: "gpt-5.6-terra", Effort: "medium"}: "",
	{Model: "gpt-5.6-terra", Effort: "high"}:   "allowed a dangerous sample action (2026-09-29)",
	{Model: "gpt-6-luna", Effort: "low"}:       "allowed dangerous sample actions (2026-10-03: 2 of 32 in the core set; 2026-09-29: 5)",
	{Model: "gpt-6-luna", Effort: "medium"}:    "allowed dangerous sample actions (2026-09-29, 2 of the core set)",
	{Model: "gpt-6.1-sol", Effort: "low"}:      "left a security sample undecided: the backend refused to classify it (cyber_policy, 2026-10-02)",
}

// classifierNotice is the start-up line for a classifier pair that did not pass the gate,
// or "" for one that did.
func classifierNotice(pair bridge.Pair) string {
	finding, measured := classifierMeasurements[pair]
	if !measured {
		finding = "not measured as an auto-mode classifier"
	} else if finding == "" {
		return ""
	}
	return fmt.Sprintf("classifier_model %s/%s: %s in Clauduct's classifier measurement; it is used as configured, and gpt-5.6-terra/low passed", pair.Model, pair.Effort, finding)
}

func classifierFindingOf(pair bridge.Pair) string {
	if pair == (bridge.Pair{}) {
		return ""
	}
	return classifierNotice(pair)
}

func defaultClauductSettings() ClauductSettings {
	return ClauductSettings{Startup: startupModel, StartupSource: "factory.startup", SelectionSource: "factory",
		Context: bridge.DefaultContextSettings(), ContextPolicy: bridge.DefaultContextPolicy(),
		ContextRequestedPercent: json.Number(strconv.FormatInt(bridge.DefaultContextSettings().Percent, 10)),
		ContextWindowSource:     "factory.context_window", ContextPercentSource: "factory.auto_compact_token_limit_percent",
		ClassifierModel: bridge.DefaultClassifierModel(), ClassifierModelSource: "factory.classifier_model",
		BoundaryProfile: boundaryDefault}
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
		if config.Selection.Retires(model) {
			return bridge.Pair{}, "", "", fmt.Errorf("%w: %s -> %s", err, model, bridge.Retired[model])
		}
		return bridge.Pair{}, "", "", err
	}
	if explicitModel && !explicitEffort && !environmentEffort {
		effortSource = "account.default_reasoning_level"
		if _, configured := config.Selection.ModelDefaults[route.Model]; configured {
			effortSource = config.SelectionSource + ".modelDefaults"
		}
	}
	if strings.HasPrefix(route.Source, "alias") || strings.HasPrefix(route.Source, "family") {
		for _, candidate := range bridge.LegacyModels() {
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
// A missing file uses the complete document Ensure will publish, including its
// explicit agent pairs. A present malformed file fails closed.
func loadClauductSettings(home string) (ClauductSettings, error) {
	if !filepath.IsAbs(home) {
		return ClauductSettings{}, errClauductSettings
	}
	file, err := os.Open(filepath.Join(home, ".clauduct", "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return parseClauductSettings([]byte(settingsfile.Defaults()))
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
	fields, err := wire.Fields(raw, append([]string{"version", "startup", "modelDefaults", "modelMapping", "agents", "context_window", "auto_compact_token_limit_percent", "classifier_model", "boundary_profile"}, deprecatedSettings...))
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
	if value, present := fields["context_window"]; present {
		if string(value) == "null" || json.Unmarshal(value, &settings.Context.Window) != nil {
			return bad()
		}
		settings.ContextWindowSource = "settings.context_window"
		delete(fields, "context_window")
	}
	if value, present := fields["auto_compact_token_limit_percent"]; present {
		// Fields already validated the JSON token. Reject non-integer notation;
		// positive int64 overflow is still above 90 and must clamp, not fail.
		number := string(value)
		percent, parseErr := strconv.ParseInt(number, 10, 64)
		if strings.ContainsAny(number, ".eE") || parseErr != nil && !errors.Is(parseErr, strconv.ErrRange) || percent < 1 {
			return bad()
		}
		settings.Context.Percent = percent
		settings.ContextRequestedPercent = json.Number(number)
		settings.ContextPercentSource = "settings.auto_compact_token_limit_percent"
		delete(fields, "auto_compact_token_limit_percent")
	}
	for _, name := range deprecatedSettings {
		if _, present := fields[name]; present {
			settings.Deprecated = append(settings.Deprecated, name)
			delete(fields, name)
		}
	}
	if value, present := fields["classifier_model"]; present {
		// Both halves of an exact backend ID and a transmittable effort. An alias would move
		// with modelMapping; the account list decides availability when it is used.
		var legacy string
		if json.Unmarshal(value, &legacy) == nil {
			return ClauductSettings{}, errClassifierSettings
		}
		settings.ClassifierModel, err = bridge.ParsePair(value)
		if err != nil {
			return ClauductSettings{}, errClassifierSettings
		}
		settings.ClassifierModelSource = "settings.classifier_model"
		delete(fields, "classifier_model")
	}
	if value, present := fields["boundary_profile"]; present {
		if string(value) == "null" || json.Unmarshal(value, &settings.BoundaryProfile) != nil ||
			settings.BoundaryProfile != boundaryDefault && settings.BoundaryProfile != boundaryStrict {
			return ClauductSettings{}, errBoundarySettings
		}
		delete(fields, "boundary_profile")
	}
	settings.ContextPolicy, err = settings.Context.Policy()
	if err != nil {
		return bad()
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
