package providerprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"time"
)

// p3CLI is a command-line launch (admission r2 background-entry checks): the case's Args, with
// {prompt} and {session} filled in, optionally after a prior -p turn that creates the session.
const p3CLI = "cli"

// p3PriorSession is the fixed session a prior turn creates for --resume and --continue checks.
const p3PriorSession = "5e55a1b2-0c3d-4e5f-8a9b-0c1d2e3f4a5b"

// p3FillArgs puts the prompt and the prior session into a case's argument template.
func p3FillArgs(args []string, prompt string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.NewReplacer("{prompt}", prompt, "{session}", p3PriorSession).Replace(a)
	}
	return out
}

// p3BoundaryReached reports whether a run crossed into native's background: a supervisor lock,
// a job directory, or a supervisor or pty-host process of the probe config dir.
func p3BoundaryReached(obs *p3Observed) bool {
	if obs.SupervisorLock || obs.JobDirs > 0 {
		return true
	}
	for _, n := range obs.Natives {
		if n.Command == "daemon" || n.Command == "--bg-pty-host" {
			return true
		}
	}
	return false
}

// p3RefusedBeforeStart reports whether the product refused the launch with code before any
// native session process started.
func p3RefusedBeforeStart(obs *p3Observed, code string) bool {
	if obs.NativeStarts != 0 {
		return false
	}
	for _, e := range obs.RunErrors {
		if strings.Contains(e, code) {
			return true
		}
	}
	return false
}

// judgeTransition is admission r2's TRANSITION verdict for a CLI case.
func judgeTransition(control, treatment *p3Observed) CaseResult {
	r := CaseResult{CLIBackgroundEntry: true}
	switch {
	case p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0:
		r.Verdict, r.Reasons = VerdictBackgroundEntryReached, []string{"the treatment reached native's background or a wrong destination"}
	case treatment.CleanupFailed || control.CleanupFailed:
		r.Verdict, r.Reasons = providercert.Inconclusive, []string{providercert.ReasonCleanupFailed}
	case !p3RefusedBeforeStart(treatment, "ROUTING_PROVIDER_UNVERIFIED"):
		r.Verdict, r.Reasons = providercert.Inconclusive, []string{"no refusal before native start in the treatment"}
	case p3BoundaryReached(control):
		r.Verdict = providercert.Pass
	case p3NativeRefused(control):
		r.Verdict, r.Reasons = VerdictPathAbsent, []string{"the control native refused the combination"}
	default:
		r.Verdict, r.Reasons = providercert.Inconclusive, []string{"the control reached no background and native did not refuse"}
	}
	return r
}

// p3NativeRefused is admission r2's PATH_ABSENT evidence for a CLI control: native started for
// the stimulus and ended at once with a non-zero exit and an error on stderr (and, checked by
// the caller first, reached no background).
func p3NativeRefused(control *p3Observed) bool {
	s := control.Stimulus
	return control.NativeStarts > 0 && s.Exit != 0 && s.Exit != app.ExitCodeUnknown && s.Stderr && s.Took < 30*time.Second
}

// judgeLaunchRefusal is admission r2's verdict for a launch-refusal case (BE09, BE17): the
// treatment is refused with code before native starts; there is no control requirement.
func judgeLaunchRefusal(code string) func(control, treatment *p3Observed) CaseResult {
	return func(_, treatment *p3Observed) CaseResult {
		r := CaseResult{}
		switch {
		case p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0:
			r.Verdict = VerdictBackgroundEntryReached
		case p3RefusedBeforeStart(treatment, code):
			r.Verdict = providercert.Pass
		default:
			r.Verdict, r.Reasons = providercert.Inconclusive, []string{"no " + code + " before native start"}
		}
		return r
	}
}

