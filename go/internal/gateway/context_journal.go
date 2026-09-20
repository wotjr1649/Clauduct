package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

var errContextJournal = errors.New("CONTEXT_JOURNAL_UNVERIFIED")

type contextJournal struct {
	Version  int                 `json:"version"`
	Identity string              `json:"identity"`
	Model    string              `json:"model"`
	Effort   string              `json:"effort"`
	Phase    string              `json:"phase"`
	Target   string              `json:"target"`
	Usage    *contextUsageAnchor `json:"usage,omitempty"`
}

// Caller holds contexts.mu. A path from the hook only selects within the native
// projects root; Root.Open also rejects reparse/symlink escapes at time of use.
func (g *Gateway) contextSession(session, transcript string) error {
	if g.delegations == nil || len(transcript) > 4096 || filepath.Base(transcript) != session+".jsonl" {
		return errContextJournal
	}
	rel, err := filepath.Rel(g.delegations.projects, transcript)
	if err != nil || !filepath.IsLocal(rel) {
		return errContextJournal
	}
	if _, found := g.contexts.sessions[session]; !found && len(g.contexts.sessions) >= maxAgents {
		return errContextJournal
	}
	if prior := g.contexts.sessions[session]; prior != "" && prior != rel {
		return errContextJournal
	}
	g.contexts.sessions[session] = rel
	g.delegations.mu.Lock()
	if g.delegations.workflowSessions == nil {
		g.delegations.workflowSessions = map[string]string{}
	}
	g.delegations.workflowSessions[session] = rel
	g.delegations.mu.Unlock()
	return nil
}

func (g *Gateway) restoreContext(session, agent string, state *contextState) error {
	state.identity = contextKey(session, agent)
	transcript := g.contexts.sessions[session]
	if transcript == "" {
		return nil
	} // Older clients report nonpersistent status.
	state.journal = strings.TrimSuffix(transcript, ".jsonl") + ".clauduct-context.json"
	if agent != "" {
		state.journal = filepath.Join(filepath.Dir(transcript), session, "subagents", "agent-"+agent+".clauduct-context.json")
	}
	root, err := os.OpenRoot(g.delegations.projects)
	if err != nil {
		return errContextJournal
	}
	defer root.Close()
	file, err := root.Open(state.journal)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errContextJournal
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return errContextJournal
	}
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return errContextJournal
	}
	fields, err := wire.Fields(raw, []string{"version", "identity", "model", "effort", "phase", "target", "usage"})
	var saved contextJournal
	if err != nil || len(fields) < 6 || len(fields) > 7 || json.Unmarshal(raw, &saved) != nil || (saved.Version != 1 && saved.Version != 2) || saved.Identity != state.identity || saved.Effort == "" || saved.Version == 1 && (len(fields) != 6 || fields["usage"] != nil) {
		return errContextJournal
	}
	for _, required := range []string{"version", "identity", "model", "effort", "phase", "target"} {
		if fields[required] == nil {
			return errContextJournal
		}
	}
	route, err := bridge.SelectRoute(saved.Model, saved.Effort)
	if err != nil || route.Model != saved.Model {
		return errContextJournal
	}
	if saved.Target != "" {
		if _, ok := policyFor(saved.Target); !ok {
			return errContextJournal
		}
	}
	switch saved.Phase {
	case "", "required", "recount", "failed":
	case "compacting":
		saved.Phase = "failed"
	default:
		return errContextJournal
	}
	state.route, state.phase, state.target = route, saved.Phase, saved.Target
	if saved.Usage != nil {
		usageFields, err := wire.Fields(fields["usage"], []string{"model", "effort", "inputTokens", "outputTokens", "textEstimate"})
		if err != nil || len(usageFields) != 5 {
			return errContextJournal
		}
		u := saved.Usage
		if _, err := bridge.SelectRoute(u.Model, u.Effort); err != nil || u.Effort == "" {
			return errContextJournal
		}
		if _, ok := policyFor(u.Model); !ok || u.Input < 0 || u.Output < 0 || u.TextEstimate < 0 || u.Input > 1<<40 || u.Output > 1<<40 || u.TextEstimate > 1<<40 {
			return errContextJournal
		}
		state.usage = u
	}
	state.saved = string(raw)
	return nil
}

func (g *Gateway) saveContext(state *contextState) error {
	if state.journal == "" || state.route.Model == "" {
		return nil
	}
	raw, _ := json.Marshal(contextJournal{Version: 2, Identity: state.identity, Model: state.route.Model, Effort: state.route.Effort, Phase: state.phase, Target: state.target, Usage: state.usage})
	if string(raw) == state.saved {
		return nil
	}
	state.persistenceError = true
	root, err := os.OpenRoot(g.delegations.projects)
	if err != nil {
		return errContextJournal
	}
	defer root.Close()
	nonce, err := newToken()
	if err != nil {
		return errContextJournal
	}
	// Native creates the project transcript directory lazily on its first write.
	// SessionStart can precede that write; create only its validated relative parent.
	if root.MkdirAll(filepath.Dir(state.journal), 0700) != nil {
		return errContextJournal
	}
	temp := state.journal + "." + nonce + ".tmp"
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errContextJournal
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || root.Rename(temp, state.journal) != nil {
		return errContextJournal
	}
	state.saved, state.persistenceError = string(raw), false
	return nil
}
