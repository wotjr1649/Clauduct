package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrAdmissionBusy is a scope another probe run still holds after the wait.
var ErrAdmissionBusy = errors.New("PROBE_BUSY: another provider-control probe run holds this scope")

// LockAdmission takes scope's writer lock under home (r4 cache.write: one runner per key at a
// time). The lock is a file opened without sharing, so the system releases it when its owner
// ends however it ends; a dead owner's lock is simply free. It waits up to wait for another
// run, then gives ErrAdmissionBusy. The file records the owner's PID and start time.
func LockAdmission(home, scope string, wait time.Duration) (release func(), err error) {
	dir := admissionDir(home)
	if reparsePath(dir) {
		return nil, errors.New("CACHE_WRITE_FAILED: the admission directory is a reparse point")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, scope+".lock"))
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		h, err := syscall.CreateFile(name, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			f := os.NewFile(uintptr(h), scope+".lock")
			f.Truncate(0)
			fmt.Fprintf(f, "pid %d since %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, errSharingViolation) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrAdmissionBusy
		}
		time.Sleep(2 * time.Second)
	}
}

// errSharingViolation is ERROR_SHARING_VIOLATION: another handle holds the file unshared.
const errSharingViolation = syscall.Errno(32)
