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
// role beside it died and every Agent call in the session ended unverified. A note whose
// first line is a --- rule is enough: the frontmatter then has no closing fence.
func TestOneUnreadableDefinitionDoesNotTakeTheOthersDown(t *testing.T) {
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
	// The skipped file may have held the role being asked for, so an absent role is
	// unverified rather than absent. Guessing a route here is what the refusal exists for.
	if _, _, err := s.resolve("absent", parent); err == nil {
		t.Fatal("a role that may have been in the skipped file resolved as simply absent")
	}
}

// Without a skipped file, an unknown role is still absent rather than an error: that is
// how delegation falls through to native's own routing.
func TestAnUnknownRoleInACleanScanIsAbsentNotAnError(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	if _, found, err := s.resolve("absent", bridge.Route{Model: "gpt-6-astra", Effort: "low"}); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

// Priority is what makes a skip matter. A file passed over in a directory searched before
// the match may hold this very role at a higher priority, so answering from the lower one
// would be the silent misroute that skipping rather than aborting was not supposed to buy.
// A skip in the matching directory or below it cannot outrank what was found, and treating
// it as if it could puts one stray markdown file back in charge of every role beside it.
func TestASkipOnlyInvalidatesAMatchItCouldHaveOutranked(t *testing.T) {
	high, low := t.TempDir(), t.TempDir()
	if os.WriteFile(filepath.Join(high, "notes.md"), []byte("---\nno closing fence\n"), 0600) != nil {
		t.Fatal("write")
	}
	if os.WriteFile(filepath.Join(low, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	parent := bridge.Route{Model: "gpt-6-astra", Effort: "low"}
	above := roleSources{directories: []roleDirectory{{path: high}, {path: low}}}
	if _, _, err := above.resolve("reviewer", parent); err == nil {
		t.Fatal("a lower-priority definition answered while a higher-priority file went unread")
	}
	// The same two directories the other way round: the skip is now below the match.
	below := roleSources{directories: []roleDirectory{{path: low}, {path: high}}}
	route, found, err := below.resolve("reviewer", parent)
	if err != nil || !found || route.Model != "gpt-5.6-terra" {
		t.Fatalf("a skip beneath the match invalidated it: %s/%s found=%v err=%v", route.Model, route.Effort, found, err)
	}
}
