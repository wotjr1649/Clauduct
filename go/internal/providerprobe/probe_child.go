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

	// Native prevention needs an OS-observed exit inside this bounded window. Heartbeats alone
	// keep their diagnostic fate but cannot establish termination. The late post stays beyond it.
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
	ReasonChildExitUnproven         = "CHILD_EXIT_UNPROVEN"
	ReasonChildNoEarlierTurn        = "CHILD_NO_EARLIER_TURN"
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
	if !time.Now().Before(start.Add(p3ChildCap)) {
		emit("probe-cap")
		return 2
	}
	emit("post-attempt")
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
	HeartbeatFate     string
	FateBecause       string `json:",omitempty"`
	// FinishInLaterTurn is the child's finish landed inside a turn that began after the hook did.
	FinishInLaterTurn bool
	// Judged is the invocation began before the last turn, so native outlived its fate window.
	Judged bool
	// NativeTimeoutRecorded is native's own transcript record of a failed UserPromptSubmit hook
	// inside the turn the invocation began in.
	NativeTimeoutRecorded  bool
	HandleKnown            bool
	ParentKnown            bool
	ObservedParentPID      uint32
	ObservedParentCreated  time.Time
	ObservedCreated        time.Time
	ExitAt                 time.Time
	ProbeCleanup, ProbeCap bool
	GenerationVerified     bool
	EvidenceTimeInvalid    bool
	ExitVerified           bool
	TerminationProven      bool
	PostAttemptAt          time.Time
	PostResult             string
	PostExit               int
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
func judgeLateHook(wrapped []p3WrapperRecord, turns []p3Turn, failures []p3HookFailure, guards p3LateGuards, exits []p3ChildExitEvidence) p3LateHook {
	var order []string
	byID := map[string][]p3WrapperRecord{}
	unidentified := false
	add := func(id string) {
		if _, exists := byID[id]; !exists {
			order = append(order, id)
			byID[id] = nil
		}
	}
	for _, r := range wrapped {
		if r.Mode != "child" {
			continue
		}
		if r.ID == "" {
			unidentified = true
			continue
		}
		add(r.ID)
		byID[r.ID] = append(byID[r.ID], r)
	}
	// Root-observed children remain obligations even if the sidecar loses their start or all rows.
	for _, exit := range exits {
		if exit.ID == "" {
			unidentified = true
			continue
		}
		add(exit.ID)
	}
	if unidentified {
		return p3LateHook{Outcome: p3LateInconclusive, Reason: ReasonChildExitUnproven}
	}
	if len(order) == 0 {
		return p3LateHook{Outcome: p3LateNotExercised, Reason: reasonNoHookInvocation}
	}
	var lastTurnStart time.Time
	if len(turns) > 0 {
		lastTurnStart = turns[len(turns)-1].Start
	}
	var proofs []p3ChildProof
	for _, id := range order {
		proofs = append(proofs, childProof(id, byID[id], wrapped, order, byID, turns, lastTurnStart, failures, guards, exits))
	}
	var judged []*p3ChildProof
	for i := range proofs {
		if proofs[i].Judged {
			judged = append(judged, &proofs[i])
		}
	}
	if len(turns) < 2 || len(judged) == 0 {
		return p3LateHook{Outcome: p3LateInconclusive, Reason: ReasonChildNoEarlierTurn, Proofs: proofs}
	}
	var ready []*p3ChildProof
	for _, p := range judged {
		if p.ReadyBeforeTimout {
			ready = append(ready, p)
		}
	}
	out := p3LateHook{Proofs: proofs}
	if slices.ContainsFunc(judged, func(p *p3ChildProof) bool { return !p.GenerationVerified || p.EvidenceTimeInvalid }) {
		out.Outcome, out.Reason = p3LateInconclusive, ReasonChildExitUnproven
		return out
	}
	if len(ready) != len(judged) {
		out.Outcome, out.Reason = p3LateNotExercised, ReasonChildNotReady
		return out
	}
	if slices.ContainsFunc(ready, func(p *p3ChildProof) bool { return !p.ExitVerified }) {
		out.Outcome, out.Reason = p3LateInconclusive, ReasonChildExitUnproven
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
		if slices.ContainsFunc(ready, func(p *p3ChildProof) bool { return !p.TerminationProven }) {
			out.Outcome, out.Reason = p3LateInconclusive, ReasonChildExitUnproven
		} else {
			out.Outcome, out.Reason = p3LateNotExercised, ReasonChildTerminated
		}
	case fates[p3FateTerminated]:
		// The child ended, but native's own handling of the hook timeout was not observed for
		// every terminated proof: the end is not shown to be native's.
		out.Outcome, out.Reason = p3LateInconclusive, ReasonChildTerminatedUnrecorded
	case late && !slices.ContainsFunc(ready, func(p *p3ChildProof) bool {
		return !p.FinishInLaterTurn || p.PostAttemptAt.IsZero() || p.PostResult != "EXITED"
	}):
		out.Outcome = p3LateExercised
	default:
		out.Outcome, out.Reason = p3LateInconclusive, reasonChildSurvivedNoLate
	}
	return out
}

