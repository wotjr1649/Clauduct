package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"net/http"
	"path/filepath"
	"regexp"
)

var errToolHistory = errors.New("TOOL_HISTORY_UNVERIFIED")

// toolIssuance is history provenance, never a substitute for a live native lease.
// Reserved IDs stay in the response owner until validated tool frames are ready.
type toolIssuance struct {
	Session    string `json:"session"`
	Source     string `json:"source"`
	Backend    string `json:"backend"`
	Name       string `json:"name"`
	Generation string `json:"generation"`
	Agent      string `json:"agent"`
	Turn       string `json:"turn"`
	Sequence   int    `json:"sequence"`
	Step       int    `json:"step"`
}

// canonicalToolHistory rewrites only backend copies. Native receipt identities,
// arguments, reasoning, ordering and results are not rewritten or reconstructed.
func canonicalToolHistory(input []bridge.InputEntry, session string, issued map[string]toolIssuance) ([]bridge.InputEntry, error) {
	hasTools := false
	for _, e := range input {
		if e.Type == "function_call" || e.Type == "function_call_output" {
			hasTools = true
			break
		}
	}
	if !hasTools {
		return input, nil
	}
	if session == "" {
		return nil, errToolHistory
	}
	ids, complete, seen := map[string]string{}, map[string]bool{}, map[string]bool{}
	names := map[string]string{}
	out := append([]bridge.InputEntry(nil), input...)
	for i, e := range input {
		switch e.Type {
		case "function_call":
			if e.CallID == "" || ids[e.CallID] != "" {
				return nil, errToolHistory
			}
			hash := sha256.Sum256([]byte(session + "\x00" + e.CallID))
			id := "call_" + hex.EncodeToString(hash[:24])
			if seen[id] {
				return nil, errToolHistory
			}
			seen[id] = true
			ids[e.CallID], names[e.CallID], out[i].CallID = id, e.Name, id
		case "function_call_output":
			id := ids[e.CallID]
			if id == "" || complete[e.CallID] {
				return nil, errToolHistory
			}
			complete[e.CallID] = true
			out[i].CallID = id
		}
	}
	for id := range ids {
		if !complete[id] {
			record, ok := issued[id]
			if !ok || !validToolIssuance(id, record, session) || record.Name != names[id] {
				return nil, errToolHistory
			}
		}
	}
	return out, nil
}

var generationShape = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validToolIssuance(id string, r toolIssuance, session string) bool {
	return anthropic.ValidToolIdentifier(id) && r.Session == session && sessionUUID.MatchString(r.Source) && anthropic.ValidToolIdentifier(r.Backend) && anthropic.ValidToolIdentifier(r.Name) && generationShape.MatchString(r.Generation) && (r.Agent == "" || correlationShape.MatchString(r.Agent)) && correlationShape.MatchString(r.Turn) && r.Sequence > 0 && r.Step >= 0
}
func cloneToolIssuances(input map[string]toolIssuance) map[string]toolIssuance {
	out := make(map[string]toolIssuance, len(input))
	for id, r := range input {
		out[id] = r
	}
	return out
}

func (g *Gateway) callGeneration() string {
	hash := sha256.Sum256([]byte(filepath.ToSlash(g.nativeEvents.directory)))
	return hex.EncodeToString(hash[:])
}
func (g *Gateway) resolveToolHistory(r *http.Request, built *bridge.Request, consume bool, owner *nativeTurnReceipt) error {
	if !g.nativeEvents.confirmationsRequired {
		return nil
	}
	session := r.Header.Get("X-Claude-Code-Session-Id")
	var records map[string]toolIssuance
	var profile *SessionProfile
	if p := g.profiles; p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		profile = p.profiles[session]
		if profile != nil {
			records = cloneToolIssuances(profile.Calls)
		}
	} else {
		g.nativeEvents.mu.Lock()
		defer g.nativeEvents.mu.Unlock()
		records = cloneToolIssuances(g.nativeEvents.issued[session])
	}
	// Pending history is not a completed legacy pair. It needs this request's
	// live owner. Explicit resume provenance admits only a backend history copy.
	completed := map[string]bool{}
	for _, e := range built.Input {
		if e.Type == "function_call_output" {
			completed[e.CallID] = true
		}
	}
	for _, e := range built.Input {
		if e.Type != "function_call" || completed[e.CallID] {
			continue
		}
		record, ok := records[e.CallID]
		if !ok || owner == nil || record.Source != session || record.Agent != r.Header.Get("X-Claude-Code-Agent-Id") || record.Session != owner.Session || record.Agent != owner.Agent {
			return errToolHistory
		}
		// An explicitly loaded resume snapshot admits history, not execution.
		// Preserve its old generation: the native tool guard still rejects it.
		p := g.profiles
		restored := g.nativeEvents.resumedSession == session && g.nativeEvents.resumedHistory[e.CallID] == record
		if p != nil {
			restored = restored && !p.fork && p.ready && p.expected == session && p.current == session
		}
		if !restored && (record.Generation != g.callGeneration() || record.Turn != owner.Turn || record.Sequence != owner.sequence) {
			return errToolHistory
		}
	}
	input, err := canonicalToolHistory(built.Input, session, records)
	if err != nil {
		return err
	}
	// A validated result makes issuance terminal. Completed history needs no live
	// authority; retain only pending calls. count_tokens never consumes authority.
	changed := false
	if consume {
		for _, e := range built.Input {
			if e.Type == "function_call_output" {
				if _, ok := records[e.CallID]; ok {
					delete(records, e.CallID)
					changed = true
				}
			}
		}
	}
	if changed {
		if profile != nil {
			next := *profile
			next.Calls = records
			if err := g.profiles.store.Save(next); err != nil {
				return err
			}
			*profile = next
		} else {
			raw, err := json.Marshal(struct {
				ID    string                  `json:"id"`
				Calls map[string]toolIssuance `json:"calls"`
			}{session, records})
			if err != nil {
				return err
			}
			if err := g.writeNativeControl("issued-"+session+".json", raw); err != nil {
				return err
			}
			g.nativeEvents.issued[session] = records
		}
	}
	built.Input = input
	return nil
}

