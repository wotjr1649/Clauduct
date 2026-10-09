package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"io"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"
)

const BackgroundRole = "--clauduct-background"

type BackgroundReply struct {
	Session *BackgroundSession `json:"session,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// StartBackground starts an explicit --bg owner, not a child of the foreground native
// job. CREATE_NO_WINDOW detaches it from the dispatching console; no global
// service, startup entry or process execution policy is installed.
func StartBackground(ctx context.Context, args []string, cwd string, out, errOut io.Writer) error {
	// A refused background resume is said here, before a resident starts and reports only that
	// it failed.
	if _, err := backgroundResume(prepareSessionArgs(args)); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	cmd := exec.Command(self, append([]string{BackgroundRole}, args...)...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = cwd, w, errOut
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		w.Close()
		return err
	}
	w.Close()
	defer cmd.Process.Release()
	type decoded struct {
		reply BackgroundReply
		err   error
	}
	done := make(chan decoded, 1)
	go func() {
		var reply BackgroundReply
		err := json.NewDecoder(io.LimitReader(r, 4096)).Decode(&reply)
		done <- decoded{reply, err}
	}()
	deadline := time.NewTimer(70 * time.Second)
	defer deadline.Stop()
	select {
	case d := <-done:
		if d.err != nil || d.reply.Session == nil {
			waited := make(chan struct{})
			go func() { _ = cmd.Wait(); close(waited) }()
			select {
			case <-waited:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				return errors.New("BACKGROUND_START_UNVERIFIED")
			}
			return errors.New("BACKGROUND_START_FAILED: see native startup diagnostic")
		}
		if d.reply.Session.PID != cmd.Process.Pid || !regexp.MustCompile(`^[a-f0-9]{8}$`).MatchString(d.reply.Session.JobID) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(d.reply.Session.ConnectionID) ||
			d.reply.Session.ForkedFrom != "" && !gateway.IsSessionUUID(d.reply.Session.ForkedFrom) {
			_ = cmd.Process.Kill()
			return errors.New("BACKGROUND_START_UNVERIFIED")
		}
		fmtBackground(out, *d.reply.Session)
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return errors.New("BACKGROUND_START_UNVERIFIED")
	case <-deadline.C:
		_ = cmd.Process.Kill()
		return errors.New("BACKGROUND_START_UNVERIFIED")
	}
}

// RegenerateResident starts the resident for link again when a dead one left its record, and
// waits up to wait for the link to answer (#319 decision 18c). The background key helper calls
// it when the link is gone; on any failure the helper still answers CONNECTION_UNAVAILABLE.
// Like StartBackground it is a detached process of this user: no service, no startup entry.
func RegenerateResident(link string, wait time.Duration) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	record, err := LoadResident(home, link)
	if err != nil {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, ResidentRestoreRole, link)
	cmd.Dir = record.Cwd
	// Out of native's job when it allows that, so the resident outlives this helper.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x01000000}
	if cmd.Start() != nil {
		cmd = exec.Command(self, ResidentRestoreRole, link)
		cmd.Dir = record.Cwd
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if cmd.Start() != nil {
			return
		}
	}
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }() // a resident that failed to start ends the wait
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if _, err := sessionlink.Read(link); err == nil {
			return
		}
		select {
		case <-exited:
			return
		default:
		}
	}
}
