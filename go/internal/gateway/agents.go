package gateway

import (
	"encoding/json"
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
// hook posts that. With native event receipts, SubagentStop withdraws it only after
// that turn actually ends; another Stop hook can request more work in the same turn.
// The registration decides which model the subagent's requests run on.
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
	stop   *agentStop
}

// SubagentStop is a proposal: another native hook may keep the same turn alive.
type agentStop struct {
	binding agentBinding
	turn    string
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
		existing.stop = nil
		return true, nil
	}
	a.byID[binding.ID] = &agentState{binding: binding, role: binding.Role, lastUsed: now, context: binding.Context}
	return true, nil
}

func (a *agentRegistry) deferStop(binding agentBinding, turn string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.byID[binding.ID]
	if s == nil {
		return false, nil
	}
	if s.role != binding.Role || s.binding.SessionID != binding.SessionID || s.binding.TranscriptPath != binding.TranscriptPath {
		return false, errBindingConflict
	}
	bytes := len(binding.Result)
	for id, state := range a.byID {
		if id != binding.ID && state.stop != nil {
			bytes += len(state.stop.binding.Result)
		}
	}
	if bytes > resultMemoryLimit {
		return false, errBindingLimit
	}
	s.stop = &agentStop{binding: binding, turn: turn}
	return true, nil
}

func (a *agentRegistry) stopOf(id string) *agentStop {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.byID[id]; s != nil {
		return s.stop
	}
	return nil
}

func (a *agentRegistry) continueStop(id string, stop *agentStop) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.byID[id]; s != nil && s.stop == stop {
		s.stop = nil
	}
}

func (a *agentRegistry) finishStop(receipt nativeTurnReceipt) (agentBinding, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.byID[receipt.Agent]
	if s == nil || s.stop == nil || s.stop.turn != receipt.Turn || s.stop.binding.SessionID != receipt.Session {
		return agentBinding{}, false
	}
	binding := s.stop.binding
	delete(a.byID, receipt.Agent)
	return binding, true
}

func (a *agentRegistry) bindingOf(id string) agentBinding {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state := a.byID[id]; state != nil {
		return state.binding
	}
	return agentBinding{}
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
	body, release, ok := g.readBounded(w, r, maxBindingBytes)
	if !ok {
		return
	}
	defer release()

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
	var registered bool
	deferred := binding.Stop && g.nativeEvents.directory != ""
	if deferred {
		turn, found, readErr := g.readCurrentNativeTurn(binding.ID)
		if readErr != nil || !found || !validActiveReceipt(turn, binding.SessionID, binding.ID) {
			g.refuseCategory(w, http.StatusBadRequest, "INVALID_AGENT_BINDING")
			return
		}
		registered, err = g.agents.deferStop(binding, turn.Turn)
		if err == nil && g.delegations != nil {
			r := &g.delegations.results
			r.mu.Lock()
			if e := r.entries[binding.ID]; e != nil && e.Session == turn.Session && e.NativeTurn == turn.Turn && e.NativeEndObserved {
				turn.Reason = e.EndReason
			}
			r.mu.Unlock()
			g.finishNativeAgentStop(turn)
			registered = g.agents.bindingOf(binding.ID).ID != ""
		}
	} else {
		registered, err = g.agents.register(binding, time.Now())
	}
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, err.Error())
		return
	}
	if binding.Stop && !deferred && g.delegations != nil {
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

// The caller holds the reservation through decoding and event processing, not
// just the upload. A small completion event can also recover an existing report.
func (g *Gateway) readBounded(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, func(), bool) {
	ctx, release, admitted := g.admitRequest(w, r, limit, controlAdmission)
	if !admitted {
		return nil, nil, false
	}
	control := http.NewResponseController(w)
	_ = control.SetReadDeadline(time.Now().Add(requestBodyTimeout))
	stop := httpguard.WatchReadCancellation(ctx, func() { _ = control.SetReadDeadline(time.Now()) })
	defer stop()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		defer release()
		if ctx.Err() != nil {
			g.refuse(w, refuseCancelled)
		} else {
			g.refuse(w, refuseTooLarge)
		}
		return nil, nil, false
	}
	return body, release, true
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
