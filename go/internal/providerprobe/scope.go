package providerprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"strings"
	"sync/atomic"
)

// ScopeOptions is one probe run: one scope's short case set (admission contract r2).
type ScopeOptions struct {
	// Product is the clauduct.exe being certified; its hooks run in the sessions.
	Product string
	// ProbeSHA256 is this probe executable's SHA-256 (cache identity).
	ProbeSHA256 string
	Scope       string
	// EvidenceDir receives the plan, each case's evidence and the summary.
	EvidenceDir string
	// Home is where the entry is written (ClauductHome).
	Home string
	// Required are the states of the scope's requiresScope entries, read by the caller.
	Required map[string]string
	// Only, when set, runs just these of the scope's cases as a diagnostic: no entry is written.
	Only []string
}

// p3Function judges a case's function after its routing verdict passed: it returns a verdict
// that replaces PASS (FAIL_FUNCTION, FIXTURE_NOT_APPLIED, ...) or "" to keep it, and a reason
// that is kept either way as the case's evidence.
type p3Function func(control, treatment *p3Observed) (verdict, reason string)

// scopeCase is a registry entry: the case to run and what the contract adds to its judgment.
type scopeCase struct {
	c        p3Case
	function p3Function
	// judge, when set, replaces Judge for a case the contract judges by its own rule.
	judge func(control, treatment *p3Observed) CaseResult
	// variants are further cases that must all pass with this one (BE17's two policies).
	variants []string
	// noControl is a case the contract runs without a paired control (BE09, BE10, BE17).
	noControl bool
	// cliBackground is a CLI background-entry transition check (judgeTransition): the only kind
	// whose PATH_ABSENT admits. A CLI launch-refusal check (BE09, BE17) is not one. The run that
	// writes an entry and the RP01 short check both read it here.
	cliBackground bool
	// synthetic is a case that runs no native at all (R04S): it returns its result and the
	// evidence to write.
	synthetic func(t TB) (CaseResult, any)
}

// caseRegistry is every case this runner implements, by contract ID.
func caseRegistry(scope string) map[string]func() scopeCase {
	reg := map[string]func() scopeCase{}
	for _, c := range p3PrintFamily() {
		c := c
		reg[c.ID] = func() scopeCase { return scopeCase{c: c} }
	}
	for id, f := range permissionCases() {
		reg[id] = f
	}
	for id, f := range backgroundEntryCases() {
		reg[id] = f
	}
	for id, f := range tuiBoundaryCases() {
		reg[id] = f
	}
	for id, f := range tuiCases() {
		reg[id] = f
	}
	for _, c := range p3PrintFamily() {
		if c.ID == "R04L" {
			// The control for R04L's child: the same case with a child that breaks away from
			// the hook's job. A diagnostic; no contract scope lists it.
			c := c
			c.ID, c.HookFaults = "R04L-ESC", map[string]string{"UserPromptSubmit": p3HookChildEscape}
			reg[c.ID] = func() scopeCase { return scopeCase{c: c} }
		}
	}
	reg["R04S"] = func() scopeCase { return scopeCase{c: p3Case{ID: "R04S"}, synthetic: staleEvents} }
	be17 := reg["BE17"]
	reg["BE17"] = func() scopeCase { sc := be17(); sc.variants = []string{"BE17-setting"}; return sc }
	for id, f := range tuiResumeCases() {
		reg[id] = f
	}
	if scope == app.ScopeResumeTUI {
		resumeTUIScope(reg)
	}
	return reg
}

