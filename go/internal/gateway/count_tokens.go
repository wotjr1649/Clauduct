package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/httpguard"
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
		g.refuseHeaders(w, r, bad)
		return
	}
	ctx, release, admitted := g.admitRequest(w, r, maxRequestBytes, modelAdmission)
	if !admitted {
		return
	}
	defer release()
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
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
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
		g.noteUnknown("COUNT_TOKENS_UNSUPPORTED", err)
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
		g.noteUnknown("COUNT_TOKENS_UNSUPPORTED", err)
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	entry := recordOf(w)
	entry.checked("input")
	entry.requestClass(r.Header.Get("X-Claude-Code-Request-Class"))
	g.stripContextDisplays(request, r.Header.Get("X-Claude-Code-Session-Id"))
	if g.delegations != nil {
		g.delegations.restoreSelectionHistory(request, r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"))
	}
	override, releaseAgent, err := g.agentSelection(r, request, entry)
	defer releaseAgent()
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, selectionCategory(err))
		return
	}
	if r.Header.Get("X-Claude-Code-Request-Class") == "auxiliary" {
		var finishCancellation func()
		ctx, finishCancellation = g.bindNativeCancellation(ctx, r, entry, false)
		defer finishCancellation()
	}
	if g.delegations != nil && !g.delegations.restrictWorkflowTools(request, r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id"), entry) {
		g.refuseCategory(w, 400, "WORKFLOW_TOOL_POLICY")
		return
	}
	override, compact, failure := g.previewCompaction(r, request, override)
	if failure != "" {
		g.refuseCategory(w, http.StatusBadRequest, failure)
		return
	}
	built, err := g.selection.BuildRequest(request, override...)
	if err != nil {
		g.refuseCategory(w, http.StatusBadRequest, "COUNT_TOKENS_UNSUPPORTED")
		return
	}
	if g.delegations != nil {
		if err := g.delegations.describe(built.Tools, r.Header.Get("X-Claude-Code-Agent-Id")); err != nil {
			g.refuseCategory(w, http.StatusBadRequest, "AGENT_SELECTION_UNVERIFIED")
			return
		}
	}
	if compact {
		addCompactGuidance(built)
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
