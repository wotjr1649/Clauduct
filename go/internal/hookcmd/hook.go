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
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/pdf"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
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
	"syscall"
	"time"
	"unsafe"
)

// Named reports whether argv0 is the program called name, in any case and with or without
// a directory or .exe: a process started from cmd.exe gets its name as it was typed.
func Named(argv0, name string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.ToLower(filepath.Base(argv0)), ".exe"), name)
}

// Arg is the whole argument list the session settings give the hook command.
const Arg = "--clauduct-hook"

// ProviderControlledArg follows the session link on a foreground hook's command line when the
// launcher started native under a provider-control strategy (P3, #320). Only the launcher
// writes a hook's command line, so a settings file cannot claim the strategy.
const ProviderControlledArg = "--clauduct-provider-controlled"

// Dispatch runs the role argv asks for, if it asks for one of these, and reports whether
// it did. It has to come before anything else the launcher does: the renderer runs with an
// empty environment and the hook inside the client's timeout, and neither is a launch.
func Dispatch(argv []string) (int, bool) {
	return dispatch(argv, os.Stdin, os.Stdout, os.Stderr)
}

func dispatch(argv []string, in io.Reader, out, errOut io.Writer) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	return dispatchContext(ctx, argv, in, out, errOut)
}

func dispatchContext(ctx context.Context, argv []string, in io.Reader, out, errOut io.Writer) (int, bool) {
	if len(argv) == 0 {
		return 0, false
	}
	args := argv[1:]
	alone := func(arg string) bool { return len(args) == 1 && args[0] == arg }
	if alone(ConfirmationArg) {
		return confirmation(in, out, platform.Environment(os.Environ())), true
	}
	if len(args) == 2 && args[0] == ConfirmationArg {
		return linkedConfirmation(args[1], in, out, platform.Environment(os.Environ())), true
	}
	if len(args) > 0 && args[0] == ConfirmationArg {
		return 2, true
	}
	controlled := len(args) == 3 && args[0] == Arg && args[2] == ProviderControlledArg
	if len(args) == 2 && args[0] == Arg && strings.HasPrefix(args[1], "http://") {
		// A session started before the session link: the gateway is on the command line and
		// the token in the environment.
		env := platform.Environment(os.Environ())
		return routedContext(ctx, sessionlink.Connection{BaseURL: args[1], Token: envValue(env, "ANTHROPIC_AUTH_TOKEN")}, false, in, out, errOut, env), true
	}
	if (len(args) == 2 || controlled) && (args[0] == Arg || args[0] == "--clauduct-background-key") {
		// The session link, foreground or background: the connection never comes from the
		// environment, so there is nothing to fall back to when the link is gone.
		read := sessionlink.Read
		if args[0] == Arg {
			read = func(link string) (sessionlink.Connection, error) { return sessionlink.ReadContext(ctx, link) }
		}
		connection, err := read(args[1])
		if err != nil && ctx.Err() != nil && args[0] == Arg {
			return hookDeadline(errOut), true
		}
		if err != nil && args[0] == Arg {
			raw, readErr := readEvent(ctx, in)
			if errors.Is(readErr, errHookDeadline) {
				return hookDeadline(errOut), true
			}
			if readErr != nil || len(raw) > maxEventBytes {
				return linkGone(args[1], bytes.NewReader(raw), errOut), true
			}
			in = bytes.NewReader(raw)
			reconnectContext(ctx, args[1], raw)
			connection, err = read(args[1])
			if err != nil && ctx.Err() != nil {
				return hookDeadline(errOut), true
			}
		}
		if err != nil {
			if args[0] == Arg {
				return linkGone(args[1], in, errOut), true
			}
			fmt.Fprintln(errOut, "CLAUDUCT_CONNECTION_UNAVAILABLE: this session's Clauduct link is gone; restart the session with Clauduct")
			return 2, true
		}
		if args[0] == "--clauduct-background-key" {
			// apiKeyHelper consumes this anonymous output pipe; never a file or argv.
			info, err := os.Stdout.Stat()
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				return 2, true
			}
			if _, err = fmt.Fprintln(out, connection.Token); err != nil {
				return 2, true
			}
			return 0, true
		}
		ctx = context.WithValue(ctx, routingLinkContext{}, args[1])
		return routedContext(ctx, connection, controlled, in, out, errOut, platform.Environment(os.Environ())), true
	}
	switch {
	case Named(argv[0], "pdftoppm"):
		return nativePDF(args, in, out, errOut), true
	case Named(argv[0], "clauduct-hook"):
		// The copy a 0.3.x updater installs, which a session started before the update still
		// names, with that version's renderer argument.
		if alone("--render-pdf") {
			return renderPDF(), true
		}
		return runWithOutputContext(ctx, in, out, errOut, platform.Environment(os.Environ())), true
	case alone(pdf.RenderArg):
		return renderPDF(), true
	case alone(Arg):
		return runWithOutputContext(ctx, in, out, errOut, platform.Environment(os.Environ())), true
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
	// Only Clauduct-owned entries use this envelope, matching native's prompt default.
	NativeHookTimeout = 30 * time.Second
	// ReconnectTimeout bounds the hook caller's wait, not restoration's lifetime.
	ReconnectTimeout    = 2 * time.Second
	proofRequestTimeout = gateway.RoutingProofWait + 2*time.Second
	// Two pipe exchanges, one reconnect, proof and registration (or mismatch report).
	hookTimeout     = 2*sessionlink.RequestTimeout + ReconnectTimeout + proofRequestTimeout + requestTimeout
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
func routed(connection sessionlink.Connection, controlled bool, in io.Reader, out, errOut io.Writer, env map[string]string) int {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	return routedContext(ctx, connection, controlled, in, out, errOut, env)
}

type routingLinkContext struct{}

var routingJob = regexp.MustCompile(`^[a-f0-9]{8}$`)

func routedContext(ctx context.Context, connection sessionlink.Connection, controlled bool, in io.Reader, out, errOut io.Writer, env map[string]string) int {
	if routedElsewhere(env, connection.BaseURL, controlled) {
		// A foreground link answers for the Clauduct process this hook runs under. One that does
		// not is another session's link; ending that session would be wrong, so this hook only
		// refuses its own event.
		if connection.Foreground && !ancestor(connection.PID) {
			fmt.Fprintln(errOut, "CLAUDUCT_ROUTING_MISMATCH: this hook names another Clauduct session's link; the event is refused.")
			return 2
		}
		fmt.Fprintln(errOut, "CLAUDUCT_ROUTING_MISMATCH: native is not sending model requests to this session's Clauduct gateway; the session is being ended.")
		postReply(ctx, []byte("{}"), map[string]string{"ANTHROPIC_BASE_URL": connection.BaseURL, "ANTHROPIC_AUTH_TOKEN": connection.Token}, RoutingMismatchPath, false)
		return 2
	}
	pinGatewayEnv(env, connection)
	raw, err := readEvent(ctx, in)
	if errors.Is(err, errHookDeadline) {
		return hookDeadline(errOut)
	}
	if err != nil {
		fmt.Fprintln(errOut, "CLAUDUCT_AGENT_ROUTE_FAILED")
		return 1
	}
	// #299: a stored Claude apps gateway login sends native's requests to that gateway while
	// the environment still names this one, so the check above cannot see it. The native's
	// model-list request may still be in flight when a hook arrives; wait briefly for that
	// same native's request to reach this gateway. SessionStart checks as early as possible,
	// and UserPromptSubmit repeats the check before a prompt can leave for another gateway.
	//
	// Under a provider-control strategy (P3, #320) the routing guarantee is the strategy's
	// certification, not this hook, and native may skip the model list altogether; the proof
	// is then a diagnostic only. That takes both the launcher's ProviderControlledArg and the
	// strategy flag still "1" in native's environment: the argument cannot come from a settings
	// file, and a settings file that clears the flag gets the block back.
	var event struct {
		Name string `json:"hook_event_name"`
	}
	if json.Unmarshal(raw, &event) == nil && (event.Name == "SessionStart" || event.Name == "UserPromptSubmit") {
		providerControlled := controlled && (hostManaged(env) || strategyFlagOn(env, gatewayCompatFlag))
		proof := []byte(fmt.Sprintf(`{"controlled":%t}`, providerControlled))
		if _, err := postReply(ctx, proof, env, RoutingProofPath, false); err != nil {
			if errors.Is(err, errHookDeadline) {
				return hookDeadline(errOut)
			}
			if errors.Is(err, errRoutingProofGatewayClosed) {
				fmt.Fprintln(errOut, "GATEWAY_CLOSED: the Clauduct gateway is shutting down; restart the session with Clauduct before submitting a prompt.")
				return 2
			}
			if errors.Is(err, errRoutingProofPriorGeneration) {
				job := "<job>"
				if link, ok := ctx.Value(routingLinkContext{}).(string); ok && Regenerable != nil {
					if id, known := Regenerable(link); known && routingJob.MatchString(id) {
						job = id
					}
				}
				fmt.Fprintf(errOut, "ROUTING_PROOF_PRIOR_GENERATION: this native's routing proof did not survive the resident restart; run claude respawn %s to reopen it, then submit again\n", job)
				return 2
			}
			if providerControlled {
				fmt.Fprintln(errOut, "CLAUDUCT_ROUTING_PROOF_MISSING: native did not ask this session's Clauduct gateway for its model list; recorded as a diagnostic, since provider control routes this session.")
			} else {
				fmt.Fprintln(errOut, "CLAUDUCT_ROUTING_MISMATCH: native did not ask this session's Clauduct gateway for its model list at start, so its requests go elsewhere -- most likely a stored Claude apps gateway login. End Clauduct, then sign out of that gateway in standalone Claude Code, or start Clauduct with its own CLAUDE_CONFIG_DIR. The session is being ended.")
				postReply(ctx, []byte("{}"), env, RoutingMismatchPath, false)
				return 2
			}
		}
	}
	return runWithOutputContext(ctx, bytes.NewReader(raw), out, errOut, env)
}

// Regenerable, set by the launcher, names the job a dead background resident left a record
// for under link, which native's key helper starts again on the job's next model request
// (#319 decision 18c). Nil, or false, means the link is simply gone.
var Regenerable func(link string) (job string, ok bool)

// Reconnect restores a recorded background job before a startup hook needs its gateway.
var Reconnect func(link, session string)

// ReconnectContext bounds only the caller's wait, never the resident restoration.
var ReconnectContext func(context.Context, string, string)

func reconnect(link string, raw []byte) {
	reconnectContext(context.Background(), link, raw)
}

func reconnectContext(ctx context.Context, link string, raw []byte) {
	if Reconnect == nil && ReconnectContext == nil || len(raw) > maxEventBytes || ctx.Err() != nil {
		return
	}
	var event struct {
		Name    string `json:"hook_event_name"`
		Session string `json:"session_id"`
	}
	if json.Unmarshal(raw, &event) == nil && identifier.MatchString(event.Session) &&
		(event.Name == "SessionStart" || event.Name == "UserPromptSubmit") {
		if ReconnectContext != nil {
			ReconnectContext(ctx, link, event.Session)
		} else {
			Reconnect(link, event.Session)
		}
	}
}

// linkGone answers a hook whose link is gone. For a link that will be regenerated, a
// SessionStart (which cannot block, and runs in a respawned worker before its first model
// request reopens the link) passes quietly: the prompt's own registration follows once the
// link is back. Any other event is refused, naming the respawn that reopens the link.
func linkGone(link string, in io.Reader, errOut io.Writer) int {
	job, regenerable := "", false
	if Regenerable != nil {
		job, regenerable = Regenerable(link)
	}
	if !regenerable {
		fmt.Fprintln(errOut, "CLAUDUCT_CONNECTION_UNAVAILABLE: this session's Clauduct link is gone; restart the session with Clauduct")
		return 2
	}
	var event struct {
		Name string `json:"hook_event_name"`
	}
	raw, _ := io.ReadAll(io.LimitReader(in, maxEventBytes+1))
	if json.Unmarshal(raw, &event) == nil && event.Name == "SessionStart" {
		return 0
	}
	fmt.Fprintf(errOut, "CLAUDUCT_CONNECTION_UNAVAILABLE: this background job's Clauduct connection ended; run claude respawn %s to reopen it, then submit again\n", job)
	return 2
}

// ancestor reports whether pid is one of this process's ancestors, within a bounded walk of
// the current process snapshot. Any failure answers no.
func ancestor(pid int) bool {
	if pid <= 0 {
		return false
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snapshot)
	parents := map[uint32]uint32{}
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}
	return providercert.IsAncestor(uint32(os.Getpid()), uint32(pid), parents, processCreatedAt)
}

