//go:build windows

package providercert

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// The image section: how the content of the image is protected until it is hashed.
//
// While a process runs, Windows refuses writes to its image file because the image is mapped. That
// protection ends when the process exits. To keep it for the short time the hash needs, the file
// that was opened while the process was ALIVE gets its own image section (CreateFileMapping with
// SEC_IMAGE | PAGE_READONLY, the same kind of section the loader created), and the section is kept
// open until the hash is done. While an image section exists Windows refuses every write to the file
// (a new handle with write access is a sharing violation, so neither WriteFile nor a writable mapped
// view can be made) and refuses deleting it, and refuses a rename-REPLACE over it, but it allows
// moving it to another name. So an ordinary activation (move the old launcher aside, install the new
// one at the freed name) is not blocked; a replace of the held file itself and a delete of it are, for
// as long as the section lives. No version is pinned: the sharing mode of the handle stays
// READ|WRITE|DELETE, and the hold is bounded three ways, whichever comes first: the owner's close,
// maxSectionHold from before CreateFileMapping (the section handle is closed by the timer without
// waiting for any file I/O), and postExitGrace after the process exits (a watcher on the process).
// A hash that has not finished by then is INCONCLUSIVE, and anything computed after the deadline is
// discarded. Measured on a synthetic image, see TestImageSectionProtectsTheImageAfterExit,
// TestSectionIsReleasedOnTimeWhileAReadIsBlocked and TestSectionEndsShortlyAfterTheProcessExits.
//
// Name checks and what they prove. At acquisition: the mapped name is read, the file opened, the name
// read again, and the current path must reach the held object, which must not be delete-pending.
// After the hash, while the process is alive: the name the process maps NOW (which may differ after an
// ordinary rename of the running file) must reach the held object. These cannot be atomic from user
// mode: the name lookups are separate calls, and any finite number of them can be defeated by enough
// overlapping ORDINARY updater activations (each doing only its own backup, install and rollback). So
// this package does not claim the binding is sound; it emits EVIDENCE: every link records PostCheck,
// "alive-verified" (the check after the hash was made on a live process) or "exited-before-check" (the
// process ended first, nothing was left to re-compare, only the acquisition checks stand). Whether the
// names were stable for the whole step is the trusted parent harness's proof (a drained, quiet record of
// the native-root namespace watch), and an exited-before-check link may be accepted by the final judge
// only together with that record. The watch's signal file (CLAUDUCT_NATIVE_STOP_SIGNAL) is read here
// only as an early abort: a raised signal blocks the link at acquisition and again as the very last
// step before a result is returned; its absence proves nothing, and Go never concludes from the
// variable that a watch ran. Closed by the checks alone: single rename or replace, the updater's
// rollback, delete-based ABA, writes after exit. Not closed by them alone: an arbitrary number of
// overlapping ordinary activations, a deliberate choreographed sequence of renames timed to the
// checks' own reads, and a deliberate privileged actor.
//
// Updater interactions (the native updater replaces a version with rename-and-replace and retries a
// busy replace after 100, 500 and 2000 ms, attempts at 0, 100, 600 and 2600 ms; cleaning up old files
// is a delete that is refused while a section exists). The updater fails only if its FINAL retry
// fails, and that retry comes 2000 ms after the previous one. The hold is
// LOGICALLY over at ExitTime + postExitGrace (250 ms: it covers the real 295 MB native's ~118 ms hash
// plus queueing) and is released as promptly as the scheduler allows, so the harness does not keep
// the file busy longer than that past the native's lifetime, up to scheduling delay (measured,
// not guaranteed: a stalled host can delay the physical CloseHandle). The residual is exactly this: the final retry of a same-version reinstall falls within
// 250 ms after that version's native exits (exit later than final-250 ms). While the native still
// runs, its own image mapping refuses the replace with or without us. The harness's own natives run
// with DISABLE_AUTOUPDATER=1, so only a concurrent updater elsewhere can be affected.
//
// The section is created while the process is alive and the process is checked alive afterwards, so
// the protection has no gap between the process's own image mapping and ours.

