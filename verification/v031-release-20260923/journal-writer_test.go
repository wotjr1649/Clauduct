package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Overlay into the candidate gateway package. Journals come from the actual
// v0.3.1 writer; only the public native metadata's role spelling is varied.
func TestReleaseWriteCompatibilityJournals(t *testing.T) {
	out := os.Getenv("CLAUDUCT_JOURNAL_EVIDENCE")
	if out == "" {
		t.Fatal("explicit evidence directory required")
	}
	for _, role := range []string{"Plan", "plan"} {
		d, scope, binding := preparedDelegation(t)
		binding.Role = role
		meta := filepath.Join(d.projects, "project", scope.session, "subagents", "agent-"+binding.ID+".meta.json")
		if err := os.WriteFile(meta, []byte(`{"toolUseId":"proof_call","agentType":"`+role+`","model":"haiku"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := d.route(scope, binding.ID, binding); err != nil || !found {
			t.Fatal("candidate route", err)
		}
		root, path, err := d.choicePath(binding)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := root.ReadFile(path)
		root.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"role":"Plan"`) {
			t.Fatal("canonical writer evidence missing")
		}
		name := "journal-control.json"
		if role == "plan" {
			name = "journal-case-folded.json"
		}
		if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := d.loadChoice(scope, binding.ID, binding); err != nil || !found {
			t.Fatal("candidate cannot read its own journal", err)
		}
		t.Logf("writer/native-role=%s candidate_reload=true", role)
	}
}
