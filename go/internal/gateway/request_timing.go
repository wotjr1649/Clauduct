package gateway

import (
	"context"
	"errors"
	"net/http/httptrace"
	"time"
)

// UpstreamEndRecord describes Execute returning, before any stream or cleanup.
// A cancelled HTTP client is evidence of a disconnect, not of the user's intent.
// No error text, address, header or request content is retained.
type UpstreamEndRecord struct {
	Error          string `json:"error"`
	ClientContext  string `json:"clientContext"`
	RequestContext string `json:"requestContext"`
	GatewayClosing bool   `json:"gatewayClosing"`
}

func (r *record) traceUpstream(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	r.upstreamMoment(&r.data.UpstreamStartedMs)
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) { r.upstreamMoment(&r.data.UpstreamConnectedMs) },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				r.upstreamMoment(&r.data.UpstreamWrittenMs)
			}
		},
		// This is the first response HEADER byte, not the first SSE event.
		GotFirstResponseByte: func() { r.upstreamMoment(&r.data.UpstreamHeaderMs) },
	})
}

func (r *record) upstreamMoment(field **int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// net/http may finish a trace callback after a failed RoundTrip returned.
	if *field == nil && r.data.UpstreamReturnedMs == nil && r.data.EndedMs == nil {
		now := time.Since(r.epoch).Milliseconds()
		*field = &now
	}
}

func (r *record) upstreamReturned(err error, request, client context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Since(r.epoch).Milliseconds()
	r.data.UpstreamReturnedMs = &now
	r.data.UpstreamEnd = &UpstreamEndRecord{
		Error: streamErrorLabel(err), ClientContext: streamErrorLabel(client.Err()),
		RequestContext: streamErrorLabel(request.Err()),
		GatewayClosing: errors.Is(context.Cause(request), errShuttingDown),
	}
}
