package platform

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var lockFile = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// LockFile takes one nonblocking exclusive byte-range lock. Closing the file
// releases it, including when its owner crashes; no stale PID-file recovery.
func LockFile(file *os.File) error {
	return LockFileAt(file, 0)
}

// LockFileAt takes the same one-byte lock at a caller-supplied offset.
func LockFileAt(file *os.File, offset int64) error {
	const failImmediately, exclusive = 1, 2
	overlapped := syscall.Overlapped{Offset: uint32(offset), OffsetHigh: uint32(offset >> 32)}
	if ok, _, err := lockFile.Call(file.Fd(), failImmediately|exclusive, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped))); ok == 0 {
		return err
	}
	return nil
}

// FileByteLocked observes an exclusive lock without acquiring one.
func FileByteLocked(file *os.File, offset int64) bool {
	var probe [1]byte
	_, err := file.ReadAt(probe[:], offset)
	return errors.Is(err, syscall.Errno(33)) // ERROR_LOCK_VIOLATION
}
