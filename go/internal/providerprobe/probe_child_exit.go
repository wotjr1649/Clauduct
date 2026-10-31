package providerprobe

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// p3ChildExitEvidence holds process identity and fate metadata only.
type p3ChildExitEvidence struct {
	ID                     string
	PID                    uint32
	Created                time.Time
	ObservedCreated        time.Time
	ExitAt                 time.Time
	HandleKnown            bool
	ParentKnown            bool
	ObservedParentPID      uint32
	ObservedParentCreated  time.Time
	ProbeCleanup, ProbeCap bool
}

type p3ChildExitWatch struct {
	sidecar  string
	cleanup  atomic.Int64
	capMu    sync.Mutex
	caps     map[p3ChildProcessKey]int64
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	current  map[string]p3ChildExitEvidence
	evidence []p3ChildExitEvidence
}

type p3ChildProcessKey struct {
	pid     uint32
	created int64
}

type p3WatchedChild struct {
	evidence              p3ChildExitEvidence
	expectedParentPID     uint32
	expectedParentCreated time.Time
	handle                syscall.Handle
	done                  bool
}

const p3ChildExitAccess = syscall.SYNCHRONIZE | 0x1000 // PROCESS_QUERY_LIMITED_INFORMATION

func p3WatchChildExits(sidecar string) *p3ChildExitWatch {
	w := &p3ChildExitWatch{sidecar: sidecar, stop: make(chan struct{}), done: make(chan struct{}), caps: map[p3ChildProcessKey]int64{}, current: map[string]p3ChildExitEvidence{}}
	go w.observe()
	return w
}

func (w *p3ChildExitWatch) markCleanup() { w.cleanup.CompareAndSwap(0, time.Now().UnixNano()) }

func (w *p3ChildExitWatch) markProbeCap(pid uint32, created time.Time) {
	if pid == 0 || created.IsZero() {
		return
	}
	w.capMu.Lock()
	w.caps[p3ChildProcessKey{pid: pid, created: created.UnixNano()}] = time.Now().UnixNano()
	w.capMu.Unlock()
}

func (w *p3ChildExitWatch) finish() []p3ChildExitEvidence {
	w.once.Do(func() { close(w.stop); <-w.done })
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]p3ChildExitEvidence(nil), w.evidence...)
}

func (w *p3ChildExitWatch) observe() {
	defer close(w.done)
	children := map[string]*p3WatchedChild{}
	executable := p3CurrentExecutablePath()
	defer func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, child := range children {
			if child.handle != 0 {
				_ = syscall.CloseHandle(child.handle)
				child.handle = 0
			}
			w.evidence = append(w.evidence, child.evidence)
		}
		sort.Slice(w.evidence, func(i, j int) bool { return w.evidence[i].ID < w.evidence[j].ID })
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		w.observeLifecycle(children, readWrapped(w.sidecar), executable)

		cleanupAt := w.cleanup.Load()
		for _, child := range children {
			if child.done {
				w.markCause(&child.evidence, cleanupAt)
				continue
			}
			w.observeExit(child)
			w.markCause(&child.evidence, cleanupAt)
		}
		w.publish(children)
		select {
		case <-w.stop:
			// One final read catches records published while shutdown began.
			w.observeLifecycle(children, readWrapped(w.sidecar), executable)
			cleanupAt := w.cleanup.Load()
			for _, child := range children {
				w.observeExit(child)
				w.markCause(&child.evidence, cleanupAt)
			}
			w.publish(children)
			return
		case <-ticker.C:
		}
	}
}

type p3ChildLifecycle struct {
	starts []p3WrapperRecord
	spawns []p3WrapperRecord
}

// observeLifecycle accepts only one wrapper start and one matching spawn record per ID. The
// sidecar is writable by the child, so a record alone must not redirect the retained handle to
// the wrapper or another process generation.
func (w *p3ChildExitWatch) observeLifecycle(children map[string]*p3WatchedChild, records []p3WrapperRecord, executable string) {
	lifecycles := map[string]*p3ChildLifecycle{}
	for _, record := range records {
		if record.Mode != p3HookChild || record.ID == "" {
			continue
		}
		lifecycle := lifecycles[record.ID]
		if lifecycle == nil {
			lifecycle = new(p3ChildLifecycle)
			lifecycles[record.ID] = lifecycle
		}
		switch record.Phase {
		case "start":
			lifecycle.starts = append(lifecycle.starts, record)
		case "spawned":
			lifecycle.spawns = append(lifecycle.spawns, record)
		}
	}
	for id, lifecycle := range lifecycles {
		if len(lifecycle.spawns) == 0 {
			continue
		}
		child, exists := children[id]
		if exists {
			if len(lifecycle.starts) != 1 || len(lifecycle.spawns) != 1 || !p3ChildSpawnMatches(lifecycle.starts[0], lifecycle.spawns[0]) {
				w.invalidate(child)
			}
			continue
		}
		spawn := lifecycle.spawns[0]
		child = &p3WatchedChild{evidence: p3ChildExitEvidence{ID: id, PID: uint32(max(spawn.ChildPID, 0)), Created: spawn.Created}}
		children[id] = child
		if len(lifecycle.starts) != 1 || len(lifecycle.spawns) != 1 || !p3ChildSpawnMatches(lifecycle.starts[0], spawn) {
			child.done = true
			continue
		}
		start := lifecycle.starts[0]
		child.expectedParentPID = uint32(start.PID)
		child.expectedParentCreated = start.Created
		w.observeGeneration(child, executable)
	}
}

