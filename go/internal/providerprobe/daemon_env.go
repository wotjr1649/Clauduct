package providerprobe

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3DaemonFacts is what the probe records of the native daemon its own config dir started: its
// identity and, for the provider keys only, a class of each value. No value is recorded.
type p3DaemonFacts struct {
	PID       int               `json:"pid"`
	ParentPID uint32            `json:"parentPid"`
	Created   time.Time         `json:"created"`
	Pinned    bool              `json:"pinned"`
	Env       map[string]string `json:"env,omitempty"`
	Error     string            `json:"error,omitempty"`
}

var daemonLockPID = regexp.MustCompile(`"pid"\s*:\s*([0-9]+)`)

// p3EnvClasses are the keys whose value class is recorded.
var p3EnvClasses = []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST", "CLAUDE_CODE_USE_GATEWAY", "CLAUDE_CONFIG_DIR", "HTTPS_PROXY", "NO_PROXY", "NODE_EXTRA_CA_CERTS"}

// classify names a value without revealing it: absent, empty, "1", a loopback URL named by
// role, the probe's own path, or "set".
func classify(key, value string, present bool, roles map[string]string) string {
	switch {
	case !present:
		return "absent"
	case value == "":
		return "empty"
	case value == "1":
		return "1"
	}
	if role, ok := roles[value]; ok {
		return role
	}
	if u, err := url.Parse(value); err == nil && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost") {
		return "loopback-other"
	}
	if strings.HasSuffix(key, "TOKEN") || strings.HasSuffix(key, "KEY") {
		return "set"
	}
	return "set-other"
}

// p3NativeProcess is one native process of the probe's config dir, found by census.
type p3NativeProcess struct {
	PID       int               `json:"pid"`
	ParentPID uint32            `json:"parentPid"`
	Created   time.Time         `json:"created"`
	Command   string            `json:"command"`
	Env       map[string]string `json:"env"`
}

// p3NativeKey identifies one native process: a PID alone can be reused by a later process.
type p3NativeKey struct {
	pid     int
	created int64
}

// addNewNatives appends the processes of found not seen before, by PID and creation time.
func addNewNatives(seen map[p3NativeKey]bool, all, found []p3NativeProcess) []p3NativeProcess {
	for _, n := range found {
		key := p3NativeKey{n.PID, n.Created.UnixNano()}
		if !seen[key] {
			seen[key] = true
			all = append(all, n)
		}
	}
	return all
}

// inConfigDir reports whether env names configDir as CLAUDE_CONFIG_DIR.
func inConfigDir(env map[string]string, configDir string) bool {
	dir := ""
	for name, v := range env {
		if strings.EqualFold(name, "CLAUDE_CONFIG_DIR") {
			dir = v
		}
	}
	return strings.EqualFold(filepath.Clean(dir), filepath.Clean(configDir))
}

// probeNatives lists the processes running the pinned native image, started at or after since,
// whose environment names the probe's config dir: the probe's own natives and no one else's.
// Observation only.
func probeNatives(pinnedImage, configDir string, since time.Time, roles map[string]string) []p3NativeProcess {
	out, _ := probeNativeCensus(pinnedImage, configDir, since, roles)
	return out
}

// probeNativeCensus is probeNatives and whether the census is complete: false when the process
// snapshot failed or a live process with the native's file name could not be read (its image,
// times or environment). A process that ended before it was read is not a gap.
func probeNativeCensus(pinnedImage, configDir string, since time.Time, roles map[string]string) (_ []p3NativeProcess, complete bool) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	complete = true
	defer syscall.CloseHandle(snapshot)
	var out []p3NativeProcess
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), filepath.Base(pinnedImage)) {
			continue
		}
		pid := int(entry.ProcessID)
		image, err := imagePath(pid)
		if err != nil {
			complete = complete && p3ProcessGone(uint32(pid))
			continue
		}
		if !strings.EqualFold(image, pinnedImage) {
			continue
		}
		h, err := syscall.OpenProcess(0x0400|0x0010, false, uint32(pid))
		if err != nil {
			complete = complete && p3ProcessGone(uint32(pid))
			continue
		}
		var created, exited, kernel, user syscall.Filetime
		timesErr := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user)
		when := time.Unix(0, created.Nanoseconds())
		env, envErr := processEnvironment(h)
		syscall.CloseHandle(h)
		if timesErr != nil || envErr != nil {
			complete = complete && p3ProcessGone(uint32(pid))
			continue
		}
		if when.Before(since) {
			continue
		}
		if !inConfigDir(env, configDir) {
			continue
		}
		classes := map[string]string{}
		for _, key := range p3EnvClasses {
			value, present := "", false
			for name, v := range env {
				if strings.EqualFold(name, key) {
					value, present = v, true
				}
			}
			classes[key] = classify(key, value, present, roles)
		}
		out = append(out, p3NativeProcess{PID: pid, ParentPID: entry.ParentProcessID, Created: when, Command: providercert.CommandWord(uint32(pid)), Env: classes})
	}
	return out, complete
}

// p3ProcessGone reports a pid with no live process: it cannot be opened as one (ERROR_INVALID_PARAMETER)
// or has exited. A process that is denied to the probe is not gone.
var p3ProcessGone = func(pid uint32) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, pid)
	if err != nil {
		return err == syscall.Errno(87) // ERROR_INVALID_PARAMETER
	}
	defer syscall.CloseHandle(h)
	state, err := syscall.WaitForSingleObject(h, 0)
	return err == nil && state == syscall.WAIT_OBJECT_0
}

