package providerprobe

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
)

const p3ObservationQuiescenceGrace = 3 * time.Second

type p3R03Observer struct {
	c       p3Case
	sidecar string
	run     int
	gateway atomic.Int64
	window  *p3ObservationWindow
	mu      sync.Mutex
	active  map[[2]int64]bool
}

func (o *p3R03Observer) enter(in p3Call) {
	if in.run != o.run || in.gateway != o.gateway.Load() || !p3R03Main(in) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active == nil {
		o.active = map[[2]int64]bool{}
	}
	o.active[[2]int64{in.gateway, in.seq}] = true
}

func (o *p3R03Observer) quiescent() bool { o.mu.Lock(); defer o.mu.Unlock(); return len(o.active) == 0 }

// Arm only after this run's root-main refusal and its executed helper are both proven.
func (o *p3R03Observer) observe(in p3Call, record gateway.RequestRecord) {
	defer func() { o.mu.Lock(); delete(o.active, [2]int64{in.gateway, in.seq}); o.mu.Unlock() }()
	if in.run != o.run || in.gateway != o.gateway.Load() || in.seq <= 0 || in.seq != record.Seq || !p3R03Main(in) ||
		!slices.Contains(in.canaries, "turn1") || record.Method != "POST" || record.Path != in.path || record.Status != 503 ||
		record.Category != "NATIVE_CONFIRMATION_TIMEOUT" || record.NativeRouteScope != "conversation" {
		return
	}
	in.confirmationSHA256 = record.ConfirmationSHA256
	receipts, complete := p3ReadVariantReceipts(o.sidecar)
	if !p3VariantExercised(o.c, &p3Observed{PluginVariantReceipts: receipts, PluginVariantReceiptsComplete: complete}, in) {
		return
	}
	o.window.start(p3ObservationTrigger{Run: in.run, Gateway: in.gateway, Seq: in.seq, Canary: "turn1", ConfirmationSHA256: in.confirmationSHA256, Status: record.Status, Category: record.Category})
}

func p3WindowCompleted(window *p3ObservationWindowEvidence) bool {
	return window != nil && window.Completed && window.DurationMs == p3ObservationWindowDuration.Milliseconds() &&
		window.ElapsedMs >= window.DurationMs && window.GraceMs >= 0 && (window.GraceMs == 0 || window.ElapsedMs <= window.DurationMs+window.GraceMs)
}

func p3R03WindowProved(window *p3ObservationWindowEvidence, requests []p3R03Request) bool {
	if !p3WindowCompleted(window) || window.Kind != "R03_REFUSAL" {
		return false
	}
	if window.GraceMs != p3ObservationQuiescenceGrace.Milliseconds() {
		return false
	}
	t := window.Trigger
	for _, r := range requests {
		if r.VariantVerified && t.Run == r.Run && t.Gateway == r.Gateway && t.Seq == r.Seq && t.Connection == 0 &&
			t.Canary == r.Turn && t.Status == r.Status && t.Category == r.Category && t.ConfirmationSHA256 == r.ConfirmationSHA256 {
			return true
		}
	}
	return false
}
