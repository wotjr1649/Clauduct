package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
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
	// ReplayKeys is how many executions the replay ledger holds; it refuses new work at
	// maxNativeExecutions (#70).
	ReplayKeys int `json:"replayKeys"`
	// RetiredTurns is how many finished child turns gave their keys back.
	RetiredTurns int64 `json:"retiredTurns"`
}
type nativeTurnReceipt struct {
	Session string `json:"session"`
	Agent   string `json:"agent"`
	Turn    string `json:"turn"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Publication number within the agent's directory; set by readCurrentNativeTurn.
	sequence int
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

// Each attempt has a unique filename. A zero-byte ready marker is published only
// after the body write completes. Seeing an unfinished newer attempt refuses the
// old turn; callers pin the returned receipt for the whole request.
func (g *Gateway) readCurrentNativeTurn(id string) (receipt nativeTurnReceipt, found bool, err error) {
	if g.nativeEvents.directory == "" {
		return
	}
	name := "root"
	if id != "" {
		if !correlationShape.MatchString(id) {
			return receipt, false, errDelegationUnverified
		}
		name = "child-" + id
	}
	root, err := os.OpenRoot(g.nativeEvents.directory)
	if err != nil {
		return receipt, false, err
	}
	defer root.Close()
	directory := "active/" + name
	dir, err := root.Open(directory)
	if errors.Is(err, os.ErrNotExist) {
		return receipt, false, nil
	}
	if err != nil {
		return receipt, false, err
	}
	defer dir.Close()
	// Superseded publications are pruned below, so this bound is reached only when
	// pruning keeps failing; the reader then refuses rather than scanning further.
	entries, err := dir.ReadDir(16385)
	if err != nil && !errors.Is(err, io.EOF) || len(entries) > 16384 {
		return receipt, false, errDelegationUnverified
	}
	latest, stem, turn := 0, "", ""
	for _, entry := range entries {
		file, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok {
			continue
		}
		number, identity, ok := strings.Cut(file, "-")
		sequence, parseErr := strconv.Atoi(number)
		if !ok || parseErr != nil || sequence < 1 || sequence > maxNativeSequence || strconv.Itoa(sequence) != number || !correlationShape.MatchString(identity) || !entry.Type().IsRegular() {
			return receipt, false, errDelegationUnverified
		}
		if sequence > latest {
			latest, stem, turn = sequence, file, identity
		}
	}
	if stem == "" {
		return receipt, false, nil
	}
	marker, err := root.Stat(directory + "/" + stem + ".ready")
	if err != nil || !marker.Mode().IsRegular() || marker.Size() != 0 {
		return receipt, false, errDelegationUnverified
	}
	found, err = g.readNativeReceipt(directory+"/"+stem+".json", &receipt)
	if err == nil && (!found || receipt.Turn != turn) {
		err = errDelegationUnverified
	}
	if err == nil && len(entries) > nativePruneAbove {
		prunePublications(root, directory, entries, latest)
	}
	receipt.sequence = latest
	return receipt, found && err == nil, err
}

// pinNativeTurn reads the request's current turn receipt once; the reading-stage
// cancellation binding, the replay claim and the selection all use this value. Separate
// reads could straddle a newer publication and disagree, and the handler then refused
// the request. ok is false when a receipt exists but cannot be verified for the request.
func (g *Gateway) pinNativeTurn(r *http.Request, entry *record) (turn *nativeTurnReceipt, ok bool) {
	if entry == nil {
		entry = &record{} // an untracked writer: nothing to share the read with
	}
	if entry.turnPinned {
		return entry.nativeTurn, entry.turnValid
	}
	entry.turnPinned = true
	entry.nativeTurn, entry.nativeResult, entry.nativeResultTurn = nil, nil, ""
	session, agent := r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id")
	// Capture the predecessor before reading the receipt. A later refusal may
	// replace only this unchanged result, never a turn admitted in the meantime.
	if g.delegations != nil {
		results := &g.delegations.results
		results.mu.Lock()
		entry.nativeResult = results.entries[agent]
		if entry.nativeResult != nil {
			entry.nativeResultTurn = entry.nativeResult.NativeTurn
		}
		results.mu.Unlock()
	}
	receipt, found, err := g.readCurrentNativeTurn(agent)
	entry.turnValid = err == nil && (!found || validActiveReceipt(receipt, session, agent))
	if found && entry.turnValid {
		entry.nativeTurn = &receipt
	}
	return entry.nativeTurn, entry.turnValid
}

// The module numbers publications with a JavaScript number; beyond this it loses
// integer precision.
const maxNativeSequence = 1<<53 - 1

// A long session publishes a turn for every prompt and child, and only the newest
// decides. Keep a margin for a reader that listed the directory just before a newer
// publication and remove the rest, so the scan above stays small.
const (
	nativePruneAbove = 256 // directory entries, two per publication
	nativeKeepRecent = 32  // publications
)

func prunePublications(root *os.Root, directory string, entries []os.DirEntry, latest int) {
	for _, entry := range entries {
		file, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok {
			continue
		}
		number, _, _ := strings.Cut(file, "-")
		if sequence, err := strconv.Atoi(number); err == nil && sequence <= latest-nativeKeepRecent {
			// ponytail: best effort; a file held open elsewhere stays until the next prune.
			_ = root.Remove(directory + "/" + file + ".json")
			_ = root.Remove(directory + "/" + file + ".ready")
		}
	}
}

func (g *Gateway) nativeEventReport() NativeEventReport {
	g.executions.Lock()
	keys, retired := len(g.executions.seen), g.executions.retired
	g.executions.Unlock()
	n := &g.nativeEvents
	n.mu.Lock()
	defer n.mu.Unlock()
	return NativeEventReport{Configured: n.directory != "", Observed: n.verified, Invalid: n.invalid, ReplayKeys: keys, RetiredTurns: retired}
}

// Bind the current native turn before inference, not by time proximity. A
// resumed agent's previous end receipt cannot terminate this new execution.
// readActiveTurn answers everything about the receipt that can refuse a request, and
// changes nothing. It is separate from applying it because the handler runs beginResult in
// between, and beginResult is destructive: it clears NativeTurn, NativeEndObserved and
// EndReason for an awaiting_children parent and drops the body of an awaiting_native_stop
// child. A refusal after that point left the request unrun and the evidence gone, and
// gone for good -- continuation requires exactly the fields it wiped, and
// reconcileNativeResults only looks at entries that still have a NativeTurn.
//
// The one check that cannot move is the turn comparison below, since clearing the old turn
// is what beginResult is for.
func (g *Gateway) readActiveTurn(session, id string) (nativeTurnReceipt, bool, bool) {
	var receipt nativeTurnReceipt
	if g.delegations == nil || id == "" || !correlationShape.MatchString(id) {
		return receipt, false, true
	}
	receipt, found, err := g.readCurrentNativeTurn(id)
	if err != nil {
		return receipt, false, false
	}
	if !found {
		return receipt, false, g.nativeEvents.directory == ""
	}
	if !validActiveReceipt(receipt, session, id) {
		return receipt, false, false
	}
	return receipt, true, true
}

func (g *Gateway) bindNativeTurn(session, id string) bool {
	receipt, present, ok := g.readActiveTurn(session, id)
	if !ok || !present {
		return ok
	}
	return g.applyNativeTurn(id, receipt)
}

func (g *Gateway) applyNativeTurn(id string, receipt nativeTurnReceipt) bool {
	r := &g.delegations.results
	r.mu.Lock()
	defer r.mu.Unlock()
	return g.applyNativeTurnLocked(id, receipt)
}

// Caller holds results.mu so replacement and binding are one state transition.
func (g *Gateway) applyNativeTurnLocked(id string, receipt nativeTurnReceipt) bool {
	if e := g.delegations.results.entries[id]; e != nil {
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
	if receipt.Session != session || receipt.Agent != id || !correlationShape.MatchString(receipt.Turn) || receipt.Reason != "" {
		return false
	}
	if id == "" {
		return receipt.Model == "" && receipt.Effort == ""
	}
	modelKnown := receipt.Model == "unlisted"
	for _, model := range bridge.Models {
		modelKnown = modelKnown || receipt.Model == model.ID
	}
	return modelKnown && (receipt.Effort == "unlisted" || slices.Contains(bridge.Efforts, receipt.Effort))
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
	active := record.nativeTurn
	if active == nil || !validActiveReceipt(*active, session, id) {
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
	owned := e != nil && e == record.nativeResult && e.NativeTurn == record.nativeResultTurn
	r.mu.Unlock()
	if !same {
		if !waiting || !owned {
			return
		}
		binding := g.agents.bindingOf(id)
		meta, err := d.metadata(binding)
		if binding.SessionID != session || !roleMatches(choice.role, binding.Role, choice.custom) || err != nil || meta.ToolUseID != choice.call || meta.ParentAgentID != choice.parent || !roleMatches(choice.role, meta.AgentType, choice.custom) || !metadataModelMatches(choice.role, choice.alias, meta.Model, choice.route.Source, choice.custom) || meta.StoppedByUser {
			return
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !same {
		current := r.entries[id]
		if current != e || current.NativeTurn != record.nativeResultTurn || current.stopped || current.State != "awaiting_children" {
			return
		}
		if !r.beginLocked(id) || !g.applyNativeTurnLocked(id, *active) {
			return
		}
	}
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
		g.executions.retire(receipt.Session, receipt.Agent, receipt.Turn)
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
	g.retireEndedChildren()
}

// retireEndedChildren retires every child turn the replay ledger still holds open whose end
// receipt is on disk (#70), whether or not a result entry reads it: a fork child has no
// entry, and an entry whose report was delivered no longer reads its receipt. The receipts
// stay where they are; consuming one is a result entry's job.
func (g *Gateway) retireEndedChildren() {
	l := &g.executions
	l.Lock()
	var open []nativeTurnReceipt
	for id, c := range l.current {
		if id[1] != "" && !c.ended && correlationShape.MatchString(id[1]) && correlationShape.MatchString(c.turn) {
			open = append(open, nativeTurnReceipt{Session: id[0], Agent: id[1], Turn: c.turn})
		}
	}
	l.Unlock()
	for _, want := range open {
		var receipt nativeTurnReceipt
		if found, err := g.readNativeReceipt("end-"+want.Agent+"-"+want.Turn+".json", &receipt); !found || err != nil {
			continue
		}
		switch receipt.Reason {
		case "answer", "aborted", "refusal", "error":
			// The body must name the turn its file is named for; retire keys on the session.
			if receipt.Agent == want.Agent && receipt.Turn == want.Turn {
				l.retire(receipt.Session, receipt.Agent, receipt.Turn)
			}
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
