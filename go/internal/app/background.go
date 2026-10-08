package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
)

// BackgroundSession identifies a public connection, not a credential. Native owns JobID; the
// connection only lasts for this Windows login and this resident process.
type BackgroundSession struct {
	JobID        string `json:"jobId"`
	ConnectionID string `json:"connectionId"`
	PID          int    `json:"pid"`
}

func BackgroundRequested(args []string) bool {
	// Native 2.1.282 dispatches exact background tokens before option parsing,
	// including after -- or --name and alongside --help/--version. Match that
	// dispatch boundary or the launcher closes a live worker's gateway.
	return slices.Contains(args, "--bg") || slices.Contains(args, "--background")
}

// backgroundResume makes a background resume an explicit fork. Native starts a copy of a
// background session it is asked to resume with flags, and this launcher always passes flags
// (--settings, --plugin-dir), so the copy's SessionStart is a fork that the resumed profile
// does not accept (#319 decision 17, product17-bg L4: SESSION_PROFILE_UNVERIFIED). Declared
// a fork, the resume of a background session and of a regular one both start from the
// resumed profile (L4f, L5f). A target that is not a session UUID (a job id) is refused: its
// copy ran no hook and never reached the gateway (L4j). --continue is refused too: native
// copies the directory's latest session, which this launcher cannot name, so the copy's fork
// has no profile to start from (L7: SESSION_PROFILE_UNVERIFIED). resumeID is prepareSessionArgs'.
// The flag goes where "--" is, or last, so the --settings positions run.go recorded stay valid.
// Options are read as prepareSessionArgs reads them, combined and attached short forms included;
// past an option of unknown arity nothing is known, and the gateway refuses what native starts.
func backgroundResume(args []string, resumeID string) ([]string, error) {
	resume, continued, fork := false, false, false
	terminator := len(args)
	for i := 0; i < len(args); {
		if args[i] == "--" {
			terminator = i
			break
		}
		end, known := nativeArgEnd(args, i)
		if !known || end > len(args) {
			break
		}
		name, _, _ := strings.Cut(args[i], "=")
		switch {
		case name == "--resume":
			resume = true
		case name == "--continue":
			continued = true
		case name == "--fork-session":
			fork = true
		case len(name) > 1 && name[0] == '-' && name[1] != '-':
			for _, c := range name[1:] {
				if c == 'r' {
					resume = true
					break
				}
				if c == 'c' {
					continued = true
				}
				if !strings.ContainsRune("cpvh", c) {
					break
				}
			}
		}
		i = end
	}
	switch {
	case continued:
		return nil, ErrBackgroundContinue
	case !resume:
		return args, nil
	case resumeID == "":
		return nil, ErrBackgroundResumeTarget
	case fork:
		return args, nil
	}
	return slices.Insert(slices.Clone(args), terminator, "--fork-session"), nil
}

func (config ClauductSettings) backgroundSettings(settings, hook, id, base string, inherited map[string]string, strategyFlag string) (string, error) {
	var s childSettings
	if json.Unmarshal([]byte(settings), &s) != nil || hook == "" {
		return "", errSettingsConflict
	}
	for _, matchers := range s.Hooks {
		for i := range matchers {
			for j := range matchers[i].Hooks {
				matchers[i].Hooks[j].Command += " " + id
				if strategyFlag != "" {
					matchers[i].Hooks[j].Command += " " + hookcmd.ProviderControlledArg
				}
			}
		}
	}
	env := config.sessionEnvironment()
	// Only bounded scalar preferences may be persisted from the launch shell.
	// Never serialize arbitrary environment entries into native's respawn flags.
	for name, value := range inherited {
		key := strings.ToUpper(name)
		if _, known := env[key]; known && !strings.HasPrefix(key, "ANTHROPIC_") {
			if !backgroundScalar.MatchString(value) {
				return "", errSettingsConflict
			}
			env[key] = value
		}
	}
	for k, v := range config.sessionRequirements() {
		env[k] = v
	}
	for k, v := range routingSettingsEnv(inherited, base) {
		env[k] = v
	}
	env["ANTHROPIC_BASE_URL"] = base
	env["ANTHROPIC_AUTH_TOKEN"], env["ANTHROPIC_API_KEY"], env["ANTHROPIC_CUSTOM_HEADERS"], env["CLAUDE_CODE_OAUTH_TOKEN"] = "", "", "", ""
	env["CLAUDE_CODE_ENABLE_FUNCTION_HOOKS"] = "1"
	if strategyFlag != "" {
		env[strategyFlag] = "1"
	}
	s.Env = env
	s.APIKeyHelper = `"` + filepath.ToSlash(hook) + `" --clauduct-background-key ` + id
	b, err := json.Marshal(s)
	return string(b), err
}

