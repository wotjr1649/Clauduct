package upstream

import (
	"errors"
	"testing"
)

// REL03, and a packaging fact G7 introduced: this binary needs the Codex CLI installed to
// send anything at all. Its version goes into a header on every request, so there is
// nothing to send without it.
//
// Tested through an injected resolver because the machine that runs this has Codex
// installed. A requirement nobody has exercised is a requirement nobody has checked.
func TestWithoutTheCodexCLINothingCanBeSent(t *testing.T) {
	notInstalled := func() (string, bool, error) {
		// The resolver reports where it looked even when it found nothing, which is what a
		// user needs in order to fix it.
		return `C:\Users\someone\.local\bin\codex.exe`, false, nil
	}
	got, err := versionFrom(notInstalled)
	if !errors.Is(err, ErrCodexNotFound) {
		t.Fatalf("versionFrom = %q, %v; want %v", got, err, ErrCodexNotFound)
	}
	if got != "" {
		t.Fatalf("a failed resolution still named a version: %q", got)
	}
}

// A resolver that cannot answer at all is different from one that answered "not here", and
// the difference reaches the caller rather than being flattened.
func TestAResolverFailureIsNotReportedAsNotInstalled(t *testing.T) {
	boom := errors.New("HOME_UNAVAILABLE")
	_, err := versionFrom(func() (string, bool, error) { return "", false, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the resolver's own failure", err)
	}
	if errors.Is(err, ErrCodexNotFound) {
		t.Fatal("a resolver failure was reported as the CLI not being installed")
	}
}