// permissionCases are PM00–PM03 (admission r2 foregroundConditions): a scripted Bash call that
// would write a marker in the probe workspace, under each permission rule.
func permissionCases() map[string]func() scopeCase {
	storedA := func(w *p3World) error { return w.storeGateway(w.sgA) }
	userRule := func(kind string) func(w *p3World) error {
		return func(w *p3World) error {
			if err := storedA(w); err != nil {
				return err
			}
			return w.mergeUserSettings("permissions", map[string]any{kind: []string{"Bash"}})
		}
	}
	// want judges the treatment's run: whether the marker was written, whether Bash was
	// offered, and what native returned for the call.
	type want func(written, offered bool, results []string) string
	denied := func(written, offered bool, results []string) string {
		switch {
		case written:
			return "the marker was written"
		case offered && !slices.ContainsFunc(results, p3PermissionDenied):
			return fmt.Sprintf("Bash was offered and its result is not a permission denial: %q", results)
		}
		return ""
	}
	asked := func(written, _ bool, results []string) string {
		if written || !slices.ContainsFunc(results, p3ApprovalRequired) {
			return fmt.Sprintf("marker written=%t; the result is not an approval-required denial: %q", written, results)
		}
		return ""
	}
	allowed := func(written, _ bool, _ []string) string {
		if !written {
			return "the marker was not written"
		}
		return ""
	}
	make := func(id string, setup func(*p3World) error, judge want, managedMark bool) func() scopeCase {
		return func() scopeCase {
			bash := &p3BashOnce{}
			c := p3Case{ID: id, Entry: p3Print, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, Args: []string{"--allowedTools", "Bash"},
				Established: gotCanary("WRONG_STORED_GATEWAY_A", 0), Script: bash.script}
			c.Setup = func(w *p3World) error {
				bash.reset(filepath.Join(w.cwd, "p3-pm-marker.txt"))
				if err := setup(w); err != nil {
					return err
				}
				if managedMark {
					mark := w.addCanary("pm-managed-file")
					extra, _ := json.Marshal(map[string]string{"p3_pm": string(mark.Value)})
					policy, _ := json.Marshal(map[string]any{"permissions": map[string]any{"deny": []string{"Bash"}}, "env": map[string]string{"CLAUDE_CODE_EXTRA_BODY": string(extra)}})
					return os.WriteFile(filepath.Join(w.managedDir, "managed-settings.json"), policy, 0o600)
				}
				return nil
			}
			function := func(control, treatment *p3Observed) (string, string) {
				if managedMark && !p3SawCanary(control, "pm-managed-file") && !p3SawCanary(treatment, "pm-managed-file") {
					return p3ManagedFixtureVerdict(bash)
				}
				if managedMark && p3SawCanary(control, "pm-managed-file") && !p3SawCanary(treatment, "pm-managed-file") {
					return providercert.FailFunction, "the treatment did not read the managed file the control read"
				}
				bash.mu.Lock()
				defer bash.mu.Unlock()
				_, statErr := os.Stat(bash.marker)
				observed := fmt.Sprintf("marker written=%t, Bash offered=%t, results %q", statErr == nil, bash.offered, bash.results)
				if reason := judge(statErr == nil, bash.offered, bash.results); reason != "" {
					return providercert.FailFunction, reason + "; " + observed
				}
				return "", observed
			}
			return scopeCase{c: c, function: function}
		}
	}
	return map[string]func() scopeCase{
		"PM00": make("PM00", storedA, allowed, false),
		"PM01": make("PM01", userRule("deny"), denied, false),
		"PM02": make("PM02", userRule("ask"), asked, false),
		"PM03": make("PM03", storedA, denied, true),
	}
}

// p3PMControlUnproven applies admission r2's PM00 rule to a run's results: when PM00's marker
// was not written, PM01–PM03 prove nothing and are INCONCLUSIVE (CONTROL_FUNCTION_UNPROVEN).
func p3PMControlUnproven(results map[string]CaseResult) {
	pm00, ran := results["PM00"]
	if !ran || pm00.Verdict == providercert.Pass {
		return
	}
	for _, id := range []string{"PM01", "PM02", "PM03"} {
		if r, ok := results[id]; ok && r.Verdict == providercert.Pass {
			r.Verdict, r.Reasons = providercert.Inconclusive, append(r.Reasons, "CONTROL_FUNCTION_UNPROVEN")
			results[id] = r
		}
	}
}

// p3SawCanary reports whether a canary with this id reached INTENDED's ingress or any receiver.
func p3SawCanary(obs *p3Observed, id string) bool {
	for _, call := range obs.Ingress {
		if slices.Contains(call.canaries, id) {
			return true
		}
	}
	for _, seen := range obs.WrongSeen {
		for _, s := range seen {
			if slices.Contains(s.Canaries, id) {
				return true
			}
		}
	}
	return false
}