// maxSectionHold bounds the whole life of one image section, from before CreateFileMapping to
// CloseHandle. It is a variable only so a test can shorten it.
var maxSectionHold = 2 * time.Second

// ChangeStamp is read through the held handle at acquisition and again after hashing: the file
// object (volume, file ID), its size and attributes, and the NTFS LastWriteTime and ChangeTime
// (FILE_BASIC_INFO). It is an extra DIAGNOSTIC, not a proof of content. A write through a writable
// mapped view can leave every one of these unchanged (observed), and a deliberate privileged actor can
// restore FILE_BASIC_INFO; neither is what the stamp is for. Content immutability until the hash is
// established by the image section above, which makes the writes impossible, not detectable.
//
// Threat model, stated precisely: this harness defends against an ACCIDENTAL change of the native
// (the updater's rename and replace, ordinary writes, an in-place rewrite by a tool), not against a
// deliberate privileged actor, who is outside it (a documented residual, not a proof).
type ChangeStamp struct {
	Volume        uint32
	FileIndex     uint64
	Size          int64
	Attributes    uint32
	LastWriteTime int64
	ChangeTime    int64
}

var (
	getFileInformationByHandleEx = kernelOwner.NewProc("GetFileInformationByHandleEx")
	createFileMappingW           = kernelOwner.NewProc("CreateFileMappingW")
)

const (
	fileBasicInfoClass    = 0
	fileStandardInfoClass = 1
	pageReadonly          = 0x02
	secImage              = 0x1000000
)

type fileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

// fileStandardInfo is FILE_STANDARD_INFO.
type fileStandardInfo struct {
	AllocationSize int64
	EndOfFile      int64
	NumberOfLinks  uint32
	DeletePending  byte
	Directory      byte
	_              [2]byte
}

func stampOf(h syscall.Handle) (ChangeStamp, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return ChangeStamp{}, err
	}
	var basic fileBasicInfo
	r, _, err := getFileInformationByHandleEx.Call(uintptr(h), fileBasicInfoClass, uintptr(unsafe.Pointer(&basic)), unsafe.Sizeof(basic))
	if r == 0 {
		return ChangeStamp{}, fmt.Errorf("file times unavailable: %v", err)
	}
	return ChangeStamp{
		Volume: info.VolumeSerialNumber, FileIndex: fileIndex(info), Size: int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow),
		Attributes: info.FileAttributes, LastWriteTime: basic.LastWriteTime, ChangeTime: basic.ChangeTime,
	}, nil
}

// standardInfoOf reads FILE_STANDARD_INFO of a held file: a file that is delete-pending or has no
// links left is no longer a file any name can reach.
func standardInfoOf(h syscall.Handle) (links uint32, deletePending bool, err error) {
	var std fileStandardInfo
	r, _, e := getFileInformationByHandleEx.Call(uintptr(h), fileStandardInfoClass, uintptr(unsafe.Pointer(&std)), unsafe.Sizeof(std))
	if r == 0 {
		return 0, false, fmt.Errorf("standard info unavailable: %v", e)
	}
	return std.NumberOfLinks, std.DeletePending != 0, nil
}

