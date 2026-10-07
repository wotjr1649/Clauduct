package providerprobe

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	conpty "github.com/wotjr1649/Clauduct/go/internal/ptydrive"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
)

// Contract r3 variants.
const (
	// p3HookDelayed runs the real hook only after its 1-second timeout has passed, so whatever
	// it posts arrives late.
	p3HookDelayed = "delayed"

	p3PluginAbsent  = "absent"  // no native-events plugin: no origin proof
	p3PluginStale   = "stale"   // the helper names a closed session link
	p3PluginForeign = "foreign" // the helper names another live session's link
	p3PluginForged  = "forged"  // the helper claims a connection it never made

	// The test binary's own roles, which the probe's hook commands and plugin may name.
	p3HookWrapperArg  = "--p3-hook-wrapper"
	p3ForgedHelperArg = "--p3-forged-helper"

	// p3HookDelay is past the hook's 1-second timeout and shorter than the wait native gives
	// the next turn's own delayed hook, so a late post lands inside a later turn.
	p3HookDelay = 1500 * time.Millisecond

	// Contract r4 continue entries: a refused --continue, the refusal followed by the --resume it
	// names, and a --continue of a session with no stored profile.
	p3ContinueRefusal = "continue-refusal"
	p3ContinueRecover = "continue-recover"
	p3ContinueLegacy  = "continue-legacy"

	// p3RestoreModel and p3RestoreEffort start R07m's session on a pair that is not the
	// default, so a --resume that restores the snapshot is told apart from one that does not.
	p3RestoreModel  = "gpt-6-luna"
	p3RestoreEffort = "low"
)

// p3Role runs one of the test binary's probe roles when argv names it.
func p3Role(argv []string) (int, bool) {
	if len(argv) > 1 && argv[1] == p3PtyDriveArg {
		os.Args = append([]string{argv[0]}, argv[2:]...)
		conpty.Main() // exits
		return 0, true
	}
	if len(argv) > 1 && argv[1] == p3HookWrapperArg {
		return p3HookWrapper(argv[2:]), true
	}
	if len(argv) > 1 && argv[1] == p3ForgedHelperArg {
		// A helper that announces the connection without making one: the gateway never sees
		// the confirmation, so the request it covers must be refused.
		io.Copy(io.Discard, io.LimitReader(os.Stdin, 8192))
		os.Stdout.WriteString("CLAUDUCT_CONFIRMATION_CONNECTED\n")
		time.Sleep(15 * time.Second) // well inside a run's deadline; native ends it sooner
		return 0, true
	}
	return 0, false
}

// p3WrapperRecord is one hook wrapper invocation. Names and booleans only: no URL, no token.
type p3WrapperRecord struct {
	At    time.Time
	ID    string // pairs an invocation's start and finish
	Mode  string
	Event string
	Phase string // "start" or "finish"
	// Epoch is the settings marker native's environment carried, if any.
	Epoch string
	// EnvMatchesPinned is native's environment naming the session's own gateway, with no
	// socket or provider switch; Differs names what did not.
	EnvMatchesPinned bool
	Differs          []string
	// ProofMissing and Mismatch are what the real hook reported (#299 and #295).
	ProofMissing, Mismatch bool
	Exit                   int
}

