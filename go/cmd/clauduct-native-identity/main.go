//go:build windows

// Command clauduct-native-identity is a verification-only helper, not shipped: the ps1 and mjs
// harnesses call it so the native identity logic exists once (providercert). It never locks,
// renames or modifies any native file.
//
//	clauduct-native-identity manifest <root>
//	clauduct-native-identity image <root> <pid> <created-unixnano> [<etw-generation-key>]
//
// <root> holds bin/claude.exe and share/claude/versions/ (normally ~/.local). It prints one
// JSON object on stdout. Exit 0 is a proven identity; 3 is INCONCLUSIVE_NATIVE_IDENTITY; 4 is
// INVALID_NATIVE_CHANGED; 2 is a usage error.
//
// image reads the process's mapped main module while the process is alive (fast, no hashing) and
// hashes that file afterwards, so a process that exits right after its image was read is still
// identified. Output schema of image (one JSON object, one line):
//
//	{
//	  "ok": true|false,
//	  "pid": <uint32>,                      echo of the argument
//	  "createdUnixNano": "<decimal>",       echo of the argument, as a string
//	  "etwKey": "<string>",                 echo of the optional 5th argument, "" when absent
//	  "result": { "pid": <uint32>, "created": "<decimal UnixNano>", "version": "<catalog version>",
//	              "identity": { "path", "volumeSerial": <uint32>, "fileIndex": "<decimal uint64>", "size": <int64>, "sha256" },
//	              "acquiredPath": "<path>",   // optional: only when the verified mapped name differs from the acquisition path
//	              "postCheck", "acquirer", "bindStartUnixNano", "bindEndUnixNano", "sectionHeldMs",
//	              "createdUnixNano": "<decimal>", "etwKey": "<string>" },     // present when ok
//	                                         // created and fileIndex are decimal STRINGS (they exceed 2^53)
//	  "error": "<message>",                 // present when not ok
//	  "errorKind": "process-exited" | "module-not-ready" | "hold-expired" | "changed" | "other"   // present when not ok
//	}
//
// "process-exited" and "module-not-ready" are failures to ACQUIRE the image (INCONCLUSIVE, exit 3):
// they say nothing about which image ran. "changed" (exit 4) is evidence of another image.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

// The seams of the image branch, replaced only by tests: the order they are called in is the
// contract (the mapped image is acquired while the process is alive, BEFORE the catalog of every
// version is hashed).
var (
	acquireImage = providercert.AcquireMappedImage
	nameVersion  = providercert.VersionOfImage
)

// imageResult is NativeImage plus the echoed join keys.
type imageResult struct {
	providercert.NativeImage
	CreatedUnixNano string `json:"createdUnixNano"`
	ETWKey          string `json:"etwKey"`
}

func run(args []string, stdout io.Writer) int {
	switch {
	case len(args) == 2 && args[0] == "manifest":
		m, _, err := providercert.NativeManifest(args[1])
		return report(stdout, map[string]any{"ok": err == nil, "result": m}, err)
	case (len(args) == 4 || len(args) == 5) && args[0] == "image":
		pid, err1 := strconv.ParseUint(args[2], 10, 32)
		created, err2 := strconv.ParseInt(args[3], 10, 64)
		if err1 != nil || err2 != nil {
			break
		}
		key := ""
		if len(args) == 5 {
			key = args[4]
		}
		out := map[string]any{"pid": uint32(pid), "createdUnixNano": args[3], "etwKey": key}
		// First the only step that needs the process alive (its mapped image, no hashing); only then the
		// catalog, which hashes every versions/ original and may outlast a short-lived process. The
		// catalog is still required: bytes it does not hold are INCONCLUSIVE.
		mapped, err := acquireImage(uint32(pid), created)
		if err != nil {
			out["ok"] = false
			return report(stdout, out, err)
		}
		// The hash is taken at once, while the hold lasts (it ends shortly after the process does); the
		// catalog is built afterwards and only names the version.
		hashed, err := mapped.Hashed()
		if err != nil {
			out["ok"] = false
			return report(stdout, out, err)
		}
		// Only the version files of the image's own size are hashed to name the version (a file of
		// another size cannot have its bytes): membership is the same, without hashing every version.
		img, err := nameVersion(filepath.Join(args[1], "share", "claude", "versions"), hashed)
		if err != nil && !errors.Is(err, providercert.ErrInconclusiveNativeIdentity) {
			err = fmt.Errorf("%w: versions: %v", providercert.ErrInconclusiveNativeIdentity, err)
		}
		out["ok"] = err == nil
		if err == nil {
			out["result"] = imageResult{NativeImage: img, CreatedUnixNano: args[3], ETWKey: key}
		}
		return report(stdout, out, err)
	}
	fmt.Fprintln(os.Stderr, "usage: clauduct-native-identity manifest <root> | image <root> <pid> <created-unixnano> [<etw-generation-key>]")
	return 2
}

// errorKind classifies a failure: acquisition failures are told apart from evidence of another image.
func errorKind(err error) string {
	switch {
	case errors.Is(err, providercert.ErrNativeChanged):
		return "changed"
	case errors.Is(err, providercert.ErrProcessExited):
		return "process-exited"
	case errors.Is(err, providercert.ErrModuleNotReady):
		return "module-not-ready"
	case errors.Is(err, providercert.ErrHoldExpired):
		// The image hold ran out before the hash was accepted: an acquisition failure, transient,
		// says nothing about which image ran. The consumer must treat it like process-exited.
		return "hold-expired"
	}
	return "other"
}

func report(stdout io.Writer, out map[string]any, err error) int {
	code := 0
	if err != nil {
		out["error"] = err.Error()
		out["errorKind"] = errorKind(err)
		code = 3
		if errors.Is(err, providercert.ErrNativeChanged) {
			code = 4
		}
	}
	json.NewEncoder(stdout).Encode(out)
	return code
}
