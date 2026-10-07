package providerprobe

// P3 provider-control probe harness (#320, routing contract r1 / probe schema r1). It drives the
// product launch path (Run) with a probe transport behind the real gateway, applies one of the
// contract's configurations at the spawn boundary, owns every wrong destination, and turns what
// they saw into providercert observations. TestP3HarnessSynthetic checks the harness itself
// against an in-process fake native; TestP3HostManagedProbe runs the real native and is opt-in.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
	"github.com/wotjr1649/Clauduct/go/internal/sessionlink"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// Contract configuration names.
const (
	p3Unprotected = "UNPROTECTED"
	p3Baseline    = "BASELINE"
	p3HostManaged = "HOST_MANAGED"
	p3GatewayEnv  = "GATEWAY_ENV_COMPAT"
)

// Hook variants.
const (
	p3HooksNormal   = "NORMAL"
	p3HooksDisabled = "DISABLED_BY_USER_SETTINGS"
	p3HooksMissing  = "COMMAND_LOAD_FAILURE"
	p3HooksTimeout  = "TIMEOUT"
)

// p3Entry is how a case drives native.
type p3Entry string

const (
	p3Print    p3Entry = "PRINT"
	p3Resume   p3Entry = "RESUME"
	p3Continue p3Entry = "CONTINUE"
	p3Multi    p3Entry = "SAME_PROCESS_MULTI_TURN"
)

// p3Case is one row of the contract's runtime matrix that this harness drives.
type p3Case struct {
	ID       string
	Entry    p3Entry
	Hooks    string
	Optional bool
	// Setup writes the case's starting state into the probe environment.
	Setup func(*p3World) error
	// Steps are a stateful case's external mutations: step i runs after turn i+1, so a case
	// has len(Steps)+1 turns.
	Steps []func(*p3World) error
	// Stimulus401 makes INTENDED refuse turn 2's first canary attempt with a 401.
	Stimulus401 bool
	// TUIStimulus are inputs typed after the TUI's first reply, each its own driver step, then
	// a confirmation prompt (turn2) 8 seconds later (admission r2 TUI background-entry checks).
	TUIStimulus [][]string
	// TUIScreenOrder are screen texts (whitespace removed) the run's whole screen must show in
	// this order; obs.ScreenOrdered records whether it did (BE14's picker open and cancel).
	TUIScreenOrder []string
	// TUIRefreshApproval approves the managed-settings dialog a mid-session policy refresh
	// raises before the second prompt (RP01-TUI).
	TUIRefreshApproval bool
	// TUIApproval answers a permission dialog after the first prompt (PM02T): these keys are
	// sent once the dialog is on screen.
	TUIApproval []string
	// WatchBoundary observes native's background boundary for a -p case (BE10).
	WatchBoundary bool
	// CLIPrior runs a -p turn with session p3PriorSession before a CLI case's own launch.
	CLIPrior bool
	// ProductEnv is added to the environment the product itself reads (not native's).
	ProductEnv func(*p3World) map[string]string
	// Script answers INTENDED's calls for this case (scripted tool calls); nil is the default.
	Script func(body string, canaries []string) string
	// Expect is P5-F's expectation for every turn; ExpectTurns overrides it per turn index.
	Expect      string
	ExpectTurns map[int]string
	// FailClosedAs are the refusals an EXPECTED_FAIL_CLOSED turn of this case must end with.
	FailClosedAs []string
	// Purpose is providercert.PurposeSuppression or providercert.PurposeSafety.
	Purpose string
	// HookFaults breaks the command hooks of named events only: p3HooksMissing,
	// p3HooksTimeout, or p3HookDelayed (the real hook, started past its timeout).
	HookFaults map[string]string
	// DiagnosticEvent is the hook event whose failure native must record for a
	// SUCCESS_WITH_HOOK_DIAGNOSTIC turn.
	DiagnosticEvent string
	// Plugin breaks the confirmation proof: p3PluginAbsent, p3PluginStale, p3PluginForeign or
	// p3PluginForged.
	Plugin string
	// Sentinel runs another live session's gateway and link beside the case; any change to
	// its state is a safety failure.
	Sentinel bool
	// ObserveHooks wraps every command hook in the observation wrapper: Clauduct's #295 and
	// #299 routing blocks become observations, for control and treatment alike. Synthetic
	// configuration and loopback receivers only.
	ObserveHooks bool
	// Settle is the bounded wait after each step, fixed before the run.
	Settle time.Duration
	// Args are added to the print entry's command line.
	Args []string
	// TUIAfterReply replaces the TUI entry's /exit after the reply with these driver steps (the
	// first one's send and wait); the native processes of the probe config dir are then counted.
	TUIAfterReply []map[string]any
	// RequireApplication is a stateful case whose native must be seen to apply the step's
	// change (by the observation wrapper) before the next turn.
	RequireApplication bool
	// StepCheck reads back each step's synthetic state; false is STATE_NOT_APPLIED.
	StepCheck func([]p3StepState) bool
	// Turns is the number of turns of a same-process case without steps; a stateful case has
	// len(Steps)+1.
	Turns int
	// KeepHooksInControl runs a start-state case's control with its hooks, for a diagnosis that
	// observes native's environment there. Certification cases leave it false.
	KeepHooksInControl bool
	// RestoreTurn is the turn index whose model request must continue turn 1's conversation
	// with turn 1's model (0: none).
	RestoreTurn int
	// ProcessEnv is added to native's process environment for both configurations.
	ProcessEnv map[string]func(*p3World) string
	// Established reads a control run: did the state move the unprotected native?
	Established func(*p3Observed) bool
}

// stateful reports whether a case mutates state between turns. Its control keeps the case's
// hooks so turn 1 can complete before the mutation (contract r2 control rule).
func (c p3Case) stateful() bool { return len(c.Steps) > 0 }

// p3CaseIDs lists the cases' IDs in order.
func p3CaseIDs(cases []p3Case) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	return ids
}

// p3NativeStarts is how many times a case starts native in one configuration: a print or
// same-process case once, a resume or continue case twice, the continue-then-resume case three.
func p3NativeStarts(c p3Case) int {
	switch c.Entry {
	case p3ContinueRecover:
		return 3
	case p3Resume, p3Continue, p3ContinueRefusal, p3ContinueLegacy:
		return 2
	}
	return 1
}

// p3StepState is what a stateful step left in the synthetic state: which files exist, their
// sizes, and which stored gateway (if any) the credentials name. Contents are synthetic.
type p3StepState struct {
	Step          int
	At            time.Time
	Files         map[string]int64 // name -> size; absent when the file is absent
	StoredGateway string           // receiver ID the synthetic credentials name, or ""
}

func (w *p3World) stateAfterStep(i int) p3StepState {
	state := p3StepState{Step: i, At: time.Now(), Files: map[string]int64{}}
	for name, path := range map[string]string{
		".credentials.json":     filepath.Join(w.configDir, ".credentials.json"),
		"settings.json":         filepath.Join(w.configDir, "settings.json"),
		"managed-settings.json": filepath.Join(w.managedDir, "managed-settings.json"),
	} {
		if info, err := os.Stat(path); err == nil {
			state.Files[name] = info.Size()
		}
	}
	if raw, err := os.ReadFile(filepath.Join(w.configDir, ".credentials.json")); err == nil {
		for _, r := range []*providercert.Receiver{w.sgA, w.sgB} {
			if strings.Contains(string(raw), r.URL) {
				state.StoredGateway = r.Observer.ID
			}
		}
	}
	return state
}

// p3LinkedHook is a product hook command that names its session link.
var p3LinkedHook = regexp.MustCompile(` --clauduct-hook [a-f0-9]{32}( --clauduct-provider-controlled)?$`)

// hooksFor is the hook state a run of this case uses: a start-state control shows the threat
// without hooks (contract r2).
func (c p3Case) hooksFor(configuration string) string {
	if configuration == p3Unprotected && !c.stateful() && !c.KeepHooksInControl {
		return p3HooksDisabled
	}
	return c.Hooks
}

func (c p3Case) expect(turn int) string {
	if e, ok := c.ExpectTurns[turn]; ok {
		return e
	}
	return c.Expect
}

// p3World is one run's probe environment: its config dir, its receivers, its canaries.
type p3World struct {
	t          TB
	configDir  string
	cwd        string
	home       string
	managedDir string
	caFile     string
	sgA, sgB   *providercert.Receiver
	settings   *providercert.Receiver
	socket     *providercert.Receiver
	catchAll   *providercert.CatchAll
	transport  *p3Transport

	// catchAllPolicy makes CATCH_ALL answer server-managed settings requests with the routing
	// policy (R15 only).
	catchAllPolicy bool
	// The case's variants, set by run before native starts.
	hookFaults    map[string]string
	observeHooks  bool
	plugin        string
	pluginApplied bool
	// sentinel is another live session (gateway and link) for cases that must not change it;
	// sentinelChanged counts its records.
	sentinel        *gateway.Gateway
	sentinelLink    *sessionlink.Server
	sentinelRecords int
	// sentinelPaths counts every request that reached the sentinel through its link's URL.
	sentinelPaths map[string]int
	// epoch is the non-routing marker a settings step writes, so its reading can be observed.
	epoch string
	// sidecar collects the hook wrappers' observations.
	sidecar string

	mu       sync.Mutex
	canaries []providercert.Canary
	// ingress is every authenticated inference body INTENDED received, accepted or not, and
	// outcomes each request's final record by the same Seq.
	ingress []p3Call
	// outcomes is keyed by gateway and record number: each Run starts its own gateway, whose
	// record numbers start again at 1.
	outcomes map[[2]int64]gateway.RequestRecord
	gateways int64
	// gatewayURL is the latest product gateway's base URL, a role for the daemon's environment.
	gatewayURL string
	// overlayGaps are the strategy values the product did not pass and the harness supplied.
	overlayGaps []string
	// surface, when non-nil, collects each ingress's request shape (surface comparison).
	surface []p3Surface
	// modelLists counts GET /v1/models at the gateway while surface is recorded.
	modelLists int
	// remotePolicy, when set, is what the stored gateways serve on /managed/settings (RP01).
	remotePolicy *p3RemotePolicy
	// loginLifetime is the synthetic stored login's validity; zero is one hour.
	loginLifetime time.Duration
	// wrongScript, when set, may answer a wrong receiver's inference request with its own
	// Anthropic stream (a scripted tool call); "" keeps the default reply.
	wrongScript func(body string) string
}

// observe registers this world's ingress and outcome observers on a newly started gateway.
func (w *p3World) observe(g *gateway.Gateway) {
	w.mu.Lock()
	w.gatewayURL = g.BaseURL()
	w.mu.Unlock()
	ingress, outcome := w.observers()
	g.ObserveIngress(ingress)
	g.ObserveOutcome(outcome)
}

// observers are one gateway's ingress and outcome observers, sharing that gateway's number.
func (w *p3World) observers() (func(gateway.Ingress), func(gateway.RequestRecord)) {
	w.mu.Lock()
	w.gateways++
	gen := w.gateways
	w.mu.Unlock()
	return func(in gateway.Ingress) {
			ids, _ := providercert.Scan(strings.NewReader(string(in.Body)), w.liveCanaries())
			var named struct {
				Model string `json:"model"`
			}
			json.Unmarshal(in.Body, &named)
			w.mu.Lock()
			w.ingress = append(w.ingress, p3Call{at: time.Now(), canaries: ids, seq: in.Seq, gateway: gen, class: in.RequestClass, path: in.Path, model: named.Model})
			if w.surface != nil {
				w.surface = append(w.surface, p3SurfaceOf(in))
			}
			w.mu.Unlock()
		}, func(r gateway.RequestRecord) {
			w.mu.Lock()
			if w.outcomes == nil {
				w.outcomes = map[[2]int64]gateway.RequestRecord{}
			}
			w.outcomes[[2]int64{gen, r.Seq}] = r
			if w.surface != nil && r.Path == "/v1/models" {
				w.modelLists++
			}
			w.mu.Unlock()
		}
}

