package app_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The install and uninstall scripts are the only install path a user follows by hand, and the
// parts worth testing are the ones that refuse: a digest that does not match, and a directory
// that would shadow the command's name. Both are tested here against synthetic files, so no
// release is downloaded and nothing touches the real install root or the user's PATH.
//
// What this does not cover: the download itself and the PATH write. The first needs the
// network and the second edits the user's registry, which is the one thing a test must not do
// on the machine it runs on. Both are stated as unverified in PACKAGING rather than asserted.

func scriptPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", name))
	if err != nil {
		t.Fatalf("resolve %s: %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
	return path
}

func runScript(t *testing.T, script string, args ...string) (string, error) {
	t.Helper()
	shell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skipf("powershell not on PATH: %v", err)
	}
	full := append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script}, args...)
	out, err := exec.Command(shell, full...).CombinedOutput()
	return string(out), err
}

// stageRelease writes the three names plus a SHA256SUMS that agrees with them.
func stageRelease(t *testing.T) (dir string, bodies map[string][]byte) {
	t.Helper()
	dir = t.TempDir()
	bodies = map[string][]byte{}
	var sums bytes.Buffer
	for _, name := range []string{"clauduct.exe", "clauduct-hook.exe", "clauduct-dev.exe"} {
		body := []byte("synthetic " + name + "\n")
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		bodies[name] = body
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(body), name)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), sums.Bytes(), 0o600); err != nil {
		t.Fatalf("write sums: %v", err)
	}
	return dir, bodies
}

func TestInstallScriptPlacesTheThreeItVerified(t *testing.T) {
	source, bodies := stageRelease(t)
	root := t.TempDir()
	keep := filepath.Join(root, "keepme.exe")
	if err := os.WriteFile(keep, []byte("someone else's tool"), 0o600); err != nil {
		t.Fatalf("plant: %v", err)
	}

	out, err := runScript(t, scriptPath(t, "install.ps1"),
		"-FromPath", source, "-InstallRoot", root, "-NoPathUpdate")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	for name, want := range bodies {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s not installed: %v\n%s", name, err, out)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from the verified source", name)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("installer disturbed an unrelated file: %v", err)
	}
	if !strings.Contains(out, "PATH: untouched") {
		t.Fatalf("-NoPathUpdate did not say it left PATH alone:\n%s", out)
	}
}

func TestInstallScriptRefusesATamperedFileAndCopiesNothing(t *testing.T) {
	source, _ := stageRelease(t)
	tampered := filepath.Join(source, "clauduct-hook.exe")
	if err := os.WriteFile(tampered, []byte("not what the digest says\n"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	root := t.TempDir()

	out, err := runScript(t, scriptPath(t, "install.ps1"),
		"-FromPath", source, "-InstallRoot", root, "-NoPathUpdate")
	if err == nil {
		t.Fatalf("installed a file whose digest did not match:\n%s", out)
	}
	if !strings.Contains(out, "INSTALL_DIGEST_MISMATCH") {
		t.Fatalf("refusal did not name the mismatch:\n%s", out)
	}
	// The one that matched must not be there either. Two new binaries beside one old one is
	// the state no release was ever tested as.
	for _, name := range []string{"clauduct.exe", "clauduct-hook.exe", "clauduct-dev.exe"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Fatalf("%s was copied before the set finished verifying", name)
		}
	}
}

func TestInstallScriptRefusesADirectoryShadowingTheCommand(t *testing.T) {
	source, _ := stageRelease(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "clauduct"), 0o700); err != nil {
		t.Fatalf("shadow: %v", err)
	}

	out, err := runScript(t, scriptPath(t, "install.ps1"),
		"-FromPath", source, "-InstallRoot", root, "-NoPathUpdate")
	if err == nil {
		t.Fatalf("installed into a root where Git Bash cannot reach the exe:\n%s", out)
	}
	if !strings.Contains(out, "INSTALL_DIRECTORY_SHADOW") {
		t.Fatalf("refusal did not name the shadowing directory:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "clauduct.exe")); err == nil {
		t.Fatalf("copied anyway")
	}
}

func TestUninstallScriptRemovesOnlyItsOwn(t *testing.T) {
	source, _ := stageRelease(t)
	root := t.TempDir()
	if out, err := runScript(t, scriptPath(t, "install.ps1"),
		"-FromPath", source, "-InstallRoot", root, "-NoPathUpdate"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	// What must survive: v1's launcher, its version store, and anything else that shares the
	// directory. Plus a leftover the updater renamed aside, which must not.
	keepers := []string{"clauduct-node.cmd", "claude.exe"}
	for _, name := range keepers {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatalf("plant %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "clauduct-node-store"), 0o700); err != nil {
		t.Fatalf("plant store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "clauduct.exe.old"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("plant old: %v", err)
	}

	out, err := runScript(t, scriptPath(t, "uninstall.ps1"), "-InstallRoot", root)
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	for _, name := range []string{"clauduct.exe", "clauduct-hook.exe", "clauduct-dev.exe", "clauduct.exe.old"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Fatalf("%s survived uninstall:\n%s", name, out)
		}
	}
	for _, name := range append(keepers, "clauduct-node-store") {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("uninstall removed %s, which is not ours: %v\n%s", name, err, out)
		}
	}
	if !strings.Contains(out, "PATH: untouched") {
		t.Fatalf("uninstall did not say it left PATH alone:\n%s", out)
	}
}