// RunScope runs one scope's short case set against the native this machine resolves, judges it
// with admission r2's order, and writes the entry. Every case the contract lists and this runner
// does not implement is CASE_NOT_IMPLEMENTED, which keeps the scope INCONCLUSIVE.
func RunScope(t TB, o ScopeOptions) (app.AdmissionEntry, error) {
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
	write := func(name string, v any) { writeJSON(t, filepath.Join(o.EvidenceDir, name+".json"), v) }
	ids := admissioncontract.ShortCases(o.Scope)
	// RESUME_TUI may be diagnosed (-only) before the contract lists it.
	if len(ids) == 0 && (o.Only == nil || o.Scope != app.ScopeResumeTUI) {
		return app.AdmissionEntry{}, fmt.Errorf("unknown scope %q", o.Scope)
	}
	// One run per scope at a time (r4 cache.write). A run that waited finds the entry the other
	// wrote and keeps it rather than measuring again.
	waitFrom := time.Now()
	if o.Only == nil {
		release, err := app.LockAdmission(o.Home, o.Scope, lockWait)
		if err != nil {
			return app.AdmissionEntry{}, err
		}
		defer release()
		if entry, err := app.LookupAdmission(o.Home, key); err == nil && entry.CreatedAt.After(waitFrom) {
			return entry, nil
		}
	}
	began := time.Now()
	certificationTotal := p3CertificationTotal(o.Scope)
	write("plan", map[string]any{"scope": o.Scope, "cases": ids, "key": key, "native": start, "admissionContractSHA256": admissioncontract.AdmissionSHA256(),
		"retries": 0, "deadline": began.Add(certificationTotal), "workers": caseWorkers})
	runner := newP3Runner(exe, pinnedStarter, p3ScopeTimeout)
	runner.identity = func() (providercert.Identity, error) { return providercert.Observe(exe) }
	registry := caseRegistry(o.Scope)
	optional := admissioncontract.Scopes()[o.Scope].Optional
	// Every deadline comes from one absolute time fixed at the start (r4 deadlines); a case that
	// cannot start or does not finish before it is INCONCLUSIVE (TIMEOUT).
	deadline := began.Add(certificationTotal)
	pin := p3ScopePin(start, &runner)
	runCase := func(id string) (r CaseResult) {
		defer func() {
			if v := recover(); v != nil {
				r = CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{fmt.Sprintf("PROBE_CASE_FAILED: %v", v)}}
			}
			if time.Now().After(deadline) && r.Verdict != VerdictBackgroundEntryReached && r.Verdict != providercert.FailRouting {
				r = CaseResult{Verdict: providercert.Inconclusive, Reasons: append([]string{providercert.ReasonTimeout}, r.Reasons...)}
			}
		}()
		if time.Now().After(deadline) {
			return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{providercert.ReasonTimeout}}
		}
		build, ok := registry[id]
		if !ok {
			return CaseResult{Verdict: VerdictCaseNotImplemented}
		}
		// Every native run of the case ends by the case's own limit or the run's, the earlier.
		sc := build()
		runner := runner
		runner.deadline = deadline
		if limit := time.Now().Add(p3CaseBudget(sc, registry)); limit.Before(deadline) {
			runner.deadline = limit
		}
		sc.c.Optional = slices.Contains(optional, id)
		if sc.synthetic != nil {
			r, evidence := sc.synthetic(t)
			write(id+"-treatment", evidence)
			return r
		}
		return p3ScopeCase(pin, id, sc, registry, func(c p3Case, configuration string) *p3Observed { return runner.run(t, c, configuration) }, write)
	}
	var order []string
	for _, id := range ids {
		if o.Only == nil || slices.Contains(o.Only, id) {
			order = append(order, id)
		}
	}
	// A diagnostic run may name a case this runner implements and the contract does not list
	// yet (R04S before the contract names it); it is judged and written like the others, and
	// belongs to no entry.
	for _, id := range o.Only {
		if _, known := registry[id]; known && !slices.Contains(ids, id) {
			order = append(order, id)
		}
	}
	results, aborted := p3Dispatch(t, order, runner.ledger, runCase)
	p3PMControlUnproven(results)
	gates := map[string]bool{}
	for id, passed := range scopeGates(results) {
		gates[id] = passed
	}
	faults := runner.ledger.conserve()
	if aborted {
		faults = append([]string{"PROBE_CASE_ABORTED"}, faults...)
	}
	write("ledger", map[string]any{"faults": faults})
	end, _ := providercert.Observe(exe)
	outcome := p3ScopeAdmit(o, results, gates, pin, end, faults)
	entry := app.AdmissionEntry{Key: key, State: outcome.State, Reasons: outcome.Reasons, CreatedAt: time.Now().UTC(),
		Probe: "clauduct-probe " + o.ProbeSHA256}
	for _, id := range ids {
		r := results[id]
		entry.Cases = append(entry.Cases, app.AdmissionCase{ID: id, Verdict: r.Verdict, Reasons: r.Reasons, Label: outcome.Labels[id]})
		if outcome.Labels[id] == "NOT_EXERCISABLE" {
			entry.NotExercisable = append(entry.NotExercisable, id)
		}
	}
	entry.Gaps = admissionGaps(outcome)
	summary, _ := json.Marshal(entry)
	entry.EvidenceDigest = digest(summary)
	write("summary", entry)
	if o.Only != nil {
		return entry, nil
	}
	if err := app.WriteAdmission(o.Home, entry); err != nil {
		return entry, fmt.Errorf("CACHE_WRITE_FAILED: %w", err)
	}
	return entry, nil
}

