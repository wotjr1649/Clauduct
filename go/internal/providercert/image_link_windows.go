//go:build windows

package providercert

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// CatalogOf builds the version catalog of the native install that exe belongs to. A native
// install keeps its executable at <root>/bin/claude.exe and one original per version under
// <root>/share/claude/versions. Any other layout, or an unobservable version file, gives
// ErrInconclusiveNativeIdentity: an image that cannot be looked up is never accepted.
func CatalogOf(exe string) (*Catalog, error) {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return nil, inconclusive("native path: %v", err)
	}
	dir := filepath.Dir(abs)
	if !strings.EqualFold(filepath.Base(dir), "bin") {
		return nil, inconclusive("%s is not under a native bin directory", filepath.Base(abs))
	}
	catalog, _, err := BuildCatalog(filepath.Join(filepath.Dir(dir), "share", "claude", "versions"))
	if err != nil {
		return nil, inconclusive("versions: %v", err)
	}
	return catalog, nil
}

// SameImage reports whether a and b are one file with one content: same volume, file ID, size and
// SHA256. The path is ignored because native's updater renames a running image.
func SameImage(a, b Identity) bool {
	return a.SHA256 != "" && a.SHA256 == b.SHA256 && a.VolumeSerial == b.VolumeSerial && a.FileIndex == b.FileIndex && a.Size == b.Size
}

// ProcessCreated is the creation time (UnixNano, as ImageIdentity takes it) of process pid.
func ProcessCreated(pid uint32) (int64, error) {
	h, err := syscall.OpenProcess(0x1000, false, pid) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return 0, inconclusive("open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, inconclusive("process times: %v", err)
	}
	return created.Nanoseconds(), nil
}

// LinkProcess links the live process pid to the exact image it runs and requires that image to
// be want. The process, not a path, is the evidence, so nothing has to be held against the
// updater. A process that runs another image, for example an older generation that survived an
// update, is ErrNativeChanged; one that cannot be linked is ErrInconclusiveNativeIdentity.
func LinkProcess(pid uint32, catalog *Catalog, want Identity) (NativeImage, error) {
	created, err := ProcessCreated(pid)
	if err != nil {
		return NativeImage{}, err
	}
	image, err := ImageIdentity(pid, created, catalog)
	if err != nil {
		return NativeImage{}, err
	}
	if !SameImage(image.Identity, want) {
		return image, fmt.Errorf("%w: process %d runs %s, not the expected image", ErrNativeChanged, pid, filepath.Base(image.Identity.Path))
	}
	return image, nil
}

// Locate is Observe without reading the content: path, volume serial, file ID and size, taken
// through a fully shared handle. It is cheap enough for every stage boundary; it cannot see an
// in-place edit that keeps the size, which the hashing Observe at the start and end can.
func Locate(path string) (Identity, error) {
	h, err := open(path, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE)
	if err != nil {
		return Identity{}, err
	}
	defer syscall.CloseHandle(h)
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
	return Identity{Path: abs, VolumeSerial: info.VolumeSerialNumber, FileIndex: fileIndex(info), Size: int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)}, nil
}

// LinkSpawned links the process pid to the exact image it runs and requires that image to be
// want, an identity observed (with its SHA256) before the process was created. Nothing is
// inherited from want: the SHA256 of the running image is computed here, from the file the process
// maps. The process's mapped main module is resolved to its current file name while the process is
// alive (the same OS evidence ImageIdentity uses), that file is located, hashed through a fully
// shared handle and located again; the file behind the name must be the same file object before
// and after hashing, and its volume, file ID, size and SHA256 must all equal want. The hash
// does not require the process to survive it: only reading the mapped name does. A process that
// is gone before its mapped image was read, or whose creation time differs (created != 0), is
// ErrInconclusiveNativeIdentity; one that runs another file or other bytes is ErrNativeChanged.
func LinkSpawned(pid uint32, created int64, want Identity) (NativeImage, error) {
	if pid == 0 || want.SHA256 == "" {
		return NativeImage{}, inconclusive("no process or no hashed expected image")
	}
	h, err := syscall.OpenProcess(processQueryInformation|processVMRead|syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		return NativeImage{}, inconclusive("open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(h)
	var c, e, k, u syscall.Filetime
	if r, _, _ := getProcessTimesOwn.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return NativeImage{}, inconclusive("process times unavailable")
	}
	if created != 0 && c.Nanoseconds() != created {
		return NativeImage{}, inconclusive("creation time differs: the process number was reused")
	}
	if wait, werr := syscall.WaitForSingleObject(h, 0); werr != nil || wait != syscall.WAIT_TIMEOUT {
		return NativeImage{}, inconclusive("process exited before its image could be read")
	}
	path, err := mappedImagePath(h)
	if err != nil {
		return NativeImage{}, err
	}
	before, err := Locate(path)
	if err != nil {
		return NativeImage{}, inconclusive("locate %s: %v", filepath.Base(path), err)
	}
	hashed, err := Observe(path)
	if err != nil {
		return NativeImage{}, inconclusive("hash %s: %v", filepath.Base(path), err)
	}
	after, err := Locate(path)
	if err != nil {
		return NativeImage{}, inconclusive("locate %s after hashing: %v", filepath.Base(path), err)
	}
	same := func(a, b Identity) bool {
		return a.VolumeSerial == b.VolumeSerial && a.FileIndex == b.FileIndex && a.Size == b.Size
	}
	if !same(before, hashed) || !same(hashed, after) {
		return NativeImage{}, fmt.Errorf("%w: the file behind the image name changed while it was hashed", ErrNativeChanged)
	}
	if !SameImage(hashed, want) {
		return NativeImage{}, fmt.Errorf("%w: process %d runs %s, not the expected bytes", ErrNativeChanged, pid, filepath.Base(path))
	}
	return NativeImage{PID: pid, Created: c.Nanoseconds(), Identity: hashed}, nil
}
