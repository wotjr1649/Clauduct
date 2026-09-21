package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkflowSourceRequiresNativeReadAndUnchangedAdapter(t *testing.T) {
	for _, mutation := range []string{"none", "missing_receipt", "session", "path", "digest", "adapter", "missing_template"} {
		t.Run(mutation, func(t *testing.T) {
			d, scope, _, link := workflowProof(t)
			delete(d.workflowCalls, delegationKey{scope.session, link.Call})
			d.events = t.TempDir()
			input, _ := json.Marshal(map[string]string{"scriptPath": filepath.Join(d.projects, "public.js")})
			out, err := d.adaptWorkflow(scope, link.Call, input)
			if err != nil || string(out) != string(input) {
				t.Fatal("source operation was replaced before native permission", err)
			}
			file := filepath.Join(d.events, "workflow-source-"+link.Call+".json")
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var source workflowSource
			if json.Unmarshal(raw, &source) != nil {
				t.Fatal("template")
			}
			const script = "export const meta={name:'public',description:'public'}; return 43;"
			receipt := map[string]string{"session": scope.session, "call": link.Call, "path": source.Paths[0], "digest": workflowDigest([]byte(script))}
			switch mutation {
			case "session":
				receipt["session"] = "other"
			case "path":
				receipt["path"] = "other"
			case "digest":
				receipt["digest"] = "wrong"
			case "adapter":
				source.Trailer += "changed"
			case "missing_template":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			}
			if mutation != "missing_receipt" {
				raw, _ = json.Marshal(receipt)
				if err := os.WriteFile(filepath.Join(d.events, "workflow-read-"+link.Call+".json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(link.Script, []byte(script+source.Trailer), 0600); err != nil {
				t.Fatal(err)
			}
			err = d.linkWorkflow(link)
			if (err == nil) != (mutation == "none") {
				t.Fatalf("unverified source linked: %v", err)
			}
			if mutation == "none" {
				repeated, _ := json.Marshal(map[string]string{"scriptPath": link.Script})
				if _, err := d.adaptWorkflow(scope, "again", repeated); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(filepath.Join(d.events, "workflow-source-again.json"))
				if err != nil {
					t.Fatal(err)
				}
				var next workflowSource
				_ = json.Unmarshal(raw, &next)
				if next.PreviousTrailer != source.Trailer || next.Trailer == source.Trailer {
					t.Fatal("cached adapter was not bounded to its original call")
				}
			}
		})
	}
}
