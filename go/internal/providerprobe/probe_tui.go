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
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3TUI is the interactive entry (R05): native runs inside a Windows pseudo console driven by
// cmd/ptydrive, which types the turn's prompt and waits for a probe reply on the screen.
const p3TUI = "tui"

var (
	ptydriveOnce sync.Once
	ptydrivePath string
	ptydriveErr  error
)

// ptydrive builds the repository's ConPTY driver once per test process.
func ptydrive() (string, error) {
	ptydriveOnce.Do(func() {
		dir, err := os.MkdirTemp("", "clauduct-ptydrive-")
		if err != nil {
			ptydriveErr = err
			return
		}
		ptydrivePath = filepath.Join(dir, "ptydrive.exe")
		build := exec.Command("go", "build", "-o", ptydrivePath, "github.com/wotjr1649/Clauduct/go/cmd/ptydrive")
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			ptydriveErr = fmt.Errorf("build ptydrive: %v %s", err, out)
		}
	})
	return ptydrivePath, ptydriveErr
}

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
		{"wait": "Managedsettingsrequireapproval", "send": []string{"\r"}, "optional": true, "timeout_s": 10, "after_ms": 300},
		{"wait": "Choosethetextstyle", "send": []string{"\r"}, "optional": true, "timeout_s": 10, "after_ms": 300},
		{"wait": "Entertocontinue", "send": []string{"\r"}, "optional": true, "timeout_s": 5, "after_ms": 300},
		{"wait": "trustthisfolder", "send": []string{"\x1b[B", "\r"}, "optional": true, "timeout_s": 10, "after_ms": 300},
		// The main prompt's footer: "? for shortcuts", or 2.1.292's "auto mode on (shift+tab to cycle)".
		{"wait": `forshortcuts|shift\+tabtocycle`, "send": []string{prompt, "\r"}, "timeout_s": 30, "after_ms": 500},
		{"wait": "CLAUDUCT-PROBE-REPLY-", "send": []string{"/exit", "\r"}, "timeout_s": 60, "after_ms": 1000},
	}
}

// tuiPinnedStarter starts native inside ptydrive under the contract's spawn pin: the executable
// is held against write, delete and rename until the claude.exe process ptydrive started is
// confirmed to run the pinned file. A run whose native image cannot be confirmed is ended and
// reported, never accepted.
func tuiPinnedStarter(steps, screen, raw, marks string, timeoutSeconds int) func(spawnReport) starter {
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
			args := append([]string{"-dir", spec.Dir, "-script", steps, "-text", screen, "-log", raw, "-marks", marks, "-timeout", strconv.Itoa(timeoutSeconds), "--", spec.File}, spec.Args...)
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

// promptSent is when ptydrive began typing the prompt step, from its marks file.
// p3TUISteps2 is two prompts in one TUI session: the first reply, a wait, the second prompt,
// its reply, /exit.
func p3TUISteps2(prompt1, prompt2 string, wait time.Duration) []map[string]any {
	base := p3TUISteps(prompt1, nil)
	steps := base[:len(base)-1]
	return append(steps,
		// The first reply, the wait, then the second prompt typed at once: the prompt box is
		// already on screen, so a wait for its footer would find no new text.
		map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-", "send": []string{prompt2, "\r"}, "timeout_s": 60, "after_ms": wait.Milliseconds()},
		map[string]any{"wait": "CLAUDUCT-PROBE-REPLY-", "send": []string{"/exit", "\r"}, "timeout_s": 60, "after_ms": 1000})
}

// p3TUIPrompt2Step is the index of p3TUISteps2's second prompt.
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
