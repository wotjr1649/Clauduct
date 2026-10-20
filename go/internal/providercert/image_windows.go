//go:build windows

package providercert

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Native's updater renames a running claude.exe to claude.exe.old.<ts>.<n> and installs the new
// binary at claude.exe. For a process started before that, QueryFullProcessImageName, WMI and
// Process.Path still name claude.exe, and opening it yields the new binary. The identity of a
// running native is therefore read from the file its main module is mapped from, whose current
// name the memory manager resolves from the section's file object (GetMappedFileName), and every
// file is observed with full sharing so none of this blocks the updater.

var (
	// ErrInconclusiveNativeIdentity means the process cannot be linked to one exact, known image.
	// It blocks completion; it is never a pass.
	ErrInconclusiveNativeIdentity = errors.New("INCONCLUSIVE_NATIVE_IDENTITY")
	// ErrNativeChanged means the image, its name or its file changed while it was observed.
	ErrNativeChanged = errors.New("INVALID_NATIVE_CHANGED")
)

func inconclusive(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInconclusiveNativeIdentity, fmt.Sprintf(format, args...))
}

var (
	enumProcessModulesEx = kernelOwner.NewProc("K32EnumProcessModulesEx")
	getMappedFileNameW   = kernelOwner.NewProc("K32GetMappedFileNameW")
	queryDosDeviceW      = kernelOwner.NewProc("QueryDosDeviceW")
)

// Catalog maps the SHA256 of every file in native's versions directory to its version.
type Catalog struct {
	byHash map[string][]string
}

// Version is the version whose file has this SHA256. Zero or several versions with the same
// bytes give no answer.
func (c *Catalog) Version(sha string) (string, bool) {
	if c == nil {
		return "", false
	}
	v := c.byHash[sha]
	if len(v) != 1 {
		return "", false
	}
	return v[0], true
}

// Versions lists the catalogued version names.
func (c *Catalog) Versions() []string {
	var out []string
	if c != nil {
		for _, v := range c.byHash {
			out = append(out, v...)
		}
	}
	sort.Strings(out)
	return out
}

// VersionFile is one observed file of the versions directory.
type VersionFile struct {
	Version  string   `json:"version"`
	Identity Identity `json:"identity"`
}

// BuildCatalog observes every regular file in dir (native keeps one per version, named by the
// version) with full sharing. A file that cannot be observed fails the whole catalog.
func BuildCatalog(dir string) (*Catalog, []VersionFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	c := &Catalog{byHash: map[string][]string{}}
	var files []VersionFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, err := Observe(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("observe version %s: %w", e.Name(), err)
		}
		c.byHash[id.SHA256] = append(c.byHash[id.SHA256], e.Name())
		files = append(files, VersionFile{Version: e.Name(), Identity: id})
	}
	return c, files, nil
}

// NativeImage is the exact image a process runs.
type NativeImage struct {
	PID     uint32 `json:"pid"`
	Created int64  `json:"created"` // UnixNano, as ProcessStatus takes it
	// Identity.Path is the image's current name, which differs from the path the process was
	// started from after native's updater renamed it.
	Identity Identity `json:"identity"`
	Version  string   `json:"version"`
}

const (
	processQueryInformation = 0x0400
	processVMRead           = 0x0010
	listModulesAll          = 3
)

