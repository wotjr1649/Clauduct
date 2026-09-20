package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

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
	name := "root"
	if id != "" {
		name = "child-" + id
	}
	var step parentStep
	found, err := g.readNativeJSON("step-"+name+".json", []string{"session", "agent", "turn", "index", "eligible", "mode"}, &step)
	if err != nil {
		return nil, err
	}
	if !found {
		if len(readiness.Pending) > 0 {
			return nil, errDelegationUnverified
		}
		return nil, nil
	} // No native wait capability was demonstrated.
	if step.Session != session || step.Agent != id || !correlationShape.MatchString(step.Turn) || step.Index < 0 || step.Index > 65536 || step.Mode != "native_tui" && step.Mode != "sdk" && step.Mode != "unclassified" || step.Eligible && step.Mode != "native_tui" {
		return nil, errDelegationUnverified
	}
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
	if step.Mode == "native_tui" {
		entry.checked("native_tui")
	}
	if step.Eligible && len(readiness.Pending) > 0 {
		return &step, nil
	}
	return nil, nil
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
	}{step.Turn, step.Index, hold})
	return root.WriteFile("decision-"+name+".json", decision, 0600)
}
