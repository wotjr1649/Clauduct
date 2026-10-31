package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UninstallOption is the third and last option this launcher owns.
//
// It lives beside --update because the two manage the same files and share the rule
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
	return UninstallIn(filepath.Dir(self), self, statusDir, os.TempDir(), args, in, out)
}

// UninstallIn is Uninstall with the directory, this executable and the diagnostics
// directory supplied, so a test can drive the whole path without removing itself.
//
// The diagnostics directory is passed in rather than computed here because the package
// that writes those files owns where they live; this one only has to name the place.
func UninstallIn(dir, self, statusDir, parkDir string, args []string, in io.Reader, out io.Writer) int {
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

	parked, remaining, err := remove(dir, self, parkDir, present, leftovers)
	switch {
	case errors.Is(err, errUninstallIncomplete):
		// Everything was moved aside, so the installation no longer runs; these copies are left.
		fmt.Fprintf(out, "clauduct: UNINSTALL_INCOMPLETE %v\n", err)
		for _, path := range remaining {
			fmt.Fprintf(out, "  del \"%s\"\n", path)
		}
		fmt.Fprintln(out, "  (running clauduct --uninstall again also removes them)")
	case errors.Is(err, errUninstallNotRestored):
		// The uninstall stopped, and putting a moved file back failed too: say exactly where.
		fmt.Fprintf(out, "clauduct: UNINSTALL_FAILED %v\n", err)
		for _, path := range remaining {
			fmt.Fprintf(out, "  left aside: %s\n", path)
		}
		fmt.Fprintln(out, "  rename each back without .uninstalling, or run clauduct --uninstall again")
	case err != nil:
		fmt.Fprintf(out, "clauduct: UNINSTALL_FAILED %v\n", err)
		fmt.Fprintln(out, "nothing was removed")
	}
	if err != nil {
		if errors.Is(err, errUninstallIncomplete) && parked != "" && sameDir(dir, parked) {
			fmt.Fprintf(out, "  del \"%s\"\n", parked)
		}
		return 1
	}

	fmt.Fprintln(out, "removed")
	switch {
	case parked == "":
	case sameDir(dir, parked):
		// The same volume was not available, so the image this process is running is still
		// in the installation. Name the one command that finishes it.
		fmt.Fprintf(out, "one file is left because this process is still running it: %s\n", parked)
		// %s inside quotes rather than %q: Go quotes a Windows path by escaping every
		// separator, and the one command this prints is one the user has to be able to paste.
		fmt.Fprintf(out, "  del \"%s\"\n", parked)
	default:
		fmt.Fprintf(out, "still running its own image, so it was moved to %s\n", parked)
		fmt.Fprintln(out, "  nothing to do -- that is the directory the OS clears")
	}
	reportPath(dir, out)
	return 0
}

// survey reports which of this build's files are in the directory: the binaries, the
// copies a 0.3.x updater left under the retired names, and the predecessors an earlier
// --update could not delete while it was running them.
func survey(dir string) (present, leftovers []string) {
	for _, name := range append(append([]string{Probe}, Binaries...), Retired...) {
		if isFile(filepath.Join(dir, name)) {
			present = append(present, name)
		}
		if old := name + ".old"; isFile(filepath.Join(dir, old)) {
			leftovers = append(leftovers, old)
		}
		// A copy an earlier uninstall staged and could not delete.
		for _, staged := range []string{name + ".uninstalling", name + ".old.uninstalling"} {
			if isFile(filepath.Join(dir, staged)) {
				leftovers = append(leftovers, staged)
			}
		}
	}
	return present, leftovers
}

