package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// The session's last word.
//
// Anything this build wants to tell the user has to wait until here. The native client owns
// the terminal while it runs, so text written during a session lands inside the prompt box.
//
// Two departures from the Node baseline, both about where the words go.
//
// The baseline prints the status JSON on stdout. This does not, ever. `claude -p "..."` puts
// its answer on stdout and that is the whole point of the flag; a JSON line appended to it
// breaks `> out.txt` and breaks `| jq`, which is exactly the use that cannot tolerate it.
// Everything here goes to stderr and to a file.
//
// And the JSON is not printed unless there is something to say. A session that ran clean has
// nothing to report and a line of JSON on every exit is noise a reader learns to skip, which
// is how the line stops being read before the day it matters. The file is always written, so
// the whole account is always available; CLAUDUCT_STATUS=1 prints it regardless.

// statusEnv forces the whole account onto stderr.
const statusEnv = "CLAUDUCT_STATUS"

// How a session ended.
const (
	CategorySuccess     = "SUCCESS"
	CategoryClientFail  = "CLIENT_FAILED"
	CategoryStartFailed = "CLIENT_START_FAILED"
	CategoryBudget      = "REQUEST_BUDGET"
	CategoryCancelled   = "USER_CANCELLED"
	CategoryDeadline    = "SESSION_DEADLINE"
)

// SessionFacts describe launcher defaults and installed integration components.
//
// Context defaults are not a reading of the native client's effective configuration.
// Desired model policies and backend observations are reported by the gateway.
type SessionFacts struct {
	// StartupModel and StartupEffort are what a session begins on when the user names
	// nothing. A default, never a reading of what the session went on to run.
	//
	// Named that way since 2026-09-18, and the rename is the whole fix. They were called
	// model and effort, and until the startup effort moved onto --effort they were
	// accidentally accurate -- CLAUDE_CODE_EFFORT_LEVEL pinned a session to exactly this
	// value, so nothing could make them wrong. Letting a session change its effort made
	// them routinely wrong, and a field called "effort" under a struct called session is
	// read as what the session ran at.
	//
	// There is no single honest value for that. A session runs its parent and its
	// subagents on different routes at the same time, which is the point of role routing.
	// Recent answers it per request, and exactly.
	StartupModel          string                `json:"startupModel"`
	StartupEffort         string                `json:"startupEffort"`
	NativeContextDefaults NativeContextDefaults `json:"nativeContextDefaults"`
	NonStreamingFallback  bool                  `json:"nonStreamingFallbackDisabled"`
	DelegationMenuEntries int                   `json:"delegationMenuEntries"`
	// HookInstalled is whether the subagent hook was found beside this executable.
	//
	// Reported because its absence is silent otherwise. findHook looks beside the binary
	// and nowhere else, so a package that shipped without clauduct-hook runs perfectly --
	// and role routing never happens, and the delegation menu moves the model without the
	// effort. That is a failure nobody would think to look for.
	HookInstalled bool `json:"hookInstalled"`
}

// Launcher defaults may be overridden by the user's environment. They are not
// evidence that a native agent applied them or that gateway policy was enforced.
type NativeContextDefaults struct {
	Window              int     `json:"window"`
	AutoCompactWindow   int     `json:"autoCompactWindow"`
	CompactPercent      float64 `json:"compactPercent"`
	ApplicationVerified bool    `json:"applicationVerified"`
}

// Status is the whole account of one session.
type Status struct {
	Completion CompletionFacts     `json:"completion"`
	Category   string              `json:"category"`
	ExitCode   int                 `json:"exitCode"`
	Attempts   int                 `json:"attempts"`
	Inferences int                 `json:"inferences"`
	Session    SessionFacts        `json:"session"`
	Gateway    gateway.Diagnostics `json:"gateway"`
	Lifecycle  *LifecycleFacts     `json:"lifecycle,omitempty"`
	// CleanupFailed is whether releasing what the session owned failed. Only the fact: the
	// error text can name paths, and it is on stderr already (#91).
	CleanupFailed bool `json:"cleanupFailed"`
}

