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

// An exclusive, delete-on-close handle prevents concurrent mutations and survives
// neither normal exit nor process termination.
func lockInstallation(dir string) (*os.File, error) {
	path := filepath.Join(dir, lockName)
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	const deleteAccess, deleteOnClose = 0x10000, 0x04000000
	handle, err := syscall.CreateFile(name, deleteAccess, 0, nil, syscall.CREATE_NEW, deleteOnClose, 0)
	if err != nil {
		return nil, fmt.Errorf("could not lock the installation: %w", &os.PathError{Op: "open", Path: path, Err: err})
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Recover finishes an interrupted pair swap before commands can use the installation.
func Recover(expected string) error {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return recoverIn(self, expected)
}

func recoverIn(self, expected string) error {
	if !strings.EqualFold(filepath.Base(self), Binaries[0]) {
		sweepLeftoverOf(self)
		return nil
	}
	dir := filepath.Dir(self)
	probe := filepath.Join(dir, Probe)
	if !mayExist(filepath.Join(dir, lockName)) && !mayExist(self+".old") &&
		!mayExist(self+stageSuffix) && !mayExist(probe+stageSuffix) {
		return nil
	}
	lock, err := lockInstallation(dir)
	if err != nil {
		if mayExist(filepath.Join(dir, lockName)) {
			return fmt.Errorf("UPDATE_BUSY: wait for the update to finish and retry: %w", err)
		}
		if !mayExist(self+stageSuffix) && !mayExist(probe+stageSuffix) {
			return nil // An old backup alone is only housekeeping.
		}
		return recoveryError(err)
	}
	defer lock.Close()

	if mayExist(self + stageSuffix) {
		if err := discardStages(self); err != nil {
			return recoveryError(err)
		}
	} else if staged := probe + stageSuffix; mayExist(staged) {
		switch {
		case probeMatches(staged, expected):
			if err := renameFile(staged, probe); err != nil {
				return recoveryError(err)
			}
			_ = removeIfPresent(probe + ".old")
		case !mayExist(self+".old") && (!mayExist(probe) || probeMatches(probe, expected)):
			// The executable was already rolled back, but stage cleanup was interrupted.
			if err := removeIfPresent(staged); err != nil {
				return recoveryError(err)
			}
		default:
			return recoveryError(fmt.Errorf("%s does not match the probe SHA-256 embedded in this executable", staged))
		}
	} else if mayExist(self+".old") && expected != "" &&
		(mayExist(probe) || mayExist(probe+".old")) && !probeMatches(probe, expected) {
		return recoveryError(fmt.Errorf("%s is missing or does not match the probe SHA-256 embedded in this executable", probe))
	}
	sweepLeftoverOf(self)
	return nil
}

func recoveryError(err error) error {
	return fmt.Errorf("UPDATE_RECOVERY_REQUIRED: %w; close running Clauduct processes and retry, or reinstall clauduct.exe and clauduct-probe.exe from the same release", err)
}

func probeMatches(path, expected string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBinaryBytes {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBinaryBytes+1))
	return err == nil && len(body) <= maxBinaryBytes && Digest(body) == expected
}

func mayExist(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func discardStages(self string) error {
	// Leave the executable stage as the pre-swap signal until the probe stage is gone.
	probe := filepath.Join(filepath.Dir(self), Probe)
	if err := removeIfPresent(probe + stageSuffix); err != nil {
		return err
	}
	return removeIfPresent(self + stageSuffix)
}
