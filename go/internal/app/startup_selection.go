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
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// ErrResumeUUIDRequired is an early refusal of an ambiguous resume entry point.
var ErrResumeUUIDRequired = errors.New("SESSION_RESUME_UUID_REQUIRED: use clauduct --resume <UUID> with a recorded Clauduct session")

// ErrSessionProfileMissing refuses a --resume UUID that has no Clauduct session profile.
var ErrSessionProfileMissing = errors.New("SESSION_PROFILE_MISSING: the requested UUID has no Clauduct profile")

var errNativeSelection = errors.New("NATIVE_SELECTION_UNVERIFIED: check Claude Code model and effort settings")
var errNativeFallback = errors.New("NATIVE_MODEL_FALLBACK_UNSUPPORTED: remove fallbackModel or --fallback-model and choose a model explicitly")
var errNativePolicy = errors.New("NATIVE_MODEL_POLICY: the configured model or effort is disallowed or its policy cannot be verified")

var tierEnvironment = map[string]string{
	"fable":  "ANTHROPIC_DEFAULT_FABLE_MODEL",
	"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
	"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
	"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
}

type nativeSelectionLayer struct {
	name   string
	fields map[string]json.RawMessage
	raw    []byte
}

type startupInputs struct {
	args      []string
	env       map[string]string
	layers    []nativeSelectionLayer
	limits    []string
	denied    []string
	maxEffort string
	matchMode string
}

func readStartupInputs(args []string, cwd, config string, env map[string]string, flag map[string]json.RawMessage, managed []policyDocument) (startupInputs, error) {
	input := startupInputs{args: args, env: env}
	cli := cliRoles{}
	if cli.scope(args, cwd) != nil || cli.flagErr != nil {
		return input, errNativeSelection
	}
	appendLayer := func(name string, raw []byte) error {
		fields, err := wire.Fields(raw, nil)
		if err != nil {
			return errNativeSelection
		}
		input.layers = append(input.layers, nativeSelectionLayer{name, fields, raw})
		return nil
	}
	// Native loads project settings in print mode without an interactive trust dialog.
	trustedProject := printMode(args) || nativeProjectTrusted(cwd, config, env)
	for _, file := range []struct{ source, path string }{
		{"user", filepath.Join(config, "settings.json")},
		{"project", filepath.Join(cwd, ".claude", "settings.json")},
		{"local", filepath.Join(cwd, ".claude", "settings.local.json")},
	} {
		if !cli.source(file.source) || file.source != "user" && !trustedProject {
			continue
		}
		raw, err := boundedRoleFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || appendLayer(file.source, raw) != nil {
			return input, errNativeSelection
		}
	}
	if flag != nil {
		raw, err := json.Marshal(flag)
		if err != nil || appendLayer("settings", raw) != nil {
			return input, errNativeSelection
		}
	}
	for _, doc := range managed {
		if err := appendLayer("managed", []byte(doc.JSON)); err != nil {
			return input, errNativePolicy
		}
	}
	if _, set := optionValue(args, "--fallback-model"); set {
		return input, errNativeFallback
	}
	if raw, set := optionValue(args, "--managed-settings"); set {
		if err := appendLayer("managed", []byte(raw)); err != nil {
			return input, errNativePolicy
		}
	}
	var managedModels []string
	for _, layer := range input.layers {
		if value, set := layer.fields["fallbackModel"]; set && string(value) != "null" && string(value) != `""` {
			return input, errNativeFallback
		}
		if raw, set := layer.fields["switchModelsOnFlag"]; layer.name == "managed" && set && string(raw) != "false" {
			return input, errNativeFallback
		}
		if raw, set := layer.fields["enforceAvailableModels"]; set && string(raw) != "false" {
			return input, errNativePolicy
		}
		for _, key := range []string{"availableModels", "deniedModels"} {
			raw, set := layer.fields[key]
			if !set || key == "deniedModels" && layer.name != "managed" {
				continue
			}
			var names []string
			if json.Unmarshal(raw, &names) != nil || names == nil {
				return input, errNativePolicy
			}
			for _, name := range names {
				if !bridge.ValidModelID(name) {
					return input, errNativePolicy
				}
			}
			switch {
			case key == "deniedModels":
				input.denied = append(input.denied, names...)
			case layer.name == "managed":
				if managedModels != nil && !slices.Equal(managedModels, names) {
					return input, errNativePolicy
				}
				managedModels = names
			default:
				input.limits = slices.Concat(input.limits, names)
				if input.limits == nil {
					input.limits = names // Keep an explicit empty list.
				}
			}
		}
		if raw, set := layer.fields["availableModelsMatch"]; set && layer.name == "managed" {
			var mode string
			if json.Unmarshal(raw, &mode) != nil || mode != "prefix" && mode != "exact" ||
				input.matchMode != "" && input.matchMode != mode {
				return input, errNativePolicy
			}
			input.matchMode = mode
		}
		if raw, set := layer.fields["maxEffortLevel"]; set {
			var effort string
			if json.Unmarshal(raw, &effort) != nil || !bridge.TransmittableEffort(effort) {
				return input, errNativePolicy
			}
			if input.maxEffort == "" || effortAbove(input.maxEffort, effort) {
				input.maxEffort = effort
			}
		}
	}
	if managedModels != nil {
		input.limits = managedModels
	}
	return input, nil
}

