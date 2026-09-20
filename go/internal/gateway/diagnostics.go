package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
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
	stageCount     = "count"     // exact local/backend preflight
	stageUpstream  = "upstream"  // the transport call
	stageDelivery  = "delivery"  // relaying the stream to the client
)

// Outcomes. A record that never reaches one is in progress, which is a third answer and not
// a missing one: a session read while a request is still running should say so.
const (
	outcomeOK      = "ok"
	outcomeRefused = "refused"
	// outcomeBroken is a response that started and then stopped. It is neither refused --
	// the client got a 200 -- nor ok, and calling it either would be false.
	outcomeBroken   = "broken"
	outcomeProgress = "in-progress"
)

// recentRequests is how many records are kept. The baseline's sixteen.
const recentRequests = 16

// refusedPathLimit is how many distinct refused paths the account will name.
//
// Small because the point is to name what a client keeps asking for, and a real one asks
// for very few. Bounded at all because refusals happen before a credential is checked:
// anything on this machine that can guess the port can put entries here.
const refusedPathLimit = 8

// RequestRecord is one request's account of itself.
type RequestRecord struct {
	Seq                int64  `json:"seq"`
	Method             string `json:"method"`
	Path               string `json:"path"`
	Stage              string `json:"stage"`
	Outcome            string `json:"outcome"`
	Category           string `json:"category,omitempty"`
	CancellationSource string `json:"cancellationSource,omitempty"`
	ToolPolicy         string `json:"toolPolicy,omitempty"`
	Control            string `json:"control,omitempty"`
	Status             int    `json:"status,omitempty"`

	// CAP03: what was asked for and what ran, kept apart. A record that keeps only the
	// second cannot answer whether the session ran what the user chose.
	Requested             string           `json:"requested,omitempty"`
	Model                 string           `json:"model,omitempty"`
	Effort                string           `json:"effort,omitempty"`
	Source                string           `json:"source,omitempty"`
	Kind                  string           `json:"kind"`
	RequestClass          string           `json:"nativeRequestClass,omitempty"`
	InputTokens           *int64           `json:"backendInputTokens,omitempty"`
	OutputTokens          *int64           `json:"backendOutputTokens,omitempty"`
	CachedInputTokens     *int64           `json:"backendCachedInputTokens,omitempty"`
	ReasoningTokens       *int64           `json:"backendReasoningTokens,omitempty"`
	UsageSource           string           `json:"usageSource,omitempty"`
	ContextEstimate       *ContextEstimate `json:"contextEstimate,omitempty"`
	CountedInputTokens    *int64           `json:"countedInputTokens,omitempty"`
	AgentID               string           `json:"agentId,omitempty"`
	ParentAgentID         string           `json:"parentAgentId,omitempty"`
	VerifiedParentAgentID string           `json:"verifiedParentAgentId,omitempty"`
	AgentRole             string           `json:"agentRole,omitempty"`
	SelectionVerified     *bool            `json:"selectionVerified,omitempty"`
	CountSource           string           `json:"countSource,omitempty"`
	CountMethod           string           `json:"countMethod,omitempty"`
	CountAgreement        string           `json:"countAgreement,omitempty"`
	CountMs               *int64           `json:"countMs,omitempty"`
	StreamEnd             *StreamEndRecord `json:"streamEnd,omitempty"`
	VerifiedChecks        []string         `json:"verifiedChecks,omitempty"`
	RejectedWorkflowCalls int64            `json:"rejectedWorkflowCalls,omitempty"`
	LastObservedMs        int64            `json:"lastObservedMs"`
	BackendProgress       BackendProgress  `json:"backendProgress"`
	ParentReadiness       *ParentReadiness `json:"parentReadiness,omitempty"`

	// Milliseconds from when this gateway started. Relative rather than wall clock: a
	// diagnostic that travels should not carry when the machine was running, and the
	// question a reader has is how long things took, not what time it was.
	StartedMs   int64  `json:"startedMs"`
	FirstByteMs *int64 `json:"firstByteMs,omitempty"`
	EndedMs     *int64 `json:"endedMs,omitempty"`
}

type ContextEstimate struct {
	Input  int64  `json:"inputTokens"`
	Source string `json:"source"`
	Opaque bool   `json:"containsUnestimatedMediaOrReasoning"`
	Exact  bool   `json:"exact"`
}

func (r *record) contextEstimate(tokens int64, source string, opaque bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.ContextEstimate = &ContextEstimate{Input: tokens, Source: source, Opaque: opaque}
}