// fileHold keeps one file open (READ|WRITE|DELETE sharing: nothing is locked or pinned) and its
// image section. Ownership is split so that the section's end never waits on file I/O:
//   - the SECTION handle is closed by whoever expires the hold (the lifetime timer, the process-exit
//     watcher, or the owner) under secMu, a lock held only for the swap of the handle, never across a
//     system call that can block; closing a section does not disturb a ReadFile in flight on the
//     separate file handle;
//   - the FILE handle (and the process handle duplicate) are used and closed under mu, by the owner
//     (Verify, Identify, Close) or, when nobody is using them, by the expiry (TryLock). A reader that is
//     inside a slow call when the hold expires finds `expired` set when the call returns, discards its
//     result and closes the file itself.
//
// The section ends at the earliest of: the owner's close, maxSectionHold after it was created, and the
// ABSOLUTE instant ExitTime + postExitGrace, where ExitTime is the exit time the system reports for
// the process (GetProcessTimes), not the moment this code noticed the exit. The physical release is
// carried out by a watcher or a timer goroutine and so is as prompt as the scheduler lets it be; the
// logical rule is stricter: any use of the hold, and the acceptance of any result, re-checks both
// deadlines itself, so nothing computed after a deadline is accepted whatever those goroutines did.
type fileHold struct {
	mu        sync.Mutex // serializes every use of the file and process handles with their close
	h         syscall.Handle
	proc      syscall.Handle // a duplicate of the acquisition's process handle, for the post-hash check
	closed    bool
	timer     *time.Timer
	sectionAt time.Time
	deadline  time.Time
	stamp     ChangeStamp // the stamp at acquisition: the key of the shared hash
	shared    *sharedHash

	secMu   sync.Mutex // guards section and stop only
	section syscall.Handle
	stop    syscall.Handle // event that ends the exit watcher
	stopped bool

	expired      atomic.Bool
	exitDeadline atomic.Int64 // UnixNano of (the process's actual ExitTime + postExitGrace); 0 while unknown
}

// postExitGrace is how long the section may outlive the process it was taken for. The native
// updater retries a busy replace at gaps of 100, 500 and 2000 ms, so the grace must be well below the
// final retry: a hold never keeps the file locked more than 250 ms past the native's own exit
// (long enough for the ~118 ms hash of the real native plus queueing), which can only matter to a
// final retry that falls inside those 250 ms. A hash that has not
// finished by then is INCONCLUSIVE.
var postExitGrace = 250 * time.Millisecond

var (
	createEventW           = kernelOwner.NewProc("CreateEventW")
	setEvent               = kernelOwner.NewProc("SetEvent")
	waitForMultipleObjects = kernelOwner.NewProc("WaitForMultipleObjects")
	errHoldReleased        = ErrHoldExpired
	heldReadSeam           func() // test seam: inside a chunk read, under the I/O lock
)

// postExitDeadline is the absolute instant after which a hold taken for a process that exited at
// exit may not be used: the exit time reported by the system plus the grace, whatever the delay in
// observing the exit.
func postExitDeadline(exit time.Time) time.Time { return exit.Add(postExitGrace) }

// holdExpired is the pure deadline rule: past the lifetime deadline, or past the post-exit deadline.
func holdExpired(now, lifetime time.Time, exitDeadline int64) bool {
	return (!lifetime.IsZero() && !now.Before(lifetime)) || (exitDeadline != 0 && now.UnixNano() >= exitDeadline)
}

// expiredNow is true once the hold was expired or a deadline has passed, whatever the timer did.
func (f *fileHold) expiredNow() bool {
	return f.expired.Load() || holdExpired(time.Now(), f.deadline, f.exitDeadline.Load())
}

// processExitTime reads the actual exit time of a process (GetProcessTimes ExitTime); false while it runs.
func processExitTime(process syscall.Handle) (time.Time, bool) {
	var c, e, k, u syscall.Filetime
	if r, _, _ := getProcessTimesOwn.Call(uintptr(process), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return time.Time{}, false
	}
	if e.HighDateTime == 0 && e.LowDateTime == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, e.Nanoseconds()), true
}

// noteExit records the absolute post-exit deadline as soon as the exit is seen by any path (the
// watcher, or a use of the hold), so the acceptance of a result never waits for the watcher.
func (f *fileHold) noteExit(process syscall.Handle) bool {
	t, ok := processExitTime(process)
	if !ok {
		return false
	}
	f.exitDeadline.CompareAndSwap(0, postExitDeadline(t).UnixNano())
	return true
}