// Process exit, transport success and task completion are different claims.
// Acceptance is never inferred from exit=0 or a model's own report.
type CompletionFacts struct {
	Acceptance            string `json:"acceptance"`
	APIFailures           int64  `json:"apiFailures"`
	CancelledRequests     int64  `json:"cancelledRequests"`
	NativeCancellations   int64  `json:"nativeCancellations"`
	NativeToolFailures    int64  `json:"nativeToolFailures"`
	RejectedWorkflowCalls int64  `json:"rejectedWorkflowCalls"`
	UnacquiredResults     int    `json:"unacquiredResultsInRecent"`
	ControlTransitions    int64  `json:"controlTransitions"`
}

func completionFacts(d gateway.Diagnostics) CompletionFacts {
	out := CompletionFacts{Acceptance: "not_assessed"}
	for category, n := range d.Totals.Failures {
		if category == "CANCELLED" {
			out.CancelledRequests += n
		} else {
			out.APIFailures += n
		}
	}
	out.NativeCancellations = d.AgentResults.Totals["cancelled"]
	out.RejectedWorkflowCalls = d.Totals.RejectedWorkflowCalls
	for _, n := range d.Totals.Controls {
		out.ControlTransitions += n
	}
	out.NativeToolFailures = d.NativeToolFailures.Total
	// Old clients without the native failure event can still expose Agent errors
	// in their next request. Do not add the two sources and double-count failures.
	if n := d.AgentSelections.Totals["native_tool_failed"]; n > out.NativeToolFailures {
		out.NativeToolFailures = n
	}
	for _, r := range d.AgentResults.Recent {
		if r.State == "result_unavailable" || r.State == "unavailable_reported" || r.State == "awaiting_parent" || r.State == "awaiting_workflow_result" {
			out.UnacquiredResults++
		}
	}
	return out
}

// Account assembles what this session did.
func Account(result Result) Status {
	return Status{
		Completion: completionFacts(result.Diagnostics),
		Category:   result.Category,
		ExitCode:   result.NativeExitCode,
		Attempts:   result.Attempts,
		Inferences: result.Inferences,
		Session: SessionFacts{
			StartupModel:  startupModel.Model,
			StartupEffort: startupModel.Effort,
			NativeContextDefaults: NativeContextDefaults{Window: contextWindow,
				AutoCompactWindow: compactAt, CompactPercent: compactPercent(), ApplicationVerified: false},
			NonStreamingFallback:  sessionRequirements()["CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK"] == "1",
			DelegationMenuEntries: len(agentDefinitions()),
			HookInstalled:         result.HookInstalled,
		},
		Gateway:       result.Diagnostics,
		Lifecycle:     result.Lifecycle,
		CleanupFailed: result.CleanupErr != nil,
	}
}

// noteworthy reports whether this session has anything a reader would want the detail of.
//
// Judged betas are deliberately not in this list. Measured 2026-09-17: the installed client
// sends structured-outputs-2025-12-15 on every request, so including them made every single
// session noteworthy and the whole account was dumped at every exit. A signal that fires on
// ordinary use is not a signal. Judged is the name for something this project already looked
// at and decided; Unknown is the name for something nobody has classified, and that is the
// one that still deserves a reader's attention.
func (s Status) noteworthy() bool {
	return s.Category != CategorySuccess || s.CleanupFailed ||
		(s.Lifecycle != nil && s.Lifecycle.CheckpointFailures > 0) ||
		s.Gateway.WorkflowPersistence.Failed > 0 ||
		!s.Session.HookInstalled ||
		s.Gateway.Requests.Refused > s.Gateway.Requests.RefusedBy["COUNT_TOKENS_UNSUPPORTED"]+s.Completion.ControlTransitions || s.Gateway.BrokenStreams() > 0 ||
		s.Completion.NativeToolFailures > 0 || s.Completion.RejectedWorkflowCalls > 0 || s.Completion.UnacquiredResults > 0 || s.Gateway.NativeToolFailures.CapacityExceeded ||
		s.Gateway.Events.Unsupported > 0 ||
		s.Gateway.Agents.Unregistered > 0 || s.Gateway.Agents.Unrouted > 0 || s.Gateway.Agents.Evicted > 0 ||
		s.Gateway.Betas.Malformed > 0 ||
		len(s.Gateway.Betas.Unknown) > 0
}

