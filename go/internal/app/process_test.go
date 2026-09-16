package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// LIFE11, LIFE12 and LIFE17: what a session owns, and what it leaves behind.
//
// The handoff is explicit that these are measured and reported rather than engineered
// around: clean only the tree this run owns, never every process named claude or codex, and
// do not take the child's Wait as proof that its grandchildren are gone.
//
// Every process here is one these tests started, and everything is addressed by process id.
// Nothing looks a process up by name: this machine had three of the user's own claude.exe
// running while these were written, one of them the session writing them, and a test that
// killed by name would have found all three.

// sleeperArgs runs a harmless long process. `cmd /c ping` is a two-level tree for free --
// cmd is the child and ping is its child -- which is exactly what LIFE11 asks about.
var sleeperArgs = []string{"/c", "ping", "-n", "120", "127.0.0.1"}

// childrenOf lists the process ids whose parent is pid. Recorded before anything is killed:
// once a parent dies its children are reparented and asking afterwards answers a different
// question.
func childrenOf(t *testing.T, pid int) []int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		"(Get-CimInstance Win32_Process -Filter 'ParentProcessId="+strconv.Itoa(pid)+
			"').ProcessId").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if n, convErr := strconv.Atoi(field); convErr == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

// alive reports whether a process id is still running, through tasklist rather than a
// syscall so this module keeps its zero dependencies.
func alive(t *testing.T, pid int) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tasklist", "/FI",
		"PID eq "+strconv.Itoa(pid), "/NH", "/FO", "CSV").Output()
	if err != nil {
		t.Skipf("tasklist: %v", err)
	}
	return strings.Contains(string(out), `"`+strconv.Itoa(pid)+`"`)
}

// killTree ends a process and its descendants, by process id.
//
// Test cleanup only. /T is scoped to the tree below the id it is given, so it cannot reach
// anything this run did not start, and leaving a sleeper behind on someone's machine is not
// an acceptable cost of a test.
func killTree(pid int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// settleProcess gives Windows a moment to finish tearing a process down. Killing is not
// instantaneous and asking immediately reads the state before the kill landed.
func settleProcess() { time.Sleep(800 * time.Millisecond) }

// decoy stands in for something the user has running: another terminal's client, an MCP
// server, an app they started themselves.
func decoy(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("cmd", sleeperArgs...)
	if err := cmd.Start(); err != nil {
		t.Skipf("starting a decoy: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		killTree(pid)
		_ = cmd.Wait()
	})
	return pid
}

// runOwning starts a session whose child is a real process this package spawned.
//
// Deliberately not an injected Process. A stub brings its own Stop, so a session driven that
// way never runs the product's cleanup at all -- which is how two mutations of
// osProcess.Stop survived while these tests passed. onStart runs in the session's own
// goroutine right after the spawn, so a caller can record the tree before the deadline
// expires.
func runOwning(ctx context.Context, t *testing.T, onStart func(pid int)) (Result, error) {
	t.Helper()
	return runOwningCommand(ctx, t, "cmd", sleeperArgs, onStart)
}

// runOwningCommand is the same with the subject chosen.
//
// Which subject matters more than it looks. `cmd /c ping` leaves a grandchild holding the
// pipes the child inherited, so Wait does not return when the child is killed -- the
// cancellation path then ends in the stop grace rather than in the deadline, and a test
// asserting only "some error" cannot tell the two apart. A subject with no children of its
// own exercises the clean path.
func runOwningCommand(ctx context.Context, t *testing.T, command string, args []string,
	onStart func(pid int)) (Result, error) {
	t.Helper()
	shell, err := exec.LookPath(command)
	if err != nil {
		t.Skipf("%s: %v", command, err)
	}

	pid := 0
	t.Cleanup(func() {
		if pid != 0 {
			killTree(pid)
		}
	})

	// Bounded. A session whose child never exits is exactly what these tests set up, so if
	// Run stops honouring its context this blocks forever -- and a suite that hangs has not
	// failed, it has said nothing. Mutating waitFor back to a bare Wait used to do that;
	// now it fails here.
	type finished struct {
		result Result
		err    error
	}
	done := make(chan finished, 1)
	go func() {
		result, runErr := runOwnedSession(ctx, t, shell, args, &pid, onStart)
		done <- finished{result, runErr}
	}()

	deadline, ok := ctx.Deadline()
	watchdog := 30 * time.Second
	if ok {
		watchdog = time.Until(deadline) + 30*time.Second
	}
	select {
	case got := <-done:
		return got.result, got.err
	case <-time.After(watchdog):
		t.Fatalf("Run did not return within %v. The child never exits on its own, so the "+
			"context is not reaching it.", watchdog)
		return Result{}, nil
	}
}

func runOwnedSession(ctx context.Context, t *testing.T, shell string, args []string,
	pid *int, onStart func(pid int)) (Result, error) {
	return Run(ctx, Options{
		Settings: &noSettings,
		Args:     args,
		Env:      isolatedEnv(t),
		// Not a t.TempDir: a running process holds its working directory open, and the
		// framework's own removal would race the kill and fail.
		Cwd:           os.TempDir(),
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		ResolveClaude: func() (string, bool, error) { return shell, true, nil },
		// The real one, wrapped only to learn the process id. Wrapping rather than
		// replacing keeps osProcess -- and its Stop -- in the path under test.
		StartProcess: func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error) {
			process, startErr := startOSProcess(spec, stdin, stdout, stderr)
			if startErr != nil {
				return nil, startErr
			}
			if p, ok := process.(*osProcess); ok && p.cmd.Process != nil {
				*pid = p.cmd.Process.Pid
				if onStart != nil {
					onStart(*pid)
				}
			}
			return process, nil
		},
	})
}

