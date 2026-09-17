package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// G8 is about the thing that ships rather than the thing that compiles. These tests build
// the real binaries and ask what they are, what they need, and what they leave behind.

// buildProduct builds a command into dir and returns the path. The flags are the release
// flags: -trimpath so the binary carries no build machine's directory layout.
func buildProduct(t *testing.T, dir, command string) string {
	t.Helper()
	out := filepath.Join(dir, command+".exe")
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, "./cmd/"+command)
	cmd.Dir = moduleRoot(t)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", command, err, combined)
	}
	return out
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// REL05: the same source produces the same binary.
//
// Provenance that stops at "it says it was built from this commit" is a claim the binary
// makes about itself. Two builds hashing the same is a claim anyone can check, and it is
// what makes a published checksum mean anything.
func TestTheBuildIsReproducible(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the product twice")
	}
	first := sha256File(t, buildProduct(t, t.TempDir(), "clauduct"))
	second := sha256File(t, buildProduct(t, t.TempDir(), "clauduct"))
	if first != second {
		t.Fatalf("two builds of the same source differ:\n  %s\n  %s\n"+
			"A published checksum means nothing if the build is not reproducible.",
			first, second)
	}
}

// REL05: the binary says what it is, and says it accurately.
func TestTheBinaryIdentifiesItself(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the product")
	}
	dev := buildProduct(t, t.TempDir(), "clauduct-dev")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, dev, "version").Output()
	if err != nil {
		t.Fatalf("clauduct-dev version: %v", err)
	}
	printed := string(raw)

	// A commit, stamped by the toolchain rather than by a release script that might forget.
	commit := regexp.MustCompile(`commit\s+([0-9a-f]{40})`).FindStringSubmatch(printed)
	if commit == nil {
		t.Fatalf("no commit stamp:\n%s", printed)
	}
	if !strings.Contains(printed, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("no target in the identity:\n%s", printed)
	}

	// The commit has to be this worktree's, not one left over from somewhere.
	head := gitHead(t)
	if commit[1] != head {
		t.Fatalf("the binary claims %s but HEAD is %s", commit[1], head)
	}

	// Whether the worktree was clean is itself part of the identity, and the check for it
	// used to be a skip placed above everything else -- so the test stood down in exactly
	// the situation the declaration exists for. Now it asserts both ways.
	dirty := strings.Contains(printed, "+dirty")
	if worktreeModified(t) != dirty {
		t.Fatalf("the binary says dirty=%t while the worktree says %t. A release record "+
			"that hides uncommitted changes names a commit that does not contain them.",
			dirty, worktreeModified(t))
	}
	if dirty {
		t.Log("this build is from a modified worktree and is not a release candidate")
	}
}

func gitHead(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = moduleRoot(t)
	raw, err := cmd.Output()
	if err != nil {
		t.Skipf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(raw))
}