// Report writes the account to a file and says what needs saying on stderr.
//
// Never fatal. A session that ran fine and could not write a diagnostic still ran fine, and
// turning that into an error would be this build breaking a working session over its own
// bookkeeping. When the file cannot be written the printed line becomes the only copy, and
// it is printed for that reason rather than withheld.
func Report(result Result, errOut io.Writer, env map[string]string) {
	if errOut == nil {
		return
	}
	account := Account(result)
	encoded, err := json.Marshal(account)
	if err != nil {
		fmt.Fprintln(errOut, "clauduct: CLAUDUCT_REQUEST_STATUS_UNAVAILABLE")
		return
	}

	path, writeErr := writeStatus(encoded)
	where := path
	if writeErr != nil {
		where = "none"
	}
	fmt.Fprintf(errOut, "clauduct: process=%s exit=%d requests=%d refused=%d%s attempts=%d inferences=%d%s status=%s\n",
		account.Category, account.ExitCode,
		account.Gateway.Requests.Received, account.Gateway.Requests.Refused,
		brokenField(account), account.Attempts, account.Inferences, quotaField(account), where)
	if account.Completion.APIFailures > 0 || account.Completion.CancelledRequests > 0 || account.Completion.NativeCancellations > 0 || account.Completion.NativeToolFailures > 0 || account.Completion.RejectedWorkflowCalls > 0 || account.Completion.UnacquiredResults > 0 || account.Completion.ControlTransitions > 0 {
		fmt.Fprintf(errOut, "clauduct: api_failures=%d cancelled_requests=%d native_cancellations=%d native_tool_failures=%d rejected_workflow_calls=%d results_unacquired_recent=%d compaction_controls=%d acceptance=not_assessed\n", account.Completion.APIFailures, account.Completion.CancelledRequests, account.Completion.NativeCancellations, account.Completion.NativeToolFailures, account.Completion.RejectedWorkflowCalls, account.Completion.UnacquiredResults, account.Completion.ControlTransitions)
	}
	if life := account.Lifecycle; life != nil && (life.Reason == "session_deadline" || life.CheckpointFailures > 0) {
		fmt.Fprintf(errOut, "clauduct: lifecycle=%s reason=%s grace_expired=%t native_reaped=%t checkpoint_failures=%d\n", life.State, life.Reason, life.GraceExpired, life.NativeReaped, life.CheckpointFailures)
	}
	if count := account.Gateway.Requests.RefusedBy["COUNT_TOKENS_UNSUPPORTED"]; count > 0 {
		fmt.Fprintf(errOut, "clauduct: count_tokens unsupported=%d; native estimation fallback remains available\n", count)
	}

	if account.noteworthy() || writeErr != nil || env[statusEnv] == "1" {
		fmt.Fprintf(errOut, "CLAUDUCT_REQUEST_STATUS %s\n", encoded)
	}
}

// writeStatus puts the account somewhere it can be read after the fact.
//
// The temporary directory, not the working directory and not the user's configuration. A
// diagnostic is disposable and writing one into a project would leave this build's litter in
// somebody's repository; writing one into ~/.claude would be touching state that is theirs.
//
// One file per process. Files from finished sessions accumulate until the operating system
// clears its temporary directory, which is what that directory is for; a sweep of our own
// would be this build deleting files by pattern, which is a worse trade.
// StatusDir is where session accounts are written and read. One place knows the location,
// and the uninstall path needs to name it without owning it.
func StatusDir() string { return filepath.Join(os.TempDir(), "clauduct") }

