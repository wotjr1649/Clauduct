package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

func (g *Gateway) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.refuse(w, refuseMethod)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	if bad, ok := checkRequestHeaders(r); !ok {
		g.refuse(w, bad)
		return
	}
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
	control := http.NewResponseController(w)
	_ = control.SetReadDeadline(time.Now().Add(requestBodyTimeout))
	stopReadCancellation := watchReadCancellation(ctx, func() { _ = control.SetReadDeadline(time.Now()) })
	defer stopReadCancellation()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		if ctx.Err() != nil {
			g.refuse(w, refuseCancelled)
			return
		}
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	// Count requests omit generation-only fields. Decode through the same strict
	// message parser after adding local structural defaults; nothing is generated.
	fields, err := wire.Fields(body, []string{"model", "messages", "system", "tools", "thinking", "metadata", "output_config", "context_management"})
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	fields["stream"] = json.RawMessage("true")
	fields["max_tokens"] = json.RawMessage("1")
	encoded, err := json.Marshal(fields)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	request, err := anthropic.DecodeRequest(encoded, anthropic.Options{ToolChanges: negotiated(r, betaToolChanges)})
	if err != nil || request.HostedSearch != nil {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	entry := recordOf(w)
	entry.checked("input")
	g.stripContextDisplays(request, r.Header.Get("X-Claude-Code-Session-Id"))
	if g.delegations != nil {
		g.delegations.restoreSelectionHistory(request, r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"))
	}
	override, releaseAgent, err := g.agentSelection(r, request, entry)
	defer releaseAgent()
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "AGENT_SELECTION_UNVERIFIED")
		return
	}
	if g.delegations != nil && !g.delegations.restrictWorkflowTools(request, r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"), entry) {
		g.refuseCategory(w, 400, "WORKFLOW_TOOL_POLICY")
		return
	}
	built, err := bridge.BuildRequest(request, override...)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	if g.delegations != nil {
		g.delegations.describeWorkflowStep(built, r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"))
		if err := g.delegations.describe(built.Tools, r.Header.Get("X-Claude-Code-Agent-Id")); err != nil {
			g.refuseCategory(w, http.StatusBadRequest, "AGENT_SELECTION_UNVERIFIED")
			return
		}
	}
	entry.at(stagePrepare)
	if err := g.prepareDocuments(ctx, built); err != nil {
		g.refuseCategory(w, 400, "COUNT_TOKENS_FAILED_"+err.Error())
		return
	}
	raw, err := json.Marshal(built)
	if err != nil {
		g.refuseCategory(w, 400, "COUNT_TOKENS_FAILED_ENCODE")
		return
	}
	started := time.Now()
	entry.route(request.Model, built.Model, built.Effort.Effort, built.Source)
	entry.checked("route")
	entry.at(stageCount)
	tokens, source, method, err := g.countInput(ctx, built, raw, request.Model)
	entry.preflight(tokens, source, time.Since(started).Milliseconds())
	entry.countMethod(method)
	if err != nil {
		if ctx.Err() != nil {
			g.refuse(w, refuseCancelled)
			return
		}
		if errors.Is(err, bridge.ErrTokenCountUnsupported) || errors.Is(err, upstream.ErrCountUnsupported) {
			g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		} else {
			// This request alone fails; the client owns its fallback. No hidden
			// retry or generated probe is issued by the gateway.
			g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_FAILED_"+categoryFor(err))
		}
		return
	}
	entry.at(stageDelivery)
	entry.route(request.Model, built.Model, built.Effort.Effort, source)
	entry.counted(tokens)
	entry.checked("count_completed")
	reply, _ := json.Marshal(struct {
		InputTokens int64 `json:"input_tokens"`
	}{tokens})
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(reply); err != nil {
		g.deliveryFailed(r.Context(), w)
	}
}
