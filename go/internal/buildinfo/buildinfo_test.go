package buildinfo

import "testing"

// A release record names a commit, and the whole point of the name is that someone can go
// and look at it. These are pure functions and were untested, which a mutation run found by
// inventing a hash and watching nothing fail.
func TestABuildNeverInventsACommit(t *testing.T) {
	for name, tc := range map[string]struct {
		info Info
		want string
	}{
		"no stamp at all": {Info{}, "unknown"},
		"a stamp, clean":  {Info{Commit: "abc123"}, "abc123"},
		"a stamp, modified": {
			Info{Commit: "abc123", Modified: true}, "abc123+dirty"},
		// Modified with nothing to be modified from is still nothing to report. Saying
		// "unknown+dirty" would be dressing an absence up as a measurement.
		"modified with no stamp": {Info{Modified: true}, "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.info.CommitOrUnknown(); got != tc.want {
				t.Fatalf("CommitOrUnknown() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A build from a modified worktree has to say so, and it is the most important thing this
// function does. A release made from uncommitted changes names a commit that does not
// contain them, which is a provenance record that is worse than none: it is confidently
// wrong.
func TestAModifiedWorktreeIsAlwaysDeclared(t *testing.T) {
	clean := Info{Commit: "0123456789abcdef0123456789abcdef01234567"}
	modified := clean
	modified.Modified = true

	if clean.CommitOrUnknown() == modified.CommitOrUnknown() {
		t.Fatalf("a modified build is indistinguishable from a clean one: both say %q",
			clean.CommitOrUnknown())
	}
	if got := modified.CommitOrUnknown(); got != clean.CommitOrUnknown()+"+dirty" {
		t.Fatalf("a modified build reports %q", got)
	}
}

// The version placeholder is not a claim that a release exists. If this ever becomes a real
// version it should be because someone decided to release, not because a constant drifted.
func TestTheVersionIsStillAPlaceholder(t *testing.T) {
	if Read().Version != Version {
		t.Fatalf("Read reported %q against the constant %q", Read().Version, Version)
	}
	if Version == "" {
		t.Fatal("an empty version says nothing at all")
	}
}