// LIFE12: ending a session touches nothing it did not start.
//
// Nothing in this package looks a process up by name, so this passes structurally today. It
// is written down so that it stops passing the moment cleanup reaches further than the
// handle it was given.
func TestEndingASessionLeavesOtherProcessesAlone(t *testing.T) {
	decoys := []int{decoy(t), decoy(t)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ownPID := 0
	_, err := runOwning(ctx, t, func(pid int) { ownPID = pid })
	if err == nil {
		t.Fatal("a session against a process that never exits returned cleanly")
	}
	if ownPID == 0 {
		t.Fatal("the session never started a child")
	}
	settleProcess()

	// Its own child is ended: that part is the session working.
	if alive(t, ownPID) {
		t.Fatalf("the session's own child (pid %d) outlived it", ownPID)
	}
	for _, pid := range decoys {
		if !alive(t, pid) {
			t.Fatalf("a process this session did not start (pid %d) was killed. Cleanup "+
				"reached past the handle it was given.", pid)
		}
	}
}

// LIFE11: what happens to a grandchild, measured rather than assumed.
//
// The handoff says not to take the child's Wait as proof that its grandchildren are gone,
// and this is why. Killing a process on Windows kills that process; its children are not in
// the handle and do not die with it.
//
// The expected result is a failure of cleanup and the test asserts it. That is deliberate:
// the limit is real, the Node baseline has the same one -- clauduct.mjs:261 is a bare
// child.kill() -- and a test that pretended otherwise would be the claim rather than the
// check. If the tree ever does die together this test fails, and rewriting it is the work.
func TestAGrandchildOutlivesTheSessionThatStartedIt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ownPID := 0
	var grandchildren []int
	_, err := runOwning(ctx, t, func(pid int) {
		ownPID = pid
		// In the session's own goroutine, right after the spawn: the child needs a moment
		// to start its own, and the record has to exist before anything is killed.
		for i := 0; i < 20 && len(grandchildren) == 0; i++ {
			time.Sleep(100 * time.Millisecond)
			grandchildren = childrenOf(t, pid)
		}
	})
	if err == nil {
		t.Fatal("a session against a process that never exits returned cleanly")
	}
	if ownPID == 0 {
		t.Fatal("the session never started a child")
	}
	if len(grandchildren) == 0 {
		t.Skip("the child spawned nothing of its own; nothing to measure")
	}
	settleProcess()

	if alive(t, ownPID) {
		t.Fatalf("the session's own child (pid %d) outlived it", ownPID)
	}

	var survivors []int
	for _, pid := range grandchildren {
		if alive(t, pid) {
			survivors = append(survivors, pid)
		}
	}
	if len(survivors) == 0 {
		t.Fatalf("every grandchild died with the child. Something now binds the tree, and "+
			"the limit this test records is no longer the limit -- rewrite it to require "+
			"the behaviour instead of recording its absence. grandchildren=%v", grandchildren)
	}
	t.Logf("measured: %d of %d grandchildren outlived the session (%v). Ending a session "+
		"ends the process it started and nothing below it.",
		len(survivors), len(grandchildren), survivors)
}

