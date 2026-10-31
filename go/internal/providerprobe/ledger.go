package providerprobe

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3Ledger is a scope's receivers in one sequential stream of intervals (DESIGN-SEQ): every
// receiver is registered when its world makes it and stays open to the scope's end, and each
// run's interval runs from the reading the previous run committed to its own. One case runs at a
// time and each run is reaped before the next starts, so every change in an interval is that
// run's. An RP01 side has a ledger of its own world only.
type p3Ledger struct {
	mu          sync.Mutex
	receivers   []*p3LedgerReceiver
	reads, runs int
	// poison is why the stream can no longer be attributed: a run that was not reaped, did not
	// settle or did not finish. Every later interval and the scope are RECEIVER_FAULT.
	poison  []string
	aborted bool
}

type p3LedgerReceiver struct {
	r *providercert.Receiver
	// source is the run whose world made it.
	source string
	last   providercert.ReceiverSnapshot
}

// Settle (r5 deadlines): every receiver drained within p3SettleDrain, then two readings
// p3SettleGap apart that agree.
var (
	p3SettleDrain = 2 * time.Second
	p3SettleGap   = 500 * time.Millisecond
)

// add registers a world's receivers at a zero baseline.
func (l *p3Ledger) add(source string, rs ...*providercert.Receiver) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range rs {
		l.receivers = append(l.receivers, &p3LedgerReceiver{r: r, source: source})
	}
}

func (l *p3Ledger) poisoned() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.poison) > 0
}

func p3SameCounts(a, b providercert.ReceiverSnapshot) bool {
	return a.Conns == b.Conns && a.Bytes == b.Bytes && a.Total == b.Total && a.Dropped == b.Dropped
}

// p3Settle reads every receiver once all are drained (at most p3SettleDrain) and again after
// p3SettleGap; the second reading counts only when both agree and every receiver was drained.
func p3Settle(rs []*p3LedgerReceiver) ([]providercert.ReceiverSnapshot, bool) {
	drained := func() bool {
		for _, r := range rs {
			if !r.r.Observer.Drained() {
				return false
			}
		}
		return true
	}
	for deadline := time.Now().Add(p3SettleDrain); !drained() && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
	}
	read := func() []providercert.ReceiverSnapshot {
		out := make([]providercert.ReceiverSnapshot, len(rs))
		for i, r := range rs {
			out[i] = r.r.Read()
		}
		return out
	}
	first := read()
	time.Sleep(p3SettleGap)
	second := read()
	stable := true
	for i := range rs {
		stable = stable && first[i].Complete && second[i].Complete && p3SameCounts(first[i], second[i])
	}
	return second, stable
}

// p3Interval is what one receiver gained in an interval.
type p3Interval struct {
	r                       *p3LedgerReceiver
	conns, bytes            int64
	total, dropped          int
	seen                    []providercert.Seen
	owners                  []providercert.ConnOwner
	reading                 providercert.ReceiverSnapshot
	ownWorld, previousWorld bool
}

func (iv p3Interval) changed() bool {
	return iv.conns != 0 || iv.bytes != 0 || iv.total != 0 || iv.dropped != 0
}

func p3IntervalOf(r *p3LedgerReceiver, now providercert.ReceiverSnapshot) p3Interval {
	iv := p3Interval{r: r, reading: now, conns: now.Conns - r.last.Conns, bytes: now.Bytes - r.last.Bytes, total: now.Total - r.last.Total, dropped: now.Dropped - r.last.Dropped}
	for _, s := range now.Seen {
		if s.Sequence > r.last.Total {
			iv.seen = append(iv.seen, s)
		}
	}
	if len(now.Owners) > len(r.last.Owners) {
		iv.owners = now.Owners[len(r.last.Owners):]
	}
	return iv
}

// p3RunReap is how one run's processes ended, read once the run's own cleanup is over.
type p3RunReap struct {
	exe, configDir string
	roles          map[string]string
	deadline       time.Time
	aborted        bool
}

