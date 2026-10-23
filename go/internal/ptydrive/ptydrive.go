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
//
// The code lives here so the provider-control probe (go/cmd/clauduct-probe) runs the same
// driver as one of its own roles, without a Go toolchain on the machine.
package ptydrive

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Wait     string   `json:"wait"`      // regexp on the screen text written after the previous step
	WaitFile string   `json:"wait_file"` // optional fresh file containing exactly "ready\n"
	Send     []string `json:"send"`      // written in order, 300ms apart
	Optional bool     `json:"optional"`  // a missing screen is skipped, not a failure
	Timeout  int      `json:"timeout_s"`
	After    int      `json:"after_ms"` // pause between the match and the first write

	// Keep the console until this signal after the direct child exits. Its interrupted
	// step and exit status still fail; this isolates launcher loss from console close.
	HoldConsoleAfterExit bool `json:"hold_console_after_exit"`
	CloseConsole         bool `json:"close_console"`

	// Until, on an optional step, is a screen that means the step's own screen will not come
	// (the main prompt is already up): the step is skipped once it has shown for a short
	// grace in which the step's own screen can still win. It replaces a fixed wait.
	Until string `json:"until"`
	// ConfirmFile, ConfirmScreen and ConfirmMS check that the input the step typed was
	// submitted: the caller writes ConfirmFile ("ready\n") when it sees the effect, or
	// ConfirmScreen shows after the typed input. The outcome is printed and marked, never
	// silently assumed. ConfirmResend sends Enter once more after an unconfirmed window.
	ConfirmFile   string `json:"confirm_file"`
	ConfirmScreen string `json:"confirm_screen"`
	ConfirmMS     int    `json:"confirm_ms"`
	ConfirmResend bool   `json:"confirm_resend"`
	// QuietMS, when set, holds the input until the screen has not changed for that long (at
	// most 5 seconds, and never past the step's own end): a TUI that is still drawing can
	// drop what it is sent.
	QuietMS int `json:"quiet_ms"`
	// EchoGate holds each write that follows a text write until the screen shows that text and
	// has been quiet for EchoQuietMS (default 300), at most EchoMaxMS (default 3000): Enter is
	// then sent alone, once the TUI has taken the typed text. A gate that times out still sends,
	// and the outcome is marked, never assumed.
	EchoGate      bool `json:"echo_gate"`
	EchoQuietMS   int  `json:"echo_quiet_ms"`
	EchoMaxMS     int  `json:"echo_max_ms"`
	pattern       *regexp.Regexp
	until         *regexp.Regexp
	confirmScreen *regexp.Regexp
}

type envList []string

func (e *envList) String() string     { return strings.Join(*e, ",") }
func (e *envList) Set(s string) error { *e = append(*e, s); return nil }

