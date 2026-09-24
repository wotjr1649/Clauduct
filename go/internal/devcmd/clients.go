package devcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
)

var claudeVersionLine = regexp.MustCompile(`^(\d+\.\d+\.\d+\S*) \(Claude Code\)$`)

// claudeVersion asks the resolved native for its version. Like codex --version it only
// prints: nothing starts that could send a request.
func claudeVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var raw bytes.Buffer
	cmd := exec.Command(path, "--version")
	cmd.Stdout = &raw
	if err := childprocess.Run(ctx, cmd); err != nil {
		return "", err
	}
	match := claudeVersionLine.FindStringSubmatch(strings.TrimSpace(raw.String()))
	if match == nil {
		return "", errors.New("unrecognised version output")
	}
	return match[1], nil
}

// versionReport compares an installed client with the version Clauduct was last measured
// against. A difference is not a failure -- both clients update on their own and sessions
// keep running -- it says a re-measure is due (#121, #127).
func versionReport(out io.Writer, name, installed string, err error, measured string) {
	label := fmt.Sprintf("%-12s ", name)
	switch {
	case err != nil:
		fmt.Fprintf(out, "%sversion unavailable, measured %s\n", label, measured)
	case installed == measured:
		fmt.Fprintf(out, "%s%s, the measured version\n", label, installed)
	default:
		fmt.Fprintf(out, "%s%s, measured %s: re-measure due; sessions still run\n", label, installed, measured)
	}
}

// credentialReport says whether the Codex login a session would use is usable. It is read
// here and nowhere else goes: the category or the expiry is all that is printed.
func credentialReport(out io.Writer, credential auth.Credential, err error) {
	if err != nil {
		category := auth.CategoryOf(err)
		if category == "" {
			category = "unreadable"
		}
		fmt.Fprintf(out, "credentials  unusable: %s\n", category)
		return
	}
	fmt.Fprintf(out, "credentials  usable, expires %s (read locally, not sent)\n",
		credential.ExpiresAt.UTC().Format(time.RFC3339))
}