// backgroundEntryCases are the CLI and launch-refusal checks of admission r2.
func backgroundEntryCases() map[string]func() scopeCase {
	storedA := p3TrustedStoredA
	cli := func(id string, prior bool, args ...string) func() scopeCase {
		return func() scopeCase {
			c := p3Case{ID: id, Entry: p3CLI, Hooks: p3HooksNormal, Setup: storedA, Expect: providercert.ExpectSuccess, Args: args, CLIPrior: prior,
				Established: func(*p3Observed) bool { return true }}
			return scopeCase{c: c, judge: judgeTransition, cliBackground: true}
		}
	}
	managedOverride := func(doc map[string]any) func() scopeCase {
		return func() scopeCase {
			c := p3Case{ID: "BE17", Entry: p3CLI, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, Args: []string{"-p", "{prompt}"},
				Established: func(*p3Observed) bool { return true }}
			c.Setup = func(w *p3World) error {
				if err := storedA(w); err != nil {
					return err
				}
				raw, _ := json.Marshal(doc)
				return os.WriteFile(filepath.Join(w.managedDir, "managed-settings.json"), raw, 0o600)
			}
			c.ProductEnv = func(w *p3World) map[string]string {
				return map[string]string{"CLAUDE_CODE_MANAGED_SETTINGS_PATH": w.managedDir}
			}
			return scopeCase{c: c, judge: judgeLaunchRefusal("AGENT_VIEW_MANAGED_OVERRIDE_REFUSED"), noControl: true}
		}
	}
	return map[string]func() scopeCase{
		"BE01": cli("BE01", false, "--bg", "{prompt}"),
		"BE02": cli("BE02", false, "--background", "{prompt}"),
		"BE03": cli("BE03", true, "--resume", "{session}", "--bg", "{prompt}"),
		"BE11": cli("BE11", true, "--continue", "--bg", "{prompt}"),
		"BE12": cli("BE12", true, "--resume", "{session}", "--fork-session", "--bg", "{prompt}"),
		"BE13": cli("BE13", false, "-p", "{prompt}", "--bg"),
		"BE09": func() scopeCase {
			c := p3Case{ID: "BE09", Entry: p3CLI, Hooks: p3HooksNormal, Setup: storedA, Expect: providercert.ExpectSuccess,
				Args: []string{"-p", "{prompt}", "--settings", `{"env":{"CLAUDE_CODE_DISABLE_AGENT_VIEW":"0"}}`}, Established: func(*p3Observed) bool { return true }}
			return scopeCase{c: c, judge: judgeLaunchRefusal("SETTINGS_CONFLICT"), noControl: true}
		},
		"BE10":         backgroundSubagentCase,
		"BE17":         managedOverride(map[string]any{"env": map[string]string{"CLAUDE_CODE_DISABLE_AGENT_VIEW": "0"}}),
		"BE17-setting": managedOverride(map[string]any{"disableAgentView": false}),
	}
}

// p3Served reports whether a backend call carrying canary id was served (the gateway forwarded
// it and the synthetic backend answered).
func p3Served(obs *p3Observed, id string) bool {
	return slices.ContainsFunc(obs.Calls, func(c p3Call) bool { return slices.Contains(c.canaries, id) })
}

// judgeTUIBoundary is admission r2's verdict for a TUI background-entry case with a paired
// control (TRANSITION and BE15/BE16): the treatment stays out of native's background, the
// stimulus was typed, the control's same stimulus did reach it, and the confirmation prompt
// typed after the stimulus was served through the gateway. fixture, when set, is BE15's
// marker check, judged before function.
func judgeTUIBoundary(fixture func(control, treatment *p3Observed) (string, string)) func(control, treatment *p3Observed) CaseResult {
	return func(control, treatment *p3Observed) CaseResult {
		r := CaseResult{}
		switch {
		case p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0:
			r.Verdict, r.Reasons = VerdictBackgroundEntryReached, []string{"the treatment reached native's background or a wrong destination"}
		case treatment.CleanupFailed || control.CleanupFailed:
			r.Verdict, r.Reasons = providercert.Inconclusive, []string{providercert.ReasonCleanupFailed}
		case !treatment.StimulusSent || !control.StimulusSent:
			r.Verdict, r.Reasons = providercert.Inconclusive, []string{"the stimulus was not typed"}
		case !p3BoundaryReached(control):
			r.Verdict, r.Reasons = providercert.Inconclusive, []string{"the control's stimulus reached no background"}
		}
		if r.Verdict != "" {
			return r
		}
		if fixture != nil {
			if verdict, reason := fixture(control, treatment); verdict != "" {
				return CaseResult{Verdict: verdict, Reasons: []string{reason}}
			}
		}
		if !p3Served(treatment, "turn2") {
			return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"the confirmation prompt after the stimulus was not served"}}
		}
		return CaseResult{Verdict: providercert.Pass}
	}
}

