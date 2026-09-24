// Package buildinfo identifies the binary that is running.
//
// An evidence record that names a commit is only worth something if the binary can say
// which commit it is. The values come from the Go toolchain's own VCS stamping rather
// than from ldflags a release script might forget to pass.
package buildinfo

import "runtime/debug"

// Version is the product version.
//
// 0.2.0 was the first release of the Go implementation. It follows v0.1.0, which released
// the Node one: the same product with a new implementation underneath and the default
// switched to it, which is a minor step rather than a new product. Still 0.x, and the
// reasons are in PACKAGING.md: unsigned, Windows only, installed by copying files.
//
// 0.2.1 adds --uninstall and nothing else. Strict semver would call a new option a minor
// bump; 0.x is exempt from that rule, and the more useful reading here is that nothing
// about what this bridge does to a request changed -- translation, streaming, routing and
// the budget are byte for byte what 0.2.0 shipped, and a session behaves identically. What
// changed is that the binary can now remove itself on a machine that never cloned the
// repository, which is tooling around the install rather than the product's behaviour.
//
// 0.2.2 fixes --update. It had no check for being already current, so it replaced three
// files with the same bytes and, because a running executable cannot delete its own
// predecessor, left a clauduct.exe.old behind every time somebody ran it. Reported from
// real use. Nothing about the bridge changed here either.
//
// 0.2.3 fixes --uninstall, which left a clauduct.exe.old in the installation. 0.2.2 had
// added a sweep on the next launch, and after an uninstall there is no next launch, so
// the running image is moved out of the installation instead of beside it. The bridge
// is untouched again.
//
// 0.3.0 is the first of these that is not a patch. 0.2.x carried a bridge that translated one
// request and streamed one response; delegation existed, but everything around it -- what a
// child inherits, what happens when a run is cancelled, what the context is actually costing --
// sat outside it. This version brings those in: Workflow scripts inline and from a file, with
// the results of a finished run recovered after a restart instead of run again; a model and
// effort named for one delegation and inherited by the children of that delegation;
// cancellation that reaches the OS process tree rather than only the request; context
// accounting from the usage the backend measured; explicit count_tokens; merged user settings;
// non-streaming JSON responses; and PDF input. None of that is what 0.2.3 sent upstream, which
// is what the minor bump says.
//
// Still 0.x, and for the reasons PACKAGING.md has given since 0.2.0: unsigned, Windows only,
// installed by copying files. The supported range is also narrower than everything the client
// can ask for, and COMPATIBILITY.md is where that line is drawn rather than here.
//
// A constant rather than a linker flag. The commit stamp comes from the toolchain's own VCS
// record precisely so a release script cannot forget it, and a version that could be passed
// in is a version a script can get wrong.
const Version = "0.3.5"

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