// A child that will not go must not hold the caller past its deadline.
//
// Found by mutating Stop into a no-op and watching the suite hang rather than fail: waitFor
// reaped unconditionally, so a Stop that did not work blocked for as long as the child chose
// to live. The deadline that got us into that branch had already passed.
func TestAChildThatWillNotStopIsReportedRatherThanWaitedOn(t *testing.T) {
	cmd := exec.Command("cmd", sleeperArgs...)
	if err := cmd.Start(); err != nil {
		t.Skipf("starting a test process: %v", err)
	}
	pid := cmd.Process.Pid
	// No Wait here. The session's own wait is still running on this Cmd -- that is the
	// whole point of a child that will not stop -- and two concurrent Wait calls on one Cmd
	// is a data race, which is what the race detector reported.
	t.Cleanup(func() { killTree(pid) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	started := time.Now()
	result, err := Run(ctx, Options{
		Settings:      &noSettings,
		Args:          []string{"x"},
		Env:           isolatedEnv(t),
		Cwd:           os.TempDir(),
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		ResolveClaude: func() (string, bool, error) { return "unused", true, nil },
		StartProcess: func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error) {
			return stubbornProcess{cmd: cmd}, nil
		},
	})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("a child that never stopped was reported as a clean session")
	}
	if !strings.Contains(err.Error(), "did not exit") {
		t.Fatalf("err = %v, want it to say the child did not exit", err)
	}
	// Bounded by the deadline plus the grace, not by the child's lifetime.
	if elapsed > 2*time.Second+stopGrace {
		t.Fatalf("took %v; the caller was held by a child that would not go", elapsed)
	}

	// And the exit code is unknown rather than zero. The wait is still running, so there is
	// no status to report: reading one is the data race the detector found in CI, and zero
	// would read as a session that finished successfully.
	if result.NativeExitCode != ExitCodeUnknown {
		t.Fatalf("NativeExitCode = %d for a child that was never reaped, want %d",
			result.NativeExitCode, ExitCodeUnknown)
	}
}

// stubbornProcess reports that it stopped and does not. It is what a Stop that silently
// fails looks like: a permission denial, a security product intervening, a handle that no
// longer controls what it names.
type stubbornProcess struct{ cmd *exec.Cmd }

func (p stubbornProcess) Wait() error   { return p.cmd.Wait() }
func (p stubbornProcess) ExitCode() int { return -1 }
func (p stubbornProcess) Stop() error   { return nil }

// LIFE17: what survives the launcher being killed outright.
//
// Not a graceful exit and not Ctrl+C -- those reach the child through the console, which is
// why SysProcAttr is left alone. This is the launcher process being terminated with a
// session in flight.
//
// It needs the real binary, and holding a session open costs one request: the client exits
// immediately when stdin is not a terminal and no prompt is given, so there is no free way
// to have one running. Gated on CLAUDUCT_LIVE for that reason.
func TestWhatSurvivesTheLauncherBeingKilled(t *testing.T) {
	if os.Getenv("CLAUDUCT_LIVE") != "1" {
		t.Skip("holds a real session open: set CLAUDUCT_LIVE=1")
	}
	nativeAvailable(t)

	exe := buildProduct(t, t.TempDir(), "clauduct-go")
	launcher := exec.Command(exe, "-p", "Count from 1 to 300, one number per line.",
		"--strict-mcp-config")
	launcher.Dir = os.TempDir()
	launcher.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+t.TempDir())
	if err := launcher.Start(); err != nil {
		t.Fatalf("starting the launcher: %v", err)
	}
	launcherPID := launcher.Process.Pid

	var owned []int
	t.Cleanup(func() {
		for _, pid := range owned {
			killTree(pid)
		}
		killTree(launcherPID)
		_ = launcher.Wait()
	})

	// Let it get as far as a running child with a request in flight. The child is found
	// through the launcher's own process id, never by name.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if owned = childrenOf(t, launcherPID); len(owned) > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(owned) == 0 {
		t.Fatal("the launcher never started a child; there is nothing to measure")
	}
	time.Sleep(2 * time.Second)

	// Killed, not asked. This is the abnormal end LIFE17 is about.
	if err := launcher.Process.Kill(); err != nil {
		t.Fatalf("killing the launcher: %v", err)
	}
	_ = launcher.Wait()
	settleProcess()

	if alive(t, launcherPID) {
		t.Fatalf("the launcher (pid %d) survived being killed", launcherPID)
	}

	var survivors []int
	for _, pid := range owned {
		if alive(t, pid) {
			survivors = append(survivors, pid)
		}
	}

	// Recorded, not demanded. Windows does not reap a child when its parent dies, so the
	// native client outliving an abruptly killed launcher is the operating system rather
	// than a defect here, and the Node baseline behaves the same way. What matters is that
	// it is written down: a user who ends clauduct-go from Task Manager is left with a
	// client whose gateway has gone, and nothing in this build cleans that up.
	t.Logf("after killing the launcher: %d of %d owned processes still running (%v)",
		len(survivors), len(owned), survivors)
	if len(survivors) == 0 {
		t.Log("nothing survived. Something now ends the tree with the parent -- the console, " +
			"a job object, or the environment -- and this record needs rewriting to require it.")
	}
}

// noSettings is the empty settings blob, for a subject that is not the native client.
var noSettings = ""
