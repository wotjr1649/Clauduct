package providerprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// R04L's late-hook variant (#319 phase 2). The "delay" mode of the wrapper starts the real hook
// only after native's 1-second timeout, so nothing exists at timeout time and a late event
// cannot be told apart from native never having run the hook. The "child" mode creates a
// probe-owned child that is ready before the timeout and posts the real hook's event late, and
// the judge below reads records, not only turn windows: the child was created and ready, what
// native did to it at the timeout, and only then whether a late event arrived.
const (
	// p3HookChild is the HookFaults variant that wraps a command hook in the child mode.
	p3HookChild = "child"
	// p3HookChildEscape is the same with a child created to break away from the hook's job
	// object: a control that shows the judge reads a surviving child (a late event that really
	// arrives) when one exists. No contract case uses it; R04L-ESC runs it as a diagnostic.
	p3HookChildEscape = "child-escape"

	p3HookChildArg = "--p3-hook-child"

	// p3HookTimeout is the timeout the case configures for the wrapped hook (whole seconds).
	p3HookTimeout = time.Second

	// p3ChildReadyWait bounds how long the wrapper waits for the child's ready record.
	p3ChildReadyWait = 400 * time.Millisecond
	// p3ChildLinger is how long the wrapper stays alive after that: well past the timeout, so
	// it is native's timeout that ends it and not its own exit.
	p3ChildLinger = 5 * time.Second
	// p3ChildHeartbeat is the child's liveness record interval.
	p3ChildHeartbeat = 100 * time.Millisecond
	// p3ChildCap ends a child that nothing else ended: the longest it may run from the start.
	p3ChildCap = 8 * time.Second

	// p3ChildLate is when the child posts the real hook's event, from the wrapper's start: well
	// past the timeout and past any termination latency native has shown (-38..+418ms after the
	// timeout; native ends the hook's tree on its own schedule), so how fast native ends the child
	// cannot decide whether the late post happens. The child beats until then.
	p3ChildLate = p3HookTimeout + 3*time.Second

	// p3ReplyDelay is how long the synthetic backend holds each turn's reply in the late-hook
	// cases: a turn then lasts about this plus the model call, so the next turn begins before a
	// child's late post and native is shown running past every child's fate window.
	p3ReplyDelay = 1500 * time.Millisecond

	// The fate window around the timeout T: a child whose last sign of life is in
	// [T-p3TerminateBefore, T+p3TerminateUntil) and that never posted was ended by native. A child
	// alive at or past T+p3TerminateUntil was not (the late post at T+3s is strictly after the
	// window, a heartbeat is 100ms apart, so a child alive at the timeout has beaten within
	// p3TerminateBefore).
	p3TerminateBefore = 400 * time.Millisecond
	p3TerminateUntil  = 2 * time.Second

	// Judge outcomes.
	p3LateExercised    = "EXERCISED"
	p3LateNotExercised = "NOT_EXERCISED"
	p3LateInconclusive = "INCONCLUSIVE"

	// Reasons the judge gives.
	ReasonChildNotReady   = "CHILD_NOT_READY_BEFORE_TIMEOUT"
	ReasonChildTerminated = "CHILD_TERMINATED_BY_NATIVE"
	// ReasonChildTerminatedUnrecorded is a terminated child whose native timeout handling is not
	// in native's own transcript for every proof.
	ReasonChildTerminatedUnrecorded = "CHILD_TERMINATED_TIMEOUT_UNRECORDED"
	reasonNoHookInvocation          = "NO_HOOK_INVOCATION"
	reasonChildFateAmbiguous        = "CHILD_FATE_AMBIGUOUS"
	reasonChildFateMixed            = "CHILD_FATE_MIXED"
	reasonChildSurvivedNoLate       = "CHILD_SURVIVED_NO_LATE_EVENT"
	p3FateSurvived                  = "SURVIVED"
	p3FateTerminated                = "TERMINATED"
	p3FateAmbiguous                 = "AMBIGUOUS"
)

