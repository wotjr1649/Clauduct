//go:build windows

// ptydrive runs one program inside a Windows pseudo console (ConPTY) and types a scripted
// sequence into it, each step waiting for text on the screen. It exists so an agent without
// a terminal can run the interactive TUI evidence checks. Standard library only. A
// maintainer tool: it is not a release asset.
//
//	go run ./cmd/ptydrive -dir <cwd> -script steps.json -log raw.log -text screen.txt -env K=V ... -- <exe> <args...>
//
// Three things a script has to handle, found writing it for v0.3.2: the child inherits the
// caller's redirected standard handles unless STARTF_USESTDHANDLES carries invalid ones, and
// native then refuses to start; the TUI places words with cursor moves, so waits match the
// screen text with all whitespace removed; the first-run folder trust dialog defaults to
// "No, exit". Steps depend on client screens, so a wait that times out prints the screen it
// was looking at.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	createPseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	closePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
	initAttributeList   = kernel32.NewProc("InitializeProcThreadAttributeList")
	updateAttribute     = kernel32.NewProc("UpdateProcThreadAttribute")
	deleteAttributeList = kernel32.NewProc("DeleteProcThreadAttributeList")
	createProcess       = kernel32.NewProc("CreateProcessW")
	setCtrlHandler      = kernel32.NewProc("SetConsoleCtrlHandler")
)

const (
	extendedStartupInfoPresent = 0x00080000
	createUnicodeEnvironment   = 0x00000400
	attributePseudoConsole     = 0x00020016
)

type startupInfoEx struct {
	syscall.StartupInfo
	attributes uintptr
}

type step struct {
	Wait     string   `json:"wait"`     // regexp on the screen text written after the previous step
	Send     []string `json:"send"`     // written in order, 300ms apart
	Optional bool     `json:"optional"` // a missing screen is skipped, not a failure
	Timeout  int      `json:"timeout_s"`
	After    int      `json:"after_ms"` // pause between the match and the first write
}

type envList []string

func (e *envList) String() string     { return strings.Join(*e, ",") }
func (e *envList) Set(s string) error { *e = append(*e, s); return nil }