// Closed labels and counters only. Never preserve transport error text, URLs,
// headers or payloads in a diagnostic.
type StreamEndRecord struct {
	ReadError        string `json:"readError"`
	ClientContext    string `json:"clientContext"`
	TerminalObserved bool   `json:"terminalObserved"`
	Events           int    `json:"events"`
	Bytes            int    `json:"bytes"`
}

func streamErrorLabel(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, io.EOF):
		return "eof"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected_eof"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "transport_error"
	}
}

func (r *record) streamEnd(readErr, clientErr error, terminal bool, events, bytes int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.StreamEnd = &StreamEndRecord{streamErrorLabel(readErr), streamErrorLabel(clientErr), terminal, events, bytes}
}

// record is the live half of a RequestRecord, mutated as the request proceeds.
type record struct {
	mu    sync.Mutex
	epoch time.Time
	data  RequestRecord
	owner *ring
}

func (r *record) rejectedWorkflow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.RejectedWorkflowCalls++
}

func (r *record) requestClass(class string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.RequestClass = class
}

func (r *record) countMethod(method string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.CountMethod = method
}

func (r *record) counted(tokens int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.CountedInputTokens = &tokens
}

func (r *record) preflight(tokens int64, source string, elapsed int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if tokens > 0 {
		r.data.CountedInputTokens = &tokens
	}
	r.data.CountSource, r.data.CountMs = source, &elapsed
}

func (r *record) agent(id, parent, role string, verified bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.AgentID, r.data.ParentAgentID, r.data.AgentRole, r.data.SelectionVerified = id, parent, role, &verified
}

func (r *record) continuation(parent string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.VerifiedParentAgentID = parent
}

func (r *record) usage(usage codex.Usage) {
	if r == nil || !usage.InputKnown {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	input := usage.InputTokens
	r.data.InputTokens = &input
	r.data.UsageSource = "backend"
	if usage.OutputKnown {
		value := usage.OutputTokens
		r.data.OutputTokens = &value
	}
	if usage.CachedInputKnown {
		value := usage.CachedInputTokens
		r.data.CachedInputTokens = &value
	}
	if usage.ReasoningKnown {
		value := usage.ReasoningTokens
		r.data.ReasoningTokens = &value
	}
}

func (r *record) kind(kind string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Kind = kind
}

func (r *record) control(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Control = reason
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
	r.data.LastObservedMs = time.Since(r.epoch).Milliseconds()
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

// streamBroke counts a broken stream and marks the record, in that order and in one place.
//
// The count is a session total and the record is one request's account of itself. Keeping
// them in the same call is what stops a delivery failing upstream without the session total
// knowing -- which is the failure this counter exists to report.
//
// Only a cancellation established by the request context is excluded from Broken.
func (g *Gateway) streamBroke(w http.ResponseWriter, category string) {
	g.broken.Add(1)
	recordOf(w).brokeAfterCommitting(category)
}

func (g *Gateway) deliveryFailed(ctx context.Context, w http.ResponseWriter) {
	if errors.Is(ctx.Err(), context.Canceled) {
		recordOf(w).brokeAfterCommitting(refuseCancelled.category)
		return
	}
	g.streamBroke(w, "DELIVERY_FAILED")
}

// brokeAfterCommitting records a stream that failed once its status was already sent.
//
// Without it these were the one failure the account called a success: the status is 200 and
// already written, so nothing goes through refusedWith, and finish() closes an unrefused
// record as ok. A response that stopped halfway with TRUNCATED_STREAM or TEXT_MISMATCH was
// filed as a clean session with Refused == 0, and the exit line said nothing.
//
// The status stays what the client actually received. Rewriting it to an error would claim
// the client saw something it did not; the outcome and the category carry the truth.
func (r *record) brokeAfterCommitting(category string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data.Outcome, r.data.Category = outcomeBroken, category
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
	if r.data.EndedMs != nil {
		return
	}
	elapsed := time.Since(r.epoch).Milliseconds()
	r.data.EndedMs = &elapsed
	if r.data.Outcome == outcomeProgress {
		r.data.Outcome = outcomeOK
	}
	if r.owner != nil {
		r.owner.count(r.data)
	}
}

// path reports where this record's request was addressed.
func (r *record) path() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data.Path
}

func (r *record) snapshot() RequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data
}

// ring keeps the most recent records.
type ring struct {
	mu           sync.Mutex
	epoch        time.Time
	seq          int64
	entries      []*record
	failures     []RequestRecord
	totals       SessionTotals
	contextUsage map[string]ContextObservation
	features     map[string]FeatureEvidence
}

