package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

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
	go func() {
		select {
		case <-ctx.Done():
			_ = control.SetReadDeadline(time.Now())
		case <-watcherDone:
		}
	}()

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

	request, err := anthropic.DecodeRequest(body)
	if err != nil {
		var refusal *anthropic.RequestError
		if errors.As(err, &refusal) {
			g.refuseCategory(w, http.StatusBadRequest, refusal.Code)
			return
		}
		g.refuse(w, refuseHeader)
		return
	}

	backendRequest, err := bridge.BuildRequest(request)
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_CONVERSION_FAILED")
		return
	}
	encoded, err := json.Marshal(backendRequest)
	if err != nil {
		g.refuseCategory(w, http.StatusInternalServerError, "REQUEST_ENCODE_FAILED")
		return
	}

	response, err := g.transport.Execute(ctx, encoded)
	if err != nil {
		g.refuseCategory(w, statusForUpstream(err), categoryFor(err))
		return
	}
	defer response.Body.Close()

	g.relay(ctx, w, control, response, request.Model)
}

// relay reads the backend stream and writes client frames as they are produced.
func (g *Gateway) relay(ctx context.Context, w http.ResponseWriter, control *http.ResponseController,
	response *upstream.Response, model string) {

	parser := stream.NewParser(stream.DefaultLimits())
	parser.IsTerminal = codex.Terminal
	translator := bridge.NewTranslator(model)

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
		for _, frame := range frames {
			if _, err := frame.WriteTo(w); err != nil {
				return err
			}
		}
		// Flushed per batch. Without this the client sees nothing until the handler
		// returns, which turns a streaming response into a slow non-streaming one.
		return control.Flush()
	}

	fail := func(err error) {
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
	return "UPSTREAM_FAILURE"
}

// statusForUpstream follows the baseline's mapping so a client cannot tell the two
// implementations apart by status alone.
func statusForUpstream(err error) int {
	switch {
	case errors.Is(err, upstream.ErrNoTransport):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return refuseCancelled.status
	}
	return http.StatusBadGateway
}