// Interactive launches use native's recorded workspace trust when previewing
// project settings. Native still owns application of those settings.
func nativeProjectTrusted(cwd, config string, env map[string]string) bool {
	home, _ := os.UserHomeDir()
	custom, _ := windowsEnvValue(env, "CLAUDE_CONFIG_DIR")
	raw, err := readIfExists(sourceGlobalConfig(config, home, custom != ""), deriveClaudeJSONBytes)
	var state struct {
		Projects map[string]struct {
			Trusted bool `json:"hasTrustDialogAccepted"`
		} `json:"projects"`
	}
	if err != nil || json.Unmarshal(raw, &state) != nil {
		return false
	}
	verified := false
	for path, project := range state.Projects {
		if sameFolder(path, cwd) {
			if !project.Trusted {
				return false
			}
			verified = true
		}
	}
	return verified
}

func effortAbove(effort, cap string) bool {
	levels := []string{"low", "medium", "high", "xhigh", "max"}
	return slices.Index(levels, effort) > slices.Index(levels, cap)
}

func (in startupInputs) permits(pair bridge.Pair, selection bridge.Selection) bool {
	if in.maxEffort != "" && effortAbove(pair.Effort, in.maxEffort) {
		return false
	}
	matches := func(name string) bool {
		if name == pair.Model {
			return true
		}
		model, known := selection.ForAlias(name)
		return known && model.ID == pair.Model
	}
	extends := func(name string) bool { return strings.HasPrefix(pair.Model, name+"-") }
	return (in.limits == nil || slices.ContainsFunc(in.limits, func(name string) bool {
		return matches(name) || in.matchMode != "exact" && extends(name)
	})) && !slices.ContainsFunc(in.denied, func(name string) bool {
		return matches(name) || strings.HasPrefix(pair.Model, name)
	})
}

// Native applies settings env after inheriting the process env. Never copy a
// project or local env value into the child env ahead of native's trust decision.
func (in startupInputs) environment(key string) (value, source string, err error) {
	if shell, set := windowsEnvValue(in.env, key); set {
		value, source = shell, "native.environment."+key
	}
	for _, layer := range in.layers {
		v, found, err := settingsEnvironment(layer.raw, key)
		if err != nil {
			return "", "", errNativeSelection
		}
		if found {
			value, source = v, "native."+layer.name+".env."+key
		}
	}
	return value, source, nil
}

func (in startupInputs) model() (string, string, error) {
	if model, set := optionValue(in.args, "--model"); set {
		if strings.TrimSpace(model) == "" {
			return "", "", errNativeSelection
		}
		return model, "native.cli.model", nil
	}
	model, source, err := in.environment("ANTHROPIC_MODEL")
	if err != nil || model != "" {
		return model, source, err
	}
	for i := len(in.layers) - 1; i >= 0; i-- {
		layer := in.layers[i]
		if raw, set := layer.fields["model"]; set {
			if json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" {
				return "", "", errNativeSelection
			}
			return model, "native." + layer.name + ".model", nil
		}
	}
	return "", "", nil
}

func (in startupInputs) explicitEffort() (string, string, error) {
	effort, source, err := in.environment(effortEnv)
	if err != nil || effort != "" {
		return effort, source, err
	}
	if effort, set := optionValue(in.args, "--effort"); set {
		if effort == "" {
			return "", "", errNativeSelection
		}
		return effort, "native.cli.effort", nil
	}
	return "", "", nil
}

