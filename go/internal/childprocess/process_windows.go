// Package childprocess binds each Windows child tree to its launching process.
package childprocess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// maxCommandLine is CreateProcessW's limit in UTF-16 units, the terminating NUL included
// (measured: 32,766 characters start, 32,767 fail with ERROR_FILENAME_EXCED_RANGE).
const maxCommandLine = 32767

// CommandLineTooLong is a launch Start refused before creating anything, because Windows
// would refuse it. Nothing is cut to fit: the caller says what made it long.
type CommandLineTooLong struct{ Characters int }

func (e *CommandLineTooLong) Error() string {
	return fmt.Sprintf("NATIVE_COMMAND_LINE_TOO_LONG: the native command line is %d characters; Windows allows %d", e.Characters, maxCommandLine-1)
}

// ERROR_NO_DATA. The pipe is being closed from the other end -- what a child that stopped
// reading produces on Windows, beside ERROR_BROKEN_PIPE. Not in syscall, so it is named here.
const errNoData = syscall.Errno(232)

var processKernel = syscall.NewLazyDLL("kernel32.dll")
var createJob = processKernel.NewProc("CreateJobObjectW")
var setJob = processKernel.NewProc("SetInformationJobObject")
var terminateJob = processKernel.NewProc("TerminateJobObject")
var isProcessInJob = processKernel.NewProc("IsProcessInJob")
var queryJob = processKernel.NewProc("QueryInformationJobObject")
var initializeAttributes = processKernel.NewProc("InitializeProcThreadAttributeList")
var updateAttribute = processKernel.NewProc("UpdateProcThreadAttribute")
var deleteAttributes = processKernel.NewProc("DeleteProcThreadAttributeList")

// JOBOBJECT_EXTENDED_LIMIT_INFORMATION. No breakaway, identity, UI or resource
// restrictions: only lifetime ownership, including nested jobs created by native.
type jobLimits struct {
	ProcessTime, JobTime                                       int64
	Flags                                                      uint32
	MinWorkingSet, MaxWorkingSet                               uintptr
	ActiveProcesses                                            uint32
	Affinity                                                   uintptr
	Priority, Scheduling                                       uint32
	IO                                                         [6]uint64
	ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory uintptr
}

type Process struct {
	cmd     *exec.Cmd
	mu      sync.Mutex
	job     syscall.Handle
	outputs sync.WaitGroup
	copyErr error
	pipes   []*os.File
	// reap is set by RecordReap only; nil keeps Wait and Stop exactly as before.
	reap *jobReap
}

// jobReap is RecordReap's state: survivors are the Job's other members still alive when the
// process had exited and nothing had ended them yet; stopped is a Stop while the process ran.
type jobReap struct {
	noted, done, stopped bool
	survivors            uint32
	err                  error
}

// reapGrace bounds the wait for the terminated Job to report no active process.
const reapGrace = 5 * time.Second

// ErrReapNotRecorded is JobEmpty on a Process without RecordReap, or before Wait returned.
var ErrReapNotRecorded = errors.New("JOB_REAP_NOT_RECORDED")

// RecordReap makes Wait prove the Job empty: before anything ends the tree it counts the Job's
// members other than the exited process (a held process), then terminates the Job and waits up
// to reapGrace for JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.ActiveProcesses to reach zero. Only
// the provider probe's starters call it; a launch without it keeps Wait's kill-on-close path.
func (p *Process) RecordReap() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reap = &jobReap{}
}

// JobEmpty is Wait's reap result under RecordReap: the held members it counted, and nil only
// when the Job was confirmed empty. stopped is true when Stop ended the tree while the process
// still ran, so no member was counted as held.
func (p *Process) JobEmpty() (survivors uint32, stopped bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reap == nil || !p.reap.done {
		return 0, false, ErrReapNotRecorded
	}
	return p.reap.survivors, p.reap.stopped, p.reap.err
}