// tuiBoundaryCases are the TUI background-entry checks of admission r2 that PRINT_SINGLE needs.
func tuiBoundaryCases() map[string]func() scopeCase {
	stimulus := [][]string{{"/background reply with exactly the word plum", "\r"}, {"\x1b[D", "\x1b[D"}}
	return map[string]func() scopeCase{
		"BE15": func() scopeCase {
			c := p3Case{ID: "BE15", Entry: p3TUI, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, TUIStimulus: stimulus,
				Established: func(*p3Observed) bool { return true }}
			c.Setup = func(w *p3World) error {
				if err := p3TrustedStoredA(w); err != nil {
					return err
				}
				mark := w.addCanary("be15-managed-file")
				extra, _ := json.Marshal(map[string]string{"p3_be15": string(mark.Value)})
				policy, _ := json.Marshal(map[string]any{"env": map[string]string{"CLAUDE_CODE_DISABLE_AGENT_VIEW": "0", "CLAUDE_CODE_EXTRA_BODY": string(extra)}})
				return os.WriteFile(filepath.Join(w.managedDir, "managed-settings.json"), policy, 0o600)
			}
			fixture := func(control, treatment *p3Observed) (string, string) {
				inControl, inTreatment := p3SawCanary(control, "be15-managed-file"), p3SawCanary(treatment, "be15-managed-file")
				switch {
				case !inControl && !inTreatment:
					return VerdictFixtureNotApplied, "the managed file's marker reached no receiver in control or treatment"
				case inControl && !inTreatment:
					return providercert.FailFunction, "the treatment did not read the managed file the control read"
				}
				return "", ""
			}
			return scopeCase{c: c, judge: judgeTUIBoundary(fixture)}
		},
		"BE16": func() scopeCase {
			c := p3Case{ID: "BE16", Entry: p3TUI, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, TUIStimulus: stimulus,
				Established: func(*p3Observed) bool { return true }}
			c.Setup = func(w *p3World) error {
				if err := p3TrustedStoredA(w); err != nil {
					return err
				}
				return w.mergeUserSettings("disableAgentView", false)
			}
			return scopeCase{c: c, judge: judgeTUIBoundary(nil)}
		},
	}
}

func backgroundSubagentCase() scopeCase {
	agent := &p3AgentBackgroundOnce{}
	c := p3Case{ID: "BE10", Entry: p3Print, Hooks: p3HooksNormal, Setup: p3TrustedStoredA, Expect: providercert.ExpectSuccess,
		Args: []string{"--allowedTools", "Agent"}, Agent: agent, WatchBoundary: true, Established: func(*p3Observed) bool { return true }}
	judge := func(_, treatment *p3Observed) CaseResult {
		return agent.judge(treatment)
	}
	return scopeCase{c: c, judge: judge, noControl: true}
}

// VerdictPathAbsentTUIObserved is BE14's disposition (owner decision 15): the TUI /resume picker
// was opened and cancelled in this run, the control never reached native's background, and the
// treatment stayed fully inside the product. It is never PASS and covers only this stimulus.
const VerdictPathAbsentTUIObserved = "PATH_ABSENT_TUI_OBSERVED"

// p3WrongTraffic is the bytes and connections the wrong receivers saw.
func p3WrongTraffic(obs *p3Observed) int64 {
	n := int64(0)
	for _, w := range obs.Wrong {
		n += w.Bytes + w.Conns
	}
	return n
}

// be14Guards switch BE14's condition checks; production has them all on. A test turns one off to
// show that the check is what decides the verdict.
type be14Guards struct {
	ControlPicker, ControlBoundary, TreatmentSafe, Confirmed, Unchanged bool
}

var be14AllGuards = be14Guards{true, true, true, true, true}

// judgeBE14 is BE14's verdict: the TRANSITION judgment when the control reached native's
// background, and otherwise PATH_ABSENT_TUI_OBSERVED only when every condition of owner decision 15
// holds in this run, with each condition's evidence in the reasons. Any other shortfall is
// INCONCLUSIVE naming the failing condition.
func judgeBE14(control, treatment *p3Observed) CaseResult {
	return judgeBE14With(control, treatment, be14AllGuards)
}

