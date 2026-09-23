package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

// Overlay into unchanged v0.3.0. The candidate's saved bytes are not rewritten.
func TestReleaseReadCandidateJournals(t *testing.T) {
	in := os.Getenv("CLAUDUCT_JOURNAL_EVIDENCE")
	if in == "" {
		t.Fatal("explicit evidence directory required")
	}
	for _, role := range []string{"Plan", "plan"} {
		d, scope, binding := preparedDelegation(t)
		binding.Role = role
		meta := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
		if err := os.WriteFile(meta, []byte(`{"toolUseId":"proof_call","agentType":"`+role+`","model":"haiku"}`), 0600); err != nil {
			t.Fatal(err)
		}
		name := "journal-control.json"
		if role == "plan" {
			name = "journal-case-folded.json"
		}
		raw, err := os.ReadFile(filepath.Join(in, name))
		if err != nil {
			t.Fatal(err)
		}
		root, path, err := d.choicePath(binding)
		if err != nil {
			t.Fatal(err)
		}
		f, err := root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			root.Close()
			t.Fatal(err)
		}
		_, writeErr := f.Write(raw)
		closeErr := f.Close()
		root.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("journal copy failed")
		}
		_, found, err := d.loadChoice(scope, binding.ID, binding)
		accepted := found && err == nil
		if accepted != (role == "Plan") {
			t.Fatal("unexpected baseline compatibility", role, accepted, err)
		}
		t.Logf("v030/native-role=%s accepted=%t", role, accepted)
	}
}