func appendWrapped(sidecar string, r p3WrapperRecord) {
	line, _ := json.Marshal(r)
	f, err := os.OpenFile(sidecar, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

func readWrapped(sidecar string) []p3WrapperRecord {
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		return nil
	}
	var out []p3WrapperRecord
	for _, line := range bytes.Split(raw, []byte("\n")) {
		var r p3WrapperRecord
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// p3HookWrapper is the probe's command-hook wrapper: args are the mode, the sidecar path and
// the real hook's argv. "delay" starts the real hook only after p3HookDelay. "observe" records
// whether native's environment still names the session's gateway, then runs the real hook
// with Clauduct's routing blocks (#295 and #299) turned into observations: the environment the
// real hook sees names the session's gateway and the hook runs as provider-controlled. Origin,
// confirmation, registration and every policy check run unchanged.
func p3HookWrapper(args []string) int {
	if len(args) < 4 {
		return 2
	}
	mode, sidecar, real := args[0], args[1], args[2:]
	event, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if err != nil {
		return 2
	}
	var named struct {
		Name string `json:"hook_event_name"`
	}
	json.Unmarshal(event, &named)
	var nonce [8]byte
	rand.Read(nonce[:])
	record := p3WrapperRecord{At: time.Now(), ID: hex.EncodeToString(nonce[:]), Mode: mode, Event: named.Name, Phase: "start"}
	env := os.Environ()
	for _, entry := range env {
		if name, value, _ := strings.Cut(entry, "="); strings.EqualFold(name, p3EpochEnv) {
			record.Epoch = value
		}
	}
	switch mode {
	case "delay":
		appendWrapped(sidecar, record)
		time.Sleep(p3HookDelay)
	case "observe":
		if len(real) < 3 || real[1] != hookcmd.Arg {
			return 2
		}
		connection, err := sessionlink.Read(real[2])
		if err != nil {
			return 2
		}
		blocked := append([]string{"ANTHROPIC_BASE_URL", "ANTHROPIC_UNIX_SOCKET", launch.HostRoutedEnv}, launch.ProviderSwitches...)
		record.EnvMatchesPinned = true
		kept := env[:0:0]
		for _, entry := range env {
			name, value, _ := strings.Cut(entry, "=")
			upper := strings.ToUpper(name)
			if !slices.Contains(blocked, upper) {
				kept = append(kept, entry)
				continue
			}
			differs := false
			switch upper {
			case "ANTHROPIC_BASE_URL":
				differs = value != connection.BaseURL
			case launch.HostRoutedEnv:
			default:
				differs = value != "" && value != "0" && strings.ToLower(value) != "false"
			}
			if differs {
				record.EnvMatchesPinned = false
				record.Differs = append(record.Differs, upper)
			}
		}
		env = append(kept, "ANTHROPIC_BASE_URL="+connection.BaseURL, launch.HostRoutedEnv+"=1")
		if !slices.Contains(real, hookcmd.ProviderControlledArg) {
			real = append(slices.Clone(real), hookcmd.ProviderControlledArg)
		}
	default:
		return 2
	}
	cmd := exec.Command(real[0], real[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout = env, bytes.NewReader(event), os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return 2
		}
		code = exit.ExitCode()
	}
	record.At, record.Phase, record.Exit = time.Now(), "finish", code
	record.ProofMissing = strings.Contains(stderr.String(), "CLAUDUCT_ROUTING_PROOF_MISSING")
	record.Mismatch = strings.Contains(stderr.String(), "CLAUDUCT_ROUTING_MISMATCH")
	appendWrapped(sidecar, record)
	return code
}

// p3EpochEnv is the non-routing marker a settings step writes beside its change. Native reading
// the changed file shows as this marker in a hook's environment, whatever --settings pins.
const p3EpochEnv = "CLAUDUCT_P3_SETTINGS_EPOCH"

// p3Applied reports whether the observation wrapper saw the step's settings marker in native's
// environment at the UserPromptSubmit hook of the given turn, before that turn's model request.
func p3Applied(obs *p3Observed, turn int) bool {
	if turn >= len(obs.Turns) || obs.Epoch == "" {
		return false
	}
	for _, r := range obs.Wrapped {
		if r.Mode == "observe" && r.Event == "UserPromptSubmit" && r.Phase == "finish" && r.Epoch == obs.Epoch && inWindow(obs.Turns, turn, r.At) {
			return true
		}
	}
	return false
}

// p3LateEvent reports whether a delayed hook started in one turn finished inside a later turn
// while native was still running it: the late event the case is about actually happened.
func p3LateEvent(obs *p3Observed) bool {
	started := map[string]time.Time{}
	for _, r := range obs.Wrapped {
		if r.Mode == "delay" && r.Phase == "start" {
			started[r.ID] = r.At
		}
	}
	for _, r := range obs.Wrapped {
		begin, ok := started[r.ID]
		if r.Mode != "delay" || r.Phase != "finish" || !ok {
			continue
		}
		for k := 1; k < len(obs.Turns); k++ {
			tr := obs.Turns[k]
			if begin.Before(tr.Start) && !r.At.Before(tr.Start) && !r.At.After(tr.End) {
				return true
			}
		}
	}
	return false
}

// p3HookFailure is native's own transcript record of a failed command hook.
type p3HookFailure struct {
	At    time.Time
	Event string
	Type  string
}

// p3HookFailureTypes are the transcript attachment types native 2.1.292 writes for a command
// hook that failed, was cancelled or erred (string table of the binary).
var p3HookFailureTypes = []string{"hook_non_blocking_error", "hook_cancelled", "hook_error_during_execution"}

func hookFailures(configDir string) []p3HookFailure {
	var out []p3HookFailure
	filepath.Walk(filepath.Join(configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 64<<10), 16<<20)
		for s.Scan() {
			var line struct {
				Timestamp  time.Time `json:"timestamp"`
				Attachment struct {
					Type  string `json:"type"`
					Event string `json:"hookEvent"`
				} `json:"attachment"`
			}
			if json.Unmarshal(s.Bytes(), &line) == nil && slices.Contains(p3HookFailureTypes, line.Attachment.Type) {
				out = append(out, p3HookFailure{At: line.Timestamp, Event: line.Attachment.Event, Type: line.Attachment.Type})
			}
		}
		return nil
	})
	return out
}

// startSentinel runs another live session beside the case: its own gateway and foreground link.
// Any record it opens, or a routing mismatch reported to it, is a change to its state.
func (w *p3World) startSentinel(t TB) {
	g, err := app.StartProductGateway(&p3Transport{world: func() *p3World { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	g.ObserveOutcome(func(gateway.RequestRecord) {
		w.mu.Lock()
		w.sentinelRecords++
		w.mu.Unlock()
	})
	// Its link names a counting proxy in front of it, so every request that reaches the other
	// session -- a confirmation included, which opens no record -- is seen.
	target, _ := url.Parse(g.BaseURL())
	// Transparent: the sentinel's boundary checks see its own host and no forwarding headers.
	forward := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = target.Host
	}}
	proxy := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		if w.sentinelPaths == nil {
			w.sentinelPaths = map[string]int{}
		}
		w.sentinelPaths[r.URL.Path]++
		w.mu.Unlock()
		forward.ServeHTTP(rw, r)
	}))
	link, err := sessionlink.StartForeground(context.Background(), sessionlink.Connection{BaseURL: proxy.URL, Token: g.Token()})
	if err != nil {
		proxy.Close()
		g.Close(context.Background())
		t.Fatal(err)
	}
	w.sentinel, w.sentinelLink = g, link
	t.Cleanup(func() {
		link.Close()
		proxy.Close()
		g.Close(context.Background())
	})
}

