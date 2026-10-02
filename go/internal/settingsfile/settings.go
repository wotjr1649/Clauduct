// Package settingsfile creates the manual Clauduct preferences and, through Sync, appends
// top-level keys a newer release added without changing any value the user's file holds.
// The same document supplies omitted runtime preferences.
package settingsfile

import (
	"crypto/rand"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
)

var ErrCreate = errors.New("CLAUDUCT_SETTINGS_CREATE_FAILED")

//go:embed defaults.json
var defaults string

// Defaults is the complete factory document, shared by initialization and routing.
func Defaults() string { return defaults }

func Ensure(home string) error {
	if !filepath.IsAbs(home) {
		return ErrCreate
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return ErrCreate
	}
	defer root.Close()
	const target = ".clauduct/settings.json"
	if info, err := root.Lstat(target); err == nil {
		if info.IsDir() {
			return ErrCreate
		}
		return nil // Preserve even malformed or future-version preferences.
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrCreate
	}
	if err := root.MkdirAll(".clauduct", 0700); err != nil {
		return ErrCreate
	}
	temp := ".clauduct/settings-" + rand.Text() + ".tmp"
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrCreate
	}
	defer root.Remove(temp)
	_, writeErr := file.WriteString(defaults)
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrCreate
	}
	// Link publishes a complete file and cannot replace an existing destination.
	// Root contains reparse-point resolution within the selected home.
	if err := root.Link(temp, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return ErrCreate
		}
		if info, err := root.Lstat(target); err != nil || info.IsDir() {
			return ErrCreate
		}
	}
	return nil
}