// p3Dispatch runs the cases one at a time (caseWorkers = 1), each in a goroutine of its own whose
// done is closed on every exit, runtime.Goexit included, so the dispatcher never waits forever. A
// case that returned no result, or whose run aborted, stops the dispatch: no later case starts,
// and the scope is PROBE_CASE_ABORTED (DESIGN-SEQ §4.3).
func p3Dispatch(t TB, order []string, ledger *p3Ledger, runCase func(string) CaseResult) (map[string]CaseResult, bool) {
	results := map[string]CaseResult{}
	for _, id := range order {
		done := make(chan struct{})
		var r CaseResult
		returned := false
		go func() {
			defer close(done)
			r = runCase(id)
			returned = true
		}()
		<-done
		if !returned {
			t.Logf("%s aborted", id)
			return results, true
		}
		results[id] = r
		t.Logf("%s %s %v", id, r.Verdict, r.Reasons)
		ledger.mu.Lock()
		aborted := ledger.aborted
		ledger.mu.Unlock()
		if aborted {
			return results, true
		}
	}
	return results, false
}

// p3VariantVerdict turns a judged case whose only unmet condition was its own variant (the judge
// returns ReasonVariantNotExercised alone, never next to another unmet rule) into the
// VARIANT_NOT_EXERCISED verdict a disposition reads, never PASS (admission r3 dispositions). Any
// other result, with its real verdict and reasons, passes through and no disposition can apply.
func p3VariantVerdict(result providercert.Result, late *p3LateHook) providercert.Result {
	if result.Verdict != providercert.Inconclusive || !slices.Contains(result.Reasons, providercert.ReasonVariantNotExercised) {
		return result
	}
	result.Verdict = VerdictVariantNotExercised
	if late != nil && late.Outcome == p3LateInconclusive {
		// The child's fate or the late event is unproven, which is not the same as the
		// condition not having happened.
		result.Verdict = providercert.Inconclusive
	}
	if late != nil && late.Reason != "" {
		result.Reasons = append(slices.DeleteFunc(slices.Clone(result.Reasons), func(r string) bool {
			return r == providercert.ReasonVariantNotExercised && late.Outcome == p3LateInconclusive
		}), late.Reason)
	}
	return result
}

// admissionGaps lists every case a run judged under a disposition label (each label of the
// contract's dispositions, plus the reported-gap labels), as "ID:LABEL", so none vanishes from
// the entry's gaps and doctor's open gaps.
func admissionGaps(out AdmissionOutcome) []string {
	labels := []string{"REPORTED_GAP", "NOT_APPLICABLE_CURRENT_NATIVE"}
	for _, d := range admissioncontract.Dispositions() {
		labels = append(labels, d.Label)
	}
	var gaps []string
	for id, label := range out.Labels {
		if slices.Contains(labels, label) {
			gaps = append(gaps, id+":"+label)
		}
	}
	slices.Sort(gaps)
	return gaps
}

