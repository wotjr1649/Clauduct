// Package sessionlink keeps a background session credential in memory. The named
// pipe's DACL admits only this Windows logon; neither its name nor the CLI carries
// a credential. There is no network listener, credential file or recovery replay.
package sessionlink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var ErrUnavailable = errors.New("BACKGROUND_CONNECTION_UNAVAILABLE")

var kernel = syscall.NewLazyDLL("kernel32.dll")
var createPipe = kernel.NewProc("CreateNamedPipeW")
var connectPipe = kernel.NewProc("ConnectNamedPipe")
var disconnectPipe = kernel.NewProc("DisconnectNamedPipe")
var createEvent = kernel.NewProc("CreateEventW")
var overlappedResult = kernel.NewProc("GetOverlappedResult")
var pipeServerPID = kernel.NewProc("GetNamedPipeServerProcessId")
var descriptorFromString = syscall.NewLazyDLL("advapi32.dll").NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Connection struct {
	BaseURL string `json:"baseURL"`
	Token   string `json:"token"`
	PID     int    `json:"-"` // verified by GetNamedPipeServerProcessId, never trusted from JSON
}

type Server struct {
	ID      string
	Stopped chan struct{}
	Done    chan struct{}
	cancel  context.CancelFunc
	once    sync.Once
}

func logonSID(token syscall.Token) (string, error) {
	// TOKEN_GROUPS contains one SID for TokenLogonSid; the Go layout supplies
	// the native pointer alignment. The buffer stays alive through SID.String.
	buf := make([]uint64, 512) // TOKEN_GROUPS contains pointer-aligned fields.
	ptr := unsafe.Pointer(&buf[0])
	var size uint32
	if err := syscall.GetTokenInformation(token, syscall.TokenLogonSid, (*byte)(ptr), uint32(len(buf)*8), &size); err != nil {
		return "", ErrUnavailable
	}
	groups := (*struct {
		Count  uint32
		Groups [1]syscall.SIDAndAttributes
	})(ptr)
	if groups.Count != 1 || groups.Groups[0].Sid == nil {
		return "", ErrUnavailable
	}
	sid, err := groups.Groups[0].Sid.String()
	runtime.KeepAlive(buf)
	return sid, err
}

func ownLogon() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", ErrUnavailable
	}
	defer token.Close()
	return logonSID(token)
}

func pipeName(id string) (*uint16, error) {
	if !validID.MatchString(id) {
		return nil, ErrUnavailable
	}
	return syscall.UTF16PtrFromString(`\\.\pipe\clauduct-` + id)
}

func Start(parent context.Context, connection Connection) (*Server, error) {
	sid, err := ownLogon()
	if err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(nonce[:])
	name, _ := pipeName(id)
	sddl, _ := syscall.UTF16PtrFromString("D:P(A;;GA;;;" + sid + ")")
	var descriptor uintptr
	if ok, _, _ := descriptorFromString.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0); ok == 0 {
		return nil, ErrUnavailable
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: descriptor}
	// Duplex, overlapped, FIRST_PIPE_INSTANCE; message mode, remote clients refused.
	h, _, _ := createPipe.Call(uintptr(unsafe.Pointer(name)), 3|0x40000000|0x80000, 4|2|8, 1, 4096, 4096, 0, uintptr(unsafe.Pointer(&sa)))
	if syscall.Handle(h) == syscall.InvalidHandle {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Server{ID: id, Stopped: make(chan struct{}), Done: make(chan struct{}), cancel: cancel}
	body, _ := json.Marshal(connection)
	go func() {
		defer close(s.Done)
		defer syscall.CloseHandle(syscall.Handle(h))
		for ctx.Err() == nil {
			if _, err := operation(ctx, syscall.Handle(h), nil, 'c'); err != nil {
				return
			}
			call, stop := context.WithTimeout(ctx, 3*time.Second)
			var command [1]byte
			n, err := operation(call, syscall.Handle(h), command[:], 'r')
			if err == nil && n == 1 && (command[0] == 'k' || command[0] == 's') {
				reply := body
				if command[0] == 's' {
					reply = []byte(`{}`)
				}
				if _, err = operation(call, syscall.Handle(h), reply, 'w'); err == nil {
					var ack [1]byte
					_, _ = operation(call, syscall.Handle(h), ack[:], 'r')
					if command[0] == 's' {
						s.once.Do(func() { close(s.Stopped) })
					}
				}
			}
			stop()
			disconnectPipe.Call(h)
		}
	}()
	return s, nil
}

