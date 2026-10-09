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
	var size uint32
	getExtendedTCP.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0) // AF_INET, TCP_TABLE_OWNER_PID_CONNECTIONS
	if size == 0 {
		return 0
	}
	buf := make([]byte, size+4096)
	size = uint32(len(buf))
	if r, _, _ := getExtendedTCP.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0); r != 0 {
		return 0
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