// ingressWithOutcomes is every ingress so far with the category its own gateway gave it.
func (w *p3World) ingressWithOutcomes() []p3Call {
	w.mu.Lock()
	defer w.mu.Unlock()
	calls := slices.Clone(w.ingress)
	for i := range calls {
		calls[i].outcome = w.outcomes[[2]int64{calls[i].gateway, calls[i].seq}].Category
	}
	return calls
}

func (w *p3World) liveCanaries() []providercert.Canary {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.canaries)
}

func (w *p3World) addCanary(id string) providercert.Canary {
	c, err := providercert.NewCanary(id)
	if err != nil {
		w.t.Fatal(err)
	}
	w.mu.Lock()
	w.canaries = append(w.canaries, c)
	w.mu.Unlock()
	return c
}

// marker is the only reply text a probe receiver sends.
func marker(receiver, nonce string) string { return "CLAUDUCT-PROBE-REPLY-" + receiver + "-" + nonce }

// turnCanaryID is a prompt turn's canary ID; the others (a managed file's, a policy generation's)
// belong to a fixture, not to a turn.
var turnCanaryID = regexp.MustCompile(`^turn[0-9]+$`)

// p3CurrentNonce is the marker nonce for a request that carried the canaries ids, which a scan
// lists in the order the canaries were made: the current turn's canary is the latest turn
// canary, because the history of earlier turns is in the body too. An answer with the first
// canary would reply to turn 1 again and never show a later turn's marker. A request with no
// turn canary gets the last canary found, or "aux".
func p3CurrentNonce(ids []string) string {
	for i := len(ids) - 1; i >= 0; i-- {
		if turnCanaryID.MatchString(ids[i]) {
			return ids[i]
		}
	}
	if len(ids) > 0 {
		return ids[len(ids)-1]
	}
	return "aux"
}

var markerPattern = regexp.MustCompile(`CLAUDUCT-PROBE-REPLY-([A-Z_]+)-(turn[0-9]+|aux)`)

// anthropicReply is an Anthropic SSE stream whose text is one marker.
func anthropicReply(text string) string {
	encoded, _ := json.Marshal(text)
	return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"x\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":" + string(encoded) + "}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

// wrongResponder answers like a working provider, with its own marker, so a misroute shows in
// native's output as well as in the receiver's count.
func (w *p3World) wrongResponder(id string) func(http.ResponseWriter, *http.Request, providercert.Seen) {
	return func(rw http.ResponseWriter, r *http.Request, seen providercert.Seen) {
		switch {
		case seen.PathClass == "managed-settings" && w.remotePolicy != nil && r.URL.Path == "/managed/settings":
			w.remotePolicy.serve(w, id, rw, r)
		case seen.PathClass == "managed-settings":
			w.routingPolicy(rw)
		case seen.PathClass == "models":
			rw.Header().Set("Content-Type", "application/json")
			rw.Write([]byte(`{"data":[{"type":"model","id":"gpt-6-luna","display_name":"gpt-6-luna"}],"has_more":false}`))
		case seen.Inference && w.wrongScript != nil && p3ScriptedReply(w, rw, r):
		case seen.Inference:
			nonce := p3CurrentNonce(seen.Canaries)
			rw.Header().Set("Content-Type", "text/event-stream")
			rw.Write([]byte(anthropicReply(marker(id, nonce))))
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}
}

func newP3World(t TB, transport *p3Transport) *p3World {
	t.Helper()
	w := &p3World{t: t, configDir: t.TempDir(), home: t.TempDir(), managedDir: t.TempDir(), transport: transport}
	_, w.cwd = workspace(t)
	var err error
	mk := func(id string, tls bool) *providercert.Receiver {
		o := &providercert.Observer{ID: id, Canaries: w.liveCanaries}
		o.Respond = w.wrongResponder(id)
		var r *providercert.Receiver
		if tls {
			r, err = providercert.ListenTLS(o)
		} else {
			r, err = providercert.Listen(o)
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Close() })
		return r
	}
	w.sgA = mk("WRONG_STORED_GATEWAY_A", true)
	w.sgB = mk("WRONG_STORED_GATEWAY_B", true)
	w.settings = mk("WRONG_SETTINGS", false)
	socketObserver := &providercert.Observer{ID: "WRONG_SOCKET", Canaries: w.liveCanaries}
	socketObserver.Respond = w.wrongResponder("WRONG_SOCKET")
	w.socket, err = providercert.ListenUnix(socketObserver, filepath.Join(t.TempDir(), "p3.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.socket.Close() })
	catch := &providercert.Observer{ID: "CATCH_ALL", Canaries: w.liveCanaries}
	w.catchAll, err = providercert.ListenCatchAll(catch)
	if err != nil {
		t.Fatal(err)
	}
	catch.Respond = func(rw http.ResponseWriter, r *http.Request, seen providercert.Seen) {
		if seen.PathClass == "managed-settings" && w.catchAllPolicy {
			w.routingPolicy(rw)
			return
		}
		if seen.Inference {
			rw.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}
	t.Cleanup(func() { w.catchAll.Close() })
	// The probe child trusts the catch-all CA and the synthetic stored gateways' certificates,
	// as a real stored gateway's certificate would be trusted; nothing else is added.
	w.caFile = filepath.Join(t.TempDir(), "p3-ca.pem")
	bundle := append(append(append([]byte{}, w.catchAll.CAPEM...), w.sgA.CertPEM...), w.sgB.CertPEM...)
	if err := os.WriteFile(w.caFile, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	return w
}

// routingPolicy is the remote managed settings a wrong provider serves: hooks off and the
// endpoint moved to WRONG_SETTINGS.
func (w *p3World) routingPolicy(rw http.ResponseWriter) {
	policy, _ := json.Marshal(map[string]any{"uuid": "p3-remote-policy", "checksum": "p3-remote-policy",
		"settings": map[string]any{"disableAllHooks": true, "allowManagedHooksOnly": true, "env": map[string]string{"ANTHROPIC_BASE_URL": w.settings.URL}}})
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(policy)
}

// storeGateway writes the synthetic stored gateway session for receiver r.
func (w *p3World) storeGateway(r *providercert.Receiver) error {
	login, _ := json.Marshal(map[string]any{
		"enterpriseGateway": map[string]any{"url": r.URL, "jwt": "e30.e30.", "expiresAt": time.Now().Add(max(w.loginLifetime, time.Hour)).UnixMilli()},
		"gatewayTrust":      map[string]string{"127.0.0.1": r.CertSHA256},
	})
	return os.WriteFile(filepath.Join(w.configDir, ".credentials.json"), login, 0o600)
}

// mergeUserSettingsEnv adds env to the user settings already written, keeping the rest.
func (w *p3World) mergeUserSettingsEnv(env map[string]string) error {
	current := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(w.configDir, "settings.json")); err == nil {
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
	}
	merged := map[string]any{}
	if existing, ok := current["env"].(map[string]any); ok {
		for k, v := range existing {
			merged[k] = v
		}
	}
	for k, v := range env {
		merged[k] = v
	}
	current["env"] = merged
	return w.userSettings(current)
}

// mergeUserSettings sets one key of the user settings the run already wrote.
func (w *p3World) mergeUserSettings(key string, value any) error {
	current := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(w.configDir, "settings.json")); err == nil {
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
	}
	current[key] = value
	return w.userSettings(current)
}

func (w *p3World) userSettings(v map[string]any) error {
	b, _ := json.Marshal(v)
	return os.WriteFile(filepath.Join(w.configDir, "settings.json"), b, 0o600)
}

// p3Call is one request INTENDED saw: a backend call, or an ingress with its Seq and class.
type p3Call struct {
	at       time.Time
	canaries []string
	seq      int64
	gateway  int64 // which of the world's gateways recorded seq
	model    string
	// backendModel and backendEffort are what the backend would run for a transport call.
	backendModel, backendEffort string
	class                       string
	path                        string
	// outcome is the gateway's final category for an ingress ("" when it was served).
	outcome string
}

// p3Transport stands behind the real gateway: INTENDED. It answers every call with INTENDED's
// marker for the turn whose canary the call carried, and never reaches a backend.
type p3Transport struct {
	world   func() *p3World
	mu      sync.Mutex
	calls   []p3Call
	fail401 int
	// script, when set, may answer a call with its own stream (a scripted tool call); "" keeps
	// the default reply.
	script func(body string, canaries []string) string
}

func (p *p3Transport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	ids := []string{}
	if w := p.world(); w != nil {
		ids, _ = providercert.Scan(strings.NewReader(string(call.Body)), w.liveCanaries())
	}
	p.mu.Lock()
	p.calls = append(p.calls, p3Call{at: time.Now(), canaries: ids, backendModel: call.Model, backendEffort: call.Effort})
	refuse := p.fail401 > 0 && len(ids) > 0
	if refuse {
		p.fail401--
	}
	p.mu.Unlock()
	if refuse {
		return nil, upstream.ClassifyStatus(http.StatusUnauthorized, nil, time.Now())
	}
	if p.script != nil {
		if stream := p.script(string(call.Body), ids); stream != "" {
			return &upstream.Response{Body: io.NopCloser(strings.NewReader(stream))}, nil
		}
	}
	nonce := p3CurrentNonce(ids)
	return &upstream.Response{Body: io.NopCloser(strings.NewReader(measuredStream(marker(providercert.IntendedID, nonce))))}, nil
}

func (p *p3Transport) snapshot() []p3Call {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

// routingSettingsKeys are what routingSettingsEnv puts in the --settings env.
func routingSettingsKeys() []string {
	return append([]string{"ANTHROPIC_BASE_URL", "ANTHROPIC_UNIX_SOCKET", launch.HostRoutedEnv, "NO_PROXY"}, launch.ProviderSwitches...)
}

// applyConfiguration rewrites the launch spec at the spawn boundary exactly as the contract's
// configuration says, then adds the probe-only environment and the case's hook variant.
func (w *p3World) applyConfiguration(spec launch.Spec, configuration, hooks string, extraEnv map[string]string) (launch.Spec, error) {
	processFlag := ""
	switch configuration {
	case p3HostManaged:
		processFlag = launch.HostRoutedEnv
	case p3GatewayEnv:
		processFlag = "CLAUDE_CODE_USE_GATEWAY"
	case p3Unprotected, p3Baseline:
	default:
		return spec, fmt.Errorf("unknown configuration %q", configuration)
	}
	settingsSeen := false
	for i, arg := range spec.Args {
		raw, attached := "", strings.HasPrefix(arg, "--settings=")
		if attached {
			raw = strings.TrimPrefix(arg, "--settings=")
		} else if i > 0 && spec.Args[i-1] == "--settings" {
			raw = arg
		}
		if raw == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			return spec, fmt.Errorf("settings: %w", err)
		}
		settingsSeen = true
		env := map[string]string{}
		if fields["env"] != nil {
			if err := json.Unmarshal(fields["env"], &env); err != nil {
				return spec, err
			}
		}
		switch configuration {
		case p3Unprotected:
			for _, key := range routingSettingsKeys() {
				delete(env, key)
			}
		case p3HostManaged, p3GatewayEnv:
			if _, ok := env[processFlag]; !ok {
				return spec, fmt.Errorf("settings env lacks %s; routingSettingsEnv changed", processFlag)
			}
			if env[processFlag] != "1" {
				w.overlayGap("settings env " + processFlag)
			}
			env[processFlag] = "1"
		}
		fields["env"], _ = json.Marshal(env)
		if fields["hooks"] != nil {
			// Generic JSON, so fields this harness does not know survive the rewrite. Only command
			// hooks change; native plugin function hooks are not touched.
			var hm map[string][]map[string]any
			if err := json.Unmarshal(fields["hooks"], &hm); err != nil {
				return spec, err
			}
			wrapper, err := os.Executable()
			if err != nil {
				return spec, err
			}
			wrap := func(mode, command string) string {
				return `"` + filepath.ToSlash(wrapper) + `" ` + p3HookWrapperArg + " " + mode + ` "` + filepath.ToSlash(w.sidecar) + `" ` + command
			}
			for event, matchers := range hm {
				fault := w.hookFaults[event]
				if hooks == p3HooksMissing || hooks == p3HooksTimeout {
					fault = hooks
				}
				for _, matcher := range matchers {
					entries, _ := matcher["hooks"].([]any)
					for _, entry := range entries {
						h, ok := entry.(map[string]any)
						if !ok || h["type"] != "command" {
							continue
						}
						command, _ := h["command"].(string)
						if !p3LinkedHook.MatchString(command) {
							return spec, fmt.Errorf("hook command without a session link; foregroundSettings changed")
						}
						if configuration == p3HostManaged || configuration == p3GatewayEnv {
							// The launcher's strategy argument, as an integrated HostManaged
							// launch will write it after the session link.
							if !strings.HasSuffix(command, " "+hookcmd.ProviderControlledArg) {
								w.overlayGap("hook " + hookcmd.ProviderControlledArg)
								command += " " + hookcmd.ProviderControlledArg
							}
						}
						switch fault {
						case p3HooksMissing:
							h["command"] = "clauduct-p3-missing-hook.exe"
						case p3HooksTimeout:
							h["command"], h["timeout"] = "ping -n 6 127.0.0.1", 1
						case p3HookDelayed:
							h["command"], h["timeout"] = wrap("delay", command), 1
						case p3HookChild, p3HookChildEscape:
							h["command"], h["timeout"] = wrap(fault, command), int(p3HookTimeout/time.Second)
						default:
							if w.observeHooks {
								command = wrap("observe", command)
							}
							h["command"] = command
						}
					}
				}
			}
			fields["hooks"], _ = json.Marshal(hm)
		}
		encoded, _ := json.Marshal(fields)
		if attached {
			spec.Args[i] = "--settings=" + string(encoded)
		} else {
			spec.Args[i] = string(encoded)
		}
	}
	if !settingsSeen {
		return spec, errors.New("no --settings in the launch spec")
	}
	return w.applyProbeEnv(spec, configuration, extraEnv)
}

// envRoles names the world's own values for an environment's value classes.
func (w *p3World) envRoles() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return map[string]string{w.gatewayURL: "intended-gateway", w.sgA.URL: "stored-gateway-a", w.catchAll.URL: "catch-all",
		w.configDir: "probe-config-dir", w.caFile: "probe-ca"}
}

// p3ControlCommand is a native control process: the supervisor or a stop command.
func p3ControlCommand(args []string) bool {
	return len(args) > 0 && (args[0] == "daemon" || args[0] == "stop")
}

// overlayGap records a difference between what the product passed under a strategy and the
// contract's configuration, which the harness then supplies (RC11).
func (w *p3World) overlayGap(what string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !slices.Contains(w.overlayGaps, what) {
		w.overlayGaps = append(w.overlayGaps, what)
	}
}

// applyProbeEnv adds the probe-only environment and the configuration's process flag.
func (w *p3World) applyProbeEnv(spec launch.Spec, configuration string, extraEnv map[string]string) (launch.Spec, error) {
	processFlag := ""
	switch configuration {
	case p3HostManaged:
		processFlag = launch.HostRoutedEnv
	case p3GatewayEnv:
		processFlag = "CLAUDE_CODE_USE_GATEWAY"
	}
	if processFlag != "" && !slices.Contains(spec.Env, processFlag+"=1") {
		w.overlayGap("process env " + processFlag)
	}
	set := map[string]string{"HTTPS_PROXY": w.catchAll.URL, "HTTP_PROXY": w.catchAll.URL, "NODE_EXTRA_CA_CERTS": w.caFile, "DISABLE_AUTOUPDATER": "1"}
	if processFlag != "" {
		set[processFlag] = "1"
	}
	for k, v := range extraEnv {
		set[k] = v
	}
	out := spec.Env[:0:0]
	for _, entry := range spec.Env {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := set[strings.ToUpper(name)]; replaced {
			continue
		}
		replaced := false
		for k := range set {
			replaced = replaced || strings.EqualFold(k, name)
		}
		if !replaced {
			out = append(out, entry)
		}
	}
	for k, v := range set {
		out = append(out, k+"="+v)
	}
	spec.Env = out
	return spec, nil
}

// p3Watched surfaces a child's own release failures. app.Process.Wait joins the exit status with
// the Job and pipe release errors, and Run reports only the exit; any part of Wait's error
// that is not an exit status is P7.
type p3Watched struct {
	app.Process
	failed *bool
}

func (w *p3Watched) Wait() error {
	err := w.Process.Wait()
	parts := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts = joined.Unwrap()
	}
	for _, part := range parts {
		var exit interface{ ExitCode() int }
		if part != nil && !errors.As(part, &exit) {
			*w.failed = true
		}
	}
	return err
}

