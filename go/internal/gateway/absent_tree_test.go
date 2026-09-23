package gateway

import (
	"errors"
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

func TestMissingChoiceInAvailableProjectsDistinguishesKnownAndCustomRoles(t *testing.T) {
	d := &delegations{projects: t.TempDir()}
	for _, role := range []string{"custom", "Plan"} {
		binding := agentBinding{ID: "proof_child", Role: role, SessionID: "s", TranscriptPath: filepath.Join(d.projects, "project", "s.jsonl")}
		_, found, err := d.loadChoice(delegationScope{session: "s"}, binding.ID, binding)
		if found || role == "custom" && err != nil || role == "Plan" && !errors.Is(err, errDelegationUnverified) {
			t.Fatalf("role=%s found=%v err=%v", role, found, err)
		}
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
