package upstream

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// ReferenceTools are the tool names native offered a Clauduct session when it was last
// re-measured, sorted (#297). The surface gate writes them; no client version is embedded
// (versions are recorded evidence, never a gate, #333). A session reports a name outside
// them rather than refusing it.
var ReferenceTools = measured.tools

// ReferenceCoreTools are, per mode ("print", "tui"), the tools native offered Clauduct's own
// launch when it was last re-measured: what a session of that mode should see unless it was
// narrowed (#300). Conditional tools are only in ReferenceTools.
var ReferenceCoreTools = measured.core

//go:embed measured-clients.json
var measuredDocument []byte

var measured = func() measuredClients {
	m, err := parseMeasured(measuredDocument)
	if err != nil {
		panic("invalid embedded measured clients")
	}
	return m
}()

type measuredClients struct {
	tools []string
	core  map[string][]string
}

var toolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

var errMeasured = errors.New("invalid measured clients")

// parseMeasured reads the measured tool surface: the sorted tool names and, per mode, the
// core names. The document is strict: the top-level keys are exactly "tools" and "core"
// (matched case-sensitively, so a client version or a case variant such as "TOOLS" is refused),
// a second document and an empty surface are refused, so a malformed file cannot quietly
// switch the surface comparison off.
func parseMeasured(document []byte) (m measuredClients, err error) {
	var top map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(document))
	if dec.Decode(&top) != nil || dec.Decode(&struct{}{}) != io.EOF { // only whitespace and EOF follow the first document
		return m, errMeasured
	}
	for key := range top {
		if key != "tools" && key != "core" {
			return m, errMeasured
		}
	}
	var doc struct {
		Tools []string
		Core  map[string][]string
	}
	if json.Unmarshal(top["tools"], &doc.Tools) != nil || json.Unmarshal(top["core"], &doc.Core) != nil {
		return m, errMeasured
	}
	if len(doc.Tools) == 0 || len(doc.Core["print"]) == 0 || len(doc.Core["tui"]) == 0 || !sortedNames(doc.Tools) {
		return m, errMeasured
	}
	known := map[string]bool{}
	for _, name := range doc.Tools {
		known[name] = true
	}
	for mode, names := range doc.Core {
		if mode != "print" && mode != "tui" || len(names) == 0 || !sortedNames(names) {
			return m, errMeasured
		}
		for _, name := range names {
			if !known[name] {
				return m, errMeasured // a core tool is a measured tool
			}
		}
	}
	m.tools, m.core = doc.Tools, doc.Core
	return m, nil
}

func sortedNames(names []string) bool {
	for i, name := range names {
		if !toolName.MatchString(name) || i > 0 && names[i-1] >= name {
			return false
		}
	}
	return true
}

var codexVersionLine = regexp.MustCompile(`^codex-cli (\S+)$`)

// Refusals this resolver produces.
var (
	// ErrCodexNotFound means the reference client is not installed. Its version is what
	// every request identifies itself as, so there is nothing to send without it.
	ErrCodexNotFound = errors.New("CODEX_NOT_FOUND")
	// ErrVersionInvalid means the installed client printed something this does not
	// recognise. A version goes into a request header, so an unrecognised one is refused
	// rather than forwarded.
	ErrVersionInvalid = errors.New("CLI_VERSION_INVALID")
)

// InstalledVersion resolves the Codex CLI's version once and remembers the answer.
//
// It is a function returning a function so that nothing runs until a request needs it. A
// session that only asked --version must not have spawned a subprocess, and a session that
// sends a hundred requests must not spawn a hundred.
func InstalledVersion() func() (string, error) {
	return InstalledVersionFunc(readInstalledVersion)
}

// InstalledVersionFunc wraps any resolver so it runs at most once.
//
// Separate from the resolver it usually wraps so that the laziness can be tested without a
// subprocess: a test counts the calls, which is the property, rather than watching for a
// process it cannot see.
func InstalledVersionFunc(resolve func() (string, error)) func() (string, error) {
	var once sync.Once
	var version string
	var err error
	return func() (string, error) {
		once.Do(func() { version, err = resolve() })
		return version, err
	}
}

func readInstalledVersion() (string, error) {
	// Through the same resolver that finds claude.exe. A version taken from a directory an
	// attacker can prepend to PATH would let them choose what this bridge claims to be.
	return versionFrom(platform.Resolver{}.Codex)
}

// versionFrom takes the resolver as an argument so the not-installed path can be tested on
// a machine where it is installed. That path is a packaging fact -- this binary needs the
// Codex CLI to send anything at all -- and a requirement nobody has exercised is a
// requirement nobody has checked.
func versionFrom(resolve func() (string, bool, error)) (string, error) {
	path, found, err := resolve()
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrCodexNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Output, not CombinedOutput: a warning on stderr must not become part of a version
	// string that goes on the wire.
	var raw bytes.Buffer
	cmd := exec.Command(path, "--version")
	cmd.Stdout = &raw
	err = childprocess.Run(ctx, cmd)
	if err != nil {
		return "", ErrNoClientVersion
	}
	return ParseVersion(raw.String())
}

// ParseVersion reads a version out of what the executable printed.
//
// Separate from running it so that what goes on the wire can be tested against output this
// machine does not produce. Whatever is printed, only a short printable token is accepted:
// this value becomes a request header.
func ParseVersion(raw string) (string, error) {
	match := codexVersionLine.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil || !plausibleVersion(match[1]) {
		return "", ErrVersionInvalid
	}
	return match[1], nil
}

// plausibleVersion bounds what may go into a header. A version is a short token with no
// whitespace; anything else is refused rather than sent.
func plausibleVersion(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}
