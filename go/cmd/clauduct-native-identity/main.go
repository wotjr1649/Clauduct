//go:build windows

// Command clauduct-native-identity is a verification-only helper, not shipped: the ps1 and mjs
// harnesses call it so the native identity logic exists once (providercert). It never locks,
// renames or modifies any native file.
//
//	clauduct-native-identity manifest <root>
//	clauduct-native-identity image <root> <pid> <created-unixnano>
//
// <root> holds bin/claude.exe and share/claude/versions/ (normally ~/.local). It prints one
// JSON object on stdout. Exit 0 is a proven identity; 3 is INCONCLUSIVE_NATIVE_IDENTITY; 4 is
// INVALID_NATIVE_CHANGED; 2 is a usage error.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	switch {
	case len(args) == 2 && args[0] == "manifest":
		m, _, err := providercert.NativeManifest(args[1])
		return report(m, err)
	case len(args) == 4 && args[0] == "image":
		pid, err1 := strconv.ParseUint(args[2], 10, 32)
		created, err2 := strconv.ParseInt(args[3], 10, 64)
		if err1 != nil || err2 != nil {
			break
		}
		catalog, _, err := providercert.BuildCatalog(filepath.Join(args[1], "share", "claude", "versions"))
		if err != nil {
			return report(nil, fmt.Errorf("%w: versions: %v", providercert.ErrInconclusiveNativeIdentity, err))
		}
		img, err := providercert.ImageIdentity(uint32(pid), created, catalog)
		return report(img, err)
	}
	fmt.Fprintln(os.Stderr, "usage: clauduct-native-identity manifest <root> | image <root> <pid> <created-unixnano>")
	return 2
}

func report(v any, err error) int {
	out := map[string]any{"ok": err == nil, "result": v}
	code := 0
	if err != nil {
		out["error"] = err.Error()
		code = 3
		if errors.Is(err, providercert.ErrNativeChanged) {
			code = 4
		}
	}
	json.NewEncoder(os.Stdout).Encode(out)
	return code
}
