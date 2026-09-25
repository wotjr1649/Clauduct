// Package hookcmd reports a subagent's start and stop to the session's gateway.
//
// The client runs it. A SubagentStart or SubagentStop hook hands it the event on stdin, it
// turns that into a binding, and it posts the binding to the loopback gateway this session
// started. That is the only way the gateway can know a subagent exists, because the client
// is what starts one.
//
// It runs inside clauduct.exe, reached by a reserved first argument no native option can
// be, or by the executable's name (#112). Until v0.4.0 it was a separate binary,
// clauduct-hook, because the launcher forwards every other argument to the native client.
//
// Start reports contain identity and context. Stop reports additionally deliver
// the existing final answer to this session's authenticated loopback gateway.
// Neither the hook nor diagnostics persist report bodies or arbitrary hook fields.
package hookcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Named reports whether argv0 is the program called name, in any case and with or without
// a directory or .exe: a process started from cmd.exe gets its name as it was typed.
func Named(argv0, name string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.ToLower(filepath.Base(argv0)), ".exe"), name)
}

// Arg is the whole argument list the session settings give the hook command.
const Arg = "--clauduct-hook"

// Dispatch runs the role argv asks for, if it asks for one of these, and reports whether
// it did. It has to come before anything else the launcher does: the renderer runs with an
// empty environment and the hook inside the client's timeout, and neither is a launch.
func Dispatch(argv []string) (int, bool) {
	if len(argv) == 0 {
		return 0, false
	}
	args := argv[1:]
	alone := func(arg string) bool { return len(args) == 1 && args[0] == arg }
	if len(args) == 2 && (args[0] == Arg || args[0] == "--clauduct-background-key") {
		connection, err := sessionlink.Read(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "BACKGROUND_CONNECTION_UNAVAILABLE: restart this session with Clauduct")
			return 2, true
		}
		if args[0] == "--clauduct-background-key" {
			// apiKeyHelper consumes this anonymous output pipe; never a file or argv.
			info, err := os.Stdout.Stat()
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				return 2, true
			}
			if _, err = fmt.Fprintln(os.Stdout, connection.Token); err != nil {
				return 2, true
			}
			return 0, true
		}
		env := environ()
		env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_AUTH_TOKEN"] = connection.BaseURL, connection.Token
		return runWithOutput(os.Stdin, os.Stdout, os.Stderr, env), true
	}
	switch {
	case Named(argv[0], "pdftoppm"):
		return nativePDF(args, os.Stdout, os.Stderr), true
	case Named(argv[0], "clauduct-hook"):
		// The copy a 0.3.x updater installs, which a session started before the update still
		// names, with that version's renderer argument.
		if alone("--render-pdf") {
			return renderPDF(), true
		}
		return runWithOutput(os.Stdin, os.Stdout, os.Stderr, environ()), true
	case alone(pdf.RenderArg):
		return renderPDF(), true
	case alone(Arg):
		return runWithOutput(os.Stdin, os.Stdout, os.Stderr, environ()), true
	}
	return 0, false
}

// Used only by the gateway's sealed, deadline-bound renderer subprocess.
// PDF bytes and page images stay in pipes/memory; diagnostics contain no data.
func renderPDF() int {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, pdf.MaxBytes+1))
	if err != nil || len(data) > pdf.MaxBytes {
		fmt.Fprintln(os.Stderr, "PDF_INPUT_LIMIT")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pages, err := pdf.Render(ctx, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "PDF_RENDER_FAILED")
		return 1
	}
	if json.NewEncoder(os.Stdout).Encode(pages) != nil {
		return 1
	}
	return 0
}