// RequiredStates reads, under the same key as this run, the state of each scope the given
// scope requires (admission r2 states.requiresScope).
func RequiredStates(home, product, probeSHA, scope string) (map[string]string, error) {
	states := map[string]string{}
	requires := admissioncontract.Scopes()[scope].RequiresScope
	if len(requires) == 0 {
		return states, nil
	}
	native, err := app.ResolvedNativeSHA256()
	if err != nil {
		return nil, err
	}
	productRaw, err := os.ReadFile(product)
	if err != nil {
		return nil, err
	}
	managed, err := app.ManagedSourceFingerprint(platform.Environment(os.Environ()))
	if err != nil {
		return nil, err
	}
	key := func(scope string) app.AdmissionKey {
		return app.AdmissionKeyFor(scope, native, digest(productRaw), probeSHA, managed, providercert.ProbeSchemaRevision)
	}
	for _, s := range requires {
		states[s] = app.ScopeState(home, key, s)
	}
	return states, nil
}

const (
	// caseWorkers is r5 deadlines.caseWorkers: one case at a time, so every receiver change in a
	// run's interval is that run's (scopeLedger).
	caseWorkers = 1
	// p3ScopeTimeout is the runner's per-turn native timeout in a certification run.
	p3ScopeTimeout = 60 * time.Second
	// p3StartTeardown is what one native start may take past its own limit: the product's stop
	// grace and the Job reap (childprocess reapGrace), with a margin.
	p3StartTeardown = 15 * time.Second
	// p3RunTeardown is the run's cleanup after its natives before any hook kill: the boundary
	// watch's supervisor stop (20 s) and its census (15 s), the no-natives check (15 s), the hook
	// reap (10 s) and the interval's settle (2.5 s). Each hook target adds p3HookKillWait.
	p3RunTeardown  = 62500 * time.Millisecond
	p3HookKillWait = 5 * time.Second
	// p3BackgroundCycle bounds one background turn: the resident's 3 minutes and its stop and
	// worker exit waits.
	p3BackgroundCycle = 4 * time.Minute
	// p3SyntheticBudget bounds R04S, which starts no native.
	p3SyntheticBudget = 5 * time.Minute
	// lockWait bounds the wait for another run of the same scope or its RP01: longer than the
	// longest holder (r5 deadlines.lockWaitSeconds, contract test).
	lockWait = 200 * time.Minute
)

// p3RunFor is one native start's limit for a case (the turns and waits its one process carries).
func p3RunFor(c p3Case, timeout, settle time.Duration) time.Duration {
	runFor := timeout
	if c.Entry == p3Print && c.Settle > 0 {
		// A single turn that runs this long by itself (a scripted tool loop).
		runFor = timeout + c.Settle
	}
	total := max(len(c.Steps)+1, c.Turns)
	if c.Entry == p3Multi || c.Entry == p3TUI && total >= 2 {
		// One native process carries every turn and the waits between them.
		waits := len(c.Steps)
		if c.Entry == p3TUI {
			waits = total - 1 // the driver waits between prompts
		}
		runFor = time.Duration(total)*timeout + time.Duration(waits)*max(settle, c.Settle)
	}
	if c.TUIStimulus != nil {
		// The reply, the stimuli, 8 seconds, then the confirmation prompt and its reply.
		runFor = 3 * timeout
	}
	return runFor
}

// p3RunNative is the run's native deadline from its start: every native start inside the run
// (seeds, resume and continue turns, background cycles) ends by it, and each start's own
// teardown and the boundary windows fit in it. The run's whole deadline adds p3RunTeardown;
// a run that misses it is unreaped (DESIGN-SEQ §4.2, Terra 4).
func p3RunNative(c p3Case, timeout, settle time.Duration) time.Duration {
	runFor := p3RunFor(c, timeout, settle)
	start := runFor + p3StartTeardown
	switch c.Entry {
	case p3CLI:
		n := time.Duration(1)
		if c.CLIPrior {
			n = 2
		}
		return n*start + 8*time.Second
	case p3Resume, p3Continue, p3ContinueRefusal, p3ContinueLegacy:
		return 2 * start
	case p3ContinueRecover:
		return 3 * start
	case p3TUI:
		if c.TUIResume != "" {
			return 2 * start
		}
		return start
	case p3Background:
		return time.Duration(1+len(c.Steps)) * (p3BackgroundCycle + p3StartTeardown)
	case p3Print:
		if c.WatchBoundary {
			return start + 8*time.Second
		}
	}
	return start
}