// remove takes the running executable out of the way first.
//
// The order is the safety, the same way it is in Apply. If this executable cannot be moved
// aside, nothing else is touched either: a half-removed installation is the one state
// nobody would think to look for.
func remove(dir, self, parkDir string, present, leftovers []string) (parked string, remaining []string, err error) {
	moved, movedOld := "", ""
	if sameDir(dir, self) {
		moved, movedOld = park(self, parkDir)
		if moved == "" {
			return "", nil, errCannotMoveSelf
		}
	}
	// Every other file is renamed aside before any is deleted. A file something holds open
	// cannot be renamed, so a failure shows up while the installation can still be put back
	// exactly as it was -- the probe included. Only after every rename has succeeded are the
	// staged copies deleted, and by then nothing is left that could refuse.
	type staged struct{ path, aside string }
	var done []staged
	var prestaged []string
	if movedOld != "" {
		// park moved an earlier update's clauduct.exe.old aside to take its name.
		done = append(done, staged{self + ".old", movedOld})
	}
	// restore puts everything back and returns what it could not.
	restore := func() (stuck []string) {
		// The executable first: when it was parked as clauduct.exe.old, that name is the one the
		// displaced earlier .old goes back to.
		if moved != "" && os.Rename(moved, self) != nil {
			stuck = append(stuck, moved)
		}
		for i := len(done) - 1; i >= 0; i-- {
			if os.Rename(done[i].aside, done[i].path) != nil {
				stuck = append(stuck, done[i].aside)
			}
		}
		return stuck
	}
	stage := func(name string) error {
		path := filepath.Join(dir, name)
		if path == self || path == moved || path == movedOld {
			return nil
		}
		if strings.HasSuffix(name, ".uninstalling") {
			// Already staged by an earlier uninstall: deleted with the rest, never put back.
			prestaged = append(prestaged, path)
			return nil
		}
		aside := path + ".uninstalling"
		_ = os.Remove(aside)
		if err := os.Rename(path, aside); err != nil {
			return err
		}
		done = append(done, staged{path, aside})
		return nil
	}
	for _, name := range append(append([]string{}, present...), leftovers...) {
		if err := stage(name); err != nil {
			if stuck := restore(); len(stuck) > 0 {
				return "", stuck, fmt.Errorf("%w: %v", errUninstallNotRestored, err)
			}
			return "", nil, err
		}
	}
	// Every file is aside; delete them all, and name each one that would not go.
	for _, s := range done {
		prestaged = append(prestaged, s.aside)
	}
	for _, path := range prestaged {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			remaining = append(remaining, path)
		}
	}
	if len(remaining) > 0 {
		return moved, remaining, fmt.Errorf("%w (%d file(s) could not be deleted)", errUninstallIncomplete, len(remaining))
	}
	return moved, nil, nil
}

// errUninstallIncomplete is a failure after every file was moved aside: the installation no
// longer runs, and the staged copies listed are what is left to delete.
var errUninstallIncomplete = errors.New("the installation was removed, but staged copies remain")

// errUninstallNotRestored is a stopped uninstall whose put-back also failed for some files: the
// installation is not as it was, and the files listed are where they were left.
var errUninstallNotRestored = errors.New("the uninstall stopped and some files could not be put back")

// errCannotMoveSelf means the running executable could not be moved out of the way, which is
// the one failure that stops an uninstall before it touches anything else.
var errCannotMoveSelf = errors.New("the running executable could not be moved aside")

// park moves the running executable out of the installation.
//
// Windows will not let a process delete the image it is running, but it will let that file be
// renamed -- including into another directory, as long as it is the same volume, because that
// is one NTFS rename and not a copy. Measured 2026-09-17: the source directory is left empty
// and the process carries on running.
//
// So the leftover goes to the temporary directory rather than sitting in the installation.
// Uninstall is the one case the sweep on the next launch cannot reach, because after an
// uninstall there is no next launch; parking it somewhere the OS already clears is what makes
// the install directory actually empty.
//
// The fallback is the old behaviour, for an installation on a different volume from TEMP,
// where the rename would be a copy and fails. Then the name beside the executable is used and
// the caller prints the one command that finishes the job.
//
// An earlier update's clauduct.exe.old in the way is moved aside, not deleted, and named as
// movedOld so a stopped uninstall can put it back too.
func park(self, parkDir string) (moved, movedOld string) {
	if parkDir != "" {
		target := filepath.Join(parkDir,
			fmt.Sprintf("clauduct-removed-%d%s", os.Getpid(), filepath.Ext(self)))
		_ = os.Remove(target)
		if os.Rename(self, target) == nil {
			return target, ""
		}
	}
	beside := self + ".old"
	if isFile(beside) {
		aside := beside + ".uninstalling"
		_ = os.Remove(aside)
		if os.Rename(beside, aside) != nil {
			return "", ""
		}
		movedOld = aside
	}
	if os.Rename(self, beside) == nil {
		return beside, movedOld
	}
	if movedOld != "" {
		_ = os.Rename(movedOld, beside)
	}
	return "", ""
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

// SweepLeftover removes the predecessor an earlier update left beside this executable.
//
// Apply renames each binary to <name>.old before writing the new one and then deletes the
// backups, but the one it cannot delete is its own: Windows holds a running image open.
// That leftover is reported and then nobody ever removes it, so it sits in the install
// directory looking like a fault produced by an update that worked.
//
// The next launch is the first process that is not running it. Only this executable's own
// predecessor, only beside this executable, and every failure is ignored -- a file that
// cannot be removed now will be offered again on the next start, and a session must not
// fail over housekeeping.
func SweepLeftover() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	sweepLeftoverOf(self)
}

// sweepLeftoverOf is SweepLeftover with the executable supplied, because os.Executable in a
// test is the test binary and a test that cannot name the file it is about measures nothing.
func sweepLeftoverOf(self string) { _ = os.Remove(self + ".old") }