// p3Observed is one run of a case: everything the verdict and the evidence read.
type p3Observed struct {
	Configuration string
	Wrong         map[string]providercert.Wrong
	WrongSeen     map[string][]providercert.Seen
	CatchAll      []providercert.Seen
	// Calls are what INTENDED answered (its backend); Ingress is what native sent it.
	Calls         []p3Call
	Ingress       []p3Call
	Turns         []p3Turn
	Outputs       string
	ExitCodes     []int
	RunErrors     []string
	TimedOut      bool
	CleanupFailed bool
	SpawnFailed   bool
	Identity      []providercert.Identity
	// IdentityError is a checkpoint that could not be taken: the run cannot show P6.
	IdentityError bool
	// Dropped counts requests a receiver saw but could not keep in its record.
	Dropped int
	// Steps is the synthetic state each stateful step left, read back after it ran.
	Steps []p3StepState
	// HookFailures are native's transcript records of failed command hooks.
	HookFailures []p3HookFailure
	// Wrapped are the hook wrappers' records (observation, delay and child modes).
	Wrapped []p3WrapperRecord
	// HookProcs is the post-case census of the child mode's wrapper and child processes.
	HookProcs p3HookProcs
	// SentinelChanged is another live session's state changed during the run.
	SentinelChanged bool
	// PluginApplied is the case's plugin variant reached the spawned native.
	PluginApplied bool
	// SentinelConfirmations counts the confirmation attempts that reached the sentinel session.
	SentinelConfirmations int
	// Epoch is the marker the case's settings step wrote, if any.
	Epoch string
	// SocketPath is the world's WRONG_SOCKET path, for a fixture check after the run.
	SocketPath string
	// Owners are the local processes that opened each wrong receiver's connections.
	Owners map[string][]providercert.ConnOwner
	// WorkerPID is the background job's worker, when one was found.
	WorkerPID int
	// Daemon is the native daemon the probe config dir started.
	Daemon *p3DaemonFacts
	// OverlayGaps are the strategy values the product did not pass (RC11).
	OverlayGaps []string
	// Natives are the probe's native processes seen while a background job ran.
	Natives []p3NativeProcess
	// Surface is each ingress's request shape when the runner records it.
	Surface []p3Surface
	// ModelLists is the gateway's GET /v1/models count when the runner records the surface.
	ModelLists int
	// Stimulus is a CLI case's stimulus launch: its exit, whether native wrote to stderr, and
	// how long it ran.
	Stimulus p3StimulusLaunch
	// StimulusSent is the driver's mark that it typed the case's first TUI stimulus;
	// StimulusMarks counts the stimulus inputs the driver began (a mark each).
	StimulusSent  bool
	StimulusMarks int
	// ScreenOrdered is the whole screen showed the case's TUIScreenOrder texts in order.
	ScreenOrdered bool
	// SessionBefore is the product's session record when the driver began the stimulus,
	// SessionAfter at the end of the run.
	SessionBefore, SessionAfter *p3SessionFacts
	// NativeStarts counts the native session processes the product started (not control commands).
	NativeStarts int
	// SupervisorLock and JobDirs are the background boundary seen in the probe config dir.
	SupervisorLock bool
	JobDirs        int
	// Sidecars are the subagent metadata files native wrote in the probe config dir, and each
	// child transcript's first entry, canaries redacted (BE06 diagnosis).
	Sidecars map[string]string
	// RefusalBranches are the gateway's refusal categories with the refusing branch's label.
	RefusalBranches []string
	// Session is the native session a continue case started; RefusalSideEffect is its stored
	// profile or earlier conversation changed across the refused turn.
	Session           string
	RefusalSideEffect bool
	// Screen is the end of a TUI run's screen text, canary values replaced by their IDs.
	Screen string
	// TUI is the driver's evidence and timeline of a TUI run.
	TUI *p3TUIEvidence
	// SnapshotBefore is the session's profile snapshot existing just before the --continue;
	// TranscriptAppended is how many bytes its conversation gained across that turn.
	SnapshotBefore     bool
	TranscriptAppended int
	// ProductCleanup are the session's own cleanup findings, kept as diagnostics.
	ProductCleanup []string
	// Refusals are INTENDED's refusal counts by category, and Labels the fixed diagnostic
	// words found in native's output; neither carries a body, a URL or a token.
	Refusals []map[string]int64
	Labels   []string
}

type p3Turn struct {
	Canary     providercert.Canary
	Start, End time.Time
	// Output is what native printed for this turn: -p stdout or the stream-json result.
	Output string
}

// starter starts the native child; the real one or the synthetic fake.
type starter func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (app.Process, error)

// spawnReport receives the spawn pin's identity, or why PRE_SPAWN/POST_SPAWN could not be shown.
type spawnReport func(providercert.Identity, error)

type p3Runner struct {
	exe string
	// newStart builds the starter for one run; it reports each spawn's pinned identity.
	newStart func(spawnReport) starter
	timeout  time.Duration
	// identity observes the native before and after each run; nil skips it (synthetic).
	identity func() (providercert.Identity, error)
	// prior holds every wrong receiver earlier runs created. They stay open, so a native that
	// reaches one of them during this run is counted against this run.
	prior *[]*providercert.Receiver
	// priorMu guards prior: a scope's cases run in parallel. A concurrent case's receivers join
	// prior when that case ends; a native reaching one earlier is counted by that receiver's own
	// run, which then fails, so the overlap can only refuse, never admit.
	priorMu *sync.Mutex
	// deadline, when set, bounds every native run (the scope run's absolute deadline).
	deadline time.Time
	// settle waits after each stateful step before the next turn. Zero in every certification
	// run; only the stateful diagnosis sets it, and its evidence records it.
	settle time.Duration
	// surface records each ingress's request shape (surface diagnosis only).
	surface bool
}

func newP3Runner(exe string, newStart func(spawnReport) starter, timeout time.Duration) p3Runner {
	return p3Runner{exe: exe, newStart: newStart, timeout: timeout, prior: new([]*providercert.Receiver), priorMu: new(sync.Mutex)}
}

