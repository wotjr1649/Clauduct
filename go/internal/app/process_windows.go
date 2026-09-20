package app

import (
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"io"
	"os/exec"
)

type osProcess = childprocess.Process

func startOSProcess(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.Command(spec.File, spec.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Dir, spec.Env, stdin, stdout, stderr
	return childprocess.Start(cmd)
}
