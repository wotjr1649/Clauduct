package bridge

import "errors"

var compatModelMapping = map[string]string{
	"fable":  "gpt-6-astra",
	"opus":   "gpt-6.1-sol",
	"sonnet": "gpt-6.1-terra",
	"haiku":  "gpt-6-luna",
}

func DefaultClassifierModel() Pair { return Pair{Model: "gpt-6.1-terra", Effort: "low"} }

// ContextSettings is shared by every model in one launcher. It is not part of
// Selection: resuming a saved selection still uses today's context preferences.
type ContextSettings struct {
	Window  int64 `json:"context_window"`
	Percent int64 `json:"auto_compact_token_limit_percent"`
}

func DefaultContextSettings() ContextSettings { return ContextSettings{Window: 272000, Percent: 90} }

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
