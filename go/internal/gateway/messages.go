package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
	"github.com/wotjr1649/Clauduct/go/internal/httpguard"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// readChunk is how much of the backend body is taken at a time. The parser does not care —
// chunk boundaries carry no meaning to it — so this is purely about not holding more than
// necessary.
const readChunk = 32 * 1024

// handleMessages runs one inference request end to end.
//
// The shape of the error handling is the important part. Before the first byte of the
// response is written, a failure is an HTTP status the client can act on. After it, the
// status is already sent and the only honest signal left is a terminal error event — so
// the point at which headers are committed is tracked explicitly rather than inferred.
func (g *Gateway) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.refuse(w, refuseMethod)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		g.refuse(w, refuseMediaType)
		return
	}
	if bad, ok := checkRequestHeaders(r); !ok {
		g.refuse(w, bad)
		return
	}
	// Capability, not the version label, decides admission. Check before
	// selection or result recovery so an old client sees the actual cause.
	if g.contexts != nil && r.Header.Get("X-Claude-Code-Request-Class") == "" {
		g.refuseCategory(w, http.StatusBadRequest, "CONTEXT_REQUEST_CLASS_UNVERIFIED")
		return
	}
	g.ReconcileNativeCancellations()

	// Admission happens before the body is read, so a request that cannot be served does
	// not first cost the memory of its own payload.
	_, ctx, release, err := g.requests.admit(r.Context())
	if err != nil {
		if errors.Is(err, errGatewayClosed) {
			g.refuse(w, refuseClosed)
			return
		}
		g.refuse(w, refuseBusy)
		return
	}
	defer release()

	// Cancelling a context does not interrupt a blocking read of the request body: the
	// handler would sit in the read while shutdown waited for it, which is what a 30s
	// Close deadline measured before this was here. The Node baseline reaches the same
	// place by destroying the socket. The stdlib equivalent is a read deadline, so the
	// deadline is both the cancellation mechanism and the ceiling on how long a client
	// may take to finish a body it has already started.
	control := http.NewResponseController(w)
	body, err := func() ([]byte, error) {
		readCtx, finish := g.bindNativeCancellation(ctx, r, recordOf(w), true)
		defer finish()
		_ = control.SetReadDeadline(time.Now().Add(requestBodyTimeout))
		stop := httpguard.WatchReadCancellation(readCtx, func() { _ = control.SetReadDeadline(time.Now()) })
		defer stop()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if readCtx.Err() != nil {
			return nil, readCtx.Err()
		}
		return body, err
	}()
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			g.refuse(w, refuseTooLarge)
		case ctx.Err() != nil || errors.Is(err, context.Canceled):
			// Record abandoned input as cancellation, including a replacement
			// connection whose predecessor the filter kept open.
			g.refuse(w, refuseCancelled)
		default:
			g.refuse(w, refuseHeader)
		}
		return
	}

	request, err := anthropic.DecodeRequest(body, anthropic.Options{
		ToolChanges: negotiated(r, betaToolChanges),
	})
	if err != nil {
		var refusal *anthropic.RequestError
		if errors.As(err, &refusal) {
			g.refuseCategory(w, http.StatusBadRequest, refusal.Code)
			return
		}
		g.refuse(w, refuseHeader)
		return
	}

	// Adapted selections must be linked to the native child before dispatch. Calls
	// outside that extension retain the legacy role/request selection below.
	// Classified, never refused -- see betas.go. Observed after the body is decoded so the
	// header of a request that never became one is not counted as a feature the session
	// asked for.
	g.betas.observe(r.Header.Get("Anthropic-Beta"))

	entry := recordOf(w)
	entry.checked("input")
	entry.requestClass(r.Header.Get("X-Claude-Code-Request-Class"))
	entry.at(stageSelection)
	execution, category := g.claimNativeExecution(r, entry, body, request)
	if category != "" {
		g.refuseCategory(w, http.StatusBadRequest, category)
		return
	}
	entry.execution = execution
	defer execution.release()

	scope := delegationScope{session: r.Header.Get("X-Claude-Code-Session-Id"), parent: r.Header.Get("X-Claude-Code-Parent-Agent-Id")}
	scope.workflow = r.Header.Get("X-Claude-Code-Request-Class") == "workflow"
	defer g.recordFailedAgentRequest(scope.session, r.Header.Get("X-Claude-Code-Agent-Id"), entry)
	g.stripContextDisplays(request, scope.session)
	g.reconcileNativeResults()
	g.reconcileWorkflowResults(false)
	g.observeMessageFailures(request, scope.session, r.Header.Get("X-Claude-Code-Agent-Id"))
	if g.delegations != nil {
		g.delegations.restoreSelectionHistory(request, scope.session, r.Header.Get("X-Claude-Code-Agent-Id"))
		g.delegations.toolFailures(scope.session, request)
	}
	override, releaseAgent, err := g.agentSelection(r, request, entry)
	defer releaseAgent()
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, selectionCategory(err))
		return
	}
	ctx, finishCancellation := g.bindNativeCancellation(ctx, r, entry, false)
	defer finishCancellation()

	entry.at(stagePrepare)
	if g.delegations != nil && !g.delegations.restrictWorkflowTools(request, scope.session, r.Header.Get("X-Claude-Code-Agent-Id"), entry) {
		g.refuseCategory(w, 400, "WORKFLOW_TOOL_POLICY")
		return
	}
	// Search uses the same selection as generation, including an agent override.
	if request.HostedSearch != nil {
		entry.kind("web_search")
		query, ok := bridge.SideQuery(request)
		if !ok {
			g.refuseCategory(w, http.StatusBadRequest, anthropic.CodeHostedToolUnsupp)
			return
		}
		g.searchFor(ctx, w, control, request, query, override...)
		return
	}
	finishResults := g.deliverResults(r, request, entry)
	parentWait, waitErr := g.prepareParentWait(r, request, entry)
	if waitErr != nil {
		g.refuseCategory(w, http.StatusBadRequest, "PARENT_WAIT_UNVERIFIED")
		return
	}
	if g.delegations != nil && r.Header.Get("X-Claude-Code-Agent-Id") != "" && conversationRequest(r, request) && r.Header.Get("X-Claude-Code-Request-Class") != "compaction" {
		agentID := r.Header.Get("X-Claude-Code-Agent-Id")
		// Read and validate the turn receipt before beginResult, which is destructive: a
		// refusal after it leaves the request unrun and the evidence it needed to recover
		// already cleared.
		receipt := entry.nativeTurn
		if receipt == nil && g.nativeEvents.directory != "" {
			g.refuseCategory(w, 400, "NATIVE_TURN_UNVERIFIED")
			return
		}
		if !g.delegations.beginResult(agentID) {
			g.refuseCategory(w, 400, "AGENT_RESULT_CAPACITY")
			return
		}
		if receipt != nil && !g.applyNativeTurn(agentID, *receipt) {
			g.refuseCategory(w, 400, "NATIVE_TURN_UNVERIFIED")
			return
		}
		if g.nativeEvents.directory != "" {
			entry.checked("native_turn")
		}
	}
	resultsDelivered := false
	defer func() { finishResults(resultsDelivered) }()
	override, finishContext, contextError := g.beginContext(r, request, entry, override)
	defer finishContext()
	if contextError != "" {
		g.refuseCategory(w, http.StatusBadRequest, contextError)
		return
	}
	backendRequest, err := bridge.BuildRequest(request, override...)
	if err != nil {
		// CAP06: a model this build cannot route is the caller's answerable problem, not
		// an internal failure. Reporting it as a 500 was wrong twice over -- it told the
		// user nothing they could act on, and the measured client retries every 5xx, so a
		// request that can never succeed was sent eight times in a minute.
		if errors.Is(err, bridge.ErrUnsupportedRoute) {
			g.refuseCategory(w, http.StatusBadRequest, routeCategory(err))
			return
		}
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_CONVERSION_FAILED")
		return
	}
	if g.delegations != nil {
		if entry.snapshot().Kind != "compaction" {
			g.delegations.describeWorkflowStep(backendRequest, scope.session, r.Header.Get("X-Claude-Code-Agent-Id"))
		}
		if err := g.delegations.describe(backendRequest.Tools, r.Header.Get("X-Claude-Code-Agent-Id")); err != nil {
			g.refuseCategory(w, http.StatusBadRequest, "AGENT_SELECTION_UNVERIFIED")
			return
		}
	}
	if entry.snapshot().Kind == "compaction" {
		addCompactGuidance(backendRequest)
	}
	if err := g.prepareDocuments(ctx, backendRequest); err != nil {
		g.refuseCategory(w, 400, err.Error())
		return
	}
	encoded, err := json.Marshal(backendRequest)
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_ENCODE_FAILED")
		return
	}

	// Requested and effective are handed over separately and deliberately. The user is
	// billed for the second one, and a record that only keeps it cannot answer whether the
	// session ran what was asked for.
	entry.route(request.Model, backendRequest.Model,
		backendRequest.Effort.Effort, backendRequest.Source)
	entry.checked("route")
	if !g.checkContext(w, r, request, backendRequest) {
		return
	}
	if entry.snapshot().Kind == "compaction" {
		entry.checked("compaction_authorized")
	}
	entry.at(stageUpstream)
	g.priorCount(encoded, entry)
	if err := execution.dispatch(ctx); err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	response, err := g.transport.Execute(ctx, upstream.Call{
		Body:      encoded,
		Requested: request.Model,
		Model:     backendRequest.Model,
		Effort:    backendRequest.Effort.Effort,
		Source:    backendRequest.Source,
	})
	execution.rejectedBeforeDispatch(err)
	if err != nil {
		if categoryFor(err) == "CONTEXT_LENGTH_EXCEEDED" && g.recoverContextOverflow(w, scope.session, r.Header.Get("X-Claude-Code-Agent-Id"), backendRequest.Model) {
			return
		}
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	defer response.Body.Close()
	if g.delegations != nil {
		g.delegations.observeBackend(r.Header.Get("X-Claude-Code-Agent-Id"), false)
	}

	// What the backend volunteered about the quota. Read, never acted on.
	g.limits.observe(response.Header)

	entry.at(stageDelivery)
	resultsDelivered = g.relay(ctx, w, control, response, request, backendRequest.Model,
		delegationScope{session: scope.session, parent: r.Header.Get("X-Claude-Code-Agent-Id"), nativeModel: request.Model, parentWait: parentWait, route: bridge.Route{Model: backendRequest.Model, Effort: backendRequest.Effort.Effort, Source: backendRequest.Source}})
	g.rememberUsage(encoded, entry)
	if resultsDelivered && g.delegations != nil {
		g.delegations.observeBackend(r.Header.Get("X-Claude-Code-Agent-Id"), true)
	}
}

