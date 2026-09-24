// Package buildinfo identifies the binary that is running.
//
// An evidence record that names a commit is only worth something if the binary can say
// which commit it is. The values come from the Go toolchain's own VCS stamping rather
// than from ldflags a release script might forget to pass.
package buildinfo

import "runtime/debug"

// Info is what a build can say about itself. An empty field means the toolchain did not
// stamp it, which is a different thing from a zero value and is reported as such.
type Info struct {
	// Version is the main module's version as the toolchain stamped it: the tag for a build
	// of a tagged commit, a pseudo-version for any other commit, "+dirty" for a modified
	// worktree, "(devel)" when there was nothing to stamp from (go run, go test). It comes
	// from the tag rather than a constant because a constant had to be raised by a separate
	// commit before every tag, and v0.3.2 nearly shipped naming 0.3.1 (#111). The toolchain
	// stamps it only because go.mod sits at the repository root.
	Version   string
	Commit    string
	Modified  bool
	GoVersion string
}

// Read returns the running binary's identity.
func Read() Info {
	var info Info
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	info.Version = build.Main.Version
	info.GoVersion = build.GoVersion
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.modified":
			info.Modified = setting.Value == "true"
		}
	}
	return info
}

// CommitOrUnknown never invents a hash. A build with no stamp says so.
func (i Info) CommitOrUnknown() string {
	if i.Commit == "" {
		return "unknown"
	}
	if i.Modified {
		return i.Commit + "+dirty"
	}
	return i.Commit
}