// Preflight totals and completed backend usage are separate observations. Neither
// is a reading of the current size of an agent conversation between requests.
type ContextObservation struct {
	CountMatches    int64  `json:"countMatches"`
	CountMismatches int64  `json:"countMismatches"`
	Preflights      int64  `json:"preflights"`
	CountsVerified  int64  `json:"successfulPreflightCounts"`
	CountCacheHits  int64  `json:"countCacheHits"`
	CountShared     int64  `json:"countShared"`
	CountMs         int64  `json:"countMs"`
	Compactions     int64  `json:"compactionRequests"`
	Requests        int64  `json:"requestsWithBackendUsage"`
	LastInputTokens *int64 `json:"lastCompletedInputTokens,omitempty"`
	PeakInputTokens *int64 `json:"peakCompletedInputTokens,omitempty"`
}

type ModelContextReport struct {
	CountQuarantine string               `json:"countQuarantine,omitempty"`
	Model           string               `json:"model"`
	Target          bridge.ContextPolicy `json:"target"`
	Application     string               `json:"application"`
	Verification    string               `json:"verification"`
	Reason          string               `json:"reason"`
	Observed        ContextObservation   `json:"observed"`
}

func (g *ring) contextReport() []ModelContextReport {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]ModelContextReport, 0, len(bridge.Models))
	for _, model := range bridge.Models {
		observed := g.contextUsage[model.ID]
		if observed.LastInputTokens != nil {
			value := *observed.LastInputTokens
			observed.LastInputTokens = &value
		}
		if observed.PeakInputTokens != nil {
			value := *observed.PeakInputTokens
			observed.PeakInputTokens = &value
		}
		out = append(out, ModelContextReport{Model: model.ID, Target: model.Context,
			Application: "not_enforced", Verification: "unverified",
			Reason: "MODEL_CONTEXT_ENFORCEMENT_NOT_IMPLEMENTED", Observed: observed})
	}
	return out
}

// Totals count completed requests, including records evicted from Recent.
// Keys are fixed request kinds, validated routes and project-owned categories.
type SessionTotals struct {
	MeasuredRequests      int64            `json:"requestsWithUsage"`
	UnmeasuredRequests    int64            `json:"generationRequestsWithoutUsage"`
	InputTokens           int64            `json:"backendInputTokens"`
	OutputTokens          int64            `json:"backendOutputTokens"`
	RejectedWorkflowCalls int64            `json:"rejectedWorkflowCalls"`
	Completed             int64            `json:"completed"`
	Kinds                 map[string]int64 `json:"kinds"`
	Routes                map[string]int64 `json:"routes"`
	KindRoutes            map[string]int64 `json:"kindRoutes"`
	Failures              map[string]int64 `json:"failures"`
	Controls              map[string]int64 `json:"controls"`
}

func newRing() *ring {
	return &ring{epoch: time.Now(), contextUsage: map[string]ContextObservation{}, totals: SessionTotals{
		Kinds: map[string]int64{}, Routes: map[string]int64{}, KindRoutes: map[string]int64{}, Failures: map[string]int64{}, Controls: map[string]int64{},
	}}
}