// noteSurvivorsLocked records, once, the Job's members other than the process that are alive
// before anything ends them. A Stop while the process still runs is its caller ending the whole
// tree, not a held process.
func (p *Process) noteSurvivorsLocked(exited bool) {
	r := p.reap
	if r == nil || r.noted || p.job == 0 {
		return
	}
	r.noted = true
	if !exited {
		r.stopped = true
		return
	}
	// JOBOBJECT_BASIC_PROCESS_ID_LIST with room for 64 ids; ERROR_MORE_DATA still fills the count.
	var list struct {
		Assigned, Listed uint32
		IDs              [64]uintptr
	}
	if ok, _, e := queryJob.Call(uintptr(p.job), 3, uintptr(unsafe.Pointer(&list)), unsafe.Sizeof(list), 0); ok == 0 && e != syscall.ERROR_MORE_DATA {
		r.err = fmt.Errorf("JOB_MEMBERS_UNREAD: %w", e)
		return
	}
	self := uintptr(p.cmd.Process.Pid)
	r.survivors = list.Assigned
	for _, id := range list.IDs[:min(list.Listed, uint32(len(list.IDs)))] {
		if id == self {
			r.survivors--
		}
	}
}

// activeProcesses is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.ActiveProcesses.
func activeProcesses(job syscall.Handle) (uint32, error) {
	var info struct {
		TotalUser, TotalKernel, PeriodUser, PeriodKernel                 int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	if ok, _, e := queryJob.Call(uintptr(job), 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0); ok == 0 {
		return 0, e
	}
	return info.ActiveProcesses, nil
}

// proveEmpty is Wait's reap under RecordReap: count held members, terminate the Job, and wait
// for it to report no active process.
func (p *Process) proveEmpty() {
	p.mu.Lock()
	r := p.reap
	if r == nil {
		p.mu.Unlock()
		return
	}
	p.noteSurvivorsLocked(true)
	job := p.job
	p.mu.Unlock()
	err := errors.New("JOB_UNAVAILABLE")
	if job != 0 {
		if ok, _, e := terminateJob.Call(uintptr(job), 1); ok == 0 {
			err = fmt.Errorf("JOB_TERMINATE: %w", e)
		} else {
			err = errors.New("JOB_NOT_EMPTY")
			for deadline := time.Now().Add(reapGrace); ; time.Sleep(50 * time.Millisecond) {
				n, e := activeProcesses(job)
				if e != nil {
					err = fmt.Errorf("JOB_ACCOUNTING_UNREAD: %w", e)
					break
				}
				if n == 0 {
					err = nil
					break
				}
				if time.Now().After(deadline) {
					break
				}
			}
		}
	}
	p.mu.Lock()
	r.err, r.done = errors.Join(r.err, err), true
	p.mu.Unlock()
}

// ProcessIdentity is a read-only observation of the process owned by Process.
// InOwnedJob refers only to this Process object's exact job handle; it never
// means AnyJob membership.
type ProcessIdentity struct {
	PID        int
	Creation   uint64
	InOwnedJob bool
}

// ProcessIdentityError keeps the operation and underlying native error
// available to callers without exposing command or payload text in Error().
type ProcessIdentityError struct {
	Operation string
	Err       error
}

func (e *ProcessIdentityError) Error() string {
	return "PROCESS_IDENTITY_" + e.Operation
}

func (e *ProcessIdentityError) Unwrap() error { return e.Err }

var errProcessIdentityNotLive = errors.New("process identity target is not live")

// Identity observes the live process handle, creation time, and membership in
// the exact Job created by Start. The mutex covers the whole observation so a
// concurrent Wait cannot close p.job between IsProcessInJob and the return.
// A successful false membership result is distinct from an API failure.
func (p *Process) Identity() (ProcessIdentity, error) {
	if p == nil {
		return ProcessIdentity{}, &ProcessIdentityError{Operation: "UNAVAILABLE", Err: errors.New("owned process is unavailable")}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil || p.job == 0 {
		return ProcessIdentity{}, &ProcessIdentityError{Operation: "UNAVAILABLE", Err: errors.New("owned process is unavailable")}
	}
	identity := ProcessIdentity{PID: p.cmd.Process.Pid}
	var callbackErr error
	handleErr := p.cmd.Process.WithHandle(func(raw uintptr) {
		handle := syscall.Handle(raw)
		state, err := syscall.WaitForSingleObject(handle, 0)
		if err != nil {
			callbackErr = &ProcessIdentityError{Operation: "WAIT", Err: err}
			return
		}
		if state != syscall.WAIT_TIMEOUT {
			callbackErr = &ProcessIdentityError{Operation: "NOT_LIVE", Err: errProcessIdentityNotLive}
			return
		}
		var created, exited, kernel, user syscall.Filetime
		if err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
			callbackErr = &ProcessIdentityError{Operation: "PROCESS_TIMES", Err: err}
			return
		}
		identity.Creation = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
		var associated int32
		if ok, _, err := isProcessInJob.Call(raw, uintptr(p.job), uintptr(unsafe.Pointer(&associated))); ok == 0 {
			callbackErr = &ProcessIdentityError{Operation: "IS_PROCESS_IN_JOB", Err: err}
			return
		}
		identity.InOwnedJob = associated != 0
		state, err = syscall.WaitForSingleObject(handle, 0)
		if err != nil {
			callbackErr = &ProcessIdentityError{Operation: "WAIT_AFTER", Err: err}
			return
		}
		if state != syscall.WAIT_TIMEOUT {
			callbackErr = &ProcessIdentityError{Operation: "NOT_LIVE_AFTER", Err: errProcessIdentityNotLive}
		}
	})
	if handleErr != nil {
		return ProcessIdentity{}, &ProcessIdentityError{Operation: "PROCESS_HANDLE", Err: handleErr}
	}
	if callbackErr != nil {
		return ProcessIdentity{}, callbackErr
	}
	return identity, nil
}

func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	if p.reap != nil {
		// A process already waited for has released its handle: it had exited.
		running := false
		p.cmd.Process.WithHandle(func(raw uintptr) {
			state, err := syscall.WaitForSingleObject(syscall.Handle(raw), 0)
			running = err == nil && state == syscall.WAIT_TIMEOUT
		})
		p.noteSurvivorsLocked(!running)
	}
	if ok, _, err := terminateJob.Call(uintptr(p.job), 1); ok == 0 {
		return err
	}
	return nil
}