func (in startupInputs) effort(model string) (string, string, error) {
	effort, source, err := in.explicitEffort()
	if err != nil || effort != "" {
		return effort, source, err
	}
	for _, perModel := range []bool{true, false} {
		for i := len(in.layers) - 1; i >= 0; i-- {
			layer := in.layers[i]
			raw, key := layer.fields["effortLevel"], "effortLevel"
			if perModel {
				var models map[string]map[string]json.RawMessage
				if value, set := layer.fields["modelSettings"]; set && (json.Unmarshal(value, &models) != nil || models == nil) {
					return "", "", errNativeSelection
				}
				raw, key = models[model]["effortLevel"], "modelSettings.effortLevel"
			}
			if raw != nil {
				if json.Unmarshal(raw, &effort) != nil || !storedEffort(effort) {
					return "", "", fmt.Errorf("%w: invalid native %s", bridge.ErrUnsupportedRoute, key)
				}
				return effort, "native." + layer.name + "." + key, nil
			}
		}
	}
	return "", "", nil
}

// launchAliases changes only shell-owned inputs. Settings-file env is applied
// by native, and is used to predict and verify routing without moving its scope.
func (config *ClauductSettings) launchAliases(in startupInputs) (map[string]string, error) {
	aliasSources := make(map[string]string, len(tierEnvironment))
	config.Selection.ModelMapping = make(map[string]string, len(tierEnvironment))
	config.nativeTierSettings = make(map[string]bool, len(tierEnvironment))
	for alias, key := range tierEnvironment {
		value, source, err := in.environment(key)
		if err != nil {
			return nil, err
		}
		if value == "" {
			aliasSources[alias] = "compat mapping"
			continue
		}
		if !bridge.ValidModelID(value) || bridge.LegacyAlias(value) {
			return nil, errNativeSelection
		}
		if _, ok := config.Selection.ModelByID(value); !ok {
			return nil, fmt.Errorf("%w: %s must name a model offered by the account", errNativeSelection, key)
		}
		config.Selection.ModelMapping[alias], aliasSources[alias] = value, source
		config.nativeTierSettings[alias] = source != "native.environment."+key
	}
	return aliasSources, nil
}

func (in startupInputs) checkRemap(selection bridge.Selection, requested, id string) error {
	for _, layer := range in.layers {
		if raw, set := layer.fields["modelOverrides"]; set {
			var remap map[string]string
			if json.Unmarshal(raw, &remap) != nil || remap == nil {
				return errNativePolicy
			}
			for from, to := range remap {
				if from == to {
					continue
				}
				route, _ := selection.SelectRoute(from, "")
				if from == requested || from == id || route.Model == id {
					return errNativePolicy
				}
			}
		}
	}
	return nil
}

func (config *ClauductSettings) selectStartup(in startupInputs, facts ModelListFacts) (string, string, error) {
	aliases, err := config.launchAliases(in)
	if err != nil {
		return "", "", err
	}
	model, source, err := in.model()
	if err != nil {
		return "", "", err
	}
	if source == "native.environment.ANTHROPIC_MODEL" {
		config.shellModel = strings.ToLower(strings.TrimSpace(model))
	}
	if model == "" {
		model, source, err = in.environment("ANTHROPIC_DEFAULT_MODEL")
		if err != nil {
			return "", "", err
		}
		if model != "" && source == "native.environment.ANTHROPIC_DEFAULT_MODEL" {
			config.autoModel = strings.ToLower(strings.TrimSpace(model))
		}
	}
	if model == "" {
		if facts.Source != "account" {
			return "", "", errors.New("ACCOUNT_PRIORITY_UNAVAILABLE: only a saved model list is available; use --model and, if needed, --effort")
		}
		pair, reason := config.Selection.Catalogue().PriorityDefault(func(pair bridge.Pair) bool {
			if in.maxEffort != "" {
				effort, _, err := in.effort(pair.Model)
				if err != nil {
					return true // Report the selected model's invalid effort below.
				}
				if effort != "" {
					pair.Effort = effort
				}
			}
			return in.permits(pair, config.Selection)
		})
		if reason != "" {
			return "", "", fmt.Errorf("ACCOUNT_PRIORITY_UNAVAILABLE: %s; use --model and, if needed, --effort", reason)
		}
		model, source, config.autoModel = pair.Model, "account priority", pair.Model
	}
	model = strings.ToLower(strings.TrimSpace(model))
	id, err := startupModelID(config.Selection, model)
	if err != nil {
		return "", "", err
	}
	if aliasSource := aliases[model]; aliasSource != "" {
		source += " + " + aliasSource
	}
	effort, effortSource, err := in.effort(id)
	if err != nil {
		return "", "", err
	}
	if effort == "" {
		effort, _ = config.Selection.DefaultFor(id)
		effortSource = "account.default_reasoning_level"
	}
	config.Startup = bridge.Pair{Model: id, Effort: effort}
	if !config.Selection.ValidPair(config.Startup) {
		return "", "", fmt.Errorf("%w: choose a supported --effort for the selected model", bridge.ErrUnsupportedRoute)
	}
	if !in.permits(config.Startup, config.Selection) {
		return "", "", errNativePolicy
	}
	return source, effortSource, in.checkRemap(config.Selection, model, id)
}

