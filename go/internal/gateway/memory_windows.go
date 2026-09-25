package gateway

import (
	"syscall"
	"unsafe"
)

var globalMemoryStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// MEMORYSTATUSEX is the Windows physical-memory snapshot. An unavailable query
// fails closed; neither a guessed budget nor a zero-valued fallback admits work.
func systemMemory() (physical, available uint64, err error) {
	var state struct {
		Length, Load                                                                         uint32
		Total, Available, TotalPage, AvailablePage, TotalVirtual, AvailableVirtual, Extended uint64
	}
	state.Length = uint32(unsafe.Sizeof(state))
	if ok, _, _ := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&state))); ok == 0 || state.Total == 0 || state.Available > state.Total {
		return 0, 0, errMemoryStatus
	}
	return state.Total, state.Available, nil
}