// WriteCheckpoint uses the same bounded metadata schema and atomic file as the
// final report. It never writes to the terminal while native owns the TUI.
func WriteCheckpoint(account Status) error {
	encoded, err := json.Marshal(account)
	if err != nil {
		return err
	}
	_, err = writeStatus(encoded)
	return err
}

func writeStatus(encoded []byte) (string, error) {
	if len(encoded) > 8<<20 {
		return "", fmt.Errorf("STATUS_TOO_LARGE")
	}
	dir := StatusDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("status-%d.json", os.Getpid()))
	file, err := os.CreateTemp(dir, fmt.Sprintf("status-%d-*.tmp", os.Getpid()))
	if err != nil {
		return "", err
	}
	temp := file.Name()
	defer os.Remove(temp)
	_, writeErr := file.Write(append(encoded, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	for _, err := range []error{writeErr, syncErr, closeErr} {
		if err != nil {
			return "", err
		}
	}
	if err := os.Rename(temp, path); err != nil {
		return "", err
	}
	return path, nil
}

// endedAs names how the session ended.
//
// Ordered by which answer is most use to a reader. A cancelled session usually also shows a
// non-zero exit, and "you stopped it" is the better of the two answers; an exhausted budget
// usually also shows a failed client, and the budget is the cause of that.
func endedAs(ctx interface{ Err() error }, result Result, ledger *upstream.Ledger) string {
	if ctx != nil && ctx.Err() != nil {
		return CategoryCancelled
	}
	if ledger != nil {
		if _, _, refused := ledger.Spent(); refused > 0 {
			return CategoryBudget
		}
	}
	if result.NativeExitCode != 0 {
		return CategoryClientFail
	}
	return CategorySuccess
}

// quotaField is the one number from the backend's accounting that belongs on every exit
// line, or nothing when this session never heard it.
//
// It is here because there is nowhere else a user would see it. The client's own /usage
// cannot: measured against a controlled listener, it skips the account request entirely
// against a gateway base URL, under either credential shape. Meanwhile the backend states
// the figure on every response and each session already writes it down.
//
// One field, not the whole report -- the window that is in force, how much of it is gone.
// `clauduct-dev usage` is where the rest lives, including how old a reading is.
// brokenField names streams that broke after their status was sent, and only when there are
// any.
//
// The one failure the counters beside it cannot report: the client received a 200, so refused
// stays at zero and received counts it as a request that happened. Measured 2026-09-18, a
// session with one of these printed "refused=0" and then its whole account, leaving a reader
// holding the JSON with no word on why -- the line reported the counter that was fine and
// omitted the one that was not.
//
// Conditional, which is the rule quotaField follows below: a line carrying broken=0 on every
// clean session is back to reporting the number that needs no reporting.
func brokenField(account Status) string {
	if broken := account.Gateway.BrokenStreams(); broken > 0 {
		return fmt.Sprintf(" broken=%d", broken)
	}
	return ""
}

func quotaField(account Status) string {
	if !hasFigure(account) {
		return ""
	}
	limits := account.Gateway.Limits
	field := fmt.Sprintf(" quota=%g%%", *limits.Primary.UsedPercent)
	if minutes := limits.Primary.WindowMinutes; minutes != nil && *minutes > 0 {
		// Hours or days, whichever divides: a weekly window reads as 7d, not 10080m.
		switch {
		case *minutes%(60*24) == 0:
			field += fmt.Sprintf("/%dd", *minutes/(60*24))
		case *minutes%60 == 0:
			field += fmt.Sprintf("/%dh", *minutes/60)
		default:
			field += fmt.Sprintf("/%dm", *minutes)
		}
	}
	return field
}