// p3HookTargets bounds the processes the hook reap may have to end: a child-mode hook leaves a
// wrapper and a child per turn; any other run allows two.
func p3HookTargets(c p3Case) int {
	for _, fault := range c.HookFaults {
		if p3ChildFault(fault) {
			return max(2, 2*max(len(c.Steps)+1, c.Turns))
		}
	}
	return 2
}

// p3RunTeardownFor is the run's cleanup deadline after its natives; the reap and the settle run
// inside it, and a run still cleaning up after it is unreaped.
func p3RunTeardownFor(c p3Case) time.Duration {
	return p3RunTeardown + time.Duration(p3HookTargets(c))*p3HookKillWait
}

// p3RunBudget is one run's whole enforced deadline.
func p3RunBudget(c p3Case) time.Duration {
	return p3RunNative(c, p3ScopeTimeout, 0) + p3RunTeardownFor(c)
}

// p3CaseBudget is the sum of the case's runs: its control unless noControl, its treatment, and
// each variant's.
func p3CaseBudget(sc scopeCase, registry map[string]func() scopeCase) time.Duration {
	if sc.synthetic != nil {
		return p3SyntheticBudget
	}
	runs := func(sc scopeCase) time.Duration {
		n := time.Duration(2)
		if sc.noControl {
			n = 1
		}
		return n * p3RunBudget(sc.c)
	}
	total := runs(sc)
	for _, v := range sc.variants {
		total += runs(registry[v]())
	}
	return total
}

// p3CertificationTotal is a scope's short run deadline: the sum of its cases' budgets (r5
// deadlines.certificationTotalSeconds, contract test).
func p3CertificationTotal(scope string) time.Duration {
	registry := caseRegistry(scope)
	var total time.Duration
	for _, id := range admissioncontract.ShortCases(scope) {
		if build, ok := registry[id]; ok {
			total += p3CaseBudget(build(), registry)
		}
	}
	return total
}

// p3ScopeCase runs and judges one native case of a scope: its control, treatment and variants.
// Each run's checkpoints are noted against the scope's start pin; the case verdict keeps what the
// case observed and the scope's identity state carries any change to Admit.
func p3ScopeCase(pin *p3ScopeIdentity, id string, sc scopeCase, registry map[string]func() scopeCase,
	run func(c p3Case, configuration string) *p3Observed, write func(name string, v any)) CaseResult {
	// Each run is noted as soon as it returns, before the next starts: a later run's failure
	// cannot drop it. The runner's checkpoint covers a run that itself fails.
	runOnce := run
	run = func(c p3Case, configuration string) *p3Observed {
		o := runOnce(c, configuration)
		pin.note(o)
		return o
	}
	control := &p3Observed{}
	if !sc.noControl {
		control = run(sc.c, p3Unprotected)
	}
	treatment := run(sc.c, p3HostManaged)
	if sc.judge != nil {
		r := sc.judge(control, treatment)
		r.CLIBackgroundEntry = sc.cliBackground
		for _, v := range sc.variants {
			vc := registry[v]()
			vcontrol := &p3Observed{}
			if !vc.noControl {
				vcontrol = run(vc.c, p3Unprotected)
			}
			vtreatment := run(vc.c, p3HostManaged)
			vj := vc.judge(vcontrol, vtreatment)
			vj.CLIBackgroundEntry = vc.cliBackground
			if vr := p3OverlayChecked(p3RoutingChecked(vc.c, vcontrol, vtreatment, p3ScopePinnedIdentityChecked(pin.start, vcontrol, vtreatment, vj)), vtreatment); p3Severity(vr.Verdict) > p3Severity(r.Verdict) {
				r = vr
				r.Reasons = append(r.Reasons, v)
			}
		}
		r = p3OverlayChecked(p3RoutingChecked(sc.c, control, treatment, p3ScopePinnedIdentityChecked(pin.start, control, treatment, r)), treatment)
		if !sc.noControl {
			write(id+"-control", p3EvidenceOf(sc.c, p3Unprotected, control, providercert.Result{CaseID: id, Verdict: "CONTROL"}))
		}
		write(id+"-treatment", p3EvidenceOf(sc.c, p3HostManaged, treatment, providercert.Result{CaseID: id, Verdict: r.Verdict, Reasons: r.Reasons}))
		return r
	}
	observation := p3Observation(sc.c, treatment, control)
	observation.IdentityChanged = observation.IdentityChanged || !p3MatchesScopePin(pin.start, control, treatment)
	result := providercert.Judge(observation)
	late := p3LateHookOf(sc.c.HookFaults, treatment)
	result = p3VariantVerdict(result, late)
	if result.Verdict == providercert.Pass && sc.function != nil {
		verdict, reason := sc.function(control, treatment)
		if verdict != "" {
			result.Verdict = verdict
		}
		if reason != "" {
			result.Reasons = append(result.Reasons, reason)
		}
	}
	r := p3OverlayChecked(CaseResult{Verdict: result.Verdict, Reasons: result.Reasons, Purpose: sc.c.Purpose}, treatment)
	result.Verdict, result.Reasons = r.Verdict, r.Reasons
	write(id+"-control", p3EvidenceOf(sc.c, p3Unprotected, control, providercert.Result{CaseID: id, Verdict: "CONTROL"}))
	evidence := p3EvidenceOf(sc.c, p3HostManaged, treatment, result)
	evidence.Observation = &observation
	write(id+"-treatment", evidence)
	return r
}

