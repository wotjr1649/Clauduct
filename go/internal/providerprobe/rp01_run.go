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

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// RP01 timing (admission r2 foregroundConditions.cases.RP01.timing): a 70-minute observation
// plan inside a 120-minute run, one native start per configuration, no retries.
const (
	rp01Wait = 70 * time.Minute
	rp01Run  = 120 * time.Minute
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
		c.Entry, c.Turns, c.Steps = p3TUI, 2, nil
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

// judgeRP01 applies admission r2's RP01 rules to a control and a treatment run and the scope's
// short entry under the same key.
func judgeRP01(control, treatment *p3Observed, controlFetches, treatmentFetches []p3PolicyFetch, treatmentEnded time.Time, short *app.AdmissionEntry) RP01Outcome {
	out := RP01Outcome{Source: "NOT_RUN"}
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
	stored := treatment.Wrong["WRONG_STORED_GATEWAY_A"]
	if stored.Conns != 0 || p3WrongBytes(treatment) != 0 || len(treatmentFetches) != 0 {
		out.Verdict = providercert.FailRouting
		out.Reasons = []string{fmt.Sprintf("the treatment reached the stored gateway or a wrong destination (stored gateway connections %d, policy fetches %d, wrong bytes %d)", stored.Conns, len(treatmentFetches), p3WrongBytes(treatment))}
		return out
	}
	switch {
	case treatment.CleanupFailed || control.CleanupFailed:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{providercert.ReasonCleanupFailed}
	case treatmentEnded.Before(v2.Add(15 * time.Minute)):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"the treatment's observation ended before the control's refresh plus 15 minutes"}
	case !slices.ContainsFunc(treatment.Calls, func(c p3Call) bool { return c.at.After(v2) }):
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"no treatment request was served at INTENDED after the control's refresh"}
	case len(treatment.OverlayGaps) > 0:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"RC11_OVERLAY_GAP:" + strings.Join(treatment.OverlayGaps, ",")}
	case short == nil:
		out.Verdict, out.Reasons = providercert.Inconclusive, []string{"no short entry under the same key"}
	case slices.ContainsFunc(short.Cases, func(c app.AdmissionCase) bool {
		return c.Verdict == providercert.FailRouting || c.Verdict == VerdictBackgroundEntryReached
	}):
		out.Verdict, out.Reasons = providercert.FailRouting, []string{"a short-entry treatment case reached a wrong destination"}
	default:
		out.Verdict, out.Label, out.Source = providercert.Pass, "SOURCE_INACTIVE", "SOURCE_INACTIVE"
		out.Reasons = []string{"treatment: no start fetch and no periodic refresh of the stored gateway's managed settings", "treatment stored gateway connections 0; short-entry cases without a wrong destination: " + fmt.Sprint(len(short.Cases))}
	}
	return out
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
	runner := newP3Runner(exe, pinnedStarter, 60*time.Second)
	runner.identity = func() (providercert.Identity, error) { return providercert.Observe(exe) }
	type side struct {
		obs     *p3Observed
		fetches []p3PolicyFetch
		ended   time.Time
		err     error
	}
	sides := map[string]*side{p3Unprotected: {}, p3HostManaged: {}}
	var wg sync.WaitGroup
	for configuration, s := range sides {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					s.err = fmt.Errorf("%s: %v", configuration, v)
				}
			}()
			c, rp := p3RP01Case(o.Scope)
			if configuration == p3Unprotected {
				// The control shows the refresh path itself: with Clauduct's hooks the #299 check
				// ends a session whose stored login is there from the start.
				c.Hooks = p3HooksMissing
			}
			began := time.Now()
			s.obs = runner.run(t, c, configuration)
			s.fetches, s.ended = rp.log(), time.Now()
			write(c.ID+"-"+configuration, map[string]any{"evidence": p3EvidenceOf(c, configuration, s.obs, providercert.Result{CaseID: c.ID, Verdict: "RP01_RAW"}),
				"policyFetches": s.fetches, "startedAt": began, "endedAt": s.ended})
		}()
	}
	wg.Wait()
	for _, s := range sides {
		if s.err != nil {
			return app.AdmissionEntry{}, s.err
		}
	}
	control, treatment := sides[p3Unprotected], sides[p3HostManaged]
	out := judgeRP01(control.obs, treatment.obs, control.fetches, treatment.fetches, treatment.ended, short)
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
