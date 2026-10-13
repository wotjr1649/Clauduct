package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	cmd.Stderr = residentDiagnostics()
	// Out of native's job when it allows that, so the resident outlives this helper.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x01000000}
	if cmd.Start() != nil {
		cmd = exec.Command(self, ResidentRestoreRole, link)
		cmd.Dir = record.Cwd
		cmd.Stderr = residentDiagnostics()
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

// Relay diagnostics so the resident cannot keep native's key-helper pipe open.
func residentDiagnostics() io.Writer { return io.MultiWriter(os.Stderr) }

var errResidentBusy = errors.New("RESIDENT_RECORD_BUSY")

func lockResident(home, link string, wait time.Duration) (func(), error) {
	if home == "" || !residentLink.MatchString(link) {
		return nil, errors.New("RESIDENT_RECORD_INVALID")
	}
	dir := filepath.Dir(residentPath(home, link))
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil, errors.New("RESIDENT_RECORD_INVALID")
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, link+".lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("RESIDENT_RECORD_INVALID")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.Errno(5)) && !errors.Is(err, errSharingViolation) {
		return nil, err
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		h, err := syscall.CreateFile(name, syscall.GENERIC_WRITE|0x00010000, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL|0x04000000, 0)
		if err == nil {
			f := os.NewFile(uintptr(h), link+".lock")
			if err := f.Truncate(0); err != nil {
				f.Close()
				return nil, err
			}
			if _, err := fmt.Fprintf(f, "pid %d since %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339)); err != nil {
				f.Close()
				return nil, err
			}
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, errSharingViolation) && !errors.Is(err, syscall.Errno(5)) {
			return nil, err
		}
		if wait == 0 || !time.Now().Before(deadline) {
			return nil, errResidentBusy
		}
		time.Sleep(min(100*time.Millisecond, time.Until(deadline)))
	}
}

var reconnectRegenerateResident = RegenerateResident

// ReconnectResident waits at most two seconds and checks native's job identity.
func ReconnectResident(link, session string) {
	ReconnectResidentContext(context.Background(), link, session)
}

// ReconnectResidentContext shortens the wait without canceling restoration.
func ReconnectResidentContext(parent context.Context, link, session string) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		record, err := LoadResident(home, link)
		if err != nil || !record.ownsSession(nativeConfigDirectory(platform.Environment(os.Environ()), record.Cwd), session) {
			return
		}
		reconnectRegenerateResident(link, 2*time.Second)
	}()
	// The caller's deadline does not cancel the resident or its restoration.
	select {
	case <-done:
	case <-ctx.Done():
	}
}
