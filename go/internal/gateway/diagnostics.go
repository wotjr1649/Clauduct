package gateway

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// What this session can say about itself.
//
// The gateway is the only place that sees every request, so it is the only place that can
// answer what a session actually did. Until now it could answer with four counters, which
// tells a reader that something was refused and nothing about what.
//
// Everything here is a count, a fixed label or a timing. No request body, no header value,
// no prompt and no model output is recorded, and nothing derived from them is either. A
// diagnostic that has to be redacted before it can be read is one nobody reads.

// Stages a request passes through, in order. Named for where it actually got to, so a
// failure says which half of the bridge it belongs to: a request that died in prepare is
// this build's problem and one that died in upstream is the backend's.
//
// The Node baseline has seven, and two of them -- review, output-validation -- are not
// separate phases here. Naming stages that do not exist would make every record claim to
// have passed through them.
const (
	stageRequest   = "request"   // boundary, admission, body, decode
	stageSelection = "selection" // which model this runs on
	stagePrepare   = "prepare"   // building and encoding the backend request
	stageUpstream  = "upstream"  // the transport call
	stageDelivery  = "delivery"  // relaying the stream to the client
)

// Outcomes. A record that never reaches one is in progress, which is a third answer and not
// a missing one: a session read while a request is still running should say so.
const (
	outcomeOK       = "ok"
	outcomeRefused  = "refused"
	outcomeProgress = "in-progress"
)

// recentRequests is how many records are kept. The baseline's sixteen.
const recentRequests = 16

// RequestRecord is one request's account of itself.
type RequestRecord struct {
	Seq      int64  `json:"seq"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Stage    string `json:"stage"`
	Outcome  string `json:"outcome"`
	Category string `json:"category,omitempty"`
	Status   int    `json:"status,omitempty"`

	// CAP03: what was asked for and what ran, kept apart. A record that keeps only the
	// second cannot answer whether the session ran what the user chose.
	Requested string `json:"requested,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
	Source    string `json:"source,omitempty"`

	// Milliseconds from when this gateway started. Relative rather than wall clock: a
	// diagnostic that travels should not carry when the machine was running, and the
	// question a reader has is how long things took, not what time it was.
	StartedMs   int64  `json:"startedMs"`
	FirstByteMs *int64 `json:"firstByteMs,omitempty"`
	EndedMs     *int64 `json:"endedMs,omitempty"`
}

// record is the live half of a RequestRecord, mutated as the request proceeds.
type record struct {
	mu    sync.Mutex
	epoch time.Time
	data  RequestRecord
}

// at advances the stage. A stage is only ever set by the code that reached it, so the last
// one set is where the request got to.
func (r *record) at(stage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Stage = stage
}

// route records what this request runs on, once that is decided.
func (r *record) route(requested, model, effort, source string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Requested, r.data.Model = requested, model
	r.data.Effort, r.data.Source = effort, source
}

// refusedWith records the category a refusal was answered with.
//
// Called from the one place every refusal goes through, which is what makes "a refused
// request is a diagnosed failure rather than an unrecorded 400" true rather than intended.
func (r *record) refusedWith(status int, category string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Outcome, r.data.Category, r.data.Status = outcomeRefused, category, status
}

func (r *record) wroteStatus(status int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data.Status == 0 {
		r.data.Status = status
	}
}

// wrote stamps the first byte that reached the client.
//
// The one timing that separates a slow backend from a slow bridge: everything before it is
// this build's, everything after it is the stream's.
func (r *record) wrote() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data.Status == 0 {
		r.data.Status = http.StatusOK
	}
	// A refusal body is not a first byte. The stamp exists to separate a slow backend from
	// a slow bridge, and an error this gateway wrote itself never went near either.
	if r.data.FirstByteMs == nil && r.data.Outcome != outcomeRefused {
		elapsed := time.Since(r.epoch).Milliseconds()
		r.data.FirstByteMs = &elapsed
	}
}