// closeSection closes the section handle if still open. It takes only secMu.
func (f *fileHold) closeSection() {
	f.secMu.Lock()
	s := f.section
	f.section = 0
	f.secMu.Unlock()
	if s != 0 {
		syscall.CloseHandle(s)
		// The shared hash counts LIVE SECTIONS: this one is gone, wherever it was closed (the owner,
		// the timer, the exit watcher), whether or not the owner is still inside a call.
		releaseSharedHash(f.stamp, f.shared)
	}
}

// signalStop ends the exit watcher, if one runs.
func (f *fileHold) signalStop() {
	f.secMu.Lock()
	if f.stop != 0 && !f.stopped {
		setEvent.Call(uintptr(f.stop))
	}
	f.secMu.Unlock()
}

// expire ends the section now, without waiting for any file I/O, marks the hold expired, and
// closes the file too when no one is using it.
func (f *fileHold) expire() {
	f.expired.Store(true)
	f.closeSection()
	f.signalStop()
	if f.mu.TryLock() {
		f.closeLocked()
		f.mu.Unlock()
	}
}

func (f *fileHold) closeLocked() {
	if f.closed {
		return
	}
	f.closed = true
	if f.timer != nil {
		f.timer.Stop()
	}
	f.closeSection()
	f.signalStop()
	syscall.CloseHandle(f.h)
	if f.proc != 0 {
		syscall.CloseHandle(f.proc)
		f.proc = 0
	}
}

func (f *fileHold) close() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeLocked()
}

// use runs fn with the file handle while the hold is open and unexpired. A hold that expired while fn
// ran is closed and fn's result is discarded (errHoldReleased): nothing computed after the deadline
// is accepted.
func (f *fileHold) use(fn func(h syscall.Handle) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errHoldReleased
	}
	if f.proc != 0 {
		f.noteExit(f.proc)
	}
	if f.expiredNow() {
		f.closeLocked()
		return errHoldReleased
	}
	err := fn(f.h)
	if f.proc != 0 {
		f.noteExit(f.proc)
	}
	if f.expiredNow() {
		f.closeLocked()
		return errHoldReleased
	}
	return err
}

// attachProcessSeam, set only by tests, runs after the handles for attachProcess are prepared and
// before the hold's lock is taken to publish them.
var attachProcessSeam func(check, watch, event syscall.Handle)

// exitObservedSeam, set only by tests, runs in the watcher after it saw the process exit and before
// it computes the remaining time: an injected observation delay.
var exitObservedSeam func()

