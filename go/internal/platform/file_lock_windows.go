package platform

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFile = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// LockFile takes one nonblocking exclusive byte-range lock. Closing the file
// releases it, including when its owner crashes; no stale PID-file recovery.
func LockFile(file *os.File) error {
	const failImmediately, exclusive = 1, 2
	var overlapped syscall.Overlapped
	if ok, _, err := lockFile.Call(file.Fd(), failImmediately|exclusive, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped))); ok == 0 {
		return err
	}
	return nil
}
