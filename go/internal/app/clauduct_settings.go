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
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

func nativeConfigDirectory(env map[string]string, cwd string) string {
	configDir, _ := windowsEnvValue(env, "CLAUDE_CONFIG_DIR")
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

func windowsEnvValue(env map[string]string, name string) (string, bool) {
	spelling := ""
	for key := range env {
		if strings.EqualFold(key, name) && key > spelling {
			spelling = key // Match launch's sorted, case-insensitive last-wins rule.
		}
	}
	return env[spelling], spelling != ""
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

var errClauductSettings = errors.New(
	`CLAUDUCT_SETTINGS_INVALID: edit ~/.clauduct/settings.json to use v2 (minimal form: {"version":2})`)

var errBoundarySettings = invalidSettingsField("boundaryProfile", nil)
var errClassifierSettings = fmt.Errorf(`%w: use "classifier":{"model":"gpt-6.1-terra","effort":"low"} to set both values`, errClauductSettings)

// ClauductSettings contains launch preferences. SessionProfile owns the saved routing format.
type ClauductSettings struct {
	Startup                 bridge.Pair
	Selection               bridge.Selection
	autoModel               string
	nativeTierSettings      map[string]bool
	restoredSelection       bool
	shellModel              string
	Context                 bridge.ContextSettings
	ContextRequestedPercent json.Number
	ContextPolicy           bridge.ContextPolicy
	ContextWindowSource     string
	ContextPercentSource    string
	ClassifierModel         bridge.Pair
	ClassifierModelSource   string
	BoundaryProfile         string
	Isolation               string
	pluginCacheRoot         string
	marketplace             string
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
	{Model: "gpt-6.1-terra", Effort: "low"}:    "blocked a critical safe sample action (2026-11-10, 1 of the core set)",
	{Model: "gpt-6.1-terra", Effort: "medium"}: "left a security sample undecided: the backend refused to classify it (cyber_policy, 2026-11-10)",
	{Model: "gpt-6.1-terra", Effort: "high"}:   "left a security sample undecided: the backend refused to classify it (cyber_policy, 2026-11-10)",
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
	return fmt.Sprintf("classifier %s/%s: %s in Clauduct's classifier measurement; it is used as configured, and gpt-5.6-terra/low passed", pair.Model, pair.Effort, finding)
}

func classifierFindingOf(pair bridge.Pair) string {
	if pair == (bridge.Pair{}) {
		return ""
	}
	return classifierNotice(pair)
}

func defaultClauductSettings() ClauductSettings {
	return ClauductSettings{
		Context: bridge.DefaultContextSettings(), ContextPolicy: bridge.DefaultContextPolicy(),
		ContextRequestedPercent: json.Number(strconv.FormatInt(bridge.DefaultContextSettings().Percent, 10)),
		ContextWindowSource:     "context default", ContextPercentSource: "context default",
		ClassifierModel: bridge.DefaultClassifierModel(), ClassifierModelSource: "classifier default",
		BoundaryProfile: boundaryDefault, Isolation: isolationOff,
	}
}

// A missing file has exactly the same defaults as {"version":2}.
func loadClauductSettings(home string) (ClauductSettings, error) {
	if !filepath.IsAbs(home) {
		return ClauductSettings{}, errClauductSettings
	}
	path := filepath.Join(home, ".clauduct", "settings.json")
	file, err := os.Open(path)
	invalid := fmt.Errorf("%w: %s", errClauductSettings, path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultClauductSettings(), nil
	}
	if err != nil {
		return ClauductSettings{}, invalid
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > clauductSettingsBytes {
		return ClauductSettings{}, invalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, clauductSettingsBytes+1))
	if err != nil || len(raw) > clauductSettingsBytes {
		return ClauductSettings{}, invalid
	}
	config, err := parseClauductSettings(raw)
	if err != nil {
		return ClauductSettings{}, fmt.Errorf("%w: %s", err, path)
	}
	return config, nil
}

func parseClauductSettings(raw []byte) (ClauductSettings, error) {
	fields, err := wire.Fields(raw, []string{"version", "boundaryProfile", "isolation", "context", "classifier"})
	if err != nil {
		return ClauductSettings{}, invalidSettingsField("settings", err)
	}
	var version int
	if json.Unmarshal(fields["version"], &version) != nil || version != 2 {
		return ClauductSettings{}, invalidSettingsField("version", nil)
	}
	config := defaultClauductSettings()
	if value, present := fields["boundaryProfile"]; present {
		config.BoundaryProfile = "" // JSON null must not inherit the default.
		if json.Unmarshal(value, &config.BoundaryProfile) != nil ||
			config.BoundaryProfile != boundaryDefault && config.BoundaryProfile != boundaryStrict {
			return ClauductSettings{}, errBoundarySettings
		}
	}
	if value, present := fields["isolation"]; present {
		config.Isolation, err = parseIsolation(value)
		if err != nil {
			return ClauductSettings{}, err
		}
	}
	if value, present := fields["context"]; present {
		context, err := wire.Fields(value, []string{"window", "compactPercent"})
		var window int64
		number := string(context["compactPercent"])
		percent, numberErr := strconv.ParseInt(number, 10, 64)
		// Positive overflow still clamps to 90. Only JSON integer notation is accepted.
		if err != nil {
			return ClauductSettings{}, invalidSettingsField("context", err)
		}
		if json.Unmarshal(context["window"], &window) != nil || window == 0 {
			return ClauductSettings{}, invalidSettingsField("context.window", nil)
		}
		if strings.ContainsAny(number, ".eE") || numberErr != nil && !errors.Is(numberErr, strconv.ErrRange) || percent < 1 {
			return ClauductSettings{}, invalidSettingsField("context.compactPercent", nil)
		}
		config.Context = bridge.ContextSettings{Window: window, Percent: percent}
		config.ContextPolicy, err = config.Context.Policy()
		if err != nil {
			return ClauductSettings{}, invalidSettingsField("context.window", nil)
		}
		config.ContextRequestedPercent = json.Number(number)
		config.ContextWindowSource, config.ContextPercentSource = "settings.context.window", "settings.context.compactPercent"
	}
	if value, present := fields["classifier"]; present {
		config.ClassifierModel, err = bridge.ParsePair(value)
		if err != nil {
			_, shape := wire.Fields(value, []string{"model", "effort"})
			var key *wire.FieldError
			if errors.As(shape, &key) {
				return ClauductSettings{}, fmt.Errorf("%w: %v %q", errClassifierSettings, key.Reason, key.Field)
			}
			return ClauductSettings{}, errClassifierSettings
		}
		config.ClassifierModelSource = "settings.classifier"
	}
	return config, nil
}

func invalidSettingsField(field string, cause error) error {
	var key *wire.FieldError
	if errors.As(cause, &key) {
		return fmt.Errorf("%w: %s: %v %q", errClauductSettings, field, key.Reason, key.Field)
	}
	return fmt.Errorf("%w: invalid %s", errClauductSettings, field)
}
