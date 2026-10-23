//go:build windows

package providercert

import (
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// HashSourceStepManifest marks a link whose bytes were NOT hashed for this generation: it names the
// file object the process maps (volume, file ID, size) and the judge inherits the version and SHA256
// of the hashed step-manifest entry for exactly that file object, which holds only under a drained,
// quiet namespace record of the step. Identity.SHA256 is empty on such a link.
const HashSourceStepManifest = "step-manifest"

// FileObject is the file a process maps: the file ID is a decimal string (it can exceed 2^53).
type FileObject struct {
	Volume uint32 `json:"volume"`
	FileID string `json:"fileId"`
	Size   int64  `json:"size"`
}

var captureWait = syscall.WaitForSingleObject // test seam for acquisition wait failures
var captureSeam func(attempt int)             // test seam between the file identity reads

func captureAlive(h syscall.Handle) (bool, error) {
	switch wait, err := captureWait(h, 0); {
	case err != nil:
		return false, inconclusive("wait: %v", err)
	case wait == syscall.WAIT_OBJECT_0:
		return false, nil
	case wait == syscall.WAIT_TIMEOUT:
		return true, nil
	default:
		return false, inconclusive("wait result %d", wait)
	}
}

// CaptureFileObject is the fast link of an alive process: its mapped main-module name is read, the
// file that name reaches is located (attributes only, nothing is opened for content, nothing is hashed
// or held), and the name and the file object are read once more. The result carries HashSource
// "step-manifest", the FileObject, the mapped name, and PostCheck: "alive-verified" when the process
// was still alive after the second read, "exited-before-check" when it ended in between. Failures are
// the acquisition failures of AcquireMappedImage (exited, not ready, reused number, access); a name
// that does not settle is not-ready.
func CaptureFileObject(pid uint32, created int64) (NativeImage, error) {
	bindStart := wallNow()
	if pid == 0 {
		return NativeImage{}, inconclusive("no process")
	}
	h, err := syscall.OpenProcess(processQueryInformation|processVMRead|syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok && errno == 87 {
			return NativeImage{}, inconclusiveKind(ErrProcessExited, "open process %d: no such process", pid)
		}
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
	if alive, err := captureAlive(h); err != nil {
		return NativeImage{}, err
	} else if !alive {
		return NativeImage{}, inconclusiveKind(ErrProcessExited, "process exited before its image could be read")
	}
	var last error
	for attempt := 0; attempt < acquireConsistencyAttempts; attempt++ {
		mapped, path, err := mappedImageNames(h)
		if err != nil {
			return NativeImage{}, err
		}
		located, err := Locate(path)
		if err == nil && captureSeam != nil {
			captureSeam(attempt)
		}
		if err != nil {
			last = inconclusive("locate %s: %v", filepath.Base(path), err)
		} else if againMapped, again, err := mappedImageNames(h); err != nil {
			return NativeImage{}, err
		} else if !strings.EqualFold(againMapped, mapped) || !strings.EqualFold(again, path) {
			last = inconclusive("the mapped image name changed from %s to %s while it was read", filepath.Base(path), filepath.Base(again))
		} else if second, err := Locate(path); err != nil || second.VolumeSerial != located.VolumeSerial || second.FileIndex != located.FileIndex || second.Size != located.Size {
			last = inconclusive("the name %s reached another file object while it was read", filepath.Base(path))
		} else if err := namespaceCheck(); err != nil {
			return NativeImage{}, err
		} else {
			postCheck := PostCheckAliveVerified
			if alive, err := captureAlive(h); err != nil {
				return NativeImage{}, err
			} else if !alive {
				postCheck = PostCheckExitedBeforeCheck
			}
			located.SHA256 = ""
			return NativeImage{
				PID: pid, Created: c.Nanoseconds(), Identity: located, PostCheck: postCheck, Acquirer: currentAcquirer(),
				HashSource: HashSourceStepManifest, MappedName: mapped, ImagePath: path,
				FileObject: &FileObject{Volume: located.VolumeSerial, FileID: strconv.FormatUint(located.FileIndex, 10), Size: located.Size},
				BindStart:  bindStart, BindEnd: wallNow(),
			}, nil
		}
		if alive, err := captureAlive(h); err != nil {
			return NativeImage{}, err
		} else if !alive {
			return NativeImage{}, inconclusiveKind(ErrProcessExited, "process exited while its image was read")
		}
	}
	return NativeImage{}, inconclusiveKind(ErrModuleNotReady, "the image file could not be read consistently while the process is alive: %v", last)
}
