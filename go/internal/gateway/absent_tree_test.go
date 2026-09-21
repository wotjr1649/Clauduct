package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// A projects tree the client has not written yet is "not there yet", not damage. Three
// readers had to learn that separately and each was silent when reverted: the metadata
// sidecar, the transcript provenance counter, and the context journal. These cover the two
// that had no test.
func TestAnAbsentProjectsTreeIsRetryableForMetadata(t *testing.T) {
	d := &delegations{projects: filepath.Join(t.TempDir(), "never-created")}
	binding := agentBinding{ID: "proof_child", Role: "Plan", SessionID: "s", TranscriptPath: filepath.Join(d.projects, "project", "s.jsonl")}
	_, err := d.metadata(binding)
	if err == nil {
		t.Fatal("metadata answered from a tree that does not exist")
	}
	// The same "not written yet" the missing sidecar one line below is allowed to retry
	// through. Classified as unverified it refused a first turn that delegates outright.
	if err != errMetadataPending {
		t.Fatalf("an absent tree is %v, where an absent sidecar in it is retryable", err)
	}
}

func TestAnAbsentProjectsTreeIsNotAnUnreadableTranscript(t *testing.T) {
	g := &Gateway{delegations: &delegations{projects: filepath.Join(t.TempDir(), "never-created")}}
	g.EnableContextPolicy()
	g.contexts.sessions = map[string]string{"s": "proj/s.jsonl"}
	req := &anthropic.Request{Messages: []anthropic.Message{{Role: "user", Blocks: []anthropic.Block{{Type: "text", Text: "hello"}}}}}
	g.stripContextDisplays(req, "s")
	if n := g.contextDisplayReport().Unreadable; n != 0 {
		t.Fatalf("a tree with nothing in it counted %d provenance failures", n)
	}
}

// The child moved on while this request was reading a metadata file. Binding to the turn it
// left files the failure under a turn that is over and, worse, lets begin() clear the
// awaiting_children evidence that turn's end receipt needs.
//
// This was recorded as untestable in an earlier round, which was wrong: what has no seam is
// the sequence, not the comparison. Naming the predicate is enough -- the receipt on disk
// and the one the caller validated are two values, and a test can simply disagree them.
func TestAReceiptThatMovedOnIsNotTakenAsCurrent(t *testing.T) {
	d, scope, binding := preparedDelegation(t)
	if _, _, err := d.route(scope, binding.ID, binding); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{delegations: d, agents: newAgentRegistry()}
	if _, err := g.agents.register(binding, time.Now()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	g.ConfigureNativeEvents(dir)
	onDisk := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "second", Model: "gpt-5.6-luna", Effort: "high"}
	raw, _ := json.Marshal(onDisk)
	if err := os.WriteFile(filepath.Join(dir, "active-"+binding.ID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	validated := nativeTurnReceipt{Session: scope.session, Agent: binding.ID, Turn: "first", Model: "gpt-5.6-luna", Effort: "high"}
	if _, still := g.stillOnTurn(scope.session, binding.ID, validated); still {
		t.Fatal("a turn the child has already left was taken as current")
	}
	if got, still := g.stillOnTurn(scope.session, binding.ID, onDisk); !still || got.Turn != "second" {
		t.Fatalf("the turn the child is on was rejected: %q still=%v", got.Turn, still)
	}
}