func (g *ring) count(r RequestRecord) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.countFeatures(r)
	g.totals.Completed++
	g.totals.RejectedWorkflowCalls += r.RejectedWorkflowCalls
	g.totals.Kinds[r.Kind]++
	if r.InputTokens != nil && r.OutputTokens != nil {
		g.totals.MeasuredRequests++
		g.totals.InputTokens += *r.InputTokens
		g.totals.OutputTokens += *r.OutputTokens
	} else if (r.Kind == "generation" || r.Kind == "compaction") && r.Control == "" {
		g.totals.UnmeasuredRequests++
	}
	if r.Kind == "compaction" {
		observed := g.contextUsage[r.Model]
		observed.Compactions++
		g.contextUsage[r.Model] = observed
	}
	if r.CountSource != "" && r.Kind != "count_tokens" {
		if _, known := policyFor(r.Model); known {
			observed := g.contextUsage[r.Model]
			if r.CountSource != "prior-count-cache" {
				observed.Preflights++
			}
			if r.CountAgreement == "matched" {
				observed.CountMatches++
			}
			if r.CountAgreement == "mismatched" {
				observed.CountMismatches++
			}
			if r.CountedInputTokens != nil && r.CountSource != "prior-count-cache" {
				observed.CountsVerified++
			}
			if r.CountSource == "exact-count-cache" {
				observed.CountCacheHits++
			}
			if r.CountSource == "exact-count-shared" {
				observed.CountShared++
			}
			if r.CountMs != nil {
				observed.CountMs += *r.CountMs
			}
			g.contextUsage[r.Model] = observed
		}
	}
	if r.Model != "" {
		g.totals.Routes[r.Model+"/"+r.Effort+"/"+r.Source]++
		g.totals.KindRoutes[r.Kind+"/"+r.Model+"/"+r.Effort+"/"+r.Source]++
	}
	if r.Control != "" {
		g.totals.Controls[r.Control]++
	} else if r.Category != "" {
		g.totals.Failures[r.Category]++
		g.failures = append(g.failures, r)
		if len(g.failures) > recentRequests {
			g.failures = g.failures[len(g.failures)-recentRequests:]
		}
	}
	if r.InputTokens != nil {
		// Bound this table to the owned catalogue, even if a future caller records
		// an unvalidated model name. No request-supplied keys accumulate here.
		for _, model := range bridge.Models {
			if model.ID != r.Model {
				continue
			}
			seen := g.contextUsage[r.Model]
			seen.Requests++
			last := *r.InputTokens
			seen.LastInputTokens = &last
			if seen.PeakInputTokens == nil || last > *seen.PeakInputTokens {
				peak := last
				seen.PeakInputTokens = &peak
			}
			g.contextUsage[r.Model] = seen
			break
		}
	}
}

func (g *ring) recentFailures() []RequestRecord {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]RequestRecord(nil), g.failures...)
}

func (g *ring) counts() SessionTotals {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := g.totals
	out.Kinds = maps.Clone(out.Kinds)
	out.Routes = maps.Clone(out.Routes)
	out.KindRoutes = maps.Clone(out.KindRoutes)
	out.Failures = maps.Clone(out.Failures)
	out.Controls = maps.Clone(out.Controls)
	return out
}