// p3ScopeIdentity is a scope's native identity state: any checkpoint of any control, treatment or
// variant that saw a native other than the start pin makes the whole scope
// NATIVE_CHANGED_DURING_CERTIFICATION (admission r3 caseVerdict 1), ahead of every case verdict.
type p3ScopeIdentity struct {
	start   providercert.Identity
	changed atomic.Bool
}

func (s *p3ScopeIdentity) note(runs ...*p3Observed) {
	if p3ScopePinMismatched(s.start, runs...) {
		s.changed.Store(true)
	}
}

// p3ScopePin starts a scope's identity state at its start pin and hooks the runner's checkpoints to it.
func p3ScopePin(start providercert.Identity, runner *p3Runner) *p3ScopeIdentity {
	pin := &p3ScopeIdentity{start: start}
	runner.checkpoint = pin.see
	return pin
}

// p3ScopeAdmit judges a scope's short run: the identity state at the scope's end is Admit's first
// rule, and the ledger's faults (an abort, an unreaped run, a stream not conserved) keep it
// INCONCLUSIVE after any confirmed routing or background failure.
func p3ScopeAdmit(o ScopeOptions, results map[string]CaseResult, gates map[string]bool, pin *p3ScopeIdentity, end providercert.Identity, faults []string) AdmissionOutcome {
	return Admit(AdmissionInput{Scope: o.Scope, Results: results, IdentityChanged: pin.changedAt(end), NativeSHA256: pin.start.SHA256,
		Platform: runtime.GOOS, GatesPassed: gates, Required: o.Required, ScopeFaults: faults})
}

// see takes one observed checkpoint (p3Runner.checkpoint).
func (s *p3ScopeIdentity) see(id providercert.Identity) {
	if id != s.start {
		s.changed.Store(true)
	}
}

func (s *p3ScopeIdentity) changedAt(end providercert.Identity) bool {
	return end != s.start || s.changed.Load()
}

// p3OverlayChecked keeps a passing verdict only when the product passed every strategy value
// itself (RC11): a value the harness had to fill in means the measured configuration is not
// the one a launch runs.
func p3OverlayChecked(r CaseResult, treatment *p3Observed) CaseResult {
	if len(treatment.OverlayGaps) > 0 && p3Severity(r.Verdict) <= p3Severity(providercert.Inconclusive) {
		r.Verdict, r.Reasons = providercert.Inconclusive, append(r.Reasons, "RC11_OVERLAY_GAP:"+strings.Join(treatment.OverlayGaps, ","))
	}
	return r
}

// p3Severity orders verdicts for combining a case's variants: a failure outranks an unknown,
// which outranks a pass.
func p3Severity(verdict string) int {
	switch verdict {
	case VerdictBackgroundEntryReached, providercert.FailRouting:
		return 4
	case providercert.FailFunction, "FAIL_SAFETY":
		return 3
	case providercert.Pass, VerdictPathAbsent:
		return 0
	}
	return 2
}