// attachProcess gives the hold two duplicates of the acquisition's process handle (one for the
// post-hash check, one for the exit watcher) and an event that stops the watcher. Preparing them and
// publishing them is one step under the hold's lock: a hold that is closed or expired by then takes
// nothing, and every prepared handle is closed.
func (f *fileHold) attachProcess(process syscall.Handle) error {
	self, _ := syscall.GetCurrentProcess()
	var check, watch syscall.Handle
	if err := syscall.DuplicateHandle(self, process, self, &check, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		return err
	}
	if err := syscall.DuplicateHandle(self, process, self, &watch, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		syscall.CloseHandle(check)
		return err
	}
	ev, _, e := createEventW.Call(0, 1, 0, 0)
	if ev == 0 {
		syscall.CloseHandle(check)
		syscall.CloseHandle(watch)
		return e
	}
	if attachProcessSeam != nil {
		attachProcessSeam(check, watch, syscall.Handle(ev))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || f.expiredNow() {
		syscall.CloseHandle(check)
		syscall.CloseHandle(watch)
		syscall.CloseHandle(syscall.Handle(ev))
		return errHoldReleased
	}
	f.proc = check
	f.secMu.Lock()
	f.stop = syscall.Handle(ev)
	f.secMu.Unlock()
	go f.watchExit(watch, syscall.Handle(ev))
	return nil
}

// watchExit expires the hold at the process's ACTUAL exit time plus postExitGrace (an absolute
// instant, not a fresh relative wait started when the exit was noticed), unless the hold ends first.
func (f *fileHold) watchExit(process, stop syscall.Handle) {
	defer func() {
		f.secMu.Lock()
		f.stopped = true
		f.secMu.Unlock()
		syscall.CloseHandle(process)
		syscall.CloseHandle(stop)
	}()
	handles := [2]syscall.Handle{process, stop}
	r, _, _ := waitForMultipleObjects.Call(2, uintptr(unsafe.Pointer(&handles[0])), 0, uintptr(syscall.INFINITE))
	if r != uintptr(syscall.WAIT_OBJECT_0) { // the hold ended first
		return
	}
	if exitObservedSeam != nil {
		exitObservedSeam()
	}
	if !f.noteExit(process) { // the exit time cannot be read: fail closed
		f.expire()
		return
	}
	remaining := time.Until(time.Unix(0, f.exitDeadline.Load()))
	if remaining <= 0 {
		f.expire()
		return
	}
	ms := uint32((remaining + time.Millisecond - 1) / time.Millisecond)
	if ev, _ := syscall.WaitForSingleObject(stop, ms); ev == syscall.WAIT_TIMEOUT {
		f.expire()
	}
}

// imageSection creates the image section on the held handle and starts its lifetime. The lifetime
// starts before CreateFileMapping, so a slow creation counts against it.
func (f *fileHold) imageSection() error {
	if f.expired.Load() {
		return errHoldReleased
	}
	start := time.Now()
	r, _, err := createFileMappingW.Call(uintptr(f.h), 0, uintptr(pageReadonly|secImage), 0, 0, 0)
	if r == 0 {
		return fmt.Errorf("image section: %v", err)
	}
	f.secMu.Lock()
	f.section, f.sectionAt, f.deadline = syscall.Handle(r), start, start.Add(maxSectionHold)
	if f.stamp != (ChangeStamp{}) {
		f.shared = joinSharedHash(f.stamp) // joined with the section, in one step: the count is of live sections
	}
	f.secMu.Unlock()
	f.mu.Lock()
	f.timer = time.AfterFunc(maxSectionHold-time.Since(start), f.expire)
	f.mu.Unlock()
	return nil
}

// holdFile opens path with full sharing, requires it to be the file object located (volume, file
// ID, size) and takes its change stamp.
func holdFile(path string, located Identity) (*fileHold, ChangeStamp, error) {
	h, err := open(path, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE)
	if err != nil {
		return nil, ChangeStamp{}, inconclusive("open %s: %v", filepath.Base(path), err)
	}
	stamp, err := stampOf(h)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, ChangeStamp{}, inconclusive("stamp %s: %v", filepath.Base(path), err)
	}
	if stamp.Volume != located.VolumeSerial || stamp.FileIndex != located.FileIndex || stamp.Size != located.Size {
		syscall.CloseHandle(h)
		return nil, ChangeStamp{}, fmt.Errorf("%w: the file behind the image name changed while it was acquired", ErrNativeChanged)
	}
	return &fileHold{h: h, stamp: stamp}, stamp, nil
}

// wallNow is the clock of the binding interval: the system wall clock in UTC as Unix nanoseconds.
// On Windows Go reads GetSystemTimePreciseAsFileTime, a FILETIME (100 ns ticks since 1601-01-01 UTC)
// converted by the runtime to Unix nanoseconds, so the resolution is 100 ns (the precise call is
// typically microsecond-accurate). The harness's .NET DateTime.UtcNow.Ticks (100 ns since 0001-01-01)
// converts as (ticks - 621355968000000000) * 100; a FILETIME converts as (ft - 116444736000000000) * 100.
func wallNow() int64 { return time.Now().UnixNano() }

// afterSignalSeam, set only by tests, runs after the last change-signal read, before the final expiry check.
var afterSignalSeam func()