var control = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[@-Z\\-_=>]|[\x00-\x08\x0b-\x1f\x7f]`)

// Main runs the driver on os.Args; it does not return.
func Main() {
	dir := flag.String("dir", "", "working directory")
	script := flag.String("script", "", "JSON steps")
	cleanupScript := flag.String("cleanup-script", "", "JSON steps run only after a failed step; the run still fails")
	rawLog := flag.String("log", "", "raw output file")
	textLog := flag.String("text", "", "screen text file")
	total := flag.Int("timeout", 600, "seconds before the child is abandoned")
	deadline := flag.Int("deadline", 0, "seconds from start by which the driver must have written its evidence and ended; steps stop earlier so a caller's own limit never cuts the driver off (0: none)")
	marks := flag.String("marks", "", "file that gets one line per matched step: its index and the time its input began, RFC 3339")
	var env envList
	flag.Var(&env, "env", "KEY=VALUE added to the inherited environment")
	flag.Parse()
	if flag.NArg() == 0 || *script == "" {
		fmt.Fprintln(os.Stderr, "usage: ptydrive -script steps.json [-dir d] [-env K=V]... -- exe args...")
		os.Exit(2)
	}
	steps, err := loadSteps(*script, nil)
	check(err, "script")
	var cleanup []step
	if *cleanupScript != "" {
		cleanup, err = loadSteps(*cleanupScript, steps)
		check(err, "cleanup-script")
	}
	holdSignal := ""
	for _, s := range steps {
		if s.HoldConsoleAfterExit {
			holdSignal = s.WaitFile
		}
	}
	for _, s := range cleanup {
		if s.HoldConsoleAfterExit {
			check(fmt.Errorf("console hold belongs to the primary script"), "cleanup-script")
		}
	}
	if holdSignal != "" && (*total <= 0 || int64(*total) > (1<<63-1)/int64(time.Second)) {
		check(fmt.Errorf("console hold needs a bounded positive timeout"), "timeout")
	}

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
	write := func(s string) error {
		if holdSignal != "" && exited(pi.Process) {
			return fmt.Errorf("direct child exited")
		}
		var n uint32
		if err := syscall.WriteFile(inWrite, []byte(s), &n, nil); err != nil {
			return err
		}
		if int(n) != len(s) {
			return io.ErrShortWrite
		}
		return nil
	}

	started := time.Now()
	// The steps end before the caller's limit so the driver can write its screen, and the wait
	// for the child's exit ends before it too.
	var stepsEnd, exitEnd time.Time
	if *deadline > 0 {
		stepsEnd = started.Add(time.Duration(*deadline)*time.Second - driverReserve)
		if earliest := started.Add(time.Second); stepsEnd.Before(earliest) {
			stepsEnd = earliest
		}
		exitEnd = started.Add(time.Duration(*deadline)*time.Second - 2*time.Second)
	}
	var holdEnd time.Time
	if holdSignal != "" {
		holdEnd = started.Add(time.Duration(*total) * time.Second)
		if !exitEnd.IsZero() && exitEnd.Before(holdEnd) {
			holdEnd = exitEnd
		}
	}
	writeText := func() {
		if *textLog != "" {
			os.WriteFile(*textLog, []byte(text()), 0600)
		}
	}
	// mark appends "<label> <time> [detail]" to the marks file; a bare index label is a step's
	// input start, any other label is a driver event (submission check, Enter resend).
	mark := func(label, detail string) {
		if *marks == "" {
			return
		}
		if f, err := os.OpenFile(*marks, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			if detail != "" {
				fmt.Fprintf(f, "%s %s %s\n", label, time.Now().Format(time.RFC3339Nano), detail)
			} else {
				fmt.Fprintf(f, "%s %s\n", label, time.Now().Format(time.RFC3339Nano))
			}
			f.Close()
		}
	}
	consoleClosed := false
	closeConsole := func() bool {
		if consoleClosed {
			return true
		}
		consoleClosed = true
		mark("console-close-begin", "")
		done := make(chan struct{})
		go func() { closePseudoConsole.Call(console); close(done) }()
		select {
		case <-done:
			mark("console-close-end", "")
			return true
		case <-time.After(10 * time.Second):
			mark("console-close-timeout", "")
			return false
		}
	}
	runSteps := func(steps []step, phase string, from int) string {
		for i, s := range steps {
			limit := time.Duration(max(s.Timeout, 1)) * time.Second
			limitAt := time.Now().Add(limit)
			capped := false
			if !stepsEnd.IsZero() && stepsEnd.Before(limitAt) {
				limitAt, capped = stepsEnd, true
			}
			if s.HoldConsoleAfterExit && limitAt.Before(holdEnd) {
				holdEnd = limitAt
			}
			matched := false
			screenMatched := false
			skipped := false
			var untilAt time.Time
			var signalErr error
			// The step is checked once even when its time is already gone.
			for first := true; first || time.Now().Before(limitAt); first = false {
				current := compact()
				if !screenMatched {
					if loc := s.pattern.FindStringIndex(current[min(from, len(current)):]); loc != nil {
						from += loc[1]
						screenMatched = true
					} else if s.until != nil && s.Optional && s.until.MatchString(current[min(from, len(current)):]) {
						if untilAt.IsZero() {
							untilAt = time.Now()
						}
						if time.Since(untilAt) >= untilGrace {
							skipped = true
							break
						}
					}
				}
				if screenMatched {
					matched, signalErr = readyFile(s.WaitFile)
					if matched || signalErr != nil {
						break
					}
				}
				if exited(pi.Process) && !s.CloseConsole {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			fmt.Printf("%sstep=%d t=%.1fs wait=%q matched=%v skipped=%v capped=%v\n", phase, i, time.Since(started).Seconds(), s.Wait, matched, skipped, capped && !matched && !skipped)
			if signalErr != nil {
				fmt.Fprintf(os.Stderr, "wait_file: %v\n", signalErr)
				return "wait_file"
			}
			if !matched && (!s.Optional || screenMatched && s.WaitFile != "") {
				failed := s.Wait
				if screenMatched && s.WaitFile != "" {
					failed = "wait_file"
				}
				fmt.Fprintf(os.Stderr, "screen (last %d bytes):\n%s\n", screenTail, tail(text(), screenTail))
				writeText()
				return failed
			}
			if !matched {
				continue
			}
			time.Sleep(time.Duration(s.After) * time.Millisecond)
			if s.QuietMS > 0 {
				quiet := awaitQuiet(func() int { return len(compact()) }, time.Duration(s.QuietMS)*time.Millisecond, 5*time.Second, time.Now, time.Sleep)
				fmt.Printf("%squiet=%d settled=%v\n", phase, i, quiet)
			}
			mark(fmt.Sprintf("%s%d", phase, i), "")
			if s.CloseConsole {
				if !closeConsole() {
					return "close_console"
				}
				return ""
			}
			// Replies can arrive while later parts of the same input are still being sent.
			from = len(compact())
			shown := make([]int, len(s.Send)) // times each part's text was on the screen before any write
			for j, part := range s.Send {
				if want := echoText(part); want != "" {
					shown[j] = strings.Count(compact(), want)
				}
			}
			for j, part := range s.Send {
				if want := echoText(s.Send[max(j-1, 0)]); j > 0 && s.EchoGate && want != "" {
					out := awaitEcho(func() bool { return echoed(compact(), want, shown[j-1]) },
						func() int { return len(compact()) }, orDefault(s.EchoQuietMS, 300), orDefault(s.EchoMaxMS, 3000), time.Now, time.Sleep)
					fmt.Printf("%secho=%d seen=%v quiet=%v after_ms=%d\n", phase, i, out.Seen, out.Quiet, out.Elapsed.Milliseconds())
					mark(fmt.Sprintf("echo%s%d", phase, i), fmt.Sprintf("seen=%v quiet=%v after_ms=%d", out.Seen, out.Quiet, out.Elapsed.Milliseconds()))
				} else if j > 0 {
					time.Sleep(300 * time.Millisecond)
				}
				if err := write(part); err != nil {
					fmt.Fprintf(os.Stderr, "write: %v\n", err)
					return "write"
				}
			}
			if s.ConfirmFile != "" || s.confirmScreen != nil {
				submitFrom := from
				out := awaitSubmit(submitCheck{Window: time.Duration(s.ConfirmMS) * time.Millisecond, Resend: s.ConfirmResend},
					func() string {
						if s.ConfirmFile != "" {
							if ok, _ := readyFile(s.ConfirmFile); ok {
								return "file"
							}
						}
						if s.confirmScreen != nil {
							if cur := compact(); s.confirmScreen.MatchString(cur[min(submitFrom, len(cur)):]) {
								return "screen"
							}
						}
						return ""
					},
					func() {
						write("\r")
						mark(fmt.Sprintf("resend%s%d", phase, i), "")
					}, time.Now, time.Sleep)
				fmt.Printf("%ssubmit=%d confirmed=%v by=%q after_ms=%d resent=%v resent_at_ms=%d\n", phase, i, out.Confirmed, out.By, out.Elapsed.Milliseconds(), out.Resent, out.ResentAt.Milliseconds())
				mark(fmt.Sprintf("submit%s%d", phase, i), fmt.Sprintf("confirmed=%v by=%s after_ms=%d resent=%v", out.Confirmed, out.By, out.Elapsed.Milliseconds(), out.Resent))
			}
		}
		return ""
	}
	failed := runSteps(steps, "", 0)
	if failed != "" && len(cleanup) > 0 && !exited(pi.Process) {
		cleanupFailed := "wait_file"
		if err := signalsAbsent(cleanup); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup signal: %v\n", err)
		} else {
			cleanupFailed = runSteps(cleanup, "cleanup_", len(compact()))
		}
		fmt.Printf("cleanup_error=%q\n", cleanupFailed)
	}
	wait := uint32(*total * 1000)
	if !exitEnd.IsZero() {
		if left := time.Until(exitEnd); left < time.Duration(wait)*time.Millisecond {
			wait = uint32(max(left, 0) / time.Millisecond)
		}
	}
	if failed != "" {
		wait = min(wait, 5000)
	}
	event, _ := syscall.WaitForSingleObject(pi.Process, wait)
	var code uint32 = 1<<32 - 1
	if event == syscall.WAIT_OBJECT_0 {
		syscall.GetExitCodeProcess(pi.Process, &code)
	} else {
		syscall.TerminateProcess(pi.Process, 1)
	}
	holdError := ""
	if holdSignal != "" {
		released := false
		if event == syscall.WAIT_OBJECT_0 {
			for time.Now().Before(holdEnd) {
				ready, err := readyFile(holdSignal)
				if err != nil {
					holdError = "signal"
					break
				}
				if ready {
					released = true
					break
				}
				time.Sleep(min(50*time.Millisecond, time.Until(holdEnd)))
			}
			if !released && holdError == "" {
				holdError = "deadline"
			}
		} else {
			holdError = "child_unconfirmed"
		}
		fmt.Printf("hold_console_released=%t hold_error=%q\n", released, holdError)
	}
	time.Sleep(time.Second)
	if !closeConsole() {
		fmt.Fprintln(os.Stderr, "ClosePseudoConsole deadline")
		os.Exit(1)
	}
	syscall.CloseHandle(inWrite)
	writeText()
	fmt.Printf("child_exit=%d failed_wait=%q elapsed=%.1fs\n", int32(code), failed, time.Since(started).Seconds())
	if failed != "" || code != 0 || holdError != "" {
		os.Exit(1)
	}
}

// echoText is a write's text as the screen shows it: the control sequences and all whitespace
// removed (the TUI places words with cursor moves). Empty for a key such as Enter.
func echoText(write string) string {
	return strings.Join(strings.Fields(control.ReplaceAllString(write, "")), "")
}

const screenTail = 4000

// driverReserve is how long before -deadline the steps stop: the failure path waits up to 5
// seconds for the child, then closes the console (1 second) and writes the screen.
const driverReserve = 8 * time.Second

// untilGrace is how long a step's Until screen must show before the step is skipped, so a
// dialog drawn just after the main prompt's text is not missed.
const untilGrace = 600 * time.Millisecond

func loadSteps(name string, previous []step) ([]step, error) {
	raw, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var steps []step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, err
	}
	previousSignals := make(map[string]bool)
	for _, s := range previous {
		if s.WaitFile != "" {
			previousSignals[strings.ToLower(filepath.Clean(s.WaitFile))] = true
		}
	}
	holdCount := 0
	for i := range steps {
		s := &steps[i]
		if s.CloseConsole && (i != len(steps)-1 || s.Optional || s.HoldConsoleAfterExit || len(s.Send) > 0 || s.Wait == "" && s.WaitFile == "" || s.Timeout <= 0 || s.ConfirmFile != "" || s.ConfirmScreen != "" || s.Until != "") {
			return nil, fmt.Errorf("close_console requires one final required observed step without input or console hold")
		}
		if s.HoldConsoleAfterExit {
			holdCount++
			if s.WaitFile == "" || s.Optional || holdCount != 1 || i != len(steps)-1 {
				return nil, fmt.Errorf("one final required wait_file may hold the console")
			}
		}
		if s.Timeout < 0 || int64(s.Timeout) > (1<<63-1)/int64(time.Second) || s.After < 0 || int64(s.After) > (1<<63-1)/int64(time.Millisecond) {
			return nil, fmt.Errorf("step duration out of range")
		}
		s.pattern, err = regexp.Compile(s.Wait)
		if err != nil {
			return nil, err
		}
		if s.Until != "" {
			if s.until, err = regexp.Compile(s.Until); err != nil {
				return nil, err
			}
		}
		if s.ConfirmScreen != "" {
			if s.confirmScreen, err = regexp.Compile(s.ConfirmScreen); err != nil {
				return nil, err
			}
		}
		if (s.ConfirmFile != "" || s.ConfirmScreen != "") && s.ConfirmMS <= 0 {
			return nil, fmt.Errorf("a submission check needs a positive confirm_ms")
		}
		if s.ConfirmFile != "" && !filepath.IsAbs(s.ConfirmFile) {
			return nil, fmt.Errorf("confirm_file must be absolute")
		}
		if s.WaitFile == "" {
			continue
		}
		if !filepath.IsAbs(s.WaitFile) {
			return nil, fmt.Errorf("wait_file must be absolute")
		}
		if clean := filepath.Clean(s.WaitFile); strings.HasPrefix(clean, `\\?\`) || strings.HasPrefix(clean, `\\.\`) {
			return nil, fmt.Errorf("wait_file must not use a device namespace")
		}
		name := filepath.FromSlash(s.WaitFile)
		for _, part := range strings.Split(name, string(filepath.Separator)) {
			if part == "" || part == "." || part == ".." {
				continue
			}
			if strings.TrimRight(part, ". ") != part {
				return nil, fmt.Errorf("wait_file contains an ambiguous component")
			}
		}
		if !filepath.IsLocal(strings.TrimLeft(name[len(filepath.VolumeName(name)):], string(filepath.Separator))) {
			return nil, fmt.Errorf("wait_file contains a reserved component")
		}
		if previousSignals[strings.ToLower(filepath.Clean(s.WaitFile))] {
			return nil, fmt.Errorf("cleanup wait_file reused from primary script")
		}
	}
	if err := signalsAbsent(steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func signalsAbsent(steps []step) error {
	for _, s := range steps {
		if s.WaitFile == "" {
			continue
		}
		_, err := os.Lstat(s.WaitFile)
		if err == nil {
			return fmt.Errorf("wait_file already exists")
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func readyFile(name string) (bool, error) {
	if name == "" {
		return true, nil
	}
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("wait_file is not a regular file")
	}
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 7))
	return string(raw) == "ready\n", err
}

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
