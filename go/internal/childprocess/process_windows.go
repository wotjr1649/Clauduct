// Package childprocess binds each Windows child tree to its launching process.
package childprocess

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// ERROR_NO_DATA. The pipe is being closed from the other end -- what a child that stopped
// reading produces on Windows, beside ERROR_BROKEN_PIPE. Not in syscall, so it is named here.
const errNoData = syscall.Errno(232)

var processKernel = syscall.NewLazyDLL("kernel32.dll")
var createJob = processKernel.NewProc("CreateJobObjectW")
var setJob = processKernel.NewProc("SetInformationJobObject")
var terminateJob = processKernel.NewProc("TerminateJobObject")
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
}

func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
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

// Go's SysProcAttr has no JOB_LIST. CreateProcessW with STARTUPINFOEX assigns the
// job before the first instruction, closing even the suspended-then-assign race.
// The non-inheritable, unnamed job handle never enters the child's handle list.
// stdio files and console group are preserved for the native TUI and Ctrl+C.
func Start(cmd *exec.Cmd) (_ *Process, err error) {
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
	if cmd.Err != nil {
		return nil, cmd.Err
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
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		args[i] = syscall.EscapeArg(arg)
	}
	line, err := syscall.UTF16PtrFromString(strings.Join(args, " "))
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
	err = syscall.CreateProcess(app, line, nil, nil, true, cmd.SysProcAttr.CreationFlags, &block[0], dir, &info.StartupInfo, &pi)
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
