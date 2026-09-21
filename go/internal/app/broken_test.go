package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// A real session whose upstream drops mid-answer, reported the way the product reports it.
//
// Measured in the wild 2026-09-18: a stream broke after its status was already sent, the
// account was printed because that is noteworthy, and the line above it said refused=0 and
// nothing else. The counters could not carry it -- the client had received a 200 -- so the
// line reported the one that was fine and omitted the one that was not.
//
// That reading came from a session that happened to break. A backend drops when it drops, so
// waiting for another is not a test; this makes one. The fixture delivers a complete-looking
// body and then fails the read, which is the shape that produced TRUNCATED_STREAM, and the
// whole product path runs on it: real client, real gateway, real Report.
func TestABrokenStreamReachesTheExitLine(t *testing.T) {
	exe := nativeAvailable(t)
	// The hook, so a missing one is not what makes this session noteworthy. Without it the
	// account is printed either way and the assertion below proves nothing about the break.
	buildHook(t)

	_, cwd := workspace(t)
	fixture := &upstream.Fixture{
		SSE:     measuredStream("ok"),
		ReadErr: errors.New("connection reset by peer"),
	}
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*defaultNativeTimeout)
	defer cancel()

	result, err := Run(ctx, Options{
		Args:          []string{"-p", "hi", "--strict-mcp-config", "--bare"},
		Env:           withoutEffortNames(isolatedEnv(t)),
		Cwd:           cwd,
		Stdout:        &stdout,
		Stderr:        &stderr,
		ResolveClaude: func() (string, bool, error) { return exe, true, nil },
		StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Fatal rather than skipped. The fixture truncates every time, so a session with none
	// means the break stopped being noticed -- which is the failure this whole counter
	// exists to report, and skipping past it would file that as a pass.
	if result.Diagnostics.Requests.Broken == 0 {
		t.Fatalf("the upstream dropped every read and the account saw no broken stream "+
			"(received=%d, refused=%d)", result.Diagnostics.Requests.Received,
			result.Diagnostics.Requests.Refused)
	}

	var reportOut bytes.Buffer
	Report(result, &reportOut, nil)
	line, _, _ := strings.Cut(reportOut.String(), "\n")
	if !strings.Contains(line, "broken=") {
		t.Fatalf("%d streams broke and the exit line does not say so:\n%s",
			result.Diagnostics.Requests.Broken, line)
	}
	// And it is still printed whole, because a broken stream is worth the detail. With the
	// hook installed and nothing refused, the break is the only thing that can have made
	// this session noteworthy.
	if !result.HookInstalled {
		t.Fatalf("the hook was not found, so this session is noteworthy for another reason")
	}
	if !strings.Contains(reportOut.String(), "CLAUDUCT_REQUEST_STATUS {") {
		t.Errorf("the account was not printed for a session with a broken stream")
	}
}
