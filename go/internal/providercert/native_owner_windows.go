package providercert

import (
	"net"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// NativeOwner names, from the OS alone, the process that opened the connection client ->
// server and the nearest process at or above it whose image is image (the native client), with
// that native's creation time so a reused PID cannot pass for it (#319 decision 18d). native is
// 0 when there is no such process within 32 parents or any lookup fails.
func NativeOwner(client, server net.Addr, image string) (owner, native uint32, created time.Time) {
	owner = ownerOf(client, server).PID
	if owner == 0 {
		return 0, 0, time.Time{}
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return owner, 0, time.Time{}
	}
	defer syscall.CloseHandle(snapshot)
	parents, names := map[uint32]uint32{}, map[uint32]string{}
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
		names[entry.ProcessID] = syscall.UTF16ToString(entry.ExeFile[:])
	}
	_, ownerCreated, _ := processFacts(owner)
	for current, hops := owner, 0; current != 0 && hops < 32; current, hops = parents[current], hops+1 {
		if strings.EqualFold(names[current], image) {
			_, created, _ = processFacts(current)
			// A parent PID outlives its process: one reused by a later process is not this
			// owner's ancestor.
			if created.IsZero() || current != owner && !created.Before(ownerCreated) {
				return owner, 0, time.Time{}
			}
			return owner, current, created
		}
	}
	return owner, 0, time.Time{}
}