func (g *ring) open(method, path string) *record {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	kind := "other"
	switch path {
	case "/v1/messages":
		kind = "generation"
	case "/v1/messages/count_tokens":
		kind = "count_tokens"
	case "/v1/models":
		kind = "models"
	case "/clauduct/agents":
		kind = "agent_binding"
	case "/clauduct/context":
		kind = "context_event"
	case "/api/hello":
		kind = "readiness"
	}
	entry := &record{epoch: g.epoch, owner: g, data: RequestRecord{
		Seq: g.seq, Method: method, Path: path,
		Kind:  kind,
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
// ClientReport is which client this session ran against, and whether that is the one the
// wire rules were measured on.
//
// Version is what the client called itself; empty means it never did. Verified false is not
// a fault -- it says the client has moved and nothing has re-measured it yet, which is the
// one fact a session that started failing after an auto-update needs to hand over.
type ClientReport struct {
	Version         string `json:"version,omitempty"`
	Reference       string `json:"reference"`
	Verified        bool   `json:"verified"`
	VerifiedMeaning string `json:"verifiedMeaning"`
}

type Diagnostics struct {
	UptimeMs            int64                          `json:"uptimeMs"`
	Requests            RequestCounts                  `json:"requests"`
	Agents              AgentCounts                    `json:"agents"`
	Betas               BetaReport                     `json:"betas"`
	Client              ClientReport                   `json:"client"`
	Features            []FeatureEvidence              `json:"features"`
	Progress            NativeProgressReport           `json:"progress"`
	Limits              *RateLimitReport               `json:"rateLimit,omitempty"`
	Events              EventReport                    `json:"events"`
	Recent              []RequestRecord                `json:"recent"`
	RecentFailures      []RequestRecord                `json:"recentFailures,omitempty"`
	Totals              SessionTotals                  `json:"totals"`
	ModelContexts       []ModelContextReport           `json:"modelContexts"`
	AgentContexts       []AgentContextReport           `json:"agentContexts,omitempty"`
	AgentResults        AgentResultReport              `json:"agentResults"`
	NativeEvents        NativeEventReport              `json:"nativeEvents"`
	ContextDisplay      ContextDisplayReport           `json:"contextDisplay"`
	WorkflowPersistence WorkflowPersistenceReport      `json:"workflowPersistence"`
	NativeToolFailures  ToolFailureReport              `json:"nativeToolFailures"`
	AgentSelections     SelectionReport                `json:"agentSelections"`
	CountConnections    *upstream.CountConnectionStats `json:"countConnections,omitempty"`
	SearchTransport     *upstream.SearchStats          `json:"searchTransport,omitempty"`
}

type RequestCounts struct {
	Received int64 `json:"received"`
	Refused  int64 `json:"refused"`
	// RefusedBy is the same refusals split by the reason each was answered with. The
	// aggregate says a session refused something; without this nothing says what, and the
	// per-request detail that could have said it is sixteen slots deep in Recent.
	RefusedBy map[string]int64 `json:"refusedBy,omitempty"`
	// RefusedPaths names the distinct paths refusals were answered on, up to
	// refusedPathLimit of them. For UNSUPPORTED_ROUTE the category says only "a path this
	// gateway does not serve", so the path is the finding; a set at exactly the cap may be
	// truncated and a reader should treat it as "at least these".
	RefusedPaths []string `json:"refusedPaths,omitempty"`
	// Broken is responses that started and then stopped, for the life of the session.
	Broken     int64 `json:"broken"`
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
	g.reconcileNativeResults()
	return g.Snapshot()
}

// Snapshot reads in-memory facts and bounded progress receipts, never native
// transcripts or report bodies. Periodic checkpoints do not perform agent work.
func (g *Gateway) Snapshot() Diagnostics {
	received, refused, active := g.Stats()
	unregistered, unrouted := g.Unrouted()
	results := AgentResultReport{}
	selections := SelectionReport{}
	var countStats *upstream.CountConnectionStats
	var searchStats *upstream.SearchStats
	if searcher, ok := g.transport.(interface{ SearchStats() upstream.SearchStats }); ok {
		snapshot := searcher.SearchStats()
		searchStats = &snapshot
	}
	if counter, ok := g.transport.(interface {
		CountStats() upstream.CountConnectionStats
	}); ok {
		snapshot := counter.CountStats()
		countStats = &snapshot
	}
	if g.delegations != nil {
		results = g.delegations.results.report()
		selections = g.delegations.selectionReport()
	}
	return Diagnostics{
		UptimeMs: time.Since(g.ring.epoch).Milliseconds(),
		Requests: RequestCounts{
			Received: received, Refused: refused,
			RefusedBy:    g.RefusalsByCategory(),
			RefusedPaths: g.RefusedPaths(),
			Broken:       g.broken.Load(),
			Active:       active, ModelLists: g.ModelLists(),
		},
		Agents: AgentCounts{
			Registered:   g.agents.Registered(),
			Unregistered: unregistered,
			Unrouted:     unrouted,
		},
		Betas: g.betas.report(),
		Client: ClientReport{
			Version:         g.ClientVersion(),
			Reference:       ReferenceClient,
			Verified:        g.ClientVersion() == ReferenceClient,
			VerifiedMeaning: "version_match_only",
		},
		Features:            g.ring.featureReport(),
		Progress:            g.nativeProgressReport(),
		Limits:              g.limits.report(),
		Events:              g.events.report(),
		Recent:              g.ring.recent(),
		RecentFailures:      g.ring.recentFailures(),
		Totals:              g.ring.counts(),
		ModelContexts:       g.modelContexts(),
		AgentContexts:       g.agentContexts(),
		AgentResults:        results,
		NativeEvents:        g.nativeEventReport(),
		ContextDisplay:      g.contextDisplayReport(),
		WorkflowPersistence: g.workflowPersistenceReport(),
		NativeToolFailures:  g.toolFailures.snapshot(),
		AgentSelections:     selections,
		CountConnections:    countStats,
		SearchTransport:     searchStats,
	}
}

func (g *Gateway) modelContexts() []ModelContextReport {
	reports := g.ring.contextReport()
	g.counts.mu.Lock()
	defer g.counts.mu.Unlock()
	if g.contexts != nil {
		for i := range reports {
			reports[i].Application = "gateway_usage_preventive_compaction"
			reports[i].Verification = "not_observed"
			reports[i].Reason = "MANAGEMENT_TARGET_NOT_EXACT_ADMISSION_CAP"
			if reports[i].Observed.Requests > 0 {
				reports[i].Verification = "backend_usage_observed"
			}
			reports[i].CountQuarantine = g.counts.quarantine[reports[i].Model]
		}
	}
	return reports
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

// BrokenStreams counts responses that started and then stopped.
//
// A session with one of these has something to report and the refusal count cannot say so:
// the client received a 200.
//
// Reads the session total rather than counting Recent, which is what it used to do. Counting
// a sixteen-slot window meant the longer a session ran -- the longer it had had to break
// something -- the more certainly it reported that nothing broke.
func (d Diagnostics) BrokenStreams() int64 { return d.Requests.Broken }