var (
	pluginHelper = regexp.MustCompile(`(?m)^const confirmationHelper = [^\r\n]*;`)
	pluginArgs   = regexp.MustCompile(`(?m)^const confirmationArgs = [^\r\n]*;`)
)

// applyPlugin breaks the session's confirmation proof as the case names, at the spawn boundary:
// Run has written the native-events plugin and puts its --plugin-dir first.
func (w *p3World) applyPlugin(spec launch.Spec) (launch.Spec, error) {
	if w.plugin == "" {
		return spec, nil
	}
	if len(spec.Args) < 2 || spec.Args[0] != "--plugin-dir" {
		return spec, errors.New("no native-events plugin at the head of the launch; Run changed")
	}
	dir := spec.Args[1]
	if w.plugin == p3PluginAbsent {
		spec.Args = spec.Args[2:]
		w.pluginApplied = true
		return spec, nil
	}
	module := filepath.Join(dir, "hooks", "events.mjs")
	raw, err := os.ReadFile(module)
	if err != nil {
		return spec, err
	}
	helper := pluginHelper.FindString(string(raw))
	var args []string
	switch w.plugin {
	case p3PluginStale:
		stale, err := sessionlink.StartForeground(context.Background(), sessionlink.Connection{BaseURL: "http://127.0.0.1:9", Token: "p3-stale"})
		if err != nil {
			return spec, err
		}
		stale.Close()
		args = []string{hookcmd.ConfirmationArg, stale.ID}
	case p3PluginForeign:
		args = []string{hookcmd.ConfirmationArg, w.sentinelLink.ID}
	case p3PluginForged:
		self, err := os.Executable()
		if err != nil {
			return spec, err
		}
		quoted, _ := json.Marshal(filepath.ToSlash(self))
		helper = "const confirmationHelper = " + string(quoted) + ";"
		args = []string{p3ForgedHelperArg}
	default:
		return spec, errors.New("unknown plugin variant " + w.plugin)
	}
	encoded, _ := json.Marshal(args)
	text := pluginHelper.ReplaceAllLiteralString(string(raw), helper)
	text = pluginArgs.ReplaceAllLiteralString(text, "const confirmationArgs = "+string(encoded)+";")
	if text == string(raw) {
		return spec, errors.New("native-events plugin did not change; its helper constants moved")
	}
	w.pluginApplied = true
	return spec, os.WriteFile(module, []byte(text), 0o600)
}

// p3SessionState is a session's stored profile snapshot and conversation, by digest and size.
type p3SessionState struct {
	profile    string
	transcript []byte
}

func (w *p3World) sessionState(session string) p3SessionState {
	var state p3SessionState
	if raw, err := os.ReadFile(filepath.Join(w.home, ".clauduct", "sessions", session+".json")); err == nil {
		sum := sha256.Sum256(raw)
		state.profile = hex.EncodeToString(sum[:])
	}
	filepath.Walk(filepath.Join(w.configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Base(path) == session+".jsonl" {
			state.transcript, _ = os.ReadFile(path)
		}
		return nil
	})
	return state
}

// keptBy reports whether a later state keeps this one: the same profile snapshot, and the
// earlier conversation still there unchanged (lines may follow it).
func (s p3SessionState) keptBy(later p3SessionState) bool {
	return s.profile == later.profile && bytes.HasPrefix(later.transcript, s.transcript)
}