// Generation, search and token counting must resolve the same child selection.
// Keep the registration live until the caller finishes the entire request.
func (g *Gateway) agentSelection(r *http.Request, request *anthropic.Request, entry *record) ([]bridge.Route, func(), error) {
	var override []bridge.Route
	releaseAgent := func() {}
	scope := delegationScope{session: r.Header.Get("X-Claude-Code-Session-Id"), parent: r.Header.Get("X-Claude-Code-Parent-Agent-Id")}
	scope.workflow = r.Header.Get("X-Claude-Code-Request-Class") == "workflow"
	// A tool-less root title/classifier request is independent of the conversation
	// turn being published. It neither inherits a child selection nor owns an abort
	// receipt. Child, tool and conversation requests still require the same proof.
	if independentAuxiliary(r, request) {
		entry.nativeTurn, entry.nativeResult, entry.nativeResultTurn = nil, nil, ""
		return nil, releaseAgent, nil
	}
	active, ok := g.pinNativeTurn(r, entry)
	if !ok {
		return nil, releaseAgent, errDelegationUnverified
	}
	scope.nativeTurn = active
	if agent := r.Header.Get("X-Claude-Code-Agent-Id"); agent != "" {
		role, release, registered := g.agents.begin(agent)
		releaseAgent = release
		entry.agent(agent, scope.parent, role, false)
		if g.delegations != nil {
			binding := g.agents.bindingOf(agent)
			resolvedScope, continued := g.continuationScope(scope, agent, binding)
			route, found, err := g.delegations.route(resolvedScope, agent, binding, r.Context())
			if errors.Is(err, bridge.ErrRetiredRoute) {
				return nil, releaseAgent, err
			}
			if err != nil {
				return nil, releaseAgent, errDelegationUnverified
			}
			if found {
				// Preserve the proven origin even if this request contradicts the
				// selection or fails later admission. This does not mark any check
				// successful or authorize execution.
				entry.route(request.Model, route.Model, route.Effort, route.Source)
				observed, err := bridge.SelectRoute(request.Model, "")
				if errors.Is(err, bridge.ErrRetiredRoute) {
					return nil, releaseAgent, err
				}
				if err != nil || observed.Model != route.Model && !g.delegations.resumeModel(scope, agent, observed.Model) {
					return nil, releaseAgent, errDelegationUnverified
				}
				// Where native chose the route, every request must match native's receipt for
				// its current turn.
				nativeChosen := route.Source == "workflow-selection" || route.Source == "native-selection" || route.Source == "native-fork"
				if (strings.HasPrefix(route.Source, "workflow-") || nativeChosen) && (request.Effort != route.Effort || nativeChosen && (scope.nativeTurn == nil || scope.nativeTurn.Model != route.Model || scope.nativeTurn.Effort != route.Effort)) {
					return nil, releaseAgent, errDelegationUnverified
				}
				if strings.HasPrefix(route.Source, "workflow-") {
					entry.checked("workflow_selection")
				}
				if continued {
					route.Source = "verified-continuation"
					entry.continuation(resolvedScope.parent)
				}
				override = []bridge.Route{route}
				entry.agent(agent, scope.parent, role, true)
				entry.checked("selection")
				if continued || route.Source == "verified-resume" {
					entry.checked("continuation")
				}
			}
		}
		if len(override) == 0 {
			// Counted before the refusal rather than instead of it. The production launcher
			// enables the context policy unconditionally, so the branch below always returns
			// and these two counters could never move: the one diagnostic that says which
			// kind of unverified child produced an AGENT_SELECTION_UNVERIFIED was
			// structurally always zero. What happens to the request is unchanged.
			switch {
			case !registered:
				g.unregisteredAgents.Add(1)
			case !bridge.KnownRole(role):
				g.unroutedRoles.Add(1)
			}
			if g.contexts != nil {
				return nil, releaseAgent, errDelegationUnverified
			}
			if route, known := bridge.RoleRoute(role); known && registered {
				override = append(override, route)
			}
		}
	}

	return override, releaseAgent, nil
}

