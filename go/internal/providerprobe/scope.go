package providerprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/admissioncontract"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
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
}

// p3Function judges a case's function after its routing verdict passed: it returns a verdict
// that replaces PASS (FAIL_FUNCTION, FIXTURE_NOT_APPLIED, ...) or "" to keep it.
type p3Function func(control, treatment *p3Observed) (verdict, reason string)

// scopeCase is a registry entry: the case to run and what the contract adds to its judgment.
type scopeCase struct {
	c        p3Case
	function p3Function
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
			return w.userSettings(map[string]any{"permissions": map[string]any{kind: []string{"Bash"}}})
		}
	}
	make := func(id string, setup func(*p3World) error, wantMarker bool, managedMark bool) func() scopeCase {
		return func() scopeCase {
			var mu sync.Mutex
			var results []string
			marker := ""
			c := p3Case{ID: id, Entry: p3Print, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, Args: []string{"--allowedTools", "Bash"},
				Established: gotCanary("WRONG_STORED_GATEWAY_A", 0)}
			c.Setup = func(w *p3World) error {
				marker = filepath.Join(w.cwd, "p3-pm-marker.txt")
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
			c.Script = p3BashOnce(&marker, &results, &mu)
			function := func(control, treatment *p3Observed) (string, string) {
				_, statErr := os.Stat(marker)
				written := statErr == nil
				if managedMark && !p3SawCanary(control, "pm-managed-file") && !p3SawCanary(treatment, "pm-managed-file") {
					return VerdictFixtureNotApplied, "managed file marker reached no receiver in control or treatment"
				}
				if managedMark && p3SawCanary(control, "pm-managed-file") && !p3SawCanary(treatment, "pm-managed-file") {
					return "FAIL_FUNCTION", "the treatment did not read the managed file the control read"
				}
				if written != wantMarker {
					return "FAIL_FUNCTION", fmt.Sprintf("marker written=%t, want %t", written, wantMarker)
				}
				return "", ""
			}
			return scopeCase{c: c, function: function}
		}
	}
	return map[string]func() scopeCase{
		"PM00": make("PM00", storedA, true, false),
		"PM01": make("PM01", userRule("deny"), false, false),
		"PM02": make("PM02", userRule("ask"), false, false),
		"PM03": make("PM03", storedA, false, true),
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
	managed, err := app.ManagedSourceFingerprint(map[string]string{})
	if err != nil {
		return app.AdmissionEntry{}, err
	}
	key := app.AdmissionKeyFor(o.Scope, start.SHA256, digest(productRaw), o.ProbeSHA256, managed, providercert.ProbeSchemaRevision)
	write := func(name string, v any) { writeJSON(t, filepath.Join(o.EvidenceDir, name+".json"), v) }
	ids := admissioncontract.ShortCases(o.Scope)
	if len(ids) == 0 {
		return app.AdmissionEntry{}, fmt.Errorf("unknown scope %q", o.Scope)
	}
	write("plan", map[string]any{"scope": o.Scope, "cases": ids, "key": key, "native": start, "admissionContractSHA256": admissioncontract.AdmissionSHA256(), "retries": 0})
	runner := newP3Runner(exe, pinnedStarter, 60*time.Second)
	runner.identity = func() (providercert.Identity, error) { return providercert.Observe(exe) }
	registry := caseRegistry()
	optional := admissioncontract.Scopes()[o.Scope].Optional
	results := map[string]CaseResult{}
	gates := map[string]bool{}
	for _, id := range ids {
		build, ok := registry[id]
		if !ok {
			results[id] = CaseResult{Verdict: VerdictCaseNotImplemented}
			continue
		}
		sc := build()
		sc.c.Optional = slices.Contains(optional, id)
		control := runner.run(t, sc.c, p3Unprotected)
		treatment := runner.run(t, sc.c, p3HostManaged)
		observation := p3Observation(sc.c, treatment, control)
		result := providercert.Judge(observation)
		if result.Verdict == providercert.Pass && sc.function != nil {
			if verdict, reason := sc.function(control, treatment); verdict != "" {
				result.Verdict, result.Reasons = verdict, append(result.Reasons, reason)
			}
		}
		write(id+"-control", p3EvidenceOf(sc.c, p3Unprotected, control, providercert.Result{CaseID: id, Verdict: "CONTROL"}))
		evidence := p3EvidenceOf(sc.c, p3HostManaged, treatment, result)
		evidence.Observation = &observation
		write(id+"-treatment", evidence)
		results[id] = CaseResult{Verdict: result.Verdict, Reasons: result.Reasons, Purpose: sc.c.Purpose}
		t.Logf("%s %s %v", id, result.Verdict, result.Reasons)
	}
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
	managed, err := app.ManagedSourceFingerprint(map[string]string{})
	if err != nil {
		return nil, err
	}
	for _, s := range requires {
		entry, err := app.LookupAdmission(home, app.AdmissionKeyFor(s, native, digest(productRaw), probeSHA, managed, providercert.ProbeSchemaRevision))
		if err != nil {
			states[s] = ""
			continue
		}
		states[s] = entry.State
	}
	return states, nil
}
