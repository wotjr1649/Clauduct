package app

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

const checkpointInterval = 2 * time.Second

// SessionDuration parses only the explicit launcher setting. It is never inferred
// from a model, prompt, or native option. Invalid limits fail before spawning.
func SessionDuration(env map[string]string) (time.Duration, error) {
	raw := env["CLAUDUCT_SESSION_TIMEOUT_MS"]
	if raw == "" {
		return 0, nil
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms < 100 || ms > int64((12*time.Hour)/time.Millisecond) {
		return 0, errors.New("INVALID_SESSION_TIMEOUT")
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// An unfinished checkpoint is evidence of the last observed state, not proof
// that the process is still alive. Final is set only after native was waited on.
type LifecycleFacts struct {
	State               string    `json:"state"`
	Final               bool      `json:"final"`
	Reason              string    `json:"reason,omitempty"`
	StartedAt           time.Time `json:"startedAt"`
	ObservedAt          time.Time `json:"observedAt"`
	DeadlineAt          time.Time `json:"deadlineAt,omitzero"`
	DeadlineReachedAt   time.Time `json:"deadlineReachedAt,omitzero"`
	GraceExpiresAt      time.Time `json:"graceExpiresAt,omitzero"`
	GraceExpired        bool      `json:"graceExpired,omitempty"`
	NativeStopRequested bool      `json:"nativeStopRequested,omitempty"`
	NativeReaped        bool      `json:"nativeReaped"`
	CheckpointFailures  int       `json:"checkpointFailures,omitempty"`
}

func drainReady(d gateway.Diagnostics) bool {
	if d.Requests.Active != 0 {
		return false
	}
	for _, c := range d.AgentContexts {
		switch c.Phase {
		case "required", "compacting", "recount":
			return false
		}
	}
	if d.Progress.Unreadable > 0 || d.Progress.CapacityExceeded {
		return false
	}
	for _, p := range d.Progress.Agents {
		if p.PendingTools > 0 || p.Phase != "turn_ended" {
			return false
		}
	}
	for _, state := range []string{"running", "awaiting_children", "awaiting_native_stop", "awaiting_workflow_result"} {
		if d.AgentResults.Current[state] > 0 {
			return false
		}
	}
	return true
}

func waitForSession(ctx context.Context, process Process, gw *gateway.Gateway, o Options, initial Result) (error, bool, LifecycleFacts) {
	life := LifecycleFacts{State: "running", StartedAt: time.Now().UTC()}
	// Keep caller cancellation immediate. A harness deadline gets a separate,
	// bounded drain so it cannot destroy the gateway in the middle of compaction.
	waitCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type waited struct {
		err    error
		reaped bool
	}
	done := make(chan waited, 1)
	go func() { err, reaped := waitFor(waitCtx, process); done <- waited{err, reaped} }()
	var deadline, grace <-chan time.Time
	if o.SessionTimeout > 0 {
		life.DeadlineAt = life.StartedAt.Add(o.SessionTimeout)
		timer := time.NewTimer(o.SessionTimeout)
		defer timer.Stop()
		deadline = timer.C
	}
	tick := time.NewTicker(checkpointInterval)
	defer tick.Stop()
	caller := ctx.Done()
	var lastReceived, lastActive int64 = -1, -1
	var lastDrainReceived int64 = -1
	var idleSince time.Time
	checkpoint := func() gateway.Diagnostics {
		gw.ReconcileNativeCancellations()
		d := gw.Snapshot()
		life.ObservedAt = time.Now().UTC()
		if o.Checkpoint != nil {
			current := initial
			copy := life
			current.Lifecycle, current.Diagnostics = &copy, d
			current.NativeExitCode, current.Category = ExitCodeUnknown, "RUNNING"
			current.Attempts, current.Inferences, _ = o.Ledger.Spent()
			if err := o.Checkpoint(Account(current)); err != nil {
				life.CheckpointFailures++
			}
		}
		lastReceived, lastActive = d.Requests.Received, d.Requests.Active
		return d
	}
	stop := func() {
		life.State, life.NativeStopRequested = "stopping", true
		checkpoint()
		cancel()
	}
	checkpoint()
	for {
		select {
		case w := <-done:
			life.State, life.Final, life.NativeReaped = "finished", w.reaped, w.reaped
			if !w.reaped {
				life.State = "exit_unverified"
			}
			life.ObservedAt = time.Now().UTC()
			if life.Reason == "" {
				life.Reason = "native_exit"
			}
			if life.Reason == "caller_cancelled" && w.err == context.Canceled {
				return ctx.Err(), w.reaped, life
			}
			return w.err, w.reaped, life
		case <-caller:
			caller = nil
			deadline, grace = nil, nil
			life.Reason = "caller_cancelled"
			stop()
		case now := <-deadline:
			deadline = nil
			life.State, life.Reason = "draining", "session_deadline"
			life.DeadlineReachedAt, life.GraceExpiresAt = now.UTC(), now.Add(o.DeadlineGrace).UTC()
			timer := time.NewTimer(o.DeadlineGrace)
			defer timer.Stop()
			grace = timer.C
			checkpoint()
		case <-grace:
			grace = nil
			life.GraceExpired = true
			stop()
		case now := <-tick.C:
			received, _, active := gw.Stats()
			if life.State == "stopping" {
				continue
			}
			if life.State == "draining" {
				d := checkpoint()
				if !drainReady(d) {
					idleSince = time.Time{}
					continue
				}
				if idleSince.IsZero() || received != lastDrainReceived {
					idleSince = now
				} else if now.Sub(idleSince) >= checkpointInterval {
					grace = nil
					stop()
				}
				lastDrainReceived = received
			} else if received != lastReceived || active != lastActive || active > 0 || initial.HookInstalled {
				checkpoint()
			}
		}
	}
}
