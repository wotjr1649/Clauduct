package providerprobe

import (
	"context"
	"errors"
	"sync"
	"time"
)

const p3ObservationWindowDuration = 8 * time.Second
const p3ObservationWindowPollInterval = 10 * time.Millisecond

var errP3ObservationComplete = errors.New("provider observation window complete")
var errP3ObservationIncomplete = errors.New("provider observation window ended before quiescence")

type p3ObservationTrigger struct {
	Run                int    `json:"run"`
	Gateway            int64  `json:"gateway"`
	Seq                int64  `json:"seq"`
	Connection         int64  `json:"connection"`
	Canary             string `json:"canary"`
	ConfirmationSHA256 string `json:"confirmationSha256"`
	Category           string `json:"category"`
	Status             int    `json:"status"`
}

type p3ObservationWindowEvidence struct {
	Kind       string               `json:"kind"`
	DurationMs int64                `json:"durationMs"`
	GraceMs    int64                `json:"graceMs"`
	ElapsedMs  int64                `json:"elapsedMs"`
	Completed  bool                 `json:"completed"`
	Trigger    p3ObservationTrigger `json:"trigger"`
}

type p3ObservationWindow struct {
	ctx      context.Context
	parent   context.Context
	cancel   context.CancelCauseFunc
	kind     string
	duration time.Duration
	grace    time.Duration
	ready    func() bool

	mu        sync.Mutex
	started   bool
	finished  bool
	completed bool
	startedAt time.Time
	elapsedMs int64
	trigger   p3ObservationTrigger
	stop      chan struct{}
	done      chan struct{}
}

func newP3ObservationWindow(ctx, parent context.Context, cancel context.CancelCauseFunc, kind string) *p3ObservationWindow {
	return &p3ObservationWindow{ctx: ctx, parent: parent, cancel: cancel, kind: kind, duration: p3ObservationWindowDuration}
}

func (w *p3ObservationWindow) start(trigger p3ObservationTrigger) {
	w.mu.Lock()
	if w.started || w.finished {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.startedAt = time.Now()
	w.trigger = trigger
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	startedAt, duration, grace, stop, done := w.startedAt, w.duration, w.grace, w.stop, w.done
	w.mu.Unlock()

	go w.wait(startedAt, duration, grace, stop, done)
}

func (w *p3ObservationWindow) wait(startedAt time.Time, duration, grace time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	minEnd := startedAt.Add(duration)
	maxEnd := minEnd.Add(grace)
	if !w.waitUntil(minEnd, stop) || !w.healthy() {
		return
	}
	if !w.parentCovers(maxEnd) {
		w.waitForEnd(stop)
		return
	}
	if w.ready == nil || w.ready() {
		w.complete(startedAt, duration, grace, maxEnd)
		return
	}
	if grace <= 0 {
		w.incomplete(maxEnd)
		return
	}

	poll := time.NewTicker(p3ObservationWindowPollInterval)
	defer poll.Stop()
	deadline := time.NewTimer(time.Until(maxEnd))
	defer deadline.Stop()
	for {
		select {
		case <-poll.C:
			if !w.healthy() {
				return
			}
			if w.ready() {
				w.complete(startedAt, duration, grace, maxEnd)
				w.incomplete(maxEnd)
				return
			}
		case <-deadline.C:
			if remaining := time.Until(maxEnd); remaining > 0 {
				deadline.Reset(remaining)
				continue
			}
			if !w.healthy() {
				return
			}
			if w.ready() {
				w.complete(startedAt, duration, grace, maxEnd)
				w.incomplete(maxEnd)
			} else {
				w.incomplete(maxEnd)
			}
			return
		case <-w.parent.Done():
			return
		case <-w.ctx.Done():
			return
		case <-stop:
			return
		}
	}
}

func (w *p3ObservationWindow) waitUntil(end time.Time, stop <-chan struct{}) bool {
	timer := time.NewTimer(time.Until(end))
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			if remaining := time.Until(end); remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			return true
		case <-w.parent.Done():
			return false
		case <-w.ctx.Done():
			return false
		case <-stop:
			return false
		}
	}
}

func (w *p3ObservationWindow) waitForEnd(stop <-chan struct{}) {
	select {
	case <-w.parent.Done():
	case <-w.ctx.Done():
	case <-stop:
	}
}

func (w *p3ObservationWindow) healthy() bool {
	return w.parent.Err() == nil && w.ctx.Err() == nil
}

func (w *p3ObservationWindow) parentCovers(end time.Time) bool {
	deadline, ok := w.parent.Deadline()
	return !ok || deadline.After(end)
}

func (w *p3ObservationWindow) complete(startedAt time.Time, duration, grace time.Duration, maxEnd time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished || !w.healthy() || !w.parentCovers(maxEnd) {
		return
	}
	elapsed := time.Since(startedAt)
	if elapsed < duration || grace > 0 && (elapsed > duration+grace || time.Now().After(maxEnd)) {
		return
	}
	w.cancel(errP3ObservationComplete)
	if !w.healthyAfterExpectedCancel() {
		return
	}
	elapsed = time.Since(startedAt)
	if elapsed < duration || grace > 0 && (elapsed > duration+grace || time.Now().After(maxEnd)) {
		return
	}
	w.elapsedMs = elapsed.Milliseconds()
	w.completed = true
}

func (w *p3ObservationWindow) incomplete(maxEnd time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished || !w.healthy() || !w.parentCovers(maxEnd) || time.Now().Before(maxEnd) {
		return
	}
	w.cancel(errP3ObservationIncomplete)
}

func (w *p3ObservationWindow) healthyAfterExpectedCancel() bool {
	return w.parent.Err() == nil && context.Cause(w.ctx) == errP3ObservationComplete
}

func (w *p3ObservationWindow) finish() *p3ObservationWindowEvidence {
	w.mu.Lock()
	if !w.started {
		w.finished = true
		w.mu.Unlock()
		return nil
	}
	if !w.finished {
		w.finished = true
		close(w.stop)
	}
	done := w.done
	w.mu.Unlock()
	<-done

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.parent.Err() != nil {
		w.completed = false
	}
	elapsed := w.elapsedMs
	if !w.completed {
		elapsed = time.Since(w.startedAt).Milliseconds()
	}
	return &p3ObservationWindowEvidence{
		Kind:       w.kind,
		DurationMs: w.duration.Milliseconds(),
		GraceMs:    w.grace.Milliseconds(),
		ElapsedMs:  elapsed,
		Completed:  w.completed,
		Trigger:    w.trigger,
	}
}
