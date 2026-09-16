package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// G5, the rest of it. Every one of these options the Node baseline refused, and the reason
// this redesign exists is that they should work. Until now the evidence was that they reach
// the child's argv, which is not the same claim.

// scriptedSession runs one scripted session and returns the requests the backend saw.
func scriptedSession(t *testing.T, env map[string]string, cwd string, args []string,
	replies ...string) (*upstream.Script, string, string) {
	t.Helper()
	exe := nativeAvailable(t)

	turns := make([]upstream.ScriptTurn, 0, len(replies))
	for _, reply := range replies {
		turns = append(turns, upstream.ScriptTurn{
			When: upstream.Conversation, SSE: textStream("turn", reply)})
	}
	script := &upstream.Script{Turns: turns, Default: textStream("side", "untitled")}

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()
	if _, err := Run(ctx, Options{
		Args: append(args, "--strict-mcp-config"), Env: env, Cwd: cwd,
		Stdout: &stdout, Stderr: &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(script) },
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return script, stdout.String(), stderr.String()
}

// conversations is the text of every request that carried tool definitions.
func conversations(t *testing.T, script *upstream.Script) []string {
	t.Helper()
	var out []string
	for _, request := range script.Requests() {
		var body struct {
			Tools []struct{} `json:"tools"`
		}
		_ = json.Unmarshal([]byte(request), &body)
		if len(body.Tools) > 0 {
			out = append(out, request)
		}
	}
	return out
}

// ARG: --resume carries a conversation across two runs of the launcher.
//
// The strongest of these, because it is the one where the bridge could lose something
// without anybody noticing: the second session has to arrive carrying the first one's turns.
func TestResumeCarriesTheConversationAcross(t *testing.T) {
	env := isolatedEnv(t)
	_, cwd := workspace(t)

	const marker = "PINEAPPLE-QUARTZ"
	scriptedSession(t, env, cwd, []string{"-p", "remember the word " + marker}, "noted")

	id := latestSession(t, env["CLAUDE_CONFIG_DIR"])
	if id == "" {
		t.Fatal("the first session left no transcript to resume")
	}

	script, _, stderr := scriptedSession(t, env, cwd,
		[]string{"-p", "what was the word", "--resume", id}, "it was the word")

	joined := strings.Join(conversations(t, script), "\n")
	if joined == "" {
		t.Fatalf("the resumed session sent no conversation.\nstderr: %s", tail(stderr, 1200))
	}
	if !strings.Contains(joined, marker) {
		t.Fatalf("the resumed session did not carry the first one's turn.\nstderr: %s",
			tail(stderr, 1200))
	}
}

// latestSession is the id of the most recently written transcript.
func latestSession(t *testing.T, configDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(configDir, "projects", "*", "*.jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		left, _ := os.Stat(matches[i])
		right, _ := os.Stat(matches[j])
		return left.ModTime().Before(right.ModTime())
	})
	return strings.TrimSuffix(filepath.Base(matches[len(matches)-1]), ".jsonl")
}

// ARG: --permission-mode changes what the session is, not just what it was passed.
func TestPermissionModeChangesTheSession(t *testing.T) {
	env := isolatedEnv(t)
	_, cwd := workspace(t)

	plain, _, _ := scriptedSession(t, env, cwd, []string{"-p", "hello"}, "ok")
	planned, _, stderr := scriptedSession(t, isolatedEnv(t), cwd,
		[]string{"-p", "hello", "--permission-mode", "plan"}, "ok")

	before := strings.Join(conversations(t, plain), "\n")
	after := strings.Join(conversations(t, planned), "\n")
	if before == "" || after == "" {
		t.Fatalf("no conversation to compare.\nstderr: %s", tail(stderr, 1200))
	}
	if before == after {
		t.Fatal("the session was identical with and without --permission-mode plan; the " +
			"option reached the child and changed nothing")
	}
	// Named rather than merely different: a difference could be a timestamp.
	if !strings.Contains(strings.ToLower(after), "plan mode") {
		t.Errorf("the session does not mention plan mode.\nstderr: %s", tail(stderr, 1200))
	}
}

// ARG: --worktree puts the session in a git worktree, which is a directory that has to exist.
func TestWorktreeActuallyCreatesOne(t *testing.T) {
	env := isolatedEnv(t)
	_, cwd := workspace(t)

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"-c", "user.email=t@example.invalid", "-c", "user.name=t", "commit",
			"--allow-empty", "-m", "first"},
	} {
		command := exec.Command("git", args...)
		command.Dir = cwd
		if out, err := command.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v\n%s", err, out)
		}
	}

	_, _, stderr := scriptedSession(t, env, cwd, []string{"-p", "hello", "--worktree", "g5probe"}, "ok")

	listing := exec.Command("git", "worktree", "list")
	listing.Dir = cwd
	out, err := listing.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "g5probe") {
		t.Fatalf("no worktree was created.\ngit worktree list:\n%s\nstderr: %s",
			out, tail(stderr, 1500))
	}
}

// ARG: --plugin-dir puts a plugin's skill in front of the model.
//
// A plugin that loads and offers nothing is indistinguishable from one that did not load, so
// the assertion is that the skill is named in what the session actually sends.
func TestAPluginsSkillReachesTheSession(t *testing.T) {
	dir := t.TempDir()
	const plugin, skill = "g5plugin", "quartzwork"
	root := filepath.Join(dir, plugin)
	for _, sub := range []string{".claude-plugin", filepath.Join("skills", skill)} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest, _ := json.Marshal(map[string]any{
		"name": plugin, "version": "0.0.1", "description": "A plugin for measuring one thing.",
	})
	files := map[string]string{
		filepath.Join(root, ".claude-plugin", "plugin.json"): string(manifest),
		filepath.Join(root, "skills", skill, "SKILL.md"): "---\nname: " + skill +
			"\ndescription: Measures that a plugin directory was actually loaded.\n---\n\nDo nothing.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	script, _, stderr := scriptedSession(t, isolatedEnv(t), mustWorkspace(t),
		[]string{"-p", "hello", "--plugin-dir", dir}, "ok")

	joined := strings.Join(conversations(t, script), "\n")
	if joined == "" {
		t.Fatalf("no conversation was sent.\nstderr: %s", tail(stderr, 1500))
	}
	if !strings.Contains(joined, skill) {
		t.Fatalf("the plugin's skill never reached the session.\nstderr: %s", tail(stderr, 1500))
	}
}

func mustWorkspace(t *testing.T) string {
	t.Helper()
	_, cwd := workspace(t)
	return cwd
}