func (s *Server) Close() { s.cancel(); <-s.Done }

// operation reaps canceled overlapped I/O before releasing its buffer/event.
func operation(ctx context.Context, h syscall.Handle, buf []byte, kind byte) (uint32, error) {
	event, _, _ := createEvent.Call(0, 1, 0, 0)
	if event == 0 {
		return 0, ErrUnavailable
	}
	defer syscall.CloseHandle(syscall.Handle(event))
	o := syscall.Overlapped{HEvent: syscall.Handle(event)}
	var n uint32
	var err error
	switch kind {
	case 'c':
		ok, _, e := connectPipe.Call(uintptr(h), uintptr(unsafe.Pointer(&o)))
		if ok != 0 || e == syscall.Errno(535) {
			return 0, nil
		} // ERROR_PIPE_CONNECTED
		err = e
	case 'r':
		err = syscall.ReadFile(h, buf, &n, &o)
	case 'w':
		err = syscall.WriteFile(h, buf, &n, &o)
	}
	if err == syscall.ERROR_IO_PENDING {
		for {
			state, waitErr := syscall.WaitForSingleObject(o.HEvent, 50)
			if state == syscall.WAIT_OBJECT_0 {
				break
			}
			if ctx.Err() != nil || waitErr != nil {
				_ = syscall.CancelIoEx(h, &o)
				overlappedResult.Call(uintptr(h), uintptr(unsafe.Pointer(&o)), uintptr(unsafe.Pointer(&n)), 1)
				return 0, ErrUnavailable
			}
		}
		if ok, _, e := overlappedResult.Call(uintptr(h), uintptr(unsafe.Pointer(&o)), uintptr(unsafe.Pointer(&n)), 0); ok == 0 {
			err = e
		} else {
			err = nil
		}
	}
	runtime.KeepAlive(buf)
	if err != nil {
		return 0, ErrUnavailable
	}
	return n, nil
}

func request(id string, command byte, wait bool) (Connection, error) {
	name, err := pipeName(id)
	if err != nil {
		return Connection{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var h syscall.Handle
	for {
		// Identification only: a hostile server cannot impersonate this client.
		h, err = syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0x40000000|0x100000|0x10000, 0)
		if err != syscall.Errno(231) {
			break
		} // ERROR_PIPE_BUSY
		select {
		case <-ctx.Done():
			return Connection{}, ErrUnavailable
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err != nil {
		return Connection{}, ErrUnavailable
	}
	defer syscall.CloseHandle(h)
	// Check the server too: a different login must not squat on a public pipe name.
	var pid uint32
	if ok, _, _ := pipeServerPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid))); ok == 0 {
		return Connection{}, ErrUnavailable
	}
	p, err := syscall.OpenProcess(0x1000|syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		return Connection{}, ErrUnavailable
	}
	defer syscall.CloseHandle(p)
	var token syscall.Token
	if syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &token) != nil {
		return Connection{}, ErrUnavailable
	}
	defer token.Close()
	server, err := logonSID(token)
	self, ownErr := ownLogon()
	if err != nil || ownErr != nil || server != self {
		return Connection{}, ErrUnavailable
	}
	if _, err = operation(ctx, h, []byte{command}, 'w'); err != nil {
		return Connection{}, err
	}
	buf := make([]byte, 4096)
	n, err := operation(ctx, h, buf, 'r')
	if err != nil {
		return Connection{}, err
	}
	var reply Connection
	if json.Unmarshal(buf[:n], &reply) != nil {
		return Connection{}, ErrUnavailable
	}
	reply.PID = int(pid)
	if _, err = operation(ctx, h, []byte{'a'}, 'w'); err != nil {
		return Connection{}, err
	}
	if wait {
		state, err := syscall.WaitForSingleObject(p, 10000)
		var code uint32
		if err != nil || state != syscall.WAIT_OBJECT_0 || syscall.GetExitCodeProcess(p, &code) != nil || code != 0 {
			return Connection{}, errors.New("BACKGROUND_STOP_UNVERIFIED: inspect the resident status")
		}
	}
	return reply, nil
}

func Read(id string) (Connection, error) { return request(id, 'k', false) }
func Stop(id string) error               { _, err := request(id, 's', false); return err }
func StopAndWait(id string) error        { _, err := request(id, 's', true); return err }
