//go:build windows

package providercert

import (
	"os"
	"strconv"
	"sync"
	"syscall"
	"unsafe"
)

// Acquirer identifies the process that made a link: its PID (a JSON number) and its creation time
// from GetProcessTimes as a UTC FILETIME decimal string (100 ns ticks since 1601-01-01; a string so
// that every digit survives JSON, matching /^[1-9][0-9]*$/). Together they are a process generation:
// the PID alone can be reused.
type Acquirer struct {
	PID      uint32 `json:"pid"`
	Creation string `json:"creation"`
}

var (
	acquirerOnce sync.Once
	acquirerSelf *Acquirer
)

// currentAcquirer is this process's Acquirer, read once at first use and cached. It is nil only when
// the creation time cannot be read, in which case a link carries no acquirer (and a judge rejects it).
func currentAcquirer() *Acquirer {
	acquirerOnce.Do(func() {
		self, err := syscall.GetCurrentProcess()
		if err != nil {
			return
		}
		var c, e, k, u syscall.Filetime
		if r, _, _ := getProcessTimesOwn.Call(uintptr(self), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
			return
		}
		ticks := uint64(c.HighDateTime)<<32 | uint64(c.LowDateTime)
		if ticks == 0 {
			return
		}
		acquirerSelf = &Acquirer{PID: uint32(os.Getpid()), Creation: strconv.FormatUint(ticks, 10)}
	})
	return acquirerSelf
}
