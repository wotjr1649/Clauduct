//go:build windows

package providercert

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

// MappedImage is what a live process maps, acquired while the process was alive: the current name
// and file identity (volume, file ID, size; no content) of its main module. It carries no process
// handle, so hashing it afterwards never needs the process to survive.
type MappedImage struct {
	PID     uint32
	Created int64
	Path    string
	Located Identity
	// hold is the image file, opened while the process was alive with READ|WRITE|DELETE sharing
	// (nothing is locked or pinned), and stamp its change stamp then. The hash reads through hold and
	// requires the stamp to be unchanged; Verify and Identify close hold, and Close does.
	hold  *fileHold
	stamp ChangeStamp
	// bindStart is wallNow() immediately before the first mapped-name read of the acquisition.
	bindStart int64
}

// acquiredPathIfMoved is the acquisition-time path when the verified mapped name differs from it
// (the updater renamed the running image while it was hashed), otherwise empty.
func (m MappedImage) acquiredPathIfMoved(hashed Identity) string {
	abs, err := filepath.Abs(m.Path)
	if err != nil || strings.EqualFold(abs, hashed.Path) {
		return ""
	}
	return abs
}

// heldMS is how long the image section has been held, in milliseconds (recorded on the result).
func (m MappedImage) heldMS() int64 {
	if m.hold == nil || m.hold.sectionAt.IsZero() {
		return 0
	}
	return time.Since(m.hold.sectionAt).Milliseconds()
}

// Close releases the held image file. Verify and Identify close it themselves; Close is for an
// acquired image that is never hashed. It is safe to call more than once.
func (m MappedImage) Close() { m.hold.close() }