// run drives one case under one configuration and returns what was observed.
func (p p3Runner) run(t TB, c p3Case, configuration string) *p3Observed {
	t.Helper()
	transport := &p3Transport{script: c.Script}
	w := newP3World(t, transport)
	if p.surface {
		w.surface = []p3Surface{}
	}
	transport.world = func() *p3World { return w }
	obs := &p3Observed{Configuration: configuration}
	hooks := c.hooksFor(configuration)
	w.hookFaults, w.observeHooks, w.plugin = c.HookFaults, c.ObserveHooks, c.Plugin
	if hooks == p3HooksDisabled {
		// A disabled-hook run has no command hook or plugin to break or observe.
		w.hookFaults, w.observeHooks, w.plugin = nil, false, ""
	}
	w.sidecar = filepath.Join(t.TempDir(), "hooks.jsonl")
	if c.Sentinel || c.Plugin == p3PluginForeign {
		w.startSentinel(t)
	}
	if err := w.userSettings(map[string]any{"disableAllHooks": hooks == p3HooksDisabled}); err != nil {
		t.Fatal(err)
	}
	if c.Setup != nil {
		if err := c.Setup(w); err != nil {
			t.Fatal(err)
		}
	}
	extra := map[string]string{"CLAUDE_CODE_MANAGED_SETTINGS_PATH": w.managedDir}
	for k, v := range c.ProcessEnv {
		extra[k] = v(w)
	}
	before := map[*providercert.Receiver]int64{}
	p.priorMu.Lock()
	for _, r := range *p.prior {
		before[r] = r.Counter.Bytes.Load() + r.Counter.Conns.Load()
	}
	p.priorMu.Unlock()
	observe := func() {
		if p.identity == nil {
			return
		}
		id, err := p.identity()
		if err != nil {
			obs.IdentityError = true
			obs.RunErrors = append(obs.RunErrors, "identity: "+err.Error())
			return
		}
		obs.Identity = append(obs.Identity, id)
	}
	report := func(id providercert.Identity, err error) {
		if err != nil {
			obs.IdentityError = true
			obs.RunErrors = append(obs.RunErrors, "spawn identity: "+err.Error())
			return
		}
		obs.Identity = append(obs.Identity, id)
	}
	start := p.newStart(report)
	// probeEnv is native's environment for this world, before the configuration's own values.
	probeEnv := func() map[string]string {
		env := map[string]string{}
		for _, entry := range os.Environ() {
			if i := strings.IndexByte(entry, '='); i > 0 {
				env[entry[:i]] = entry[i+1:]
			}
		}
		// The probe owns every proxy setting: an inherited NO_PROXY entry such as * or a domain
		// would let native's egress skip CATCH_ALL and leave the machine uncounted. It also
		// starts native as from an ordinary shell: a Claude Code session the test itself runs
		// under (an agent's) leaves its own markers, messaging socket and options in the
		// environment, which a user's shell does not have.
		p3ScrubProbeEnv(env)
		env["CLAUDE_CONFIG_DIR"] = w.configDir
		return env
	}
	// A background run is driven from outside: its parent context, its ready callback and its
	// time limit are the background entry's.
	var bgParent context.Context
	var bgReady func(app.BackgroundSession) error
	runFor := p.timeout
	if c.Entry == p3Print && c.Settle > 0 {
		// A single turn that runs this long by itself (a scripted tool loop).
		runFor = p.timeout + c.Settle
	}
	total := max(len(c.Steps)+1, c.Turns)
	if c.Entry == p3Multi || c.Entry == p3TUI && total >= 2 {
		// One native process carries every turn and the waits between them.
		waits := len(c.Steps)
		if c.Entry == p3TUI {
			waits = total - 1 // the driver waits between prompts
		}
		runFor = time.Duration(total)*p.timeout + time.Duration(waits)*max(p.settle, c.Settle)
	}
	if c.TUIStimulus != nil {
		// The reply, the stimuli, 8 seconds, then the confirmation prompt and its reply.
		runFor = 3 * p.timeout
	}
	// onceLimit is the end of the current native run's context: a TUI driver ends before it.
	var onceLimit time.Time
	once := func(args []string, stdin io.Reader, stdout io.Writer) {
		observe()
		parent := context.Background()
		if bgParent != nil {
			parent = bgParent
		}
		limit := time.Now().Add(runFor)
		if !p.deadline.IsZero() && p.deadline.Before(limit) {
			limit = p.deadline
		}
		onceLimit = limit
		ctx, cancel := context.WithDeadline(parent, limit)
		defer cancel()
		var stderr strings.Builder
		env := probeEnv()
		if c.ProductEnv != nil {
			for k, v := range c.ProductEnv(w) {
				env[k] = v
			}
		}
		strategy := ""
		if configuration == p3HostManaged || configuration == p3GatewayEnv {
			strategy = configuration
		}
		startGateway := func() (*gateway.Gateway, error) {
			// The product's own gateway start (run.go), so the admission boundaries are the
			// product's; only the transport is synthetic.
			g, err := app.StartProductGateway(transport)
			if err == nil {
				w.observe(g)
			}
			return g, err
		}
		base := app.Options{
			ProviderStrategy: strategy,
			BackgroundReady:  bgReady,
			Args:             append(slices.Clone(args), "--strict-mcp-config"), Env: env, Cwd: w.cwd, Stdin: stdin, Stdout: stdout, Stderr: &stderr,
			ClauductHome:  w.home,
			ResolveClaude: func() (string, bool, error) { return p.exe, true, nil },
			StartProcess: func(spec launch.Spec, in io.Reader, out, errOut io.Writer) (app.Process, error) {
				var err error
				if p3ControlCommand(spec.Args) {
					// Native's control processes (supervisor, stop) the product starts under a
					// strategy: they carry no settings, only the probe's own environment.
					spec, err = w.applyProbeEnv(spec, configuration, extra)
				} else {
					spec, err = w.applyConfiguration(spec, configuration, hooks, extra)
					if err == nil {
						spec, err = w.applyPlugin(spec)
					}
				}
				if err != nil {
					obs.SpawnFailed = true
					return nil, err
				}
				process, err := start(spec, in, out, errOut)
				if err != nil {
					return nil, err
				}
				if !p3ControlCommand(spec.Args) {
					obs.NativeStarts++
				}
				return &p3Watched{Process: process, failed: &obs.CleanupFailed}, nil
			},
		}
		// The probe's upstream is synthetic: ProbeOptions refuses a session without it, and Run is
		// never given an Options the probe assembled itself. The account list is synthetic too, so
		// no probe session reads a credential or reaches the network for it.
		options, optErr := app.ProbeOptions(base, startGateway, p3ModelList)
		var result app.Result
		var err error
		if optErr != nil {
			err = optErr
		} else {
			result, err = app.Run(ctx, options)
		}
		obs.ExitCodes = append(obs.ExitCodes, result.NativeExitCode)
		obs.Refusals = append(obs.Refusals, result.Diagnostics.Requests.RefusedBy)
		if err != nil {
			obs.RunErrors = append(obs.RunErrors, err.Error())
			if !result.NativeStarted {
				obs.SpawnFailed = true
			}
		}
		if ctx.Err() != nil {
			obs.TimedOut = true
		}
		// P7 is about the probe's own child: a child Run could not reap. The session's other
		// cleanup verdicts (native event receipts and the like) are product diagnostics.
		if result.NativeStarted && result.NativeExitCode == app.ExitCodeUnknown {
			obs.CleanupFailed = true
		}
		// Run joins its cleanup errors one per line. Only the native event receipt check is a
		// product diagnostic; a job, listener or plugin directory that was not released is P7.
		if result.CleanupErr != nil {
			for _, line := range strings.Split(result.CleanupErr.Error(), "\n") {
				obs.ProductCleanup = append(obs.ProductCleanup, line)
				if strings.TrimSpace(line) != "" && strings.TrimSpace(line) != "NATIVE_EVENT_CLEANUP_UNVERIFIED" {
					obs.CleanupFailed = true
				}
			}
		}
		obs.Outputs += stderr.String()
		observe()
	}
	prompt := func(c providercert.Canary) string {
		return "Reply with exactly the text of your instructions. " + string(c.Value)
	}
	// turn returns an index: obs.Turns grows, so a pointer into it would go stale.
	turn := func(id string) int {
		obs.Turns = append(obs.Turns, p3Turn{Canary: w.addCanary(id), Start: time.Now()})
		return len(obs.Turns) - 1
	}
	end := func(i int) { obs.Turns[i].End = time.Now() }
	switch c.Entry {
	case p3CLI:
		i := turn("turn1")
		var out strings.Builder
		if c.CLIPrior {
			once([]string{"-p", prompt(obs.Turns[i].Canary), "--session-id", p3PriorSession}, nil, &out)
		}
		// Delivery evidence is the stimulus launch's own native starts and exit, not the prior
		// turn's.
		obs.NativeStarts = 0
		finish := p.boundaryWatch(w, obs)
		exits, outputs, began := len(obs.ExitCodes), len(obs.Outputs), time.Now()
		once(p3FillArgs(c.Args, prompt(obs.Turns[i].Canary)), nil, &out)
		obs.Stimulus = p3StimulusLaunch{Exit: app.ExitCodeUnknown, Took: time.Since(began), Stderr: strings.TrimSpace(obs.Outputs[outputs:]) != ""}
		if len(obs.ExitCodes) > exits {
			obs.Stimulus.Exit = obs.ExitCodes[len(obs.ExitCodes)-1]
		}
		end(i)
		time.Sleep(8 * time.Second) // admission r2 boundary window: 8s after the stimulus
		finish()
		p3CheckNoNatives(p.exe, w, obs)
		t.Logf("%s %s output: %.600q stderr: %.600q", c.ID, configuration, out.String(), obs.Outputs)
		obs.Turns[i].Output = out.String()
		obs.Outputs = out.String() + obs.Outputs
	case p3Print:
		i := turn("turn1")
		var out strings.Builder
		if c.WatchBoundary {
			defer p3CheckNoNatives(p.exe, w, obs)
			defer p.boundaryWatch(w, obs)()
		}
		once(append([]string{"-p", prompt(obs.Turns[i].Canary)}, c.Args...), nil, &out)
		end(i)
		if c.WatchBoundary {
			time.Sleep(8 * time.Second) // admission r2 boundary window: 8s after the stimulus
		}
		obs.Turns[i].Output = out.String()
		obs.Outputs = out.String() + obs.Outputs
	case p3Resume, p3Continue:
		session := fmt.Sprintf("%08x-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffff, time.Now().UnixNano()&0xffffffffffff)
		i := turn("turn1")
		var first, second strings.Builder
		once([]string{"-p", prompt(obs.Turns[i].Canary), "--session-id", session}, nil, &first)
		end(i)
		j := turn("turn2")
		args := []string{"--resume", session, "-p", prompt(obs.Turns[j].Canary)}
		if c.Entry == p3Continue {
			args = []string{"--continue", "-p", prompt(obs.Turns[j].Canary)}
		}
		once(args, nil, &second)
		end(j)
		obs.Turns[i].Output, obs.Turns[j].Output = first.String(), second.String()
		obs.Outputs = first.String() + second.String() + obs.Outputs
	case p3ContinueRefusal, p3ContinueRecover, p3ContinueLegacy:
		session := fmt.Sprintf("%08x-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffff, time.Now().UnixNano()&0xffffffffffff)
		obs.Session = session
		var outs [3]strings.Builder
		i := turn("turn1")
		first := []string{"-p", prompt(obs.Turns[i].Canary), "--session-id", session}
		if c.Entry == p3ContinueRecover {
			// Not today's default pair, so a restored pair is told apart from a default one.
			if bridge.DefaultStartup() == (bridge.Pair{Model: p3RestoreModel, Effort: p3RestoreEffort}) {
				t.Fatal("the restore pair is the default pair; pick another")
			}
			first = append(first, "--model", p3RestoreModel, "--effort", p3RestoreEffort)
		}
		once(first, nil, &outs[0])
		end(i)
		if c.Entry == p3ContinueLegacy {
			// A session from before profile snapshots: nothing stored for it.
			if err := os.Remove(filepath.Join(w.home, ".clauduct", "sessions", session+".json")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		before := w.sessionState(session)
		obs.SnapshotBefore = before.profile != ""
		j := turn("turn2")
		once([]string{"--continue", "-p", prompt(obs.Turns[j].Canary)}, nil, &outs[1])
		end(j)
		after := w.sessionState(session)
		obs.RefusalSideEffect = !before.keptBy(after)
		obs.TranscriptAppended = len(after.transcript) - len(before.transcript)
		n := 2
		if c.Entry == p3ContinueRecover {
			k := turn("turn3")
			once([]string{"--resume", session, "-p", prompt(obs.Turns[k].Canary)}, nil, &outs[2])
			end(k)
			n = 3
		}
		for x := 0; x < n; x++ {
			obs.Turns[x].Output = outs[x].String()
			obs.Outputs = outs[x].String() + obs.Outputs
		}
	case p3TUI:
		i := turn("turn1")
		steps, screen, err := writeTUISteps(t.TempDir(), prompt(obs.Turns[i].Canary), c.TUIAfterReply)
		if err != nil {
			t.Fatal(err)
		}
		timeoutSeconds := 50
		i2, prompt2Step := -1, p3TUIPrompt2Step
		var extra []int // turns after the first, when the case has several prompts
		if c.TUIStimulus != nil {
			i2 = turn("turn2")
			var stimulusSteps []map[string]any
			stimulusSteps, prompt2Step = p3TUIStimulusSteps(prompt(obs.Turns[i].Canary), prompt(obs.Turns[i2].Canary), c.TUIStimulus)
			raw, _ := json.Marshal(stimulusSteps)
			if err := os.WriteFile(steps, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			timeoutSeconds = 180
			defer p3CheckNoNatives(p.exe, w, obs)
			defer p.boundaryWatch(w, obs)()
		} else if total := max(len(c.Steps)+1, c.Turns); total >= 2 {
			// Same process, several prompts. A state change between prompts k and k+1 runs
			// once prompt k's request reached any receiver and its reply had time to show; the
			// driver types the next prompt only after the probe writes that step's ready file.
			wait := max(p.settle, c.Settle)
			prompts := []string{prompt(obs.Turns[i].Canary)}
			ready := make([]string, total-1)
			for k := 1; k < total; k++ {
				extra = append(extra, turn(fmt.Sprintf("turn%d", k+1)))
				prompts = append(prompts, prompt(obs.Turns[extra[k-1]].Canary))
				if k-1 < len(c.Steps) {
					ready[k-1] = filepath.Join(filepath.Dir(steps), fmt.Sprintf("ready-%d", k))
				}
			}
			raw, _ := json.Marshal(p3TUIStepsN(prompts, ready, wait, c.TUIApproval, c.TUIRefreshApproval))
			if err := os.WriteFile(steps, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			timeoutSeconds = int((time.Duration(total-1)*wait + time.Duration(total)*time.Minute + 2*time.Minute).Seconds())
			if len(c.Steps) > 0 {
				stepsDone := make(chan struct{})
				go func() {
					defer close(stepsDone)
					for k, step := range c.Steps {
						turnID := fmt.Sprintf("turn%d", k+1)
						for deadline := time.Now().Add(p.timeout + time.Minute); !p3CanarySeenAnywhere(w, turnID); time.Sleep(300 * time.Millisecond) {
							if time.Now().After(deadline) {
								return
							}
						}
						time.Sleep(3 * time.Second) // the reply reaches the screen
						if err := step(w); err != nil {
							obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("step %d: %v", k+1, err))
							return
						}
						obs.Steps = append(obs.Steps, w.stateAfterStep(k))
						time.Sleep(wait)
						if err := os.WriteFile(ready[k], []byte("ready\n"), 0o600); err != nil {
							obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("step %d ready: %v", k+1, err))
							return
						}
					}
				}()
				defer func() { <-stepsDone }()
			}
		} else if c.TUIApproval != nil {
			raw, _ := json.Marshal(p3TUIStepsN([]string{prompt(obs.Turns[i].Canary)}, nil, 0, c.TUIApproval, false))
			if err := os.WriteFile(steps, raw, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if c.TUIAfterReply != nil {
			defer p.boundaryWatch(w, obs)()
		}
		raw := filepath.Join(filepath.Dir(steps), "raw.log")
		marks := filepath.Join(filepath.Dir(steps), "marks.txt")
		// Each typed prompt is checked for submission against the receivers: the driver waits a
		// bounded window for the turn's canary to be seen and records the outcome.
		submits := []p3SubmitStep{{p3TUIPromptStep, obs.Turns[i].Canary.ID}}
		if i2 >= 0 {
			submits = append(submits, p3SubmitStep{prompt2Step, obs.Turns[i2].Canary.ID})
		}
		for k, x := range extra {
			submits = append(submits, p3SubmitStep{p3TUIPromptNStep(k+2, c.TUIApproval != nil, c.TUIRefreshApproval), obs.Turns[x].Canary.ID})
		}
		dir := filepath.Dir(steps)
		if err := addSubmitChecks(steps, submits, func(turn string) string { return p3SubmitFile(dir, turn) }, p3TUISubmitWindow, p3TUIResendEnter); err != nil {
			t.Fatal(err)
		}
		var submitTurns []string
		for _, sub := range submits {
			submitTurns = append(submitTurns, sub.Turn)
		}
		stopWatch := make(chan struct{})
		watched := make(chan struct{})
		go func() { defer close(watched); watchSubmits(w, dir, submitTurns, stopWatch) }()
		driverDeadline := 0
		snapshotted := make(chan struct{})
		if c.TUIStimulus != nil {
			go func() {
				defer close(snapshotted)
				before := w.sessionFacts()
				for !stopped(stopWatch) {
					if _, ok := markAt(marks, p3TUIPrompt2Step); ok {
						break
					}
					before = w.sessionFacts()
					time.Sleep(50 * time.Millisecond)
				}
				obs.SessionBefore = before
			}()
		} else {
			close(snapshotted)
		}
		driverStart := tuiPinnedStarter(steps, screen, raw, marks, timeoutSeconds, func() int { driverDeadline = tuiDriverDeadline(onceLimit); return driverDeadline })(report)
		var spawnedAt time.Time
		start = func(spec launch.Spec, in io.Reader, out, errOut io.Writer) (app.Process, error) {
			process, err := driverStart(spec, in, out, errOut)
			if err == nil {
				spawnedAt = time.Now()
			}
			return process, err
		}
		var out strings.Builder
		once(c.Args, nil, &out)
		close(stopWatch)
		<-watched
		end(i)
		// The turn is the prompt: it starts when the driver began typing it, not when native
		// started. Requests native sent while it was still starting belong to no turn.
		if sent, ok := promptSent(marks); ok {
			obs.Turns[i].Start = sent
		} else {
			obs.RunErrors = append(obs.RunErrors, "the TUI prompt was never typed")
		}
		if i2 >= 0 {
			end(i2)
			if sent, ok := markAt(marks, prompt2Step); ok {
				obs.Turns[i2].Start = sent
			} else {
				obs.RunErrors = append(obs.RunErrors, "the second TUI prompt was never typed")
			}
		}
		for k, x := range extra {
			end(x)
			if sent, ok := markAt(marks, p3TUIPromptNStep(k+2, c.TUIApproval != nil, c.TUIRefreshApproval)); ok {
				obs.Turns[x].Start = sent
			} else {
				obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("TUI prompt %d was never typed", k+2))
			}
		}
		text, _ := os.ReadFile(screen)
		obs.Turns[i].Output = string(text)
		if len(text) == 0 {
			// The driver was ended before it wrote its screen; its raw log was written as it went.
			text = []byte(rawScreen(raw))
		}
		if c.TUIStimulus != nil {
			_, obs.StimulusSent = markAt(marks, p3TUIPrompt2Step)
			for j := range c.TUIStimulus {
				if _, ok := markAt(marks, p3TUIPrompt2Step+j); ok {
					obs.StimulusMarks++
				}
			}
			obs.ScreenOrdered = p3ScreenOrdered(string(text), c.TUIScreenOrder)
			<-snapshotted
			obs.SessionAfter = w.sessionFacts()
		}
		obs.Screen = p3RedactCanaries(string(text), obs.Turns)
		obs.TUI = p3TUIEvidenceOf(w, obs, marks, raw, out.String(), obs.Outputs, submits, spawnedAt, driverDeadline)
		obs.RunErrors = append(obs.RunErrors, obs.TUI.SubmitUnconfirmed...)
		obs.RunErrors = append(obs.RunErrors, obs.TUI.EnterResent...)
		obs.Outputs = string(text) + out.String() + obs.Outputs
	case p3Background:
		// One background cycle per turn: dispatch, worker, reply, stop. Turn k > 1 runs after
		// step k-1 and resumes the previous job's session in the background, so a state change
		// between them meets a freshly started worker.
		resumeSession := ""
		pinnedEarly, _ := providercert.Observe(p.exe)
		cycle := func(i int, args []string) (session app.BackgroundSession) {
			ready := make(chan app.BackgroundSession, 1)
			ctx, cancelResident := context.WithCancel(context.Background())
			bgParent, bgReady, runFor = ctx, func(s app.BackgroundSession) error { ready <- s; return nil }, 3*time.Minute
			finished := make(chan struct{})
			var out strings.Builder
			go func() {
				defer close(finished)
				once(args, nil, &out)
			}()
			dispatched := time.Now()
			// The daemon is read as soon as the probe config dir's lock names it: it may end
			// with the job before the worker is looked for.
			daemonFacts := make(chan *p3DaemonFacts, 1)
			daemonStop := make(chan struct{})
			go func() {
				deadline := time.Now().Add(60 * time.Second)
				for time.Now().Before(deadline) {
					if _, err := os.Stat(filepath.Join(w.configDir, "daemon.lock")); err == nil {
						daemonFacts <- ownDaemon(w.configDir, pinnedEarly, dispatched.Add(-time.Second), w.envRoles())
						return
					}
					select {
					case <-daemonStop:
						daemonFacts <- &p3DaemonFacts{Error: "no daemon lock appeared while the job ran"}
						return
					case <-time.After(50 * time.Millisecond):
					}
				}
				daemonFacts <- &p3DaemonFacts{Error: "no daemon lock within 60s"}
			}()
			// Every native of the probe config dir, first sighting of each, from dispatch until
			// the job is done with.
			natives := make(chan []p3NativeProcess, 1)
			go func() {
				seen := map[int]bool{}
				var all []p3NativeProcess
				for {
					for _, n := range probeNatives(p.exe, w.configDir, dispatched.Add(-time.Second), w.envRoles()) {
						if !seen[n.PID] {
							seen[n.PID] = true
							all = append(all, n)
						}
					}
					select {
					case <-daemonStop:
						natives <- all
						return
					case <-time.After(300 * time.Millisecond):
					}
				}
			}()
			select {
			case session = <-ready:
			case <-finished:
			case <-time.After(70 * time.Second):
			}
			pinned, pinErr := providercert.Observe(p.exe)
			var worker *p3Worker
			if session.JobID == "" {
				obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("turn %d: background dispatch did not report a job", i+1))
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
				}
				obs.Screen = p3RedactCanaries(out.String()+"\n"+obs.Outputs, obs.Turns)
			} else if pinErr != nil {
				obs.IdentityError = true
				obs.RunErrors = append(obs.RunErrors, "pin: "+pinErr.Error())
			} else if w2, err := findWorkerByCensus(p.exe, w.configDir, w.envRoles(), pinned, dispatched.Add(-time.Second), 30*time.Second); err != nil {
				obs.IdentityError = true
				obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("turn %d: worker identity: %v", i+1, err))
			} else {
				worker = w2
				obs.WorkerPID = worker.PID
				obs.Identity = append(obs.Identity, worker.Identity)
				if text, ok := waitJobReply(w, obs.Turns[i].Canary, session.JobID, 60*time.Second); ok {
					obs.Turns[i].Output = text
					obs.Outputs = text + obs.Outputs
				} else {
					obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("turn %d: background job gave no probe reply: %s", i+1, p3LastJobState))
				}
				resumeSession = jobSessionID(w.configDir, session.JobID)
			}
			close(daemonStop)
			if facts := <-daemonFacts; obs.Daemon == nil {
				obs.Daemon = facts
			}
			obs.Natives = append(obs.Natives, <-natives...)
			end(i)
			// The resident stops its job (native stop) when its context ends.
			cancelResident()
			select {
			case <-finished:
			case <-time.After(45 * time.Second):
				obs.CleanupFailed = true
				obs.RunErrors = append(obs.RunErrors, "the background resident did not end")
			}
			if worker != nil {
				if !worker.exited(15 * time.Second) {
					// The product left its worker running: recorded, then ended through the
					// handle opened for that very process.
					obs.CleanupFailed = true
					obs.RunErrors = append(obs.RunErrors, "the background worker outlived its stopped job")
					worker.end()
				}
				worker.close()
			}
			obs.Outputs = out.String() + obs.Outputs
			bgParent, bgReady, runFor = nil, nil, p.timeout
			return session
		}
		i := turn("turn1")
		cycle(i, []string{"--bg", prompt(obs.Turns[i].Canary), "--name", "p3-" + strings.ToLower(c.ID), "--tools", ""})
		for k, step := range c.Steps {
			if err := step(w); err != nil {
				t.Fatal(err)
			}
			obs.Steps = append(obs.Steps, w.stateAfterStep(k))
			i := turn(fmt.Sprintf("turn%d", k+2))
			if resumeSession == "" {
				obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("turn %d: no session to resume", k+2))
				end(i)
				break
			}
			cycle(i, []string{"--resume", resumeSession, "--bg", prompt(obs.Turns[i].Canary), "--tools", ""})
		}
	case p3Multi:
		p.multi(t, w, c, obs, once, prompt, turn, end)
	}
	// Bytes that were already in flight when native exited are still counted.
	time.Sleep(2 * time.Second)
	obs.Calls = transport.snapshot()
	obs.Ingress = w.ingressWithOutcomes()
	obs.Wrong, obs.WrongSeen = map[string]providercert.Wrong{}, map[string][]providercert.Seen{}
	own := []*providercert.Receiver{w.sgA, w.sgB, w.settings, w.socket}
	w.mu.Lock()
	obs.OverlayGaps = slices.Clone(w.overlayGaps)
	obs.Surface = slices.Clone(w.surface)
	obs.ModelLists = w.modelLists
	w.mu.Unlock()
	obs.Owners = map[string][]providercert.ConnOwner{}
	for _, r := range own {
		obs.Wrong[r.Observer.ID] = providercert.Wrong{Bytes: r.Counter.Bytes.Load(), Conns: r.Counter.Conns.Load()}
		if owners := r.Counter.Owners(); len(owners) > 0 {
			obs.Owners[r.Observer.ID] = owners
		}
		var dropped int
		obs.WrongSeen[r.Observer.ID], _, dropped = r.Observer.Seen()
		obs.Dropped += dropped
	}
	var dropped int
	obs.CatchAll, _, dropped = w.catchAll.Observer.Seen()
	obs.Dropped += dropped
	// A tunnel through CATCH_ALL to a probe receiver is contact with it, even though the
	// receiver itself saw nothing; one to an earlier run's receiver counts as prior contact.
	var prior providercert.Wrong
	for _, s := range obs.CatchAll {
		if s.Method != http.MethodConnect {
			continue
		}
		for _, r := range own {
			if address := strings.TrimPrefix(strings.TrimPrefix(r.URL, "https://"), "http://"); s.Authority == address {
				contact := obs.Wrong[r.Observer.ID]
				contact.Conns++
				obs.Wrong[r.Observer.ID] = contact
			}
		}
		p.priorMu.Lock()
		for _, r := range *p.prior {
			if address := strings.TrimPrefix(strings.TrimPrefix(r.URL, "https://"), "http://"); s.Authority == address {
				prior.Conns++
			}
		}
		p.priorMu.Unlock()
	}
	// Contact with an earlier run's receiver during this run counts against this run.
	for r, count := range before {
		if now := r.Counter.Bytes.Load() + r.Counter.Conns.Load(); now != count {
			prior.Conns += now - count
		}
	}
	obs.Wrong["PRIOR_RUN_RECEIVERS"] = prior
	p.priorMu.Lock()
	*p.prior = append(*p.prior, own...)
	p.priorMu.Unlock()
	obs.Outputs += transcripts(w.configDir)
	obs.HookFailures = hookFailures(w.configDir)
	obs.PluginApplied = w.pluginApplied
	// The child mode's wrapper and child are found by the pid and creation time they recorded;
	// a child still running past its own cap is ended, and counted as a cleanup failure.
	obs.HookProcs = p3ReapHookProcesses(w.sidecar, 10*time.Second)
	if obs.HookProcs.Killed > 0 {
		obs.CleanupFailed = true
		obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("%d hook wrapper processes outlived the case and were ended", obs.HookProcs.Killed))
	}
	obs.Wrapped = readWrapped(w.sidecar)
	if w.sentinel != nil {
		w.mu.Lock()
		others := 0
		for path, n := range w.sentinelPaths {
			if path != "/clauduct/confirmation" {
				others += n
			}
		}
		obs.SentinelChanged = w.sentinelRecords > 0 || others > 0 || w.sentinel.RoutingMismatch()
		obs.SentinelConfirmations = w.sentinelPaths["/clauduct/confirmation"]
		w.mu.Unlock()
	}
	obs.Epoch = w.epoch
	obs.SocketPath = w.socket.URL
	for _, label := range p3Labels {
		if strings.Contains(strings.ToLower(obs.Outputs), strings.ToLower(label)) {
			obs.Labels = append(obs.Labels, label)
		}
	}
	return obs
}