func p3ChildSpawnMatches(start, spawn p3WrapperRecord) bool {
	maxPID := uint64(^uint32(0))
	return start.Mode == p3HookChild && start.Phase == "start" && start.ID != "" &&
		start.PID > 0 && uint64(start.PID) <= maxPID && !start.Created.IsZero() && !start.At.Before(start.Created) &&
		start.TimeoutMs == int(p3HookTimeout/time.Millisecond) &&
		spawn.Mode == p3HookChild && spawn.Phase == "spawned" && spawn.ID == start.ID &&
		spawn.Event == start.Event && spawn.TimeoutMs == start.TimeoutMs && spawn.Escape == start.Escape &&
		spawn.PID == start.PID && spawn.ChildPID > 0 && uint64(spawn.ChildPID) <= maxPID &&
		spawn.ChildPID != start.PID && !spawn.Created.IsZero()
}

func (w *p3ChildExitWatch) invalidate(child *p3WatchedChild) {
	child.evidence.ParentKnown = false
	child.done = true
	if child.handle != 0 {
		_ = syscall.CloseHandle(child.handle)
		child.handle = 0
	}
}

func (w *p3ChildExitWatch) observeGeneration(child *p3WatchedChild, executable string) {
	if child.evidence.Created.IsZero() {
		child.done = true
		return
	}
	handle, err := syscall.OpenProcess(p3ChildExitAccess, false, child.evidence.PID)
	if err != nil {
		child.done = true
		return
	}
	child.handle = handle
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		child.done = true
		_ = syscall.CloseHandle(handle)
		child.handle = 0
		return
	}
	child.evidence.ObservedCreated = time.Unix(0, created.Nanoseconds())
	child.evidence.HandleKnown = true
	if !child.evidence.ObservedCreated.Equal(child.evidence.Created) {
		child.done = true
		_ = syscall.CloseHandle(handle)
		child.handle = 0
		return
	}
	w.observeParent(child, executable)
	w.observeExit(child)
}

// observeParent independently binds the watched generation to its recorded wrapper. All
// serialized values are OS-observed PID/creation evidence; executable paths stay local.
func (w *p3ChildExitWatch) observeParent(child *p3WatchedChild, executable string) {
	parentPID := parentPID(child.evidence.PID)
	if parentPID == 0 {
		return
	}
	child.evidence.ObservedParentPID = parentPID
	parent, err := syscall.OpenProcess(p3ChildExitAccess, false, parentPID)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(parent)
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(parent, &created, &exited, &kernel, &user) != nil {
		return
	}
	child.evidence.ObservedParentCreated = time.Unix(0, created.Nanoseconds())
	if parentPID != child.expectedParentPID || !child.evidence.ObservedParentCreated.Equal(child.expectedParentCreated) ||
		!child.evidence.ObservedParentCreated.Before(child.evidence.ObservedCreated) || executable == "" {
		return
	}
	parentImage, err := imagePath(int(parentPID))
	if err != nil || !p3SameExecutable(parentImage, executable) {
		return
	}
	childImage, err := imagePath(int(child.evidence.PID))
	if err != nil || !p3SameExecutable(childImage, executable) {
		return
	}
	child.evidence.ParentKnown = true
}

func p3CurrentExecutablePath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return ""
	}
	return filepath.Clean(absolute)
}

func p3SameExecutable(image, executable string) bool {
	if image == "" || executable == "" {
		return false
	}
	imageInfo, imageErr := os.Stat(image)
	executableInfo, executableErr := os.Stat(executable)
	return imageErr == nil && executableErr == nil && os.SameFile(imageInfo, executableInfo)
}

func (w *p3ChildExitWatch) publish(children map[string]*p3WatchedChild) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, child := range children {
		w.current[id] = child.evidence
	}
}

func (w *p3ChildExitWatch) observeExit(child *p3WatchedChild) {
	if child.done || child.handle == 0 {
		return
	}
	state, err := syscall.WaitForSingleObject(child.handle, 0)
	if err != nil || state != syscall.WAIT_OBJECT_0 {
		return
	}
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(child.handle, &created, &exited, &kernel, &user) == nil {
		child.evidence.ObservedCreated = time.Unix(0, created.Nanoseconds())
		child.evidence.HandleKnown = true
		if child.evidence.ObservedCreated.Equal(child.evidence.Created) {
			child.evidence.ExitAt = time.Unix(0, exited.Nanoseconds())
		}
	}
	child.done = true
	_ = syscall.CloseHandle(child.handle)
	child.handle = 0
}

func (w *p3ChildExitWatch) markCause(e *p3ChildExitEvidence, cleanupAt int64) {
	cleanupBeforeExit := cleanupAt > 0 && (e.ExitAt.IsZero() || e.ExitAt.UnixNano() >= cleanupAt)
	if cleanupBeforeExit {
		e.ProbeCleanup = true
	}
	w.capMu.Lock()
	capAt, found := w.caps[p3ChildProcessKey{pid: e.PID, created: e.Created.UnixNano()}]
	w.capMu.Unlock()
	if found && (e.ExitAt.IsZero() || e.ExitAt.UnixNano() >= capAt) {
		e.ProbeCap = true
	}
}
