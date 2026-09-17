package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// probeWorkflow is the smallest script the Workflow tool accepts: a literal meta block and
// one delegated agent. One agent, because the question is whether a workflow's agents reach
// this bridge at all, not how many of them can.
const probeWorkflow = `export const meta = { name: 'probe', description: 'one delegated agent' }
const out = await agent('Reply with the single word: probe', { label: 'probe' })
return { out }`

// A workflow, end to end, through the real client.
//
// What was measured before was a workflow agent's routing, by hand, once. What nobody had
// run is the whole path as a test: the client executing a Workflow call this bridge
// delivered, the agents it starts coming back here as requests, and the account saying the
// right thing about them afterwards.
//
// The account is half the point. A workflow agent carries the role name "workflow-subagent"
// and no route of its own, and an earlier build counted that as a routing failure -- which
// made every session that ran a workflow report itself as having something wrong. That is
// the diagnosis that cries every time and so stops being read.
func TestAWorkflowsAgentsReachTheBridge(t *testing.T) {
	buildHook(t)
	exe := nativeAvailable(t)

	input, err := json.Marshal(map[string]string{"script": probeWorkflow})
	if err != nil {
		t.Fatalf("encode tool input: %v", err)
	}
	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation, SSE: toolStream("call_wf_1", "Workflow", string(input))},
			{When: upstream.Conversation, SSE: textStream("wf_agent", "probe")},
			{When: upstream.Conversation, SSE: textStream("main", "done")},
		},
		Default: textStream("side", "untitled"),
	}

	var g *gateway.Gateway
	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args: []string{"-p", "run the workflow", "--allowedTools", "Workflow",
			"--strict-mcp-config"},
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

	unregistered, unrouted := g.Unrouted()

	// Which request is the workflow's agent is decided by what it was handed, not by where
	// it falls: the session can send a side request at any point. A workflow agent cannot
	// start another workflow, so the conversation carrying tools but no Workflow among them
	// is the agent's.
	var spawner, agent string
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
		if json.Unmarshal([]byte(request), &envelope) != nil || len(envelope.Tools) == 0 {
			continue
		}
		where := envelope.Model + "/" + envelope.Reasoning.Effort
		carries := false
		for _, tool := range envelope.Tools {
			if tool.Name == "Workflow" {
				carries = true
				break
			}
		}
		if carries {
			spawner = where
			continue
		}
		if agent != "" && agent != where {
			t.Fatalf("two different workflow agent requests: %s and %s", agent, where)
		}
		agent = where
	}

	t.Logf("exit=%d requests=%d conversation=%d spawner=%s agent=%s unregistered=%d unrouted=%d",
		result.NativeExitCode, script.Calls(), len(script.Conversations()),
		spawner, agent, unregistered, unrouted)

	if agent == "" {
		t.Fatal("no workflow agent request arrived: the client either refused the call or " +
			"the workflow never started, and nothing about workflows was measured")
	}
	// Parent inherit. The baseline reaches the same answer by reading a journal off disk
	// and verifying it; this build reaches it by keeping the model the client asked for
	// when a role has no route of its own, and reads nothing.
	if agent != spawner {
		t.Fatalf("the workflow agent ran on %s while the session ran on %s; a workflow agent "+
			"inherits the session route", agent, spawner)
	}
	// And the account has nothing to report. A workflow agent has a role name and no route,
	// which an earlier build counted as a routing failure -- so every session that ran a
	// workflow reported itself as having something wrong.
	if unregistered != 0 || unrouted != 0 {
		t.Fatalf("counts = (%d, %d), want (0, 0): the hook reached the gateway and "+
			"workflow-subagent is an intentional inherit, not a failure", unregistered, unrouted)
	}
	if result.NativeExitCode != 0 {
		t.Fatalf("exit = %d", result.NativeExitCode)
	}
}
