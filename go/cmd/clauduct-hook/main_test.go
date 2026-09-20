package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func TestRejectedWorkflowIsDeniedWithoutGatewayOrOriginalOperation(t *testing.T) {
	for _, script := range []string{bridge.RejectedWorkflowScript, "return 42;", bridge.RejectedWorkflowScript + "\nreturn 42;"} {
		raw, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Workflow", "tool_input": map[string]string{"script": script}})
		var out, errOut bytes.Buffer
		if code := runWithOutput(bytes.NewReader(raw), &out, &errOut, nil); code != 0 {
			t.Fatal(code, errOut.String())
		}
		if script != bridge.RejectedWorkflowScript {
			if out.Len() != 0 {
				t.Fatal("unrelated call changed")
			}
			continue
		}
		var resultFields map[string]map[string]string
		if json.Unmarshal(out.Bytes(), &resultFields) != nil || resultFields["hookSpecificOutput"]["permissionDecision"] != "deny" || resultFields["hookSpecificOutput"]["permissionDecisionReason"] != bridge.RejectedWorkflowReason {
			t.Fatal("not a native denial", out.String())
		}
	}
}

func gatewayEnv(base string) map[string]string {
	return map[string]string{"ANTHROPIC_BASE_URL": base, "ANTHROPIC_AUTH_TOKEN": "session-token"}
}

func TestCompactionEventsSendOnlyIdentityAndFailClosed(t *testing.T) {
	for _, status := range []int{204, 400} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/clauduct/context" || strings.Contains(string(raw), "synthetic-private") {
				t.Error("context event forwarded content or wrong destination")
			}
			w.WriteHeader(status)
		}))
		for _, event := range []string{"PreCompact", "PostCompact"} {
			var out bytes.Buffer
			code := run(strings.NewReader(`{"hook_event_name":"`+event+`","session_id":"s1","agent_id":"a1","compact_summary":"synthetic-private"}`), &out, gatewayEnv(server.URL))
			if status == 204 && code != 0 || status == 400 && code != 2 {
				t.Fatalf("event result %d: code=%d", status, code)
			}
		}
		server.Close()
	}
}

