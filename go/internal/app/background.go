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
	// ForkedFrom is the session a --resume <id> --fork-session --bg job copied, unchanged by it.
	ForkedFrom string `json:"forkedFrom,omitempty"`
}

func BackgroundRequested(args []string) bool {
	// Native 2.1.282 dispatches exact background tokens before option parsing,
	// including after -- or --name and alongside --help/--version. Match that
	// dispatch boundary or the launcher closes a live worker's gateway.
	return slices.Contains(args, "--bg") || slices.Contains(args, "--background")
}

// backgroundResume checks a background resume. Native starts a copy of a background session
// it is asked to resume with flags, and this launcher always passes flags (--settings,
// --plugin-dir), so the copy's SessionStart is a fork that a resumed profile does not accept
// (#319 decision 17, product17-bg L4: SESSION_PROFILE_UNVERIFIED). The copy is therefore the
// user's explicit choice, native's own --fork-session: without it the launch is refused rather
// than turned into a fork the user did not ask for. With it the job continues a new session
// whose conversation starts from the original, which stays unchanged (L4f, L5f); forkedFrom
// names the original for the launcher's report. A target that is not a session UUID (a job
// id) is refused: its copy ran no hook and never reached the gateway (L4j). --continue is
// refused too: native copies the directory's latest session, which this launcher cannot name
// (L7). resumeID is prepareSessionArgs'. Options are read as prepareSessionArgs reads them,
// combined and attached short forms included; past an option of unknown arity nothing is
// known, and the gateway refuses what native starts.
func backgroundResume(args []string, resumeID string) (forkedFrom string, err error) {
	resume, continued, fork := false, false, false
	for i := 0; i < len(args) && args[i] != "--"; {
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
		return "", ErrBackgroundContinue
	case !resume:
		return "", nil
	case resumeID == "":
		return "", ErrBackgroundResumeTarget
	case !fork:
		return "", ErrBackgroundResumeNeedsFork
	}
	return resumeID, nil
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
	// forkedFrom is the session a background --fork-session resume copied (backgroundResume).
	forkedFrom string
	// start, when set, starts native's control commands as the launch starts native.
	start func(launch.Spec, io.Reader, io.Writer, io.Writer) (Process, error)
	// crash, set only by a test, ends the wait without stopping the native job, as the
	// resident process dying would (decision 18c X7).
	crash <-chan struct{}
	// job is set when this resident was regenerated for a job another resident dispatched:
	// there is no dispatch to wait for (#319 decision 18c).
	job string
	// record, when set, is written to home once the job exists and removed on every ending
	// this resident controls, so a later key helper can regenerate it.
	record *ResidentRecord
	home   string
	// session reports the gateway's current native session, kept in the record.
	session func() string
	// sessions reports every session registered on the gateway; their jobs join this one.
	sessions func() []string
	crashed  bool // the test crash ended the wait: the record stays, as after a real death
}

// noDispatch stands in for the dispatch process of a regenerated resident.
type noDispatch struct{}

func (noDispatch) Wait() error   { return nil }
func (noDispatch) ExitCode() int { return 0 }
func (noDispatch) Stop() error   { return nil }

