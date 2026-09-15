package upstream

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// ReferenceClientVersion is the version this project measured the wire against.
//
// Evidence, not a pin. A different installed version still runs and is reported as
// unverified, because refusing to start on a version nobody has checked would be stricter
// than the evidence supports and would break on every Codex release.
const ReferenceClientVersion = "0.153.4"

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
	path, found, err := platform.Resolver{}.Codex()
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
	raw, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", ErrNoClientVersion
	}
	return ParseVersion(string(raw))
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
