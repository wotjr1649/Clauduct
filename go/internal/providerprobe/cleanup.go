package providerprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// p3CleanupNative is one native process the harness itself started to end a case's supervisor:
// the pid and creation time identify it, Run names the case and configuration that started it.
type p3CleanupNative struct {
	PID     uint32    `json:"pid"`
	Created time.Time `json:"created"`
	Run     string    `json:"run"`
	// Exited is the process's exact exit time, read from the retained process handle once it was
	// waited for. Zero while it may still run or when the handle could not say: a child is then
	// not proven to have had this generation as its parent. Never serialized.
	Exited time.Time `json:"-"`
}

// p3CleanupLog is every such process of a scope's runs, shared by its parallel cases. It is the
// only proof the verdict accepts that a connection to an earlier run's receiver was another
// case's cleanup and not this case's native.
type p3CleanupLog struct {
	mu      sync.Mutex
	natives []p3CleanupNative
}

// add records n and returns its index for exited.
func (l *p3CleanupLog) add(n p3CleanupNative) int {
	if l == nil {
		return -1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.natives = append(l.natives, n)
	return len(l.natives) - 1
}

// exited records the exact exit time of the process add returned i for.
func (l *p3CleanupLog) exited(i int, at time.Time) {
	if l == nil || i < 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < len(l.natives) {
		l.natives[i].Exited = at
	}
}

func (l *p3CleanupLog) all() []p3CleanupNative {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]p3CleanupNative(nil), l.natives...)
}

// cleanupEnvironment gives probe-owned control commands a bounded system environment and the
// same routing overlay as the case's native process.
func (w *p3World) cleanupEnvironment(configuration string, extra map[string]string) ([]string, error) {
	ambient := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, ok := strings.Cut(entry, "="); ok && name != "" {
			ambient[strings.ToUpper(name)] = value
		}
	}
	p3ScrubProbeEnv(ambient)

	// Keep only the process settings needed by the system. User and application paths below
	// always point into this world's temporary home, never the agent's shell environment.
	const systemEnv = "COMSPEC PATH PATHEXT SYSTEMROOT WINDIR OS PROCESSOR_ARCHITECTURE PROCESSOR_IDENTIFIER NUMBER_OF_PROCESSORS"
	env := map[string]string{}
	for _, name := range strings.Fields(systemEnv) {
		if value, ok := ambient[name]; ok {
			env[name] = value
		}
	}
	if w.home == "" || w.configDir == "" || w.managedDir == "" {
		return nil, errors.New("CLEANUP_WORLD_PATH_REQUIRED")
	}
	appData := filepath.Join(w.home, "AppData", "Roaming")
	localAppData := filepath.Join(w.home, "AppData", "Local")
	temp := filepath.Join(w.home, "Temp")
	for _, dir := range []string{appData, localAppData, temp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("CLEANUP_WORLD_DIR: %w", err)
		}
	}
	env["HOME"], env["USERPROFILE"] = w.home, w.home
	env["APPDATA"], env["LOCALAPPDATA"] = appData, localAppData
	env["TEMP"], env["TMP"] = temp, temp
	env["XDG_CONFIG_HOME"] = filepath.Join(w.home, ".config")
	env["XDG_CACHE_HOME"] = filepath.Join(w.home, ".cache")
	env["XDG_DATA_HOME"] = filepath.Join(w.home, ".local", "share")
	if drive := filepath.VolumeName(w.home); drive != "" {
		env["HOMEDRIVE"] = drive
		env["HOMEPATH"] = strings.TrimPrefix(w.home, drive)
	}
	env["CLAUDE_CONFIG_DIR"] = w.configDir
	// This command belongs to the probe, so its explicit flags are not a product overlay-gap observation.
	switch configuration {
	case p3HostManaged:
		env[launch.HostRoutedEnv] = "1"
	case p3GatewayEnv:
		env["CLAUDE_CODE_USE_GATEWAY"] = "1"
	case p3Unprotected, p3Baseline:
	default:
		return nil, fmt.Errorf("unknown cleanup configuration %q", configuration)
	}
	caseEnv, err := p3CleanupCaseEnv(extra)
	if err != nil {
		return nil, err
	}
	for name, value := range caseEnv {
		env[name] = value
	}
	// These paths are probe-owned. In particular, neither ambient nor case-provided
	// managed-settings paths can redirect the cleanup command.
	env["CLAUDE_CONFIG_DIR"] = w.configDir
	env["CLAUDE_CODE_MANAGED_SETTINGS_PATH"] = w.managedDir
	env["DISABLE_AUTOUPDATER"] = "1"
	entries := make([]string, 0, len(env)+4)
	for name, value := range env {
		entries = append(entries, name+"="+value)
	}
	spec, err := w.applyProbeEnv(launch.Spec{Env: entries}, configuration, nil)
	return spec.Env, err
}