// p3ReapFailures are the reasons a run is not proven reaped (DESIGN-SEQ §4.2): any one poisons.
func p3ReapFailures(obs *p3Observed, reap p3RunReap) []string {
	var out []string
	if reap.aborted {
		out = append(out, "PROBE_CASE_ABORTED")
	}
	if obs.CleanupFailed {
		out = append(out, "CLEANUP_FAILED")
	}
	if obs.HeldProcess {
		out = append(out, "HELD_PROCESS")
	}
	if obs.JobUnconfirmed {
		out = append(out, "JOB_UNCONFIRMED")
	}
	if slices.ContainsFunc(obs.Runs, func(r p3RunEvidence) bool { return r.NativeStarted && r.Exit == app.ExitCodeUnknown }) {
		out = append(out, "NATIVE_EXIT_UNKNOWN")
	}
	if obs.HookProcs.Killed > 0 {
		out = append(out, "HOOK_PROCESS_KILLED")
	}
	if reap.configDir != "" {
		if left, complete := p3Census(reap.exe, reap.configDir, time.Time{}, reap.roles); !complete || len(left) > 0 {
			out = append(out, fmt.Sprintf("NATIVE_CENSUS_NOT_EMPTY (left %d, complete %t)", len(left), complete))
		}
	}
	return out
}

// p3PastDeadline is the run's deadline check; commit reads it once the interval has settled.
func p3PastDeadline(reap p3RunReap) bool {
	return !reap.deadline.IsZero() && time.Now().After(reap.deadline)
}

// p3Census is the config dir census the reap reads; a variable so a test can make it incomplete.
var p3Census = probeNativeCensus

// commit ends one run's interval (the deferred first statement of p3Runner.run, on every path):
// it proves the run reaped, settles every receiver, places each change (DESIGN-SEQ §3) into obs
// and commits the reading. own are the run's world's receivers; treatment is whether a change at
// an earlier world's receiver is this run's routing failure (a treatment interval) or only a
// fault (control, SAFETY, SAFE_REFUSAL).
func (l *p3Ledger) commit(obs *p3Observed, w *p3World, cleanups []p3CleanupNative, treatment bool, reap p3RunReap) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.runs++
	poisonedBefore := len(l.poison) > 0
	failures := p3ReapFailures(obs, reap)
	if reap.aborted {
		l.aborted = true
	}
	for _, f := range failures {
		l.poison = append(l.poison, obs.Run+": "+f)
		obs.RunErrors = append(obs.RunErrors, "UNREAPED: "+f)
	}
	if len(l.receivers) == 0 {
		if p3PastDeadline(reap) {
			l.poison = append(l.poison, obs.Run+": RUN_DEADLINE_EXCEEDED")
		}
		obs.ReceiverFault = true
		return
	}
	readings, stable := p3Settle(l.receivers)
	obs.ReceiversDrained = stable
	if p3PastDeadline(reap) {
		l.poison = append(l.poison, obs.Run+": RUN_DEADLINE_EXCEEDED")
		obs.RunErrors = append(obs.RunErrors, "UNREAPED: RUN_DEADLINE_EXCEEDED")
	}
	if !stable {
		l.poison = append(l.poison, obs.Run+": SETTLE_FAILED")
		obs.RunErrors = append(obs.RunErrors, "SETTLE_FAILED")
	} else {
		l.reads++
	}
	intervals := make([]p3Interval, len(l.receivers))
	for i, r := range l.receivers {
		intervals[i] = p3IntervalOf(r, readings[i])
		intervals[i].ownWorld = w != nil && slices.Contains(p3WorldReceivers(w), r.r)
		intervals[i].previousWorld = !intervals[i].ownWorld
		// The next interval starts here even when this one did not settle, so no change is
		// placed twice; the poison covers what this reading may have missed.
		r.last = readings[i]
	}
	if !reap.aborted && w != nil {
		p3PlaceInterval(obs, w, intervals, cleanups, treatment, poisonedBefore)
	}
	if len(l.poison) > 0 {
		obs.ReceiverFault = true
	}
}

