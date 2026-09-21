package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// The client creates its projects tree lazily. With auto memory disabled it has not touched
// it when the first request arrives, so on a configuration directory that is new the
// directory is simply absent -- which is a session with no journal yet, not a damaged one.
// Reading it as damage refused every first /v1/messages with CONTEXT_JOURNAL_UNVERIFIED and
// the session died having made no inference.
func TestAProjectsDirectoryThatDoesNotExistYetIsNotADamagedJournal(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "never-created")
	g := &Gateway{delegations: &delegations{projects: absent}}
	g.EnableContextPolicy()
	g.contexts.sessions = map[string]string{"s": "proj/s.jsonl"}
	state := &contextState{}
	if err := g.restoreContext("s", "", state); err != nil {
		t.Fatalf("a session with nothing written yet was refused: %v", err)
	}
	if _, err := os.Stat(absent); err == nil {
		t.Fatal("restoring created the directory; reading must not write")
	}
}

// Saving is what establishes the tree, so it creates what restoring was content to do
// without. Otherwise the first turn passes admission and the next one fails on the journal.
func TestSavingEstablishesTheProjectsTree(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "never-created")
	g := &Gateway{delegations: &delegations{projects: absent}}
	g.EnableContextPolicy()
	state := &contextState{journal: "proj/s.clauduct-context.json", identity: "s|", route: bridge.Route{Model: "gpt-6-astra", Effort: "low", Source: "catalogue"}}
	if err := g.saveContext(state); err != nil {
		t.Fatalf("save refused on a tree it is allowed to create: %v", err)
	}
	if state.persistenceError {
		t.Fatal("save left the state latched as unwritable")
	}
	if _, err := os.Stat(filepath.Join(absent, "proj", "s.clauduct-context.json")); err != nil {
		t.Fatalf("journal not written: %v", err)
	}
}