func (p *Process) closeJob() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	err := syscall.CloseHandle(p.job)
	if err == nil {
		p.job = 0
	}
	return err
}

func (p *Process) Wait() error {
	state, err := p.cmd.Process.Wait()
	p.cmd.ProcessState = state
	// Reap native, then terminate descendants before waiting for their inherited
	// output pipes. Otherwise a surviving child could keep EOF away forever.
	p.proveEmpty()
	jobErr := p.closeJob()
	p.outputs.Wait()
	for _, f := range p.pipes {
		_ = f.Close()
	}
	if err == nil && state != nil && !state.Success() {
		err = &exec.ExitError{ProcessState: state}
	}
	return errors.Join(err, jobErr, p.copyErr)
}

func (p *Process) ExitCode() int         { return p.cmd.ProcessState.ExitCode() }
func (p *Process) PID() int              { return p.cmd.Process.Pid }
func (p *Process) CreationFlags() uint32 { return p.cmd.SysProcAttr.CreationFlags }

func Run(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := Start(cmd)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = p.Stop() })
	defer stop()
	err = p.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Start uses CreateProcessW with STARTUPINFOEX because Go's SysProcAttr has no JOB_LIST. It assigns the
// job before the first instruction, closing even the suspended-then-assign race.
// The non-inheritable, unnamed job handle never enters the child's handle list.
// stdio files and console group are preserved for the native TUI and Ctrl+C.
func Start(cmd *exec.Cmd) (*Process, error) { return start(cmd, nil) }