// p3WorldReceivers are a world's five receivers, catch-all first.
func p3WorldReceivers(w *p3World) []*providercert.Receiver {
	return []*providercert.Receiver{w.catchAll.Receiver, w.sgA, w.sgB, w.settings, w.socket}
}

// p3Present is DESIGN-SEQ §3's contact: any of the receiver's categories moved, or this run's
// CATCH_ALL tunnelled to it.
func p3Present(iv p3Interval, tunnel int64) bool {
	return iv.conns > 0 || iv.bytes > 0 || iv.total > 0 || tunnel > 0
}

// p3PlaceInterval places one interval's changes (DESIGN-SEQ §3). The own world's wrong receivers
// are the run's Wrong and WrongSeen; its CATCH_ALL is the run's CatchAll; an earlier world's
// receiver that moved is prior contact, FAIL_ROUTING in a treatment interval and a receiver fault
// in any other, and only a fault once the stream is poisoned. Categories are presence only: no
// sum across receivers or categories counts one contact twice.
func p3PlaceInterval(obs *p3Observed, w *p3World, intervals []p3Interval, cleanups []p3CleanupNative, treatment, poisoned bool) {
	byReceiver := map[*providercert.Receiver]p3Interval{}
	for _, iv := range intervals {
		byReceiver[iv.r.r] = iv
		if iv.dropped > 0 {
			obs.ReceiverFault = true
			obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("RECEIVER_DROPPED: %s dropped %d", iv.r.r.Observer.ID, iv.dropped))
		}
	}
	proxy := byReceiver[w.catchAll.Receiver]
	obs.CatchAll = proxy.seen
	obs.Dropped += proxy.dropped
	obs.Wrong, obs.WrongSeen = map[string]providercert.Wrong{}, map[string][]providercert.Seen{}
	obs.Owners = map[string][]providercert.ConnOwner{}
	own := []*providercert.Receiver{w.sgA, w.sgB, w.settings, w.socket}
	ownReadings := make([]providercert.ReceiverSnapshot, 0, len(own))
	for _, r := range own {
		iv := byReceiver[r]
		ownReadings = append(ownReadings, iv.reading)
		id := r.Observer.ID
		obs.WrongSeen[id] = iv.seen
		if len(iv.owners) > 0 {
			obs.Owners[id] = slices.Clone(iv.owners)
		}
		// In-process proxy delivery keeps the native's accept identity, not a probe dial: one
		// owner per proxy connection.
		proxied := map[int64]bool{}
		for _, s := range iv.seen {
			if c := s.Connection; c != nil && c.Server == strings.TrimPrefix(w.catchAll.URL, "http://") && !proxied[c.ID] {
				proxied[c.ID] = true
				obs.Owners[id] = append(obs.Owners[id], c.Owner)
			}
		}
		contact := providercert.Wrong{Bytes: iv.bytes, Conns: iv.conns}
		tunnel := p3ProxyContacts(obs.CatchAll, r.URL, iv.seen)
		if p3Present(iv, tunnel) && contact.Bytes == 0 && contact.Conns == 0 {
			// Seen only (in-process proxy delivery) or a tunnel: still a contact Judge reads.
			contact.Conns = 1
		}
		obs.Wrong[id] = contact
		obs.Dropped += iv.dropped
		obs.Intervals = append(obs.Intervals, p3IntervalEvidence{Receiver: id, Source: obs.Run, Conns: iv.conns, Bytes: iv.bytes, Total: iv.total, Dropped: iv.dropped, Tunnel: tunnel})
	}
	if !p3ProxyClassified(w.catchAll.URL, proxy.reading, ownReadings...) {
		obs.ReceiverFault = true
	}
	var prior providercert.Wrong
	for _, iv := range intervals {
		if !iv.previousWorld {
			continue
		}
		tunnel := p3ProxyContacts(obs.CatchAll, iv.r.r.URL, iv.seen)
		if !p3Present(iv, tunnel) {
			continue
		}
		c := p3PriorContact{Receiver: iv.r.r.Observer.ID, URL: iv.r.r.URL, Source: iv.r.source, Bytes: iv.bytes, Conns: iv.conns, Tunnel: tunnel, Total: iv.total, Owners: iv.owners}
		obs.PriorContacts = append(obs.PriorContacts, c)
		obs.Intervals = append(obs.Intervals, p3IntervalEvidence{Receiver: c.Receiver, Source: c.Source, Conns: iv.conns, Bytes: iv.bytes, Total: iv.total, Dropped: iv.dropped, Tunnel: tunnel})
		if !treatment || poisoned {
			obs.ReceiverFault = true
			obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("PRIOR_RECEIVER_CONTACT: %s of %s", c.Receiver, c.Source))
			continue
		}
		prior.Bytes += c.Bytes
		prior.Conns++
	}
	obs.Wrong["PRIOR_RUN_RECEIVERS"] = prior
	if prior.Conns > 0 {
		obs.PriorCrossTalk = p3PriorCrossTalk(obs.PriorContacts, cleanups, obs.Run)
	}
}

