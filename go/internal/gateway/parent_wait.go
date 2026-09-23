package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// This is a structural completion condition, never a review of a child's findings.
// Identifiers only: result bodies stay in the existing delivery path.
// A journal this build could not write is a local, permanent condition, and the status
// class is a retry instruction: a plain error here fell through categoryFor to
// UPSTREAM_FAILURE and 502, which the measured client answers with eight requests in sixty
// seconds. The sibling path that meets the identical failure one write earlier already
// returns 400 by name. One condition, one classification.
var errParentWaitUnverified = errors.New("PARENT_WAIT_UNVERIFIED")

type ParentReadiness struct {
	Pending     []string `json:"pending,omitempty"`
	Included    []string `json:"includedResults,omitempty"`
	Unavailable []string `json:"unavailableResults,omitempty"`
	Eligible    bool     `json:"completionEligible"`
	Meaning     string   `json:"meaning"`
	Withheld    bool     `json:"replyWithheld"`
	Empty       bool     `json:"backendReplyEmpty,omitempty"`
	ControlMode string   `json:"waitControlMode,omitempty"`
}

type parentStep struct {
	Session  string `json:"session"`
	Agent    string `json:"agent"`
	Turn     string `json:"turn"`
	Index    int    `json:"index"`
	Eligible bool   `json:"eligible"`
	Mode     string `json:"mode"`
	waiting  bool   // gateway snapshot, never accepted from the native receipt
}

func (g *Gateway) readNativeStep(session, id string) (parentStep, bool, error) {
	var step parentStep
	if !correlationShape.MatchString(session) || id != "" && !correlationShape.MatchString(id) {
		return step, false, errDelegationUnverified
	}
	name := "root"
	if id != "" {
		name = "child-" + id
	}
	found, err := g.readNativeJSON("step-"+name+".json", []string{"session", "agent", "turn", "index", "eligible", "mode"}, &step)
	if err != nil {
		return step, false, err
	}
	if found && (step.Session != session || step.Agent != id || !correlationShape.MatchString(step.Turn) || step.Index < 0 || step.Index > 65536 || step.Mode != "native_tui" && step.Mode != "sdk" && step.Mode != "unclassified" || step.Eligible && step.Mode != "native_tui" && step.Mode != "sdk") {
		return step, false, errDelegationUnverified
	}
	return step, found, nil
}

// The same admission snapshot used for result delivery explains whether an answer
// could contain all child reports. Native notification arrival is not the criterion.
func (d *delegations) deliverWithReadiness(request *anthropic.Request, session, parent string) (func(bool), *ParentReadiness) {
	out := &ParentReadiness{Meaning: "known_child_reports_in_input_not_task_success", Eligible: true}
	d.mu.Lock()
	defer d.mu.Unlock()
	for key, choice := range d.pending {
		if key.session == session && choice.parent == parent {
			out.Pending = append(out.Pending, key.call)
		}
	}
	finish := d.results.deliver(request, session, parent, out)
	return finish, out
}

// Native writes a fresh step before requesting inference. The decision is read by
// that same streaming hook before yielding any response; it is never a model poll.
func (g *Gateway) prepareParentWait(r *http.Request, request *anthropic.Request, entry *record) (*parentStep, error) {
	if g.delegations == nil || !conversationRequest(r, request) || r.Header.Get("X-Claude-Code-Request-Class") == "compaction" {
		return nil, nil
	}
	session, id := r.Header.Get("X-Claude-Code-Session-Id"), r.Header.Get("X-Claude-Code-Agent-Id")
	if !correlationShape.MatchString(session) || id != "" && !correlationShape.MatchString(id) {
		return nil, nil
	}
	readiness := entry.snapshot().ParentReadiness
	if readiness == nil {
		return nil, nil
	}
	if g.nativeEvents.directory == "" {
		return nil, nil
	}
	step, found, err := g.readNativeStep(session, id)
	if err != nil {
		return nil, err
	}
	if !found {
		if len(readiness.Pending) > 0 {
			return nil, errDelegationUnverified
		}
		return nil, nil
	} // No native wait capability was demonstrated.
	if err := g.writeParentDecision(&step, false); err != nil {
		return nil, err
	}
	entry.checked("parent_input_snapshot")
	entry.checked("native_wait_step")
	entry.mu.Lock()
	snapshot := *entry.data.ParentReadiness
	snapshot.ControlMode = step.Mode
	entry.data.ParentReadiness = &snapshot
	entry.mu.Unlock()
	if step.Mode == "native_tui" || step.Mode == "sdk" {
		entry.checked("native_wait_control")
	}
	workflowLaunch := step.Eligible && len(readiness.Pending) == 0 && id == "" && step.Index > 0 && g.delegations.returnedWorkflowLaunch(request, session, id)
	forkLaunch := step.Eligible && len(readiness.Pending) == 0 && id == "" && step.Index > 0 && returnedForkLaunch(request)
	step.waiting = len(readiness.Pending) > 0
	// Eligible root index zero is published only for a task notification, never
	// explicit user input. With no pending children it may consume only an empty
	// terminal reply; a real answer or tool call must still reach native.
	if step.Eligible && (step.waiting || workflowLaunch || forkLaunch || id == "" && step.Index == 0) {
		return &step, nil
	}
	return nil, nil
}