const (
	// PROCESS_TERMINATE | SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION.
	p3ProcessAccess = syscall.PROCESS_TERMINATE | syscall.SYNCHRONIZE | 0x1000
	// p3ChildArgs are the child role's arguments before the real hook's argv: sidecar, ID,
	// start (unix nanoseconds) and event file.
	p3ChildArgs       = 4
	p3CreateNoWindow  = 0x08000000
	p3CreateBreakaway = 0x01000000
)

// selfCreated is this process's OS creation time.
func selfCreated() time.Time {
	var created, exited, kernel, user syscall.Filetime
	h, err := syscall.GetCurrentProcess()
	if err != nil || syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return time.Time{}
	}
	return time.Unix(0, created.Nanoseconds())
}

// p3ChildWrapper is the child mode: it records the start, creates the probe-owned child, records
// the child's identity from its own process handle, waits briefly for the child's ready record
// and then stays alive past the timeout. The child holds none of the hook's pipes: it gets its
// event from a file and its standard handles are the null device.
func p3ChildWrapper(record p3WrapperRecord, sidecar string, event []byte, real []string) int {
	escape := record.Mode == p3HookChildEscape
	record.Mode, record.Escape = "child", escape
	self, err := os.Executable()
	if err != nil {
		return 2
	}
	record.PID, record.Created, record.TimeoutMs = os.Getpid(), selfCreated(), int(p3HookTimeout/time.Millisecond)
	appendWrapped(sidecar, record)
	eventFile := sidecar + "." + record.ID + ".event"
	if err := os.WriteFile(eventFile, event, 0o600); err != nil {
		return 2
	}
	args := append([]string{p3HookChildArg, sidecar, record.ID, strconv.FormatInt(record.At.UnixNano(), 10), eventFile}, real...)
	cmd := exec.Command(self, args...)
	cmd.Env = os.Environ()
	flags := uint32(p3CreateNoWindow)
	if escape {
		flags |= p3CreateBreakaway
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
	if err := cmd.Start(); err != nil {
		os.Remove(eventFile)
		record.Phase, record.Failure = "spawn-failed", err.Error()
		appendWrapped(sidecar, record)
		return 2
	}
	spawned := record
	spawned.At, spawned.Phase, spawned.ChildPID = time.Now(), "spawned", cmd.Process.Pid
	spawned.Created = time.Time{}
	cmd.Process.WithHandle(func(h uintptr) {
		var created, exited, kernel, user syscall.Filetime
		if syscall.GetProcessTimes(syscall.Handle(h), &created, &exited, &kernel, &user) == nil {
			spawned.Created = time.Unix(0, created.Nanoseconds())
		}
	})
	appendWrapped(sidecar, spawned)
	for deadline := time.Now().Add(p3ChildReadyWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if slices.ContainsFunc(readWrapped(sidecar), func(r p3WrapperRecord) bool { return r.ID == record.ID && r.Phase == "ready" }) {
			break
		}
	}
	time.Sleep(p3ChildLinger)
	return 0
}

// p3HookChildMain is the child role: it reports ready, beats until the late moment, runs the real
// hook with the captured event and reports how it ended.
func p3HookChildMain(args []string) int {
	if len(args) < p3ChildArgs+1 {
		return 2
	}
	sidecar, id, eventFile, real := args[0], args[1], args[3], args[4:]
	ns, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return 2
	}
	start := time.Unix(0, ns)
	event, err := os.ReadFile(eventFile)
	os.Remove(eventFile)
	if err != nil {
		return 2
	}
	var named struct {
		Name string `json:"hook_event_name"`
	}
	json.Unmarshal(event, &named)
	record := p3WrapperRecord{ID: id, Mode: "child", Event: named.Name, PID: os.Getpid(), PPID: int(parentPID(uint32(os.Getpid()))), Created: selfCreated()}
	for _, entry := range os.Environ() {
		if name, value, _ := strings.Cut(entry, "="); strings.EqualFold(name, p3EpochEnv) {
			record.Epoch = value
		}
	}
	emit := func(phase string) {
		record.At, record.Phase = time.Now(), phase
		appendWrapped(sidecar, record)
	}
	emit("ready")
	for time.Now().Before(start.Add(p3ChildLate)) && time.Now().Before(start.Add(p3ChildCap)) {
		time.Sleep(p3ChildHeartbeat)
		emit("heartbeat")
	}
	cmd := exec.Command(real[0], real[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout = os.Environ(), bytes.NewReader(event), io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			record.Exit = -1
			emit("run-failed")
			return 2
		}
		code = exit.ExitCode()
	}
	record.Exit = code
	record.ProofMissing = strings.Contains(stderr.String(), "CLAUDUCT_ROUTING_PROOF_MISSING")
	record.Mismatch = strings.Contains(stderr.String(), "CLAUDUCT_ROUTING_MISMATCH")
	emit("finish")
	return code
}