// p3IntervalEvidence is one receiver's change in a run's interval, by category.
type p3IntervalEvidence struct {
	Receiver string `json:"receiver"`
	Source   string `json:"sourceRun"`
	Conns    int64  `json:"conns"`
	Bytes    int64  `json:"bytes"`
	Total    int    `json:"total"`
	Dropped  int    `json:"dropped"`
	Tunnel   int64  `json:"tunnel,omitempty"`
}

// conserve is the scope's end (DESIGN-SEQ §4.4, Sonnet C1): a final stable reading with every
// listener still open must equal the last committed one; only then are the listeners closed, and a
// reading after they drained must equal it again. With reads == runs, no drop and no poison, the
// stream is conserved; any other end is RECEIVER_FAULT.
func (l *p3Ledger) conserve() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var faults []string
	final, stable := p3Settle(l.receivers)
	if !stable {
		faults = append(faults, "FINAL_SETTLE_FAILED")
	}
	for i, r := range l.receivers {
		if !p3SameCounts(final[i], r.last) {
			faults = append(faults, fmt.Sprintf("CHANGED_AFTER_LAST_RUN: %s of %s", r.r.Observer.ID, r.source))
		}
	}
	faults = append(faults, p3CloseAndCheck(l.receivers)...)
	if l.reads != l.runs {
		faults = append(faults, fmt.Sprintf("READS_NOT_RUNS: %d reads, %d runs", l.reads, l.runs))
	}
	faults = append(faults, l.poison...)
	return faults
}

// p3CloseReceiver closes one receiver; its error includes a Serve that ended early. A variable so a
// test can fail it.
var p3CloseReceiver = func(r *providercert.Receiver) error { return r.Close() }

// p3CloseAndCheck closes the listeners only after the final reading, then requires every receiver
// to close cleanly (a listener that stopped serving early may have refused a contact), drain and
// read the same again, with nothing dropped.
func p3CloseAndCheck(rs []*p3LedgerReceiver) []string {
	var faults []string
	for _, r := range rs {
		if err := p3CloseReceiver(r.r); err != nil {
			faults = append(faults, fmt.Sprintf("CLOSE_FAILED: %s of %s: %v", r.r.Observer.ID, r.source, err))
		}
	}
	closed, stable := p3Settle(rs)
	if !stable {
		faults = append(faults, "CLOSE_NOT_DRAINED")
	}
	for i, r := range rs {
		if !p3SameCounts(closed[i], r.last) {
			faults = append(faults, fmt.Sprintf("CHANGED_AT_CLOSE: %s of %s", r.r.Observer.ID, r.source))
		}
		if closed[i].Dropped != 0 {
			faults = append(faults, "DROPPED: "+r.r.Observer.ID)
		}
	}
	return faults
}
