package providerprobe

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// RP01 timing (admission r2 foregroundConditions.cases.RP01.timing): a 70-minute observation
// plan inside a 120-minute run, one native start per configuration, no retries.
const (
	rp01Wait = 70 * time.Minute
	rp01Run  = 120 * time.Minute
	// rp01TreatmentDelay places the treatment's end after the control's refresh plus 15 minutes:
	// the control refreshes about 60 minutes after its start, and a MULTI_TURN or TUI treatment
	// ends a little over 70 minutes after its own.
	rp01TreatmentDelay = 10 * time.Minute
)

// p3RP01Case is RP01 for one scope, with its own synthetic policy source: generation 1 at
// start, generation 2 once turn 1's canary reached any receiver (MULTI_TURN: at the step).
func p3RP01Case(scope string) (p3Case, *p3RemotePolicy) {
	rp := &p3RemotePolicy{gen: 1}
	c := p3Case{ID: "RP01-" + scope, Entry: p3Multi, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, Purpose: "SUPPRESSION",
		Setup: func(w *p3World) error {
			rp.marks = map[int]providercert.Canary{1: w.addCanary("rp-gen1"), 2: w.addCanary("rp-gen2")}
			w.remotePolicy, w.loginLifetime = rp, 4*time.Hour
			return w.storeGateway(w.sgA)
		},
		Steps:       []func(*p3World) error{func(*p3World) error { rp.set(2); return nil }},
		Settle:      rp01Wait,
		Established: gotCanary("WRONG_SETTINGS", 1),
	}
	whenTurn1Seen := func(setup func(*p3World) error) func(*p3World) error {
		return func(w *p3World) error {
			if err := setup(w); err != nil {
				return err
			}
			go func() {
				for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
					if p3CanarySeenAnywhere(w, "turn1") {
						rp.set(2)
						return
					}
				}
			}()
			return nil
		}
	}
	switch scope {
	case app.ScopeTUI:
		c.Entry, c.Turns, c.Steps, c.TUIRefreshApproval = p3TUI, 2, nil, true
		c.Setup = whenTurn1Seen(c.Setup)
	case app.ScopePrintSingle:
		// One -p turn that outlasts the wait: a scripted Bash sleep loop, served by INTENDED
		// (Codex stream) in the treatment and by the stored gateway (Anthropic stream) in the
		// control. Its run limit is the RP01 run limit, not the wait.
		per := 590 * time.Second
		rounds := int((rp01Wait + per - time.Second) / per)
		seconds := int(per.Seconds())
		c.Entry, c.Steps, c.Args, c.Settle = p3Print, nil, []string{"--allowedTools", "Bash"}, rp01Run-time.Minute
		codex := p3SleepLoop(rounds, seconds, "function_call_output", func(id, input string) string { return toolStream(id, "Bash", input) })
		c.Script = func(body string, _ []string) string { return codex(body) }
		setup := c.Setup
		c.Setup = whenTurn1Seen(func(w *p3World) error {
			if err := setup(w); err != nil {
				return err
			}
			w.wrongScript = p3SleepLoop(rounds, seconds, `"tool_result"`, func(id, input string) string { return anthropicToolUse(id, "Bash", input) })
			return nil
		})
	}
	return c, rp
}

