package providerprobe

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// tuiCases are the TUI scope's cases beyond R05 (admission r2 scopes.TUI): the r4 multi-turn
// cases run through the TUI entry with their own rules, X06 and X07 (r4 releaseMatrix), PM02T,
// and the TUI background-entry checks.
func tuiCases() map[string]func() scopeCase {
	reg := map[string]func() scopeCase{}
	for _, c := range p3PrintFamily() {
		if !slices.Contains([]string{"R04L", "R12o", "R09", "R10"}, c.ID) {
			continue
		}
		c := c
		c.ID, c.Entry = c.ID+"-TUI", p3TUI
		reg[c.ID] = func() scopeCase { return scopeCase{c: c} }
	}
	for _, c := range p3TUIFamily() {
		c := c
		reg[c.ID] = func() scopeCase { return scopeCase{c: c} }
	}
	storedA := func(w *p3World) error { return w.storeGateway(w.sgA) }
	profile := []string{"SESSION_PROFILE_UNVERIFIED"}
	for id, hooks := range map[string]string{"X06": p3HooksMissing, "X07": p3HooksTimeout} {
		c := p3Case{ID: id, Entry: p3TUI, Hooks: hooks, Setup: storedA, Expect: providercert.ExpectFailClosed, FailClosedAs: profile,
			Established: gotCanary("WRONG_STORED_GATEWAY_A", 0)}
		reg[id] = func() scopeCase { return scopeCase{c: c} }
	}
	reg["PM02T"] = pm02TUI
	transition := func(id string, stimulus ...[]string) func() scopeCase {
		return func() scopeCase {
			c := p3Case{ID: id, Entry: p3TUI, Hooks: p3HooksNormal, Setup: p3TrustedStoredA, Expect: providercert.ExpectSuccess, TUIStimulus: stimulus,
				Established: func(*p3Observed) bool { return true }}
			return scopeCase{c: c, judge: judgeTUIBoundary(nil)}
		}
	}
	reg["BE04"] = transition("BE04", []string{"/bg", "\r"})
	reg["BE05"] = transition("BE05", []string{"/background reply with exactly the word plum", "\r"})
	reg["BE06"] = forkCase
	reg["BE07"] = transition("BE07", []string{"\x1b[D", "\x1b[D"})
	reg["BE14"] = transition("BE14", []string{"/resume", "\r"}, []string{"\x1b"})
	reg["BE08"] = func() scopeCase {
		c := p3Case{ID: "BE08", Entry: p3CLI, Hooks: p3HooksNormal, Setup: p3TrustedStoredA, Expect: providercert.ExpectSuccess, Args: []string{"agents"},
			Established: func(*p3Observed) bool { return true }}
		return scopeCase{c: c, judge: judgeLaunchRefusal("ROUTING_PROVIDER_UNVERIFIED"), noControl: true}
	}
	return reg
}

// pm02TUI is PM02T: with an ask rule for Bash the TUI asks before the scripted Bash call, the
// driver refuses, and the next prompt carries native's tool result for the refusal.
func pm02TUI() scopeCase {
	bash := &p3BashOnce{}
	c := p3Case{ID: "PM02T", Entry: p3TUI, Turns: 2, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess, Args: []string{"--allowedTools", "Bash"},
		TUIApproval: []string{"\x1b"}, Script: bash.script, Established: gotCanary("WRONG_STORED_GATEWAY_A", 0),
		// The first prompt delivers the tool call the driver refuses: native ends that turn without
		// a reply by design. The second prompt, which carries the refusal, must be answered.
		ExpectTurns: map[int]string{0: providercert.ExpectStimulus}}
	c.Setup = func(w *p3World) error {
		bash.reset(filepath.Join(w.cwd, "p3-pm-marker.txt"))
		if err := p3TrustedStoredA(w); err != nil {
			return err
		}
		return w.mergeUserSettings("permissions", map[string]any{"ask": []string{"Bash"}})
	}
	function := func(_, _ *p3Observed) (string, string) {
		bash.mu.Lock()
		defer bash.mu.Unlock()
		_, statErr := os.Stat(bash.marker)
		observed := fmt.Sprintf("marker written=%t, Bash offered=%t, results %q", statErr == nil, bash.offered, bash.results)
		if statErr == nil || !slices.ContainsFunc(bash.results, p3UserRejected) {
			return providercert.FailFunction, "the refusal is not native's user-rejection tool result; " + observed
		}
		return "", observed
	}
	return scopeCase{c: c, function: function}
}

// p3UserRejected is the tool result native returns for a call the user refused in the TUI.
func p3UserRejected(result string) bool {
	lower := strings.ToLower(result)
	return strings.Contains(lower, "doesn't want to proceed") || strings.Contains(lower, "rejected")
}

// forkCase is BE06: /fork after a reply. The forked work must complete in the same process
// through the gateway: a request carrying the fork's task, sent before the confirmation prompt
// (turn2) exists, reaches the backend. A later main request may carry the /fork line in its
// history; it also carries turn2's canary, so it is not counted. Each run counts its own.
func forkCase() scopeCase {
	var mu sync.Mutex
	forked := 0
	c := p3Case{ID: "BE06", Entry: p3TUI, Hooks: p3HooksNormal, Expect: providercert.ExpectSuccess,
		TUIStimulus: [][]string{{"/fork p3-be06-fork: reply with exactly the word plum", "\r"}}, Established: func(*p3Observed) bool { return true }}
	c.Setup = func(w *p3World) error {
		mu.Lock()
		forked = 0
		mu.Unlock()
		return p3TrustedStoredA(w)
	}
	c.Script = func(body string, canaries []string) string {
		if strings.Contains(body, "p3-be06-fork") && !slices.Contains(canaries, "turn2") {
			mu.Lock()
			forked++
			mu.Unlock()
		}
		return ""
	}
	boundary := judgeTUIBoundary(nil)
	judge := func(control, treatment *p3Observed) CaseResult {
		r := boundary(control, treatment)
		mu.Lock()
		defer mu.Unlock()
		if r.Verdict == providercert.Pass && forked == 0 {
			return CaseResult{Verdict: providercert.FailFunction, Reasons: []string{"no request of the forked work reached the backend through the gateway"}}
		}
		return r
	}
	return scopeCase{c: c, judge: judge}
}
