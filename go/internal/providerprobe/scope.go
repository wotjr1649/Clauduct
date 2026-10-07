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
	"sync"
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
	// synthetic is a case that runs no native at all (R04S): it returns its result and the
	// evidence to write.
	synthetic func(t TB) (CaseResult, any)
}

// caseRegistry is every case this runner implements, by contract ID.
func caseRegistry() map[string]func() scopeCase {
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
					return VerdictFixtureNotApplied, "managed file marker reached no receiver in control or treatment"
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
	if len(ids) == 0 {
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
	write("plan", map[string]any{"scope": o.Scope, "cases": ids, "key": key, "native": start, "admissionContractSHA256": admissioncontract.AdmissionSHA256(),
		"retries": 0, "deadline": began.Add(certificationTotal), "workers": caseWorkers})
	runner := newP3Runner(exe, pinnedStarter, 60*time.Second)
	runner.identity = func() (providercert.Identity, error) { return providercert.Observe(exe) }
	registry := caseRegistry()
	optional := admissioncontract.Scopes()[o.Scope].Optional
	// Every deadline comes from one absolute time fixed at the start (r4 deadlines); a case that
	// cannot start or does not finish before it is INCONCLUSIVE (TIMEOUT).
	deadline := began.Add(certificationTotal)
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
		runner := runner
		runner.deadline = deadline
		if limit := time.Now().Add(perCase); limit.Before(deadline) {
			runner.deadline = limit
		}
		sc := build()
		sc.c.Optional = slices.Contains(optional, id)
		if sc.synthetic != nil {
			r, evidence := sc.synthetic(t)
			write(id+"-treatment", evidence)
			return r
		}
		control := &p3Observed{}
		if !sc.noControl {
			control = runner.run(t, sc.c, p3Unprotected)
		}
		treatment := runner.run(t, sc.c, p3HostManaged)
		if sc.judge != nil {
			r := sc.judge(control, treatment)
			for _, v := range sc.variants {
				vc := registry[v]()
				vcontrol := &p3Observed{}
				if !vc.noControl {
					vcontrol = runner.run(t, vc.c, p3Unprotected)
				}
				vtreatment := runner.run(t, vc.c, p3HostManaged)
				if vr := p3OverlayChecked(p3RoutingChecked(vc.c, vcontrol, vtreatment, vc.judge(vcontrol, vtreatment)), vtreatment); p3Severity(vr.Verdict) > p3Severity(r.Verdict) {
					r = vr
					r.Reasons = append(r.Reasons, v)
				}
			}
			r = p3OverlayChecked(p3RoutingChecked(sc.c, control, treatment, r), treatment)
			if !sc.noControl {
				write(id+"-control", p3EvidenceOf(sc.c, p3Unprotected, control, providercert.Result{CaseID: id, Verdict: "CONTROL"}))
			}
			write(id+"-treatment", p3EvidenceOf(sc.c, p3HostManaged, treatment, providercert.Result{CaseID: id, Verdict: r.Verdict, Reasons: r.Reasons}))
			return r
		}
		observation := p3Observation(sc.c, treatment, control)
		result := providercert.Judge(observation)
		late := p3LateHookOf(sc.c.HookFaults, treatment)
		if result.Verdict == providercert.Inconclusive && slices.Contains(result.Reasons, providercert.ReasonVariantNotExercised) {
			// The case's own condition did not happen (R04L's late hook): its disposition reads
			// this verdict, never PASS (admission r2 caseFixtureDisposition).
			result.Verdict = VerdictVariantNotExercised
			if late != nil && late.Outcome == p3LateInconclusive {
				// The child's fate or the late event is unproven, which is not the same as the
				// condition not having happened.
				result.Verdict = providercert.Inconclusive
			}
			if late != nil && late.Reason != "" {
				result.Reasons = append(slices.DeleteFunc(slices.Clone(result.Reasons), func(r string) bool { return r == providercert.ReasonVariantNotExercised && late.Outcome == p3LateInconclusive }), late.Reason)
			}
		}
		if result.Verdict == providercert.Pass && sc.function != nil {
			verdict, reason := sc.function(control, treatment)
			if verdict != "" {
				result.Verdict = verdict
			}
			if reason != "" {
				result.Reasons = append(result.Reasons, reason)
			}
		}
		r = p3OverlayChecked(CaseResult{Verdict: result.Verdict, Reasons: result.Reasons, Purpose: sc.c.Purpose}, treatment)
		result.Verdict, result.Reasons = r.Verdict, r.Reasons
		write(id+"-control", p3EvidenceOf(sc.c, p3Unprotected, control, providercert.Result{CaseID: id, Verdict: "CONTROL"}))
		evidence := p3EvidenceOf(sc.c, p3HostManaged, treatment, result)
		evidence.Observation = &observation
		write(id+"-treatment", evidence)
		return r
	}
	results := map[string]CaseResult{}
	gates := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan string)
	for range caseWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				r := runCase(id)
				mu.Lock()
				results[id] = r
				mu.Unlock()
				t.Logf("%s %s %v", id, r.Verdict, r.Reasons)
			}
		}()
	}
	for _, id := range ids {
		if o.Only == nil || slices.Contains(o.Only, id) {
			work <- id
		}
	}
	// A diagnostic run may name a case this runner implements and the contract does not list
	// yet (R04S before the contract names it); it is judged and written like the others, and
	// belongs to no entry.
	for _, id := range o.Only {
		if _, known := registry[id]; known && !slices.Contains(ids, id) {
			work <- id
		}
	}
	close(work)
	wg.Wait()
	p3PMControlUnproven(results)
	// BE15's disposition holds only when its equivalent gate, the product's managed agent-view
	// check, refused both of BE17's policies in this same run.
	gates["BE15"] = results["BE17"].Verdict == providercert.Pass
	end, _ := providercert.Observe(exe)
	outcome := Admit(AdmissionInput{Scope: o.Scope, Results: results, IdentityChanged: end != start, NativeSHA256: start.SHA256,
		Platform: runtime.GOOS, GatesPassed: gates, Required: o.Required})
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

// admissionGaps lists the reported gaps a run's labels carry.
func admissionGaps(out AdmissionOutcome) []string {
	var gaps []string
	for id, label := range out.Labels {
		if label == "REPORTED_GAP" || label == "NOT_APPLICABLE_CURRENT_NATIVE" {
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
	// certificationTotal and perCase are r4 deadlines.certificationTotalSeconds and
	// perCaseSeconds for one scope's short run.
	certificationTotal = 900 * time.Second
	perCase            = 180 * time.Second
	// caseWorkers runs this many cases at once, each in its own world (config dir, receivers,
	// workspace), so a scope's run fits the deadline.
	caseWorkers = 4
	// lockWait bounds the wait for another run of the same scope: longer than a whole run
	// (the deadline plus the last case's limit).
	lockWait = 25 * time.Minute
)

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
	return r
}

// staleEvents is R04S: a late hook event or confirmation, produced against the product gateway.
func staleEvents(t TB) (CaseResult, any) {
	began := time.Now()
	report := p3Stale(t, productStaleMaker)
	return CaseResult{Verdict: report.Verdict, Reasons: report.Reasons, Purpose: providercert.PurposeSafety},
		struct {
			CaseID     string
			Report     p3StaleReport
			DurationMs int64
		}{"R04S", report, time.Since(began).Milliseconds()}
}
