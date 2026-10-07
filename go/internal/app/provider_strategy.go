package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

// Provider strategies (#320) hold native to this session's gateway against a stored Claude apps
// gateway login, which otherwise outranks the launch's own endpoint (#299). Each sets one native
// flag in the process environment, the settings env and the background respawn settings, and
// gives native's control processes the launch's own provider environment.
const (
	StrategyHostManaged      = "HOST_MANAGED"
	StrategyGatewayEnvCompat = "GATEWAY_ENV_COMPAT"
)

// ErrProviderStrategyUnknown refuses a strategy this build does not define; it never falls back
// to another one or to none.
var ErrProviderStrategyUnknown = errors.New("PROVIDER_STRATEGY_UNKNOWN")

// ErrSupervisorUnverified refuses a background launch whose native supervisor this launch did
// not start: its provider settings are unknown.
var ErrSupervisorUnverified = errors.New("ROUTING_PROVIDER_UNVERIFIED: a native background supervisor this launch did not start is running for this configuration directory, so its provider settings are unknown. Nothing was dispatched")

// ErrBackgroundUnverified refuses a background launch under a strategy: on the measured native
// (2.1.292, Windows) a session native's supervisor starts does not receive the launch's endpoint
// under HOST_MANAGED, and under GATEWAY_ENV_COMPAT it still reads a stored gateway login
// (#320 R08). Refused before anything starts; not a permanent removal of background sessions.
var ErrBackgroundUnverified = errors.New("ROUTING_PROVIDER_UNVERIFIED: background sessions are not verified under this launch's provider control on this Claude Code version, so --bg is refused for now. Nothing was started; run the session in the foreground")

// agentViewOff turns off native's agent view under a strategy, so /background, /bg and /fork stay
// in the foreground process instead of reaching native's supervisor, and claude agents is
// unavailable (code.claude.com/docs/en/agent-view).
const agentViewOff = "CLAUDE_CODE_DISABLE_AGENT_VIEW"

// userKeepsAgentViewOff refuses user --settings whose env sets agentViewOff to anything but "1".
func userKeepsAgentViewOff(fields map[string]json.RawMessage) error {
	if fields["env"] == nil {
		return nil
	}
	var env map[string]any
	if json.Unmarshal(fields["env"], &env) != nil {
		return errSettingsConflict
	}
	for key, value := range env {
		if strings.EqualFold(key, agentViewOff) && value != "1" {
			return errSettingsConflict
		}
	}
	return nil
}

// strategyFlag is native's flag for a strategy, "" for none.
func strategyFlag(strategy string) (string, error) {
	switch strategy {
	case "":
		return "", nil
	case StrategyHostManaged:
		return launch.HostRoutedEnv, nil
	case StrategyGatewayEnvCompat:
		return "CLAUDE_CODE_USE_GATEWAY", nil
	}
	return "", ErrProviderStrategyUnknown
}

// withProcessFlag returns env with flag set to "1" and no other spelling of it.
func withProcessFlag(env []string, flag string) []string {
	if flag == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if key, _, _ := strings.Cut(entry, "="); !strings.EqualFold(key, flag) {
			out = append(out, entry)
		}
	}
	return append(out, flag+"=1")
}

var supervisorLockPID = regexp.MustCompile(`"pid"\s*:\s*([0-9]+)`)

// supervisorLockOwner is the live process native's supervisor lock in configDir names, or 0.
func supervisorLockOwner(configDir string) int {
	raw, err := os.ReadFile(filepath.Join(configDir, "daemon.lock"))
	if err != nil {
		return 0
	}
	m := supervisorLockPID.FindSubmatch(raw)
	if m == nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(m[1]))
	if pid <= 0 || !processAlive(pid) {
		return 0
	}
	return pid
}

func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	return syscall.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

// childOfThisProcess reports whether pid's parent is this process and it started at or after
// since, so a reused number is not taken for the supervisor this launch started.
func childOfThisProcess(pid int, since time.Time) bool {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	parent := uint32(0)
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(pid) {
			parent = entry.ParentProcessID
			break
		}
	}
	if parent != uint32(os.Getpid()) {
		return false
	}
	h, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	return syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil &&
		!time.Unix(0, created.Nanoseconds()).Before(since.Add(-time.Second))
}

// startSupervisor starts native's background supervisor for configDir from env, the launch's
// own process environment: native does not hand provider settings to a supervisor it starts
// itself under HOST_MANAGED (measured 2.1.292, #320 R08), and one started that way reached the
// stored gateway. The supervisor is this launch's child, ended with it. A supervisor already
// running for the directory is refused, as is one whose lock does not name the child started.
func startSupervisor(start func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error), exe, cwd, configDir string, env []string, wait time.Duration) (Process, error) {
	if configDir == "" {
		return nil, errors.New("BACKGROUND_SUPERVISOR_UNAVAILABLE: the configuration directory is unknown")
	}
	if supervisorLockOwner(configDir) != 0 {
		return nil, ErrSupervisorUnverified
	}
	began := time.Now()
	process, err := start(launch.Spec{File: exe, Args: []string{"daemon", "run"}, Dir: cwd, Env: env}, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if pid := supervisorLockOwner(configDir); pid != 0 {
			if childOfThisProcess(pid, began) {
				return process, nil
			}
			process.Stop()
			process.Wait()
			return nil, ErrSupervisorUnverified
		}
		time.Sleep(50 * time.Millisecond)
	}
	process.Stop()
	process.Wait()
	return nil, errors.New("BACKGROUND_SUPERVISOR_UNAVAILABLE: native's background supervisor did not start")
}

// managedAgentViewOverride refuses a strategy launch whose machine policy turns agent view back
// on: managed settings outrank the launch's --settings, so the restriction would not hold and
// /background could reach native's supervisor (#320, admission contract r2 BE15's equivalent
// gate). Only "1" for the env variable and true for the settings key leave it off.
func managedAgentViewOverride(docs []policyDocument) error {
	for _, doc := range docs {
		var fields struct {
			DisableAgentView *bool          `json:"disableAgentView"`
			Env              map[string]any `json:"env"`
		}
		if json.Unmarshal([]byte(doc.JSON), &fields) != nil {
			return fmt.Errorf("%w: %s is not a settings object", errPolicyUnreadable, doc.Source)
		}
		if fields.DisableAgentView != nil && !*fields.DisableAgentView {
			return fmt.Errorf("%w: %s sets disableAgentView false", ErrAgentViewManagedOverride, doc.Source)
		}
		for name, value := range fields.Env {
			if strings.EqualFold(name, agentViewOff) && value != "1" {
				return fmt.Errorf("%w: %s sets %s", ErrAgentViewManagedOverride, doc.Source, agentViewOff)
			}
		}
	}
	return nil
}

// ErrAgentViewManagedOverride refuses a strategy launch whose machine policy turns agent view on.
var ErrAgentViewManagedOverride = errors.New("AGENT_VIEW_MANAGED_OVERRIDE_REFUSED: machine managed settings turn agent view back on, which this launch's provider control cannot hold; nothing was started")