// p3ChildProof is what the records show about one wrapped hook invocation and its child.
type p3ChildProof struct {
	ID                string
	WrapperPID        int
	ChildPID          int
	ChildPPID         int
	Start, TimeoutAt  time.Time
	Ready             time.Time
	ReadyBeforeTimout bool
	NotReadyBecause   string `json:",omitempty"`
	Heartbeats        int
	LastHeartbeat     time.Time
	FinishAt          time.Time
	Fate              string
	FateBecause       string `json:",omitempty"`
	// FinishInLaterTurn is the child's finish landed inside a turn that began after the hook did.
	FinishInLaterTurn bool
	// Judged is the invocation began before the last turn, so native outlived its fate window.
	Judged bool
	// NativeTimeoutRecorded is native's own transcript record of a failed UserPromptSubmit hook
	// inside the turn the invocation began in.
	NativeTimeoutRecorded bool
}

// p3LateHook is the judge's reading of a late-hook case's records.
type p3LateHook struct {
	Outcome string
	Reason  string `json:",omitempty"`
	Proofs  []p3ChildProof
}

// p3LateGuards switch the judge's checks; production always has them all on. A test turns one
// off to show that the check is what decides the verdict.
type p3LateGuards struct {
	Ready, PIDMatch bool
}

var p3AllGuards = p3LateGuards{Ready: true, PIDMatch: true}

// judgeLateHook reads the child-mode records of UserPromptSubmit invocations.
func judgeLateHook(wrapped []p3WrapperRecord, turns []p3Turn, failures []p3HookFailure, guards p3LateGuards) p3LateHook {
	var order []string
	byID := map[string][]p3WrapperRecord{}
	for _, r := range wrapped {
		if r.Mode != "child" || r.ID == "" {
			continue
		}
		if r.Phase == "start" && r.Event == "UserPromptSubmit" {
			order = append(order, r.ID)
		}
		byID[r.ID] = append(byID[r.ID], r)
	}
	if len(order) == 0 {
		return p3LateHook{Outcome: p3LateNotExercised, Reason: reasonNoHookInvocation}
	}
	var lastTurnStart time.Time
	if len(turns) > 1 {
		lastTurnStart = turns[len(turns)-1].Start
	}
	var proofs []p3ChildProof
	for _, id := range order {
		proofs = append(proofs, childProof(id, byID[id], order, byID, turns, lastTurnStart, failures, guards))
	}
	var judged []*p3ChildProof
	for i := range proofs {
		if proofs[i].Judged {
			judged = append(judged, &proofs[i])
		}
	}
	if len(judged) == 0 {
		// Every invocation began in the last turn: no later turn exists to receive a late event.
		for i := range proofs {
			proofs[i].Judged = true
			judged = append(judged, &proofs[i])
		}
	}
	var ready []*p3ChildProof
	for _, p := range judged {
		if p.ReadyBeforeTimout {
			ready = append(ready, p)
		}
	}
	out := p3LateHook{Proofs: proofs}
	if len(ready) == 0 {
		out.Outcome, out.Reason = p3LateNotExercised, ReasonChildNotReady
		return out
	}
	fates := map[string]bool{}
	late := false
	for _, p := range ready {
		fates[p.Fate] = true
		late = late || p.FinishInLaterTurn
	}
	switch {
	case fates[p3FateAmbiguous]:
		out.Outcome, out.Reason = p3LateInconclusive, reasonChildFateAmbiguous
	case fates[p3FateSurvived] && fates[p3FateTerminated]:
		out.Outcome, out.Reason = p3LateInconclusive, reasonChildFateMixed
	case fates[p3FateTerminated] && !slices.ContainsFunc(ready, func(p *p3ChildProof) bool { return p.Fate == p3FateTerminated && !p.NativeTimeoutRecorded }):
		out.Outcome, out.Reason = p3LateNotExercised, ReasonChildTerminated
	case fates[p3FateTerminated]:
		// The child ended, but native's own handling of the hook timeout was not observed for
		// every terminated proof: the end is not shown to be native's.
		out.Outcome, out.Reason = p3LateInconclusive, ReasonChildTerminatedUnrecorded
	case late:
		out.Outcome = p3LateExercised
	default:
		out.Outcome, out.Reason = p3LateInconclusive, reasonChildSurvivedNoLate
	}
	return out
}

