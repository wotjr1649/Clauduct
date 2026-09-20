package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// Real OS processes, ready messages and held process handles. No PID/name
// polling after exit and no backend, profile or native trust configuration.
func TestJobHelper(t *testing.T) {
	role := os.Getenv("CLAUDUCT_JOB_HELPER")
	if role == "" {
		return
	}
	exe, _ := os.Executable()
	env := func(role string) []string {
		var out []string
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "CLAUDUCT_JOB_HELPER=") {
				out = append(out, value)
			}
		}
		return append(out, "CLAUDUCT_JOB_HELPER="+role)
	}
	if role == "launcher" {
		p, err := startOSProcess(launch.Spec{File: exe, Args: []string{"-test.run=^TestJobHelper$"}, Env: env("worker")}, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err = p.Wait(); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if role == "worker" {
		child := exec.Command(exe, "-test.run=^TestJobHelper$")
		child.Env = env("leaf")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		fmt.Printf("worker %d\n", os.Getpid())
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		os.Exit(0)
	}
	if role == "leaf" {
		fmt.Printf("leaf %d\n", os.Getpid())
		time.Sleep(20 * time.Second) // bounded even if the ownership implementation breaks
		os.Exit(0)
	}
	os.Exit(5)
}

func TestJobOwnsTreeOnExitStopAndLauncherKill(t *testing.T) {
	for _, ending := range []string{"exit", "stop", "launcher-kill"} {
		t.Run(ending, func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			inR, inW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer inR.Close()
			defer inW.Close()
			outR, outW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer outR.Close()
			defer outW.Close()
			role := "worker"
			if ending == "launcher-kill" {
				role = "launcher"
			}
			spec := launch.Spec{File: exe, Args: []string{"-test.run=^TestJobHelper$"}, Env: append(os.Environ(), "CLAUDUCT_JOB_HELPER="+role)}
			var wait func() error
			var stop func() error
			if ending == "launcher-kill" {
				launcher := exec.Command(exe, spec.Args...)
				launcher.Env = spec.Env
				launcher.Stdin = inR
				launcher.Stdout = outW
				launcher.Stderr = io.Discard
				if err = launcher.Start(); err != nil {
					t.Fatal(err)
				}
				wait = launcher.Wait
				stop = launcher.Process.Kill
			} else {
				p, e := startOSProcess(spec, inR, outW, io.Discard)
				if e != nil {
					t.Fatal(e)
				}
				wait = p.Wait
				stop = p.Stop
			}
			defer stop()
			ready := make(chan []syscall.Handle, 1)
			go func() {
				scanner := bufio.NewScanner(outR)
				var handles []syscall.Handle
				for scanner.Scan() {
					fields := strings.Fields(scanner.Text())
					if len(fields) != 2 {
						continue
					}
					if fields[0] != "worker" && fields[0] != "leaf" {
						continue
					}
					pid, e := strconv.Atoi(fields[1])
					if e != nil {
						break
					}
					h, e := syscall.OpenProcess(syscall.SYNCHRONIZE|syscall.PROCESS_TERMINATE, false, uint32(pid))
					if e != nil {
						break
					}
					handles = append(handles, h)
					if len(handles) == 2 {
						break
					}
				}
				ready <- handles
			}()
			var handles []syscall.Handle
			select {
			case handles = <-ready:
			case <-time.After(10 * time.Second):
				t.Fatal("tree did not become ready")
			}
			defer func() {
				for _, h := range handles {
					_ = syscall.TerminateProcess(h, 9)
					_ = syscall.CloseHandle(h)
				}
			}()
			if len(handles) != 2 {
				t.Fatal("two owned process handles required")
			}
			// A separate real process must survive every termination mechanism.
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			decoy := exec.CommandContext(ctx, exe, "-test.run=^TestJobHelper$")
			decoy.Env = append(os.Environ(), "CLAUDUCT_JOB_HELPER=leaf")
			if err = decoy.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = decoy.Process.Kill(); _ = decoy.Wait() }()
			decoyHandle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(decoy.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.CloseHandle(decoyHandle)
			if ending == "exit" {
				_, err = inW.Write([]byte{1})
			} else {
				err = stop()
			}
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() { finished <- wait() }()
			select {
			case err = <-finished:
				if ending == "exit" && err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("launcher did not finish")
			}
			for _, h := range handles {
				state, e := syscall.WaitForSingleObject(h, 3000)
				if e != nil || state != syscall.WAIT_OBJECT_0 {
					t.Fatalf("owned descendant survived: state=%d error=%v", state, e)
				}
			}
			state, err := syscall.WaitForSingleObject(decoyHandle, 0)
			if err != nil || state != syscall.WAIT_TIMEOUT {
				t.Fatal("unrelated process was terminated")
			}
		})
	}
}