// B4. What the hook reports is an identifier, a role, and where the client keeps its
// transcript. The event it reads carries a great deal more and none of that travels.
func TestOnlyTheBindingTravels(t *testing.T) {
	var got []byte
	var header http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(io.LimitReader(r.Body, 1<<16))
		header = r.Header.Clone()
		if r.URL.Path != "/clauduct/agents" {
			t.Errorf("posted to %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := `{"hook_event_name":"SubagentStart","agent_id":"agent_1","agent_type":"Explore",
	  "session_id":"1b0b3297-ade4-4aa4-86c8-80eaf43db0a3",
	  "transcript_path":"C:/x/.claude/projects/p/s.jsonl",
	  "prompt":"SECRET PROMPT TEXT","tool_input":{"task":"SECRET TASK"},
	  "transcript":"SECRET TRANSCRIPT"}`

	var errOut bytes.Buffer
	if code := run(strings.NewReader(event), &errOut, gatewayEnv(server.URL)); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}

	for _, secret := range []string{"SECRET PROMPT TEXT", "SECRET TASK", "SECRET TRANSCRIPT"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Errorf("the event's own content travelled: %q in %s", secret, got)
		}
	}
	var sent binding
	if err := json.Unmarshal(got, &sent); err != nil {
		t.Fatalf("decode %s: %v", got, err)
	}
	if sent.ID != "agent_1" || sent.Role != "Explore" || sent.Stop {
		t.Fatalf("binding = %+v", sent)
	}
	if sent.SessionID == "" || sent.TranscriptPath == "" {
		t.Errorf("the transcript location did not travel: %+v", sent)
	}
	if header.Get("Authorization") != "Bearer session-token" {
		t.Errorf("Authorization = %q", header.Get("Authorization"))
	}
}

// Completion carries only correlation and its existing report, for event-first
// parent delivery. Context settings and unrelated hook fields stay behind.
func TestAStopCarriesCorrelationAndExistingResult(t *testing.T) {
	var got []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(io.LimitReader(r.Body, 1<<16))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := `{"hook_event_name":"SubagentStop","agent_id":"agent_1","agent_type":"Explore",
	  "session_id":"1b0b3297-ade4-4aa4-86c8-80eaf43db0a3",
	  "transcript_path":"C:/x/.claude/projects/p/s.jsonl","last_assistant_message":"PUBLIC_REPORT","unrelated":"DO_NOT_FORWARD"}`
	var errOut bytes.Buffer
	if code := run(strings.NewReader(event), &errOut, gatewayEnv(server.URL)); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var sent binding
	if err := json.Unmarshal(got, &sent); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !sent.Stop {
		t.Fatal("a SubagentStop did not report one")
	}
	if sent.TranscriptPath == "" || sent.SessionID == "" || sent.Result != "PUBLIC_REPORT" || sent.Context != nil || bytes.Contains(got, []byte("DO_NOT_FORWARD")) {
		t.Error("completion lost correlation/report or forwarded unrelated data")
	}
}

func TestToolFailureEventNeverForwardsInputsOrErrorText(t *testing.T) {
	var got []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/clauduct/tool-failures" {
			t.Error("wrong route")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	var log bytes.Buffer
	code := run(strings.NewReader(`{"hook_event_name":"PostToolUseFailure","session_id":"public_session","tool_use_id":"public_call","tool_name":"mcp__private_name","is_interrupt":true,"error":"DO_NOT_COPY","tool_input":{"value":"DO_NOT_COPY"}}`), &log, gatewayEnv(server.URL))
	if code != 0 || bytes.Contains(got, []byte("private_name")) || bytes.Contains(got, []byte("DO_NOT_COPY")) || !bytes.Contains(got, []byte(`"tool":"MCP"`)) || !bytes.Contains(got, []byte(`"interrupted":true`)) {
		t.Fatal("failure receipt boundary", code)
	}
}

func TestEscapedResultAtDecodedLimitSurvivesJSONExpansion(t *testing.T) {
	result := strings.Repeat("<", 256<<10)
	event, _ := json.Marshal(map[string]any{"hook_event_name": "SubagentStop", "agent_id": "public_child", "agent_type": "Plan", "session_id": "public_session", "transcript_path": "C:/public/public_session.jsonl", "last_assistant_message": result})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b binding
		if json.NewDecoder(r.Body).Decode(&b) != nil || b.Result != result {
			t.Error("escaped result was lost")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	var log bytes.Buffer
	if run(bytes.NewReader(event), &log, gatewayEnv(server.URL)) != 0 {
		t.Fatal("valid decoded-limit result refused", log.String())
	}
}

// The session token goes to this session's gateway and nowhere else.
//
// The destination comes from the environment and so does the token, so without the
// loopback check the two together are a way to post a credential wherever a variable says.
func TestTheTokenGoesNowhereButTheLoopbackGateway(t *testing.T) {
	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	elsewhere := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	for name, base := range map[string]string{
		"a hostname that resolves to the same place": elsewhere,
		"somewhere else entirely":                    "http://example.invalid:443",
		"https rather than the loopback":             strings.Replace(server.URL, "http://", "https://", 1),
		"nothing at all":                             "",
	} {
		t.Run(name, func(t *testing.T) {
			reached = false
			var errOut bytes.Buffer
			event := `{"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"Explore"}`
			if code := run(strings.NewReader(event), &errOut, gatewayEnv(base)); code == 0 {
				t.Fatal("it reported success for a destination it must not use")
			}
			if reached {
				t.Fatal("the token was sent somewhere other than the loopback gateway")
			}
		})
	}
}

// An event this hook does not report on is not an error. The client fires hooks for its own
// reasons and a matcher may hand over more than this acts on.
func TestAnEventThisHookDoesNotReportOnIsNotAFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("it posted for an event it does not report on")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	for name, event := range map[string]string{
		"another hook entirely":         `{"hook_event_name":"PostToolUse","tool_name":"Read"}`,
		"an identifier that is not one": `{"hook_event_name":"SubagentStart","agent_id":"../x","agent_type":"Explore"}`,
		"no role":                       `{"hook_event_name":"SubagentStart","agent_id":"a1"}`,
		"not JSON":                      `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			var errOut bytes.Buffer
			if code := run(strings.NewReader(event), &errOut, gatewayEnv(server.URL)); code != 0 {
				t.Fatalf("exit %d for an event that is simply not this one: %s", code, errOut.String())
			}
		})
	}
}

// A gateway that refuses is a failure the client should see.
func TestARefusedRegistrationIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	var errOut bytes.Buffer
	event := `{"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"Explore"}`
	if code := run(strings.NewReader(event), &errOut, gatewayEnv(server.URL)); code == 0 {
		t.Fatal("a refused registration was reported as success")
	}
	if !strings.Contains(errOut.String(), "CLAUDUCT_AGENT_ROUTE_FAILED") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

// The context window travels only when the client stated all of it.
func TestThePolicyTravelsWholeOrNotAtAll(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"all three stated": {
			"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  "400000",
			"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "320000",
			"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "84.2",
		},
		"one of them missing": {
			"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  "400000",
			"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "84.2",
		},
		"a percentage outside its range": {
			"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  "400000",
			"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "320000",
			"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "140",
		},
	} {
		t.Run(name, func(t *testing.T) {
			policy := contextFrom(env)
			whole := len(env) == 3 && env["CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"] == "84.2"
			if whole != (policy != nil) {
				t.Fatalf("policy = %+v for %v", policy, env)
			}
		})
	}
}
