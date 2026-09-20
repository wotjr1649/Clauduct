package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

type nativeEventState struct {
	mu            sync.Mutex
	directory     string
	verified      bool
	invalid       int64
	cancellations map[*nativeCancellation]struct{}
}
type NativeEventReport struct {
	Configured bool  `json:"configured"`
	Observed   bool  `json:"observed"`
	Invalid    int64 `json:"invalid"`
}
type nativeTurnReceipt struct {
	Session string `json:"session"`
	Agent   string `json:"agent"`
	Turn    string `json:"turn"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Set once before spawning native. The generated plugin writes only bounded
// receipts into this private per-process directory; no prompt/answer is copied.
func (g *Gateway) ConfigureNativeEvents(directory string) {
	g.nativeEvents.directory = directory
	if g.delegations != nil {
		g.delegations.events = directory
	}
}

func (g *Gateway) readNativeReceipt(name string, out any) (found bool, err error) {
	found, err = g.readNativeJSON(name, []string{"session", "agent", "turn", "model", "effort", "reason"}, out)
	if err != nil {
		g.nativeEvents.mu.Lock()
		g.nativeEvents.invalid++
		g.nativeEvents.mu.Unlock()
	}
	return
}

func (g *Gateway) readNativeJSON(name string, fields []string, out any) (found bool, err error) {
	n := &g.nativeEvents
	if n.directory == "" {
		return false, nil
	}
	root, err := os.OpenRoot(n.directory)
	if err != nil {
		return false, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2048 {
		return false, errDelegationUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, 2049))
	if err != nil || len(raw) > 2048 {
		return false, errDelegationUnverified
	}
	if _, err := wire.Fields(raw, fields); err != nil {
		return false, err
	}
	err = json.Unmarshal(raw, out)
	return err == nil, err
}

func (g *Gateway) nativeEventReport() NativeEventReport {
	n := &g.nativeEvents
	n.mu.Lock()
	defer n.mu.Unlock()
	return NativeEventReport{Configured: n.directory != "", Observed: n.verified, Invalid: n.invalid}
}

// Bind the current native turn before inference, not by time proximity. A
// resumed agent's previous end receipt cannot terminate this new execution.
func (g *Gateway) bindNativeTurn(session, id string) bool {
	if g.delegations == nil || id == "" || !correlationShape.MatchString(id) {
		return true
	}
	var receipt nativeTurnReceipt
	found, err := g.readNativeReceipt("active-"+id+".json", &receipt)
	if err != nil {
		return false
	}
	if !found {
		return g.nativeEvents.directory == ""
	}
	if !validActiveReceipt(receipt, session, id) {
		return false
	}
	r := &g.delegations.results
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil {
		if e.NativeTurn != "" && e.NativeTurn != receipt.Turn {
			return false
		}
		e.NativeTurn = receipt.Turn
		e.NativeModel = receipt.Model
		e.NativeEffort = receipt.Effort
	}
	g.nativeEvents.mu.Lock()
	g.nativeEvents.verified = true
	g.nativeEvents.mu.Unlock()
	return true
}

func validActiveReceipt(receipt nativeTurnReceipt, session, id string) bool {
	modelKnown := receipt.Model == "unlisted"
	for _, model := range bridge.Models {
		modelKnown = modelKnown || receipt.Model == model.ID
	}
	effortKnown := false
	for _, effort := range []string{"unlisted", "low", "medium", "high", "xhigh", "max"} {
		effortKnown = effortKnown || receipt.Effort == effort
	}
	if receipt.Session != session || receipt.Agent != id || !correlationShape.MatchString(receipt.Turn) || !modelKnown || !effortKnown || receipt.Reason != "" {
		return false
	}
	return true
}

// A selection refusal may occur before normal result binding. Attach its fixed
// category to the actual native turn only when independent identity still agrees.
// This records a failure; it never grants selection or starts/retries inference.
func (g *Gateway) recordFailedAgentRequest(session, id string, record *record) {
	if record == nil || g.delegations == nil || g.agents == nil || !correlationShape.MatchString(id) {
		return
	}
	record.mu.Lock()
	category := record.data.Category
	record.mu.Unlock()
	if category == "" || category == "CANCELLED" {
		return
	}
	var active nativeTurnReceipt
	found, err := g.readNativeReceipt("active-"+id+".json", &active)
	if !found || err != nil || !validActiveReceipt(active, session, id) {
		return
	}
	d := g.delegations
	d.mu.Lock()
	choice, known := d.resolved[id]
	d.mu.Unlock()
	if !known || choice.session != session {
		return
	}
	r := &d.results
	r.mu.Lock()
	e := r.entries[id]
	same := e != nil && e.NativeTurn == active.Turn
	waiting := e != nil && !e.stopped && e.State == "awaiting_children"
	r.mu.Unlock()
	if !same {
		binding := g.agents.bindingOf(id)
		meta, err := d.metadata(binding)
		if !waiting || binding.SessionID != session || binding.Role != choice.role || err != nil || meta.ToolUseID != choice.call || meta.ParentAgentID != choice.parent || meta.AgentType != choice.role || !metadataModelMatches(choice.role, choice.alias, meta.Model) || meta.StoppedByUser {
			return
		}
		if !r.begin(id) || !g.bindNativeTurn(session, id) {
			return
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current := r.entries[id]; current != nil && current.NativeTurn == active.Turn {
		current.RequestFailure = category
	}
}

// Receipt consumption happens on actual requests/status/finalization. It never
// polls a model or reruns work. Incomplete writes remain unverified.
func (g *Gateway) reconcileNativeResults() {
	if g.delegations == nil || g.nativeEvents.directory == "" {
		return
	}
	r := &g.delegations.results
	r.mu.Lock()
	type snapshot struct {
		key    string
		entry  *agentResult
		record AgentResultRecord
	}
	pending := make([]snapshot, 0, len(r.entries))
	for key, e := range r.entries {
		// A receipt settles a turn whose outcome is still open. Once the report has reached
		// the parent there is nothing left for it to settle, and applying one anyway undid
		// the delivery: a late aborted receipt cleared the body and moved the entry to
		// cancelled, so the next turn told a parent that already held the real report that
		// no completed report was expected from it. The pointer guard below protects a
		// resumed entry, which is a different entry; this is the same one.
		if !e.NativeEndObserved && e.NativeTurn != "" && !resultReported(e.State) {
			pending = append(pending, snapshot{key, e, e.AgentResultRecord})
		}
	}
	r.mu.Unlock()
	for _, item := range pending {
		e := item.record
		var receipt nativeTurnReceipt
		name := "end-" + e.Agent + "-" + e.NativeTurn + ".json"
		if found, err := g.readNativeReceipt(name, &receipt); !found || err != nil {
			continue
		}
		if receipt.Session != e.Session || receipt.Agent != e.Agent || receipt.Turn != e.NativeTurn {
			continue
		}
		switch receipt.Reason {
		case "answer", "aborted", "refusal", "error":
		default:
			continue
		}
		// A turn that starts an asynchronous child can end before the delegated
		// task ends. Only SubagentStop supplies its successful completion body.
		if receipt.Reason == "answer" {
			g.delegations.noteNativeAnswer(e.Agent, receipt.Turn)
		}
		r.mu.Lock()
		// Pointer identity prevents a concurrent resume from being closed by an old receipt.
		if current := r.entries[item.key]; current == item.entry && current.NativeTurn == receipt.Turn {
			current.EndReason = receipt.Reason
			current.NativeEndObserved = true
			if receipt.Reason != "answer" {
				current.stopped = true
				r.bytes -= len(current.body)
				current.body = ""
				current.Bytes = 0
				if receipt.Reason == "aborted" {
					r.change(current, "cancelled")
				} else {
					r.change(current, "result_unavailable")
				}
			}
		}
		r.mu.Unlock()
		// Only this task-created validated receipt is removed; active metadata stays.
		if root, err := os.OpenRoot(g.nativeEvents.directory); err == nil {
			_ = root.Remove(name)
			root.Close()
		}
	}
}

// Called only after native exited/reaping succeeded. A missing terminal receipt
// is unknown, never guessed to be cancellation or left falsely running.
func (g *Gateway) FinalizeNativeResults() {
	g.reconcileNativeResults()
	if g.delegations == nil {
		return
	}
	r0 := &g.delegations.results
	r0.mu.Lock()
	var ended []AgentResultRecord
	for key, e := range r0.entries {
		if key == e.Agent && !e.stopped && e.NativeEndObserved && e.EndReason == "answer" {
			ended = append(ended, e.AgentResultRecord)
		}
	}
	r0.mu.Unlock()
	for _, e := range ended {
		if g.agents != nil {
			b := g.agents.bindingOf(e.Agent)
			b.Result = ""
			g.delegations.stoppedTurn(b, e.NativeTurn)
		}
	}
	g.reconcileWorkflowResults(true)
	r := &g.delegations.results
	r.mu.Lock()
	for _, e := range r.entries {
		if !e.stopped {
			e.stopped = true
			e.EndReason = "session_ended_unverified"
			r.bytes -= len(e.body)
			e.body = ""
			e.Bytes = 0
			r.change(e, "result_unavailable")
		}
	}
	r.mu.Unlock()
	g.delegations.saveWorkflowCheckpoints()
}
