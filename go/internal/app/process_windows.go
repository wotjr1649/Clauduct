package app

import (
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

func startOSProcess(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.Command(spec.File, spec.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Dir, spec.Env, stdin, stdout, stderr
	return childprocess.Start(cmd)
}

func nativeProcessIdentity(process Process) gateway.NativeProcess {
	switch child := process.(type) {
	case *childprocess.Process:
		if id, err := child.Identity(); err == nil && id.InOwnedJob {
			created := syscall.Filetime{LowDateTime: uint32(id.Creation), HighDateTime: uint32(id.Creation >> 32)}
			return gateway.NativeProcess{PID: uint32(id.PID), Created: created.Nanoseconds()}
		}
	case interface{ NativeProcessIdentity() (uint32, time.Time) }:
		// The terminal test driver owns the pseudoconsole and proves the actual child.
		if pid, created := child.NativeProcessIdentity(); pid > 1 && !created.IsZero() {
			return gateway.NativeProcess{PID: pid, Created: created.UnixNano()}
		}
	}
	return gateway.NativeProcess{}
}
