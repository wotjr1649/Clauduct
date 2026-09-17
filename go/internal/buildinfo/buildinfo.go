// Package buildinfo identifies the binary that is running.
//
// An evidence record that names a commit is only worth something if the binary can say
// which commit it is. The values come from the Go toolchain's own VCS stamping rather
// than from ldflags a release script might forget to pass.
package buildinfo

import "runtime/debug"

// Version is the product version.
//
// 0.2.0 is the first release of the Go implementation. It follows v0.1.0, which released
// the Node one: the same product with a new implementation underneath and the default
// switched to it, which is a minor step rather than a new product. Still 0.x, and the
// reasons are in PACKAGING.md: unsigned, Windows only, installed by copying files.
//
// A constant rather than a linker flag. The commit stamp comes from the toolchain's own VCS
// record precisely so a release script cannot forget it, and a version that could be passed
// in is a version a script can get wrong.
const Version = "0.2.0"

// Info is what a build can say about itself. An empty field means the toolchain did not
// stamp it, which is a different thing from a zero value and is reported as such.
type Info struct {
	Version   string
	Commit    string
	Modified  bool
	GoVersion string
}

// Read returns the running binary's identity.
func Read() Info {
	info := Info{Version: Version}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
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
