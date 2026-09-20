package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// hookPath is where findHook looks, which under `go test` is beside the test binary.
func hookPath(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path: %v", err)
	}
	return filepath.Join(filepath.Dir(self), hookBinary+hookSuffix)
}

// buildHook puts the hook program where findHook looks for it.
//
// Building it rather than pointing hand-written settings at something is the point: what
// installs the hook here is the production path, findHook included.
func buildHook(t *testing.T) {
	t.Helper()
	path := hookPath(t)
	build := exec.Command("go", "build", "-o", path,
		"github.com/wotjr1649/Clauduct/go/cmd/clauduct-hook")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hook: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.Remove(path) })
}

// agentStream asks the client to start a subagent.
//
// The tool is named Agent in this client, not Task. Measured 2026-09-16: a scripted call to
// Task produced no subagent and no tool result at all -- the client dropped the call and
// re-sent the same conversation, so a test written against the baseline's name would have
// passed while measuring nothing.
// callerModel, when given, is the Agent tool's own model argument. Measured 2026-09-18 in a
// real session: the calling model fills that argument in unasked -- four delegations, four
// times, none of them requested by the user -- so a test that never sets it is testing the
// polite case and nothing else.
func agentStream(role, prompt string, callerModel ...string) string {
	model := ""
	if len(callerModel) > 0 && callerModel[0] != "" {
		model = `"model":"` + callerModel[0] + `",`
	}
	return toolStream("call_agent_1", "Agent", `{"subagent_type":"`+role+
		`",`+model+`"description":"look around","prompt":"`+prompt+`"}`)
}

// subagentRun starts a session whose first turn spawns one subagent.
func subagentRun(t *testing.T, accounts ...*gateway.Diagnostics) (session, sub string, unregistered, unrouted int64) {
	t.Helper()
	exe := nativeAvailable(t)
	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: agentStream("Explore", "say ok")},
			{When: upstream.Conversation, SSE: textStream("sub", "ok")},
			{When: upstream.Conversation, SSE: textStream("main", "done")},
		},
		Default: textStream("side", "untitled"),
	}

	var g *gateway.Gateway
	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()

	if _, err := Run(ctx, Options{
		Args:          []string{"-p", "explore this", "--allowedTools", "Agent", "--strict-mcp-config"},
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
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, account := range accounts {
		*account = g.Diagnose()
	}

	// Which request is the subagent's is decided by what it was given to work with, not by
	// where it falls in the order: the session can send a side request at any point and the
	// count moved between two runs of this. A subagent cannot start another subagent, so the
	// conversation that has tools but no Agent among them is the subagent's.
	for _, request := range script.Requests() {
		var envelope struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		_ = json.Unmarshal([]byte(request), &envelope)
		if len(envelope.Tools) == 0 {
			continue
		}
		spawner := false
		for _, tool := range envelope.Tools {
			if tool.Name == "Agent" {
				spawner = true
				break
			}
		}
		where := envelope.Model + "/" + envelope.Reasoning.Effort
		if spawner {
			session = where
			continue
		}
		if sub != "" && sub != where {
			t.Fatalf("two different subagent requests: %s and %s", sub, where)
		}
		sub = where
	}
	unregistered, unrouted = g.Unrouted()
	t.Logf("session=%s subagent=%s unregistered=%d unrouted=%d stdout=%q",
		session, sub, unregistered, unrouted, stdout.String())
	return session, sub, unregistered, unrouted
}

// C2 end to end: the client starts a subagent, the hook reports its role, and the subagent's
// own requests run where the role says rather than where the client asked.
//
// Nothing had measured this. Each piece was tested in isolation and the pieces agreed with
// each other, which is exactly the arrangement that has been wrong four times in this
// redesign. What it took to see it: the client asks for gpt-5.6-sol for a built-in Explore
// -- visible in its own stderr as agent:builtin:Explore -- and what leaves this bridge is
// gpt-5.6-luna at max. sol to luna is not a tier mapping. It is the role.
func TestASubagentRunsWhereItsRoleSaysAndNotWhereTheClientAsked(t *testing.T) {
	buildHook(t)

	session, sub, unregistered, unrouted := subagentRun(t)
	if sub == "" {
		t.Fatal("the subagent never made a request of its own")
	}
	if sub != "gpt-5.6-luna/max" {
		t.Fatalf("the subagent ran on %s, want gpt-5.6-luna/max for Explore", sub)
	}
	if unregistered != 0 || unrouted != 0 {
		t.Fatalf("counts = (%d, %d); the registration did not reach the request",
			unregistered, unrouted)
	}
	// The session is untouched. Routing a subagent is not routing the conversation.
	if session != "gpt-6-astra/low" {
		t.Errorf("the session moved to %s", session)
	}
}

// Missing registration must stop an adapted child. Falling back to the client's
// request was the old behavior; it contradicts the verified-selection contract.
func TestWithNoHookTheUnverifiedSubagentNeverReachesBackend(t *testing.T) {
	// Another test in this package builds it beside the test binary. Make its absence the
	// condition rather than an assumption about test order.
	if path := hookPath(t); path != "" {
		if _, err := os.Stat(path); err == nil {
			os.Remove(path)
			t.Cleanup(func() {})
		}
	}

	var account gateway.Diagnostics
	_, sub, _, _ := subagentRun(t, &account)
	if sub != "" || account.Requests.RefusedBy["AGENT_SELECTION_UNVERIFIED"] == 0 {
		t.Fatal("unverified child executed or its refusal was not observed")
	}
}
