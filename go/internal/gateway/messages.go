package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/auth"
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
	_ = control.SetReadDeadline(time.Now().Add(requestBodyTimeout))
	watcherDone := make(chan struct{})
	defer close(watcherDone)
	go abortOnCancel(ctx, watcherDone, func() { _ = control.SetReadDeadline(time.Now()) })

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			g.refuse(w, refuseTooLarge)
		case ctx.Err() != nil:
			// The client went away. Nothing will read this, but the status is recorded
			// so a cancelled request is counted as cancelled rather than as a success.
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

	// The client's search side query is a server tool call addressed to this gateway, not a
	// model request. Answered before conversion so the model path stays untouched.
	if request.HostedSearch != nil {
		query, ok := bridge.SideQuery(request)
		if !ok {
			// The tool is here but the request is not the side query the client sends. The
			// Node baseline drops the tool and answers from the model without search
			// results; this refuses instead. A WebSearch that quietly returns nothing is
			// worse than one that says it broke, because only the second gets fixed.
			g.refuseCategory(w, http.StatusBadRequest, anthropic.CodeHostedToolUnsupp)
			return
		}
		g.searchFor(ctx, w, control, request, query)
		return
	}

	// A subagent's role decides where it runs, whatever model the client asked for. The
	// registration comes from the client's own hook, so this is the client telling us what
	// it started rather than this build inferring it.
	//
	// Every way this can fail leaves the client's own choice in place. A header that is
	// absent, a registration that has not arrived yet, a role nobody has a route for: none
	// of them is a reason to end a turn. The baseline refuses the request in some of these
	// cases, and that is defensible there because it verifies the subagent's identity
	// against the client's own metadata first. Without that verification the same refusal
	// would only add a way to fail.
	// Classified, never refused -- see betas.go. Observed after the body is decoded so the
	// header of a request that never became one is not counted as a feature the session
	// asked for.
	g.betas.observe(r.Header.Get("Anthropic-Beta"))

	entry := recordOf(w)
	entry.at(stageSelection)

	var override []bridge.Route
	if agent := r.Header.Get("X-Claude-Code-Agent-Id"); agent != "" {
		role, release, registered := g.agents.begin(agent)
		defer release()
		switch {
		case !registered:
			g.unregisteredAgents.Add(1)
		default:
			if route, known := bridge.RoleRoute(role); known {
				override = append(override, route)
			} else {
				g.unroutedRoles.Add(1)
			}
		}
	}

	entry.at(stagePrepare)
	backendRequest, err := bridge.BuildRequest(request, override...)
	if err != nil {
		// CAP06: a model this build cannot route is the caller's answerable problem, not
		// an internal failure. Reporting it as a 500 was wrong twice over -- it told the
		// user nothing they could act on, and the measured client retries every 5xx, so a
		// request that can never succeed was sent eight times in a minute.
		if errors.Is(err, bridge.ErrUnsupportedRoute) {
			g.refuseCategory(w, http.StatusBadRequest, "UNSUPPORTED_MODEL_OR_EFFORT")
			return
		}
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_CONVERSION_FAILED")
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
	entry.at(stageUpstream)
	response, err := g.transport.Execute(ctx, upstream.Call{
		Body:      encoded,
		Requested: request.Model,
		Model:     backendRequest.Model,
		Effort:    backendRequest.Effort.Effort,
		Source:    backendRequest.Source,
	})
	if err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	defer response.Body.Close()

	// What the backend volunteered about the quota. Read, never acted on.
	g.limits.observe(response.Header)

	entry.at(stageDelivery)
	g.relay(ctx, w, control, response, request)
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
	control *http.ResponseController, request *anthropic.Request, query bridge.SearchQuery) {

	searcher, ok := g.transport.(upstream.Searcher)
	if !ok {
		// Saying so beats answering the query out of nothing. A reply with no results is
		// indistinguishable from a web that had nothing to say.
		g.refuseCategory(w, http.StatusNotImplemented, "SEARCH_UNSUPPORTED")
		return
	}

	route, err := bridge.SelectRoute(request.Model, request.Effort)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "UNSUPPORTED_MODEL_OR_EFFORT")
		return
	}
	body, err := json.Marshal(bridge.BuildSearchRequest(route.Model, query))
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_ENCODE_FAILED")
		return
	}

	raw, err := searcher.Search(ctx, body)
	if err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	results, err := bridge.DecodeSearchResults(raw)
	if err != nil {
		g.refuseCategory(w, http.StatusBadGateway, err.Error())
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	bounded := &chunkedWriter{to: w, control: control}
	defer func() { _ = control.SetWriteDeadline(time.Time{}) }()
	for _, frame := range bridge.SearchFrames(route.Model, query, results, bridge.SearchID) {
		if _, err := frame.WriteTo(bounded); err != nil {
			return
		}
	}
	_ = control.Flush()
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

// abortOnCancel expires the read deadline when the client goes away while the handler is
// still reading, and never after the handler is done.
//
// The re-check is the whole point. By the time this goroutine first runs, both channels
// are usually closed already: done by the handler's defer, ctx by net/http the instant the
// handler returns. A plain two-case select picks between closed channels at random, so
// half the time it expired the deadline on a connection the handler no longer owned. The
// response was still sitting in the server's write buffer at that moment, and the expired
// deadline tore the connection down before the flush, so the client saw a reset instead of
// its reply. That was a real intermittent failure in this suite, on fast requests only.
//
// done is closed by a defer, which runs strictly before net/http cancels ctx, so checking
// it again here is exact rather than another guess.
func abortOnCancel(ctx context.Context, done <-chan struct{}, expire func()) {
	select {
	case <-ctx.Done():
		select {
		case <-done:
		default:
			expire()
		}
	case <-done:
	}
}

// relay reads the backend stream and writes client frames as they are produced.
func (g *Gateway) relay(ctx context.Context, w http.ResponseWriter, control *http.ResponseController,
	response *upstream.Response, request *anthropic.Request) {

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
	// The translator is told which tools are callable now, so a call naming a withdrawn
	// tool is refused rather than passed to a client that would try to run it.
	translator := bridge.NewTranslatorFor(request)

	committed := false
	emit := func(frames []anthropic.Frame) error {
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
		if !committed {
			g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
			return
		}
		// The status is already sent. A terminal error event is the only signal left, and
		// leaving the stream to simply stop would look to the client like a short answer
		// rather than a failure.
		_, _ = anthropic.ErrorFrame(categoryFor(err)).WriteTo(w)
		_ = control.Flush()
	}

	buffer := make([]byte, readChunk)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			events, err := parser.Push(buffer[:n])
			if err != nil {
				fail(err)
				return
			}
			for _, event := range events {
				frames, err := translator.Accept(event)
				if err != nil {
					fail(err)
					return
				}
				if err := emit(frames); err != nil {
					// The client stopped reading. Nothing more can be delivered and
					// there is nobody left to tell.
					return
				}
			}
		}
		if readErr != nil {
			// A read error that is not EOF means the body did not arrive whole, and the
			// parser is told so rather than being asked to judge well-formed framing.
			if err := parser.Finish(errors.Is(readErr, io.EOF)); err != nil {
				fail(err)
			} else if !committed {
				// Well formed, terminal, and it produced nothing to send. The client
				// still needs a message, which Complete would have emitted — reaching
				// here means the backend ended without one.
				g.refuseCategory(w, http.StatusBadGateway, "EMPTY_UPSTREAM_RESPONSE")
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// categoryFor maps an error to the fixed category a client sees. Nothing from a backend
// body reaches this: every arm returns a constant chosen here.
func categoryFor(err error) string {
	switch {
	case errors.Is(err, upstream.ErrNoTransport):
		return "NO_UPSTREAM_TRANSPORT"
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
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
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
// A genuine upstream failure stays 502. Retrying one can succeed, and whether the client's
// retries and this bridge's should both exist is a question the real transport has to
// settle rather than one to pre-empt here.
func statusForUpstream(err error) int {
	switch {
	case errors.Is(err, upstream.ErrNoTransport):
		return http.StatusBadRequest
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return refuseCancelled.status
	}

	// A credential problem is answered the way the Node baseline answers it
	// (src/native-gateway.mjs:557): 503, so a client that keeps asking recovers the moment
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

	// A rate limit with no parsable delay still deserves the status that makes a client
	// wait rather than the one that makes it hurry.
	if failure.Category == "RATE_LIMITED" {
		return http.StatusTooManyRequests
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
