package app

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// reported runs Report into its own buffer and returns what was written and what was filed.
func reported(t *testing.T, result Result, env map[string]string) (string, Status) {
	t.Helper()
	var errOut bytes.Buffer
	Report(result, &errOut, env)

	text := errOut.String()
	path := ""
	for _, field := range strings.Fields(strings.SplitN(text, "\n", 2)[0]) {
		if rest, found := strings.CutPrefix(field, "status="); found {
			path = rest
		}
	}
	if path == "" {
		t.Fatalf("the line did not say where the account went: %q", text)
	}
	var filed Status
	if path != "none" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := json.Unmarshal(raw, &filed); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return text, filed
}

// D2. A clean session says one line, and the detail is on disk rather than in the way.
func TestACleanSessionSaysOneLine(t *testing.T) {
	text, filed := reported(t, Result{
		Category: CategorySuccess, Attempts: 5, Inferences: 4, HookInstalled: true,
		Diagnostics: gateway.Diagnostics{
			Requests: gateway.RequestCounts{Received: 12},
		},
	}, nil)

	if lines := strings.Count(strings.TrimSpace(text), "\n"); lines != 0 {
		t.Fatalf("a session with nothing to report said %d extra lines:\n%s", lines, text)
	}
	if !strings.Contains(text, "SUCCESS") || !strings.Contains(text, "requests=12") ||
		!strings.Contains(text, "inferences=4") {
		t.Errorf("line = %q", text)
	}
	if strings.Contains(text, "CLAUDUCT_REQUEST_STATUS ") {
		t.Errorf("the whole account was printed for a session with nothing to say:\n%s", text)
	}
	// It is on disk all the same, so nothing is lost by not printing it.
	if filed.Category != CategorySuccess || filed.Gateway.Requests.Received != 12 {
		t.Fatalf("the file holds %+v", filed)
	}
	// And the half of a session's behaviour that the gateway cannot see is in it.
	if filed.Session.ContextWindow != contextWindow || filed.Session.Model != startupModel.Model {
		t.Errorf("session facts = %+v", filed.Session)
	}
	if !filed.Session.NonStreamingFallback {
		t.Error("the one setting this build cannot run without is not recorded")
	}
	if !filed.Session.HookInstalled {
		t.Error("the hook was installed and the account says it was not")
	}
}

// A session with something to say says it, without being asked.
func TestASessionWithSomethingToSaySaysIt(t *testing.T) {
	for name, result := range map[string]Result{
		"a request was refused": {Category: CategorySuccess, HookInstalled: true, Diagnostics: gateway.Diagnostics{
			Requests: gateway.RequestCounts{Received: 2, Refused: 1}}},
		"an event could not be read": {Category: CategorySuccess, HookInstalled: true, Diagnostics: gateway.Diagnostics{
			Events: gateway.EventReport{Unsupported: 1, Names: []string{"response.new_thing"}}}},
		"a subagent went unrouted": {Category: CategorySuccess, HookInstalled: true, Diagnostics: gateway.Diagnostics{
			Agents: gateway.AgentCounts{Unrouted: 1}}},
		"a beta this build does not do": {Category: CategorySuccess, HookInstalled: true, Diagnostics: gateway.Diagnostics{
			Betas: gateway.BetaReport{Requests: 1, Judged: []string{"STRUCTURED_OUTPUTS"}}}},
		"the client failed": {Category: CategoryClientFail, NativeExitCode: 2, HookInstalled: true},
		// The one failure nobody would think to look for: the package shipped without
		// the hook, everything works, and role routing silently never happens.
		"the hook is not there": {Category: CategorySuccess},
	} {
		t.Run(name, func(t *testing.T) {
			text, _ := reported(t, result, nil)
			if !strings.Contains(text, "CLAUDUCT_REQUEST_STATUS {") {
				t.Fatalf("it kept quiet about it:\n%s", text)
			}
		})
	}
}

// Asking for it always gets it.
func TestAskingForTheAccountAlwaysGetsIt(t *testing.T) {
	text, _ := reported(t, Result{Category: CategorySuccess, HookInstalled: true},
		map[string]string{statusEnv: "1"})
	if !strings.Contains(text, "CLAUDUCT_REQUEST_STATUS {") {
		t.Fatalf("CLAUDUCT_STATUS=1 did not print it:\n%s", text)
	}
}

// When the file cannot be written, the printed line is the only copy, so it is printed.
func TestWhenTheFileFailsThePrintedLineIsTheOnlyCopy(t *testing.T) {
	// A temporary directory that is a file. MkdirAll under it cannot succeed.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", blocked)
	t.Setenv("TMP", blocked)
	t.Setenv("TEMP", blocked)

	var errOut bytes.Buffer
	Report(Result{Category: CategorySuccess, HookInstalled: true}, &errOut, nil)
	text := errOut.String()
	if !strings.Contains(text, "status=none") {
		t.Fatalf("it claimed to have filed the account:\n%s", text)
	}
	if !strings.Contains(text, "CLAUDUCT_REQUEST_STATUS {") {
		t.Fatalf("the account was withheld when the file was the thing that failed:\n%s", text)
	}
}

