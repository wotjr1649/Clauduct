package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// LIFE06: the property the whole registry exists for. Cancelling one request must not
// reach any other. A subagent turn ending must not kill the main session.
func TestCancelDoesNotReachSiblings(t *testing.T) {
	r := newRegistry()

	idA, ctxA, releaseA, err := r.admit(context.Background())
	if err != nil {
		t.Fatalf("admit A: %v", err)
	}
	defer releaseA()
	_, ctxB, releaseB, err := r.admit(context.Background())
	if err != nil {
		t.Fatalf("admit B: %v", err)
	}
	defer releaseB()
	_, ctxC, releaseC, err := r.admit(context.Background())
	if err != nil {
		t.Fatalf("admit C: %v", err)
	}
	defer releaseC()

	cause := errors.New("user cancelled A")
	if !r.cancel(idA, cause) {
		t.Fatal("cancel reported A was not registered")
	}

	if ctxA.Err() == nil {
		t.Error("A was not cancelled")
	}
	if got := context.Cause(ctxA); !errors.Is(got, cause) {
		t.Errorf("A cause = %v, want the supplied cause", got)
	}
	if ctxB.Err() != nil {
		t.Errorf("B was cancelled by A's cancellation: %v", context.Cause(ctxB))
	}
	if ctxC.Err() != nil {
		t.Errorf("C was cancelled by A's cancellation: %v", context.Cause(ctxC))
	}
	if got := r.count(); got != 2 {
		t.Errorf("active = %d, want 2", got)
	}
}

// A cancelled parent must still reach its child request: per-request isolation is about
// siblings, not about escaping the session that owns them.
func TestParentCancellationReachesTheRequest(t *testing.T) {
	r := newRegistry()
	parent, cancelParent := context.WithCancel(context.Background())

	_, ctx, release, err := r.admit(parent)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	defer release()

	cancelParent()
	<-ctx.Done()
}

// LIFE07: release is reached from a handler's defer while a cancel path may be touching
// the same entry. "Already gone" has to be ordinary.
func TestReleaseIsIdempotentAndCancelAfterReleaseIsFalse(t *testing.T) {
	r := newRegistry()
	id, ctx, release, err := r.admit(context.Background())
	if err != nil {
		t.Fatalf("admit: %v", err)
	}

	release()
	release()
	release()

	if ctx.Err() == nil {
		t.Error("release must cancel the request context, or its goroutines and timers leak")
	}
	if r.cancel(id, errors.New("late")) {
		t.Error("cancel reported success for a request that had already finished")
	}
	if got := r.count(); got != 0 {
		t.Errorf("active = %d after release, want 0", got)
	}
}

// Cancelling something that never existed, or twice, is a normal late arrival.
func TestCancelUnknownIsFalseNotPanic(t *testing.T) {
	r := newRegistry()
	if r.cancel(99999, errors.New("x")) {
		t.Error("cancel reported success for an id that was never admitted")
	}
	id, _, _, err := r.admit(context.Background())
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !r.cancel(id, errors.New("first")) {
		t.Error("first cancel should report success")
	}
	if r.cancel(id, errors.New("second")) {
		t.Error("second cancel should report the request was already gone")
	}
}

// LIFE07 under real concurrency. Run with -race this is the test that catches a registry
// that is correct only when nothing happens at once.
func TestConcurrentAdmitCancelRelease(t *testing.T) {
	r := newRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _, release, err := r.admit(context.Background())
			if err != nil {
				return // the cap or a close; both are legitimate answers here
			}
			var inner sync.WaitGroup
			inner.Add(2)
			go func() { defer inner.Done(); r.cancel(id, errors.New("racing cancel")) }()
			go func() { defer inner.Done(); release() }()
			inner.Wait()
			release()
		}()
	}
	wg.Wait()

	if got := r.count(); got != 0 {
		t.Errorf("active = %d after every request finished, want 0", got)
	}
}

// Shutdown must refuse new work before cancelling what is in flight. Admitting between
// those two steps would produce a request nothing ever cancels, holding exit open.
func TestCloseAllRefusesThenCancels(t *testing.T) {
	r := newRegistry()
	_, ctxA, releaseA, _ := r.admit(context.Background())
	defer releaseA()
	_, ctxB, releaseB, _ := r.admit(context.Background())
	defer releaseB()

	if got := r.closeAll(errShuttingDown); got != 2 {
		t.Errorf("closeAll cancelled %d, want 2", got)
	}
	for name, ctx := range map[string]context.Context{"A": ctxA, "B": ctxB} {
		if ctx.Err() == nil {
			t.Errorf("%s survived shutdown", name)
		}
		if got := context.Cause(ctx); !errors.Is(got, errShuttingDown) {
			t.Errorf("%s cause = %v, want the shutdown cause so a handler can tell it from a user cancel", name, got)
		}
	}
	if _, _, _, err := r.admit(context.Background()); !errors.Is(err, errGatewayClosed) {
		t.Errorf("admit after close = %v, want errGatewayClosed", err)
	}
	if got := r.count(); got != 0 {
		t.Errorf("active = %d after close, want 0", got)
	}
}

// An explicit ceiling on in-flight requests, so a runaway client cannot make this process
// hold unbounded per-request state.
func TestActiveRequestsAreCapped(t *testing.T) {
	r := newRegistry()
	releases := make([]func(), 0, maxActiveRequests)
	for i := 0; i < maxActiveRequests; i++ {
		_, _, release, err := r.admit(context.Background())
		if err != nil {
			t.Fatalf("admit %d of %d: %v", i, maxActiveRequests, err)
		}
		releases = append(releases, release)
	}

	if _, _, _, err := r.admit(context.Background()); !errors.Is(err, errTooManyRequests) {
		t.Fatalf("admit past the cap = %v, want errTooManyRequests", err)
	}

	releases[0]()
	if _, _, release, err := r.admit(context.Background()); err != nil {
		t.Fatalf("admit after a slot freed: %v", err)
	} else {
		release()
	}
	for _, release := range releases[1:] {
		release()
	}
}