var control = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[@-Z\\-_=>]|[\x00-\x08\x0b-\x1f\x7f]`)

func main() {
	dir := flag.String("dir", "", "working directory")
	script := flag.String("script", "", "JSON steps")
	rawLog := flag.String("log", "", "raw output file")
	textLog := flag.String("text", "", "screen text file")
	total := flag.Int("timeout", 600, "seconds before the child is abandoned")
	var env envList
	flag.Var(&env, "env", "KEY=VALUE added to the inherited environment")
	flag.Parse()
	if flag.NArg() == 0 || *script == "" {
		fmt.Fprintln(os.Stderr, "usage: ptydrive -script steps.json [-dir d] [-env K=V]... -- exe args...")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*script)
	check(err, "script")
	var steps []step
	check(json.Unmarshal(raw, &steps), "script")

	var inRead, inWrite, outRead, outWrite syscall.Handle
	check(syscall.CreatePipe(&inRead, &inWrite, nil, 0), "input pipe")
	check(syscall.CreatePipe(&outRead, &outWrite, nil, 0), "output pipe")
	var console uintptr
	size := uintptr(uint32(50)<<16 | uint32(160)) // COORD{X:160, Y:50} passed by value
	if r, _, e := createPseudoConsole.Call(size, uintptr(inRead), uintptr(outWrite), 0, uintptr(unsafe.Pointer(&console))); r != 0 {
		check(e, "CreatePseudoConsole")
	}
	var listSize uintptr
	initAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&listSize)))
	list := make([]byte, listSize)
	if r, _, e := initAttributeList.Call(uintptr(unsafe.Pointer(&list[0])), 1, 0, uintptr(unsafe.Pointer(&listSize))); r == 0 {
		check(e, "InitializeProcThreadAttributeList")
	}
	if r, _, e := updateAttribute.Call(uintptr(unsafe.Pointer(&list[0])), 0, attributePseudoConsole, console, unsafe.Sizeof(console), 0, 0); r == 0 {
		check(e, "UpdateProcThreadAttribute")
	}
	si := startupInfoEx{attributes: uintptr(unsafe.Pointer(&list[0]))}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Without this the child inherits this process's redirected handles (an agent's pipes)
	// and never sees the pseudo console: native then refuses to start without --print input.
	si.Flags = syscall.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = syscall.InvalidHandle, syscall.InvalidHandle, syscall.InvalidHandle
	args := flag.Args()
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	cmdline, _ := syscall.UTF16PtrFromString(strings.Join(quoted, " "))
	var cwd *uint16
	if *dir != "" {
		cwd, _ = syscall.UTF16PtrFromString(*dir)
	}
	// Whether Ctrl+C is ignored is inherited, and an agent's shell may have started this
	// process with it ignored. Cleared so that a "\u0003" step reaches the child as a console
	// control event, the way it does in a terminal a person opened.
	setCtrlHandler.Call(0, 0)
	block := environment(env)
	var pi syscall.ProcessInformation
	if r, _, e := createProcess.Call(0, uintptr(unsafe.Pointer(cmdline)), 0, 0, 0,
		extendedStartupInfoPresent|createUnicodeEnvironment, uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(cwd)), uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi))); r == 0 {
		check(e, "CreateProcess")
	}
	deleteAttributeList.Call(uintptr(unsafe.Pointer(&list[0])))
	// The pseudo console holds its own copies of the child-side ends.
	syscall.CloseHandle(inRead)
	syscall.CloseHandle(outWrite)

	var mu sync.Mutex
	var screen []byte
	var logFile *os.File
	if *rawLog != "" {
		logFile, err = os.Create(*rawLog)
		check(err, "log")
	}
	go func() {
		buf := make([]byte, 64<<10)
		for {
			var n uint32
			if err := syscall.ReadFile(outRead, buf, &n, nil); err != nil || n == 0 {
				return
			}
			mu.Lock()
			screen = append(screen, buf[:n]...)
			mu.Unlock()
			if logFile != nil {
				logFile.Write(buf[:n])
			}
		}
	}()
	text := func() string {
		mu.Lock()
		defer mu.Unlock()
		return control.ReplaceAllString(string(screen), "")
	}
	// The TUI places words with cursor moves, so stripped text loses its spaces; steps
	// match against the text with all whitespace removed and are written without spaces.
	compact := func() string { return strings.Join(strings.Fields(text()), "") }
	write := func(s string) {
		var n uint32
		check(syscall.WriteFile(inWrite, []byte(s), &n, nil), "write")
	}

	started := time.Now()
	from := 0
	failed := ""
	for i, s := range steps {
		pattern := regexp.MustCompile(s.Wait)
		limit := time.Duration(max(s.Timeout, 1)) * time.Second
		deadline := time.Now().Add(limit)
		matched := false
		for time.Now().Before(deadline) {
			current := compact()
			if loc := pattern.FindStringIndex(current[min(from, len(current)):]); loc != nil {
				from += loc[1]
				matched = true
				break
			}
			if exited(pi.Process) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		fmt.Printf("step=%d t=%.1fs wait=%q matched=%v\n", i, time.Since(started).Seconds(), s.Wait, matched)
		if !matched && !s.Optional {
			failed = s.Wait
			fmt.Fprintf(os.Stderr, "screen (last %d bytes):\n%s\n", screenTail, tail(text(), screenTail))
			break
		}
		if !matched {
			continue
		}
		time.Sleep(time.Duration(s.After) * time.Millisecond)
		for j, part := range s.Send {
			if j > 0 {
				time.Sleep(300 * time.Millisecond)
			}
			write(part)
		}
		from = len(compact())
	}
	wait := uint32(*total * 1000)
	if failed != "" {
		wait = 5000
	}
	event, _ := syscall.WaitForSingleObject(pi.Process, wait)
	var code uint32 = 1<<32 - 1
	if event == syscall.WAIT_OBJECT_0 {
		syscall.GetExitCodeProcess(pi.Process, &code)
	} else {
		syscall.TerminateProcess(pi.Process, 1)
	}
	time.Sleep(time.Second)
	closePseudoConsole.Call(console)
	syscall.CloseHandle(inWrite)
	if *textLog != "" {
		os.WriteFile(*textLog, []byte(text()), 0600)
	}
	fmt.Printf("child_exit=%d failed_wait=%q elapsed=%.1fs\n", int32(code), failed, time.Since(started).Seconds())
	if failed != "" || code != 0 {
		os.Exit(1)
	}
}

const screenTail = 4000

// tail keeps the end of s, cut at a rune boundary.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

func exited(process syscall.Handle) bool {
	event, _ := syscall.WaitForSingleObject(process, 0)
	return event == syscall.WAIT_OBJECT_0
}

// environment returns the inherited environment with the overrides applied, as the
// sorted, double-NUL-terminated UTF-16 block CreateProcessW expects.
func environment(overrides []string) []uint16 {
	values := map[string]string{}
	names := map[string]string{}
	for _, kv := range append(os.Environ(), overrides...) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		upper := strings.ToUpper(k)
		names[upper] = k
		values[upper] = v
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var block []uint16
	for _, k := range keys {
		block = append(block, utf16.Encode([]rune(names[k]+"="+values[k]))...)
		block = append(block, 0)
	}
	return append(block, 0)
}

func check(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
		os.Exit(2)
	}
}