type backgroundOutput struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *backgroundOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Len()+len(p) > 64<<10 {
		return 0, errors.New("BACKGROUND_DISPATCH_OUTPUT_LIMIT")
	}
	return b.Buffer.Write(p)
}
func (b *backgroundOutput) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.String() }

var backgroundID = regexp.MustCompile(`(?m)^\s*claude attach ([a-f0-9]{8})\s+open in this terminal\s*$`)
var backgroundScalar = regexp.MustCompile(`^(|true|false|auto(:[0-9]{1,3})?|[0-9]{1,10}(\.[0-9]{1,6})?)$`)

// The ordinary job still owns the dispatch client. Once it exits, the native
// supervisor owns the worker; this wrapper keeps only its connection alive.
type backgroundProcess struct {
	Process
	link             *sessionlink.Server
	ctx              context.Context
	cancel           context.CancelFunc
	exe, cwd, config string
	env              []string
	output           *backgroundOutput
	ready            func(BackgroundSession) error
	stdout           io.Writer
	stderr           io.Writer
	// start, when set, starts native's control commands as the launch starts native.
	start func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error)
}

func (p *backgroundProcess) Stop() error { p.cancel(); return p.Process.Stop() }
func (p *backgroundProcess) Wait() error {
	err := p.Process.Wait()
	if err != nil {
		io.WriteString(p.stderr, p.output.text())
		return err
	}
	matches := backgroundID.FindAllStringSubmatch(p.output.text(), -1)
	if len(matches) != 1 {
		io.WriteString(p.stderr, p.output.text())
		return errors.New("BACKGROUND_DISPATCH_UNVERIFIED")
	}
	id := matches[0][1]
	stopNative := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if p.start != nil {
			process, err := p.start(launch.Spec{File: p.exe, Args: []string{"stop", id}, Dir: p.cwd, Env: p.env}, nil, nil, nil)
			if err != nil {
				return errors.New("BACKGROUND_NATIVE_STOP_FAILED")
			}
			done := make(chan error, 1)
			go func() { done <- process.Wait() }()
			select {
			case err = <-done:
			case <-ctx.Done():
				process.Stop()
				<-done
				err = ctx.Err()
			}
			if err != nil {
				return errors.New("BACKGROUND_NATIVE_STOP_FAILED")
			}
			return nil
		}
		cmd := exec.Command(p.exe, "stop", id)
		cmd.Dir, cmd.Env = p.cwd, p.env
		if childprocess.Run(ctx, cmd) != nil {
			return errors.New("BACKGROUND_NATIVE_STOP_FAILED")
		}
		return nil
	}
	defer p.cancel()
	if p.ctx.Err() != nil {
		return stopNative()
	}
	if p.ready != nil {
		if err := p.ready(BackgroundSession{JobID: id, ConnectionID: p.link.ID, PID: os.Getpid()}); err != nil {
			return errors.Join(err, stopNative())
		}
	} else {
		fmtBackground(p.stdout, id, p.link.ID)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	missing := 0
	for {
		select {
		case <-p.ctx.Done():
			return stopNative()
		case <-p.link.Stopped:
			return stopNative()
		case <-p.link.Done:
			return errors.Join(sessionlink.ErrUnavailable, stopNative())
		case <-tick.C:
			// Observe existence only. Native owns this state schema and all writes.
			_, err := os.Stat(filepath.Join(p.config, "jobs", id, "state.json"))
			if errors.Is(err, os.ErrNotExist) {
				missing++
			} else if err != nil {
				return errors.Join(errors.New("BACKGROUND_STATE_UNREADABLE"), stopNative())
			} else {
				missing = 0
			}
			if missing >= 3 {
				return nil
			}
		}
	}
}

func fmtBackground(out io.Writer, id, connection string) {
	io.WriteString(out, "Backgrounded as "+id+"\nClauduct connection: "+connection+"\n")
	io.WriteString(out, "Manage with claude agents; end this connection with clauduct --background-stop "+connection+"\n")
}

func backgroundControlEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "ANTHROPIC_") && !strings.EqualFold(key, "CLAUDE_CODE_OAUTH_TOKEN") {
			out = append(out, entry)
		}
	}
	return out
}
