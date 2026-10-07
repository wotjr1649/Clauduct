package providerprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
			return scopeCase{c: c, judge: judgeTransition}
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

// p3AgentBackgroundOnce answers the first main request that offers the Agent tool with one
// Agent call whose run_in_background is true (BE10), and records what followed: whether a
// later main request carried the call's output back, and whether the subagent's own request
// (the task's marker, no Agent tool) reached the backend through the gateway.
type p3AgentBackgroundOnce struct {
	mu                         sync.Mutex
	issued, returned, subagent bool
}

const p3BE10Task = "p3-be10-task: reply with exactly the word plum"

func (a *p3AgentBackgroundOnce) script(body string, _ []string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	// The subagent's requests carry the turn's canary too (measured): what tells them apart is
	// that only the main thread is offered the Agent tool.
	main := strings.Contains(body, `"name":"Agent"`)
	switch {
	case !main && strings.Contains(body, "p3-be10-task"):
		a.subagent = true
		return ""
	case main && a.issued && strings.Contains(body, "function_call_output"):
		a.returned = true
		return ""
	case main && !a.issued:
		a.issued = true
		args, _ := json.Marshal(map[string]any{"description": "p3 background subagent", "prompt": p3BE10Task, "subagent_type": "general-purpose", "run_in_background": true})
		return toolStream("call_p3be10", "Agent", string(args))
	}
	return ""
}

func backgroundSubagentCase() scopeCase {
	agent := &p3AgentBackgroundOnce{}
	c := p3Case{ID: "BE10", Entry: p3Print, Hooks: p3HooksNormal, Setup: p3TrustedStoredA, Expect: providercert.ExpectSuccess,
		Args: []string{"--allowedTools", "Agent"}, Script: agent.script, WatchBoundary: true, Established: func(*p3Observed) bool { return true }}
	judge := func(_, treatment *p3Observed) CaseResult {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		switch {
		case p3BoundaryReached(treatment) || p3WrongBytes(treatment) != 0:
			return CaseResult{Verdict: VerdictBackgroundEntryReached}
		case treatment.CleanupFailed:
			return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{providercert.ReasonCleanupFailed}}
		case !agent.issued || !agent.returned:
			return CaseResult{Verdict: providercert.Inconclusive, Reasons: []string{fmt.Sprintf("the background Agent call did not reach native (issued=%t returned=%t)", agent.issued, agent.returned)}}
		case !agent.subagent:
			return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"the background subagent's request did not reach the backend through the gateway"}}
		}
		return CaseResult{Verdict: providercert.Pass}
	}
	return scopeCase{c: c, judge: judge, noControl: true}
}
