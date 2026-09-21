package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

func TestLegacyContextJournalMigratesWithoutInventingUsage(t *testing.T) {
	projects := t.TempDir()
	g := journalGateway(t, projects)
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "first"))
	s := g.contexts.states[contextKey("public-session", "")]
	raw, err := json.Marshal(contextJournal{Version: 1, Identity: s.identity, Model: s.route.Model, Effort: s.route.Effort, Phase: s.phase, Target: s.target})
	if err != nil || os.WriteFile(filepath.Join(projects, s.journal), raw, 0600) != nil {
		t.Fatal("legacy fixture")
	}
	restarted := journalGateway(t, projects)
	state := &contextState{}
	if err := restarted.restoreContext("public-session", "", state); err != nil || state.usage != nil || state.route.Model != "gpt-5.6-sol" {
		t.Fatal("legacy route or unknown usage lost")
	}
	response := contextRequest(t, restarted, "gpt-5.6-sol", "continue")
	bodyText(t, response)
	if response.StatusCode != 200 || restarted.agentContexts()[0].LastUsage == nil {
		t.Fatal("legacy session did not establish fresh usage")
	}
}

func journalGateway(t *testing.T, projects string) *Gateway {
	t.Helper()
	g, _ := newContextFixture(t)
	g.ConfigureDelegations(projects)
	if err := g.contextSession("public-session", filepath.Join(projects, "public-project", "public-session.jsonl")); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestContextJournalRestoresOldModelAndPendingSwitch(t *testing.T) {
	projects := t.TempDir()
	g := journalGateway(t, projects)
	g.transport.(*contextFixture).tokens = 239000
	bodyText(t, contextRequest(t, g, "gpt-6-astra", "first"))
	bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "switch"))
	restarted := journalGateway(t, projects)
	ticket := compactEvent(t, restarted, "PreCompact")
	response := contextRequest(t, restarted, "gpt-5.6-sol", strings.Replace(compactPrompt(), "synthetic history.", "synthetic history. "+ticket, 1))
	bodyText(t, response)
	if response.StatusCode != 200 {
		t.Fatal("journal restore refused")
	}
	report := restarted.agentContexts()
	if len(report) != 1 || report[0].Model != "gpt-6-astra" || report[0].Effort != "medium" || report[0].PendingModel != "gpt-5.6-sol" || report[0].Phase != "recount" || !report[0].Persistent {
		t.Fatal("old route / pending switch lost")
	}
	bodyText(t, contextRequest(t, restarted, "gpt-5.6-sol", "summary"))
	if report := restarted.agentContexts()[0]; report.Model != "gpt-5.6-sol" || report.Phase != "ready" {
		t.Fatal("restored switch not completed")
	}
}

func TestContextJournalRejectsCorruptionAndInterruptedCompaction(t *testing.T) {
	for _, mode := range []string{"corrupt", "identity", "interrupted", "duplicate_usage", "unknown_usage", "incomplete_usage"} {
		t.Run(mode, func(t *testing.T) {
			projects := t.TempDir()
			g := journalGateway(t, projects)
			bodyText(t, contextRequest(t, g, "gpt-5.6-sol", "first"))
			state := g.contexts.states[contextKey("public-session", "")]
			path := filepath.Join(projects, state.journal)
			if mode == "interrupted" {
				state.phase = "compacting"
				if g.saveContext(state) != nil {
					t.Fatal("save")
				}
			} else {
				raw := []byte("invalid")
				if mode == "identity" {
					raw = []byte(strings.Replace(state.saved, "public-session/", "different/", 1))
				}
				if mode == "duplicate_usage" {
					raw = []byte(strings.Replace(state.saved, `"usage":{`, `"usage":{"inputTokens":1,`, 1))
				}
				if mode == "unknown_usage" {
					raw = []byte(strings.Replace(state.saved, `"usage":{`, `"usage":{"unverified":true,`, 1))
				}
				if mode == "incomplete_usage" {
					raw = []byte(strings.Replace(state.saved, `"outputTokens":2,`, "", 1))
				}
				if os.WriteFile(path, raw, 0600) != nil {
					t.Fatal("write")
				}
			}
			restarted := journalGateway(t, projects)
			response := contextRequest(t, restarted, "gpt-5.6-sol", "continue")
			bodyText(t, response)
			if response.StatusCode != 400 {
				t.Fatal("uncertain state dispatched")
			}
		})
	}
}

func TestContextJournalContainsPathsAndEvictsOnlyDurableIdleState(t *testing.T) {
	projects := t.TempDir()
	g := journalGateway(t, projects)
	if g.contextSession("escape", filepath.Join(projects, "..", "escape.jsonl")) == nil {
		t.Fatal("escaping path accepted")
	}
	for i := 0; i < maxAgents; i++ {
		key := string(rune(i + 1000))
		g.contexts.states[key] = &contextState{busy: true}
	}
	response := contextRequest(t, g, "gpt-5.6-sol", "first")
	if body := bodyText(t, response); response.StatusCode != 400 || !strings.Contains(body, "CONTEXT_STATE_LIMIT") {
		t.Fatal("active state evicted")
	}
	g.contexts.states["durable"] = &contextState{route: bridge.Route{Model: "gpt-5.6-sol", Effort: "low"}, saved: "checkpoint"}
	// Replace one active entry, keeping exactly the cap.
	delete(g.contexts.states, string(rune(1000)))
	response = contextRequest(t, g, "gpt-5.6-sol", "first")
	bodyText(t, response)
	if response.StatusCode != 200 || len(g.contexts.states) != maxAgents {
		t.Fatal("durable idle state not reclaimed")
	}
}
