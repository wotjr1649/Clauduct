package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/httpguard"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// The client's subagent hooks report here.
//
// A subagent is started by the client, not by this gateway, so the only way to know one
// exists -- and what role it was started as -- is for the client to say so. A SubagentStart
// hook posts that, a SubagentStop hook withdraws it, and what the registration is for is
// deciding which model the subagent's requests run on.
//
// Stop events additionally carry the existing final answer for parent delivery.
// Bodies remain transient; diagnostics expose only delivery state and byte count.

const (
	// maxAgents bounds the table. The baseline's number.
	maxAgents = 1024
	// agentIdle is how long a registration may go unused before a later registration may
	// sweep it. A SubagentStop normally removes one; this is for the sessions where that
	// hook never arrives, and it runs on registration rather than on a timer so a session
	// that has stopped registering stops paying for the bookkeeping too.
	agentIdle = 30 * time.Minute
)

// contextPolicy is the window the client told the subagent to work in.
//
// Carried because the client sends it and dropping a field it went to the trouble of
// reporting is how a record stops matching the thing it describes. Nothing acts on it yet.
type contextPolicy struct {
	Window            int64   `json:"window"`
	AutoCompactWindow int64   `json:"autoCompactWindow"`
	CompactPercent    float64 `json:"compactPercent"`
}

// agentBinding is what a hook reports.
type agentBinding struct {
	ID             string
	Role           string
	Stop           bool
	SessionID      string
	TranscriptPath string
	Context        *contextPolicy
	Result         string
}

type agentState struct {
	binding  agentBinding
	role     string
	lastUsed time.Time
	context  *contextPolicy
	// active is how many requests this registration currently has in flight. A
	// registration with live work is never swept and never evicted.
	active int
}

// agentRegistry is the table of live subagent registrations.
type agentRegistry struct {
	mu      sync.Mutex
	byID    map[string]*agentState
	expired int64
	evicted int64
}

func newAgentRegistry() *agentRegistry {
	return &agentRegistry{byID: make(map[string]*agentState, 8)}
}

// register applies one binding and reports whether a registration now exists for it.
func (a *agentRegistry) register(binding agentBinding, now time.Time) (registered bool, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// A role change under the same identifier is two different subagents wearing one name.
	// Refused rather than resolved, because either answer would be a guess about which one
	// the next request belongs to.
	if existing, known := a.byID[binding.ID]; known && existing.role != binding.Role {
		return false, errBindingConflict
	}

	if binding.Stop {
		delete(a.byID, binding.ID)
		return false, nil
	}

	// Registrations are removed by SubagentStop. For a session where that never arrives,
	// drop what has gone unused past the window -- but never something with live requests,
	// which is work in progress rather than a leftover.
	stale := now.Add(-agentIdle)
	for id, state := range a.byID {
		if id == binding.ID || state.active > 0 || state.lastUsed.After(stale) {
			continue
		}
		delete(a.byID, id)
		a.expired++
	}

	// The cap is the backstop for when even the window has not released enough.
	if _, known := a.byID[binding.ID]; !known && len(a.byID) >= maxAgents {
		evicted := false
		for id, state := range a.byID {
			if state.active > 0 {
				continue
			}
			delete(a.byID, id)
			a.evicted++
			evicted = true
			break
		}
		if !evicted {
			return false, errBindingLimit
		}
	}

	// Updated in place when the id is already here, rather than replaced.
	//
	// begin() hands out a release closure over the state it incremented. Replacing the
	// object leaves that closure decrementing something no longer in the map, so the new
	// state's active count stays zero however many requests are in flight -- and zero is
	// what both the idle sweep and the cap read as "not busy". A subagent that received a
	// second SubagentStart while streaming could then be evicted mid-answer, after which
	// its requests ran with no role at all.
	if existing, known := a.byID[binding.ID]; known {
		existing.binding = binding
		existing.role, existing.context, existing.lastUsed = binding.Role, binding.Context, now
		return true, nil
	}
	a.byID[binding.ID] = &agentState{binding: binding, role: binding.Role, lastUsed: now, context: binding.Context}
	return true, nil
}

func (a *agentRegistry) bindingOf(id string) agentBinding {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state := a.byID[id]; state != nil {
		return state.binding
	}
	return agentBinding{}
}

// roleOf reports the role a registration was started as.
func (a *agentRegistry) roleOf(id string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, known := a.byID[id]
	if !known {
		return "", false
	}
	state.lastUsed = time.Now()
	return state.role, true
}

// begin claims a registration for the duration of one request.
//
// The count is what makes "never sweep something with work in progress" true rather than
// intended. Without it the sweep and the cap are deciding about entries they cannot see the
// state of.
func (a *agentRegistry) begin(id string) (role string, release func(), ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, known := a.byID[id]
	if !known {
		return "", func() {}, false
	}
	state.active++
	state.lastUsed = time.Now()
	return state.role, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if state.active > 0 {
			state.active--
		}
		state.lastUsed = time.Now()
	}, true
}

// Retired reports how many registrations expired or were evicted to make room. Counted all
// along and reported nowhere until #91: an evicted child's next request is refused as
// unregistered, and this is the only place that says why.
func (a *agentRegistry) Retired() (expired, evicted int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.expired, a.evicted
}

