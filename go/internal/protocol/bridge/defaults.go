package bridge

import (
	"encoding/json"
	"errors"

	"github.com/wotjr1649/Clauduct/go/internal/settingsfile"
)

// Only the embedded, release-checked document is read here, and only for its shape. No
// model is looked up at this point: commands that never route (help, version, hooks) must
// not depend on which models a factory default names or an account offers. User files
// use the strict session parser and never mutate these process-wide factory fallbacks.
var builtinDefaults = func() (config struct {
	Version         int  `json:"version"`
	Startup         Pair `json:"startup"`
	ClassifierModel Pair `json:"classifier_model"`
	ContextSettings
	Selection
}) {
	if json.Unmarshal([]byte(settingsfile.Defaults()), &config) != nil || config.Version != 1 {
		panic("invalid embedded Clauduct defaults")
	}
	return config
}()

// DefaultStartup stays independent from per-model and per-agent effort defaults.
func DefaultStartup() Pair { return builtinDefaults.Startup }

// DefaultClassifierModel is the factory classifier pair; the zero Pair follows native's request.
func DefaultClassifierModel() Pair { return builtinDefaults.ClassifierModel }

// ContextSettings is shared by every model in one launcher. It is not part of
// Selection: resuming a saved selection still uses today's context preferences.
type ContextSettings struct {
	Window  int64 `json:"context_window"`
	Percent int64 `json:"auto_compact_token_limit_percent"`
}

func DefaultContextSettings() ContextSettings { return builtinDefaults.ContextSettings }

// Policy resolves the launch-time target once. Clamp before multiplication so
// even the largest accepted integer percentage cannot overflow the calculation.
func (s ContextSettings) Policy() (ContextPolicy, error) {
	if s.Window < 100000 || s.Window > 872000 || s.Percent < 1 {
		return ContextPolicy{}, errors.New("INVALID_CONTEXT_SETTINGS")
	}
	return ContextPolicy{Window: s.Window, CompactAt: s.Window * min(s.Percent, 90) / 100}, nil
}

func DefaultContextPolicy() ContextPolicy {
	policy, err := DefaultContextSettings().Policy()
	if err != nil {
		panic("invalid embedded Clauduct context defaults")
	}
	return policy
}
