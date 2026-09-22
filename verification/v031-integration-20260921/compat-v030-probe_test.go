package gateway

import (
	"encoding/json"
	"os"
	"testing"
)

// Run against the unchanged v0.3.0 reader. Only public synthetic journal fields
// are changed; a PASS means refusal was verified, not that downgrade works.
func TestV031ChoiceDowngradeBoundary(t *testing.T) {
	for _, kind := range []string{"unchanged", "customRole", "native-selection"} {
		t.Run(kind, func(t *testing.T) {
			d, scope, binding := preparedDelegation(t)
			if _, found, err := d.route(scope, binding.ID, binding); err != nil || !found {
				t.Fatal("baseline route")
			}
			root, path, err := d.choicePath(binding)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			raw, err := root.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if json.Unmarshal(raw, &fields) != nil {
				t.Fatal("journal decode")
			}
			if kind == "customRole" {
				fields["customRole"] = true
			}
			if kind == "native-selection" {
				fields["source"] = "native-selection"
			}
			raw, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			file, err := root.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.Write(raw)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatal("fixture write")
			}
			_, found, err := d.loadChoice(scope, binding.ID, binding)
			accepted := found && err == nil
			t.Logf("v030 accepted=%v", accepted)
			if accepted != (kind == "unchanged") {
				t.Fatal("unexpected downgrade boundary")
			}
		})
	}
}