// correlationHeaders are the identifiers the client uses to tie a request to the session
// and the subagent that made it. Measured 2026-09-16: a real session sends the session id
// as a UUID and sends the other two only when a subagent is running.
var correlationHeaders = []string{
	"X-Claude-Code-Session-Id",
	"X-Claude-Code-Agent-Id",
	"X-Claude-Code-Parent-Agent-Id",
}

// correlationShape is what one of those identifiers may look like. The baseline's.
var correlationShape = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// anthropicVersion is the only version of the message API this build speaks.
//
// Measured: the installed client sends exactly this. Checking it is not ceremony -- a
// client speaking a version nobody here implements would otherwise be answered as though
// it had been understood.
const anthropicVersion = "2023-06-01"

// checkRequestHeaders applies the boundary checks the Node baseline makes and this build
// did not.
//
// Absence is not a failure for the correlation identifiers: CAP06 separates "no identifier"
// from "an identifier that is not one", because the first is an ordinary request and the
// second is a malformed one. A shape that is not the client's is refused rather than
// carried, since these values are what a later binding would be matched against.
func checkRequestHeaders(r *http.Request) (refusal, bool) {
	switch r.Header.Get("X-Claude-Code-Request-Class") {
	case "", "main", "subagent", "workflow", "auxiliary", "compaction":
	default:
		return refuseHeader, false
	}
	for _, name := range correlationHeaders {
		if value := r.Header.Get(name); value != "" && !correlationShape.MatchString(value) {
			return refuseSessionID, false
		}
	}
	if r.Header.Get("Anthropic-Version") != anthropicVersion {
		return refuseVersion, false
	}
	// Nothing here decompresses, so a body that arrives compressed is one this build would
	// read as gibberish. Measured: the client sends no Content-Encoding at all.
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return refuseEncoding, false
	}
	return refusal{}, true
}