// The bounds. A hook event is a small object; anything past these is not one.
const (
	maxEventBytes   = (256<<10)*6 + 16384
	maxBindingBytes = (256<<10)*6 + 8192
	requestTimeout  = 3 * time.Second
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// loopback is the only address this will send a credential to.
//
// The session token is in the environment and the destination comes from the environment
// too, so without this the two together are a way to post the token wherever a variable
// says. It is pinned to the loopback host, which is where the gateway is and the only place
// it can be.
var loopback = regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]{1,5}$`)

func run(in io.Reader, errOut io.Writer, env map[string]string) int {
	return runWithOutput(in, io.Discard, errOut, env)
}

func runWithOutput(in io.Reader, out, errOut io.Writer, env map[string]string) int {
	raw, err := io.ReadAll(io.LimitReader(in, maxEventBytes+1))
	if err != nil || len(raw) > maxEventBytes {
		fmt.Fprintln(errOut, "CLAUDUCT_AGENT_ROUTE_FAILED")
		return 1
	}
	var pre struct {
		Event string                  `json:"hook_event_name"`
		Tool  string                  `json:"tool_name"`
		Input struct{ Script string } `json:"tool_input"`
	}
	if json.Unmarshal(raw, &pre) == nil && pre.Event == "PreToolUse" && pre.Tool == "Workflow" && pre.Input.Script == bridge.RejectedWorkflowScript {
		if json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": bridge.RejectedWorkflowReason,
		}}) != nil {
			return 2
		}
		return 0
	}
	var failure struct {
		Event       string `json:"hook_event_name"`
		Session     string `json:"session_id"`
		Agent       string `json:"agent_id"`
		Call        string `json:"tool_use_id"`
		Tool        string `json:"tool_name"`
		Interrupted bool   `json:"is_interrupt"`
	}
	if json.Unmarshal(raw, &failure) == nil && failure.Event == "PostToolUseFailure" {
		if !identifier.MatchString(failure.Session) || !identifier.MatchString(failure.Call) || (failure.Agent != "" && !identifier.MatchString(failure.Agent)) {
			fmt.Fprintln(errOut, "CLAUDUCT_TOOL_EVENT_INVALID")
			return 1
		}
		tool := "other"
		for _, name := range []string{"Agent", "Workflow", "Read", "Write", "Edit", "Bash", "Grep", "Glob", "WebSearch", "WebFetch", "ToolSearch", "SendMessage", "TaskOutput", "TaskStop"} {
			if failure.Tool == name {
				tool = name
			}
		}
		if strings.HasPrefix(failure.Tool, "mcp__") {
			tool = "MCP"
		}
		body, _ := json.Marshal(map[string]any{"session": failure.Session, "agent": failure.Agent, "call": failure.Call, "tool": tool, "interrupted": failure.Interrupted})
		if _, err := postReply(body, env, "/clauduct/tool-failures"); err != nil {
			fmt.Fprintln(errOut, "CLAUDUCT_TOOL_EVENT_FAILED")
			return 1
		}
		return 0
	}
	var workflow struct {
		Event      string                                                                            `json:"hook_event_name"`
		Tool       string                                                                            `json:"tool_name"`
		Session    string                                                                            `json:"session_id"`
		Parent     string                                                                            `json:"agent_id"`
		Call       string                                                                            `json:"tool_use_id"`
		Transcript string                                                                            `json:"transcript_path"`
		Result     struct{ Status, TaskType, RunID, WorkflowName, TranscriptDir, ScriptPath string } `json:"tool_response"`
	}
	if json.Unmarshal(raw, &workflow) == nil && workflow.Event == "PostToolUse" && workflow.Tool == "Workflow" {
		if workflow.Result.Status != "async_launched" || workflow.Result.TaskType != "local_workflow" {
			fmt.Fprintln(errOut, "CLAUDUCT_WORKFLOW_UNVERIFIED")
			return 1
		}
		body, _ := json.Marshal(map[string]string{"sessionId": workflow.Session, "parent": workflow.Parent, "toolUseId": workflow.Call, "transcriptPath": workflow.Transcript, "runId": workflow.Result.RunID, "workflowName": workflow.Result.WorkflowName, "transcriptDir": workflow.Result.TranscriptDir, "scriptPath": workflow.Result.ScriptPath})
		if _, err := postReply(body, env, "/clauduct/workflows"); err != nil {
			fmt.Fprintln(errOut, "CLAUDUCT_WORKFLOW_UNVERIFIED")
			return 1
		}
		return 0
	}
	var compact struct {
		Event      string `json:"hook_event_name"`
		Session    string `json:"session_id"`
		Agent      string `json:"agent_id"`
		Transcript string `json:"transcript_path"`
		Trigger    string `json:"trigger"`
	}
	if json.Unmarshal(raw, &compact) == nil && (compact.Event == "PreCompact" || compact.Event == "PostCompact" || compact.Event == "SessionStart" || compact.Event == "UserPromptSubmit") {
		if !identifier.MatchString(compact.Session) || (compact.Agent != "" && !identifier.MatchString(compact.Agent)) {
			fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_INVALID")
			return 2
		}
		fields := map[string]string{"event": compact.Event, "sessionId": compact.Session, "agentId": compact.Agent}
		if compact.Trigger != "" {
			if compact.Trigger != "auto" && compact.Trigger != "manual" {
				return 2
			}
			fields["trigger"] = compact.Trigger
		}
		if compact.Event == "SessionStart" || compact.Event == "UserPromptSubmit" {
			if compact.Transcript == "" || len(compact.Transcript) > 4096 {
				fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_INVALID")
				return 2
			}
			// SessionStart cannot block native startup. Reconfirm the same
			// idempotent registration before each prompt, without sending its text.
			fields["event"] = "SessionStart"
			fields["transcriptPath"] = compact.Transcript
		}
		body, _ := json.Marshal(fields)
		reply, err := postReply(body, env, "/clauduct/context")
		if err != nil {
			if compact.Event == "UserPromptSubmit" {
				fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_SESSION_UNVERIFIED: session registration failed; restore the Clauduct hook connection and submit the prompt again.")
				return 2
			}
			fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_FAILED")
			return 2
		}
		if compact.Event == "PreCompact" && len(reply) > 0 {
			var receipt struct {
				Ticket string `json:"ticket"`
			}
			if json.Unmarshal(reply, &receipt) != nil || len(receipt.Ticket) != 43 || !identifier.MatchString(receipt.Ticket) {
				fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_RECEIPT_INVALID")
				return 2
			}
			fmt.Fprintf(out, "[clauduct-compact:%s]", receipt.Ticket)
		}
		return 0
	}

	binding, ok := bindingFrom(raw, env)
	if !ok {
		// Not an event this hook reports on. Nothing to do and nothing wrong: the client
		// may fire hooks this build does not act on.
		return 0
	}

	body, err := json.Marshal(binding)
	if err != nil || len(body) > maxBindingBytes {
		fmt.Fprintln(errOut, "CLAUDUCT_AGENT_ROUTE_FAILED")
		return 1
	}
	if err := post(body, env); err != nil {
		fmt.Fprintln(errOut, "CLAUDUCT_AGENT_ROUTE_FAILED", err)
		return 1
	}
	return 0
}

// binding is what the gateway takes.
type binding struct {
	ID             string         `json:"id"`
	Role           string         `json:"role"`
	Stop           bool           `json:"stop"`
	SessionID      string         `json:"sessionId,omitempty"`
	TranscriptPath string         `json:"transcriptPath,omitempty"`
	Context        *contextPolicy `json:"contextPolicy,omitempty"`
	Result         string         `json:"result,omitempty"`
}

type contextPolicy struct {
	Window            int64   `json:"window"`
	AutoCompactWindow int64   `json:"autoCompactWindow"`
	CompactPercent    float64 `json:"compactPercent"`
}

// bindingFrom reads the event and reports the binding it describes, if it describes one.
//
// Only the fields that go on are read. The event carries more -- the client's own record of
// what the subagent is doing -- and leaving it unread is how none of it travels.
func bindingFrom(raw []byte, env map[string]string) (binding, bool) {
	var event struct {
		HookEventName  string `json:"hook_event_name"`
		AgentID        string `json:"agent_id"`
		AgentType      string `json:"agent_type"`
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
		Result         string `json:"last_assistant_message"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return binding{}, false
	}
	stop := event.HookEventName == "SubagentStop"
	if event.HookEventName != "SubagentStart" && !stop {
		return binding{}, false
	}
	if !identifier.MatchString(event.AgentID) ||
		event.AgentType == "" || len(event.AgentType) > 200 {
		return binding{}, false
	}

	out := binding{ID: event.AgentID, Role: event.AgentType, Stop: stop}
	if identifier.MatchString(event.SessionID) && event.TranscriptPath != "" &&
		len(event.TranscriptPath) <= 4096 {
		out.SessionID = event.SessionID
		out.TranscriptPath = event.TranscriptPath
	}
	if stop {
		if len(event.Result) <= 256<<10 {
			out.Result = event.Result
		}
		return out, true
	}
	out.Context = contextFrom(env)
	return out, true
}