// processCreatedAt is when pid was created, or the zero time when the OS will not say.
func processCreatedAt(pid uint32) time.Time {
	created, err := providercert.ProcessCreated(pid)
	if err != nil || created <= 0 {
		return time.Time{}
	}
	return time.Unix(0, created)
}

// RoutingProofPath is the gateway's answer to whether native's start-up model list reached it.
const RoutingProofPath = "/clauduct/routing-proof"

// hostManaged reports whether native's environment still carries the HostManaged strategy
// flag.
func hostManaged(env map[string]string) bool {
	return strategyFlagOn(env, "CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST")
}

// gatewayCompatFlag is the GatewayEnvCompat strategy's flag: a provider switch to #295's check
// unless the launcher named the strategy on the hook's command line.
const gatewayCompatFlag = "CLAUDE_CODE_USE_GATEWAY"

// strategyFlagOn reports whether every spelling of flag in native's environment says "1":
// Windows reads names without case, and which of two spellings native applied last is not
// known.
func strategyFlagOn(env map[string]string, flag string) bool {
	seen := false
	for name, value := range env {
		if strings.EqualFold(name, flag) {
			if value != "1" {
				return false
			}
			seen = true
		}
	}
	return seen
}

// routedElsewhere reads names as Windows does, without case: a settings file may spell one
// in lower case. Under a provider-control strategy the launcher named (controlled), the
// GatewayEnvCompat flag at "1" sends native to ANTHROPIC_BASE_URL, which must still be this
// gateway; every other switch still counts.
func routedElsewhere(env map[string]string, gateway string, controlled bool) bool {
	baseSeen := false
	for name, value := range env {
		switch strings.ToUpper(name) {
		case "ANTHROPIC_BASE_URL":
			baseSeen = true
			if value != gateway {
				return true
			}
		case "ANTHROPIC_UNIX_SOCKET":
			if value != "" {
				return true
			}
		}
	}
	if !baseSeen {
		return true
	}
	for _, name := range launch.ProviderSwitches {
		if controlled && name == gatewayCompatFlag && strategyFlagOn(env, gatewayCompatFlag) {
			continue
		}
		for spelling, value := range env {
			if strings.EqualFold(spelling, name) && value != "" && value != "0" && strings.ToLower(value) != "false" {
				return true
			}
		}
	}
	return false
}

