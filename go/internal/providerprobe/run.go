package providerprobe

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Role runs a fixture role the probe binary was started for (a hook wrapper, a helper
// variant), reporting whether argv asked for one. Call it first in main.
func Role(argv []string) (int, bool) { return p3Role(argv) }

// ProbeRun is one probe process's TB: temporary directories under its root, cleanups run in
// reverse at Close, and a fatal condition that ends the run with an error.
type ProbeRun struct {
	root     string
	log      io.Writer
	mu       sync.Mutex
	n        int
	cleanups []func()
	failed   []string
}

// fatal is what Fatal panics with; Do recovers it.
type fatal struct{ message string }

// NewRun makes a run rooted at root (created if missing), logging to log.
func NewRun(root string, log io.Writer) (*ProbeRun, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &ProbeRun{root: root, log: log}, nil
}

func (r *ProbeRun) Helper() {}

func (r *ProbeRun) TempDir() string {
	r.mu.Lock()
	r.n++
	dir := filepath.Join(r.root, strconv.Itoa(r.n))
	r.mu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		r.Fatalf("temp dir: %v", err)
	}
	r.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func (r *ProbeRun) Cleanup(f func()) {
	r.mu.Lock()
	r.cleanups = append(r.cleanups, f)
	r.mu.Unlock()
}

func (r *ProbeRun) Fatal(args ...any) { panic(fatal{fmt.Sprint(args...)}) }

func (r *ProbeRun) Fatalf(format string, args ...any) { panic(fatal{fmt.Sprintf(format, args...)}) }

func (r *ProbeRun) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.mu.Lock()
	r.failed = append(r.failed, msg)
	r.mu.Unlock()
	fmt.Fprintln(r.log, "error:", msg)
}

func (r *ProbeRun) Logf(format string, args ...any) { fmt.Fprintf(r.log, format+"\n", args...) }

// Do runs f, turning a Fatal into an error, then runs every cleanup.
func (r *ProbeRun) Do(f func() error) (err error) {
	defer func() {
		r.mu.Lock()
		cleanups := r.cleanups
		r.cleanups = nil
		r.mu.Unlock()
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}()
	defer func() {
		if v := recover(); v != nil {
			if f, ok := v.(fatal); ok {
				err = fmt.Errorf("PROBE_RUN_FAILED: %s", f.message)
				return
			}
			panic(v)
		}
	}()
	if err := f(); err != nil {
		return err
	}
	if len(r.failed) > 0 {
		return fmt.Errorf("PROBE_RUN_FAILED: %v", r.failed)
	}
	return nil
}
