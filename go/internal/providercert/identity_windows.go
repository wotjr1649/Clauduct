//go:build windows

// Package providercert certifies, for one exact native executable, that a provider-control
// strategy keeps every inference request on this session's loopback gateway (P3, #320).
// The routing contract it implements is fixed before any measurement; a native that has not
// passed it for the executable about to run receives no user prompt.
package providercert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Identity is what certification is bound to. Path alone is not identity: native's updater
// replaces the file behind the same path, so the file ID and the bytes are compared too.
type Identity struct {
	Path         string `json:"path"`
	VolumeSerial uint32 `json:"volumeSerial"`
	FileIndex    uint64 `json:"fileIndex"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

// identityWire is Identity as written: fileIndex is a decimal STRING because an NTFS file index can
// exceed 2^53, which a JS number cannot hold (distinct indexes would become indistinguishable). The
// other numbers fit: volumeSerial is 32 bits and size is far below 2^53.
type identityWire struct {
	Path         string `json:"path"`
	VolumeSerial uint32 `json:"volumeSerial"`
	FileIndex    string `json:"fileIndex"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

func (i Identity) MarshalJSON() ([]byte, error) {
	return json.Marshal(identityWire{i.Path, i.VolumeSerial, strconv.FormatUint(i.FileIndex, 10), i.Size, i.SHA256})
}

// UnmarshalJSON accepts fileIndex as a decimal string (the written form) or a bare number (older
// records); anything else is an error.
func (i *Identity) UnmarshalJSON(b []byte) error {
	var w struct {
		Path         string          `json:"path"`
		VolumeSerial uint32          `json:"volumeSerial"`
		FileIndex    json.RawMessage `json:"fileIndex"`
		Size         int64           `json:"size"`
		SHA256       string          `json:"sha256"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	text := strings.Trim(strings.TrimSpace(string(w.FileIndex)), `"`)
	var index uint64
	if len(w.FileIndex) != 0 && string(w.FileIndex) != "null" {
		v, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return fmt.Errorf("fileIndex %q is not a decimal integer: %w", text, err)
		}
		index = v
	}
	*i = Identity{w.Path, w.VolumeSerial, index, w.Size, w.SHA256}
	return nil
}

// Observe reads the identity through a single handle opened with full sharing, so a probe
// checkpoint never blocks native's own updater in another terminal.
func Observe(path string) (Identity, error) {
	h, err := open(path, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE)
	if err != nil {
		return Identity{}, err
	}
	defer syscall.CloseHandle(h)
	return identify(path, h)
}

// Pin holds the executable so that no other process can write, delete or rename it until
// Close. A process created from Pin's path while it is held runs exactly the pinned bytes.
// Hold it only around process creation: holding it longer would block native's updater.
type Pin struct {
	handle   syscall.Handle
	Identity Identity
}

// PinFile opens path with read sharing only and reads its identity through that handle.
func PinFile(path string) (*Pin, error) {
	h, err := open(path, syscall.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	id, err := identify(path, h)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	return &Pin{handle: h, Identity: id}, nil
}

// Close releases the pin. It is safe to call twice.
func (p *Pin) Close() error {
	if p == nil || p.handle == syscall.InvalidHandle || p.handle == 0 {
		return nil
	}
	err := syscall.CloseHandle(p.handle)
	p.handle = syscall.InvalidHandle
	return err
}

// SameFile reports whether path, typically a created process's image path, names the pinned
// file. Only the file ID is compared: the pin itself guarantees the bytes did not change.
func (p *Pin) SameFile(path string) (bool, error) {
	h, err := open(path, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return false, err
	}
	return info.VolumeSerialNumber == p.Identity.VolumeSerial && fileIndex(info) == p.Identity.FileIndex, nil
}

// ErrNotRegularFile is returned for directories, devices and other non-files.
var ErrNotRegularFile = errors.New("NATIVE_NOT_A_REGULAR_FILE")

func open(path string, share uint32) (syscall.Handle, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	name, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(name, syscall.GENERIC_READ, share, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
}

func identify(path string, h syscall.Handle) (Identity, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return Identity{}, ErrNotRegularFile
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Identity{}, err
	}
	sum := sha256.New()
	n, err := io.CopyBuffer(sum, handleReader(h), make([]byte, 1<<20))
	if err != nil {
		return Identity{}, err
	}
	size := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
	if n != size {
		return Identity{}, fmt.Errorf("NATIVE_SIZE_CHANGED_WHILE_HASHING: read %d of %d bytes", n, size)
	}
	return Identity{Path: abs, VolumeSerial: info.VolumeSerialNumber, FileIndex: fileIndex(info), Size: size, SHA256: hex.EncodeToString(sum.Sum(nil))}, nil
}

func fileIndex(info syscall.ByHandleFileInformation) uint64 {
	return uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
}

// handleReader reads a handle it does not own. An *os.File would close the handle from its
// finalizer, which is why the bytes are read with ReadFile directly.
type handleReader syscall.Handle

func (r handleReader) Read(b []byte) (int, error) {
	var done uint32
	err := syscall.ReadFile(syscall.Handle(r), b, &done, nil)
	if err != nil {
		return int(done), err
	}
	if done == 0 && len(b) > 0 {
		return 0, io.EOF
	}
	return int(done), nil
}
