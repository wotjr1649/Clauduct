package providerprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3Background is the background entry (R08): native dispatches the turn to a worker under a
// supervisor that belongs to the probe's own config dir.
const p3Background = "background"

// p3Worker is the inference worker of one background job, held by a handle opened as soon as it
// was found, so the probe acts on that process and no other that later reuses its number.
type p3Worker struct {
	PID      int
	Image    string
	Identity providercert.Identity
	handle   syscall.Handle
}

// findWorker reads the job's worker from the job's own state in the probe's config dir (native's
// control commands are not used: with a stored login they contact that gateway themselves), opens
// it and confirms it runs the pinned native executable and started after the dispatch.
func findWorker(configDir, job string, pinned providercert.Identity, since time.Time, wait time.Duration) (*p3Worker, []string, error) {
	deadline := time.Now().Add(wait)
	var keys []string
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Join(configDir, "jobs", job, "state.json"))
		var state map[string]any
		if err == nil && json.Unmarshal(raw, &state) == nil {
			keys = keys[:0]
			pid := 0
			for key, value := range state {
				keys = append(keys, key)
				if n, ok := value.(float64); ok && strings.EqualFold(key, "workerPid") && n > 0 {
					pid = int(n)
				}
			}
			slices.Sort(keys)
			if pid > 0 {
				w, err := openWorker(pid, pinned, since)
				return w, keys, err
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, keys, fmt.Errorf("job %s names no worker in its state", job)
}

func openWorker(pid int, pinned providercert.Identity, since time.Time) (*p3Worker, error) {
	// PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE | PROCESS_TERMINATE.
	h, err := syscall.OpenProcess(0x1000|syscall.SYNCHRONIZE|syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return nil, fmt.Errorf("worker %d: %w", pid, err)
	}
	var created, exited, kernel, user syscall.Filetime
	err = syscall.GetProcessTimes(h, &created, &exited, &kernel, &user)
	if err == nil && time.Unix(0, created.Nanoseconds()).Before(since) {
		err = fmt.Errorf("worker %d started before this run", pid)
	}
	var image string
	var id providercert.Identity
	if err == nil {
		image, err = imagePath(pid)
	}
	if err == nil {
		if id, err = providercert.Observe(image); err == nil && id != pinned {
			err = fmt.Errorf("worker image is not the pinned native")
		}
	}
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	return &p3Worker{PID: pid, Image: image, Identity: id, handle: h}, nil
}

// exited waits up to wait for the worker to end.
func (w *p3Worker) exited(wait time.Duration) bool {
	state, err := syscall.WaitForSingleObject(w.handle, uint32(wait.Milliseconds()))
	return err == nil && state == syscall.WAIT_OBJECT_0
}

// end terminates the worker through the handle opened for it; only used when the product left
// it running, which the probe records as a cleanup failure.
func (w *p3Worker) end() {
	syscall.TerminateProcess(w.handle, 1)
	w.exited(5 * time.Second)
}

func (w *p3Worker) close() { syscall.CloseHandle(w.handle) }

// waitJobDone waits for the job's state to be done with a probe reply marker in its output, and
// returns that output.
func waitJobDone(configDir, job string, wait time.Duration) (string, bool) {
	deadline := time.Now().Add(wait)
	last := "no state file"
	defer func() { p3LastJobState = last }()
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Join(configDir, "jobs", job, "state.json"))
		var state struct {
			State  string
			Output json.RawMessage
		}
		if err == nil && json.Unmarshal(raw, &state) == nil {
			last = fmt.Sprintf("state %q, output %d bytes, marker %t", state.State, len(state.Output), bytes.Contains(state.Output, []byte("CLAUDUCT-PROBE-REPLY-")))
			if state.State == "done" && bytes.Contains(state.Output, []byte("CLAUDUCT-PROBE-REPLY-")) {
				return string(state.Output), true
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", false
}

// p3LastJobState is the last job state waitJobDone read, for the run's errors.
var p3LastJobState string

// p3BackgroundFamily is the background row of the runtime matrix measured so far (R08's first
// turn). Its respawn turn and R08b are not driven yet. The probe never stops a native daemon:
// whether native keeps one per config dir or one per user is not established, and a daemon the
// user runs must not be stopped. A job is stopped through its own resident only.
func p3BackgroundFamily() []p3Case {
	// A background dispatch needs the workspace already trusted (it cannot ask): the synthetic
	// profile is written as the release TUI harness writes it, with the stored gateway login.
	storedA := func(w *p3World) error {
		state, _ := json.Marshal(map[string]any{"hasCompletedOnboarding": true, "theme": "dark",
			"projects": map[string]any{filepath.ToSlash(w.cwd): map[string]any{"hasTrustDialogAccepted": true}}})
		if err := os.WriteFile(filepath.Join(w.configDir, ".claude.json"), state, 0o600); err != nil {
			return err
		}
		return w.storeGateway(w.sgA)
	}
	return []p3Case{
		{ID: "R08", Entry: p3Background, Hooks: p3HooksNormal, Setup: storedA, Expect: providercert.ExpectSuccess, Established: gotCanary("WRONG_STORED_GATEWAY_A", 0)},
	}
}

// findWorkerByCensus finds the job's worker among the probe config dir's natives: the
// --session-id process, opened at once and confirmed to run the pinned native and to have
// started after the dispatch.
func findWorkerByCensus(exe, configDir string, roles map[string]string, pinned providercert.Identity, since time.Time, wait time.Duration) (*p3Worker, error) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		for _, n := range probeNatives(exe, configDir, since, roles) {
			if n.Command == "--session-id" {
				return openWorker(n.PID, pinned, since)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("no worker of the probe config dir within %s", wait)
}

// p3DedicatedFamily is the dedicated-config feasibility row (decision 10): background in a
// synthetic config root of its own, no login at start.
func p3DedicatedFamily() []p3Case {
	trusted := func(w *p3World) error {
		state, _ := json.Marshal(map[string]any{"hasCompletedOnboarding": true, "theme": "dark",
			"projects": map[string]any{filepath.ToSlash(w.cwd): map[string]any{"hasTrustDialogAccepted": true}}})
		return os.WriteFile(filepath.Join(w.configDir, ".claude.json"), state, 0o600)
	}
	return []p3Case{
		{ID: "F1-empty", Entry: p3Background, Hooks: p3HooksNormal, Setup: trusted, Expect: providercert.ExpectSuccess, Established: func(*p3Observed) bool { return true }},
		// The first job's session resumed in the background with nothing changed: resume itself.
		{ID: "F4-resume", Entry: p3Background, Hooks: p3HooksNormal, Setup: trusted, Expect: providercert.ExpectSuccess,
			Steps: []func(*p3World) error{func(*p3World) error { return nil }}, Established: func(*p3Observed) bool { return true }},
		// A stored gateway login appears in the dedicated root after the first job; the resumed
		// session's new worker meets it.
		{ID: "F2-login-created", Entry: p3Background, Hooks: p3HooksNormal, Setup: trusted, Expect: providercert.ExpectSuccess,
			Steps: []func(*p3World) error{func(w *p3World) error { return w.storeGateway(w.sgA) }}, Established: gotCanary("WRONG_STORED_GATEWAY_A", 1)},
		// The login is replaced by another one between two resumed jobs.
		{ID: "F3-login-replaced", Entry: p3Background, Hooks: p3HooksNormal, Setup: func(w *p3World) error {
			if err := trusted(w); err != nil {
				return err
			}
			return w.storeGateway(w.sgA)
		}, Expect: providercert.ExpectSuccess, Steps: []func(*p3World) error{func(w *p3World) error { return w.storeGateway(w.sgB) }}, Established: gotCanary("WRONG_STORED_GATEWAY_B", 1)},
	}
}

// waitJobReply waits for the job's reply to this turn: its state's output carries the probe
// reply marker, or INTENDED served this turn's canary on a main request and the job has a
// state (a background session stays alive after replying, so "done" may never come).
func waitJobReply(w *p3World, canary providercert.Canary, job string, wait time.Duration) (string, bool) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if text, ok := waitJobDone(w.configDir, job, 250*time.Millisecond); ok {
			return text, true
		}
		for _, call := range w.ingressWithOutcomes() {
			if call.class == "main" && call.outcome == "" && slices.Contains(call.canaries, canary.ID) {
				time.Sleep(2 * time.Second) // the reply's stream completes
				return "CLAUDUCT-PROBE-REPLY-INTENDED served at the gateway", true
			}
		}
	}
	return "", false
}

// jobSessionID is the native session the job ran, from its state.
func jobSessionID(configDir, job string) string {
	raw, err := os.ReadFile(filepath.Join(configDir, "jobs", job, "state.json"))
	var state struct {
		SessionID string `json:"sessionId"`
	}
	if err != nil || json.Unmarshal(raw, &state) != nil {
		return ""
	}
	return state.SessionID
}

// p3SubagentSidecars reads native's subagent sidecars in the probe config dir: every
// agent-*.meta.json, and the first line of every agent-*.jsonl, each at most 4 KB, canaries
// redacted. The config dir is the probe's own synthetic one.
func p3SubagentSidecars(configDir string, turns []p3Turn) map[string]string {
	out := map[string]string{}
	filepath.WalkDir(filepath.Join(configDir, "projects"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(filepath.Dir(path)) != "subagents" || !strings.HasPrefix(d.Name(), "agent-") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".jsonl") {
			if i := bytes.IndexByte(raw, '\n'); i >= 0 {
				raw = raw[:i]
			}
		}
		if len(raw) > 4096 {
			raw = raw[:4096]
		}
		out[d.Name()] = p3RedactCanaries(string(raw), turns)
		return nil
	})
	return out
}
