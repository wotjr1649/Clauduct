package providercert

import (
	"encoding/binary"
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
	owner = exactOwner(client, server)
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
	native, created = nativeAncestor(owner, image, parents, names, func(pid uint32) time.Time {
		_, when, _ := processFacts(pid)
		return when
	})
	return owner, native, created
}

// nativeAncestor walks owner's parents (PID -> parent PID) up to 32 hops for the nearest
// process whose image is image. A parent PID outlives its process, so one reused by a later
// process is not this owner's ancestor: every edge must be a parent created before its child,
// and a creation time that cannot be read proves nothing. A walk that breaks that finds no
// native, wherever the reused process sits along it.
func nativeAncestor(owner uint32, image string, parents map[uint32]uint32, names map[uint32]string, createdOf func(uint32) time.Time) (native uint32, created time.Time) {
	child := createdOf(owner)
	for current, hops := owner, 0; current != 0 && hops < 32; current, hops = parents[current], hops+1 {
		created = child
		if current != owner {
			created = createdOf(current)
		}
		if created.IsZero() && (current != owner || strings.EqualFold(names[current], image)) {
			return 0, time.Time{}
		}
		if current != owner && !created.Before(child) {
			return 0, time.Time{}
		}
		if strings.EqualFold(names[current], image) {
			return current, created
		}
		child = created
	}
	return 0, time.Time{}
}

// IsAncestor reports whether target is an ancestor of start through parents (PID -> parent PID),
// within 32 hops. A parent PID outlives its process, so a reused one would lead the walk into an
// unrelated ancestry: every edge, the one into target included, must name a parent created
// strictly before its child, and a creation time createdOf cannot read (the zero time) proves
// nothing. nativeAncestor applies the same rule while searching for an image.
func IsAncestor(start, target uint32, parents map[uint32]uint32, createdOf func(uint32) time.Time) bool {
	child := createdOf(start)
	for current, hops := start, 0; hops < 32; hops++ {
		parent, ok := parents[current]
		if !ok || parent == 0 || parent == current || child.IsZero() {
			return false
		}
		created := createdOf(parent)
		if created.IsZero() || !created.Before(child) {
			return false
		}
		if parent == target {
			return true
		}
		current, child = parent, created
	}
	return false
}

// exactOwner is the PID whose IPv4 TCP connection has exactly client as its local end and
// server as its remote end: addresses and ports. Anything but IPv4 (the gateway listens on
// 127.0.0.1 only) or a failed lookup is 0. ownerOf, which P3 diagnostics use, compares ports
// alone; an identity cannot rest on that.
func exactOwner(client, server net.Addr) uint32 {
	c, ok1 := client.(*net.TCPAddr)
	s, ok2 := server.(*net.TCPAddr)
	if !ok1 || !ok2 || c.IP.To4() == nil || s.IP.To4() == nil {
		return 0
	}
	// The table can grow between sizing and reading it (ERROR_INSUFFICIENT_BUFFER): read it
	// again at the new size, a few times, before answering nothing.
	var buf []byte
	var size uint32
	getExtendedTCP.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0) // AF_INET, TCP_TABLE_OWNER_PID_CONNECTIONS
	for attempt := 0; ; attempt++ {
		if size == 0 || attempt == 4 {
			return 0
		}
		buf = make([]byte, size+4096)
		size = uint32(len(buf))
		r, _, _ := getExtendedTCP.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0)
		if r == 0 {
			break
		}
		if r != 122 { // anything but a table that outgrew the buffer
			return 0
		}
	}
	// Addresses are in network order in memory; ports in network order in the low 16 bits.
	addr := func(ip net.IP) uint32 { return binary.LittleEndian.Uint32(ip.To4()) }
	port := func(p uint32) int { return int(p&0xff)<<8 | int(p>>8&0xff) }
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	var found uint32
	for _, row := range unsafe.Slice((*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4])), count) {
		if row.LocalAddr == addr(c.IP) && port(row.LocalPort) == c.Port && row.RemoteAddr == addr(s.IP) && port(row.RemotePort) == s.Port {
			if found != 0 && found != row.OwningPID {
				return 0 // two owners for one tuple: no answer
			}
			found = row.OwningPID
		}
	}
	return found
}