// taskStartup uses a worker's own native dispatch flags and SessionStart directory.
// The native's job state must bind the same UUID and this exact session link.
func taskStartup(config ClauductSettings, launchIn startupInputs, facts ModelListFacts, foreground func(gateway.NativeProcess) bool, configDir, link string, env map[string]string, managed []policyDocument) func(string, string, gateway.NativeProcess) (bridge.Pair, func(bridge.Pair) bool, error) {
	return func(session, cwd string, native gateway.NativeProcess) (bridge.Pair, func(bridge.Pair) bool, error) {
		if native.PID <= 1 || native.Created <= 0 {
			return bridge.Pair{}, nil, errNativeSelection
		}
		if foreground != nil && foreground(native) {
			return config.Startup, func(pair bridge.Pair) bool { return launchIn.permits(pair, config.Selection) }, nil
		}
		if !filepath.IsAbs(cwd) || len(cwd) > 4096 {
			return bridge.Pair{}, nil, errNativeSelection
		}
		var flags []string
		ambiguous := false
		walkNativeJobs(configDir, link, func(_ string, raw []byte) bool {
			var job struct {
				Session string   `json:"sessionId"`
				Cwd     string   `json:"cwd"`
				Flags   []string `json:"respawnFlags"`
			}
			if json.Unmarshal(raw, &job) != nil || job.Session != session {
				return true
			}
			if flags != nil || job.Cwd != "" && !strings.EqualFold(filepath.Clean(cwd), filepath.Clean(job.Cwd)) {
				ambiguous = true
				return false
			}
			flags = job.Flags
			return true
		})
		if flags == nil || ambiguous {
			return bridge.Pair{}, nil, errNativeSelection
		}
		value, _ := optionValue(flags, "--settings")
		fields, err := wire.Fields([]byte(value), nil)
		if err != nil {
			return bridge.Pair{}, nil, errNativeSelection
		}
		in, err := readStartupInputs(flags, cwd, configDir, env, fields, managed)
		if err != nil {
			return bridge.Pair{}, nil, err
		}
		task := config
		if _, _, err := task.selectStartup(in, facts); err != nil {
			return bridge.Pair{}, nil, err
		}
		return task.Startup, func(pair bridge.Pair) bool { return in.permits(pair, task.Selection) }, nil
	}
}

// Exact backend IDs and known short names are valid launch choices. Versioned
// Claude family names can appear on auxiliary requests but are not launch aliases.
func startupModelID(selection bridge.Selection, model string) (string, error) {
	id := model
	if alias, known := selection.ForAlias(model); known {
		id = alias.ID
	}
	for _, legacy := range bridge.LegacyModels() {
		if legacy.Key == model {
			id = legacy.ID
			break
		}
	}
	if _, listed := selection.ModelByID(id); listed {
		return id, nil
	}
	if selection.Retires(id) {
		return "", fmt.Errorf("%w: %s -> %s", bridge.ErrRetiredRoute, id, bridge.Retired[id])
	}
	return "", fmt.Errorf("%w: choose an account model with --model", bridge.ErrUnsupportedRoute)
}

// requiresResumeUUID recognises combined short flags at native argv boundaries.
func requiresResumeUUID(args []string, resumeID string) bool {
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			return false
		}
		name, _, _ := strings.Cut(args[i], "=")
		switch name {
		case "--continue", "--from-pr", "--teleport":
			return true
		case "--resume", "--fork-session":
			if resumeID == "" {
				return true
			}
		}
		if len(name) > 1 && name[0] == '-' && name[1] != '-' {
			for _, flag := range name[1:] {
				if flag == 'c' || flag == 'r' && resumeID == "" {
					return true
				}
				if !strings.ContainsRune("pvh", flag) {
					break
				}
			}
		}
		i = launch.VariadicValuesEnd(args, i, end)
	}
	return false
}