// Registered reports how many subagent registrations are live.
func (a *agentRegistry) Registered() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.byID)
}

// handleAgents takes one binding from a hook.
func (g *Gateway) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.refuse(w, refuseMethod)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		g.refuse(w, refuseMediaType)
		return
	}
	// Bound the identity fields plus one existing completion report. No arbitrary
	// native event payload or transcript is accepted by this endpoint.
	body, ok := g.readBounded(w, r, maxBindingBytes)
	if !ok {
		return
	}

	binding, err := decodeAgentBinding(body)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "INVALID_AGENT_BINDING")
		return
	}
	if binding.Stop {
		prior := g.agents.bindingOf(binding.ID)
		if prior.SessionID != "" && (binding.SessionID != prior.SessionID || binding.TranscriptPath != prior.TranscriptPath) {
			g.refuseCategory(w, http.StatusBadRequest, "INVALID_AGENT_BINDING")
			return
		}
	}
	registered, err := g.agents.register(binding, time.Now())
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, err.Error())
		return
	}
	if binding.Stop && g.delegations != nil {
		g.delegations.stopped(binding)
	}

	reply, err := json.Marshal(struct {
		Registered bool `json:"registered"`
	}{registered})
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REPLY_ENCODE_FAILED")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(reply)
}

// maxBindingBytes bounds a hook's report.
const maxBindingBytes = resultBodyLimit*6 + 8192 // worst-case JSON escaping; decoded report remains bounded

// Event uploads need the same shutdown cancellation and body deadline as model
// requests. Their admission is held only while reading the bounded event body.
func (g *Gateway) readBounded(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	_, ctx, release, err := g.requests.admit(r.Context())
	if err != nil {
		if errors.Is(err, errGatewayClosed) {
			g.refuse(w, refuseClosed)
		} else {
			g.refuse(w, refuseBusy)
		}
		return nil, false
	}
	defer release()
	control := http.NewResponseController(w)
	_ = control.SetReadDeadline(time.Now().Add(requestBodyTimeout))
	stop := httpguard.WatchReadCancellation(ctx, func() { _ = control.SetReadDeadline(time.Now()) })
	defer stop()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		if ctx.Err() != nil {
			g.refuse(w, refuseCancelled)
		} else {
			g.refuse(w, refuseTooLarge)
		}
		return nil, false
	}
	return body, true
}

// decodeAgentBinding validates a hook's report before any of it is believed.
//
// Closed key set, every field bounded, and the shapes are the baseline's. This arrives from
// a process this build started but did not write -- the hook runs inside the client -- so
// it is checked like anything else that crosses a boundary.
func decodeAgentBinding(raw []byte) (agentBinding, error) {
	fields, err := wire.Fields(raw, []string{"id", "role", "stop", "sessionId",
		"transcriptPath", "contextPolicy", "result"})
	if err != nil {
		return agentBinding{}, errInvalidBinding
	}

	var binding agentBinding
	if value, present := wire.Of(fields, "result"); present == wire.Present {
		if json.Unmarshal(value, &binding.Result) != nil || len(binding.Result) > resultBodyLimit {
			return binding, errInvalidBinding
		}
	}
	if value, present := wire.Of(fields, "id"); present != wire.Present ||
		json.Unmarshal(value, &binding.ID) != nil || !correlationShape.MatchString(binding.ID) {
		return agentBinding{}, errInvalidBinding
	}
	if value, present := wire.Of(fields, "role"); present != wire.Present ||
		json.Unmarshal(value, &binding.Role) != nil ||
		binding.Role == "" || len(binding.Role) > 200 {
		return agentBinding{}, errInvalidBinding
	}
	if value, present := wire.Of(fields, "stop"); present != wire.Present ||
		json.Unmarshal(value, &binding.Stop) != nil {
		return agentBinding{}, errInvalidBinding
	}
	if value, present := wire.Of(fields, "sessionId"); present == wire.Present {
		if json.Unmarshal(value, &binding.SessionID) != nil ||
			!correlationShape.MatchString(binding.SessionID) {
			return agentBinding{}, errInvalidBinding
		}
	}
	if value, present := wire.Of(fields, "transcriptPath"); present == wire.Present {
		if json.Unmarshal(value, &binding.TranscriptPath) != nil ||
			len(binding.TranscriptPath) > 4096 {
			return agentBinding{}, errInvalidBinding
		}
	}
	if value, present := wire.Of(fields, "contextPolicy"); present == wire.Present {
		policy, err := decodeContextPolicy(value)
		if err != nil {
			return agentBinding{}, err
		}
		binding.Context = policy
	}
	return binding, nil
}

func decodeContextPolicy(raw json.RawMessage) (*contextPolicy, error) {
	fields, err := wire.Fields(raw, []string{"window", "autoCompactWindow", "compactPercent"})
	if err != nil || len(fields) != 3 {
		return nil, errInvalidBinding
	}
	var policy contextPolicy
	if json.Unmarshal(raw, &policy) != nil {
		return nil, errInvalidBinding
	}
	if policy.Window <= 0 || policy.AutoCompactWindow <= 0 ||
		policy.CompactPercent <= 0 || policy.CompactPercent > 100 {
		return nil, errInvalidBinding
	}
	return &policy, nil
}
