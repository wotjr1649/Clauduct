package bridge

import (
	"encoding/json"

	"github.com/wotjr1649/Clauduct/go/internal/settingsfile"
)

// Only the embedded, release-checked document is read here. User files use the
// strict session parser and never mutate these process-wide factory fallbacks.
var builtinDefaults = func() (config struct {
	Version int  `json:"version"`
	Startup Pair `json:"startup"`
	Selection
}) {
	if json.Unmarshal([]byte(settingsfile.Defaults()), &config) != nil || config.Version != 1 {
		panic("invalid embedded Clauduct defaults")
	}
	return config
}()

// DefaultStartup stays independent from per-model and per-agent effort defaults.
func DefaultStartup() Pair { return builtinDefaults.Startup }
