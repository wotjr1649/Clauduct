package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UninstallOption is the third and last option this launcher owns.
//
// It lives beside --update because the two manage the same three files and share the rule
// that makes either safe to recognise. The client defines no --uninstall of its own
// (measured on 2.1.274, where `update|upgrade` is a subcommand and there is no such
// option), so this name takes nothing over.
//
// Why the binary and not only scripts/uninstall.ps1: the script needs the repository. A
// machine that installed from a release has the binary and nothing else, and that is
// exactly where someone wants to remove it.
const UninstallOption = "--uninstall"

// UninstallRequested applies the same rule as Requested, for the same reason: first
// argument only, and nothing after it but --yes. A prompt is an argument like any other,
// and `clauduct -p "how do I --uninstall"` must not delete the installation.
func UninstallRequested(args []string) (wanted, consented bool) {
	if len(args) == 0 || !strings.EqualFold(args[0], UninstallOption) {
		return false, false
	}
	for _, arg := range args[1:] {
		if !strings.EqualFold(arg, Consent) {
			return false, false
		}
		consented = true
	}
	return true, consented
}

// Uninstall removes the installation this executable belongs to.
func Uninstall(args []string, statusDir string, in io.Reader, out io.Writer) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(out, "clauduct: UNINSTALL_SELF_UNKNOWN")
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return UninstallIn(filepath.Dir(self), self, statusDir, args, in, out)
}

// UninstallIn is Uninstall with the directory, this executable and the diagnostics
// directory supplied, so a test can drive the whole path without removing itself.
//
// The diagnostics directory is passed in rather than computed here because the package
// that writes those files owns where they live; this one only has to name the place.
func UninstallIn(dir, self, statusDir string, args []string, in io.Reader, out io.Writer) int {
	_, consented := UninstallRequested(args)

	present, leftovers := survey(dir)
	if len(present) == 0 && len(leftovers) == 0 {
		fmt.Fprintf(out, "nothing of this build is in %s\n", dir)
		return 0
	}

	fmt.Fprintf(out, "remove from %s\n", dir)
	for _, name := range append(append([]string{}, present...), leftovers...) {
		fmt.Fprintf(out, "  %s\n", name)
	}
	fmt.Fprintln(out, "keep")
	fmt.Fprintln(out, "  clauduct-node.cmd and its version store -- a different product, and the way back")
	fmt.Fprintln(out, "  everything under CLAUDE_CONFIG_DIR -- session state, which this build never wrote")
	if statusDir != "" {
		if n := diagnostics(statusDir); n > 0 {
			fmt.Fprintf(out, "  %d session accounts in %s -- diagnostics the OS clears\n", n, statusDir)
		}
	}

	if !consented && !confirmedRemoval(in, out) {
		fmt.Fprintln(out, "nothing was removed")
		return 1
	}

	if err := remove(dir, self, present, leftovers); err != nil {
		fmt.Fprintf(out, "clauduct: UNINSTALL_FAILED %v\n", err)
		fmt.Fprintln(out, "nothing was removed")
		return 1
	}

	fmt.Fprintln(out, "removed")
	if renamed := filepath.Base(self) + ".old"; sameDir(dir, self) {
		fmt.Fprintf(out, "one file is left because this process is still running it: %s\n",
			filepath.Join(dir, renamed))
		// %s inside quotes rather than %q: Go quotes a Windows path by escaping every
		// separator, and the one command this prints is one the user has to be able to paste.
		fmt.Fprintf(out, "  del \"%s\"\n", filepath.Join(dir, renamed))
	}
	reportPath(dir, out)
	return 0
}

// survey reports which of this build's files are in the directory: the binaries, and the
// predecessors an earlier --update could not delete while it was running them.
func survey(dir string) (present, leftovers []string) {
	for _, name := range Binaries {
		if isFile(filepath.Join(dir, name)) {
			present = append(present, name)
		}
		if old := name + ".old"; isFile(filepath.Join(dir, old)) {
			leftovers = append(leftovers, old)
		}
	}
	return present, leftovers
}

// remove takes the running executable out of the way first.
//
// The order is the safety, the same way it is in Apply. A directory holding clauduct.exe
// without clauduct-hook.exe beside it is not a partial uninstall, it is a working install
// with role routing silently dead -- findHook looks only next to the executable, and a
// session that cannot find it starts anyway and reports hookInstalled false. So if this
// executable cannot be moved aside, nothing else is touched either.
func remove(dir, self string, present, leftovers []string) error {
	renamed := ""
	if sameDir(dir, self) {
		renamed = self + ".old"
		_ = os.Remove(renamed)
		if err := os.Rename(self, renamed); err != nil {
			return err
		}
	}
	restore := func() {
		if renamed != "" {
			_ = os.Rename(renamed, self)
		}
	}
	for _, name := range present {
		path := filepath.Join(dir, name)
		if path == self {
			continue
		}
		if err := os.Remove(path); err != nil {
			restore()
			return err
		}
	}
	for _, name := range leftovers {
		path := filepath.Join(dir, name)
		if path == renamed {
			continue
		}
		if err := os.Remove(path); err != nil {
			restore()
			return err
		}
	}
	return nil
}

// reportPath says what the directory still holds and stops there.
//
// Editing the user's Path means a raw registry read that preserves the value kind, because
// the ordinary read expands %USERPROFILE% and the ordinary write stores the result as
// REG_SZ, after which no remaining %VAR% entry expands again. Doing that safely needs a
// dependency this module does not have, and this is the one place in the install path that
// can destroy something the user did not ask about. On a default install the entry is
// shared anyway -- claude.exe lives in the same directory -- so the answer is almost always
// to leave it. When it is not, the command is one line and it is printed.
func reportPath(dir string, out io.Writer) {
	others := executables(dir)
	if len(others) > 0 {
		fmt.Fprintf(out, "PATH: untouched. %s still holds %d executable(s), starting with %s\n",
			dir, len(others), others[0])
		return
	}
	fmt.Fprintf(out, "PATH: untouched, and %s now holds no executables.\n", dir)
	fmt.Fprintln(out, "  to drop the entry too: scripts/uninstall.ps1 -RemovePath, from a checkout")
}

func executables(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".exe", ".cmd", ".bat", ".ps1", ".com":
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out
}

func diagnostics(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "status-") {
			n++
		}
	}
	return n
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func sameDir(dir, path string) bool {
	return strings.EqualFold(filepath.Clean(filepath.Dir(path)), filepath.Clean(dir))
}

// confirmedRemoval asks once, and reads end of input as a no for the same reason confirmed
// does: a removal nobody was there to object to is not one anybody approved.
func confirmedRemoval(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "remove them? [y/N] ")
	return yes(in, out)
}
