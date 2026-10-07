package providerprobe

import (
	"encoding/json"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3TUI is the interactive entry (R05): native runs inside a Windows pseudo console driven by
// cmd/ptydrive, which types the turn's prompt and waits for a probe reply on the screen.
const p3TUI = "tui"

// p3PtyDriveArg starts this binary as the ConPTY driver (internal/ptydrive): the probe drives
// the TUI with its own code, with no toolchain or separate driver binary on the machine.
const p3PtyDriveArg = "--p3-ptydrive"

// ptydrive is this executable, started with p3PtyDriveArg first.
func ptydrive() (string, error) { return os.Executable() }

// p3TUIReady is the main prompt's footer: "? for shortcuts", or 2.1.292's "auto mode on
// (shift+tab to cycle)". Whitespace removed, like every wait.
const p3TUIReady = `forshortcuts|shift\+tabtocycle`

// p3TUISteps answers native's first-run screens, types the prompt, waits for any probe reply on
// the screen and leaves with /exit. Waits match the screen text with whitespace removed.
func p3TUISteps(prompt string, afterReply []map[string]any) []map[string]any {
	if afterReply != nil {
		// The reply step sends the case's own steps instead of /exit; they end the session.
		base := p3TUISteps(prompt, nil)
		base[len(base)-1]["send"] = afterReply[0]["send"]
		base[len(base)-1]["after_ms"] = afterReply[0]["after_ms"]
		return append(base, afterReply[1:]...)
	}
	return []map[string]any{
		// The synthetic stored gateway's managed settings, shown first at start: the threat a user
		// who once trusted that gateway carries (control). A treatment that ignores the stored
		// gateway never sees it.
		// Each optional screen ends as soon as its own dialog shows or the main prompt is up
		// (until), whichever comes first: a screen that never appears costs no fixed wait.
		// A screen's until also names the screens that follow it: a start that shows the text
		// style dialog first never shows the approval dialog.
		{"wait": "Managedsettingsrequireapproval", "send": []string{"\r"}, "optional": true, "timeout_s": 10, "after_ms": 300, "until": "Choosethetextstyle|Entertocontinue|trustthisfolder|" + p3TUIReady},
		{"wait": "Choosethetextstyle", "send": []string{"\r"}, "optional": true, "timeout_s": 10, "after_ms": 300, "until": "Entertocontinue|trustthisfolder|" + p3TUIReady},
		{"wait": "Entertocontinue", "send": []string{"\r"}, "optional": true, "timeout_s": 5, "after_ms": 300, "until": "trustthisfolder|" + p3TUIReady},
		{"wait": "trustthisfolder", "send": []string{"\x1b[B", "\r"}, "optional": true, "timeout_s": 10, "after_ms": 300, "until": p3TUIReady},
		{"wait": p3TUIReady, "send": []string{prompt, "\r"}, "timeout_s": 30, "after_ms": 500},
		{"wait": p3TUIEnded, "send": []string{"/exit", "\r"}, "timeout_s": 60, "after_ms": 1000},
	}
}

// p3TUIDriverReserve is how long before the case's own limit the driver must be done: it ends
// its steps earlier still (ptydrive driverReserve), writes its screen and exits, so the case's
// context never cuts it off and its diagnostics survive. The rest covers the driver's start and
// the spawn pin check, which happen after the limit is set.
const p3TUIDriverReserve = 6 * time.Second

// tuiDriverDeadline is the seconds the driver may run, from now, before limit minus the reserve.
func tuiDriverDeadline(limit time.Time) int {
	return max(int((time.Until(limit)-p3TUIDriverReserve)/time.Second), 3)
}

// tuiPinnedStarter starts native inside ptydrive under the contract's spawn pin: the executable
// is held against write, delete and rename until the claude.exe process ptydrive started is
// confirmed to run the pinned file. A run whose native image cannot be confirmed is ended and
// reported, never accepted.
func tuiPinnedStarter(steps, screen, raw, marks string, timeoutSeconds int, deadlineSeconds func() int) func(spawnReport) starter {
	return func(report spawnReport) starter {
		return func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (app.Process, error) {
			driver, err := ptydrive()
			if err != nil {
				report(providercert.Identity{}, err)
				return nil, err
			}
			pin, err := providercert.PinFile(spec.File)
			if err != nil {
				report(providercert.Identity{}, fmt.Errorf("PRE_SPAWN pin: %w", err))
				return nil, err
			}
			defer pin.Close()
			args := append([]string{p3PtyDriveArg, "-dir", spec.Dir, "-script", steps, "-text", screen, "-log", raw, "-marks", marks, "-timeout", strconv.Itoa(timeoutSeconds), "-deadline", strconv.Itoa(deadlineSeconds()), "--", spec.File}, spec.Args...)
			cmd := exec.Command(driver, args...)
			cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = spec.Dir, spec.Env, stdout, stderr
			process, err := childprocess.Start(cmd)
			if err != nil {
				return nil, err
			}
			native, err := descendantImage(uint32(process.PID()), "claude.exe", 15*time.Second)
			if err == nil {
				var same bool
				if same, err = pin.SameFile(native); err == nil && !same {
					err = fmt.Errorf("image is not the pinned executable")
				}
			}
			if err != nil {
				process.Stop()
				report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: %w", err))
				return nil, fmt.Errorf("POST_SPAWN unverified")
			}
			report(pin.Identity, nil)
			return process, nil
		}
	}
}

// descendantImage waits for a process named image under root and returns its image path.
func descendantImage(root uint32, image string, wait time.Duration) (string, error) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		table := snapshotProcesses()
		for pid, p := range table {
			if p.image == image && table.descends(pid, root) {
				return imagePath(int(pid))
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("no %s under the driver within %s", image, wait)
}

// writeTUISteps writes the steps for one turn and returns the script and screen paths.
func writeTUISteps(dir, prompt string, afterReply []map[string]any) (string, string, error) {
	steps := filepath.Join(dir, "steps.json")
	raw, _ := json.Marshal(p3TUISteps(prompt, afterReply))
	return steps, filepath.Join(dir, "screen.txt"), os.WriteFile(steps, raw, 0o600)
}

// p3TUIFamily is the interactive row of the runtime matrix.
func p3TUIFamily() []p3Case {
	storedA := func(w *p3World) error { return w.storeGateway(w.sgA) }
	return []p3Case{
		{ID: "R05", Entry: p3TUI, Hooks: p3HooksNormal, Setup: storedA, Expect: providercert.ExpectSuccess, Established: gotCanary("WRONG_STORED_GATEWAY_A", 0)},
	}
}

// p3RedactCanaries keeps the end of a screen for evidence with every canary value replaced by
// its ID, so the evidence carries no canary.
func p3RedactCanaries(text string, turns []p3Turn) string {
	for _, tr := range turns {
		text = strings.ReplaceAll(text, string(tr.Canary.Value), "<canary:"+tr.Canary.ID+">")
	}
	if len(text) > 4000 {
		text = text[len(text)-4000:]
	}
	return text
}

// p3AgentSessionEnv names the environment a Claude Code session leaves for the processes it
// starts. The probe's natives must not inherit it from a test that runs under one.
func p3AgentSessionEnv(upper string) bool {
	return upper == "CLAUDECODE" || upper == "AI_AGENT" || strings.HasPrefix(upper, "CLAUDE_")
}

// ansiSequence matches terminal control sequences in a raw ConPTY log.
var ansiSequence = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)|\x1b[()][A-Za-z0-9]|\x1b[=>]")

// rawScreen is a raw ConPTY log as text: control sequences removed.
func rawScreen(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return ansiSequence.ReplaceAllString(string(raw), "")
}

// p3TUIPromptStep is the index of the step in p3TUISteps that types the turn's prompt.
const p3TUIPromptStep = 4

// p3TUIPrompt2Step is the index of the first step after the first prompt's reply: the second
// prompt, or a stimulus.
var p3TUIPrompt2Step = len(p3TUISteps("", nil)) - 1

// markAt is the time the driver began step index's input.
func markAt(marks string, index int) (time.Time, bool) {
	raw, err := os.ReadFile(marks)
	if err != nil {
		return time.Time{}, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		i, at, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && i == strconv.Itoa(index) {
			if when, err := time.Parse(time.RFC3339Nano, at); err == nil {
				return when, true
			}
		}
	}
	return time.Time{}, false
}

// promptSent is when ptydrive began typing the prompt step, from its marks file.
func promptSent(marks string) (time.Time, bool) {
	raw, err := os.ReadFile(marks)
	if err != nil {
		return time.Time{}, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		index, at, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && index == strconv.Itoa(p3TUIPromptStep) {
			if when, err := time.Parse(time.RFC3339Nano, at); err == nil {
				return when, true
			}
		}
	}
	return time.Time{}, false
}

// p3TUIStimulusSteps types prompt1, waits for its reply, types each stimulus (the first right
// after the reply, the rest 3 seconds apart), then prompt2 8 seconds after the last one, and
// leaves once prompt2's own reply is on screen. It returns the steps and prompt2's step index;
// the first stimulus is always at p3TUIPrompt2Step.
func p3TUIStimulusSteps(prompt1, prompt2 string, stimulus [][]string) ([]map[string]any, int) {
	base := p3TUISteps(prompt1, nil)
	steps := append(base[:len(base)-1:len(base)-1],
		map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-", "send": stimulus[0], "timeout_s": 60, "after_ms": 1000})
	for _, s := range stimulus[1:] {
		steps = append(steps, map[string]any{"send": s, "after_ms": 3000})
	}
	confirm := len(steps)
	return append(steps,
		map[string]any{"send": []string{prompt2, "\r"}, "after_ms": 8000},
		map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-[A-Z_]+-turn2", "send": []string{"/exit", "\r"}, "timeout_s": 60, "after_ms": 1000}), confirm
}

// p3TUIStepsN types prompts in one session. Before prompt k+1 the driver waits for a reply on
// screen and, when ready[k-1] is set, for that file (a state change the probe made); otherwise it
// waits settle. approval, when set, answers the permission dialog the first prompt raises.
func p3TUIStepsN(prompts []string, ready []string, settle time.Duration, approval []string, refresh bool) []map[string]any {
	base := p3TUISteps(prompts[0], nil)
	steps := base[: len(base)-1 : len(base)-1]
	if approval != nil {
		// Optional: the control's unscripted receivers ask for no tool, so no dialog appears
		// there. The treatment's verdict reads the marker and the tool result, not this step.
		steps = append(steps, map[string]any{"wait": "Doyouwanttoproceed", "send": approval, "optional": true, "timeout_s": 30, "after_ms": 500})
	}
	for k := 1; k < len(prompts); k++ {
		step := map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-", "send": []string{prompts[k], "\r"}, "timeout_s": 60, "after_ms": settle.Milliseconds()}
		if approval != nil && k == 1 {
			// A refused tool ends the turn without a reply: the next prompt goes in once the
			// refusal has settled, and carries the tool result to the backend.
			delete(step, "wait")
			step["after_ms"] = 3000
		}
		if ready[k-1] != "" {
			step["wait_file"], step["after_ms"] = ready[k-1], 500
			step["timeout_s"] = int((settle + 2*time.Minute).Seconds())
		}
		if refresh && k == 1 {
			// RP01: a refreshed policy with security-sensitive env asks for approval in the TUI
			// (measured 2026-10-08). The driver approves it, as the user it models would; the
			// next prompt then carries the refreshed settings. A treatment that never fetches
			// sees no dialog, and the optional wait ends 3 minutes before the planned wait.
			steps = append(steps,
				map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-", "send": []string{}, "timeout_s": 60},
				map[string]any{"wait": p3TUIPolicyApproval, "send": []string{"\r"}, "optional": true, "timeout_s": int((settle - 3*time.Minute).Seconds()), "after_ms": 500})
			delete(step, "wait")
			step["after_ms"] = (3 * time.Minute).Milliseconds()
		}
		steps = append(steps, step)
	}
	return append(steps, map[string]any{"wait": p3TUIEnded, "send": []string{"/exit", "\r"}, "timeout_s": 60, "after_ms": 1000})
}

// p3TUIPromptNStep is the driver step index that types prompt n (n >= 2) in p3TUIStepsN.
func p3TUIPromptNStep(n int, approval, refresh bool) int {
	i := p3TUIPrompt2Step + n - 2
	if approval {
		i++
	}
	if refresh {
		i += 2
	}
	return i
}

// p3TUIPolicyApproval is the managed-settings approval dialog's first choice, whitespace removed.
const p3TUIPolicyApproval = "Yes,Itrustthesesettings"

// p3TUIEnded is the screen a turn ends on: a probe reply, or the API error a fail-closed turn
// shows. It only tells the driver when to leave; the verdict reads the receivers.
const p3TUIEnded = "CLAUDUCT-PROBE-REPLY-|APIError:"
