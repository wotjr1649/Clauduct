package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gatewayEnv(base string) map[string]string {
	return map[string]string{"ANTHROPIC_BASE_URL": base, "ANTHROPIC_AUTH_TOKEN": "session-token"}
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

// A stop withdraws a registration and says nothing else about it.
func TestAStopCarriesNothingButTheWithdrawal(t *testing.T) {
	var got []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(io.LimitReader(r.Body, 1<<16))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := `{"hook_event_name":"SubagentStop","agent_id":"agent_1","agent_type":"Explore",
	  "session_id":"1b0b3297-ade4-4aa4-86c8-80eaf43db0a3",
	  "transcript_path":"C:/x/.claude/projects/p/s.jsonl"}`
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
	if sent.TranscriptPath != "" || sent.SessionID != "" {
		t.Errorf("a finished subagent still described where its transcript is: %+v", sent)
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