// ownDaemon reads the daemon named by the probe config dir's lock, confirms it runs the pinned
// native and started after since, and classifies its provider environment.
func ownDaemon(configDir string, pinned providercert.Identity, since time.Time, roles map[string]string) *p3DaemonFacts {
	raw, err := os.ReadFile(filepath.Join(configDir, "daemon.lock"))
	if err != nil {
		return &p3DaemonFacts{Error: "no daemon lock in the probe config dir"}
	}
	m := daemonLockPID.FindSubmatch(raw)
	if m == nil {
		return &p3DaemonFacts{Error: "daemon lock names no pid"}
	}
	pid, _ := strconv.Atoi(string(m[1]))
	f := &p3DaemonFacts{PID: pid}
	// PROCESS_QUERY_INFORMATION | PROCESS_VM_READ.
	h, err := syscall.OpenProcess(0x0400|0x0010, false, uint32(pid))
	if err != nil {
		f.Error = "open: " + err.Error()
		return f
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err == nil {
		f.Created = time.Unix(0, created.Nanoseconds())
	}
	if f.Created.Before(since) {
		f.Error = "the lock's process started before this run"
		return f
	}
	if image, err := imagePath(pid); err == nil {
		if id, err := providercert.Observe(image); err == nil {
			f.Pinned = id == pinned
		}
	}
	f.ParentPID = parentPID(uint32(pid))
	env, err := processEnvironment(h)
	if err != nil {
		f.Error = "environment: " + err.Error()
		return f
	}
	if !inConfigDir(env, configDir) {
		// The lock's PID may have been reused by another native after the daemon ended.
		f.Pinned = false
		f.Error = "the lock's process is not of the probe config dir"
		return f
	}
	if providercert.CommandWord(uint32(pid)) != "daemon" {
		// A worker or pty host of the same config dir may have taken the PID.
		f.Pinned = false
		f.Error = "the lock's process is not a daemon"
		return f
	}
	f.Env = map[string]string{}
	for _, key := range p3EnvClasses {
		value, present := "", false
		for name, v := range env {
			if strings.EqualFold(name, key) {
				value, present = v, true
			}
		}
		f.Env[key] = classify(key, value, present, roles)
	}
	return f
}

func parentPID(pid uint32) uint32 {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return entry.ParentProcessID
		}
	}
	return 0
}

var (
	ntQueryInformationProcess = syscall.NewLazyDLL("ntdll.dll").NewProc("NtQueryInformationProcess")
	readProcessMemory         = syscall.NewLazyDLL("kernel32.dll").NewProc("ReadProcessMemory")
)

func readMemory(h syscall.Handle, addr uintptr, buf []byte) error {
	var n uintptr
	if r, _, err := readProcessMemory.Call(uintptr(h), addr, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); r == 0 {
		return err
	}
	if int(n) != len(buf) {
		return fmt.Errorf("short read")
	}
	return nil
}

// processEnvironment reads another 64-bit process's environment block from its PEB: PEB+0x20
// is ProcessParameters, whose Environment is at +0x80 and EnvironmentSize at +0x3F0.
func processEnvironment(h syscall.Handle) (map[string]string, error) {
	var info [6]uintptr // PROCESS_BASIC_INFORMATION
	var size uint32
	if r, _, _ := ntQueryInformationProcess.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&info[0])), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&size))); r != 0 {
		return nil, fmt.Errorf("NtQueryInformationProcess %#x", r)
	}
	word := make([]byte, 8)
	if err := readMemory(h, info[1]+0x20, word); err != nil {
		return nil, err
	}
	params := *(*uintptr)(unsafe.Pointer(&word[0]))
	if err := readMemory(h, params+0x80, word); err != nil {
		return nil, err
	}
	block := *(*uintptr)(unsafe.Pointer(&word[0]))
	if err := readMemory(h, params+0x3F0, word); err != nil {
		return nil, err
	}
	length := *(*uint64)(unsafe.Pointer(&word[0]))
	if length == 0 || length > 1<<20 {
		return nil, fmt.Errorf("environment size %d", length)
	}
	buf := make([]byte, length)
	if err := readMemory(h, block, buf); err != nil {
		return nil, err
	}
	units := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), len(buf)/2)
	env := map[string]string{}
	// The block is NUL-separated and ends with an empty entry.
	start := 0
	for i, u := range units {
		if u != 0 {
			continue
		}
		if i == start {
			break
		}
		entry := syscall.UTF16ToString(units[start:i])
		if name, value, ok := strings.Cut(entry, "="); ok && name != "" {
			env[name] = value
		}
		start = i + 1
	}
	return env, nil
}

// p3SelfEnvironmentCheck reads this process's own block the way ownDaemon reads the daemon's.
func p3SelfEnvironmentCheck() error {
	os.Setenv("CLAUDUCT_P3_ENV_SELF_CHECK", "p3-self")
	h, err := syscall.OpenProcess(0x0400|0x0010, false, uint32(os.Getpid()))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	env, err := processEnvironment(h)
	if err != nil {
		return err
	}
	if env["CLAUDUCT_P3_ENV_SELF_CHECK"] != "p3-self" || env["Path"]+env["PATH"] == "" {
		return fmt.Errorf("read %d entries without the marker or PATH", len(env))
	}
	return nil
}