func childProof(id string, records []p3WrapperRecord, order []string, byID map[string][]p3WrapperRecord, turns []p3Turn, lastTurnStart time.Time, failures []p3HookFailure, guards p3LateGuards) p3ChildProof {
	p := p3ChildProof{ID: id, Fate: p3FateAmbiguous}
	var start, spawned, ready *p3WrapperRecord
	for i := range records {
		r := &records[i]
		switch r.Phase {
		case "start":
			if start == nil {
				start = r
			}
		case "spawned":
			if spawned == nil {
				spawned = r
			}
		case "ready":
			if ready == nil {
				ready = r
			}
		}
	}
	p.Start, p.WrapperPID = start.At, start.PID
	// The timeout runs from native's spawn of the hook, which is at or before the wrapper's own
	// creation: the earlier of the two is the conservative start.
	base := start.At
	if !start.Created.IsZero() && start.Created.Before(base) {
		base = start.Created
	}
	timeout := p3HookTimeout
	if start.TimeoutMs > 0 {
		timeout = time.Duration(start.TimeoutMs) * time.Millisecond
	}
	p.TimeoutAt = base.Add(timeout)
	p.Judged = lastTurnStart.IsZero() || start.At.Before(lastTurnStart)
	notReady := func(why string) p3ChildProof {
		p.NotReadyBecause = why
		return p
	}
	if spawned == nil || spawned.ChildPID <= 0 || spawned.Created.IsZero() {
		return notReady("no spawn record with the child's pid and creation time")
	}
	p.ChildPID = spawned.ChildPID
	if ready == nil {
		return notReady("no ready record")
	}
	p.Ready, p.ChildPPID = ready.At, ready.PPID
	if guards.PIDMatch && (ready.PID != spawned.ChildPID || ready.PPID != start.PID || !ready.Created.Equal(spawned.Created)) {
		return notReady("the ready record is not the process the wrapper created")
	}
	if guards.Ready && !ready.At.Before(p.TimeoutAt) {
		return notReady("ready after the timeout")
	}
	p.ReadyBeforeTimout = true
	// Only the created child's own records count from here on.
	pid := spawned.ChildPID
	if !guards.PIDMatch {
		pid = ready.PID
	}
	for _, r := range records {
		if r.PID != pid {
			continue
		}
		switch r.Phase {
		case "heartbeat":
			p.Heartbeats++
			if r.At.After(p.LastHeartbeat) {
				p.LastHeartbeat = r.At
			}
		case "finish":
			p.FinishAt = r.At
		}
	}
	alive := ready.At
	if p.LastHeartbeat.After(alive) {
		alive = p.LastHeartbeat
	}
	if p.FinishAt.After(alive) {
		alive = p.FinishAt
	}
	// Native demonstrably ran past the fate window: a later turn or a later hook invocation.
	var horizon time.Time
	for _, tr := range turns {
		if tr.End.After(horizon) {
			horizon = tr.End
		}
	}
	for _, other := range order {
		if other != id {
			if s := byID[other][0]; s.At.After(horizon) {
				horizon = s.At
			}
		}
	}
	switch {
	case !alive.Before(p.TimeoutAt.Add(p3TerminateUntil)):
		p.Fate = p3FateSurvived
	case p.FinishAt.IsZero() && !alive.Before(p.TimeoutAt.Add(-p3TerminateBefore)):
		if horizon.Before(p.TimeoutAt.Add(p3TerminateUntil)) {
			p.FateBecause = "native was not shown running past the fate window"
		} else if p.Heartbeats == 0 {
			p.FateBecause = "no heartbeat before the timeout"
		} else {
			p.Fate = p3FateTerminated
		}
	default:
		p.FateBecause = "last sign of life is outside the window around the timeout"
	}
	if !p.FinishAt.IsZero() {
		for k := 1; k < len(turns); k++ {
			tr := turns[k]
			if start.At.Before(tr.Start) && !p.FinishAt.Before(tr.Start) && !p.FinishAt.After(tr.End) {
				p.FinishInLaterTurn = true
			}
		}
	}
	for _, f := range failures {
		if f.Event != "UserPromptSubmit" {
			continue
		}
		for i := range turns {
			if inWindow(turns, i, start.At) && inWindow(turns, i, f.At) {
				p.NativeTimeoutRecorded = true
			}
		}
	}
	return p
}