// p3SeenAt is the first time a canary with this id reached INTENDED's backend or any receiver.
func p3SeenAt(obs *p3Observed, id string) (time.Time, bool) {
	var first time.Time
	note := func(at time.Time) {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	for _, c := range obs.Ingress {
		if slices.Contains(c.canaries, id) {
			note(c.at)
		}
	}
	for _, seen := range obs.WrongSeen {
		for _, s := range seen {
			if slices.Contains(s.Canaries, id) {
				note(s.At)
			}
		}
	}
	return first, !first.IsZero()
}

// RP01Outcome is one RP01 run's judgment (admission r2 caseVerdict.entries.rp01).
type RP01Outcome struct {
	Verdict, Label, Source string
	// ControlRefreshedAt is the control's generation-2 fetch; zero when it never refreshed.
	ControlRefreshedAt time.Time
	Reasons            []string
}

// p3RP01Final is RP01's final observation (DESIGN-SEQ §6): after both sides ended and were reaped,
// both worlds settle and are read once more with every listener open, and only then close.
type p3RP01Final struct {
	// Faults keep RP01 INCONCLUSIVE: a final reading that did not settle, a change at close, a
	// side whose interval was not read or was poisoned.
	Faults []string
	// Control is what the control world's receivers gained after its commit; ControlReadings are
	// those receivers as finally read, for the owner binding.
	Control         []p3Interval
	ControlReadings []providercert.ReceiverSnapshot
	// TreatmentReadings are the treatment world's receivers as finally read (CATCH_ALL first): its
	// whole run, also when its side aborted and placed nothing.
	TreatmentReadings []providercert.ReceiverSnapshot
}

// p3RP01Observe is the final observation over both sides' ledgers.
func p3RP01Observe(control, treatment *p3Ledger) p3RP01Final {
	control.mu.Lock()
	defer control.mu.Unlock()
	treatment.mu.Lock()
	defer treatment.mu.Unlock()
	var f p3RP01Final
	all := append(slices.Clone(control.receivers), treatment.receivers...)
	readings, stable := p3Settle(all)
	if !stable {
		f.Faults = append(f.Faults, "FINAL_SETTLE_FAILED")
	}
	for i, r := range all {
		iv := p3IntervalOf(r, readings[i])
		if i < len(control.receivers) {
			f.Control, f.ControlReadings = append(f.Control, iv), append(f.ControlReadings, readings[i])
		} else {
			f.TreatmentReadings = append(f.TreatmentReadings, readings[i])
		}
		r.last = readings[i]
	}
	// Each world's CATCH_ALL is classified again on its final reading: a connection accepted after
	// the side's commit that produced no kept request (a bare TCP connection, a partial header) is
	// not an observed request and leaves RP01 INCONCLUSIVE.
	for side, world := range map[string][]providercert.ReceiverSnapshot{p3Unprotected: readings[:len(control.receivers)], p3HostManaged: readings[len(control.receivers):]} {
		rs := control.receivers
		if side == p3HostManaged {
			rs = treatment.receivers
		}
		if len(rs) != 5 || !p3ProxyClassified(rs[0].r.URL, world[0], world[1:]...) {
			f.Faults = append(f.Faults, side+": FINAL_CATCH_ALL_UNCLASSIFIED")
		}
	}
	f.Faults = append(f.Faults, p3CloseAndCheck(all)...)
	for side, l := range map[string]*p3Ledger{p3Unprotected: control, p3HostManaged: treatment} {
		if l.reads != l.runs || l.runs != 1 {
			f.Faults = append(f.Faults, fmt.Sprintf("%s: %d reads, %d runs", side, l.reads, l.runs))
		}
		f.Faults = append(f.Faults, l.poison...)
	}
	return f
}

// p3RP01OwnerBound is the final observation's owner binding: every connection any control-world
// receiver accepted, by the raw accept owner and by each record's connection, is the control's own
// native generation (PRINT and MULTI: the direct native; TUI: the Claude process the driver
// started). Another owner, an unknown one, an unknown generation or more connections than recorded
// owners (the Counter keeps 256) leaves RP01 INCONCLUSIVE.
func p3RP01OwnerBound(control *p3Observed, readings []providercert.ReceiverSnapshot) bool {
	if control == nil || len(control.Runs) != 1 || control.Runs[0].NativePID <= 0 || control.Runs[0].NativeCreationFiletime == 0 {
		return false
	}
	run := control.Runs[0]
	for _, r := range readings {
		if r.Conns > int64(len(r.Owners)) {
			return false
		}
		for _, o := range r.Owners {
			if !p3OwnerMatchesNativeRun(o.PID, o.Created, run) {
				return false
			}
		}
		for _, s := range r.Seen {
			if s.Connection != nil && !p3OwnerMatchesNativeRun(s.Connection.Owner.PID, s.Connection.Owner.Created, run) {
				return false
			}
		}
	}
	return true
}

// p3OwnerMatchesNativeRun accepts only the exact process generation that started this run.
// TCP owner times are Unix nanoseconds; the launcher records the same Windows FILETIME ticks.
// RP01 only: a worker of the control's native has another generation and is INCONCLUSIVE.
func p3OwnerMatchesNativeRun(ownerPID uint32, ownerCreated time.Time, run p3RunEvidence) bool {
	if ownerPID == 0 || ownerCreated.IsZero() || run.NativePID <= 0 || run.NativeCreationFiletime == 0 || uint64(run.NativePID) != uint64(ownerPID) {
		return false
	}
	nanos := ownerCreated.UnixNano()
	if nanos <= 0 || nanos%100 != 0 {
		return false
	}
	const windowsEpochTicks = uint64(116444736000000000)
	return uint64(nanos/100)+windowsEpochTicks == run.NativeCreationFiletime
}

// p3RP01TreatmentRouting is the treatment's routing over its whole run and its final interval:
// any wrong receiver of its world that moved, and r4 P3 at its own CATCH_ALL (an inference or a
// canary fails routing; an undecodable record is INCONCLUSIVE; any other request is allowed). A
// CATCH_ALL record naming a control-world receiver's authority without inference or canary is
// cross-world contact: INCONCLUSIVE. The world's final reading is read too, so a treatment whose
// run returned no observation (an aborted side) is still judged on what its receivers saw.
func p3RP01TreatmentRouting(treatment *p3Observed, final p3RP01Final, controlURLs []string) (fail, inconclusive []string) {
	var records []providercert.Seen
	if treatment != nil {
		for _, id := range []string{"WRONG_STORED_GATEWAY_A", "WRONG_STORED_GATEWAY_B", "WRONG_SETTINGS", "WRONG_SOCKET"} {
			if w := treatment.Wrong[id]; w.Conns != 0 || w.Bytes != 0 {
				fail = append(fail, "treatment reached "+id)
			}
		}
		if p3WrongBytes(treatment) != 0 {
			fail = append(fail, "treatment wrong bytes")
		}
		records = slices.Clone(treatment.CatchAll)
	}
	// The world's whole final reading: its run and anything after its commit. Its CATCH_ALL record
	// already holds what the run's CatchAll holds, so it replaces it and nothing is counted twice.
	for i, r := range final.TreatmentReadings {
		if i == 0 {
			records = slices.Clone(r.Seen)
		} else if r.Conns != 0 || r.Bytes != 0 || r.Total != 0 {
			fail = append(fail, fmt.Sprintf("treatment world receiver %d was reached", i))
		}
	}
	inference, canary, undecodable := p3CatchAllP3(records)
	if inference > 0 || canary > 0 {
		fail = append(fail, fmt.Sprintf("treatment CATCH_ALL: %d inference, %d canary", inference, canary))
	}
	if undecodable > 0 {
		inconclusive = append(inconclusive, providercert.ReasonUndecodable)
	}
	for _, s := range records {
		if slices.ContainsFunc(controlURLs, func(u string) bool { return p3AuthorityNamesReceiver(s.Authority, u) }) {
			inconclusive = append(inconclusive, "CROSS_WORLD_PROXY_CONTACT")
			break
		}
	}
	return fail, inconclusive
}

// p3RP01Violation is a confirmed routing failure: the treatment's own contact or P3 at its
// CATCH_ALL, a treatment policy fetch, or a short-entry case that failed routing. It is
// FAIL_ROUTING ahead of everything else, an unrefreshed control and an aborted side included.
func p3RP01Violation(treatment *p3Observed, treatmentFetches []p3PolicyFetch, short *app.AdmissionEntry, final p3RP01Final, controlURLs []string) []string {
	fail, _ := p3RP01TreatmentRouting(treatment, final, controlURLs)
	if len(treatmentFetches) != 0 {
		fail = append(fail, fmt.Sprintf("treatment policy fetches %d", len(treatmentFetches)))
	}
	if short != nil && slices.ContainsFunc(short.Cases, func(c app.AdmissionCase) bool {
		return c.Verdict == providercert.FailRouting || c.Verdict == VerdictBackgroundEntryReached
	}) {
		fail = append(fail, "a short-entry treatment case reached a wrong destination")
	}
	return fail
}

// judgeRP01 applies admission r2's RP01 rules to a control and a treatment run, their final
// observation and the scope's short entry under the same key.
func judgeRP01(control, treatment *p3Observed, controlFetches, treatmentFetches []p3PolicyFetch, treatmentEnded time.Time, short *app.AdmissionEntry, final p3RP01Final, controlURLs []string) RP01Outcome {
	out := RP01Outcome{Source: "NOT_RUN"}
	// A confirmed routing violation is FAIL_ROUTING whatever the control showed.
	if fail := p3RP01Violation(treatment, treatmentFetches, short, final, controlURLs); len(fail) > 0 {
		out.Verdict, out.Reasons = providercert.FailRouting, fail
		return out
	}
	_, unproven := p3RP01TreatmentRouting(treatment, final, controlURLs)
	// The control shows the refresh path working: V1 fetched and applied, then V2 fetched and
	// applied by a later request.
	var v1, v2 time.Time
	for _, f := range controlFetches {
		if f.Served == 1 && v1.IsZero() {
			v1 = f.At
		}
		if f.Served == 2 && v2.IsZero() {
			v2 = f.At
		}
	}
	v1Applied, v1ok := p3SeenAt(control, "rp-gen1")
	v2Applied, v2ok := p3SeenAt(control, "rp-gen2")
	if v1.IsZero() || !v1ok || v1Applied.Before(v1) || v2.IsZero() || !v2ok || v2Applied.Before(v2) {
		out.Verdict = providercert.Inconclusive
		out.Reasons = []string{fmt.Sprintf("the control did not show the refresh (V1 fetch %t applied %t, V2 fetch %t applied %t)", !v1.IsZero(), v1ok, !v2.IsZero(), v2ok && !v2Applied.Before(v2))}
		return out
	}
	out.ControlRefreshedAt, out.Source = v2, "ACTIVE_REFRESHED"
	controlLate := slices.ContainsFunc(final.Control, func(iv p3Interval) bool { return iv.changed() })
	switch {
	case len(final.Faults) > 0:
		out.Verdict, out.Reasons = providercert.Inconclusive, append([]string{providercert.ReasonReceiverFault}, final.Faults...)
	case len(unproven) > 0:
		out.Verdict, out.Reasons = providercert.Inconclusive, unproven
	case controlLate:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"CONTROL_WORLD_CHANGED_AFTER_CONTROL"}
	case !p3RP01OwnerBound(control, final.ControlReadings):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"CONTROL_OWNER_UNBOUND"}
	case treatment.ReceiverFault || control.ReceiverFault:
		// An incomplete receiver snapshot may hide a contact: no SOURCE_INACTIVE from it.
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{providercert.ReasonReceiverFault}
	case treatment.CleanupFailed || control.CleanupFailed || treatment.HeldProcess || control.HeldProcess || treatment.JobUnconfirmed || control.JobUnconfirmed:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{providercert.ReasonCleanupFailed}
	case treatment.TimedOut || treatment.SpawnFailed || treatment.IdentityError || treatment.Dropped != 0:
		// The treatment did not run to its own end as planned: its requests are not a
		// completed session (caseVerdict.entries.rp01: TIMEOUT and the like are INCONCLUSIVE).
		out.Verdict = providercert.Inconclusive
		out.Reasons = []string{fmt.Sprintf("the treatment run is not complete (timed out %t, spawn failed %t, identity error %t, dropped %d)", treatment.TimedOut, treatment.SpawnFailed, treatment.IdentityError, treatment.Dropped)}
	case treatmentEnded.Before(v2.Add(15 * time.Minute)):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"the treatment's observation ended before the control's refresh plus 15 minutes"}
	case !slices.ContainsFunc(treatment.Calls, func(c p3Call) bool { return c.at.After(v2) }):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"no treatment request was served at INTENDED after the control's refresh"}
	case len(treatment.OverlayGaps) > 0:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"RC11_OVERLAY_GAP:" + strings.Join(treatment.OverlayGaps, ",")}
	case short == nil:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"no short entry under the same key"}
	case !p3ShortAdmitted(short):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"SHORT_ENTRY_NOT_ADMITTED: " + short.State}
	default:
		out.Verdict, out.Label, out.Source = providercert.Pass, "SOURCE_INACTIVE", "SOURCE_INACTIVE"
		out.Reasons = []string{"treatment: no start fetch and no periodic refresh of the stored gateway's managed settings", "treatment stored gateway connections 0; short entry ADMITTED with all " + fmt.Sprint(len(short.Cases)) + " cases"}
	}
	return out
}

