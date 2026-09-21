package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// A definition that names a model and no effort is the common shape, and it used to take
// the parent's effort. The same choice written as Agent(model:) takes the model's own
// default, so one intent routed two ways: astra ran at a max parent's effort rather than
// its own medium, and a parent at low quietly downgraded a role pinned to sol.
func TestANamedRoleModelTakesItsOwnDefaultEffort(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-6-astra\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	for _, parent := range []bridge.Route{{Model: "gpt-5.6-luna", Effort: "max"}, {Model: "gpt-5.6-luna", Effort: "low"}} {
		route, found, err := s.resolve("reviewer", parent)
		if err != nil || !found {
			t.Fatalf("resolve: %v", err)
		}
		if route.Model != "gpt-6-astra" || route.Effort != "medium" {
			t.Fatalf("parent %s leaked into the role: got %s/%s", parent.Effort, route.Model, route.Effort)
		}
	}
}

// A definition that names no model still inherits the parent's whole route, which is the
// only thing it can mean.
func TestARoleWithoutAModelStillInheritsTheParentRoute(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "helper.md"), []byte("---\nname: helper\ndescription: proof\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	route, found, err := s.resolve("helper", bridge.Route{Model: "gpt-5.6-sol", Effort: "low"})
	if err != nil || !found || route.Model != "gpt-5.6-sol" || route.Effort != "low" {
		t.Fatalf("got %s/%s found=%v err=%v", route.Model, route.Effort, found, err)
	}
}

// One markdown file the parser cannot read used to abort the whole walk, so every valid
// role beside it died. A note whose first line is a --- rule is enough: the frontmatter then
// has no closing fence. Roles it could not have defined are unaffected -- refusing those was
// the same failure one level up.
func TestOneUnreadableDefinitionOnlyDistrustsItsOwnName(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	if os.WriteFile(filepath.Join(dir, "notes.md"), []byte("---\nmeeting notes, no closing fence\n"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	parent := bridge.Route{Model: "gpt-6-astra", Effort: "low"}
	route, found, err := s.resolve("reviewer", parent)
	if err != nil || !found || route.Model != "gpt-5.6-terra" || route.Effort != "high" {
		t.Fatalf("a valid role beside an unreadable file was lost: %s/%s found=%v err=%v", route.Model, route.Effort, found, err)
	}
	if _, _, err := s.resolve("notes", parent); err == nil {
		t.Fatal("the skipped file's own role resolved as simply absent")
	}
	if _, found, err := s.resolve("unrelated", parent); err != nil || found {
		t.Fatalf("an unrelated role was refused over someone else's unreadable file: found=%v err=%v", found, err)
	}
}

// A file passed over in a higher-priority directory may hold the role being asked for, and
// then the lower-priority answer is one this scan cannot stand behind. A skipped file of a
// different name says nothing about it.
func TestASkipOutranksAMatchOnlyWhenItSharesItsName(t *testing.T) {
	parent := bridge.Route{Model: "gpt-6-astra", Effort: "low"}
	low := t.TempDir()
	if os.WriteFile(filepath.Join(low, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	sameName := t.TempDir()
	if os.WriteFile(filepath.Join(sameName, "reviewer.md"), []byte("---\nno closing fence\n"), 0600) != nil {
		t.Fatal("write")
	}
	if _, _, err := (roleSources{directories: []roleDirectory{{path: sameName}, {path: low}}}).resolve("reviewer", parent); err == nil {
		t.Fatal("a lower-priority definition answered while a file of that name went unread above it")
	}
	otherName := t.TempDir()
	if os.WriteFile(filepath.Join(otherName, "notes.md"), []byte("---\nno closing fence\n"), 0600) != nil {
		t.Fatal("write")
	}
	route, found, err := (roleSources{directories: []roleDirectory{{path: otherName}, {path: low}}}).resolve("reviewer", parent)
	if err != nil || !found || route.Model != "gpt-5.6-terra" {
		t.Fatalf("an unrelated unreadable file above the match refused it: %s/%s found=%v err=%v", route.Model, route.Effort, found, err)
	}
}

// The limit of naming a skipped file by its filename, recorded rather than hidden.
//
// A file that declares `name: reviewer` in frontmatter this scan cannot parse may be called
// anything, and then the only evidence of what it would have defined is gone with the
// frontmatter. Two files declaring one role is the ambiguity def.invalid refuses when both
// parse; when one does not, and its filename does not say so, the readable one answers.
//
// The alternative is refusing every role in the tree whenever any file is unreadable, which
// is the failure this whole mechanism exists to avoid -- one stray markdown file ending
// every delegation in the session. The filename convention is native's own for a definition
// whose frontmatter omits a name, so it covers the ordinary case; this is what it costs.
func TestAnUnreadableDuplicateUnderAnotherFilenameIsNotCaught(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	if os.WriteFile(filepath.Join(dir, "reviewer-old.md"), []byte("---\nname: reviewer\nmodel: [broken]\n---\n"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	route, found, err := s.resolve("reviewer", bridge.Route{Model: "gpt-6-astra", Effort: "low"})
	if err != nil || !found || route.Model != "gpt-5.6-terra" {
		t.Fatalf("behaviour changed; the comment above no longer describes it: %s found=%v err=%v", route.Model, found, err)
	}
	// The property the found==true branch of the refusal exists for, which needs two
	// directories: one file cannot be both readable and not in a single directory. Overwriting
	// reviewer.md here instead left no definition anywhere, so the assertion degenerated into
	// the !found case covered above and the branch survived deletion under it.
	above := t.TempDir()
	if os.WriteFile(filepath.Join(above, "reviewer.md"), []byte("---\nname: reviewer\nmodel: [broken]\n---\n"), 0600) != nil {
		t.Fatal("write")
	}
	shadowed := roleSources{directories: []roleDirectory{{path: above}, {path: dir}}}
	if _, found, err := shadowed.resolve("reviewer", bridge.Route{Model: "gpt-6-astra", Effort: "low"}); err == nil {
		t.Fatalf("a readable definition answered while a file of that name went unread above it: found=%v", found)
	}
}