func childProof(id string, records, all []p3WrapperRecord, order []string, byID map[string][]p3WrapperRecord, turns []p3Turn, lastTurnStart time.Time, failures []p3HookFailure, guards p3LateGuards, exits []p3ChildExitEvidence) p3ChildProof {
	p := p3ChildProof{ID: id, Judged: true, Fate: p3FateAmbiguous, PostResult: "NOT_ATTEMPTED"}
	var start, spawned, ready *p3WrapperRecord
	startCount, spawnCount, readyCount := 0, 0, 0
	for i := range records {
		r := &records[i]
		switch r.Phase {
		case "start":
			startCount++
			if start == nil {
				start = r
			}
		case "spawned":
			spawnCount++
			if spawned == nil {
				spawned = r
			}
		case "ready":
			readyCount++
			if ready == nil {
				ready = r
			}
		}
	}
	if start != nil {
		p.Start, p.WrapperPID = start.At, start.PID
		p.TimeoutAt = start.Created.Add(p3HookTimeout)
	}
	notReady := func(why string) p3ChildProof {
		p.NotReadyBecause = why
		return p
	}
	if startCount != 1 || spawnCount != 1 || readyCount > 1 || start.Event != "UserPromptSubmit" {
		return notReady("duplicate or conflicting identity records")
	}
	if spawned == nil || !p3ChildSpawnMatches(*start, *spawned) {
		return notReady("no spawn record with the child's pid and creation time")
	}
	p.ChildPID = spawned.ChildPID
	var exit *p3ChildExitEvidence
	duplicate := false
	for i := range exits {
		if exits[i].ID == id {
			duplicate = duplicate || exit != nil
			exit = &exits[i]
		}
	}
	if exit != nil {
		p.HandleKnown, p.ObservedCreated, p.ExitAt = exit.HandleKnown, exit.ObservedCreated, exit.ExitAt
		p.ParentKnown, p.ObservedParentPID, p.ObservedParentCreated = exit.ParentKnown, exit.ObservedParentPID, exit.ObservedParentCreated
		p.ProbeCleanup, p.ProbeCap = exit.ProbeCleanup, exit.ProbeCap
		p.GenerationVerified = !duplicate && exit.HandleKnown && exit.ParentKnown && exit.ObservedParentPID == uint32(start.PID) &&
			exit.ObservedParentCreated.Equal(start.Created) && exit.PID == uint32(spawned.ChildPID) &&
			exit.Created.Equal(spawned.Created) && exit.ObservedCreated.Equal(spawned.Created) &&
			exit.ObservedParentCreated.Before(exit.ObservedCreated)
	}
	// Contradictory child records remain obligations, including in the last turn. Every row that
	// names this invocation or its child process is checked, whatever its Mode says.
	if p.GenerationVerified {
		for _, r := range all {
			namesChild := (r.PID == spawned.ChildPID || r.ChildPID == spawned.ChildPID) && (r.Created.IsZero() || r.Created.Equal(spawned.Created))
			if r.ID != id && !namesChild {
				continue
			}
			if r.Mode != p3HookChild || r.ID != id {
				p.EvidenceTimeInvalid = true
				return notReady("a record names the child under another mode or invocation")
			}
			switch r.Phase {
			case "ready", "heartbeat", "post-attempt", "finish", "run-failed":
				if r.At.Before(p.ObservedCreated) || r.Phase == "heartbeat" && ready != nil && r.At.Before(ready.At) {
					p.EvidenceTimeInvalid = true
					return notReady("child evidence precedes OS creation or readiness")
				}
				if r.PPID != 0 && r.PPID != start.PID {
					p.EvidenceTimeInvalid = true
					return notReady("child evidence names another parent")
				}
			}
		}
	}
	base := start.Created
	// An unverified generation is never excluded as a final-turn child.
	if p.GenerationVerified {
		base = exit.ObservedParentCreated
		p.Judged = !lastTurnStart.IsZero() && base.Before(lastTurnStart)
	}
	p.TimeoutAt = base.Add(p3HookTimeout)
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
		if r.PID != pid || guards.PIDMatch && !r.Created.Equal(spawned.Created) {
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
			p.PostResult, p.PostExit = "EXITED", r.Exit
		case "post-attempt":
			p.PostAttemptAt, p.PostResult = r.At, "INCOMPLETE"
		case "run-failed":
			p.PostResult, p.PostExit = "START_FAILED", r.Exit
		case "probe-cap":
			p.ProbeCap = true
		}
	}
	alive := ready.At
	if p.LastHeartbeat.After(alive) {
		alive = p.LastHeartbeat
	}
	if p.FinishAt.After(alive) {
		alive = p.FinishAt
	}
	// Only root-observed turn endings establish that native ran past the fate window.
	var horizon time.Time
	for _, tr := range turns {
		if tr.End.After(horizon) {
			horizon = tr.End
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
	p.HeartbeatFate = p.Fate
	// Missing and incomplete root exits have the same fail-closed fate, including survivors.
	p.ExitVerified = p.GenerationVerified && !p.ExitAt.IsZero() && !p.ExitAt.Before(alive) && !p.ExitAt.Before(p.ObservedCreated)
	switch {
	case !p.ExitVerified || p.ProbeCleanup || p.ProbeCap:
		p.Fate, p.FateBecause = p3FateAmbiguous, "exit proof is missing, inconsistent, or probe-caused"
	case !p.ExitAt.Before(p.TimeoutAt.Add(p3TerminateUntil)):
		p.Fate = p3FateSurvived
	case !p.ExitAt.Before(p.TimeoutAt.Add(-p3TerminateBefore)) && p.FinishAt.IsZero() && p.PostAttemptAt.IsZero() && !horizon.Before(p.TimeoutAt.Add(p3TerminateUntil)):
		p.Fate = p3FateTerminated
	default:
		p.Fate, p.FateBecause = p3FateAmbiguous, "exit is outside the termination window or a post was attempted"
	}
	if !p.FinishAt.IsZero() {
		for k := 1; k < len(turns); k++ {
			tr := turns[k]
			if base.Before(tr.Start) && !p.FinishAt.Before(tr.Start) && !p.FinishAt.After(tr.End) && !p.PostAttemptAt.IsZero() && !p.PostAttemptAt.After(p.FinishAt) {
				p.FinishInLaterTurn = true
			}
		}
	}
	for _, f := range failures {
		if f.Event != "UserPromptSubmit" || f.Type != "hook_cancelled" || f.At.Before(p.TimeoutAt) {
			continue
		}
		for i := range turns {
			if inWindow(turns, i, base) && inWindow(turns, i, f.At) {
				invocations := 0
				for _, other := range order {
					if slices.ContainsFunc(byID[other], func(r p3WrapperRecord) bool { return r.Phase == "start" && inWindow(turns, i, r.Created) }) {
						invocations++
					}
				}
				p.NativeTimeoutRecorded = invocations == 1
			}
		}
	}
	p.TerminationProven = p.ExitVerified && p.Fate == p3FateTerminated && p.NativeTimeoutRecorded &&
		!p.ProbeCleanup && !p.ProbeCap && p.Heartbeats > 0 && p.FinishAt.IsZero() && p.PostAttemptAt.IsZero() &&
		!horizon.Before(p.TimeoutAt.Add(p3TerminateUntil))
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
	j := judgeLateHook(obs.Wrapped, obs.Turns, obs.HookFailures, p3AllGuards, p3ChildExitFacts(obs))
	return &j
}

// Root release faults taint both the judge and serialized exit facts.
func p3ChildExitFacts(obs *p3Observed) []p3ChildExitEvidence {
	exits := slices.Clone(obs.ChildExits)
	if obs.HeldProcess || obs.JobUnconfirmed || obs.CleanupFailed || obs.TimedOut {
		for i := range exits {
			exits[i].ProbeCleanup = true
		}
	}
	return exits
}

// p3HookProcs is the post-case census of the processes the child-mode wrapper left.
type p3HookProcs struct {
	// Seen are the wrapper and child processes the records name; StillRunning were alive when
	// the census began; Killed had to be ended by the census (a child never outlives its cap).
	Seen, StillRunning, Killed int
}

// p3ReapHookProcesses finds the wrapper and child processes of a world's sidecar by the pid and
// creation time they recorded, waits for those that are still ending, and ends the rest.
// stop, when set, is the run's absolute deadline: no wait, the kill waits included, runs past it
// (a target still alive then is ended and counted as killed).
func p3ReapHookProcesses(sidecar string, wait time.Duration, stop time.Time, watch *p3ChildExitWatch) p3HookProcs {
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
	// One end for every wait: the reap's own bound, never past the run's absolute deadline.
	deadline := time.Now().Add(wait)
	if !stop.IsZero() && stop.Before(deadline) {
		deadline = stop
	}
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
			if watch != nil {
				watch.markProbeCap(uint32(t.pid), t.created)
			}
			syscall.TerminateProcess(h, 1)
			kill := 5 * time.Second
			if !stop.IsZero() {
				kill = min(kill, max(time.Until(stop), 0))
			}
			syscall.WaitForSingleObject(h, uint32(kill.Milliseconds()))
			out.Killed++
		}
		syscall.CloseHandle(h)
	}
	return out
}

func (h p3HookProcs) String() string {
	return fmt.Sprintf("seen=%d stillRunning=%d killed=%d", h.Seen, h.StillRunning, h.Killed)
}