// AcquireMappedImage reads, under one process handle, the creation time and the mapped main
// module's file name, and locates that file. It is the only step that needs the process alive and is
// fast: no hashing. Failures: ErrProcessExited (gone before the image was read), ErrModuleNotReady
// (alive but still starting after the bounded retry), a reused PID (created != 0 and different), or
// an access error; all of them wrap ErrInconclusiveNativeIdentity.
func AcquireMappedImage(pid uint32, created int64) (MappedImage, error) {
	if pid == 0 {
		return MappedImage{}, inconclusive("no process")
	}
	h, err := syscall.OpenProcess(processQueryInformation|processVMRead|syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok && errno == 87 { // ERROR_INVALID_PARAMETER: no such process
			return MappedImage{}, inconclusiveKind(ErrProcessExited, "open process %d: no such process", pid)
		}
		return MappedImage{}, inconclusive("open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(h)
	var c, e, k, u syscall.Filetime
	if r, _, _ := getProcessTimesOwn.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return MappedImage{}, inconclusive("process times unavailable")
	}
	if created != 0 && c.Nanoseconds() != created {
		return MappedImage{}, inconclusive("creation time differs: the process number was reused")
	}
	if wait, werr := syscall.WaitForSingleObject(h, 0); werr != nil || wait != syscall.WAIT_TIMEOUT {
		return MappedImage{}, inconclusiveKind(ErrProcessExited, "process exited before its image could be read")
	}
	// The name is read, the file located and opened by that name, the file's image section is created,
	// and then, under the SAME process handle, the process must still be alive and its mapped image name
	// must still be the name that was opened, that name must still reach the held file object, and the
	// held file must still be a file (not delete-pending, links left). Between the name read and the open
	// the updater may rename the running file and put another at the name (the open would then hold the
	// wrong file); after the open the names may be rearranged again so the old name returns; the process
	// may exit. Each is caught here and retried within a bounded budget while the process is alive; a
	// process gone before the checks end is INCONCLUSIVE (exited); a name that keeps changing is
	// INCONCLUSIVE (not ready). Nothing is linked from an acquisition that could not be shown consistent.
	// The image section also makes the file unwritable and undeletable (not renamable-proof, not
	// pinned) from the moment it exists until the hash is done, whether or not the process lives on.
	// The binding interval starts immediately before the first mapped-name read and keeps that start
	// across retries.
	bindStart := wallNow()
	var last error
	for attempt := 0; attempt < acquireConsistencyAttempts; attempt++ {
		path, err := mappedImagePath(h)
		if err != nil {
			return MappedImage{}, err
		}
		if acquireSeam != nil {
			acquireSeam(attempt) // test seam: between the name read and the open
		}
		located, err := Locate(path)
		if err != nil {
			last = inconclusive("locate %s: %v", filepath.Base(path), err)
		} else if hold, stamp, err := holdFile(path, located); err != nil {
			last = err // the file behind the name is not the one located: the name moved under us
		} else if err := hold.imageSection(); err != nil {
			hold.close()
			last = inconclusive("%s: %v", filepath.Base(path), err)
		} else {
			if err := hold.attachProcess(h); err != nil {
				hold.close()
				return MappedImage{}, inconclusive("process handle for the hold: %v", err)
			}
			if acquireSeamAfterHold != nil {
				acquireSeamAfterHold(attempt) // test seam: between the hold and the second name read
			}
			if wait, werr := syscall.WaitForSingleObject(h, 0); werr != nil || wait != syscall.WAIT_TIMEOUT {
				hold.close()
				return MappedImage{}, inconclusiveKind(ErrProcessExited, "process exited before its image file was held")
			}
			again, err := mappedImagePath(h)
			if err != nil {
				hold.close()
				return MappedImage{}, err
			}
			if !strings.EqualFold(again, path) {
				hold.close()
				last = fmt.Errorf("the mapped image name changed from %s to %s while it was acquired", filepath.Base(path), filepath.Base(again))
			} else {
				if acquireSeamBeforeObjectCheck != nil {
					acquireSeamBeforeObjectCheck(attempt)
				}
				if why := heldFileProblem(hold, path); why != "" {
					hold.close()
					last = fmt.Errorf("%s", why)
				} else if err := namespaceCheck(); err != nil {
					hold.close()
					return MappedImage{}, err
				} else {
					return MappedImage{PID: pid, Created: c.Nanoseconds(), Path: path, Located: located, hold: hold, stamp: stamp, bindStart: bindStart}, nil
				}
			}
		}
		if wait, werr := syscall.WaitForSingleObject(h, 0); werr != nil || wait != syscall.WAIT_TIMEOUT {
			return MappedImage{}, inconclusiveKind(ErrProcessExited, "process exited while its image was acquired")
		}
	}
	return MappedImage{}, inconclusiveKind(ErrModuleNotReady, "the image could not be acquired consistently while the process is alive: %v", last)
}

// heldFileProblem is "" when the held file is still the file the current path reaches and is still a
// file; otherwise why not. The current path is opened without content access and with full sharing
// and its volume and file ID must be the held handle's (a name that returned to its old place
// pointing at another object is a mismatch); the held file must not be delete-pending and must keep a link.
func heldFileProblem(f *fileHold, path string) string {
	var why string
	if err := f.use(func(h syscall.Handle) error { why = heldHandleProblem(h, path); return nil }); err != nil {
		return err.Error()
	}
	return why
}

func heldHandleProblem(h syscall.Handle, path string) string {
	held, err := stampOf(h)
	if err != nil {
		return "the held file cannot be examined: " + err.Error()
	}
	current, err := Locate(path)
	if err != nil {
		return "the current " + filepath.Base(path) + " cannot be examined: " + err.Error()
	}
	if current.VolumeSerial != held.Volume || current.FileIndex != held.FileIndex {
		return "the name " + filepath.Base(path) + " now reaches another file object than the one held"
	}
	links, pending, err := standardInfoOf(h)
	if err != nil {
		return err.Error()
	}
	if pending || links == 0 {
		return "the held file is deleted or delete-pending"
	}
	return ""
}

// postHashSeam, set only by tests, runs after the hash and before the post-hash consistency check.
var postHashSeam func()

// resultSeam, set only by tests, runs as the last step of a hash, before the change signal is read.
var resultSeam func()

// confirmStillMapped is the check after the hash. The change signal (when one is raised) aborts
// early. While the process is alive, the name it maps now (which may differ from the acquisition's
// after an ordinary rename of the running file) must reach the held file object: PostCheckAliveVerified.
// A process that already exited cannot be compared any more: the result is
// PostCheckExitedBeforeCheck, a candidate link that only a final judge holding a drained, quiet
// namespace record for the step may accept. A wait or name read that fails for another reason is INCONCLUSIVE.
func (f *fileHold) confirmStillMapped() (postCheck, mapped string, err error) {
	if postHashSeam != nil {
		postHashSeam()
	}
	if err := namespaceCheck(); err != nil {
		return "", "", err
	}
	postCheck = PostCheckAliveVerified
	var result error
	useErr := f.use(func(h syscall.Handle) error {
		if f.proc == 0 {
			postCheck = PostCheckExitedBeforeCheck
			return nil
		}
		wait, werr := syscall.WaitForSingleObject(f.proc, 0)
		if werr != nil {
			result = inconclusive("the process cannot be waited on after the hash: %v", werr)
			return nil
		}
		if wait != syscall.WAIT_TIMEOUT {
			postCheck = PostCheckExitedBeforeCheck
			return nil
		}
		now, err := mappedImagePath(f.proc)
		if err != nil {
			if errors.Is(err, ErrProcessExited) {
				postCheck = PostCheckExitedBeforeCheck
			} else {
				result = inconclusive("the mapped image name cannot be read after the hash: %v", err)
			}
			return nil
		}
		if why := heldHandleProblem(h, now); why != "" {
			result = inconclusiveKind(ErrModuleNotReady, "%s", why)
		} else {
			mapped = now
		}
		return nil
	})
	if useErr != nil {
		return "", "", inconclusive("%v", useErr)
	}
	return postCheck, mapped, result
}

// acquireConsistencyAttempts bounds the retries of an acquisition whose name moved under it.
const acquireConsistencyAttempts = 5

// acquireSeam, set only by tests, runs between the mapped name read and the open of the file.
var acquireSeam func(attempt int)

// acquireSeamAfterHold, set only by tests, runs after the file is held and its image section exists,
// before the process is checked alive and the mapped name is read the second time.
var acquireSeamAfterHold func(attempt int)

// acquireSeamBeforeObjectCheck, set only by tests, runs after the second mapped-name read and before the
// current path is compared with the held object.
var acquireSeamBeforeObjectCheck func(attempt int)

// hash computes the SHA256 of the image through the handle held since acquisition (the process was
// alive then) and requires the change stamp to be unchanged: a write since acquisition, even one
// that restores the bytes, is ErrNativeChanged. The process need not be alive.
func (m MappedImage) hash() (Identity, string, int64, error) {
	if m.hold == nil {
		return Identity{}, "", 0, inconclusive("no acquired image")
	}
	return hashHeld(m.hold, m.stamp, m.Path)
}

// Verify hashes the mapped image and requires it to be want (volume, file ID, size and SHA256).
func (m MappedImage) Verify(want Identity) (NativeImage, error) {
	defer m.Close()
	if want.SHA256 == "" {
		return NativeImage{}, inconclusive("no hashed expected image")
	}
	hashed, postCheck, bindEnd, err := m.hash()
	if err != nil {
		return NativeImage{}, err
	}
	if !SameImage(hashed, want) {
		return NativeImage{PID: m.PID, Created: m.Created, Identity: hashed, SectionHeldMS: m.heldMS(), PostCheck: postCheck, Acquirer: currentAcquirer(), BindStart: m.bindStart, BindEnd: bindEnd, AcquiredPath: m.acquiredPathIfMoved(hashed)}, fmt.Errorf("%w: process %d runs %s, not the expected bytes", ErrNativeChanged, m.PID, filepath.Base(m.Path))
	}
	return NativeImage{PID: m.PID, Created: m.Created, Identity: hashed, SectionHeldMS: m.heldMS(), PostCheck: postCheck, Acquirer: currentAcquirer(), BindStart: m.bindStart, BindEnd: bindEnd, AcquiredPath: m.acquiredPathIfMoved(hashed)}, nil
}

// Hashed hashes the mapped image and closes it, without naming a version. It is the step to take
// right after AcquireMappedImage: the hold ends shortly after the process does.
func (m MappedImage) Hashed() (NativeImage, error) {
	defer m.Close()
	hashed, postCheck, bindEnd, err := m.hash()
	if err != nil {
		return NativeImage{}, err
	}
	return NativeImage{PID: m.PID, Created: m.Created, Identity: hashed, SectionHeldMS: m.heldMS(), PostCheck: postCheck, Acquirer: currentAcquirer(), BindStart: m.bindStart, BindEnd: bindEnd, AcquiredPath: m.acquiredPathIfMoved(hashed)}, nil
}

// NameVersion names the version of a hashed image through the catalog (unknown bytes are INCONCLUSIVE).
func (n NativeImage) NameVersion(catalog *Catalog) (NativeImage, error) {
	version, ok := catalog.Version(n.Identity.SHA256)
	if !ok {
		return NativeImage{}, inconclusive("image %s is not in the version catalog", filepath.Base(n.Identity.Path))
	}
	n.Version = version
	return n, nil
}

// Identify hashes the mapped image and names its version through the catalog (unknown bytes are
// INCONCLUSIVE). Like Verify, it does not need the process alive after AcquireMappedImage, but the
// hold ends shortly (postExitGrace) after the process does.
func (m MappedImage) Identify(catalog *Catalog) (NativeImage, error) {
	img, err := m.Hashed()
	if err != nil {
		return NativeImage{}, err
	}
	return img.NameVersion(catalog)
}

// ImageOfProcess is AcquireMappedImage followed by Identify: the image a process runs, read
// alive and hashed afterwards, with its catalog version.
func ImageOfProcess(pid uint32, created int64, catalog *Catalog) (NativeImage, error) {
	m, err := AcquireMappedImage(pid, created)
	if err != nil {
		return NativeImage{}, err
	}
	return m.Identify(catalog)
}

// LinkSpawned links the process pid to the exact image it runs and requires that image to be
// want, an identity observed (with its SHA256) before the process was created. Nothing is
// inherited from want: the SHA256 of the running image is computed from the file the process maps
// (AcquireMappedImage while alive, then hashed through a fully shared handle, which does not need
// the process to survive). A process gone before its mapped image was read is
// ErrInconclusiveNativeIdentity (ErrProcessExited); one that runs another file or other bytes is
// ErrNativeChanged.
func LinkSpawned(pid uint32, created int64, want Identity) (NativeImage, error) {
	if pid == 0 || want.SHA256 == "" {
		return NativeImage{}, inconclusive("no process or no hashed expected image")
	}
	m, err := AcquireMappedImage(pid, created)
	if err != nil {
		return NativeImage{}, err
	}
	return m.Verify(want)
}
