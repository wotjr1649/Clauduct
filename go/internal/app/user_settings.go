package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

const settingsBytes = 2 << 20

var errUserSettings = errors.New("SETTINGS_INVALID: expected one bounded JSON object or regular JSON file")
var errSettingsConflict = errors.New("SETTINGS_CONFLICT: required Clauduct hooks or connection settings cannot be disabled or replaced")

// Read the last real --settings source and record every settings slot for Run to
// replace with the merged blob. Keep each slot: removing it lets an earlier
// optional/variadic option consume the following prompt. Other argv is unchanged.
func takeUserSettings(args []string, cwd string) ([]string, map[string]json.RawMessage, []int, error) {
	forward := make([]string, 0, len(args))
	var slots []int
	value, present := "", false
	for i := 0; i < len(args); {
		if args[i] == "--" {
			forward = append(forward, args[i:]...)
			break
		}
		end, known := nativeArgEnd(args, i)
		if !known {
			// An unknown option might consume even the next '--' or known
			// option name. Do not trust any later boundary for settings.
			for _, arg := range args[i+1:] {
				if name, _, _ := strings.Cut(arg, "="); name == "--settings" {
					return nil, nil, nil, errUserSettings
				}
			}
			forward = append(forward, args[i:]...)
			break
		}
		name, attached, hasValue := strings.Cut(args[i], "=")
		if name != "--settings" {
			end = min(end, len(args))
			forward = append(forward, args[i:end]...)
			i = end
			continue
		}
		if end > len(args) {
			return nil, nil, nil, errUserSettings
		}
		if !hasValue {
			attached = args[i+1]
		}
		value, present = attached, true
		slots = append(slots, len(forward))
		forward = append(forward, "--settings="+attached)
		i = end
	}
	if !present {
		return forward, nil, nil, nil
	}
	if len(value) > settingsBytes || strings.TrimSpace(value) == "" {
		return nil, nil, nil, errUserSettings
	}
	raw := []byte(value)
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		file := value
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		f, err := os.Open(file)
		if err != nil {
			return nil, nil, nil, errUserSettings
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > settingsBytes {
			return nil, nil, nil, errUserSettings
		}
		raw, err = io.ReadAll(io.LimitReader(f, settingsBytes+1))
		if err != nil || len(raw) > settingsBytes {
			return nil, nil, nil, errUserSettings
		}
	}
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, nil, nil, errUserSettings
	}
	if raw, ok := fields["disableAllHooks"]; ok && string(raw) != "false" {
		return nil, nil, nil, errSettingsConflict
	}
	if raw, ok := fields["env"]; ok {
		env, err := wire.Fields(raw, nil)
		if err != nil {
			return nil, nil, nil, errUserSettings
		}
		for key, raw := range env {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return nil, nil, nil, errUserSettings
			}
			upper := strings.ToUpper(key)
			if required, ok := sessionRequirements()[upper]; ok && value != required {
				return nil, nil, nil, errSettingsConflict
			}
			switch upper {
			case "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR", "CLAUDUCT_PDF_PROJECTS_ROOT":
				return nil, nil, nil, errSettingsConflict
			case "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS":
				if value != "1" {
					return nil, nil, nil, errSettingsConflict
				}
			}
		}
	}
	return forward, fields, slots, nil
}

// Native remains responsible for user setting schemas and hook permissions.
// Keep user hook entries intact and append the required bindings to each event.
func mergeUserSettings(required string, user map[string]json.RawMessage) (string, error) {
	if user == nil {
		return required, nil
	}
	base, err := wire.Fields([]byte(required), nil)
	if err != nil {
		return "", errUserSettings
	}
	out := make(map[string]json.RawMessage, len(user)+len(base))
	for key, value := range user {
		out[key] = value
	}
	for key, value := range base {
		if key == "hooks" {
			merged, err := mergeHookSettings(value, user[key])
			if err != nil {
				return "", err
			}
			out[key] = merged
		} else if specified, ok := user[key]; ok && !jsonEqual(specified, value) {
			return "", errSettingsConflict
		} else {
			out[key] = value
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > settingsBytes {
		return "", errUserSettings
	}
	return string(raw), nil
}

func mergeHookSettings(required, user json.RawMessage) (json.RawMessage, error) {
	base, err := wire.Fields(required, nil)
	if err != nil {
		return nil, errUserSettings
	}
	if len(user) == 0 {
		return required, nil
	}
	fields, err := wire.Fields(user, nil)
	if err != nil {
		return nil, errUserSettings
	}
	for event, raw := range base {
		var bindings, extra []json.RawMessage
		if json.Unmarshal(raw, &bindings) != nil {
			return nil, errUserSettings
		}
		if old, ok := fields[event]; ok {
			if json.Unmarshal(old, &extra) != nil || extra == nil {
				return nil, errUserSettings
			}
		}
		fields[event], _ = json.Marshal(append(extra, bindings...))
	}
	return json.Marshal(fields)
}

func jsonEqual(a, b json.RawMessage) bool {
	var first, second any
	left, right := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	left.UseNumber()
	right.UseNumber()
	if left.Decode(&first) != nil || right.Decode(&second) != nil {
		return false
	}
	one, _ := json.Marshal(first)
	two, _ := json.Marshal(second)
	return string(one) == string(two)
}
