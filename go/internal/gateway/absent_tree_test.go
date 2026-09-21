package gateway

import (
	"path/filepath"
	"testing"

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

// Not covered, and recorded as such rather than covered badly.
//
// recordFailedAgentRequest reads the active receipt, reads a metadata file, then records the
// turn. The hook overwrites that receipt on every new turn, so the value read first can be
// stale by the time it is written; the fix re-reads and compares before anything
// destructive. Reaching that window deterministically needs the receipt to change *during*
// the call, and there is no injection point for it -- applyNativeTurn records whatever it is
// handed, by design, so a test that drives it directly proves nothing about the caller's
// comparison. One was written that way first and asserted a property of the wrong function.
//
// What holds without a test: the re-read cannot make the outcome worse than applying a value
// already known to be possibly stale, and the comparison fails closed, before begin().
func TestTheStaleTurnWindowIsNarrowedButNotTested(t *testing.T) {
	t.Skip("race window; no injection point that does not test the wrong function")
}