// hashHeld hashes the whole file through the held handle (the handle that was opened while the
// process was alive, so a rename of the NAME afterwards is irrelevant), within maxSectionHold, and
// cross-checks the stamp: a difference is ErrNativeChanged (a diagnostic; the section already made
// writes impossible), one that cannot be read is INCONCLUSIVE.
func hashHeld(f *fileHold, before ChangeStamp, path string) (Identity, string, int64, error) {
	sum, n, err := sha256Held(f)
	if err != nil {
		return Identity{}, "", 0, inconclusive("hash %s: %v", filepath.Base(path), err)
	}
	var after ChangeStamp
	err = f.use(func(h syscall.Handle) (e error) { after, e = stampOf(h); return })
	if err != nil {
		return Identity{}, "", 0, inconclusive("stamp %s after hashing: %v", filepath.Base(path), err)
	}
	// The diagnostic compares the file object, size and LastWriteTime. ChangeTime and attributes are
	// recorded but not compared: the updater's rename (allowed, and the whole point of not pinning)
	// moves ChangeTime without touching the content.
	if after.Volume != before.Volume || after.FileIndex != before.FileIndex || after.Size != before.Size || after.LastWriteTime != before.LastWriteTime {
		return Identity{}, "", 0, fmt.Errorf("%w: the image file was modified after it was acquired", ErrNativeChanged)
	}
	if n != before.Size {
		return Identity{}, "", 0, fmt.Errorf("%w: read %d of %d bytes", ErrNativeChanged, n, before.Size)
	}
	// Second consistency check, still under the section and while the process is alive: the name the
	// process maps NOW must reach the held file object, which must still be a file. Concurrent
	// activations of the updater can move names between the acquisition checks and the end of the
	// hash; a mismatch is never linked.
	postCheck, mapped, err := f.confirmStillMapped()
	if err != nil {
		return Identity{}, "", 0, err
	}
	if f.expiredNow() {
		return Identity{}, "", 0, inconclusiveKind(ErrHoldExpired, "the image hold expired before the hash was accepted")
	}
	// The change signal is read last: a raised signal blocks the result, an absent one proves nothing.
	if resultSeam != nil {
		resultSeam()
	}
	if err := namespaceCheck(); err != nil {
		return Identity{}, "", 0, err
	}
	if afterSignalSeam != nil {
		afterSignalSeam()
	}
	// The hold may have expired while the signal was read: nothing is accepted after either deadline.
	// use() records the process exit and checks both deadlines under the hold's lock.
	if err := f.use(func(syscall.Handle) error { return nil }); err != nil || f.expiredNow() {
		return Identity{}, "", 0, inconclusiveKind(ErrHoldExpired, "the image hold expired while the change signal was read")
	}
	// The name the process maps NOW (verified by the post-hash check) is the identity's path; the
	// acquisition-time name is kept on the NativeImage (AcquiredPath) when it differs.
	if mapped != "" {
		path = mapped
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Identity{}, "", 0, err
	}
	// The end of the binding interval: the wall clock read after the last signal check, immediately
	// before success is returned.
	return Identity{Path: abs, VolumeSerial: before.Volume, FileIndex: before.FileIndex, Size: before.Size, SHA256: sum}, postCheck, wallNow(), nil
}

// heldReader reads the held file in chunks; each chunk is read under the hold's lock, only while
// the hold is open and unexpired, and a chunk that finishes after the deadline is discarded.
type heldReader struct {
	f        *fileHold
	deadline time.Time
}

func (d heldReader) Read(b []byte) (int, error) {
	if !time.Now().Before(d.deadline) {
		return 0, fmt.Errorf("%w: the image section was held longer than %s", ErrHoldExpired, maxSectionHold)
	}
	if len(b) > 1<<20 {
		b = b[:1<<20]
	}
	var n int
	err := d.f.use(func(h syscall.Handle) (e error) {
		if heldReadSeam != nil {
			heldReadSeam()
		}
		n, e = handleReader(h).Read(b)
		return
	})
	return n, err
}