func (config *ClauductSettings) selectResume(in startupInputs, resident bool) (string, string, error) {
	modelSource, effortSource := "session-snapshot.last", "session-snapshot.last"
	pair := config.Startup
	requested := pair.Model
	// Resident argv was saved at the original dispatch. It does not change the
	// last pair of the job being recovered.
	if !resident {
		if model, explicit := optionValue(in.args, "--model"); explicit {
			requested = strings.ToLower(strings.TrimSpace(model))
			id, err := startupModelID(config.Selection, requested)
			if err != nil {
				return "", "", err
			}
			if id != pair.Model {
				pair.Model = id
				pair.Effort, _ = config.Selection.DefaultFor(id)
				effortSource = "session-snapshot.model-default"
				if _, configured := config.Selection.ModelDefaults[id]; !configured {
					effortSource = "account.default_reasoning_level"
				}
			}
			modelSource = "native.cli.model"
		}
		if effort, explicit := optionValue(in.args, "--effort"); explicit {
			pair.Effort, effortSource = effort, "native.cli.effort"
		}
	}
	if !config.Selection.ValidPair(pair) {
		return "", "", fmt.Errorf("%w: the saved or explicitly selected session pair is unavailable on this account", bridge.ErrUnsupportedRoute)
	}
	envEffort, _, err := in.environment(effortEnv)
	if err != nil {
		return "", "", err
	}
	if !resident && envEffort != "" && envEffort != pair.Effort {
		return "", "", errors.New("SESSION_PAIR_CONFLICT: CLAUDE_CODE_EFFORT_LEVEL conflicts with the resume effort")
	}
	if !in.permits(pair, config.Selection) {
		return "", "", errNativePolicy
	}
	for alias, key := range tierEnvironment {
		value, source, err := in.environment(key)
		if err != nil {
			return "", "", err
		}
		if source != "" && source != "native.environment."+key {
			model, listed := config.Selection.ForAlias(alias)
			if !listed && value != "" || listed && value != model.ID {
				return "", "", errNativePolicy
			}
		}
	}
	if err := in.checkRemap(config.Selection, requested, pair.Model); err != nil {
		return "", "", err
	}
	config.Startup = pair
	return modelSource, effortSource, nil
}

// StartupReport previews selection without making a backend request.
func StartupReport(out io.Writer, home, cwd string, env map[string]string, account string) {
	config, err := loadClauductSettings(home)
	if err != nil {
		fmt.Fprintln(out, "selection   "+err.Error())
		return
	}
	fmt.Fprintf(out, "classifier  %s/%s (%s)\n", config.ClassifierModel.Model, config.ClassifierModel.Effort, config.ClassifierModelSource)
	docs, err := readManagedPolicy(env)
	if err != nil || managedRouting(docs, err) != nil {
		fmt.Fprintln(out, "selection   managed settings could not be verified")
		return
	}
	in, err := readStartupInputs(nil, cwd, nativeConfigDirectory(env, cwd), env, nil, docs)
	if err != nil {
		fmt.Fprintln(out, "selection   "+err.Error())
		return
	}
	list, err := upstream.LoadAccountModels(home, account)
	if err != nil {
		model, source, modelErr := in.model()
		if modelErr != nil {
			fmt.Fprintln(out, "selection   "+modelErr.Error())
			return
		}
		model = strings.ToLower(strings.TrimSpace(model))
		if bridge.ValidModelID(model) {
			fmt.Fprintf(out, "model       %s (%s, account availability unverified)\n", model, source)
			if effort, source, err := in.effort(model); err == nil && bridge.TransmittableEffort(effort) {
				fmt.Fprintf(out, "effort      %s (%s, account availability unverified)\n", effort, source)
			}
		}
		fmt.Fprintln(out, "selection   account priority and account default effort require a live model list")
		return
	}
	catalogue, _, err := catalogueFor(list)
	if err != nil {
		fmt.Fprintln(out, "selection   saved catalogue has no routable models")
		return
	}
	config.Selection = config.Selection.WithCatalogue(catalogue)
	model, effort, err := config.selectStartup(in, ModelListFacts{Source: "account"})
	if err != nil {
		fmt.Fprintln(out, "selection   "+err.Error())
		return
	}
	fmt.Fprintf(out, "model       %s (%s)\neffort      %s (%s)\n", config.Startup.Model, model, config.Startup.Effort, effort)
	fmt.Fprintf(out, "selection   preview from saved account list (%s); auto selection requires a fresh fetch\n", list.FetchedAt.UTC().Format("2006-01-02T15:04:05Z"))
	if selected, ok := catalogue.ByID(config.Startup.Model); ok && selected.Upgrade != "" {
		fmt.Fprintf(out, "upgrade     account suggests %s\n", selected.Upgrade)
	}
}