func judgeBE14With(control, treatment *p3Observed, g be14Guards) CaseResult {
	transition := judgeTUIBoundary(nil)
	if p3BoundaryReached(control) && g.ControlBoundary {
		return transition(control, treatment)
	}
	inconclusive := func(reason string) CaseResult {
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{"BE14 " + reason}}
	}
	// Anything the treatment did outside the product is a failure, whatever else holds.
	if p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0 || p3WrongTraffic(treatment) != 0 {
		return CaseResult{Verdict: VerdictBackgroundEntryReached, Reasons: []string{"the treatment reached native's background or a wrong destination"}}
	}
	if treatment.CleanupFailed || control.CleanupFailed {
		return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{providercert.ReasonCleanupFailed}}
	}
	// A run that timed out, did not start or could not take its identity checkpoints is a
	// harness defect: its record shows nothing about the picker.
	for _, run := range []struct {
		name string
		obs  *p3Observed
	}{{"control", control}, {"treatment", treatment}} {
		for _, defect := range []struct {
			on     bool
			reason string
		}{{run.obs.TimedOut, providercert.ReasonTimeout}, {run.obs.SpawnFailed, providercert.ReasonSpawnFailed}, {run.obs.IdentityError, providercert.ReasonHarness}} {
			if defect.on {
				return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{defect.reason, "BE14 harness defect in the " + run.name + " run"}}
			}
		}
	}
	var evidence []string
	// 1. The control opened the picker and cancelled it: input marks and the screen.
	if g.ControlPicker {
		if control.StimulusMarks < 2 || !control.StimulusSent {
			return inconclusive(fmt.Sprintf("condition 1: the control's driver began %d of the 2 stimulus inputs (/resume Enter, Esc)", control.StimulusMarks))
		}
		if !control.ScreenOrdered {
			return inconclusive("condition 1: the control's screen did not show 'Resume session' and then 'Resume cancelled'")
		}
	}
	evidence = append(evidence, fmt.Sprintf("1 control: driver began %d stimulus inputs (/resume Enter, Esc); screen showed 'Resume session' then 'Resume cancelled'", control.StimulusMarks))
	// 2. The control reached no background.
	if g.ControlBoundary && p3BoundaryReached(control) {
		return inconclusive("condition 2: the control reached native's background")
	}
	evidence = append(evidence, "2 control boundary 0: no daemon lock, job directory, daemon or --bg-pty-host native")
	// 3. The treatment reached no background and no wrong destination.
	if g.TreatmentSafe && (p3BoundaryReached(treatment) || p3WrongTraffic(treatment) != 0) {
		return inconclusive("condition 3: the treatment reached a background or a wrong destination")
	}
	if !treatment.StimulusSent || treatment.StimulusMarks < 2 {
		return inconclusive(fmt.Sprintf("condition 3: the treatment's driver began %d of the 2 stimulus inputs", treatment.StimulusMarks))
	}
	evidence = append(evidence, "3 treatment boundary 0, wrong-destination bytes and connections 0")
	// 4. The treatment's confirmation prompt after the cancel completed through the gateway.
	if g.Confirmed && (!p3Served(treatment, "turn2") || !strings.Contains(treatment.Outputs, marker(providercert.IntendedID, "turn2"))) {
		return inconclusive("condition 4: the treatment's confirmation prompt after the cancel was not served and answered through the gateway")
	}
	evidence = append(evidence, "4 treatment confirmation prompt (turn2) served and answered through the gateway")
	// 5. Session, profile snapshot and the confirmation turn's model selection are unchanged.
	if g.Unchanged {
		if treatment.SessionBefore == nil || len(treatment.SessionBefore.Profiles) == 0 || !treatment.SessionBefore.same(treatment.SessionAfter) {
			return inconclusive("condition 5: the treatment's session id or profile snapshot is not shown unchanged across the stimulus")
		}
		inBefore, beBefore := p3SelectionsOf(treatment, "turn1", "turn2")
		inAfter, beAfter := p3SelectionsOf(treatment, "turn2", "")
		if !inAfter.within(inBefore) || !beAfter.within(beBefore) {
			return inconclusive(fmt.Sprintf("condition 5: the confirmation turn's model selection (%v, %v) is not within the first turn's (%v, %v)", inAfter, beAfter, inBefore, beBefore))
		}
	}
	evidence = append(evidence, "5 treatment session id, profile snapshot and the confirmation turn's model selection unchanged")
	return CaseResult{Verdict: VerdictPathAbsentTUIObserved, Reasons: evidence}
}