// p3ChildFault reports a HookFaults value that wraps a hook in the child mode.
func p3ChildFault(fault string) bool { return fault == p3HookChild || fault == p3HookChildEscape }

// p3LateHookOf is the judge's reading for a case whose UserPromptSubmit hook is wrapped in the
// child mode, nil for any other case.
func p3LateHookOf(hookFaults map[string]string, obs *p3Observed) *p3LateHook {
	if !p3ChildFault(hookFaults["UserPromptSubmit"]) {
		return nil
	}
	j := judgeLateHook(obs.Wrapped, obs.Turns, obs.HookFailures, p3AllGuards)
	return &j
}

// p3HookProcs is the post-case census of the processes the child-mode wrapper left.
type p3HookProcs struct {
	// Seen are the wrapper and child processes the records name; StillRunning were alive when
	// the census began; Killed had to be ended by the census (a child never outlives its cap).
	Seen, StillRunning, Killed int
}

// p3ReapHookProcesses finds the wrapper and child processes of a world's sidecar by the pid and
// creation time they recorded, waits for those that are still ending, and ends the rest.
func p3ReapHookProcesses(sidecar string, wait time.Duration) p3HookProcs {
	type target struct {
		pid     int
		created time.Time
	}
	var targets []target
	for _, r := range readWrapped(sidecar) {
		if r.Mode != "child" || r.Created.IsZero() {
			continue
		}
		switch r.Phase {
		case "start":
			targets = append(targets, target{r.PID, r.Created})
		case "spawned":
			targets = append(targets, target{r.ChildPID, r.Created})
		}
	}
	var out p3HookProcs
	deadline := time.Now().Add(wait)
	for _, t := range targets {
		if t.pid <= 0 {
			continue
		}
		h, err := syscall.OpenProcess(p3ProcessAccess, false, uint32(t.pid))
		if err != nil {
			continue // gone
		}
		var created, exited, kernel, user syscall.Filetime
		if syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil || !time.Unix(0, created.Nanoseconds()).Equal(t.created) {
			syscall.CloseHandle(h) // a different process now holds the pid
			continue
		}
		out.Seen++
		left := time.Until(deadline)
		if state, err := syscall.WaitForSingleObject(h, 0); err == nil && state == syscall.WAIT_OBJECT_0 {
			syscall.CloseHandle(h)
			continue
		}
		out.StillRunning++
		state, err := syscall.WaitForSingleObject(h, uint32(max(left, 0).Milliseconds()))
		if err != nil || state != syscall.WAIT_OBJECT_0 {
			syscall.TerminateProcess(h, 1)
			syscall.WaitForSingleObject(h, 5000)
			out.Killed++
		}
		syscall.CloseHandle(h)
	}
	return out
}

func (h p3HookProcs) String() string {
	return fmt.Sprintf("seen=%d stillRunning=%d killed=%d", h.Seen, h.StillRunning, h.Killed)
}