// p3CleanupCaseEnv keeps only case inputs that the diagnostic deliberately adds to native.
// Credentials, arbitrary provider settings and user-selected paths never reach cleanup.
func p3CleanupCaseEnv(extra map[string]string) (map[string]string, error) {
	allowed := map[string]string{
		"ANTHROPIC_BASE_URL":            "ANTHROPIC_BASE_URL",
		"ANTHROPIC_BEDROCK_BASE_URL":    "ANTHROPIC_BEDROCK_BASE_URL",
		"ANTHROPIC_UNIX_SOCKET":         "ANTHROPIC_UNIX_SOCKET",
		"AWS_REGION":                    "AWS_REGION",
		"CLAUDE_CODE_SKIP_BEDROCK_AUTH": "CLAUDE_CODE_SKIP_BEDROCK_AUTH",
		"CLAUDE_CODE_USE_BEDROCK":       "CLAUDE_CODE_USE_BEDROCK",
		"CLAUDUCT_DIAG2_STOP_STUB":      "CLAUDUCT_DIAG2_STOP_STUB",
		"CLAUDUCT_SEQ_SIDE":             "CLAUDUCT_SEQ_SIDE",
	}
	caseEnv := make(map[string]string, len(extra))
	for name, value := range extra {
		canonical, ok := allowed[strings.ToUpper(name)]
		if !ok {
			continue
		}
		if _, duplicate := caseEnv[canonical]; duplicate {
			return nil, fmt.Errorf("CLEANUP_ENV_DUPLICATE_ALLOWED_NAME: %s", canonical)
		}
		caseEnv[canonical] = value
	}
	return caseEnv, nil
}

// p3StopOwnSupervisor runs with its world's prepared environment and records the cleanup generation.
func p3StopOwnSupervisor(exe, configDir string, env []string, log *p3CleanupLog, run string) error {
	if len(env) == 0 {
		return errors.New("CLEANUP_ENV_REQUIRED")
	}
	cmd := exec.Command(exe, "daemon", "stop", "--any")
	cmd.Env = slices.Clone(env)
	cmd.Dir = configDir
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err := p3RunRecorded(ctx, cmd, log, run)
	cancel()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// An incomplete census cannot show the dir empty.
		if left, complete := probeNativeCensus(exe, configDir, time.Time{}, nil); complete && len(left) == 0 {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("natives of the probe config dir still running, or not all readable, after the supervisor stopped")
}

// p3RunRecorded is childprocess.Run that records the process's pid and creation time right
// after it starts. A process too short-lived to be read is recorded with a zero creation time,
// which proves nothing about any connection it made.
func p3RunRecorded(ctx context.Context, cmd *exec.Cmd, log *p3CleanupLog, run string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := childprocess.Start(cmd)
	if err != nil {
		return err
	}
	p.RecordReap()
	n := p3CleanupNative{PID: uint32(p.PID()), Run: run}
	if id, err := p.Identity(); err == nil {
		n.Created = time.Unix(0, int64(id.Creation-116444736000000000)*100)
	}
	index := log.add(n)
	stop := context.AfterFunc(ctx, func() { _ = p.Stop() })
	defer stop()
	err = p.Wait()
	// The stop native's own Job reports to the run through the error: a held member or a Job not
	// confirmed empty is a cleanup failure.
	if held, _, reapErr := p.JobEmpty(); held > 0 || reapErr != nil {
		err = errors.Join(err, fmt.Errorf("HELD_PROCESS or JOB_UNCONFIRMED: %d held, %v", held, reapErr))
	}
	if !n.Created.IsZero() && cmd.ProcessState != nil {
		if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && usage != nil {
			log.exited(index, time.Unix(0, usage.ExitTime.Nanoseconds()))
		}
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return err
}

// p3PriorContact is one earlier run's receiver this case's window reached: which receiver, the
// run that created it, what it counted since this case began (bytes and connections apart) and
// the owners the Counter recorded for those connections. Tunnel counts CONNECTs that went
// through CATCH_ALL to the receiver's address, whose owner nothing records.
type p3PriorContact struct {
	Receiver string                   `json:"receiver"`
	URL      string                   `json:"url"`
	Source   string                   `json:"sourceRun"`
	Bytes    int64                    `json:"bytes"`
	Conns    int64                    `json:"conns"`
	Tunnel   int64                    `json:"tunnel,omitempty"`
	Total    int                      `json:"total,omitempty"`
	Owners   []providercert.ConnOwner `json:"owners,omitempty"`
}

// p3PriorCrossTalk reports whether every prior-receiver contact of the run self is accounted for
// by another case's cleanup native. Cross-talk is proven only when, for each contact, there is
// no tunnel, there is at least one connection, every connection has a recorded owner (the
// Counter keeps a bounded number), and every owner is a cleanup native another run started:
// the same pid and creation time, or a child whose parent pid is one and that was created within
// that process's exact lifetime (creation through the exit time its handle reported); a child of
// a pid whose generation exit is unknown is not proven. Anything else, no
// owner, an owner that is this run's own cleanup or a native nobody recorded, is not cross-talk
// and the contact fails routing. A contact of this case's own native is never cross-talk.
func p3PriorCrossTalk(contacts []p3PriorContact, cleanups []p3CleanupNative, self string) bool {
	if len(contacts) == 0 {
		return false
	}
	for _, c := range contacts {
		if c.Tunnel != 0 || c.Conns <= 0 || int64(len(c.Owners)) != c.Conns {
			return false
		}
		for _, o := range c.Owners {
			if o.PID == 0 || o.Created.IsZero() {
				return false
			}
			proven := false
			for _, n := range cleanups {
				if n.PID == 0 || n.Created.IsZero() {
					continue
				}
				child := o.ParentPID == n.PID && !n.Exited.IsZero() && !o.Created.Before(n.Created) && !o.Created.After(n.Exited)
				if (o.PID == n.PID && o.Created.Equal(n.Created)) || child {
					if n.Run == self {
						return false
					}
					proven = true
				}
			}
			if !proven {
				return false
			}
		}
	}
	return true
}
