package gateway

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

func (g *Gateway) commandLineForkSession(process processKey) string {
	if process.pid == 0 || process.created <= 0 {
		return ""
	}
	read := g.commandLine
	if read == nil {
		read = providercert.ProcessCommandLine
	}
	return forkResumeSession(read(process.pid, process.created))
}

// Only native's explicit long resume option proves a parent; option values and prompts do not.
func forkResumeSession(line string) string {
	if line == "" {
		return ""
	}
	text, err := syscall.UTF16PtrFromString(line)
	if err != nil {
		return ""
	}
	var count int32
	argv, err := syscall.CommandLineToArgv(text, &count)
	if err != nil {
		return ""
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(argv)))
	if count < 2 || count > int32(len(argv)) {
		return ""
	}
	args := make([]string, 0, count-1)
	for _, arg := range argv[1:count] {
		args = append(args, syscall.UTF16ToString(arg[:]))
	}
	var resume string
	fork := false
	for i := 0; i < len(args) && args[i] != "--"; {
		end, known := launch.NativeArgEnd(args, i)
		if !known || end > len(args) {
			return "" // An unknown arity could hide a later override of the resume target.
		}
		name, value, attached := strings.Cut(args[i], "=")
		switch name {
		case "--resume":
			resume = value
			if !attached && end == i+2 {
				resume = args[i+1]
			}
		case "--fork-session":
			if attached {
				return ""
			}
			fork = true
		case "--continue":
			return ""
		default:
			if len(name) > 1 && name[0] == '-' && name[1] != '-' {
				for _, flag := range name[1:] {
					if flag == 'r' || flag == 'c' {
						return "" // Do not infer a parent from another resume or continue form.
					}
					if !strings.ContainsRune("pvh", flag) {
						break
					}
				}
			}
		}
		i = end
	}
	if !fork {
		return ""
	}
	resume = strings.ToLower(resume)
	if !IsSessionUUID(resume) {
		if filepath.Ext(resume) != ".jsonl" {
			return ""
		}
		resume = strings.TrimSuffix(filepath.Base(resume), ".jsonl")
	}
	if !IsSessionUUID(resume) {
		return ""
	}
	return resume
}
