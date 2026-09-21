package app

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// lockDirectory holds a directory open with no sharing, which makes os.ReadDir fail with a
// sharing violation. No elevation needed, and WalkDir reports it through the error callback
// with a non-nil entry whose IsDir is true -- the branch under test.
func lockDirectory(t *testing.T, dir string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Skipf("path not representable: %v", err)
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Skipf("cannot take an exclusive directory handle here: %v", err)
	}
	t.Cleanup(func() { syscall.CloseHandle(h) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("the handle did not block enumeration on this filesystem")
	}
}

// A subdirectory nobody can enumerate is skipped, the way an unreadable file is: the
// definitions beside it still answer.
func TestAnUnenumerableSubdirectoryDoesNotEndEveryRole(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	sub := filepath.Join(dir, "drafts")
	if os.MkdirAll(sub, 0700) != nil {
		t.Fatal("mkdir")
	}
	lockDirectory(t, sub)
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	route, found, err := s.resolve("reviewer", bridge.Route{Model: "gpt-6-astra", Effort: "low"})
	if err != nil || !found || route.Model != "gpt-5.6-terra" {
		t.Fatalf("one unenumerable subdirectory ended every role: %s found=%v err=%v", route.Model, found, err)
	}
}

// The root is not a subdirectory. Swallowing its own enumeration failure returned an empty
// map with no error, so an agents directory that exists and cannot be read answered "no
// roles here" -- and every role in it then ran on the caller's model with no refusal and no
// signal that reaches the account.
func TestAnUnenumerableRootIsRefusedNotReportedEmpty(t *testing.T) {
	dir := t.TempDir()
	if os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: proof\nmodel: gpt-5.6-terra\neffort: high\n---\nPrompt"), 0600) != nil {
		t.Fatal("write")
	}
	lockDirectory(t, dir)
	s := roleSources{directories: []roleDirectory{{path: dir}}}
	if _, found, err := s.resolve("reviewer", bridge.Route{Model: "gpt-6-astra", Effort: "low"}); err == nil {
		t.Fatalf("an unreadable agents directory answered as empty: found=%v", found)
	}
}
