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
)

// SessionFacts are what this build told the child about itself.
//
// Kept beside the gateway's account because half of a session's behaviour is decided here
// and is invisible from there: where it compacts, what it starts on, and whether a
// non-streaming retry was allowed. A reader asking why a session compacted at a particular
// point has no other place to look.
type SessionFacts struct {
	Model                 string  `json:"model"`
	Effort                string  `json:"effort"`
	ContextWindow         int     `json:"contextWindow"`
	AutoCompactWindow     int     `json:"autoCompactWindow"`
	CompactPercent        float64 `json:"compactPercent"`
	NonStreamingFallback  bool    `json:"nonStreamingFallbackDisabled"`
	DelegationMenuEntries int     `json:"delegationMenuEntries"`
	// HookInstalled is whether the subagent hook was found beside this executable.
	//
	// Reported because its absence is silent otherwise. findHook looks beside the binary
	// and nowhere else, so a package that shipped without clauduct-hook runs perfectly --
	// and role routing never happens, and the delegation menu moves the model without the
	// effort. That is a failure nobody would think to look for.
	HookInstalled bool `json:"hookInstalled"`
}

// Status is the whole account of one session.
type Status struct {
	Category   string              `json:"category"`
	ExitCode   int                 `json:"exitCode"`
	Attempts   int                 `json:"attempts"`
	Inferences int                 `json:"inferences"`
	Session    SessionFacts        `json:"session"`
	Gateway    gateway.Diagnostics `json:"gateway"`
}

// Account assembles what this session did.
func Account(result Result) Status {
	return Status{
		Category:   result.Category,
		ExitCode:   result.NativeExitCode,
		Attempts:   result.Attempts,
		Inferences: result.Inferences,
		Session: SessionFacts{
			Model:                 startupModel.Model,
			Effort:                startupModel.Effort,
			ContextWindow:         contextWindow,
			AutoCompactWindow:     compactAt,
			CompactPercent:        compactPercent(),
			NonStreamingFallback:  sessionRequirements()["CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK"] == "1",
			DelegationMenuEntries: len(agentDefinitions()),
			HookInstalled:         result.HookInstalled,
		},
		Gateway: result.Diagnostics,
	}
}

// noteworthy reports whether this session has anything a reader would want the detail of.
func (s Status) noteworthy() bool {
	return s.Category != CategorySuccess ||
		!s.Session.HookInstalled ||
		s.Gateway.Requests.Refused > 0 || s.Gateway.BrokenStreams() > 0 ||
		s.Gateway.Events.Unsupported > 0 ||
		s.Gateway.Agents.Unregistered > 0 || s.Gateway.Agents.Unrouted > 0 ||
		s.Gateway.Betas.Malformed > 0 ||
		len(s.Gateway.Betas.Judged) > 0 || len(s.Gateway.Betas.Unknown) > 0
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
	fmt.Fprintf(errOut, "clauduct: %s exit=%d requests=%d refused=%d attempts=%d inferences=%d%s status=%s\n",
		account.Category, account.ExitCode,
		account.Gateway.Requests.Received, account.Gateway.Requests.Refused,
		account.Attempts, account.Inferences, quotaField(account), where)

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
// statusDir is where session accounts are written and read. One place knows the location.
func statusDir() string { return filepath.Join(os.TempDir(), "clauduct") }

func writeStatus(encoded []byte) (string, error) {
	dir := statusDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("status-%d.json", os.Getpid()))
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
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
