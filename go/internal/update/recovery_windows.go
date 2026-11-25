package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const lockName = "clauduct.exe.update-lock"

var (
	errUpdateBusy          = errors.New("UPDATE_BUSY: wait for the update to finish and retry")
	errUpdateRecovery      = errors.New("UPDATE_RECOVERY_REQUIRED")
	createInstallationLock = syscall.CreateFile
)

// An exclusive, delete-on-close handle prevents concurrent mutations, including after a crash.
func lockInstallation(dir string) (*os.File, error) {
	path := filepath.Join(dir, lockName)
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	const deleteAccess, deleteOnClose = 0x10000, 0x04000000
	handle, err := createInstallationLock(name, deleteAccess, 0, nil, syscall.CREATE_NEW, deleteOnClose, 0)
	if err != nil {
		cause := &os.PathError{Op: "open", Path: path, Err: err}
		if errors.Is(err, syscall.ERROR_FILE_EXISTS) || errors.Is(err, syscall.ERROR_ALREADY_EXISTS) {
			// A pre-existing path can be a collision rather than a live exclusive handle.
			const shareAll = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
			const openEntry = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
			existing, openErr := syscall.CreateFile(name, syscall.GENERIC_READ, shareAll, nil, syscall.OPEN_EXISTING, openEntry, 0)
			if openErr == nil {
				syscall.CloseHandle(existing)
				return nil, recoveryError(fmt.Errorf("unrecognized update lock entry %s", path))
			}
			if !errors.Is(openErr, os.ErrNotExist) && !errors.Is(openErr, syscall.Errno(32)) {
				return nil, &os.PathError{Op: "open existing update lock", Path: path, Err: openErr}
			}
			return nil, fmt.Errorf("%w: %w", errUpdateBusy, cause)
		}
		if errors.Is(err, syscall.Errno(32)) { // ERROR_SHARING_VIOLATION
			return nil, fmt.Errorf("%w: %w", errUpdateBusy, cause)
		}
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			if info, statErr := os.Lstat(path); statErr == nil && (!info.Mode().IsRegular() || reparsePoint(info)) {
				return nil, recoveryError(fmt.Errorf("unrecognized update lock entry: %w", cause))
			}
		}
		return nil, fmt.Errorf("could not lock the installation: %w", cause)
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Recover finishes an interrupted executable swap before commands can use the installation.
func Recover(sweep bool) error {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return recoverIn(self, sweep, os.Stderr)
}

func recoverIn(self string, sweep bool, out io.Writer) error {
	if !strings.EqualFold(filepath.Base(self), Binaries[0]) {
		if sweep {
			sweepLeftoverOf(self)
		}
		return nil
	}
	dir := filepath.Dir(self)
	journal := filepath.Join(dir, journalName)
	cleanup := filepath.Join(dir, cleanupName)
	lockPath := filepath.Join(dir, lockName)
	if err := refuseUnmarkedStages(dir); err != nil {
		return err
	}
	backup := mayExist(self + ".old")
	recorded := mayExist(journal) || mayExist(cleanup)
	if !mayExist(lockPath) && !recorded && !backup {
		return nil
	}

	lock, err := lockInstallation(dir)
	if errors.Is(err, errUpdateBusy) {
		// An earlier launch may hold the lock just to sweep the previous image.
		if backup && !recorded && !mayExist(journal) && !mayExist(cleanup) {
			return nil
		}
		return err
	}
	if errors.Is(err, errUpdateRecovery) {
		return err
	}
	if err != nil {
		if mayExist(lockPath) || mayExist(journal) || mayExist(cleanup) {
			return recoveryError(err)
		}
		return nil // Only housekeeping remains.
	}
	defer lock.Close()

	if err := refuseUnmarkedStages(dir); err != nil {
		return err
	}
	pending, cleaning := mayExist(journal), mayExist(cleanup)
	switch {
	case pending && cleaning:
		return recoveryError(fmt.Errorf("both %s and %s exist", journal, cleanup))
	case cleaning:
		pair, err := readStagedPair(dir, cleanupName)
		if err != nil {
			return recoveryError(err)
		}
		switch err = pair.verify(dir); {
		case err == nil:
			err = pair.cleanup(dir)
		case errors.Is(err, os.ErrNotExist):
			err = removeFile(cleanup)
		}
		if err != nil {
			return recoveryError(err)
		}
	case pending:
		if err := recoverStagedPair(self); err != nil {
			return recoveryError(err)
		}
	}
	if pending || cleaning {
		for _, path := range removeRetiredProbes(dir) {
			fmt.Fprintln(out, "clauduct: leftover", path, "(close any process using it and retry --update)")
		}
	}
	if pending || cleaning || sweep {
		sweepLeftoverOf(self)
	}
	return nil
}

func recoverStagedPair(self string) error {
	dir := filepath.Dir(self)
	pair, err := readStagedPair(dir, journalName)
	if err != nil {
		return err
	}
	if err := pair.verify(dir); err != nil {
		if !errors.Is(err, os.ErrNotExist) || !binaryMatches(self, pair.Exe) {
			return err
		}
		return removeFile(filepath.Join(dir, journalName))
	}
	if mayExist(pair.stage(dir, Binaries[0])) {
		return pair.discard(dir) // The original executable is still installed.
	}
	if !binaryMatches(self, pair.Exe) {
		if mayExist(self + ".old") {
			return fmt.Errorf("%s does not match the update journal", self)
		}
		return pair.discard(dir) // The executable replacement did not finish.
	}

	return pair.discard(dir)
}

func recoveryError(err error) error {
	return fmt.Errorf("%w: %w; close running Clauduct processes and retry, or reinstall clauduct.exe in a clean directory", errUpdateRecovery, err)
}

func mayExist(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func sweepLeftoverOf(self string) { _ = removeFile(self + ".old") }

// DeleteFile removes a file name without os.Remove's read-only attribute fallback.
func removeFile(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	err = syscall.DeleteFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &os.PathError{Op: "delete", Path: path, Err: err}
	}
	return nil
}