// publishToolIssuance runs only after Accept validated a completion. Inspect the
// actual outgoing frames: pending suppression may have discarded reservations.
func (g *Gateway) publishToolIssuance(frames []anthropic.Frame, reserved map[string]string, proof *nativeConfirmation, turn *nativeTurnReceipt) error {
	selected := map[string]toolIssuance{}
	for _, frame := range frames {
		if frame.Type != "content_block_start" {
			continue
		}
		var block struct {
			Content struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if json.Unmarshal(frame.Data, &block) != nil {
			return errToolHistory
		}
		if block.Content.Type != "tool_use" {
			continue
		}
		id := block.Content.ID
		if proof == nil || !proof.Confirmations || turn == nil || turn.sequence < 1 || turn.Session != proof.Session || turn.Agent != proof.Agent || turn.Turn != proof.Turn || reserved[id] == "" {
			return errToolHistory
		}
		selected[id] = toolIssuance{Session: proof.Session, Source: proof.Session, Backend: reserved[id], Name: block.Content.Name, Generation: g.callGeneration(), Agent: proof.Agent, Turn: proof.Turn, Sequence: turn.sequence, Step: proof.Index}
	}
	if len(selected) == 0 {
		return nil
	}
	// The pointer contains no issuance. Publish it before committing records so a
	// pointer failure cannot leave an undisclosed call authorized on disk.
	root, prefix := g.nativeEvents.directory, "issued-"
	if g.profiles != nil {
		root, prefix = g.profiles.store.root.Name(), ""
	}
	pointer, _ := json.Marshal(struct {
		Root       string `json:"root"`
		Prefix     string `json:"prefix"`
		Generation string `json:"generation"`
	}{filepath.ToSlash(root), prefix, g.callGeneration()})
	if err := g.writeNativeControl("call-store.json", pointer); err != nil {
		return err
	}
	if p := g.profiles; p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		current := p.profiles[proof.Session]
		if current == nil || !p.ready {
			return errToolHistory
		}
		next := *current
		next.Version = 2
		next.Calls = cloneToolIssuances(current.Calls)
		for id, record := range selected {
			if _, duplicate := next.Calls[id]; duplicate {
				return errToolHistory
			}
			next.Calls[id] = record
		}
		if err := p.store.Save(next); err != nil {
			return err
		}
		*current = next
	} else {
		n := &g.nativeEvents
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.issued == nil {
			n.issued = map[string]map[string]toolIssuance{}
		}
		next := cloneToolIssuances(n.issued[proof.Session])
		for id, record := range selected {
			if _, duplicate := next[id]; duplicate {
				return errToolHistory
			}
			next[id] = record
		}
		if len(next) > 4096 || !correlationShape.MatchString(proof.Session) {
			return errToolHistory
		}
		raw, err := json.Marshal(struct {
			ID    string                  `json:"id"`
			Calls map[string]toolIssuance `json:"calls"`
		}{proof.Session, next})
		if err != nil {
			return err
		}
		if err := g.writeNativeControl("issued-"+proof.Session+".json", raw); err != nil {
			return err
		}
		n.issued[proof.Session] = next
	}
	return nil
}
