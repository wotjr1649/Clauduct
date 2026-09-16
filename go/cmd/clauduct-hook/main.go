// Command clauduct-hook reports a subagent's start and stop to the session's gateway.
//
// The client runs it. A SubagentStart or SubagentStop hook hands it the event on stdin, it
// turns that into a binding, and it posts the binding to the loopback gateway this session
// started. That is the only way the gateway can know a subagent exists, because the client
// is what starts one.
//
// A separate binary, and not a subcommand of clauduct-go, for the reason clauduct-dev is
// separate too: the launcher forwards every argument to the native client, so a subcommand
// there could collide with a native option or a native option's value.
//
// What it sends is an identifier, a role name, where the client keeps its transcript, and
// the context window it was given. Never a prompt, an output, or a tool call -- those are in
// the event it reads and are deliberately left there.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() { os.Exit(run(os.Stdin, os.Stderr, environ())) }

// The bounds. A hook event is a small object; anything past these is not one.
const (
	maxEventBytes   = 1 << 20
	maxBindingBytes = 4096
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
	raw, err := io.ReadAll(io.LimitReader(in, maxEventBytes+1))
	if err != nil || len(raw) > maxEventBytes {
		fmt.Fprintln(errOut, "CLAUDUCT_AGENT_ROUTE_FAILED")
		return 1
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
	if stop {
		// A stop withdraws a registration. Where the transcript is and how big the window
		// was are facts about a subagent that is finished, so they are not sent.
		return out, true
	}
	if identifier.MatchString(event.SessionID) && event.TranscriptPath != "" &&
		len(event.TranscriptPath) <= 4096 {
		out.SessionID = event.SessionID
		out.TranscriptPath = event.TranscriptPath
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
	base := env["ANTHROPIC_BASE_URL"]
	token := env["ANTHROPIC_AUTH_TOKEN"]
	if !loopback.MatchString(base) || token == "" {
		return errInvalidGateway
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return errInvalidGateway
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return errInvalidGateway
	}

	request, err := http.NewRequest(http.MethodPost, base+"/clauduct/agents", bytes.NewReader(body))
	if err != nil {
		return err
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
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("REGISTRATION_FAILED %d", response.StatusCode)
	}
	return nil
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
