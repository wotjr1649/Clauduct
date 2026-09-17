//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func files(paths ...string) func(string) bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[strings.ToLower(filepath.Clean(p))] = true
	}
	return func(path string) bool { return set[strings.ToLower(filepath.Clean(path))] }
}

// The standalone install wins even when PATH also offers one, so resolution does not
// change because a user reordered PATH.
func TestStandaloneInstallPreferred(t *testing.T) {
	home := `C:\Users\dev`
	standalone := filepath.Join(home, ".local", "bin", "claude.exe")
	onPath := `C:\tools\claude.exe`

	got, found, err := Resolver{
		Home:   home,
		Cwd:    `C:\work`,
		Env:    map[string]string{"PATH": `C:\tools`},
		IsFile: files(standalone, onPath),
	}.Claude()

	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got != standalone {
		t.Fatalf("resolved %q, want the standalone install %q", got, standalone)
	}
}

// PATH is the fallback, and its name may be cased any way Windows likes.
func TestPathIsSearchedCaseInsensitively(t *testing.T) {
	for _, name := range []string{"PATH", "Path", "path", "pAtH"} {
		want := `C:\tools\claude.exe`
		got, found, err := Resolver{
			Home:   `C:\Users\dev`,
			Cwd:    `C:\work`,
			Env:    map[string]string{name: `C:\tools`},
			IsFile: files(want),
		}.Claude()
		if err != nil || !found || got != want {
			t.Errorf("%s: got %q found=%v err=%v", name, got, found, err)
		}
	}
}

// The working directory is excluded from the search. Otherwise a claude.exe that merely
// happens to sit beside the user's project is treated as an installed tool.
func TestWorkingDirectoryIsNeverASource(t *testing.T) {
	cwd := `C:\work\project`
	planted := filepath.Join(cwd, "claude.exe")

	got, found, err := Resolver{
		Home:   `C:\Users\dev`,
		Cwd:    cwd,
		Env:    map[string]string{"PATH": cwd + `;C:\work\project\`},
		IsFile: files(planted),
	}.Claude()

	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if found {
		t.Fatalf("resolved %q from the working directory", got)
	}
}

// A relative PATH entry resolves against whatever the process happens to be doing, so it
// is not a trustworthy install location.
//
// IsFile says yes to everything except the standalone install. Answering yes to everything
// would let the standalone hit first and the test would pass without ever reaching a
// relative entry, which is how its first version passed for the wrong reason.
func TestRelativePathEntriesIgnored(t *testing.T) {
	notStandalone := func(path string) bool { return !strings.Contains(path, ".local") }

	got, found, err := Resolver{
		Home:   `C:\Users\dev`,
		Cwd:    `C:\work`,
		Env:    map[string]string{"PATH": `.;..\bin;bin;.\tools`},
		IsFile: notStandalone,
	}.Claude()

	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if found {
		t.Fatalf("a relative PATH entry was accepted as an install location: %q", got)
	}
}

// Quoted entries appear in real PATH values and have to be unwrapped, not skipped.
func TestQuotedPathEntriesUnwrapped(t *testing.T) {
	want := `C:\Program Files\claude\claude.exe`
	got, found, err := Resolver{
		Home:   `C:\Users\dev`,
		Cwd:    `C:\work`,
		Env:    map[string]string{"PATH": `"C:\Program Files\claude"`},
		IsFile: files(want),
	}.Claude()

	if err != nil || !found || got != want {
		t.Fatalf("got %q found=%v err=%v", got, found, err)
	}
}

// The baseline caps the raw entries before filtering. Matching the order of operations
// keeps both implementations resolving the same executable on a long PATH.
func TestRawPathEntriesCappedAtSixtyFour(t *testing.T) {
	entries := make([]string, 0, 70)
	for i := 0; i < 70; i++ {
		entries = append(entries, `C:\d`+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	beyond := filepath.Join(entries[69], "claude.exe")
	within := filepath.Join(entries[63], "claude.exe")

	_, found, err := Resolver{
		Home: `C:\Users\dev`, Cwd: `C:\work`,
		Env:    map[string]string{"PATH": strings.Join(entries, ";")},
		IsFile: files(beyond),
	}.Claude()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if found {
		t.Error("an entry past the 64th was searched")
	}

	_, found, err = Resolver{
		Home: `C:\Users\dev`, Cwd: `C:\work`,
		Env:    map[string]string{"PATH": strings.Join(entries, ";")},
		IsFile: files(within),
	}.Claude()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !found {
		t.Error("the 64th entry was not searched")
	}
}

// When nothing is found the caller still gets the standalone location to report, and a
// found=false it must not spawn.
func TestNotFoundStillNamesWhereItLooked(t *testing.T) {
	home := `C:\Users\dev`
	got, found, err := Resolver{
		Home: home, Cwd: `C:\work`,
		Env:    map[string]string{"PATH": `C:\tools`},
		IsFile: func(string) bool { return false },
	}.Claude()

	if err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if want := filepath.Join(home, ".local", "bin", "claude.exe"); got != want {
		t.Fatalf("reported %q, want %q", got, want)
	}
}

// The measured reason OSUserHome does not call os.UserHomeDir. Poisoning USERPROFILE must
// not move where Clauduct looks for the executable it is about to run.
func TestOSUserHomeIgnoresUserProfileOverride(t *testing.T) {
	real, err := OSUserHome()
	if err != nil {
		t.Skipf("no OS user home available: %v", err)
	}
	t.Setenv("USERPROFILE", `D:\attacker\controlled`)

	poisoned, err := OSUserHome()
	if err != nil {
		t.Fatalf("OSUserHome after override: %v", err)
	}
	if poisoned != real {
		t.Fatalf("home moved to %q when USERPROFILE was set; executable resolution is redirectable", poisoned)
	}
	if viaStdlib, err := os.UserHomeDir(); err == nil && viaStdlib == real {
		t.Skip("os.UserHomeDir did not follow the override on this build; the guard is still correct but this run did not exercise the difference")
	}
}
