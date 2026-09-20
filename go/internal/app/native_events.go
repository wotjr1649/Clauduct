package app

import (
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed native-events.mjs
var nativeEventModule string

// Per-session CLI plugin only; no user profile/plugin installation changes.
// Native permission/trust validation remains responsible for loading this code.
func prepareNativeEvents() (directory string, err error) {
	directory, err = os.MkdirTemp("", "clauduct-native-events-")
	if err != nil {
		return "", err
	}
	for _, name := range []string{".claude-plugin", "hooks", "receipts"} {
		if err = os.Mkdir(filepath.Join(directory, name), 0700); err != nil {
			return directory, err
		}
	}
	encoded, _ := json.Marshal(filepath.ToSlash(filepath.Join(directory, "receipts")))
	module := strings.Replace(nativeEventModule, "__CLAUDUCT_EVENT_ROOT__", string(encoded), 1)
	files := map[string]string{
		".claude-plugin/plugin.json": `{"name":"clauduct-native-events","version":"1.0.0","description":"Per-session native child identity and terminal receipts for Clauduct status","author":{"name":"Clauduct"}}`,
		"hooks/hooks.json":           `{"modules":["./events.mjs"]}`,
		"hooks/events.mjs":           module,
	}
	for name, body := range files {
		if err = os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
			return directory, err
		}
	}
	return directory, nil
}

// Native's optional PDF executable is provided only inside this child process's
// PATH. Existing system installations take precedence. No global tool is installed.
func prepareNativePDF(directory, hook string) (string, error) {
	bin := filepath.Join(directory, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		return "", err
	}
	alias := filepath.Join(bin, "pdftoppm.exe")
	if err := os.Link(hook, alias); err == nil {
		return bin, nil
	}
	in, err := os.Open(hook)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(alias, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return bin, nil
}
