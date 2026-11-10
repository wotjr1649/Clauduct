package platform

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var rtlGetVersion = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")

// osVersionInfo is RTL_OSVERSIONINFOW.
type osVersionInfo struct {
	size, major, minor, build, platform uint32
	csd                                 [128]uint16
}

// ID names the operating system this process runs on: GOOS/GOARCH and the Windows
// major.minor.build, without the update revision, so monthly updates keep the same ID.
// RtlGetVersion is used because GetVersion reports a compatibility value to an
// unmanifested program.
func ID() (string, error) {
	info := osVersionInfo{}
	info.size = uint32(unsafe.Sizeof(info))
	if status, _, _ := rtlGetVersion.Call(uintptr(unsafe.Pointer(&info))); status != 0 {
		return "", fmt.Errorf("RtlGetVersion: status %#x", status)
	}
	return fmt.Sprintf("%s/%s windows %d.%d.%d", runtime.GOOS, runtime.GOARCH, info.major, info.minor, info.build), nil
}