// p3ShortAdmitted is the same-key short entry RP01 builds on: ADMITTED, each of the scope's short
// cases exactly once, and its cases admitting the scope under Admit's own case rules (admitCases):
// a required case NOT_EXERCISABLE, PATH_ABSENT outside a CLI background-entry check, a SAFETY or
// SAFE_REFUSAL verdict outside its purpose, or a disposition not valid for its case, verdict, reason,
// platform, native or gate, does not admit, and every case's stored label must be the one Admit
// gives. Each case's purpose and CLI background kind (scopeCase.cliBackground) are the registry's,
// the same the run that wrote the entry read.
func p3ShortAdmitted(short *app.AdmissionEntry) bool {
	if short == nil || short.State != app.AdmissionAdmitted {
		return false
	}
	scope := short.Key.Scope
	if len(short.Cases) != len(admissioncontract.ShortCases(scope)) {
		return false
	}
	registry := caseRegistry(scope)
	results := map[string]CaseResult{}
	for _, c := range short.Cases {
		if _, twice := results[c.ID]; twice {
			return false
		}
		r := CaseResult{Verdict: c.Verdict, Reasons: c.Reasons}
		if build, ok := registry[c.ID]; ok {
			sc := build()
			r.Purpose, r.CLIBackgroundEntry = sc.c.Purpose, sc.cliBackground
		}
		results[c.ID] = r
	}
	in := AdmissionInput{Scope: scope, Results: results, NativeSHA256: short.Key.NativeSHA256, Platform: runtime.GOOS, GatesPassed: scopeGates(results)}
	labels, reasons, mismatch := admitCases(in)
	if mismatch || len(reasons) > 0 {
		return false
	}
	for _, c := range short.Cases {
		if labels[c.ID] != c.Label {
			return false
		}
	}
	return true
}

