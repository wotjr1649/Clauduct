package gateway

import (
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"net/http"
	"strings"
	"testing"
)

func TestSendMessageSoftFailureReleasesResumeAndCountsOnce(t *testing.T) {
	for _, tool := range []string{"SendMessage", "Read"} {
		t.Run(tool, func(t *testing.T) {
			g := startWith(t, nil)
			g.ConfigureDelegations(t.TempDir())
			g.delegations.resumes = map[string]*resumeBinding{"child": {session: "session", call: "call"}}
			r := &anthropic.Request{Messages: []anthropic.Message{
				{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", Name: tool, ID: "call"}}},
				{Role: "user", Blocks: []anthropic.Block{{Type: "tool_result", ToolUseID: "call", Result: []anthropic.ResultPart{{Type: "text", Text: `{"success":false,"message":"PUBLIC_NOT_DELIVERED"}`}}}}},
			}}
			for i := 0; i < 2; i++ {
				g.observeMessageFailures(r, "session", "")
			}
			want := int64(0)
			if tool == "SendMessage" {
				want = 1
			}
			if g.toolFailures.snapshot().Total != want || (g.delegations.resumes["child"] == nil) != (want == 1) {
				t.Fatal("failure was not linked, released and deduplicated")
			}
		})
	}
}

func TestNativeToolFailuresCountOnceWithoutNextInference(t *testing.T) {
	d, scope, _ := preparedDelegation(t)
	g := startWith(t, nil)
	g.delegations = d
	event := `{"session":"` + scope.session + `","call":"proof_call","tool":"Agent","interrupted":false}`
	for i := 0; i < 2; i++ {
		reply := do(t, g, request{method: http.MethodPost, path: "/clauduct/tool-failures", headers: map[string]string{"Content-Type": "application/json"}, body: strings.NewReader(event)})
		bodyText(t, reply)
		if reply.StatusCode != 204 {
			t.Fatal(reply.StatusCode)
		}
	}
	if g.toolFailures.snapshot().Total != 1 || d.selectionReport().Totals["native_tool_failed"] != 1 || len(d.pending) != 0 {
		t.Fatal("failure event not linked/deduplicated")
	}
	bad := strings.Replace(event, `"Agent"`, `"private arbitrary tool"`, 1)
	reply := do(t, g, request{method: http.MethodPost, path: "/clauduct/tool-failures", headers: map[string]string{"Content-Type": "application/json"}, body: strings.NewReader(bad)})
	bodyText(t, reply)
	if reply.StatusCode != 400 || g.toolFailures.snapshot().Total != 1 {
		t.Fatal("arbitrary data admitted to diagnostics")
	}
}

func TestToolResultErrorsWithoutNativeFailureHook(t *testing.T) {
	for _, tool := range []string{"TaskStop", "Read", "Bash", "mcp__private_tool", "private_tool"} {
		t.Run(tool, func(t *testing.T) {
			g := startWith(t, nil)
			req := &anthropic.Request{Messages: []anthropic.Message{
				{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", ID: "public_call", Name: tool}}},
				{Role: "user", Blocks: []anthropic.Block{{Type: "tool_result", ToolUseID: "public_call", IsError: true, Result: []anthropic.ResultPart{{Type: "text", Text: "private error must not be retained"}}}}},
			}}
			g.observeMessageFailures(req, "session", "")
			g.observeMessageFailures(req, "session", "")
			r := g.toolFailures.snapshot()
			want := tool
			if strings.HasPrefix(tool, "mcp__") {
				want = "MCP"
			}
			if tool == "private_tool" {
				want = "other"
			}
			if r.Total != 1 || r.Recent[0].Tool != want || r.Recent[0].Source != "tool_result" {
				t.Fatalf("wrong identity or dedup: %+v", r)
			}
		})
	}
	for _, linked := range []bool{false, true} {
		g := startWith(t, nil)
		req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "tool_result", ToolUseID: "missing", IsError: !linked}}}}}
		if linked {
			req.Messages = append([]anthropic.Message{{Role: "assistant", Blocks: []anthropic.Block{{Type: "tool_use", ID: "missing", Name: "TaskStop"}}}}, req.Messages...)
		}
		g.observeMessageFailures(req, "session", "")
		if g.toolFailures.snapshot().Total != 0 {
			t.Fatal("unlinked or successful result counted as failure")
		}
	}
}

func TestToolFailureSourcesPreserveCancellationAndPolicyRejections(t *testing.T) {
	for _, hookFirst := range []bool{false, true} {
		g := startWith(t, nil)
		hook := ToolFailureRecord{Session: "session", Call: "call", Tool: "Bash", Interrupted: true, Source: "native_failure_hook"}
		result := ToolFailureRecord{Session: "session", Call: "call", Tool: "Bash", Source: "tool_result"}
		if hookFirst {
			g.recordToolFailure(hook)
		}
		g.recordToolFailure(result)
		g.recordToolFailure(hook)
		g.recordToolFailure(result)
		r := g.toolFailures.snapshot()
		if r.Total != 1 || r.Interrupted != 1 || !r.Recent[0].Interrupted {
			t.Fatalf("lost cancellation evidence: %+v", r)
		}
	}
	g := startWith(t, nil)
	key := delegationKey{"session", "call"}
	g.delegations = &delegations{workflowCalls: map[delegationKey]workflowOrigin{key: {rejected: true}}}
	g.recordToolFailure(ToolFailureRecord{Session: "session", Call: "call", Tool: "Workflow", Source: "native_failure_hook"})
	if g.toolFailures.snapshot().Total != 0 || !g.delegations.workflowCalls[key].rejected {
		t.Fatal("policy rejection reclassified or its history removed")
	}
}
