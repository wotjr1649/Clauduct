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
	"errors"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
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
	"sync/atomic"
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
	if alone(ConfirmationArg) {
		return confirmation(os.Stdin, os.Stdout, platform.Environment(os.Environ())), true
	}
	if len(args) == 2 && args[0] == ConfirmationArg {
		return linkedConfirmation(args[1], os.Stdin, os.Stdout, platform.Environment(os.Environ())), true
	}
	if len(args) > 0 && args[0] == ConfirmationArg {
		return 2, true
	}
	if len(args) == 2 && args[0] == Arg && strings.HasPrefix(args[1], "http://") {
		// Foreground: the gateway is on the command line and the token in the environment.
		env := platform.Environment(os.Environ())
		return routed(sessionlink.Connection{BaseURL: args[1], Token: env["ANTHROPIC_AUTH_TOKEN"]}, os.Stdin, os.Stdout, os.Stderr, env), true
	}
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
		return routed(connection, os.Stdin, os.Stdout, os.Stderr, platform.Environment(os.Environ())), true
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
		return runWithOutput(os.Stdin, os.Stdout, os.Stderr, platform.Environment(os.Environ())), true
	case alone(pdf.RenderArg):
		return renderPDF(), true
	case alone(Arg):
		return runWithOutput(os.Stdin, os.Stdout, os.Stderr, platform.Environment(os.Environ())), true
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

// RoutingMismatchPath is where a hook reports that native is not using its gateway.
const RoutingMismatchPath = "/clauduct/routing-mismatch"

// routed runs a hook named with its session's gateway (#295): the URL on a foreground hook's
// command line, or the background session link. A hook inherits the environment native
// applied its settings to, so it sees where native's requests go. When that is not the
// gateway -- another endpoint, a cloud provider, a unix socket -- model requests are leaving
// Clauduct: say so, tell the gateway so the launcher ends the session, and fail the event,
// which blocks a prompt. Nothing is posted to the other destination and its value is not
// printed.
func routed(connection sessionlink.Connection, in io.Reader, out, errOut io.Writer, env map[string]string) int {
	if routedElsewhere(env, connection.BaseURL) {
		fmt.Fprintln(errOut, "CLAUDUCT_ROUTING_MISMATCH: native is not sending model requests to this session's Clauduct gateway; the session is being ended.")
		postReply([]byte("{}"), map[string]string{"ANTHROPIC_BASE_URL": connection.BaseURL, "ANTHROPIC_AUTH_TOKEN": connection.Token}, RoutingMismatchPath, false)
		return 2
	}
	env["ANTHROPIC_AUTH_TOKEN"] = connection.Token
	return runWithOutput(in, out, errOut, env)
}

// routedElsewhere reads names as Windows does, without case: a settings file may spell one
// in lower case.
func routedElsewhere(env map[string]string, gateway string) bool {
	upper := make(map[string]string, len(env))
	for name, value := range env {
		upper[strings.ToUpper(name)] = value
	}
	if upper["ANTHROPIC_BASE_URL"] != gateway || upper["ANTHROPIC_UNIX_SOCKET"] != "" {
		return true
	}
	for _, name := range launch.ProviderSwitches {
		if value := strings.ToLower(upper[name]); value != "" && value != "0" && value != "false" {
			return true
		}
	}
	return false
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
		if _, err := postReply(body, env, "/clauduct/tool-failures", false); err != nil {
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
		if _, err := postReply(body, env, "/clauduct/workflows", false); err != nil {
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
		Source     string `json:"source"`
		Tool       string `json:"tool_name"`
	}
	decoded := json.Unmarshal(raw, &compact) == nil
	worktreeMove := decoded && compact.Event == "PostToolUse" && (compact.Tool == "EnterWorktree" || compact.Tool == "ExitWorktree")
	if decoded && (compact.Event == "PreCompact" || compact.Event == "PostCompact" || compact.Event == "SessionStart" || compact.Event == "UserPromptSubmit" || worktreeMove) {
		if !identifier.MatchString(compact.Session) || (compact.Agent != "" && !identifier.MatchString(compact.Agent)) {
			fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_INVALID")
			return 2
		}
		if worktreeMove && compact.Agent != "" {
			fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_INVALID")
			return 2
		}
		fields := map[string]string{"event": compact.Event, "sessionId": compact.Session, "agentId": compact.Agent}
		if compact.Event == "SessionStart" && compact.Source != "" {
			fields["source"] = compact.Source
		}
		if compact.Trigger != "" {
			if compact.Trigger != "auto" && compact.Trigger != "manual" {
				return 2
			}
			fields["trigger"] = compact.Trigger
		}
		if compact.Event == "SessionStart" || compact.Event == "UserPromptSubmit" || worktreeMove {
			if compact.Transcript == "" || len(compact.Transcript) > 4096 {
				fmt.Fprintln(errOut, "CLAUDUCT_CONTEXT_EVENT_INVALID")
				return 2
			}
			// SessionStart cannot block native startup. Reconfirm the same
			// idempotent registration before each prompt, without sending its text.
			// Worktree tools move the transcript without a new prompt. The gateway
			// verifies the actual move before reusing its context and profile.
			fields["event"] = "SessionStart"
			fields["transcriptPath"] = compact.Transcript
		}
		body, _ := json.Marshal(fields)
		reply, err := postReply(body, env, "/clauduct/context", compact.Event == "SessionStart")
		if err != nil {
			if errors.Is(err, errSessionRestart) {
				fmt.Fprintf(errOut, "SESSION_RESTART_REQUIRED: save any draft, finish native, then run clauduct --resume %s with any native options you need.\n", compact.Session)
				return 2
			}
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
	_, err := postReply(body, env, "/clauduct/agents", false)
	return err
}

func postReply(body []byte, env map[string]string, path string, retryRegistration bool) ([]byte, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var dialed atomic.Bool
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
				// A registration retry may reuse this connection, never a new
				// listener that acquired the same loopback port after it closed.
				if retryRegistration && dialed.Swap(true) {
					return nil, errInvalidGateway
				}
				return (&net.Dialer{Timeout: requestTimeout}).DialContext(ctx, network, address)
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errInvalidGateway },
	}
	defer client.CloseIdleConnections()
	for attempt := 0; attempt < 2; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Content-Length", strconv.Itoa(len(body)))
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
		response.Body.Close()
		if err != nil || len(reply) > 4096 {
			return nil, errInvalidGateway
		}
		// Only the original SessionStart can carry the first profile's source.
		// Retry its metadata once on temporary unavailability, within the SAME
		// three-second budget. Explicit refusals and all other events stay final.
		if retryRegistration && path == "/clauduct/context" && attempt == 0 && response.StatusCode == http.StatusServiceUnavailable && len(reply) == 0 {
			continue
		}
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
			if path == "/clauduct/context" && response.StatusCode == http.StatusBadRequest {
				var envelope struct {
					Type  string `json:"type"`
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal(reply, &envelope) == nil && envelope.Type == "error" &&
					(envelope.Error.Message == errSessionRestart.Error() || strings.HasPrefix(envelope.Error.Message, errSessionRestart.Error()+";")) {
					return nil, errSessionRestart // Never print arbitrary gateway response text.
				}
			}
			return nil, fmt.Errorf("REGISTRATION_FAILED %d", response.StatusCode)
		}
		return reply, nil
	}
	return nil, errInvalidGateway
}

var errInvalidGateway = fmt.Errorf("INVALID_GATEWAY")
var errSessionRestart = errors.New("SESSION_RESTART_REQUIRED")