// p3Labels are the words whose presence in native's output, never the output itself, goes
// into evidence to say why a run produced no reply.
var p3Labels = []string{"API Error", "certificate", "self-signed", "self signed", "unable to verify", "ECONNREFUSED", "ECONNRESET",
	"401", "403", "400", "Invalid API key", "authentication", "login", "gateway", "managed", "not allowed", "provider",
	"ROUTING_UNVERIFIED", "NATIVE_CONFIRMATION_UNVERIFIED", "timeout", "timed out", "model", "hook", "error"}

// multi drives a stream-json session: turn 1, the case's mutation, turn 2.
func (p p3Runner) multi(t TB, w *p3World, c p3Case, obs *p3Observed, once func([]string, io.Reader, io.Writer), prompt func(providercert.Canary) string, turn func(string) int, end func(int)) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	var out strings.Builder
	go func() {
		defer close(done)
		once([]string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}, inR, outW)
		outW.Close()
		// A session that ended early reads no more input: a pending turn write fails instead of
		// waiting for a reader that is gone.
		inR.Close()
	}()
	results := make(chan string, 4)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		s := bufio.NewScanner(outR)
		s.Buffer(make([]byte, 64<<10), 8<<20)
		for s.Scan() {
			line := s.Text()
			out.WriteString(line + "\n")
			var e struct {
				Type   string `json:"type"`
				Result string `json:"result"`
			}
			if json.Unmarshal([]byte(line), &e) == nil && e.Type == "result" {
				results <- e.Result
			}
		}
	}()
	send := func(i int) bool {
		msg, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": prompt(obs.Turns[i].Canary)}})
		if _, err := inW.Write(append(msg, '\n')); err != nil {
			return false
		}
		select {
		case result := <-results:
			end(i)
			obs.Turns[i].Output = result
			return true
		case <-done:
			return false
		case <-time.After(p.timeout):
			return false
		}
	}
	total := max(len(c.Steps)+1, c.Turns)
	for i := 0; ; i++ {
		if !send(turn(fmt.Sprintf("turn%d", i+1))) || i+1 >= total {
			break
		}
		if i >= len(c.Steps) {
			continue
		}
		if err := c.Steps[i](w); err != nil {
			t.Fatal(err)
		}
		obs.Steps = append(obs.Steps, w.stateAfterStep(i))
		if settle := max(p.settle, c.Settle); settle > 0 {
			time.Sleep(settle)
		}
		if c.Stimulus401 && i == 0 {
			w.transport.mu.Lock()
			w.transport.fail401 = 1
			w.transport.mu.Unlock()
		}
	}
	inW.Close()
	<-done
	<-scanned
	obs.Outputs = out.String() + obs.Outputs
}