// p3RP01Side is one RP01 side's run. aborted names a side whose run did not return normally
// (a panic, runtime.Goexit, Fatal): RP01 is then INCONCLUSIVE, never judged from a missing run.
type p3RP01Side struct {
	obs     *p3Observed
	rp      *p3RemotePolicy
	fetches []p3PolicyFetch
	began   time.Time
	ended   time.Time
	aborted string
	// ledger is the side's own: its commit reads its own world only, and no prior-world rule
	// runs between the two worlds (DESIGN-SEQ §6).
	ledger *p3Ledger
}

// p3RunRP01Sides runs RP01's control and treatment side by side, the treatment delay later, each
// with its own ledger, and waits for both however they end.
func p3RunRP01Sides(t TB, runner p3Runner, build func(configuration string) (p3Case, *p3RemotePolicy), delay time.Duration, write func(string, any)) map[string]*p3RP01Side {
	sides := map[string]*p3RP01Side{p3Unprotected: {ledger: new(p3Ledger)}, p3HostManaged: {ledger: new(p3Ledger)}}
	var wg sync.WaitGroup
	for configuration, s := range sides {
		wg.Add(1)
		go func() {
			defer wg.Done()
			returned := false
			defer func() {
				if v := recover(); v != nil {
					s.aborted = fmt.Sprintf("RP01_SIDE_ABORTED: %s: %v", configuration, v)
				} else if !returned {
					s.aborted = "RP01_SIDE_ABORTED: " + configuration + ": the run did not return"
				}
			}()
			c, rp := build(configuration)
			s.rp = rp
			if configuration == p3HostManaged {
				time.Sleep(delay)
			}
			s.began = time.Now()
			side := runner
			side.ledger = s.ledger
			s.obs = side.run(t, c, configuration)
			s.fetches, s.ended = rp.log(), time.Now()
			write(c.ID+"-"+configuration, map[string]any{"evidence": p3EvidenceOf(c, configuration, s.obs, providercert.Result{CaseID: c.ID, Verdict: "RP01_RAW"}),
				"policyFetches": s.fetches, "startedAt": s.began, "endedAt": s.ended})
			returned = true
		}()
	}
	wg.Wait()
	for _, s := range sides {
		if s.rp != nil {
			s.fetches = s.rp.log()
		}
	}
	return sides
}