func (p *backgroundProcess) Stop() error { p.cancel(); return p.Process.Stop() }
func (p *backgroundProcess) Wait() error {
	id := p.job
	if id == "" {
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
		id = matches[0][1]
	}
	// Every job on this link: a job started from the agent view of one runs
	// on the same settings, so on this gateway too (#319 decision 18h).
	jobs := map[string]int{id: 0}
	if p.record != nil {
		for _, job := range p.record.Jobs {
			jobs[job] = 0
		}
	}
	stopOne := func(id string) error {
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
	stopNative := func() error {
		var err error
		for job := range jobs {
			err = errors.Join(err, stopOne(job))
		}
		return err
	}
	defer p.cancel()
	if p.ctx.Err() != nil {
		return stopNative()
	}
	if p.record != nil {
		// Best effort: without the record this session still runs, but a resident that dies
		// cannot be regenerated for it.
		p.record.Job = id
		if writeResident(p.home, *p.record) == nil {
			defer func() {
				// A stopped job is still listed and can be opened again: its record stays for
				// the resident that opening regenerates (#319 decision 18h).
				if !p.crashed && !jobListed(p.config, p.record.Job, p.record.Jobs) {
					removeResident(p.home, p.record.Link)
				}
			}()
		}
	}
	if p.ready != nil {
		if err := p.ready(BackgroundSession{JobID: id, ConnectionID: p.link.ID, PID: os.Getpid(), ForkedFrom: p.forkedFrom}); err != nil {
			return errors.Join(err, stopNative())
		}
	} else {
		fmtBackground(p.stdout, BackgroundSession{JobID: id, ConnectionID: p.link.ID, ForkedFrom: p.forkedFrom})
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.crash:
			p.crashed = true
			return errors.New("BACKGROUND_TEST_CRASH")
		case <-p.ctx.Done():
			return stopNative()
		case <-p.link.Stopped:
			return stopNative()
		case <-p.link.Done:
			return errors.Join(sessionlink.ErrUnavailable, stopNative())
		case <-tick.C:
			if p.record != nil && p.session != nil {
				if session := p.session(); session != "" && session != p.record.Session {
					p.record.Session = session
					writeResident(p.home, *p.record)
				}
			}
			if p.sessions != nil {
				added := false
				for _, job := range nativeJobs(p.config, p.sessions()) {
					if _, known := jobs[job]; !known {
						jobs[job], added = 0, true
					}
				}
				if added && p.record != nil {
					p.record.Jobs = p.record.Jobs[:0]
					for job := range jobs {
						if job != id {
							p.record.Jobs = append(p.record.Jobs, job)
						}
					}
					slices.Sort(p.record.Jobs)
					writeResident(p.home, *p.record)
				}
			}
			for job, missing := range jobs {
				running, err := jobRunning(p.config, job)
				if err != nil {
					return errors.Join(errors.New("BACKGROUND_STATE_UNREADABLE"), stopNative())
				}
				if missing++; running {
					missing = 0
				}
				if jobs[job] = missing; missing >= 3 {
					delete(jobs, job)
				}
			}
			if len(jobs) == 0 {
				return nil
			}
		}
	}
}

// jobRunning reports whether a job may still use its gateway: native lists it and has not
// stopped it. Native owns this schema and all writes; only its state is read. A stopped job's
// worker has exited (measured 2.1.295); any other state, or a file being rewritten, counts as
// running.
func jobRunning(config, job string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(config, "jobs", job, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var state struct {
		State string `json:"state"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return true, nil
	}
	return state.State != "stopped", nil
}

// jobListed reports whether native still lists any of these jobs, stopped ones included.
func jobListed(config, job string, jobs []string) bool {
	for _, id := range append([]string{job}, jobs...) {
		if _, err := os.Stat(filepath.Join(config, "jobs", id, "state.json")); err == nil {
			return true
		}
	}
	return false
}

// nativeJobs names the jobs of these sessions that native lists now. A job is named by the
// first eight digits of the session it was started with (observed on 2.1.295 for dispatched,
// forked and agent view jobs); a session with no such job, as after /clear, adds nothing.
func nativeJobs(config string, sessions []string) []string {
	var out []string
	for _, session := range sessions {
		if len(session) < 8 || !residentJob.MatchString(session[:8]) {
			continue
		}
		if _, err := os.Stat(filepath.Join(config, "jobs", session[:8], "state.json")); err == nil {
			out = append(out, session[:8])
		}
	}
	return out
}

func fmtBackground(out io.Writer, s BackgroundSession) {
	io.WriteString(out, "Backgrounded as "+s.JobID+"\n")
	if s.ForkedFrom != "" {
		io.WriteString(out, "Forked from session "+s.ForkedFrom+": the job continues a new session that starts from its conversation; "+s.ForkedFrom+" itself is unchanged\n")
	}
	io.WriteString(out, "Clauduct connection: "+s.ConnectionID+"\n")
	io.WriteString(out, "Manage with claude agents; end this connection with clauduct --background-stop "+s.ConnectionID+"\n")
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
