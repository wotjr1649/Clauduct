package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// The agent view handover (#319 decision 18h). Native keeps an agent view session and the
// jobs dispatched from it running after its terminal closes, and they all use this launch's
// gateway. The terminal waits for the process it started, so for an interactive launch that
// process is a launcher: the launch runs in a child on the same console, and the launcher
// exits with native's code once the child has either ended or handed its gateway over
// (Options.Handover) and left the console.

const handoverEnv = "CLAUDUCT_AGENT_VIEW_HANDOVER"

var freeConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("FreeConsole")

// HandoverLaunch reports whether an interactive session goes through the launcher.
func HandoverLaunch(args []string) bool {
	return !BackgroundRequested(args) && !printMode(args) && nativeControl(args) == "" && !nativeInformation(args)
}

// LaunchHandover runs this executable again with args on this console and returns the exit
// code the terminal should see.
func LaunchHandover(args []string) int {
	self, err := os.Executable()
	if err != nil {
		return 1
	}
	r, w, err := os.Pipe()
	if err != nil {
		return 1
	}
	defer r.Close()
	h := syscall.Handle(w.Fd())
	if syscall.SetHandleInformation(h, syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT) != nil {
		w.Close()
		return 1
	}
	// Ctrl+C reaches every process on the console; the child answers it, this one waits.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for range interrupts {
		}
	}()
	start := func(flags uint32) (*exec.Cmd, error) {
		cmd := exec.Command(self, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), handoverEnv+"="+strconv.FormatUint(uint64(h), 10))
		cmd.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{h}, CreationFlags: flags}
		return cmd, cmd.Start()
	}
	// Out of the terminal's job when it allows that: the child may outlive this process.
	cmd, err := start(0x01000000)
	if err != nil {
		cmd, err = start(0)
	}
	w.Close()
	if err != nil {
		return 1
	}
	line, err := bufio.NewReader(r).ReadString('\n')
	if err == nil {
		if code, convErr := strconv.Atoi(strings.TrimSpace(line)); convErr == nil {
			return code
		}
	}
	cmd.Wait()
	return cmd.ProcessState.ExitCode()
}

// HandoverFromEnv is the child's side: nil unless the launcher started this process. It must
// run before the environment is read for native, which must not see the variable or the handle.
func HandoverFromEnv() func(nativeExit int) bool {
	value := os.Getenv(handoverEnv)
	os.Unsetenv(handoverEnv)
	if value == "" {
		return nil
	}
	h, err := strconv.ParseUint(value, 10, 64)
	if err != nil || syscall.SetHandleInformation(syscall.Handle(h), syscall.HANDLE_FLAG_INHERIT, 0) != nil {
		return nil
	}
	f := os.NewFile(uintptr(h), "handover")
	return func(nativeExit int) bool {
		_, err := fmt.Fprintf(f, "%d\n", nativeExit)
		f.Close()
		if err != nil {
			return false
		}
		freeConsole.Call()
		return true
	}
}