// p3RP01Judge is RP01's outcome from both sides: the final observation always runs (it closes the
// listeners); a side that aborted is INCONCLUSIVE.
func p3RP01Judge(sides map[string]*p3RP01Side, short *app.AdmissionEntry) (RP01Outcome, p3RP01Final) {
	control, treatment := sides[p3Unprotected], sides[p3HostManaged]
	final := p3RP01Observe(control.ledger, treatment.ledger)
	var controlURLs []string
	for _, r := range control.ledger.receivers {
		controlURLs = append(controlURLs, r.r.URL)
	}
	// A confirmed violation stands ahead of an aborted side: what was observed is kept.
	if fail := p3RP01Violation(treatment.obs, treatment.fetches, short, final, controlURLs); len(fail) > 0 {
		return RP01Outcome{Verdict: providercert.FailRouting, Source: "NOT_RUN", Reasons: fail}, final
	}
	var aborted []string
	for _, configuration := range []string{p3Unprotected, p3HostManaged} {
		if s := sides[configuration]; s.aborted != "" {
			aborted = append(aborted, s.aborted)
		} else if s.obs == nil {
			aborted = append(aborted, "RP01_SIDE_ABORTED: "+configuration+": no observation")
		}
	}
	if len(aborted) > 0 {
		return RP01Outcome{Verdict: providercert.Inconclusive, Source: "NOT_RUN", Reasons: aborted}, final
	}
	return judgeRP01(control.obs, treatment.obs, control.fetches, treatment.fetches, treatment.ended, short, final, controlURLs), final
}