// forkLaunchMarker is how native 2.1.280 reports a forked skill that went to the background:
// `Skill "<name>" launched (forked execution, running in the background).`
const forkLaunchMarker = "launched (forked execution, running in the background)"

// In the TUI a forked skill runs in the background (#81). The Skill tool returns at once,
// the parent may end its turn empty while it waits, and the report arrives later as native's
// own notification -- the Workflow launch case below, without a gateway-side record, because
// nothing was prepared for a fork. Correlate only a successful Skill result in this request
// that says so; it permits an empty control reply, never fabricates a child or a result.
// An inline skill returns its content instead, and its empty answer stays EMPTY_REPLY.
func returnedForkLaunch(request *anthropic.Request) bool {
	start := len(request.Messages) - 1
	for start >= 0 && request.Messages[start].Role != "assistant" {
		start--
	}
	if start < 0 {
		return false
	}
	for _, message := range request.Messages[start+1:] {
		if message.Role != "user" {
			continue
		}
		for _, b := range message.Blocks {
			if b.Type != "tool_result" || b.IsError {
				continue
			}
			called := false
			for _, call := range request.Messages[start].Blocks {
				called = called || call.Type == "tool_use" && call.Name == "Skill" && call.ID == b.ToolUseID
			}
			for _, part := range b.Result {
				if called && strings.HasPrefix(part.Text, `Skill "`) && strings.Contains(part.Text, forkLaunchMarker) {
					return true
				}
			}
		}
	}
	return false
}

// A successful Workflow launch returns before its first child is registered.
// Correlate only a tool result in this request with a prepared/linked local run;
// it permits an empty control reply, never fabricates a pending child or result.
func (d *delegations) returnedWorkflowLaunch(request *anthropic.Request, session, parent string) bool {
	if parent != "" || len(request.Messages) == 0 {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// Native appends system reminders after tool results. The latest assistant
	// tool turn is the boundary; step.Eligible excludes a new explicit input.
	start := len(request.Messages) - 1
	for start >= 0 && request.Messages[start].Role != "assistant" {
		start--
	}
	if start < 0 {
		return false
	}
	for _, message := range request.Messages[start+1:] {
		if message.Role != "user" {
			continue
		}
		for _, b := range message.Blocks {
			if b.Type != "tool_result" || b.IsError {
				continue
			}
			called := false
			for _, call := range request.Messages[start].Blocks {
				if call.Type == "tool_use" && call.Name == "Workflow" && call.ID == b.ToolUseID {
					called = true
				}
			}
			if !called {
				continue
			}
			if origin, ok := d.workflowCalls[delegationKey{session, b.ToolUseID}]; ok && !origin.rejected {
				return true
			}
			for _, run := range d.workflows {
				if run.Session == session && run.Call == b.ToolUseID && !run.origin.rejected {
					return true
				}
			}
		}
	}
	return false
}

func (g *Gateway) writeParentDecision(step *parentStep, hold bool) error {
	name := "root"
	if step.Agent != "" {
		name = "child-" + step.Agent
	}
	root, err := os.OpenRoot(g.nativeEvents.directory)
	if err != nil {
		return err
	}
	defer root.Close()
	decision, _ := json.Marshal(struct {
		Turn  string `json:"turn"`
		Index int    `json:"index"`
		Hold  bool   `json:"hold"`
		Wait  bool   `json:"wait"`
	}{step.Turn, step.Index, hold, hold && step.waiting})
	// Written through a temporary and renamed, the way every other file this plugin reads is
	// written. WriteFile truncates in place, so a read landing in that window returns a
	// partial document; the plugin parses this one with a bare JSON.parse, and the
	// SyntaxError escapes the generator rather than reaching the deliberate refusal three
	// lines below it. A rename is atomic, so the reader sees one version or the other.
	nonce, err := newToken()
	if err != nil {
		return err
	}
	temp := "decision-" + name + "." + nonce + ".tmp"
	if err := root.WriteFile(temp, decision, 0600); err != nil {
		return err
	}
	defer root.Remove(temp)
	// Bounded retry, which the other atomic writers here do not need. They are renamed over
	// files only this process reads; this one is renamed over a file the plugin reads at
	// exactly this moment -- that race is why it stopped being an in-place write. Windows
	// refuses to replace a file whose reader did not share delete access, and libuv does
	// share it, so the expected number of retries is zero; an antivirus or indexer holding
	// it briefly is the case this covers. A few milliseconds beats turning a transient
	// sharing violation into a hard PARENT_WAIT_UNVERIFIED.
	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		if renameErr = root.Rename(temp, "decision-"+name+".json"); renameErr == nil {
			return nil
		}
		if attempt == 4 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	return renameErr
}
