package app

import (
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
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
	files := map[string]string{
		".claude-plugin/plugin.json": `{"name":"clauduct-native-events","version":"1.0.0","description":"Per-session native child identity and terminal receipts for Clauduct status","author":{"name":"Clauduct"}}`,
		"hooks/hooks.json":           `{"modules":["./events.mjs"]}`,
		"hooks/events.mjs":           nativeEventSource(filepath.Join(directory, "receipts")),
	}
	for name, body := range files {
		if err = os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
			return directory, err
		}
	}
	return directory, nil
}

// nativeEventSource fills in the module. The receipt's model and effort labels come from
// the routing table: a model missing there is labelled unlisted, and the gateway cannot
// route a fork or a native selection from an unlisted receipt.
func nativeEventSource(receipts string) string {
	root, _ := json.Marshal(filepath.ToSlash(receipts))
	models := make([]string, 0, len(bridge.Models))
	for _, model := range bridge.Models {
		models = append(models, model.ID)
	}
	ids, _ := json.Marshal(models)
	efforts, _ := json.Marshal(bridge.Efforts)
	return strings.NewReplacer("__CLAUDUCT_EVENT_ROOT__", string(root), "__CLAUDUCT_MODELS__", string(ids),
		"__CLAUDUCT_EFFORTS__", string(efforts)).Replace(nativeEventModule)
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