// RunRP01 runs one scope's RP01 (control and treatment side by side, each its own native and
// world) and writes the scope's RP01 entry under the short key.
func RunRP01(t TB, o ScopeOptions) (app.AdmissionEntry, error) {
	productExecutable = o.Product
	app.UseProductExecutable(o.Product)
	exe := nativeAvailable(t)
	start, err := providercert.Observe(exe)
	if err != nil {
		return app.AdmissionEntry{}, err
	}
	productRaw, err := os.ReadFile(o.Product)
	if err != nil {
		return app.AdmissionEntry{}, err
	}
	managed, err := app.ManagedSourceFingerprint(platform.Environment(os.Environ()))
	if err != nil {
		return app.AdmissionEntry{}, err
	}
	key := app.AdmissionKeyFor(o.Scope, start.SHA256, digest(productRaw), o.ProbeSHA256, managed, providercert.ProbeSchemaRevision)
	release, err := app.LockAdmission(o.Home, app.RP01Scope(o.Scope), lockWait)
	if err != nil {
		return app.AdmissionEntry{}, err
	}
	defer release()
	var short *app.AdmissionEntry
	if entry, err := app.LookupAdmission(o.Home, key); err == nil {
		short = &entry
	}
	key.Scope = app.RP01Scope(o.Scope)
	write := func(name string, v any) { writeJSON(t, filepath.Join(o.EvidenceDir, name+".json"), v) }
	write("plan-rp01", map[string]any{"scope": o.Scope, "key": key, "native": start, "waitMinutes": rp01Wait.Minutes(), "runMinutes": rp01Run.Minutes(), "nativeStarts": 2, "retries": 0})
	runner := newP3Runner(exe, pinnedStarter, p3ScopeTimeout)
	runner.identity = func() (providercert.Identity, error) { return providercert.Observe(exe) }
	build := func(configuration string) (p3Case, *p3RemotePolicy) {
		c, rp := p3RP01Case(o.Scope)
		if configuration == p3Unprotected {
			// The control shows the refresh path itself: with Clauduct's hooks the #299 check
			// ends a session whose stored login is there from the start.
			c.Hooks = p3HooksMissing
		}
		return c, rp
	}
	// The treatment starts later than the control, so its observation still runs 15 minutes past
	// the control's hourly refresh (SOURCE_INACTIVE) with the planned 70-minute wait.
	sides := p3RunRP01Sides(t, runner, build, rp01TreatmentDelay, write)
	treatment := sides[p3HostManaged]
	out, final := p3RP01Judge(sides, short)
	write("final-rp01", map[string]any{"faults": final.Faults})
	if end, err := providercert.Observe(exe); err != nil || end != start {
		out.Verdict, out.Label = providercert.Inconclusive, ""
		out.Reasons = append(out.Reasons, "IDENTITY_CHANGED")
	}
	reasons := append([]string{"source " + out.Source}, out.Reasons...)
	if !out.ControlRefreshedAt.IsZero() {
		reasons = append(reasons, "control refreshed at "+out.ControlRefreshedAt.Format(time.RFC3339), "treatment observed until "+treatment.ended.Format(time.RFC3339))
	}
	entry := app.AdmissionEntry{Key: key, State: RP01Satisfied(out.Verdict, out.Label), Reasons: reasons, CreatedAt: time.Now().UTC(),
		Probe: "clauduct-probe " + o.ProbeSHA256 + " " + runtime.GOOS,
		Cases: []app.AdmissionCase{{ID: "RP01", Verdict: out.Verdict, Reasons: out.Reasons, Label: out.Label}}}
	write("summary-rp01", entry)
	if err := app.WriteAdmission(o.Home, entry); err != nil {
		return entry, fmt.Errorf("CACHE_WRITE_FAILED: %w", err)
	}
	return entry, nil
}