// How a session ended, in the order that is most use to a reader.
func TestHowASessionEndedIsNamedByWhatMattersMost(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	full := upstream.NewLedger(upstream.Budget{Limit: 0, Model: "gpt-5.6-luna", Effort: "low"})
	_ = full.Reserve(upstream.Attempt{Requested: "gpt-5.6-luna", Model: "gpt-5.6-luna", Effort: "low"})

	for name, tc := range map[string]struct {
		ctx    interface{ Err() error }
		result Result
		ledger *upstream.Ledger
		want   string
	}{
		"it ran and the client was happy": {
			ctx: context.Background(), result: Result{}, want: CategorySuccess,
		},
		"the client exited non-zero": {
			ctx: context.Background(), result: Result{NativeExitCode: 7}, want: CategoryClientFail,
		},
		// A cancelled session usually shows a non-zero exit too, and "you stopped it" is
		// the better of the two answers.
		"the user stopped it": {
			ctx: cancelled, result: Result{NativeExitCode: 1}, want: CategoryCancelled,
		},
		// And an exhausted budget usually shows a failed client, which it caused.
		"the budget ran out": {
			ctx: context.Background(), result: Result{NativeExitCode: 1},
			ledger: full, want: CategoryBudget,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := endedAs(tc.ctx, tc.result, tc.ledger); got != tc.want {
				t.Fatalf("category = %s, want %s", got, tc.want)
			}
		})
	}
}

// Nothing the session carried is in the account, and the account is now a file on disk.
//
// The gateway's own test covers its half. This covers the file, which is the copy that
// outlives the session and the one somebody might attach to a bug report.
func TestTheFiledAccountCarriesNothingFromTheSession(t *testing.T) {
	exe := nativeAvailable(t)
	script := &upstream.Script{
		Turns:   []upstream.ScriptTurn{{When: upstream.Conversation, SSE: textStream("main", "REPLY-SECRET")}},
		Default: textStream("side", "untitled"),
	}
	var g *gateway.Gateway
	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args:          []string{"-p", "PROMPT-SECRET", "--strict-mcp-config"},
		Env:           isolatedEnv(t),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway: func() (*gateway.Gateway, error) {
			started, err := gateway.Start(script)
			g = started
			return started, err
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = g

	// What main does, composed the way main composes it: the report goes to stderr and
	// stdout is left exactly as the client wrote it.
	before := stdout.String()
	var reportOut bytes.Buffer
	Report(result, &reportOut, nil)
	if stdout.String() != before {
		t.Fatalf("the report reached stdout: %q", stdout.String())
	}
	if strings.Contains(reportOut.String(), "PROMPT-SECRET") ||
		strings.Contains(reportOut.String(), "REPLY-SECRET") {
		t.Errorf("the report carried the session's own content:\n%s", reportOut.String())
	}

	_, filed := reported(t, result, map[string]string{statusEnv: "1"})
	encoded, _ := json.Marshal(filed)
	for _, secret := range []string{"PROMPT-SECRET", "REPLY-SECRET"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("the filed account holds %q:\n%s", secret, encoded)
		}
	}
	if filed.Gateway.Requests.Received == 0 {
		t.Fatal("the filed account is empty, so the check above proves nothing")
	}
	// And Run filled the category on the ordinary path, not only in the tests that build a
	// Result by hand.
	if filed.Category != CategorySuccess {
		t.Errorf("category = %q after a session that worked", filed.Category)
	}
}

// The report goes to stderr, and the source is where that is settled.
//
// stdout belongs to the answer `claude -p` was asked for. A change that sent the report
// there would break `> out.txt` and break `| jq` while every test above still passed, so the
// thing that must not change is checked where it is written rather than where it is used.
func TestTheReportIsNeverSentToStdout(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "cmd", "clauduct", "main.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	calls := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Report" {
			return true
		}
		calls++
		if len(call.Args) < 2 {
			t.Fatalf("Report called with %d arguments", len(call.Args))
		}
		target, ok := call.Args[1].(*ast.SelectorExpr)
		if !ok || target.Sel.Name != "Stderr" {
			t.Fatalf("the report is written to %s, not os.Stderr", render(call.Args[1]))
		}
		return true
	})
	if calls != 1 {
		t.Fatalf("Report is called %d times in main.go, want once", calls)
	}
}

func render(node ast.Node) string {
	var out strings.Builder
	_ = printer.Fprint(&out, token.NewFileSet(), node)
	return out.String()
}

// The account's answer about the hook is the real findHook, not a flag someone set.
//
// Both arms matter. Without the present arm nothing checks that a shipped hook is ever
// found; without the absent arm the report could be a constant.
func TestTheAccountSaysWhetherTheHookWasActuallyFound(t *testing.T) {
	for _, present := range []bool{true, false} {
		name := "the hook is beside the binary"
		if !present {
			name = "the hook was not shipped"
		}
		t.Run(name, func(t *testing.T) {
			if present {
				buildHook(t)
			} else if path := hookPath(t); path != "" {
				os.Remove(path)
			}

			exe := nativeAvailable(t)
			script := &upstream.Script{Default: textStream("side", "ok")}
			_, cwd := workspace(t)
			var stdout, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
			defer cancel()

			result, err := Run(ctx, Options{
				Args:          []string{"-p", "say ok", "--strict-mcp-config"},
				Env:           isolatedEnv(t),
				Cwd:           cwd,
				Stdout:        &stdout,
				Stderr:        &stderr,
				ResolveClaude: func() (string, bool, error) { return exe, true, nil },
				StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := Account(result).Session.HookInstalled; got != present {
				t.Fatalf("hookInstalled = %v, want %v", got, present)
			}
		})
	}
}