// transcripts concatenates the probe config's session transcripts, where markers also appear.
func transcripts(configDir string) string {
	var b strings.Builder
	filepath.Walk(filepath.Join(configDir, "projects"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			if data, err := os.ReadFile(path); err == nil {
				b.Write(data)
			}
		}
		return nil
	})
	return b.String()
}

// turnOf attributes one request to a turn. A body carries the whole conversation, so the
// newest turn whose canary it holds is the turn it belongs to. A request with no canary (a
// title, a classifier) belongs to the turn whose half-open window [start, next start) holds
// it: closed windows double-count on Windows, where two readings can share a timestamp.
func turnOf(turns []p3Turn, canaries []string, at time.Time) int {
	newest := -1
	for i, tr := range turns {
		if slices.Contains(canaries, tr.Canary.ID) {
			newest = i
		}
	}
	if newest >= 0 {
		return newest
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if !at.Before(turns[i].Start) {
			return i
		}
	}
	return -1
}

// inWindow reports whether at lies in turn i's half-open window [start, next start).
func inWindow(turns []p3Turn, i int, at time.Time) bool {
	if at.Before(turns[i].Start) {
		return false
	}
	return i+1 >= len(turns) || at.Before(turns[i+1].Start)
}

// observation turns a treatment run and its control into the verdict's input.
func p3Observation(c p3Case, treatment, control *p3Observed) providercert.Observation {
	o := providercert.Observation{CaseID: c.ID, Optional: c.Optional, Purpose: c.Purpose, ForeignSessionChanged: treatment.SentinelChanged,
		StoredGateway: []providercert.Wrong{treatment.Wrong["WRONG_STORED_GATEWAY_A"], treatment.Wrong["WRONG_STORED_GATEWAY_B"]},
		Settings:      []providercert.Wrong{treatment.Wrong["WRONG_SETTINGS"], treatment.Wrong["WRONG_SOCKET"], treatment.Wrong["PRIOR_RUN_RECEIVERS"]}}
	for _, s := range treatment.CatchAll {
		if s.Inference {
			o.CatchAllInference++
		}
		if len(s.Canaries) > 0 {
			o.CatchAllCanary++
		}
		if s.Undecodable {
			o.CatchAllUndecodable++
		}
	}
	for _, seen := range treatment.WrongSeen {
		for _, s := range seen {
			if len(s.Canaries) > 0 {
				o.ForeignCanary++
			}
		}
	}
	for i, tr := range treatment.Turns {
		turn := providercert.Turn{Expect: c.expect(i), FailClosedAs: c.FailClosedAs, RequireRestore: c.RestoreTurn != 0 && c.RestoreTurn == i}
		if turn.Expect == providercert.ExpectSafeRefusal {
			turn.SafeRefusal = treatment.Session != "" && treatment.SnapshotBefore && strings.Contains(treatment.Outputs, "SESSION_RESTART_REQUIRED") &&
				strings.Contains(treatment.Outputs, "clauduct --resume "+treatment.Session)
			turn.SideEffect = treatment.RefusalSideEffect
			// Generations of this turn only: a later resume replays this turn's prompt in its
			// conversation, and its own generation is not this turn's.
			for _, call := range treatment.Calls {
				if slices.Contains(call.canaries, tr.Canary.ID) && turnOf(treatment.Turns, call.canaries, call.at) == i {
					turn.BackendCalls++
				}
			}
		}
		if turn.RequireRestore && len(treatment.Turns) > 0 {
			// Restored: this turn's own backend generation runs turn 1's pair and its request
			// carries turn 1's conversation.
			first := treatment.Turns[0].Canary.ID
			var pair, now *p3Call
			for k := range treatment.Calls {
				call := &treatment.Calls[k]
				switch turnOf(treatment.Turns, call.canaries, call.at) {
				case 0:
					if pair == nil && slices.Contains(call.canaries, first) {
						pair = call
					}
				case i:
					if slices.Contains(call.canaries, tr.Canary.ID) && slices.Contains(call.canaries, first) {
						now = call
					}
				}
			}
			turn.Restored = pair != nil && now != nil && pair.backendModel != "" && pair.backendEffort != "" && now.backendModel == pair.backendModel && now.backendEffort == pair.backendEffort
			if c.Entry == p3ContinueRecover && pair != nil && (pair.backendModel != p3RestoreModel || pair.backendEffort != p3RestoreEffort) {
				// Turn 1 did not run the non-default pair it was started with, so an equal pair at
				// the resume would not show a restored snapshot.
				turn.Restored = false
			}
		}
		for _, f := range treatment.HookFailures {
			if f.Event == c.DiagnosticEvent && inWindow(treatment.Turns, i, f.At) {
				turn.HookDiagnostic = true
			}
		}
		for _, in := range treatment.Ingress {
			if slices.Contains(in.canaries, tr.Canary.ID) {
				turn.IntendedCanary++
				if !strings.EqualFold(in.class, "auxiliary") {
					turn.IntendedMainCanary++
				}
			}
			if turnOf(treatment.Turns, in.canaries, in.at) == i {
				turn.IntendedInference++
				// The gateway's own record of this turn's conversation-bearing requests: the
				// category it refused one with, matched by the record number, or "" if served.
				// Only the turn's own model request: count_tokens meets other boundaries.
				if len(in.canaries) > 0 && !strings.EqualFold(in.class, "auxiliary") && in.path == "/v1/messages" {
					turn.Refusals = append(turn.Refusals, in.outcome)
				}
			}
		}
		// Served: INTENDED answered a conversation-bearing call of this turn: its newest canary is
		// this turn's. A call with no canary is auxiliary and does not count, and neither does a
		// late call for an earlier turn, so a turn whose main request was refused before its body
		// was read stays inconclusive rather than failing.
		for _, call := range treatment.Calls {
			if len(call.canaries) > 0 && turnOf(treatment.Turns, call.canaries, call.at) == i {
				turn.IntendedServed = true
			}
		}
		for _, m := range markerPattern.FindAllStringSubmatch(treatment.Outputs, -1) {
			if m[2] == tr.Canary.ID && !slices.Contains(turn.Markers, m[1]) {
				turn.Markers = append(turn.Markers, m[1])
			}
		}
		if out := strings.TrimSpace(tr.Output); out != "" && !markerPattern.MatchString(out) {
			turn.UnmarkedReply = true
		}
		if control != nil && i < len(control.Turns) {
			for _, seen := range control.WrongSeen {
				for _, s := range seen {
					if s.Inference && turnOf(control.Turns, s.Canaries, s.At) == i {
						turn.ControlInference++
					}
				}
			}
		}
		o.Turns = append(o.Turns, turn)
	}
	o.Harness = providercert.Harness{SpawnFailed: treatment.SpawnFailed, TimedOut: treatment.TimedOut, CleanupFailed: treatment.CleanupFailed,
		ReceiverFault: treatment.Dropped > 0, Integrity: treatment.IdentityError}
	if len(treatment.Turns) > 0 && slices.ContainsFunc(treatment.ExitCodes, func(code int) bool { return code != 0 }) && len(treatment.Ingress) == 0 {
		o.Harness.NativeErrorBeforeInference = true
	}
	for i := 1; i < len(treatment.Identity); i++ {
		if treatment.Identity[i] != treatment.Identity[0] {
			o.IdentityChanged = true
		}
	}
	if c.StepCheck != nil && len(c.Steps) > 0 && !c.StepCheck(treatment.Steps) {
		o.StateNotApplied = true
	}
	if c.RequireApplication && !p3Applied(treatment, 1) {
		o.ApplicationUnobserved = true
	}
	switch {
	case c.HookFaults["UserPromptSubmit"] == p3HookDelayed && !p3LateEvent(treatment):
		o.VariantUnexercised = true
	case p3ChildFault(c.HookFaults["UserPromptSubmit"]) && p3LateHookOf(c.HookFaults, treatment).Outcome != p3LateExercised:
		// The child was not ready, native ended it, or its fate or lateness is unproven: the
		// case's own condition did not happen (scope.go reads the reason).
		o.VariantUnexercised = true
	case c.Plugin == p3PluginForeign && treatment.SentinelConfirmations == 0:
		o.VariantUnexercised = true
	}
	if control == nil {
		o.Control.Fault = true
	} else {
		o.Control.Fault = control.SpawnFailed || control.TimedOut || control.CleanupFailed || control.Dropped > 0 || control.IdentityError
		// A stateful control must finish turn 1 at INTENDED, or the mutation was never tested.
		if c.stateful() && (len(control.Turns) == 0 || !strings.Contains(control.Outputs, marker(providercert.IntendedID, control.Turns[0].Canary.ID))) {
			o.Control.Fault = true
		}
		o.Control.Established = c.Established(control)
		for i := 1; i < len(control.Identity); i++ {
			if control.Identity[i] != control.Identity[0] || (len(treatment.Identity) > 0 && control.Identity[i] != treatment.Identity[0]) {
				o.IdentityChanged = true
			}
		}
	}
	return o
}

