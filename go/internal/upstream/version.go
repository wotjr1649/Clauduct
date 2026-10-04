package upstream

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// ReferenceClientVersion is the Codex CLI version last re-measured against this bridge's wire.
// It is product data (measured-clients.json, written by the re-measure's --accept), not
// code; the installed version is always read from the machine at run time.
//
// Evidence, not a pin. A different installed version still runs and is reported as
// unverified, because refusing to start on a version nobody has checked would be stricter
// than the evidence supports and would break on every Codex release. A re-measure records
// how the installed client's requests differ from this bridge's; it does not copy them, so
// matching this version says the difference is known, not that there is none (#121).
var ReferenceClientVersion = measured.codex

// ReferenceClaudeVersion is the Claude Code version last re-measured, from the same file.
var ReferenceClaudeVersion = measured.claude

// ReferenceTools are the tool names native offered a Clauduct session when it was last
// re-measured, sorted (#297). The surface gate writes them with the versions; a session
// reports a name outside them rather than refusing it. Empty when none were recorded.
var ReferenceTools = measured.tools

// ReferenceCoreTools are, per mode ("print", "tui"), the tools native offered Clauduct's own
// launch when it was last re-measured: what a session of that mode should see unless it was
// narrowed (#300). Conditional tools are only in ReferenceTools.
var ReferenceCoreTools = measured.core

//go:embed measured-clients.json
var measuredDocument []byte

var measuredVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var measured = func() measuredClients {
	m, err := parseMeasured(measuredDocument)
	if err != nil {
		panic("invalid embedded measured clients")
	}
	return m
}()

type measuredClients struct {
	claude, codex string
	tools         []string
	core          map[string][]string
}

var toolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

var errMeasured = errors.New("invalid measured clients")

// parseMeasured reads the re-measure's own versions.json shape, so --accept writes one file.
func parseMeasured(document []byte) (m measuredClients, err error) {
	var doc struct {
		Claude string              `json:"claude"`
		Codex  string              `json:"codex"`
		Tools  []string            `json:"tools"`
		Core   map[string][]string `json:"core"`
	}
	if json.Unmarshal(document, &doc) != nil {
		return m, errMeasured
	}
	claude, okClaude := strings.CutSuffix(doc.Claude, " (Claude Code)")
	codex, okCodex := strings.CutPrefix(doc.Codex, "codex-cli ")
	if !okClaude || !okCodex || !measuredVersion.MatchString(claude) || !measuredVersion.MatchString(codex) || !sortedNames(doc.Tools) {
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
	m.claude, m.codex, m.tools, m.core = claude, codex, doc.Tools, doc.Core
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

// Status reports whether a version is the one the wire was measured against. It is printed,
// never sent: the header carries the installed version whatever this says.
func Status(version string) string {
	if version == ReferenceClientVersion {
		return "reference"
	}
	return "unverified"
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
