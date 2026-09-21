package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

type heldCounter struct {
	upstream.None
	started chan struct{}
	release chan struct{}
	count   atomic.Int64
	err     error
}

func (f *heldCounter) Count(ctx context.Context, _ upstream.Call) (int64, error) {
	if f.count.Add(1) == 1 {
		close(f.started)
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-f.release:
		return 100, f.err
	}
}

func TestCountSharesConcurrentInputAndWaiterCancellation(t *testing.T) {
	for _, fails := range []bool{false, true} {
		f := &heldCounter{started: make(chan struct{}), release: make(chan struct{})}
		if fails {
			f.err = upstream.ErrCountFailed
		}
		g := startWith(t, f)
		request, err := anthropic.DecodeRequest([]byte(strings.Replace(validRequest, `"messages":`, `"tools":[{"name":"proof","input_schema":{"type":"object"}}],"messages":`, 1)))
		if err != nil {
			t.Fatal(err)
		}
		built, err := bridge.BuildRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(built)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _, _, err := g.countInput(ctx, built, raw, request.Model)
			if (err != nil) != fails {
				t.Error("owner result mismatch")
			}
		})
		<-f.started
		cancelled, stop := context.WithCancel(ctx)
		stop()
		if _, _, _, err := g.countInput(cancelled, built, raw, request.Model); err != context.Canceled {
			t.Fatal("waiter cancellation lost")
		}
		waiter := make(chan struct{})
		wg.Go(func() {
			close(waiter)
			_, _, _, err := g.countInput(ctx, built, raw, request.Model)
			if (err != nil) != fails {
				t.Error("waiter result mismatch")
			}
		})
		<-waiter
		// The owner is held; let the waiter enter the shared-count branch.
		time.Sleep(20 * time.Millisecond)
		close(f.release)
		wg.Wait()
		if f.count.Load() != 1 {
			t.Fatal("identical concurrent request counted twice")
		}
	}
}

func TestCountDriftInvalidatesCacheAndQuarantinesOnlyAffectedModel(t *testing.T) {
	for _, method := range []string{localCount, backendCount} {
		g, f := newContextFixture(t)
		r := g.ring.open("POST", "/v1/messages")
		r.route("sol", "gpt-5.6-sol", "low", "direct")
		r.preflight(100, method, 0)
		r.countMethod(method)
		r.usage(codex.Usage{InputKnown: true, InputTokens: 101})
		if err := g.verifyCount(r); err == nil {
			t.Fatal("drift accepted")
		}
		if r.snapshot().CountAgreement != "mismatched" {
			t.Fatal("drift not diagnosed")
		}
		response := contextRequest(t, g, "gpt-5.6-sol", "after drift")
		bodyText(t, response)
		if response.StatusCode != 200 || f.counts.Load() != 0 || f.Calls() != 1 {
			t.Fatal("optional count quarantine blocked generation")
		}
		response = countPost(t, g, "gpt-5.6-sol", "explicit new count")
		body := bodyText(t, response)
		if method == backendCount {
			if response.StatusCode != 400 || f.counts.Load() != 0 || f.Calls() != 1 || !strings.Contains(body, "COUNT_SOURCE_QUARANTINED") {
				t.Fatal("unverified backend count dispatched")
			}
		} else if response.StatusCode != 200 || f.counts.Load() != 1 {
			t.Fatal("local quarantine did not use backend")
		}
		response = countPost(t, g, "gpt-5.6-terra", "independent")
		bodyText(t, response)
		if response.StatusCode != 200 || f.counts.Load() == 0 {
			t.Fatal("unrelated model counter quarantined")
		}
	}
}

func TestUnsupportedRouteExplainsAvailableChoices(t *testing.T) {
	message := refusalMessage("UNSUPPORTED_MODEL_OR_EFFORT")
	for _, model := range bridge.Models {
		if !strings.Contains(message, model.ID) {
			t.Fatal("model choice omitted")
		}
	}
	for _, effort := range bridge.Efforts {
		if !strings.Contains(message, effort) {
			t.Fatal("effort choice omitted")
		}
	}
}

func TestBackendCountFailureNeverFallsBackToLocalText(t *testing.T) {
	f := &heldCounter{started: make(chan struct{}), release: make(chan struct{}), err: upstream.ErrCountFailed}
	close(f.release)
	g := startWith(t, f)
	request, err := anthropic.DecodeRequest([]byte(validRequest))
	if err != nil {
		t.Fatal(err)
	}
	built, err := bridge.BuildRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(built)
	_, _, method, err := g.countInput(context.Background(), built, raw, request.Model)
	if err != upstream.ErrCountFailed || method != backendCount || f.count.Load() != 1 {
		t.Fatal("local fallback masked exact backend failure")
	}
}

func TestCountInFlightCannotOverwriteCompletedUsage(t *testing.T) {
	f := &heldCounter{started: make(chan struct{}), release: make(chan struct{})}
	g := startWith(t, f)
	request, err := anthropic.DecodeRequest([]byte(validRequest))
	if err != nil {
		t.Fatal(err)
	}
	built, err := bridge.BuildRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(built)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, _, _, err := g.countInput(ctx, built, raw, request.Model); finished <- err }()
	select {
	case <-f.started:
	case <-ctx.Done():
		t.Fatal("counter not started")
	}
	r := g.ring.open("POST", "/v1/messages")
	r.route(request.Model, built.Model, built.Effort.Effort, "direct")
	r.usage(codex.Usage{InputKnown: true, InputTokens: 101})
	g.rememberUsage(raw, r)
	close(f.release)
	if err := <-finished; err == nil {
		t.Fatal("mismatched warmup claimed exactness")
	}
	n, _, method, err := g.countInput(ctx, built, raw, request.Model)
	if err != nil || n != 101 || method != "backend-usage" || f.count.Load() != 1 {
		t.Fatal("older warmup replaced measured usage")
	}
}
