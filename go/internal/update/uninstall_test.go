package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plant writes the three binaries and returns the directory holding them.
func plant(t *testing.T, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range append(append([]string{}, Binaries...), extra...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
	}
	return dir
}

func TestUninstallRequestedTakesTheFirstArgumentOnly(t *testing.T) {
	cases := []struct {
		args      []string
		wanted    bool
		consented bool
	}{
		{[]string{"--uninstall"}, true, false},
		{[]string{"--uninstall", "--yes"}, true, true},
		{[]string{"--UNINSTALL"}, true, false},
		// A prompt is an argument like any other. This is the case that decides whether the
		// option is safe to have at all.
		{[]string{"-p", "how do I --uninstall clauduct"}, false, false},
		{[]string{"--uninstall", "extra"}, false, false},
		{[]string{"--update"}, false, false},
		{nil, false, false},
	}
	for _, c := range cases {
		wanted, consented := UninstallRequested(c.args)
		if wanted != c.wanted || consented != c.consented {
			t.Fatalf("%q: got (%v,%v) want (%v,%v)", c.args, wanted, consented, c.wanted, c.consented)
		}
	}
}

func TestUninstallRemovesTheSetAndLeavesTheRestAlone(t *testing.T) {
	dir := plant(t, "clauduct-node.cmd", "claude.exe")
	if err := os.WriteFile(filepath.Join(dir, "clauduct.exe.old"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("plant leftover: %v", err)
	}
	self := filepath.Join(dir, "clauduct.exe")
	var out strings.Builder

	if code := UninstallIn(dir, self, "", []string{"--uninstall", "--yes"}, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	for _, name := range []string{"clauduct-hook.exe", "clauduct-dev.exe", "clauduct.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("%s survived\n%s", name, out.String())
		}
	}
	// The running executable cannot delete itself, so it is renamed and reported.
	if _, err := os.Stat(self + ".old"); err != nil {
		t.Fatalf("self was not moved aside: %v\n%s", err, out.String())
	}
	for _, name := range []string{"clauduct-node.cmd", "claude.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("removed %s, which is not ours: %v", name, err)
		}
	}
	if !strings.Contains(out.String(), "PATH: untouched") {
		t.Fatalf("did not say it left PATH alone:\n%s", out.String())
	}
}

func TestUninstallWithoutAnAnswerRemovesNothing(t *testing.T) {
	dir := plant(t)
	self := filepath.Join(dir, "clauduct.exe")
	var out strings.Builder

	// End of input, which is what an unattended run looks like. A removal nobody was there
	// to object to is not one anybody approved.
	if code := UninstallIn(dir, self, "", []string{"--uninstall"}, strings.NewReader(""), &out); code == 0 {
		t.Fatalf("proceeded without consent\n%s", out.String())
	}
	for _, name := range Binaries {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was removed without an answer: %v", name, err)
		}
	}
}

func TestUninstallNamesWhatItWillRemoveBeforeAsking(t *testing.T) {
	dir := plant(t)
	var out strings.Builder
	UninstallIn(dir, filepath.Join(dir, "clauduct.exe"), "", []string{"--uninstall"}, strings.NewReader("n\n"), &out)

	// Consent to an unnamed set is not consent. Every file has to be printed before the
	// question, and the things that are deliberately kept have to be printed too.
	text := out.String()
	for _, want := range append(append([]string{}, Binaries...), "clauduct-node", "CLAUDE_CONFIG_DIR", "remove them?") {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt did not mention %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "remove them?") < strings.Index(text, Binaries[1]) {
		t.Fatalf("asked before naming the files:\n%s", text)
	}
}

func TestUninstallKeepsEverythingWhenItCannotMoveItselfAside(t *testing.T) {
	dir := plant(t)
	// A directory where the executable should be: renaming onto it is refused, which stands
	// in for the executable being locked. Nothing else may be touched when that happens.
	self := filepath.Join(dir, "clauduct.exe")
	if err := os.Remove(self); err != nil {
		t.Fatalf("clear: %v", err)
	}
	// A non-empty directory: an empty one would simply be removed and the rename would
	// succeed, which would make this test agree with itself rather than measure anything.
	if err := os.Mkdir(self+".old", 0o700); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := os.WriteFile(filepath.Join(self+".old", "held"), []byte("x"), 0o600); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := os.WriteFile(self, []byte("clauduct.exe"), 0o600); err != nil {
		t.Fatalf("replant: %v", err)
	}
	var out strings.Builder

	code := UninstallIn(dir, self, "", []string{"--uninstall", "--yes"}, strings.NewReader(""), &out)
	if code == 0 {
		t.Fatalf("reported success while blocked\n%s", out.String())
	}
	if !strings.Contains(out.String(), "nothing was removed") {
		t.Fatalf("did not say it removed nothing:\n%s", out.String())
	}
	for _, name := range Binaries {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was removed although the set could not complete: %v\n%s", name, err, out.String())
		}
	}
}

func TestUninstallOnAnEmptyDirectorySaysSoAndStops(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	if code := UninstallIn(dir, filepath.Join(dir, "clauduct.exe"), "", []string{"--uninstall"}, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "nothing of this build") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestUninstallCountsDiagnosticsWithoutRemovingThem(t *testing.T) {
	dir := plant(t)
	status := t.TempDir()
	for _, name := range []string{"status-1.json", "status-2.json"} {
		if err := os.WriteFile(filepath.Join(status, name), []byte("{}"), 0o600); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
	}
	var out strings.Builder
	UninstallIn(dir, filepath.Join(dir, "clauduct.exe"), status, []string{"--uninstall", "--yes"}, strings.NewReader(""), &out)

	if !strings.Contains(out.String(), "2 session accounts") {
		t.Fatalf("did not report the diagnostics:\n%s", out.String())
	}
	// Reported, not swept. Deleting files by pattern out of the OS temp directory is a
	// worse trade than leaving them for the sweep that directory exists for.
	entries, err := os.ReadDir(status)
	if err != nil || len(entries) != 2 {
		t.Fatalf("diagnostics were touched: %v %d", err, len(entries))
	}
}
