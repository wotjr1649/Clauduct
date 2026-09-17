package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// G5. The options the baseline refused are the reason this redesign exists, and until now
// they were only checked as far as the child's argv. Reaching argv is not working.
//
// MCP is the one worth measuring most. The baseline dropped every environment name matching
// TOKEN or SECRET before starting the child, which is why MCP servers did not work under it
// -- a Slack or Notion key never arrived. This build drops ANTHROPIC_* and the OAuth token
// and nothing else, and the user accepted that trade. Whether it actually holds is a fact
// about a running server, not about a denylist.

// mcpSecret is named to match the baseline's own rule exactly. It contains both of the
// substrings that rule looked for.
const mcpSecret = "MCP_STUB_SECRET_TOKEN"

// stubServer builds the MCP server and writes a config naming it.
func stubServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	server := filepath.Join(dir, "mcpstub"+hookSuffix)
	build := exec.Command("go", "build", "-o", server,
		"github.com/wotjr1649/Clauduct/go/internal/app/testdata/mcpstub")
	build.Dir = moduleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, out)
	}

	config := filepath.Join(dir, "mcp.json")
	encoded, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"stub": map[string]any{"command": server, "args": []string{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return config
}

// TestAnMCPServerActuallyRunsAndKeepsItsCredential is the whole G5 claim in one measurement:
// the option is honoured, the server starts, its tool is offered to the model, the call runs
// through this bridge, and the server's own environment survived the launcher.
func TestAnMCPServerActuallyRunsAndKeepsItsCredential(t *testing.T) {
	exe := nativeAvailable(t)
	config := stubServer(t)

	script := &upstream.Script{
		Turns: []upstream.ScriptTurn{
			{When: upstream.Conversation,
				SSE: toolStream("call_mcp_1", "mcp__stub__report", `{"name":"`+mcpSecret+`"}`)},
			{When: upstream.Conversation, SSE: textStream("done", "ok")},
		},
		Default: textStream("side", "untitled"),
	}

	env := isolatedEnv(t)
	env[mcpSecret] = "the-servers-own-credential"
	_, cwd := workspace(t)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*defaultNativeTimeout)
	defer cancel()

	if _, err := Run(ctx, Options{
		Args: []string{"-p", "ask the server", "--mcp-config", config,
			"--strict-mcp-config", "--allowedTools", "mcp__stub__report"},
		Env: env, Cwd: cwd, Stdout: &stdout, Stderr: &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The tool result's output is a list of parts, not a string. Read it as one.
	offered, result := false, ""
	for _, request := range script.Requests() {
		var body struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Input []struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output []struct {
					Text string `json:"text"`
				} `json:"output"`
			} `json:"input"`
		}
		_ = json.Unmarshal([]byte(request), &body)
		for _, tool := range body.Tools {
			if tool.Name == "mcp__stub__report" {
				offered = true
			}
		}
		for _, entry := range body.Input {
			if entry.Type != "function_call_output" || entry.CallID != "call_mcp_1" {
				continue
			}
			for _, part := range entry.Output {
				result += part.Text
			}
		}
	}

	if !offered {
		t.Fatalf("the server's tool was never offered to the model.\nstderr: %s",
			tail(stderr.String(), 1500))
	}
	if result == "" {
		t.Fatalf("the tool call never came back.\nstderr: %s", tail(stderr.String(), 1500))
	}
	// The measurement the baseline fails. ABSENT here would mean this launcher strips the
	// server's credential the way the old one did.
	if !strings.Contains(result, "PRESENT:the-servers-own-credential") {
		t.Fatalf("the server did not get its own environment: %s", result)
	}
}