// gotCanary reports whether receiver id saw turn n's canary in an inference request.
func gotCanary(id string, n int) func(*p3Observed) bool {
	return func(o *p3Observed) bool {
		if n >= len(o.Turns) {
			return false
		}
		for _, s := range o.WrongSeen[id] {
			if s.Inference && slices.Contains(s.Canaries, o.Turns[n].Canary.ID) {
				return true
			}
		}
		return false
	}
}

func either(a, b func(*p3Observed) bool) func(*p3Observed) bool {
	return func(o *p3Observed) bool { return a(o) || b(o) }
}

// p3PrintFamily is the runtime matrix rows this harness drives: every entry except TUI and
// background, which need their own drivers (R05, R08, R08b).
func p3PrintFamily() []p3Case {
	storedA := func(w *p3World) error { return w.storeGateway(w.sgA) }
	storedB := func(w *p3World) error { return w.storeGateway(w.sgB) }
	deleteStored := func(w *p3World) error { return os.Remove(filepath.Join(w.configDir, ".credentials.json")) }
	// tamper merges env into the user settings the run already wrote, so the case's hook state
	// is left as it was: a tamper case is not also a hook-failure case.
	tamper := func(env map[string]string) func(*p3World) error {
		return func(w *p3World) error { return w.mergeUserSettingsEnv(env) }
	}
	redirect := func(n int) func(*p3Observed) bool {
		return either(gotCanary("WRONG_SETTINGS", n), gotCanary("WRONG_SOCKET", n))
	}
	flagsCleared := func(w *p3World) map[string]string {
		return map[string]string{"ANTHROPIC_BASE_URL": w.settings.URL, launch.HostRoutedEnv: "", "CLAUDE_CODE_USE_GATEWAY": ""}
	}
	stored := func(want ...string) func([]p3StepState) bool {
		return func(steps []p3StepState) bool {
			if len(steps) != len(want) {
				return false
			}
			for i, id := range want {
				if steps[i].StoredGateway != id {
					return false
				}
			}
			return true
		}
	}
	managed := func(w *p3World) error {
		policy, _ := json.Marshal(map[string]any{"env": map[string]string{"ANTHROPIC_BASE_URL": w.settings.URL}})
		return os.WriteFile(filepath.Join(w.managedDir, "managed-settings.json"), policy, 0o600)
	}
	ok, okDiag, closed := providercert.ExpectSuccess, providercert.ExpectSuccessHookDiagnostic, providercert.ExpectFailClosed
	safety := providercert.PurposeSafety
	// A session with no command hook never registers its profile: the product refuses it at that
	// boundary, before origin is read (messages.go order, context policy on as in run.go).
	profile := []string{"SESSION_PROFILE_UNVERIFIED"}
	origin, confirmation := []string{"NATIVE_REQUEST_ORIGIN_UNVERIFIED"}, []string{"NATIVE_CONFIRMATION_UNVERIFIED"}
	promptFails := func(fault string) map[string]string { return map[string]string{"UserPromptSubmit": fault} }
	anchorA := gotCanary("WRONG_STORED_GATEWAY_A", 0)
	cases := []p3Case{
		// Start-state anchors with the stored gateway.
		{ID: "R01", Entry: p3Print, Hooks: p3HooksNormal, Setup: storedA, Expect: ok, Established: anchorA},
		{ID: "R02", Entry: p3Print, Hooks: p3HooksDisabled, Setup: storedA, Expect: closed, FailClosedAs: profile, Established: anchorA},
		{ID: "R03", Entry: p3Print, Hooks: p3HooksMissing, Setup: storedA, Expect: closed, FailClosedAs: profile, Established: anchorA},
		{ID: "R04", Entry: p3Print, Hooks: p3HooksTimeout, Setup: storedA, Expect: closed, FailClosedAs: profile, Established: anchorA},
		// One command hook fails while SessionStart has registered the session: the turn
		// completes and native records the failure, as long as the plugin's proof is valid ...
		{ID: "R03p", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksMissing), DiagnosticEvent: "UserPromptSubmit", Setup: storedA, Expect: okDiag, Established: anchorA},
		{ID: "R04p", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksTimeout), DiagnosticEvent: "UserPromptSubmit", Setup: storedA, Expect: okDiag, Established: anchorA},
		// ... and fails closed when the proof is absent, stale, another session's or forged.
		{ID: "R03pa", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksMissing), Plugin: p3PluginAbsent, Setup: storedA, Expect: closed, FailClosedAs: origin, Established: anchorA},
		{ID: "R03ps", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksMissing), Plugin: p3PluginStale, Setup: storedA, Expect: closed, FailClosedAs: confirmation, Established: anchorA},
		{ID: "R03pf", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksMissing), Plugin: p3PluginForeign, Setup: storedA, Expect: closed, FailClosedAs: confirmation, Established: anchorA},
		{ID: "R03px", Entry: p3Print, Hooks: p3HooksNormal, HookFaults: promptFails(p3HooksMissing), Plugin: p3PluginForged, Setup: storedA, Expect: closed, FailClosedAs: confirmation, Established: anchorA},
		// A timed-out hook that still posts later changes neither the next turn nor another
		// live session.
		{ID: "R04L", Entry: p3Multi, Turns: 3, Hooks: p3HooksNormal, HookFaults: promptFails(p3HookChild), DiagnosticEvent: "UserPromptSubmit", Sentinel: true, Setup: storedA, Expect: okDiag, Established: anchorA},
		{ID: "R06", Entry: p3Resume, Hooks: p3HooksNormal, Setup: storedA, Expect: ok, Established: anchorA},
		// --continue of a session with a stored profile is refused with the exact --resume to run
		// (a kept contract, not --continue working); that --resume restores it; a session with no
		// stored profile still continues.
		{ID: "R07r", Entry: p3ContinueRefusal, Hooks: p3HooksNormal, Purpose: providercert.PurposeSafeRefusal, Setup: storedA, Expect: ok,
			ExpectTurns: map[int]string{1: providercert.ExpectSafeRefusal}, Established: anchorA},
		{ID: "R07m", Entry: p3ContinueRecover, Hooks: p3HooksNormal, Setup: storedA, Expect: ok, RestoreTurn: 2,
			ExpectTurns: map[int]string{1: providercert.ExpectSafeRefusal}, Established: anchorA},
		{ID: "R07n", Entry: p3ContinueLegacy, Hooks: p3HooksNormal, Setup: storedA, Expect: ok, RestoreTurn: 1, Established: anchorA},
		// Same-process state changes (SAFETY): the change is applied and the session stays
		// correct. No suppression is claimed; the control documents whether the threat appeared.
		{ID: "R09", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, Purpose: safety, Steps: []func(*p3World) error{storedA}, StepCheck: stored("WRONG_STORED_GATEWAY_A"), Expect: ok, Established: gotCanary("WRONG_STORED_GATEWAY_A", 1)},
		{ID: "R09s", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, Purpose: safety, Steps: []func(*p3World) error{storedA}, StepCheck: stored("WRONG_STORED_GATEWAY_A"), Stimulus401: true, Expect: ok,
			ExpectTurns: map[int]string{1: providercert.ExpectStimulus}, Established: gotCanary("WRONG_STORED_GATEWAY_A", 1)},
		{ID: "R10", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, Purpose: safety, Steps: []func(*p3World) error{storedA, storedB}, StepCheck: stored("WRONG_STORED_GATEWAY_A", "WRONG_STORED_GATEWAY_B"), Expect: ok, Established: gotCanary("WRONG_STORED_GATEWAY_B", 2)},
		{ID: "R11", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, Purpose: safety, Steps: []func(*p3World) error{storedA, deleteStored}, StepCheck: stored("WRONG_STORED_GATEWAY_A", ""), Expect: ok, Established: gotCanary("WRONG_STORED_GATEWAY_A", 1)},
		{ID: "R14", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, Purpose: safety, Steps: []func(*p3World) error{managed}, StepCheck: func(steps []p3StepState) bool {
			if len(steps) != 1 {
				return false
			}
			_, present := steps[0].Files["managed-settings.json"]
			return present
		}, Expect: ok, Established: redirect(1)},
		// Local settings reload (suppression comparison): turn 1, the synthetic settings change, a
		// bounded wait fixed here, then turn 2, whose UserPromptSubmit must show native applied the
		// change. Clauduct's #295 and #299 blocks are observations in control and treatment alike.
		{ID: "R12o", Entry: p3Multi, Hooks: p3HooksNormal, Optional: true, ObserveHooks: true, Settle: 10 * time.Second, RequireApplication: true,
			Steps: []func(*p3World) error{func(w *p3World) error {
				env := flagsCleared(w)
				w.epoch = fmt.Sprintf("%x", time.Now().UnixNano())
				env[p3EpochEnv] = w.epoch
				return tamper(env)(w)
			}}, Expect: ok, Established: redirect(1)},
		// Start-state tamper with hooks disabled: no hook can hide where native's provider
		// selection sends the turn.
		{ID: "R13a", Entry: p3Print, Hooks: p3HooksDisabled, Optional: true, Setup: func(w *p3World) error { return tamper(map[string]string{"ANTHROPIC_BASE_URL": w.settings.URL})(w) }, Expect: closed, FailClosedAs: profile, Established: redirect(0)},
		{ID: "R13b", Entry: p3Print, Hooks: p3HooksDisabled, Optional: true, Setup: func(w *p3World) error {
			return tamper(map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1", "CLAUDE_CODE_SKIP_BEDROCK_AUTH": "1", "ANTHROPIC_BEDROCK_BASE_URL": w.settings.URL, "AWS_REGION": "us-east-1"})(w)
		}, Expect: closed, FailClosedAs: profile, Established: redirect(0)},
		{ID: "R13c", Entry: p3Print, Hooks: p3HooksDisabled, Optional: true, Setup: func(w *p3World) error { return tamper(map[string]string{"ANTHROPIC_UNIX_SOCKET": w.socket.URL})(w) }, Expect: closed, FailClosedAs: profile, Established: redirect(0)},
		{ID: "R13d", Entry: p3Print, Hooks: p3HooksDisabled, Optional: true, Setup: func(w *p3World) error { return tamper(flagsCleared(w))(w) }, Expect: closed, FailClosedAs: profile, Established: redirect(0)},
		// A new native start with a local managed file already in place (suppression comparison).
		{ID: "R14n", Entry: p3Print, Hooks: p3HooksNormal, Optional: true, ObserveHooks: true, Setup: managed, Expect: ok, Established: redirect(0)},
		{ID: "R15", Entry: p3Print, Hooks: p3HooksDisabled, Expect: closed, FailClosedAs: profile, Setup: func(w *p3World) error {
			w.catchAllPolicy = true
			creds, _ := json.Marshal(map[string]any{
				"claudeAiOauth":     map[string]any{"accessToken": "p3-synthetic", "refreshToken": "p3-synthetic", "expiresAt": time.Now().Add(time.Hour).UnixMilli(), "scopes": []string{"user:inference"}, "subscriptionType": "max"},
				"enterpriseGateway": map[string]any{"url": w.sgA.URL, "jwt": "e30.e30.", "expiresAt": time.Now().Add(time.Hour).UnixMilli()},
				"gatewayTrust":      map[string]string{"127.0.0.1": w.sgA.CertSHA256},
			})
			if err := os.WriteFile(filepath.Join(w.configDir, ".credentials.json"), creds, 0o600); err != nil {
				return err
			}
			state, _ := json.Marshal(map[string]any{"primaryApiKey": "sk-ant-p3-synthetic", "hasCompletedOnboarding": true})
			if err := os.WriteFile(filepath.Join(w.configDir, ".claude.json"), state, 0o600); err != nil {
				return err
			}
			return w.userSettings(map[string]any{"disableAllHooks": true, "apiKeyHelper": "echo sk-ant-p3-helper"})
		}, Established: anchorA},
	}
	return cases
}

