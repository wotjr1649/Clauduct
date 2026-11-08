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

// versionReport prints an installed client's version. It is recorded evidence: both clients
// update on their own, sessions keep running, and what a session compares is the tool names
// it was offered, not the version (#333).
func versionReport(out io.Writer, name, installed string, err error) {
	label := fmt.Sprintf("%-12s ", name)
	if err != nil {
		fmt.Fprintf(out, "%sversion unavailable\n", label)
		return
	}
	fmt.Fprintf(out, "%s%s (recorded; sessions compare tool names, not the version)\n", label, installed)
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
