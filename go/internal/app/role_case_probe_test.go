package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Measure native role lookup before Clauduct rewrites any Agent arguments.
func TestNativeCaseDistinctRole(t *testing.T) {
	native := nativeAvailable(t)
	config := t.TempDir()
	_, cwd := workspace(t)
	dir := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Fork.md"), []byte("---\nname: Fork\ndescription: public case proof\ntools: Read\nmodel: gpt-6-astra\neffort: medium\n---\nPUBLIC_CASE_DISTINCT_ROLE_PROOF"), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	spawned, custom, childModel := false, false, ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages/count_tokens" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"input_tokens":1000}`)
			return
		}
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request struct {
			Model string
			Tools []json.RawMessage
		}
		if json.Unmarshal(raw, &request) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		isCustom := strings.Contains(string(raw), "PUBLIC_CASE_DISTINCT_ROLE_PROOF")
		if isCustom {
			custom, childModel = true, request.Model
		}
		delegate := !spawned && !isCustom && len(request.Tools) != 0
		if delegate {
			spawned = true
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
		event("message_start", `{"type":"message_start","message":{"id":"msg_case","type":"message","role":"assistant","model":"public-fixture","content":[],"usage":{"input_tokens":1000,"output_tokens":0}}}`)
		stop := "end_turn"
		if delegate {
			event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_case","name":"Agent","input":{}}}`)
			input, _ := json.Marshal(`{"subagent_type":"Fork","description":"public proof","prompt":"say ok"}`)
			event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+string(input)+`}}`)
			stop = "tool_use"
		} else {
			event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
		}
		event("content_block_stop", `{"type":"content_block_stop","index":0}`)
		event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"`+stop+`","stop_sequence":null},"usage":{"output_tokens":1}}`)
		event("message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, native, "-p", "delegate public proof", "--model", "gpt-5.6-luna", "--effort", "low", "--allowedTools", "Agent", "--strict-mcp-config")
	cmd.Dir = cwd
	for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATH", "PATHEXT"} {
		cmd.Env = append(cmd.Env, key+"="+os.Getenv(key))
	}
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "CLAUDE_CONFIG_DIR"} {
		cmd.Env = append(cmd.Env, key+"="+config)
	}
	cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+server.URL, "ANTHROPIC_AUTH_TOKEN=synthetic-local-only", "CLAUDE_CODE_FORK_SUBAGENT=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1", "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1")
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	mu.Lock()
	defer mu.Unlock()
	if err != nil || !spawned || !custom || childModel != "gpt-6-astra" {
		t.Fatalf("native case lookup: err=%v spawned=%v custom=%v model=%s output=%s", err, spawned, custom, childModel, tail(string(output), 1200))
	}
	t.Log("native Fork.md is reachable and selects gpt-6-astra independently of the parent")
}
