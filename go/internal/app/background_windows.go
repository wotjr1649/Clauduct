package app

import (
	"context"
	"encoding/json"
	"errors"
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

// The resident is an explicit --bg owner, not a child of the foreground native
// job. CREATE_NO_WINDOW detaches it from the dispatching console; no global
// service, startup entry or process execution policy is installed.
func StartBackground(ctx context.Context, args []string, cwd string, out, errOut io.Writer) error {
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
		if d.reply.Session.PID != cmd.Process.Pid || !regexp.MustCompile(`^[a-f0-9]{8}$`).MatchString(d.reply.Session.JobID) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(d.reply.Session.ConnectionID) {
			_ = cmd.Process.Kill()
			return errors.New("BACKGROUND_START_UNVERIFIED")
		}
		fmtBackground(out, d.reply.Session.JobID, d.reply.Session.ConnectionID)
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return errors.New("BACKGROUND_START_UNVERIFIED")
	case <-deadline.C:
		_ = cmd.Process.Kill()
		return errors.New("BACKGROUND_START_UNVERIFIED")
	}
}