// REL04: nothing credential-shaped ships inside the binary.
//
// The synthetic tokens these tests use are JWT-shaped on purpose, so their absence from the
// product binary is worth confirming rather than assuming: test files do not link into a
// binary, but a constant moved into a product file one day would.
func TestNoCredentialShapedStringShipsInAnyBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the product")
	}

	// Both of them. The package ships two binaries, and the first version of this test
	// scanned only clauduct -- which does not even import buildinfo, so a mutation that
	// put a token in a shipped constant survived by landing in the half nobody looked at.
	// clauduct-dev is also the one that reads a credential.
	dir := t.TempDir()
	for _, command := range []string{"clauduct", "clauduct-dev"} {
		t.Run(command, func(t *testing.T) {
			raw, err := os.ReadFile(buildProduct(t, dir, command))
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			// A JWT's own signature rather than its general shape. Every one of these
			// tokens begins eyJ, which is base64 for the opening of a JSON object, and
			// both of the first two segments do.
			//
			// The first version of this pattern asked only for three long base64url runs
			// joined by dots, and matched 331 bytes of Go's string table -- unicode
			// category names running into "Content-Type" running into "gpt-5.6-luna".
			// Constants pack adjacently in a binary, so any pattern loose enough to span
			// them finds something in every Go program, and a check that fires on every
			// build is one somebody disables.
			jwt := regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.`)
			if found := jwt.Find(raw); found != nil {
				t.Fatalf("a JWT is compiled into %s: %d bytes beginning %q",
					command, len(found), found[:12])
			}
			// Named markers for credential-shaped test material that has no business in a
			// shipped binary. SYNTHETIC-NOT-VERIFIED is the signature segment of the auth
			// tests' fake JWT; the others are real credential prefixes.
			//
			// BUILD-TOKEN was on this list and came off it. Widening the scan to both
			// binaries found it in clauduct-dev, correctly: probeToolResult lives in a
			// product file, so it does ship. But it is a made-up value the probe asks the
			// model to repeat back, with no secrecy meaning at all -- it was the marker
			// that was wrong, not the binary. Recorded rather than quietly deleted,
			// because removing an assertion that just fired is how a check stops checking.
			for _, marker := range []string{"SYNTHETIC-NOT-VERIFIED", "sk-ant-", "sk-proj-"} {
				if bytes.Contains(raw, []byte(marker)) {
					t.Fatalf("%s contains %q", command, marker)
				}
			}
		})
	}
}

// REL06: installed anywhere, and it writes nothing where it was installed.
//
// A binary that needs to write beside itself cannot live in a read-only or shared install
// directory. Checking the directory rather than the file permissions is what makes this
// meaningful on Windows, where a read-only flag on a directory does not stop writes.
func TestTheInstallDirectoryIsNotWrittenTo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the product")
	}
	install := filepath.Join(t.TempDir(), "Program Files", "Clauduct")
	if err := os.MkdirAll(install, 0o700); err != nil {
		t.Fatalf("install dir: %v", err)
	}
	exe := buildProduct(t, install, "clauduct")
	before := tree(t, install)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--version")
	// Run from somewhere else entirely, so nothing resolves relative to the install.
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+t.TempDir())
	raw, err := cmd.CombinedOutput()

	// Exit status is not the question. A machine without claude.exe answers CLAUDE_NOT_FOUND
	// and exits 1, which is correct -- and is the stronger case for this test, because the
	// binary still must not have written anything. CI found this: the runner has no client
	// installed and the first version of the test demanded exit 0.
	//
	// What would matter is the binary failing to run at all, which shows up as no output.
	if err != nil && len(bytes.TrimSpace(raw)) == 0 {
		t.Fatalf("the binary produced nothing when run from %s: %v", install, err)
	}

	if appeared := added(before, tree(t, install)); len(appeared) != 0 {
		t.Fatalf("running the binary wrote %v into its own install directory", appeared)
	}
}

// LIFE14: sessions in a row must not accumulate anything.
//
// Measured by counting goroutines, which is the leak this design could actually have: every
// session starts a listener, a registry and per-request contexts, and a release path that
// missed one would show as a count that climbs.
func TestRepeatedSessionsDoNotAccumulateGoroutines(t *testing.T) {
	exe := nativeAvailable(t)

	run := func() {
		fixture := &upstream.Fixture{SSE: measuredStream("ok")}
		ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
		defer cancel()
		var out, errOut bytes.Buffer
		_, err := Run(ctx, Options{
			Args:          []string{"-p", "say ok", "--strict-mcp-config"},
			Env:           isolatedEnv(t),
			Cwd:           t.TempDir(),
			Stdout:        &out,
			Stderr:        &errOut,
			ResolveClaude: func() (string, bool, error) { return exe, true, nil },
			StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) },
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	}

	// One first, so the settled baseline excludes whatever a first run initialises once.
	run()
	settle()
	before := runtime.NumGoroutine()

	const sessions = 4
	for i := 0; i < sessions; i++ {
		run()
	}
	settle()
	after := runtime.NumGoroutine()

	// Not zero growth: the runtime moves on its own and a test demanding an exact number
	// would fail for reasons that are not leaks. One goroutine per session is the shape a
	// real leak has, and this is well under it.
	if after > before+2 {
		t.Fatalf("%d sessions took goroutines from %d to %d. A session that leaks one "+
			"goroutine looks exactly like this.", sessions, before, after)
	}
}

// settle gives release paths a moment to finish. They are asynchronous by design -- a
// session returns when the child has exited and its own resources are released, and the
// runtime reclaims on its own schedule.
func settle() {
	for i := 0; i < 20; i++ {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	runtime.GC()
}

// REL07: sessions run side by side without noticing each other.
//
// Each one binds its own ephemeral port and mints its own session token, so the thing that
// would break is shared state rather than contention. Running them at once is the only way
// to find out; running them in sequence would pass whatever the answer is.
func TestConcurrentSessionsDoNotInterfere(t *testing.T) {
	exe := nativeAvailable(t)

	const sessions = 3
	type result struct {
		addr   string
		stdout string
		err    error
	}
	results := make(chan result, sessions)

	for i := 0; i < sessions; i++ {
		go func() {
			fixture := &upstream.Fixture{SSE: measuredStream("ok")}
			ctx, cancel := context.WithTimeout(context.Background(), defaultNativeTimeout)
			defer cancel()
			var out, errOut bytes.Buffer
			r, err := Run(ctx, Options{
				Args:          []string{"-p", "say ok", "--strict-mcp-config"},
				Env:           isolatedEnv(t),
				Cwd:           t.TempDir(),
				Stdout:        &out,
				Stderr:        &errOut,
				ResolveClaude: func() (string, bool, error) { return exe, true, nil },
				StartGateway:  func() (*gateway.Gateway, error) { return gateway.Start(fixture) },
			})
			if err == nil && r.NativeExitCode != 0 {
				err = fmt.Errorf("exit %d: %s%s", r.NativeExitCode, out.String(), errOut.String())
			}
			results <- result{addr: r.GatewayAddr, stdout: out.String(), err: err}
		}()
	}

	addresses := map[string]bool{}
	for i := 0; i < sessions; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("a concurrent session failed: %v", got.err)
		}
		if !strings.Contains(got.stdout, "ok") {
			t.Fatalf("a concurrent session lost its reply: %q", got.stdout)
		}
		if addresses[got.addr] {
			t.Fatalf("two sessions reported the same listener %s; they are sharing a port",
				got.addr)
		}
		addresses[got.addr] = true
	}
}

// worktreeModified reports modified in the sense the Go toolchain uses.
//
// That includes untracked files. The first version of this passed --untracked-files=no and
// then compared the answer against the toolchain's stamp, which counts them -- so writing a
// new file made the binary say dirty while this said clean, and the test failed for a
// disagreement about the word rather than about the worktree. Two definitions of the same
// term is how a comparison becomes noise.
func worktreeModified(t *testing.T) bool {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = moduleRoot(t)
	raw, err := cmd.Output()
	if err != nil {
		t.Skipf("git status: %v", err)
	}
	return strings.TrimSpace(string(raw)) != ""
}

// G8: what ships is built without cgo, and the builder's environment says so.
//
// Nothing in this module imports "C", so on Windows this changes no behaviour today -- net
// and os/user reach the platform through syscalls either way. What it changes is who the
// artifact depends on: CGO_ENABLED defaults to 1 wherever a C toolchain happens to exist,
// so a runner with gcc would build, test and possibly publish different bytes than a
// machine without one, and TestTheBuildIsReproducible would not notice because it compares
// two builds from the same environment.
//
// The build here deliberately inherits the environment rather than setting the variable
// itself. A test that pins the value it then asserts agrees only with itself; this one
// fails on any machine or runner whose environment does not already produce a cgo-free
// build, which is exactly where the pin has to live.
func TestTheShippedBinaryIsBuiltWithoutCgo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the product")
	}
	if raceDetector {
		// CI runs this suite twice and the second run turns cgo on for the detector. That
		// job's binary is not a release candidate, so asking it to be cgo-free is asking
		// the wrong question -- and it failed on the first CI run that saw it.
		t.Skip("the race detector requires cgo; the binary this run builds is not the one that ships")
	}
	exe := buildProduct(t, t.TempDir(), "clauduct")
	info, err := buildinfo.ReadFile(exe)
	if err != nil {
		t.Fatalf("read build info from %s: %v", exe, err)
	}
	for _, setting := range info.Settings {
		if setting.Key != "CGO_ENABLED" {
			continue
		}
		if setting.Value != "0" {
			t.Fatalf("the built binary records CGO_ENABLED=%s.\n"+
				"Set CGO_ENABLED=0 for builds and tests; the race job is the one exception, "+
				"and the binary it produces is not the one that ships.", setting.Value)
		}
		return
	}
	t.Fatal("no CGO_ENABLED setting recorded in the binary")
}