// p3Evidence is what one case leaves in evidence: counts, classes and digests only.
type p3Evidence struct {
	CaseID, Configuration string
	Verdict               providercert.Result
	Wrong                 map[string]providercert.Wrong
	WrongSeen             map[string][]providercert.Seen
	CatchAll              []providercert.Seen
	IntendedCalls         int
	IntendedIngress       int
	Turns                 []map[string]any
	ExitCodes             []int
	Errors                []string
	Identity              []providercert.Identity
	Markers               []string
	ProductCleanup        []string
	Refusals              []map[string]int64
	Labels                []string
	Steps                 []p3StepState                       `json:",omitempty"`
	PluginApplied         bool                                `json:",omitempty"`
	Wrapped               []p3WrapperRecord                   `json:",omitempty"`
	LateHook              *p3LateHook                         `json:",omitempty"`
	HookProcs             *p3HookProcs                        `json:",omitempty"`
	SentinelConfirmations int                                 `json:",omitempty"`
	SentinelChanged       bool                                `json:",omitempty"`
	SnapshotBefore        bool                                `json:",omitempty"`
	Owners                map[string][]providercert.ConnOwner `json:",omitempty"`
	WorkerPID             int                                 `json:",omitempty"`
	Daemon                *p3DaemonFacts                      `json:",omitempty"`
	OverlayGaps           []string                            `json:",omitempty"`
	Natives               []p3NativeProcess                   `json:",omitempty"`
	ProbePID              int                                 `json:",omitempty"`
	Screen                string                              `json:",omitempty"`
	TUI                   *p3TUIEvidence                      `json:",omitempty"`
	TranscriptAppended    int                                 `json:",omitempty"`
	// Observation is the verdict's input: counts, receiver IDs and refusal categories only.
	Observation *providercert.Observation `json:",omitempty"`
}

func p3EvidenceOf(c p3Case, configuration string, obs *p3Observed, result providercert.Result) p3Evidence {
	e := p3Evidence{CaseID: c.ID, Configuration: configuration, Verdict: result, Wrong: obs.Wrong, WrongSeen: obs.WrongSeen, CatchAll: obs.CatchAll,
		IntendedCalls: len(obs.Calls), IntendedIngress: len(obs.Ingress), ExitCodes: obs.ExitCodes, Identity: obs.Identity}
	e.ProductCleanup, e.Refusals, e.Labels, e.Steps, e.PluginApplied = obs.ProductCleanup, obs.Refusals, obs.Labels, obs.Steps, obs.PluginApplied
	e.Wrapped, e.SentinelConfirmations, e.SentinelChanged = obs.Wrapped, obs.SentinelConfirmations, obs.SentinelChanged
	if e.LateHook = p3LateHookOf(c.HookFaults, obs); e.LateHook != nil {
		e.HookProcs = &obs.HookProcs
	}
	e.SnapshotBefore, e.TranscriptAppended, e.Screen, e.TUI = obs.SnapshotBefore, obs.TranscriptAppended, obs.Screen, obs.TUI
	e.Owners, e.WorkerPID, e.ProbePID, e.Daemon, e.OverlayGaps, e.Natives = obs.Owners, obs.WorkerPID, os.Getpid(), obs.Daemon, obs.OverlayGaps, obs.Natives
	for _, err := range obs.RunErrors {
		e.Errors = append(e.Errors, redactURLs(err))
	}
	for _, tr := range obs.Turns {
		e.Turns = append(e.Turns, map[string]any{"canarySHA256": tr.Canary.Digest(), "id": tr.Canary.ID, "durationMs": tr.End.Sub(tr.Start).Milliseconds()})
	}
	for _, m := range markerPattern.FindAllString(obs.Outputs, -1) {
		if !slices.Contains(e.Markers, m) {
			e.Markers = append(e.Markers, m)
		}
	}
	return e
}

var urlPattern = regexp.MustCompile(`https?://[^\s"']+`)

func redactURLs(s string) string {
	s = urlPattern.ReplaceAllString(s, "<url>")
	if len(s) > 400 {
		s = s[:400]
	}
	return s
}

// runMatrix runs every case's control and treatment and decides the strategy. stop, when set,
// ends the run after a case whose observation calls for it; the remaining cases are not run and
// the decision cannot be CERTIFIED, because their results are missing.
func (p p3Runner) runMatrix(t TB, cases []p3Case, strategy string, write func(string, any), stop func(control, treatment *p3Observed) string) providercert.Strategy {
	var results []providercert.Result
	var optional, safety, refusals, ids []string
	for _, c := range cases {
		ids = append(ids, c.ID)
		if c.Optional {
			optional = append(optional, c.ID)
		}
		if c.Purpose == providercert.PurposeSafety {
			safety = append(safety, c.ID)
		}
		if c.Purpose == providercert.PurposeSafeRefusal {
			refusals = append(refusals, c.ID)
		}
		control := p.run(t, c, p3Unprotected)
		treatment := p.run(t, c, strategy)
		observation := p3Observation(c, treatment, control)
		result := providercert.Judge(observation)
		results = append(results, result)
		write(c.ID+"-control", p3EvidenceOf(c, p3Unprotected, control, providercert.Result{CaseID: c.ID, Verdict: "CONTROL", Reasons: []string{fmt.Sprintf("established=%t", c.Established(control))}}))
		evidence := p3EvidenceOf(c, strategy, treatment, result)
		evidence.Observation = &observation
		write(c.ID+"-treatment", evidence)
		t.Logf("%s %s %v", c.ID, result.Verdict, result.Reasons)
		if stop != nil {
			if reason := stop(control, treatment); reason != "" {
				write("stopped", map[string]string{"afterCase": c.ID, "reason": reason})
				t.Logf("STOPPED after %s: %s", c.ID, reason)
				for _, rest := range cases[len(results):] {
					ids = append(ids, rest.ID)
				}
				break
			}
		}
	}
	return providercert.Decide(ids, optional, safety, results, refusals...)
}

// boundaryWatch counts the probe config dir's native processes from now until the returned
// function runs, then records the background boundary (admission r2 backgroundEntryChecks):
// a supervisor lock, job directories, the processes seen. A supervisor that appeared is stopped
// through native for that config dir only (PL13).
func (p p3Runner) boundaryWatch(w *p3World, obs *p3Observed) func() {
	since, stop := time.Now(), make(chan struct{})
	natives := make(chan []p3NativeProcess, 1)
	go func() {
		seen := map[int]bool{}
		var all []p3NativeProcess
		for {
			for _, n := range probeNatives(p.exe, w.configDir, since.Add(-time.Second), w.envRoles()) {
				if !seen[n.PID] {
					seen[n.PID] = true
					all = append(all, n)
				}
			}
			select {
			case <-stop:
				natives <- all
				return
			case <-time.After(300 * time.Millisecond):
			}
		}
	}()
	return func() {
		close(stop)
		obs.Natives = <-natives
		if _, err := os.Stat(filepath.Join(w.configDir, "daemon.lock")); err == nil {
			obs.SupervisorLock = true
			if err := p3StopOwnSupervisor(p.exe, w.configDir); err != nil {
				obs.CleanupFailed = true
				obs.RunErrors = append(obs.RunErrors, "own supervisor stop: "+err.Error())
			}
		}
		if jobs, _ := os.ReadDir(filepath.Join(w.configDir, "jobs")); len(jobs) > 0 {
			obs.JobDirs = len(jobs)
		}
		obs.Sidecars = p3SubagentSidecars(w.configDir, obs.Turns)
		w.mu.Lock()
		for _, r := range w.outcomes {
			if r.Category != "" {
				obs.RefusalBranches = append(obs.RefusalBranches, r.Category+"|"+r.SelectionRefusal)
			}
		}
		w.mu.Unlock()
	}
}

// p3CheckNoNatives is admission r2's cleanup check: once a case ends, no pinned native points
// at the probe config dir. Natives still exiting get 15 seconds.
func p3CheckNoNatives(exe string, w *p3World, obs *p3Observed) {
	deadline := time.Now().Add(15 * time.Second)
	for {
		left := probeNatives(exe, w.configDir, time.Time{}, w.envRoles())
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			obs.CleanupFailed = true
			obs.RunErrors = append(obs.RunErrors, fmt.Sprintf("%d natives of the probe config dir remain after the case", len(left)))
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// p3StimulusLaunch is what a CLI background-entry case's own launch did.
type p3StimulusLaunch struct {
	Exit   int
	Stderr bool
	Took   time.Duration
}

// p3CredentialEnv names the credentials a probe's native must never inherit from the machine
// it runs on: the probe is synthetic end to end.
func p3CredentialEnv(upper string) bool {
	switch upper {
	case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CODEX_HOME", "OPENAI_API_KEY":
		return true
	}
	return false
}

// p3ScrubProbeEnv deletes from env, which holds the inherited environment, everything the probe
// owns: proxy settings (an inherited NO_PROXY entry such as * or a domain would let native's
// egress skip CATCH_ALL and leave the machine uncounted), a Claude Code session's own markers,
// messaging socket and options (a user's shell does not have them), and real credentials.
// Values the case's configuration sets afterwards still apply.
func p3ScrubProbeEnv(env map[string]string) {
	for name := range env {
		upper := strings.ToUpper(name)
		switch {
		case upper == "NO_PROXY", upper == "HTTPS_PROXY", upper == "HTTP_PROXY", upper == "ALL_PROXY":
			delete(env, name)
		case p3AgentSessionEnv(upper), p3CredentialEnv(upper):
			delete(env, name)
		}
	}
}