// searchFor answers one side query from the backend's search endpoint.
//
// No model turn and no inference: one JSON round trip, then the blocks the client reduces.
// Nothing from the conversation travels with the query -- only the query does.
func (g *Gateway) searchFor(ctx context.Context, w http.ResponseWriter,
	control *http.ResponseController, request *anthropic.Request, query bridge.SearchQuery, override ...bridge.Route) {

	searcher, ok := g.transport.(upstream.Searcher)
	if !ok {
		// Saying so beats answering the query out of nothing. A reply with no results is
		// indistinguishable from a web that had nothing to say.
		g.refuseCategory(w, http.StatusNotImplemented, "SEARCH_UNSUPPORTED")
		return
	}

	route, err := bridge.ResolveRoute(request, override...)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, routeCategory(err))
		return
	}
	body, err := json.Marshal(bridge.BuildSearchRequest(route.Model, query))
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_ENCODE_FAILED")
		return
	}

	entry := recordOf(w)
	entry.route(request.Model, route.Model, route.Effort, route.Source)
	entry.checked("route")
	entry.checked("search_available")
	entry.at(stageUpstream)
	if err := entry.execution.dispatch(ctx); err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	raw, err := searcher.Search(ctx, body)
	entry.execution.rejectedBeforeDispatch(err)
	if err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	results, err := bridge.DecodeSearchResults(raw)
	if err != nil {
		g.refuseCategory(w, http.StatusBadGateway, err.Error())
		return
	}

	entry.at(stageDelivery)
	frames := bridge.SearchFrames(route.Model, query, results, bridge.SearchID)
	if request.NonStreaming {
		var message anthropic.ResponseMessage
		if err := message.Add(frames); err != nil {
			g.refuseCategory(w, 502, categoryFor(err))
			return
		}
		g.deliverMessage(ctx, w, control, &message)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	bounded := &chunkedWriter{to: w, control: control}
	defer func() { _ = control.SetWriteDeadline(time.Time{}) }()
	for _, frame := range frames {
		if _, err := frame.WriteTo(bounded); err != nil {
			g.deliveryFailed(ctx, w)
			return
		}
	}
	if err := control.Flush(); err != nil {
		g.deliveryFailed(ctx, w)
	}
}

// handleModels answers the client's model discovery.
//
// The client only asks when CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY is set, and until it
// is answered the user's /model list is the client's built-in Anthropic one: names that do
// not exist on this backend, two of which used to resolve to the same route, and one
// backend model -- terra -- that no entry could reach. Answering here is what lets the
// picker name what will actually run.
//
// Nothing about the account, the credential or the session appears in the reply. It is the
// build's own catalogue, which is a constant.
func (g *Gateway) handleModels(w http.ResponseWriter) {
	type entry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	routes := bridge.Catalogue()
	data := make([]entry, 0, len(routes))
	for _, route := range routes {
		data = append(data, entry{ID: route.Model, Object: "model", OwnedBy: "openai"})
	}
	body, err := json.Marshal(struct {
		Object string  `json:"object"`
		Data   []entry `json:"data"`
	}{Object: "list", Data: data})
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "MODEL_LIST_ENCODE_FAILED")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// chunkedWriter hands the response out in bounded pieces, refreshing the write deadline
// before each one.
//
// Per batch was not enough, and the WIRE13 measurements ran into why: when the backend
// delivers faster than the parser is drained, one batch can be megabytes, and a single
// write that large cannot finish inside any bound a live client deserves. The bound then
// refuses a client that was keeping up. Splitting the write is what makes "one write may
// block for writeStall" a statement about a bounded amount of data.
//
// The size is the Node baseline's: native-delivery.mjs writes 16 KiB at a time and awaits
// backpressure per chunk, against the same 30 s timeout. This arrived at the timeout
// independently and missed the chunking, which is the half that gives it its meaning.
type chunkedWriter struct {
	to      io.Writer
	control *http.ResponseController
}