// contextFrom reads the window the client is working in, when it stated all of it.
//
// All three or none. Two of the three describe a window with a piece missing, and a
// registration carrying half a policy is worse than one carrying none: it looks answered.
func contextFrom(env map[string]string) *contextPolicy {
	window, windowOK := positiveInt(env["CLAUDE_CODE_MAX_CONTEXT_TOKENS"])
	auto, autoOK := positiveInt(env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"])
	percent, percentErr := strconv.ParseFloat(env["CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"], 64)
	if !windowOK || !autoOK || percentErr != nil || percent <= 0 || percent > 100 {
		return nil
	}
	return &contextPolicy{Window: window, AutoCompactWindow: auto, CompactPercent: percent}
}

func positiveInt(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}

// post sends the binding to this session's gateway and nowhere else.
func post(body []byte, env map[string]string) error {
	_, err := postReply(body, env, "/clauduct/agents")
	return err
}

func postReply(body []byte, env map[string]string, path string) ([]byte, error) {
	base := env["ANTHROPIC_BASE_URL"]
	token := env["ANTHROPIC_AUTH_TOKEN"]
	if !loopback.MatchString(base) || token == "" {
		return nil, errInvalidGateway
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, errInvalidGateway
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errInvalidGateway
	}

	request, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Length", strconv.Itoa(len(body)))

	client := &http.Client{
		Timeout: requestTimeout,
		// A connection to anywhere but the loopback address is refused at the dial, so a
		// name that resolves elsewhere cannot carry the token out even if the address
		// check above were somehow satisfied.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(address)
				if err != nil || host != "127.0.0.1" {
					return nil, errInvalidGateway
				}
				return (&net.Dialer{Timeout: requestTimeout}).DialContext(ctx, network, address)
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errInvalidGateway },
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(reply) > 4096 {
		return nil, errInvalidGateway
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("REGISTRATION_FAILED %d", response.StatusCode)
	}
	return reply, nil
}

var errInvalidGateway = fmt.Errorf("INVALID_GATEWAY")

func environ() map[string]string {
	out := map[string]string{}
	for _, entry := range os.Environ() {
		if i := indexByte(entry, '='); i > 0 {
			out[entry[:i]] = entry[i+1:]
		}
	}
	return out
}

func indexByte(value string, b byte) int { return strings.IndexByte(value, b) }
