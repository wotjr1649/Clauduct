package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/gateway"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// B1. What the child is told about this session, and every value measured against the real
// client rather than assumed.
//
// These are offline assertions about the map. The live half is below: a real claude driven
// against a fixture backend, which costs no inference and is the only thing that can say
// whether a key does what its name suggests.
func TestTheSessionTellsTheChildWhatThisBuildRoutes(t *testing.T) {
	session := sessionEnvironment()

	// The tiers are derived, not written out. A model added to bridge.Models with the
	// sonnet alias has to change this without anyone editing it here.
	for alias, name := range map[string]string{
		"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
		"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
	} {
		model, known := bridge.ForAlias(alias)
		if !known {
			t.Fatalf("no model answers to %q, so the client's %s tier points at nothing",
				alias, name)
		}
		if session[name] != model.ID {
			t.Errorf("%s = %q, want %q", name, session[name], model.ID)
		}
		// And the value has to be one the gateway will actually route, or the client is
		// being told to ask for something that comes back refused.
		if _, err := bridge.SelectRoute(session[name], ""); err != nil {
			t.Errorf("%s names %q, which does not route: %v", name, session[name], err)
		}
	}

	// The startup route, likewise routable.
	if _, err := bridge.SelectRoute(session["ANTHROPIC_MODEL"], session["CLAUDE_CODE_EFFORT_LEVEL"]); err != nil {
		t.Errorf("the startup route %s/%s does not route: %v",
			session["ANTHROPIC_MODEL"], session["CLAUDE_CODE_EFFORT_LEVEL"], err)
	}

	for _, name := range []string{
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY",
		"CLAUDE_CODE_DISABLE_ADVISOR_TOOL",
		"DISABLE_TELEMETRY", "DISABLE_ERROR_REPORTING",
	} {
		if session[name] == "" {
			t.Errorf("%s is not set", name)
		}
	}

	// The credential names are not this map's to hold. They are settled separately and
	// unconditionally, and a preference that could overwrite one would be a way to point
	// the child somewhere else.
	for name := range session {
		if name == "ANTHROPIC_BASE_URL" || name == "ANTHROPIC_AUTH_TOKEN" {
			t.Errorf("%s is a preference here; it must not be one", name)
		}
	}
}

// The one value that is not a preference.
func TestOnlyTheNonStreamingGuardIsEnforced(t *testing.T) {
	enforced := sessionRequirements()
	if len(enforced) != 1 || enforced["CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK"] != "1" {
		t.Fatalf("enforced = %v. This build refuses a non-streaming request, so a client "+
			"free to fall back to one produces a broken turn rather than a slower answer. "+
			"Everything else is a preference the user may hold differently.", enforced)
	}
}

// NATIVE_SYNTH. The real client, a fixture backend, and no inference spent.
//
// Every claim the map makes is a claim about what this client does with a name. None of
// them can be established by reading the map, and the Node baseline reaches the same
// results by passing --model, --effort and a settings blob on the command line -- so
// "the environment is enough" is exactly the thing that had to be measured.
func TestTheRealClientHonoursTheSession(t *testing.T) {
	for _, c := range []struct {
		name          string
		env           map[string]string
		args          []string
		wantRequested string
		wantEffort    string
	}{
		{name: "the shipped session", wantRequested: startupModel.Model, wantEffort: startupModel.Effort},
		// --model beats ANTHROPIC_MODEL, which is why nothing here has to own an option.
		{name: "a user's own --model wins", args: []string{"--model", "gpt-5.6-terra"},
			wantRequested: "gpt-5.6-terra", wantEffort: startupModel.Effort},
		// ARG05 still holds: an option named inside a value is text.
		{name: "an option named inside a prompt is text",
			args:          []string{"--append-system-prompt", "--model is a string"},
			wantRequested: startupModel.Model, wantEffort: startupModel.Effort},
		// And the user keeps the effort choice the baseline hands out as --effort.
		{name: "a user's own effort wins", env: map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "max"},
			wantRequested: startupModel.Model, wantEffort: "max"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, value := range c.env {
				t.Setenv(name, value)
			}
			calls, lists := sessionRun(t, c.args)
			if len(calls) == 0 {
				t.Fatal("the client sent no inference")
			}
			if calls[0].Requested != c.wantRequested {
				t.Errorf("the client asked for %q, want %q", calls[0].Requested, c.wantRequested)
			}
			if calls[0].Effort != c.wantEffort {
				t.Errorf("effort = %q, want %q", calls[0].Effort, c.wantEffort)
			}
			// Model discovery is on, so the client asks for the list. Without that its
			// /model picker is the built-in Anthropic one: names this backend does not have.
			// Counted rather than inferred from the request total, which the readiness
			// probe alone already pushes past the inference count.
			if lists == 0 {
				t.Error("the client never asked for the model list, so its picker is still " +
					"the built-in Anthropic one")
			}
		})
	}
}

// sessionRun drives a real claude against a fixture backend and reports what it asked for.
func sessionRun(t *testing.T, args []string) ([]upstream.Call, int64) {
	t.Helper()
	recorder := &recordingTransport{inner: &upstream.Script{Default: measuredStream("ok")}}
	got := nativeRun{
		Args:      append([]string{"-p", "say ok"}, args...),
		Timeout:   90 * time.Second,
		transport: recorder,
	}.run(t)
	if got.err != nil {
		t.Fatalf("Run: %v", got.err)
	}
	return recorder.calls, got.modelLists
}

// environmentNames is a guard against a name drifting out of the set without anyone
// noticing, because every one of them was measured individually.
func TestTheSessionNamesAreTheMeasuredOnes(t *testing.T) {
	want := strings.Fields(`
		CLAUDE_CODE_RETRY_WATCHDOG DISABLE_TELEMETRY DISABLE_ERROR_REPORTING
		CLAUDE_CODE_RESUME_INTERRUPTED_TURN ANTHROPIC_MODEL CLAUDE_CODE_EFFORT_LEVEL
		ANTHROPIC_CUSTOM_MODEL_OPTION ANTHROPIC_CUSTOM_MODEL_OPTION_NAME
		ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY
		CLAUDE_CODE_DISABLE_ADVISOR_TOOL ANTHROPIC_DEFAULT_HAIKU_MODEL
		ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL`)
	session := sessionEnvironment()
	if len(session) != len(want) {
		t.Fatalf("the session carries %d names, the measured set is %d: %v",
			len(session), len(want), session)
	}
	for _, name := range want {
		if _, present := session[name]; !present {
			t.Errorf("%s left the session", name)
		}
	}
}

// recordingTransport keeps the calls so a test can read what the client asked for rather
// than what it happened to send as bytes.
type recordingTransport struct {
	inner upstream.Transport
	mu    sync.Mutex
	calls []upstream.Call
}

func (r *recordingTransport) Execute(ctx context.Context, call upstream.Call) (*upstream.Response, error) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	return r.inner.Execute(ctx, call)
}

// receivedBy reports how many requests a gateway answered, or zero if it never started.
func receivedBy(g *gateway.Gateway) int64 {
	if g == nil {
		return 0
	}
	received, _, _ := g.Stats()
	return received
}

func modelListsBy(g *gateway.Gateway) int64 {
	if g == nil {
		return 0
	}
	return g.ModelLists()
}
