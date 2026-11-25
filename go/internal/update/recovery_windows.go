package update

import (
	"errors"
	"fmt"
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

// Recover finishes an interrupted pair swap before commands can use the installation.
func Recover(expected string, sweep bool) error {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return recoverIn(self, expected, sweep)
}

func recoverIn(self, expected string, sweep bool) error {
	if !strings.EqualFold(filepath.Base(self), Binaries[0]) {
		if sweep {
			sweepLeftoverOf(self)
		}
		return nil
	}
	dir := filepath.Dir(self)
	probe := filepath.Join(dir, Probe)
	journal := filepath.Join(dir, journalName)
	cleanup := filepath.Join(dir, cleanupName)
	lockPath := filepath.Join(dir, lockName)
	if err := refuseUnmarkedStages(dir); err != nil {
		return err
	}
	backup := mayExist(self + ".old")
	if !mayExist(lockPath) && !mayExist(journal) && !mayExist(cleanup) && !backup {
		return nil
	}

	lock, err := lockInstallation(dir)
	if errors.Is(err, errUpdateBusy) || errors.Is(err, errUpdateRecovery) {
		return err
	}
	if err != nil {
		if mayExist(lockPath) || mayExist(journal) || mayExist(cleanup) || probeMismatch(probe, expected) {
			return recoveryError(err)
		}
		return nil // There is only housekeeping and no evidence of a mismatched pair.
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
		if err == nil {
			err = pair.cleanup(dir)
		}
		if err != nil {
			return recoveryError(err)
		}
	case pending:
		if err := recoverStagedPair(self, probe, expected); err != nil {
			return recoveryError(err)
		}
	case probeMismatch(probe, expected):
		return recoveryError(fmt.Errorf("%s is missing or does not match the probe SHA-256 embedded in this executable", probe))
	}
	if pending || cleaning || sweep {
		sweepLeftoverOf(self)
	}
	return nil
}

func recoverStagedPair(self, probe, expected string) error {
	dir := filepath.Dir(self)
	pair, err := readStagedPair(dir, journalName)
	if err != nil {
		return err
	}
	if mayExist(pair.stage(dir, Binaries[0])) {
		return pair.discard(dir) // The original pair is still installed.
	}
	if !binaryMatches(self, pair.Exe) {
		if mayExist(self+".old") || probeMismatch(probe, expected) {
			return fmt.Errorf("%s does not match the update journal", self)
		}
		return pair.discard(dir) // The rollback restored the original pair.
	}

	if pair.Probe != expected {
		return fmt.Errorf("staged probe SHA-256 does not match this executable")
	}
	if pair.Probe == "" {
		if err := removeFile(probe); err != nil {
			return err
		}
	} else {
		staged := pair.stage(dir, Probe)
		if mayExist(staged) {
			// Apply may already have made this backup before it was interrupted.
			if sameFile(probe, probe+".old") {
				err = renameFile(staged, probe)
			} else {
				err = replaceStaged(staged, probe, true)
			}
			if err != nil {
				return err
			}
		}
		if !binaryMatches(probe, expected) {
			return fmt.Errorf("%s is missing or does not match the probe SHA-256 embedded in this executable", probe)
		}
		_ = removeFile(probe + ".old")
	}
	return pair.discard(dir)
}

func sameFile(a, b string) bool {
	first, firstErr := os.Lstat(a)
	second, secondErr := os.Lstat(b)
	return firstErr == nil && secondErr == nil && os.SameFile(first, second)
}

func probeMismatch(probe, expected string) bool {
	return expected != "" && (mayExist(probe) || mayExist(probe+".old")) && !binaryMatches(probe, expected)
}

func recoveryError(err error) error {
	return fmt.Errorf("%w: %w; close running Clauduct processes and retry, or reinstall clauduct.exe and clauduct-probe.exe from the same release in a clean directory", errUpdateRecovery, err)
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
