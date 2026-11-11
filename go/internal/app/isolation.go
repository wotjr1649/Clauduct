package app

import (
	"encoding/json"
	"fmt"
)

// Isolated mode (#379): Clauduct sessions run native on a dedicated config folder, so the stored
// Claude login and the shared supervisor stay out of reach of the requests native sends on its own.
const (
	isolationOff = "off"
	isolationOn  = "on"
)

var errIsolationSettings = fmt.Errorf(`%w: isolation must be "off" or "on". Edit ~/.clauduct/settings.json`, errClauductSettings)

// parseIsolation reads the settings value. Anything but the two names is refused, never defaulted.
func parseIsolation(raw json.RawMessage) (string, error) {
	var mode string
	if string(raw) == "null" || json.Unmarshal(raw, &mode) != nil || mode != isolationOff && mode != isolationOn {
		return "", errIsolationSettings
	}
	return mode, nil
}

// Isolated reports whether the settings turn isolated mode on.
func (config ClauductSettings) Isolated() bool { return config.Isolation == isolationOn }