// hashRunSeam, set only by tests, runs each time a hold really reads and hashes the file.
var hashRunSeam func()

// sharedHash is one hash per file object among concurrent holds. A hold joins the entry for its
// acquisition stamp (volume, file ID, size, attributes, LastWriteTime, ChangeTime) once its own image
// section exists, and the entry lives while at least one such hold lives. Writes to the file are
// refused for as long as any one section exists, so every join happens while the content has been
// protected continuously since the entry began, and an equal stamp means the same bytes; N concurrent
// generations of the same 295 MB file hash it once. A different file object, or a stamp that moved,
// is a different key and never shares. A failed or expired hash is not shared: a waiter hashes itself.
type sharedHash struct {
	mu        sync.Mutex
	refs      int
	computing bool
	retired   bool // the last live section ended: a done result is never handed out again
	done      bool
	sum       string
	n         int64
	wake      chan struct{}
}

var sharedHashes = struct {
	mu sync.Mutex
	m  map[ChangeStamp]*sharedHash
}{m: map[ChangeStamp]*sharedHash{}}

func joinSharedHash(key ChangeStamp) *sharedHash {
	sharedHashes.mu.Lock()
	defer sharedHashes.mu.Unlock()
	e := sharedHashes.m[key]
	if e == nil {
		e = &sharedHash{wake: make(chan struct{})}
		sharedHashes.m[key] = e
	}
	e.refs++
	return e
}

func releaseSharedHash(key ChangeStamp, e *sharedHash) {
	if e == nil {
		return
	}
	sharedHashes.mu.Lock()
	defer sharedHashes.mu.Unlock()
	if e.refs--; e.refs <= 0 {
		if sharedHashes.m[key] == e {
			delete(sharedHashes.m, key)
		}
		e.mu.Lock()
		e.retired = true
		e.mu.Unlock()
	}
}

func sha256Held(f *fileHold) (string, int64, error) {
	e := f.shared
	if e == nil {
		return hashOwn(f)
	}
	for {
		e.mu.Lock()
		if e.done && !e.retired && !f.expiredNow() {
			sum, n := e.sum, e.n
			e.mu.Unlock()
			return sum, n, nil
		}
		if !e.computing {
			e.computing = true
			e.mu.Unlock()
			sum, n, err := hashOwn(f)
			e.mu.Lock()
			e.computing = false
			if err == nil && !e.retired {
				e.done, e.sum, e.n = true, sum, n
			}
			ch := e.wake
			e.wake = make(chan struct{})
			e.mu.Unlock()
			close(ch)
			return sum, n, err
		}
		ch := e.wake
		e.mu.Unlock()
		wait := time.Until(f.deadline)
		if f.deadline.IsZero() {
			wait = maxSectionHold
		}
		select {
		case <-ch:
		case <-time.After(wait):
			return "", 0, fmt.Errorf("%w: waited for the same file's hash past this hold's lifetime", ErrHoldExpired)
		}
		if f.expiredNow() {
			return "", 0, errHoldReleased
		}
	}
}

func hashOwn(f *fileHold) (string, int64, error) {
	if hashRunSeam != nil {
		hashRunSeam()
	}
	since := f.sectionAt
	if since.IsZero() {
		since = time.Now() // a hold without a section (tests) is bounded from now
	}
	if err := f.use(func(h syscall.Handle) error { _, e := syscall.Seek(h, 0, io.SeekStart); return e }); err != nil {
		return "", 0, err
	}
	return sha256Of(heldReader{f: f, deadline: since.Add(maxSectionHold)})
}

func sha256Of(r io.Reader) (string, int64, error) {
	sum := sha256.New()
	n, err := io.CopyBuffer(sum, r, make([]byte, 1<<20))
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(sum.Sum(nil)), n, nil
}