// ImageIdentity links process pid, created at created (UnixNano), to the file it runs: the
// current name of its mapped main module, that file's ID, size and SHA256 and the version the
// catalog gives for those bytes. The name, file ID and process are checked again after hashing.
// Anything unknown or inconsistent fails closed (ErrInconclusiveNativeIdentity or
// ErrNativeChanged).
func ImageIdentity(pid uint32, created int64, catalog *Catalog) (NativeImage, error) {
	if pid == 0 || created <= 0 {
		return NativeImage{}, inconclusive("no process")
	}
	// One handle for the whole observation: it keeps the PID from being reused meanwhile.
	h, err := syscall.OpenProcess(processQueryInformation|processVMRead|syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		return NativeImage{}, inconclusive("open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(h)
	if err := processStill(h, created); err != nil {
		return NativeImage{}, err
	}
	first, err := mappedImagePath(h)
	if err != nil {
		return NativeImage{}, err
	}
	id, err := Observe(first)
	if err != nil {
		return NativeImage{}, inconclusive("observe %s: %v", filepath.Base(first), err)
	}
	// The name must still be the same, and name the same file, after the bytes were read.
	second, err := mappedImagePath(h)
	if err != nil {
		return NativeImage{}, err
	}
	if !strings.EqualFold(first, second) {
		return NativeImage{}, fmt.Errorf("%w: image renamed while hashing", ErrNativeChanged)
	}
	vol, index, size, err := fileID(second)
	if err != nil {
		return NativeImage{}, inconclusive("reopen %s: %v", filepath.Base(second), err)
	}
	if vol != id.VolumeSerial || index != id.FileIndex || size != id.Size {
		return NativeImage{}, fmt.Errorf("%w: file behind the image name changed while hashing", ErrNativeChanged)
	}
	if err := processStill(h, created); err != nil {
		return NativeImage{}, err
	}
	version, ok := catalog.Version(id.SHA256)
	if !ok {
		return NativeImage{}, inconclusive("image %s is not in the version catalog", filepath.Base(id.Path))
	}
	return NativeImage{PID: pid, Created: created, Identity: id, Version: version}, nil
}

// processStill checks, on an open handle, that the process has not exited and was created at created.
func processStill(h syscall.Handle, created int64) error {
	switch wait, err := syscall.WaitForSingleObject(h, 0); {
	case err != nil:
		return inconclusive("wait: %v", err)
	case wait == syscall.WAIT_OBJECT_0:
		return inconclusive("process exited")
	case wait != syscall.WAIT_TIMEOUT:
		return inconclusive("wait result %d", wait)
	}
	var c, e, k, u syscall.Filetime
	if r, _, _ := getProcessTimesOwn.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return inconclusive("process times unavailable")
	}
	if c.Nanoseconds() != created {
		return inconclusive("creation time differs: the process number was reused")
	}
	return nil
}

// mappedImagePath is the DOS path of the file the process's main module is mapped from.
func mappedImagePath(h syscall.Handle) (string, error) {
	var modules [1]uintptr
	var needed uint32
	// A process still starting has no module list yet (ERROR_PARTIAL_COPY): wait for it briefly.
	for attempt := 0; ; attempt++ {
		r, _, err := enumProcessModulesEx.Call(uintptr(h), uintptr(unsafe.Pointer(&modules[0])), unsafe.Sizeof(modules), uintptr(unsafe.Pointer(&needed)), listModulesAll)
		if r != 0 && needed != 0 && modules[0] != 0 {
			break
		}
		if attempt == 100 {
			return "", inconclusive("main module unavailable: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	buf := make([]uint16, 32768)
	n, _, err := getMappedFileNameW.Call(uintptr(h), modules[0], uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) >= len(buf) {
		return "", inconclusive("mapped file name unavailable: %v", err)
	}
	return deviceToDOS(syscall.UTF16ToString(buf[:n]))
}

// deviceToDOS turns \Device\HarddiskVolumeN\... into a drive-letter path. A device without a
// drive letter (UNC, unmapped volume) has no answer.
func deviceToDOS(device string) (string, error) {
	for letter := 'A'; letter <= 'Z'; letter++ {
		drive := string(letter) + ":"
		name, err := syscall.UTF16PtrFromString(drive)
		if err != nil {
			return "", err
		}
		buf := make([]uint16, 1024)
		if n, _, _ := queryDosDeviceW.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n == 0 {
			continue
		}
		target := syscall.UTF16ToString(buf) // first of the multi-string
		if len(device) > len(target) && strings.EqualFold(device[:len(target)], target) && device[len(target)] == '\\' {
			return drive + device[len(target):], nil
		}
	}
	return "", inconclusive("no drive letter for %s", device)
}

// fileID reads a file's volume, index and size through a fully shared handle, without reading it.
func fileID(path string) (uint32, uint64, int64, error) {
	h, err := open(path, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE)
	if err != nil {
		return 0, 0, 0, err
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return 0, 0, 0, err
	}
	return info.VolumeSerialNumber, fileIndex(info), int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow), nil
}

// Manifest is the installed native and every per-version original, observed without locking.
type Manifest struct {
	Installed Identity `json:"installed"`
	// InstalledVersion is what the installed executable reports for --version.
	InstalledVersion string `json:"installedVersion"`
	// CatalogVersion is the versions/ entry with the installed bytes.
	CatalogVersion string        `json:"catalogVersion"`
	Versions       []VersionFile `json:"versions"`
}

// NativeManifest observes root/bin/claude.exe and root/share/claude/versions/*, runs the
// installed executable with --version and cross-checks that version against the catalog entry
// with the same SHA256. Any gap or disagreement is ErrInconclusiveNativeIdentity. Only the
// harness calls it: it executes the installed native.
func NativeManifest(root string) (Manifest, *Catalog, error) {
	return nativeManifest(root, versionOutput)
}

func nativeManifest(root string, run func(path string) (string, error)) (Manifest, *Catalog, error) {
	catalog, files, err := BuildCatalog(filepath.Join(root, "share", "claude", "versions"))
	if err != nil {
		return Manifest{}, nil, inconclusive("versions: %v", err)
	}
	installed, err := Observe(filepath.Join(root, "bin", "claude.exe"))
	if err != nil {
		return Manifest{}, nil, inconclusive("installed: %v", err)
	}
	m := Manifest{Installed: installed, Versions: files}
	var ok bool
	if m.CatalogVersion, ok = catalog.Version(installed.SHA256); !ok {
		return m, catalog, inconclusive("installed claude.exe is not in the version catalog")
	}
	if m.InstalledVersion, err = run(installed.Path); err != nil {
		return m, catalog, inconclusive("--version: %v", err)
	}
	if m.InstalledVersion != m.CatalogVersion {
		return m, catalog, inconclusive("--version %q disagrees with catalog %q", m.InstalledVersion, m.CatalogVersion)
	}
	// The file may have been replaced while --version ran.
	if again, err := Observe(installed.Path); err != nil || again != installed {
		return m, catalog, fmt.Errorf("%w: installed claude.exe changed during the manifest", ErrNativeChanged)
	}
	return m, catalog, nil
}

// versionOutput runs `path --version` and returns its first word.
func versionOutput(path string) (string, error) {
	cmd := exec.Command(path, "--version")
	timer := time.AfterFunc(30*time.Second, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", errors.New("empty output")
	}
	return fields[0], nil
}