// p3RoutingChecked applies r4's routing rules (P1) to a case the contract judges by its own
// rule: any contact with a stored gateway, the settings target or another receiver, inference
// or a canary through CATCH_ALL, or a foreign reply marker is FAIL_ROUTING, whatever the
// case's own verdict (admission r2 backgroundEntryChecks.boundary: wrong-destination bytes and
// connections).
func p3RoutingChecked(c p3Case, control, treatment *p3Observed, r CaseResult) CaseResult {
	routing := providercert.Judge(p3Observation(c, treatment, control))
	if routing.Verdict == providercert.FailRouting && p3Severity(r.Verdict) < p3Severity(providercert.FailRouting) {
		return CaseResult{Verdict: providercert.FailRouting, Reasons: routing.Reasons, CLIBackgroundEntry: r.CLIBackgroundEntry}
	}
	// An incomplete receiver snapshot cannot support a PASS, PATH_ABSENT or disposition verdict;
	// a confirmed routing or background failure, or a function failure, stands.
	if (control.ReceiverFault || treatment.ReceiverFault) && p3Severity(r.Verdict) < p3Severity(providercert.FailFunction) {
		r.Verdict, r.Reasons = providercert.Inconclusive, append([]string{providercert.ReasonReceiverFault}, r.Reasons...)
	}
	return r
}

// A catalogued replacement B must not certify the start pin A, even if A is restored before scope end.
func p3MatchesScopePin(pin providercert.Identity, runs ...*p3Observed) bool {
	if pin.Path == "" || pin.SHA256 == "" {
		return false
	}
	for _, o := range runs {
		if o == nil {
			continue
		}
		if o.IdentityError || o.NativeStarts > 0 && len(o.Identity) < 2+o.NativeStarts {
			return false
		}
		for _, id := range o.Identity {
			if id != pin {
				return false
			}
		}
	}
	return true
}

// p3ScopePinnedIdentityChecked keeps an observed FAIL_ROUTING or BACKGROUND_ENTRY_REACHED as evidence;
// the scope's identity state, not the case verdict, gives the identity change its priority in Admit.
func p3ScopePinnedIdentityChecked(pin providercert.Identity, control, treatment *p3Observed, r CaseResult) CaseResult {
	if !p3MatchesScopePin(pin, control, treatment) {
		if r.Verdict != providercert.FailRouting && r.Verdict != VerdictBackgroundEntryReached {
			r.Verdict = providercert.Inconclusive
		}
		r.Reasons = append([]string{providercert.ReasonNativeChanged}, r.Reasons...)
	}
	return r
}

// p3ScopePinMismatched reports a checkpoint that observed a native other than the start pin. A
// missing checkpoint is not a confirmed change; it keeps its case INCONCLUSIVE only.
func p3ScopePinMismatched(pin providercert.Identity, runs ...*p3Observed) bool {
	for _, o := range runs {
		if o != nil && slices.ContainsFunc(o.Identity, func(id providercert.Identity) bool { return id != pin }) {
			return true
		}
	}
	return false
}

// staleEvents is R04S: a late hook event or confirmation, produced against the product gateway.
func staleEvents(t TB) (CaseResult, any) {
	began := time.Now()
	report := p3Stale(t, productStaleMaker)
	return CaseResult{Verdict: report.Verdict, Reasons: report.Reasons, Purpose: ""},
		struct {
			CaseID     string
			Report     p3StaleReport
			DurationMs int64
		}{"R04S", report, time.Since(began).Milliseconds()}
}

// scopeGates are the equivalent gates a run showed working, by the case whose disposition needs
// them (admission r3 dispositions): BE15's holds only when the product's managed agent-view
// check refused both of BE17's policies in this same run, and R04L's (and R04L-TUI's) only when
// R04S's prevention check on stale late events passed in this same run.
func scopeGates(results map[string]CaseResult) map[string]bool {
	r04s := results["R04S"].Verdict == providercert.Pass
	return map[string]bool{
		"BE15":     results["BE17"].Verdict == providercert.Pass,
		"R04L":     r04s,
		"R04L-TUI": r04s,
	}
}