// StartObserved captures the owned generation before its first instruction. A failed
// observation terminates the still-suspended job; ordinary Start has no extra suspension.
func StartObserved(cmd *exec.Cmd, observe func(*Process) error) (*Process, error) {
	if observe == nil {
		return nil, errors.New("PROCESS_OBSERVER_REQUIRED")
	}
	return start(cmd, observe)
}

func start(cmd *exec.Cmd, observe func(*Process) error) (_ *Process, err error) {
	stdin, stdout, stderr := cmd.Stdin, cmd.Stdout, cmd.Stderr
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	attrs := *cmd.SysProcAttr
	attrs.HideWindow = false
	if !reflect.DeepEqual(attrs, syscall.SysProcAttr{}) || cmd.Cancel != nil || cmd.Process != nil {
		return nil, errors.New("UNSUPPORTED_OWNED_PROCESS_ATTRIBUTES")
	}
	cmd.SysProcAttr.CreationFlags = syscall.CREATE_UNICODE_ENVIRONMENT | 0x80000
	if observe != nil {
		cmd.SysProcAttr.CreationFlags |= 0x4
	} // CREATE_SUSPENDED
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		args[i] = syscall.EscapeArg(arg)
	}
	line, err := syscall.UTF16FromString(strings.Join(args, " "))
	if err != nil {
		return nil, err
	}
	if len(line) > maxCommandLine {
		return nil, &CommandLineTooLong{Characters: len(line) - 1}
	}
	p := &Process{cmd: cmd}
	job, _, callErr := createJob.Call(0, 0)
	if job == 0 {
		return nil, callErr
	}
	p.job = syscall.Handle(job)
	var children []*os.File
	var copies []func()
	defer func() {
		for _, f := range children {
			_ = f.Close()
		}
		if err != nil {
			_ = p.closeJob()
			for _, f := range p.pipes {
				_ = f.Close()
			}
		}
	}()
	limits := jobLimits{Flags: 0x2000} // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if ok, _, e := setJob.Call(job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 {
		return nil, e
	}
	files := [3]*os.File{}
	if f, ok := stdin.(*os.File); ok {
		files[0] = f
	} else if stdin == nil {
		files[0], err = os.Open(os.DevNull)
		if err != nil {
			return nil, err
		}
		children = append(children, files[0])
	} else {
		r, w, e := os.Pipe()
		if e != nil {
			return nil, e
		}
		files[0] = r
		children = append(children, r)
		p.pipes = append(p.pipes, w)
		// Registered like the output copiers, and for the reason exec.Cmd waits for its own
		// stdin goroutine: Wait() closes p.pipes as soon as outputs.Wait() returns, and this
		// one was not in that group, so a copy still parked on a full pipe had the pipe
		// closed under it. Its error went nowhere either, so a child that consumed only a
		// prefix of a 24 MB document and exited 0 was reported as a clean run and its
		// truncated output parsed as complete.
		//
		// A peer that stops reading is not a failure -- that is a child exiting early, which
		// exec suppresses here too -- so only an unexpected error is recorded.
		p.outputs.Add(1)
		copies = append(copies, func() {
			defer p.outputs.Done()
			_, e := io.Copy(w, stdin)
			_ = w.Close()
			if e != nil && !errors.Is(e, os.ErrClosed) && !errors.Is(e, io.ErrClosedPipe) && !errors.Is(e, syscall.ERROR_BROKEN_PIPE) && !errors.Is(e, errNoData) {
				p.mu.Lock()
				p.copyErr = errors.Join(p.copyErr, e)
				p.mu.Unlock()
			}
		})
	}
	// Serialize writes when stdout and stderr share a destination, as exec does.
	var outputMu sync.Mutex
	for i, writer := range []io.Writer{stdout, stderr} {
		if f, ok := writer.(*os.File); ok {
			files[i+1] = f
			continue
		}
		if writer == nil {
			writer = io.Discard
		}
		r, w, e := os.Pipe()
		if e != nil {
			return nil, e
		}
		files[i+1] = w
		children = append(children, w)
		p.pipes = append(p.pipes, r)
		p.outputs.Add(1)
		copies = append(copies, func() {
			defer p.outputs.Done()
			_, e := io.Copy(&lockedProcessWriter{writer, &outputMu}, r)
			_ = r.Close()
			p.mu.Lock()
			p.copyErr = errors.Join(p.copyErr, e)
			p.mu.Unlock()
		})
	}
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	var handles [3]syscall.Handle
	for i, f := range files {
		if err = syscall.DuplicateHandle(current, syscall.Handle(f.Fd()), current, &handles[i], 0, true, syscall.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		defer syscall.CloseHandle(handles[i])
	}
	var size uintptr
	initializeAttributes.Call(0, 2, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 || size > 65536 {
		return nil, errors.New("PROCESS_ATTRIBUTE_SIZE_INVALID")
	}
	attributes := make([]byte, size)
	ptr := uintptr(unsafe.Pointer(&attributes[0]))
	if ok, _, e := initializeAttributes.Call(ptr, 2, 0, uintptr(unsafe.Pointer(&size))); ok == 0 {
		return nil, e
	}
	defer deleteAttributes.Call(ptr)
	if ok, _, e := updateAttribute.Call(ptr, 0, 0x2000d, uintptr(unsafe.Pointer(&p.job)), unsafe.Sizeof(p.job), 0, 0); ok == 0 {
		return nil, e
	}
	if ok, _, e := updateAttribute.Call(ptr, 0, 0x20002, uintptr(unsafe.Pointer(&handles[0])), unsafe.Sizeof(handles), 0, 0); ok == 0 {
		return nil, e
	}
	info := struct {
		syscall.StartupInfo
		Attributes uintptr
	}{Attributes: ptr}
	info.Cb = uint32(unsafe.Sizeof(info))
	info.Flags = syscall.STARTF_USESTDHANDLES
	if cmd.SysProcAttr.HideWindow {
		info.Flags |= syscall.STARTF_USESHOWWINDOW
		info.ShowWindow = syscall.SW_HIDE
	}
	info.StdInput, info.StdOutput, info.StdErr = handles[0], handles[1], handles[2]
	file, err := filepath.Abs(cmd.Path)
	if err != nil {
		return nil, err
	}
	app, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if cmd.Dir != "" {
		dir, err = syscall.UTF16PtrFromString(cmd.Dir)
		if err != nil {
			return nil, err
		}
	}
	// Preserve the explicit launch environment; embedded NUL would truncate it.
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	var block []uint16
	for _, value := range env {
		part, e := syscall.UTF16FromString(value)
		if e != nil {
			return nil, e
		}
		block = append(block, part...)
	}
	block = append(block, 0, 0)
	var pi syscall.ProcessInformation
	err = syscall.CreateProcess(app, &line[0], nil, nil, true, cmd.SysProcAttr.CreationFlags, &block[0], dir, &info.StartupInfo, &pi)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(p)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(pi.Thread)
	defer syscall.CloseHandle(pi.Process)
	cmd.Process, err = os.FindProcess(int(pi.ProcessId))
	if err != nil {
		return nil, err
	}
	if observe != nil {
		if err = observe(p); err != nil {
			_ = cmd.Process.Release()
			return nil, err
		}
		resumed, _, resumeErr := processKernel.NewProc("ResumeThread").Call(uintptr(pi.Thread))
		if resumed != 1 {
			_ = cmd.Process.Release()
			return nil, &ProcessIdentityError{Operation: "RESUME", Err: resumeErr}
		}
	}
	for _, copy := range copies {
		go copy()
	}
	return p, nil
}

type lockedProcessWriter struct {
	writer io.Writer
	mu     *sync.Mutex
}

func (w *lockedProcessWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(b)
}