func (c *chunkedWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		size := len(p)
		if size > writeChunk {
			size = writeChunk
		}
		_ = c.control.SetWriteDeadline(time.Now().Add(writeStall))
		n, err := c.to.Write(p[:size])
		written += n
		if err != nil {
			return written, err
		}
		p = p[size:]
	}
	return written, nil
}

// relay reads the backend stream and writes client frames as they are produced.
func (g *Gateway) relay(ctx context.Context, w http.ResponseWriter, control *http.ResponseController,
	response *upstream.Response, request *anthropic.Request, effective string, scopes ...delegationScope) bool {

	// Cleared when this response ends, and that is not tidiness.
	//
	// A deadline set through the ResponseController lives on the connection, not on the
	// request, and this server configures no WriteTimeout of its own, so nothing else
	// resets it. Left in place it is still counting down when the next request arrives on
	// the same keep-alive connection, and that request inherits whatever is left of a
	// bound it never earned.
	defer func() { _ = control.SetWriteDeadline(time.Time{}) }()

	parser := stream.NewParser(stream.DefaultLimits())
	parser.IsTerminal = codex.Terminal
	var lastReadErr error
	defer func() {
		// Parsing or translation can stop before EOF, including on a terminal
		// empty reply. Keep the observed read state on every exit, not just EOF.
		events, bytes := parser.Stats()
		recordOf(w).streamEnd(lastReadErr, ctx.Err(), parser.Completed(), events, bytes)
	}()
	// The translator is told which tools are callable now, so a call naming a withdrawn
	// tool is refused rather than passed to a client that would try to run it.
	translator := bridge.NewTranslatorFor(request, effective)
	if entry := recordOf(w); entry != nil {
		record := entry.snapshot()
		if len(scopes) > 0 && scopes[0].parentWait != nil {
			// SDK keeps real answers and emits only a verified empty-reply status;
			// its native scheduler owns background task completion.
			if len(record.ParentReadiness.Pending) > 0 && record.ParentReadiness.ControlMode == "native_tui" {
				translator.Builder().WaitForChildren()
			} else {
				translator.Builder().ConsumeEmptyNotification()
			}
		}
		// Native SDK/print returns the last assistant block. Keep opaque reasoning
		// before the final text there, as for Workflow; TUI text still streams.
		if record.RequestClass == "workflow" || record.ParentReadiness != nil && record.ParentReadiness.ControlMode == "sdk" {
			translator.Builder().DeferTextUntilComplete()
		}
	}
	var prepared []string
	relayCompleted := false
	if g.delegations != nil && len(scopes) > 0 && scopes[0].parent != "" {
		class := recordOf(w).snapshot().RequestClass
		if class != "compaction" && class != "auxiliary" {
			finishAnswer := g.delegations.beginAnswer(scopes[0].session, scopes[0].parent)
			defer func() {
				answer := ""
				if relayCompleted {
					answer = translator.Answer()
				}
				finishAnswer(answer, relayCompleted)
			}()
		}
	}
	defer func() {
		if !relayCompleted && g.delegations != nil && len(scopes) > 0 {
			g.delegations.discard(scopes[0].session, prepared)
		}
	}()
	if g.delegations != nil && len(scopes) > 0 {
		translator.PrepareToolCall = func(id, name string, raw json.RawMessage) (json.RawMessage, error) {
			if name == "Workflow" {
				var fields map[string]json.RawMessage
				if json.Unmarshal(raw, &fields) == nil {
					if _, requested := fields["resumeFromRunId"]; requested {
						recordOf(w).checked("workflow_recovery_request")
					}
				}
			}
			adapted, err := g.delegations.prepare(scopes[0], id, name, raw)
			if name == "Workflow" && (errors.Is(err, errWorkflowRecoveryUnverified) || errors.Is(err, errDelegationUnverified)) {
				adapted, err = g.delegations.rejectWorkflow(scopes[0], id, raw)
				if err == nil {
					recordOf(w).rejectedWorkflow()
				}
			}
			if err != nil && name == "Agent" {
				g.delegations.rejectedSelection(scopes[0], id, raw, err)
			}
			if err == nil && (name == "Agent" || name == "Workflow" || name == "SendMessage") {
				prepared = append(prepared, id)
			}
			if err == nil && name == "Workflow" {
				g.delegations.mu.Lock()
				origin := g.delegations.workflowCalls[delegationKey{scopes[0].session, id}]
				isRecovery := origin.recoveryOf != ""
				restored := isRecovery && g.delegations.workflows[delegationKey{scopes[0].session, origin.recoveryOf}].restored
				g.delegations.mu.Unlock()
				if isRecovery {
					recordOf(w).checked("workflow_recovery")
					if restored {
						recordOf(w).checked("workflow_checkpoint")
					}
					if origin.plan != nil {
						recordOf(w).checked("workflow_plan_resume")
					}
				}
			}
			return adapted, err
		}
	}

	committed := false
	var message anthropic.ResponseMessage
	emit := func(frames []anthropic.Frame) error {
		if request.NonStreaming {
			return message.Add(frames)
		}
		if len(frames) == 0 {
			return nil
		}
		if !committed {
			committed = true
			header := w.Header()
			header.Set("Content-Type", "text/event-stream")
			header.Set("Cache-Control", "no-cache")
			header.Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
		}
		// Bounded per chunk, not per batch and certainly not per response. A global
		// WriteTimeout would end a long answer that is being delivered perfectly well;
		// this bounds how long one write may block, which is a different thing. A client
		// that keeps reading resets it constantly and never meets it.
		//
		// Without it a client that stops reading blocks the write once the socket buffer
		// fills, and holds a goroutine, the upstream connection and a request that is still
		// running on the user's subscription -- for as long as it likes.
		bounded := &chunkedWriter{to: w, control: control}
		for _, frame := range frames {
			if _, err := frame.WriteTo(bounded); err != nil {
				return err
			}
		}
		// Flushed per batch. Without this the client sees nothing until the handler
		// returns, which turns a streaming response into a slow non-streaming one.
		return control.Flush()
	}

	fail := func(err error) {
		// An event this build could not read is the one failure whose cause is a name, and
		// the name is the whole fix. Recorded here because this is where every stream
		// failure funnels, so no path can stop a response without the account knowing.
		var unsupported *bridge.UnsupportedEvent
		if errors.As(err, &unsupported) {
			g.events.observe(unsupported)
		}
		// Its own deadline: the path that got here may be the write that just stalled, and
		// an error frame must not inherit a deadline that has already passed.
		_ = control.SetWriteDeadline(time.Now().Add(writeStall))
		detail := recordOf(w).upstreamFailure(err)
		if !committed {
			if errors.Is(err, codex.ErrContextLimit) && len(scopes) > 0 && g.recoverContextOverflow(w, scopes[0].session, scopes[0].parent, effective) {
				return
			}
			g.refuseDetail(w, refusal{categoryFor(err), statusForUpstream(err)}, detail)
			return
		}
		// The status is already sent. A terminal error event is the only signal left, and
		// leaving the stream to simply stop would look to the client like a short answer
		// rather than a failure.
		//
		// And the account is told, which it was not before: a 200 already written meant
		// nothing marked the record, so a stream that broke halfway was filed as a clean
		// success and the session reported nothing wrong.
		g.streamBroke(w, categoryFor(err))
		_, _ = anthropic.ErrorFrame(refusalMessage(categoryFor(err)) + detail).WriteTo(w)
		_ = control.Flush()
	}

	buffer := make([]byte, readChunk)
	for {
		n, readErr := response.Body.Read(buffer)
		lastReadErr = readErr
		if n > 0 {
			events, err := parser.Push(buffer[:n])
			if err != nil {
				fail(err)
				return false
			}
			for _, event := range events {
				recordOf(w).backendEvent(event.Type)
				frames, err := translator.Accept(event)
				if err != nil {
					fail(err)
					return false
				}
				recordOf(w).usage(translator.ObservedUsage())
				// A count-source mismatch quarantines that optional counter, not
				// a valid model response. The provider usage remains authoritative.
				_ = g.verifyCount(recordOf(w))
				if translator.Builder().WaitingForChildren() {
					if err := g.writeParentDecision(scopes[0].parentWait, true); err != nil {
						fail(errParentWaitUnverified)
						return false
					}
					entry := recordOf(w)
					entry.mu.Lock()
					if entry.data.ParentReadiness != nil {
						snapshot := *entry.data.ParentReadiness
						snapshot.Withheld = true
						snapshot.Empty = translator.Builder().ReplyEmpty()
						entry.data.ParentReadiness = &snapshot
					}
					entry.mu.Unlock()
				}
				if err := emit(frames); err != nil {
					if request.NonStreaming {
						fail(err)
					} else {
						g.deliveryFailed(ctx, w)
					}
					return false
				}
			}
		}
		if readErr != nil {
			// A transport read interrupted by the client's cancelled context is
			// cancellation evidence. An upstream reset/deadline alone is not.
			if errors.Is(ctx.Err(), context.Canceled) {
				g.deliveryFailed(ctx, w)
				return false
			}
			// A read error that is not EOF means the body did not arrive whole, and the
			// parser is told so rather than being asked to judge well-formed framing.
			if err := parser.Finish(errors.Is(readErr, io.EOF)); err != nil {
				fail(err)
				return false
			}
			if request.NonStreaming {
				committed = g.deliverMessage(ctx, w, control, &message)
			}
			if !committed && !request.NonStreaming {
				// Well formed, terminal, and it produced nothing to send. The client
				// still needs a message, which Complete would have emitted — reaching
				// here means the backend ended without one.
				g.refuseCategory(w, http.StatusBadGateway, "EMPTY_UPSTREAM_RESPONSE")
			}
			relayCompleted = committed
			return committed
		}
		if ctx.Err() != nil {
			g.deliveryFailed(ctx, w)
			return false
		}
	}
}

