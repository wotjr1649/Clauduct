package providercert

import (
	"net"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ConnOwner is the local process that opened one connection to a receiver, read from the TCP
// table when the connection was accepted.
type ConnOwner struct {
	At        time.Time `json:"at"`
	PID       uint32    `json:"pid"`
	ParentPID uint32    `json:"parentPid"`
	Image     string    `json:"image"`
	// Created is the process's creation time, so a later record cannot confuse it with another
	// process that reused the number.
	Created time.Time `json:"created"`
}

var (
	iphlpapi           = syscall.NewLazyDLL("iphlpapi.dll")
	getExtendedTCP     = iphlpapi.NewProc("GetExtendedTcpTable")
	kernelOwner        = syscall.NewLazyDLL("kernel32.dll")
	queryImageNameW    = kernelOwner.NewProc("QueryFullProcessImageNameW")
	getProcessTimesOwn = kernelOwner.NewProc("GetProcessTimes")
)

// mibTCPRowOwnerPID is MIB_TCPROW_OWNER_PID; ports are in network order in the low 16 bits.
type mibTCPRowOwnerPID struct {
	State, LocalAddr, LocalPort, RemoteAddr, RemotePort, OwningPID uint32
}

// ownerOf finds the process on this machine whose IPv4 TCP connection has client as its local
// end and server as its remote end. Any failure returns a zero owner.
func ownerOf(client, server net.Addr) ConnOwner {
	owner := ConnOwner{At: time.Now()}
	c, ok1 := client.(*net.TCPAddr)
	s, ok2 := server.(*net.TCPAddr)
	if !ok1 || !ok2 {
		return owner
	}
	var size uint32
	// AF_INET = 2, TCP_TABLE_OWNER_PID_CONNECTIONS = 4.
	getExtendedTCP.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0)
	if size == 0 {
		return owner
	}
	buf := make([]byte, size+4096)
	size = uint32(len(buf))
	if r, _, _ := getExtendedTCP.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 4, 0); r != 0 {
		return owner
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rows := unsafe.Slice((*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4])), count)
	port := func(p uint32) int { return int(p&0xff)<<8 | int(p>>8&0xff) }
	for _, row := range rows {
		if port(row.LocalPort) == c.Port && port(row.RemotePort) == s.Port {
			owner.PID = row.OwningPID
			break
		}
	}
	if owner.PID == 0 {
		return owner
	}
	owner.Image, owner.Created = processFacts(owner.PID)
	owner.ParentPID = parentOf(owner.PID)
	return owner
}

func processFacts(pid uint32) (string, time.Time) {
	h, err := syscall.OpenProcess(0x1000, false, pid) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return "", time.Time{}
	}
	defer syscall.CloseHandle(h)
	name := make([]uint16, 1024)
	size := uint32(len(name))
	image := ""
	if r, _, _ := queryImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&size))); r != 0 {
		image = syscall.UTF16ToString(name[:size])
		if i := strings.LastIndexAny(image, `\/`); i >= 0 {
			image = image[i+1:]
		}
	}
	var created, exited, kernel, user syscall.Filetime
	var when time.Time
	if r, _, _ := getProcessTimesOwn.Call(uintptr(h), uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r != 0 {
		when = time.Unix(0, created.Nanoseconds())
	}
	return image, when
}

func parentOf(pid uint32) uint32 {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return entry.ParentProcessID
		}
	}
	return 0
}