// finish closes the record. A request that was never refused succeeded.
func (r *record) finish() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	elapsed := time.Since(r.epoch).Milliseconds()
	r.data.EndedMs = &elapsed
	if r.data.Outcome == outcomeProgress {
		r.data.Outcome = outcomeOK
	}
}

func (r *record) snapshot() RequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data
}

// ring keeps the most recent records.
type ring struct {
	mu      sync.Mutex
	epoch   time.Time
	seq     int64
	entries []*record
}

func newRing() *ring { return &ring{epoch: time.Now()} }

func (g *ring) open(method, path string) *record {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	entry := &record{epoch: g.epoch, data: RequestRecord{
		Seq: g.seq, Method: method, Path: path,
		Stage: stageRequest, Outcome: outcomeProgress,
		StartedMs: time.Since(g.epoch).Milliseconds(),
	}}
	g.entries = append(g.entries, entry)
	if len(g.entries) > recentRequests {
		g.entries = g.entries[len(g.entries)-recentRequests:]
	}
	return entry
}

func (g *ring) recent() []RequestRecord {
	g.mu.Lock()
	entries := append([]*record(nil), g.entries...)
	g.mu.Unlock()

	out := make([]RequestRecord, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.snapshot())
	}
	return out
}

// tracked carries a request's record alongside its ResponseWriter.
//
// The record has to reach the refusal helper and the stream writer, and both of them are
// given a ResponseWriter and nothing else. Threading a second argument through every call
// site would have been the same change made fifteen times.
type tracked struct {
	http.ResponseWriter
	rec *record
}

// Unwrap is what http.NewResponseController follows to reach the real writer, which is
// where the deadlines and the flush have to land.
func (t *tracked) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t *tracked) WriteHeader(status int) {
	t.rec.wroteStatus(status)
	t.ResponseWriter.WriteHeader(status)
}

func (t *tracked) Write(p []byte) (int, error) {
	t.rec.wrote()
	return t.ResponseWriter.Write(p)
}

// recordOf finds the record behind a ResponseWriter, or nil when there is none.
func recordOf(w http.ResponseWriter) *record {
	if t, ok := w.(*tracked); ok {
		return t.rec
	}
	return nil
}

// Diagnostics is the whole account, as GET /clauduct/status answers it.
type Diagnostics struct {
	UptimeMs int64            `json:"uptimeMs"`
	Requests RequestCounts    `json:"requests"`
	Agents   AgentCounts      `json:"agents"`
	Betas    BetaReport       `json:"betas"`
	Limits   *RateLimitReport `json:"rateLimit,omitempty"`
	Recent   []RequestRecord  `json:"recent"`
}

type RequestCounts struct {
	Received   int64 `json:"received"`
	Refused    int64 `json:"refused"`
	Active     int64 `json:"active"`
	ModelLists int64 `json:"modelLists"`
}

type AgentCounts struct {
	Registered   int   `json:"registered"`
	Unregistered int64 `json:"unregistered"`
	Unrouted     int64 `json:"unrouted"`
}

// Diagnose is the account, readable in a session and at the end of one.
func (g *Gateway) Diagnose() Diagnostics {
	received, refused, active := g.Stats()
	unregistered, unrouted := g.Unrouted()
	return Diagnostics{
		UptimeMs: time.Since(g.ring.epoch).Milliseconds(),
		Requests: RequestCounts{
			Received: received, Refused: refused,
			Active: active, ModelLists: g.ModelLists(),
		},
		Agents: AgentCounts{
			Registered:   g.agents.Registered(),
			Unregistered: unregistered,
			Unrouted:     unrouted,
		},
		Betas:  g.betas.report(),
		Limits: g.limits.report(),
		Recent: g.ring.recent(),
	}
}

// statusPath is where a session reads its own account.
const statusPath = "/clauduct/status"

// handleStatus answers with the account. Authorised like every other route: a session's
// record of what it ran is not public, and this listener is reachable by anything on the
// machine that can guess a port.
func (g *Gateway) handleStatus(w http.ResponseWriter) {
	body, err := json.Marshal(g.Diagnose())
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "STATUS_ENCODE_FAILED")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