func (g *Gateway) deliverMessage(ctx context.Context, w http.ResponseWriter, control *http.ResponseController, message *anthropic.ResponseMessage) bool {
	defer func() { _ = control.SetWriteDeadline(time.Time{}) }()
	raw, err := message.JSON()
	if err != nil {
		g.refuseCategory(w, 502, categoryFor(err))
		return false
	}
	if ctx.Err() != nil {
		g.deliveryFailed(ctx, w)
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(http.StatusOK)
	if _, err = (&chunkedWriter{to: w, control: control}).Write(raw); err != nil {
		g.deliveryFailed(ctx, w)
		return false
	}
	if err = control.Flush(); err != nil {
		g.deliveryFailed(ctx, w)
		return false
	}
	return true
}

// categoryFor maps an error to the fixed category a client sees. Nothing from a backend
// body reaches this: every arm returns a constant chosen here.
func categoryFor(err error) string {
	switch {
	case errors.Is(err, codex.ErrContextLimit):
		return "CONTEXT_LENGTH_EXCEEDED"
	case errors.Is(err, bridge.ErrUnsupportedRoute):
		return routeCategory(err)
	case errors.Is(err, errDelegationUnverified):
		return "AGENT_SELECTION_UNVERIFIED"
	case errors.Is(err, errParentWaitUnverified):
		return "PARENT_WAIT_UNVERIFIED"
	case errors.Is(err, errWorkflowRecoveryUnverified):
		return "WORKFLOW_RECOVERY_UNVERIFIED"
	case errors.Is(err, upstream.ErrNoTransport):
		return "NO_UPSTREAM_TRANSPORT"
	case errors.Is(err, upstream.ErrBudgetExhausted):
		return "REQUEST_BUDGET"
	case errors.Is(err, upstream.ErrRouteNotAuthorised):
		return "ROUTE_NOT_AUTHORISED"
	case errors.Is(err, codex.ErrResponseFailed):
		return "UPSTREAM_RESPONSE_FAILED"
	case errors.Is(err, codex.ErrResponseIncomplt):
		return "UPSTREAM_RESPONSE_INCOMPLETE"
	case errors.Is(err, codex.ErrErrorEvent):
		return "UPSTREAM_ERROR_EVENT"
	case errors.Is(err, codex.ErrEventShape):
		return "UPSTREAM_EVENT_SHAPE"
	case errors.Is(err, bridge.ErrUnsupportedEvent):
		return "UNSUPPORTED_EVENT"
	case errors.Is(err, anthropic.ErrTextMismatch):
		return "TEXT_MISMATCH"
	case errors.Is(err, anthropic.ErrStreamOrder):
		return "STREAM_ORDER"
	case errors.Is(err, anthropic.ErrUnsupportedToolCall):
		return "UNSUPPORTED_TOOL_CALL"
	case errors.Is(err, anthropic.ErrInvalidToolCall):
		return "INVALID_TOOL_CALL"
	case errors.Is(err, anthropic.ErrEmptyReply):
		return "EMPTY_REPLY"
	case errors.Is(err, bridge.ErrOutputLimitExceeded):
		return "OUTPUT_TOKEN_LIMIT_EXCEEDED"
	case errors.Is(err, bridge.ErrUsageUnknown):
		return "INVALID_USAGE"
	// Contract faults found while translating the backend's reply. Unnamed, they fell
	// through to UPSTREAM_FAILURE and read as a backend outage.
	case errors.Is(err, bridge.ErrArgumentsMismatch):
		return "ARGUMENTS_MISMATCH"
	case errors.Is(err, bridge.ErrOutputItemOrder):
		return "INVALID_OUTPUT_ITEM"
	case errors.Is(err, bridge.ErrItemSnapshotMismatch):
		return "SNAPSHOT_MISMATCH"
	case errors.Is(err, bridge.ErrMissingEncryptedReasoning):
		return "MISSING_ENCRYPTED_REASONING"
	case errors.Is(err, anthropic.ErrResponseTooLarge), errors.Is(err, stream.ErrResponseTooLarge):
		return "RESPONSE_TOO_LARGE"
	case errors.Is(err, stream.ErrInvalidSSE):
		return "INVALID_SSE"
	case errors.Is(err, stream.ErrInvalidUTF8):
		return "INVALID_UTF8"
	case errors.Is(err, stream.ErrFrameTooLarge):
		return "FRAME_TOO_LARGE"
	case errors.Is(err, stream.ErrTooManyEvents):
		return "TOO_MANY_EVENTS"
	case errors.Is(err, stream.ErrTruncatedStream):
		return "TRUNCATED_STREAM"
	case errors.Is(err, stream.ErrIncompleteResponse):
		return "INCOMPLETE_RESPONSE"
	case errors.Is(err, stream.ErrEventAfterCompletion):
		return "EVENT_AFTER_COMPLETION"
	case errors.Is(err, stream.ErrSequenceMismatch):
		return "SEQUENCE_MISMATCH"
	case errors.Is(err, context.DeadlineExceeded):
		return "REQUEST_TIMEOUT"
	case errors.Is(err, context.Canceled):
		return "CANCELLED"
	}
	// The real transport's own categories. Each is a constant chosen in this project, never
	// a string from a backend body, so passing one through carries nothing out with it.
	if category := auth.CategoryOf(err); category != "" {
		return category
	}
	var failure upstream.Failure
	if errors.As(err, &failure) && failure.Category != "" {
		return failure.Category
	}
	return "UPSTREAM_FAILURE"
}

// statusForUpstream picks the status class, and the class is a retry instruction as much
// as a blame assignment.
//
// Measured against claude 2.1.272: a 4xx ends the turn after two attempts, while 501, 502
// and 503 are all retried — eight requests in sixty seconds and still going. So the class
// has to follow whether retrying could ever help, not only whose fault the failure was.
//
// No transport configured is permanent for the life of the process. Answering it with any
// 5xx leaves the client backing off against a condition that will never change, so it is
// reported in the class that stops. The category says what actually happened; a reader who
// needs the cause reads that rather than the number.
//
// Upstream 502/429/503 retain their failure class. These are not permission to
// execute twice: the native ledger refuses replay after dispatch, even if the
// client would otherwise back off and retry. A new explicit turn is separate.
func statusForUpstream(err error) int {
	switch {
	case errors.Is(err, bridge.ErrUnsupportedRoute):
		return http.StatusBadRequest
	case errors.Is(err, errDelegationUnverified), errors.Is(err, errWorkflowRecoveryUnverified), errors.Is(err, errParentWaitUnverified):
		return http.StatusBadRequest
	case errors.Is(err, upstream.ErrNoTransport), errors.Is(err, upstream.ErrBudgetExhausted), errors.Is(err, upstream.ErrRouteNotAuthorised):
		return http.StatusBadRequest
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return refuseCancelled.status
	}

	// A credential problem is answered the way the Node baseline answers it
	// (src/native-gateway.mjs:557 at 1b1c5e1): 503, so a client that keeps asking recovers the moment
	// the user logs in again. Retrying costs nothing upstream -- the failure happens before
	// the socket -- and the category in the message says what to fix.
	switch auth.CategoryOf(err) {
	case auth.CategoryUnavailable, auth.CategoryTokenExpired, auth.CategoryAccountChanged,
		auth.CategoryInvalidCache, auth.CategoryFileUnavailable:
		return http.StatusServiceUnavailable
	case "":
	default:
		// A runtime or store refusal is about this machine's configuration and will not
		// change by asking again.
		return http.StatusBadRequest
	}

	var failure upstream.Failure
	if errors.As(err, &failure) {
		// A rate limit gets the status that makes a client wait, whether or not the server
		// named a delay. Reading the delay is what decides Deferred, so a 429 that arrives
		// without a parsable Retry-After stays Retryable -- and answering that with 502
		// hands the client the one class it was measured to retry eight times in sixty
		// seconds, against an account that has just said it is out of room.
		if failure.Category == upstream.RateLimited {
			return http.StatusTooManyRequests
		}
		switch failure.Disposition {
		case upstream.Deferred:
			// The server named a time. 429 is the one status this client backs off from
			// properly rather than hammering.
			return http.StatusTooManyRequests
		case upstream.Retryable:
			return http.StatusBadGateway
		default:
			// Measured against claude 2.1.272: every 5xx is retried, eight requests in
			// sixty seconds and still going. Answering a terminal failure with 502 buys the
			// same refusal eight times on the user's subscription. A 4xx ends the turn.
			//
			// This is a deliberate departure from the baseline, which answers 502 for every
			// upstream failure. The measurement is the reason, and the category in the
			// message still says exactly what happened.
			return http.StatusBadRequest
		}
	}

	return http.StatusBadGateway
}

// betaToolChanges is the beta that carries mid-conversation tool changes.
const betaToolChanges = "mid-conversation-tool-changes-2026-07-01"

// negotiated reports whether the request's anthropic-beta header named a feature.
//
// Presence only. A beta name is never refused here and that is measured rather than
// chosen: the Node baseline records refusing one breaking WebFetch in a real session,
// because refusing a header fails the whole request while the feature it names is already
// inert against this backend. So the header is read for what it enables and for nothing
// else.
func negotiated(r *http.Request, feature string) bool {
	for _, header := range r.Header.Values("Anthropic-Beta") {
		for _, name := range strings.Split(header, ",") {
			if strings.TrimSpace(name) == feature {
				return true
			}
		}
	}
	return false
}