// Native's last spelling is unknown; absent or conflicting values are unusable.
func envValue(env map[string]string, name string) string {
	key := strings.ToUpper(name)
	value, seen := "", false
	for spelling, candidate := range env {
		if strings.ToUpper(spelling) == key {
			if seen && candidate != value {
				return ""
			}
			value, seen = candidate, true
		}
	}
	return value
}

// A native spelling variant must not override the session link's connection.
func pinGatewayEnv(env map[string]string, connection sessionlink.Connection) {
	for name := range env {
		if strings.EqualFold(name, "ANTHROPIC_BASE_URL") || strings.EqualFold(name, "ANTHROPIC_AUTH_TOKEN") {
			delete(env, name)
		}
	}
	env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_AUTH_TOKEN"] = connection.BaseURL, connection.Token
}

func runWithOutput(in io.Reader, out, errOut io.Writer, env map[string]string) int {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	return runWithOutputContext(ctx, in, out, errOut, env)
}

func runWithOutputContext(ctx context.Context, in io.Reader, out, errOut io.Writer, env map[string]string) int {
	raw, err := readEvent(ctx, in)
	if errors.Is(err, errHookDeadline) {
		return hookDeadline(errOut)
	}
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
		if _, err := postReply(ctx, body, env, "/clauduct/tool-failures", false); err != nil {
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
		if _, err := postReply(ctx, body, env, "/clauduct/workflows", false); err != nil {
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
		reply, err := postReply(ctx, body, env, "/clauduct/context", compact.Event == "SessionStart")
		if err != nil {
			if errors.Is(err, errHookDeadline) {
				return hookDeadline(errOut)
			}
			if errors.Is(err, errSessionRestart) {
				fmt.Fprintf(errOut, "SESSION_RESTART_REQUIRED: save any draft, finish native, then run clauduct --resume %s with any native options you need.\n", compact.Session)
				return 2
			}
			if compact.Event == "UserPromptSubmit" {
				var refusal *registrationFailure
				cause := ""
				if errors.As(err, &refusal) && refusal.category != "" {
					if refusal.category == "SESSION_START_UNREGISTERED" {
						fmt.Fprintf(errOut, "SESSION_START_UNREGISTERED: Clauduct has no start registration it can use for this session, so a prompt cannot register it; save any draft, finish native, then run clauduct --resume %s to continue in the foreground, or clauduct --resume %s --fork-session --bg to start a background copy, leaving the original unchanged. Model and effort will use that launch's starting selection.\n", compact.Session, compact.Session)
						return 2
					}
					cause = " (" + refusal.category + ")"
				}
				fmt.Fprintf(errOut, "CLAUDUCT_CONTEXT_SESSION_UNVERIFIED: session registration failed%s; restore the Clauduct hook connection and submit the prompt again.\n", cause)
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
	if err := post(ctx, body, env); err != nil {
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
	window, windowOK := positiveInt(envValue(env, "CLAUDE_CODE_MAX_CONTEXT_TOKENS"))
	auto, autoOK := positiveInt(envValue(env, "CLAUDE_CODE_AUTO_COMPACT_WINDOW"))
	percent, percentErr := strconv.ParseFloat(envValue(env, "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"), 64)
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
func post(ctx context.Context, body []byte, env map[string]string) error {
	_, err := postReply(ctx, body, env, "/clauduct/agents", false)
	return err
}

func postReply(parent context.Context, body []byte, env map[string]string, path string, retryRegistration bool) ([]byte, error) {
	base := envValue(env, "ANTHROPIC_BASE_URL")
	token := envValue(env, "ANTHROPIC_AUTH_TOKEN")
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

	requestBudget := requestTimeout
	if path == RoutingProofPath {
		requestBudget = proofRequestTimeout
	}
	ctx, cancel := context.WithTimeout(parent, requestBudget)
	defer cancel()
	var dialed atomic.Bool
	client := &http.Client{
		Timeout: requestBudget,
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
				return (&net.Dialer{Timeout: requestBudget}).DialContext(ctx, network, address)
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
			if errors.Is(parent.Err(), context.DeadlineExceeded) {
				return nil, errHookDeadline
			}
			return nil, err
		}
		reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
		response.Body.Close()
		if err != nil || len(reply) > 4096 {
			if err != nil && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent) && errors.Is(parent.Err(), context.DeadlineExceeded) {
				return nil, errHookDeadline
			}
			return nil, errInvalidGateway
		}
		// Only the original SessionStart can carry the first profile's source.
		// Retry its metadata once on temporary unavailability, within the SAME
		// three-second budget. Explicit refusals and all other events stay final.
		if retryRegistration && path == "/clauduct/context" && attempt == 0 && response.StatusCode == http.StatusServiceUnavailable && len(reply) == 0 {
			continue
		}
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
			if path == RoutingProofPath && (response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusServiceUnavailable) {
				var envelope struct {
					Type  string `json:"type"`
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal(reply, &envelope) == nil && envelope.Type == "error" {
					if response.StatusCode == http.StatusConflict && envelope.Error.Message == "ROUTING_PROOF_PRIOR_GENERATION" {
						return nil, errRoutingProofPriorGeneration
					}
					if response.StatusCode == http.StatusServiceUnavailable && envelope.Error.Message == "GATEWAY_CLOSED" {
						return nil, errRoutingProofGatewayClosed
					}
				}
			}
			if path == "/clauduct/context" {
				var envelope struct {
					Type  string `json:"type"`
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal(reply, &envelope) == nil && envelope.Type == "error" {
					if response.StatusCode == http.StatusBadRequest &&
						(envelope.Error.Message == errSessionRestart.Error() || strings.HasPrefix(envelope.Error.Message, errSessionRestart.Error()+";")) {
						return nil, errSessionRestart
					}
					if category := DiagnosticCategory(envelope.Error.Message); category != "" {
						return nil, &registrationFailure{response.StatusCode, category}
					}
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
var errHookDeadline = errors.New("CLAUDUCT_HOOK_DEADLINE")

// The one-shot hook process exits on timeout; a blocked reader cannot delay its exit.
func readEvent(ctx context.Context, in io.Reader) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errHookDeadline
	}
	type result struct {
		raw []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := io.ReadAll(io.LimitReader(in, maxEventBytes+1))
		done <- result{raw, err}
	}()
	select {
	case value := <-done:
		if ctx.Err() != nil {
			return nil, errHookDeadline
		}
		return value.raw, value.err
	case <-ctx.Done():
		return nil, errHookDeadline
	}
}

var errRoutingProofPriorGeneration = errors.New("ROUTING_PROOF_PRIOR_GENERATION")
var errRoutingProofGatewayClosed = errors.New("GATEWAY_CLOSED")

func hookDeadline(errOut io.Writer) int {
	fmt.Fprintln(errOut, "CLAUDUCT_HOOK_DEADLINE: the Clauduct hook did not finish before its deadline; restore the Clauduct hook connection and submit the prompt again.")
	return 2
}

type registrationFailure struct {
	status   int
	category string
}

func (e *registrationFailure) Error() string {
	return fmt.Sprintf("REGISTRATION_FAILED %d %s", e.status, e.category)
}

// DiagnosticCategory admits only fixed failure names, never gateway or OS error text.
func DiagnosticCategory(value string) string {
	switch value {
	case "MEMORY_ADMISSION_TIMEOUT", "MEMORY_BUDGET_EXCEEDED", "MEMORY_QUEUE_FULL", "MEMORY_STATUS_UNAVAILABLE",
		"SESSION_SELECTION_UNVERIFIED", "SESSION_PROFILE_UNVERIFIED", "SESSION_START_UNREGISTERED", "SESSION_ALREADY_IN_USE", "CONTEXT_JOURNAL_UNVERIFIED",
		"RESIDENT_RECORD_INVALID", "RESIDENT_PORT_MISMATCH":
		return value
	}
	return ""
}
