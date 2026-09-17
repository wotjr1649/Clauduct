package update

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/buildinfo"
)

// Option is what this command owns on the command line.
//
// Two names, matched exactly, neither of which takes a value -- the same rule the refusal
// list uses, and for the same reason: recognising an option that consumes the next argument
// means tracking which options do, which is the parser this launcher does not have and must
// not grow. Everything else still goes to the native client untouched.
//
// --update rather than a bare `update`: the client has its own `update` subcommand, so a
// bare word here would take it over and there would be no way left to reach it. Measured on
// 2.1.274: the client defines no --update option, so this name collides with nothing and
// `clauduct update` still updates Claude Code.
const (
	Option  = "--update"
	Consent = "--yes"
)

// Requested reports whether this invocation is an update, and whether it may proceed
// without asking.
//
// The option has to come first and nothing may follow it but --yes. Scanning the whole
// argument vector instead would turn `clauduct -p "how do I --update"` into a binary
// replacement, because a prompt is an argument like any other. The refusal list can afford
// to over-match -- it refuses, and the user rewords -- but this one writes files.
func Requested(args []string) (update, consented bool) {
	if len(args) == 0 || !strings.EqualFold(args[0], Option) {
		return false, false
	}
	for _, arg := range args[1:] {
		if !strings.EqualFold(arg, Consent) {
			return false, false
		}
		consented = true
	}
	return true, consented
}

// Run performs the update and returns a process exit code.
//
// What it prints before doing anything is the whole point of asking: which build is running,
// which tag would replace it, and the digest of every file that would be written. A consent
// given without those is consent to something unnamed.
func Run(ctx context.Context, args []string, in io.Reader, out io.Writer) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(out, "clauduct: cannot locate this executable:", err)
		return 1
	}
	return RunIn(ctx, &http.Client{Timeout: Timeout}, API, filepath.Dir(self), args, in, out)
}

// RunIn is Run with the origin and the installation directory supplied.
//
// Separated so a test can drive the whole path -- release, digests, consent, replacement --
// without reaching the network or writing beside the test binary.
func RunIn(ctx context.Context, client *http.Client, api, dir string, args []string,
	in io.Reader, out io.Writer) int {

	_, consented := Requested(args)
	running := buildinfo.Read()

	release, err := Latest(ctx, client, api)
	if err != nil {
		fmt.Fprintln(out, "clauduct: no release to update from:", err)
		return 1
	}

	fmt.Fprintln(out, "installed", dir)
	fmt.Fprintln(out, "running  ", running.Version, "commit", running.CommitOrUnknown())
	fmt.Fprintln(out, "release  ", release.Tag, "from", Repository)

	if missing := release.Missing(); len(missing) > 0 {
		// Named rather than summarised. A release that carries a source archive and no
		// binaries is the normal state of a project that has not published one yet, and the
		// user can see that from the list.
		fmt.Fprintln(out, "clauduct: release", release.Tag, "does not carry",
			strings.Join(missing, ", "))
		return 1
	}

	sums, err := digests(ctx, client, release)
	if err != nil {
		fmt.Fprintln(out, "clauduct: cannot read", SumsAsset+":", err)
		return 1
	}
	for _, name := range Binaries {
		fmt.Fprintln(out, "  ", name, sums[name])
	}

	if !consented && !confirmed(in, out) {
		fmt.Fprintln(out, "clauduct: nothing was changed")
		return 0
	}

	files, err := Download(ctx, client, release, sums)
	if err != nil {
		// Nothing has been touched at this point, and saying so is worth a line: a failed
		// update reads as a broken installation unless it says otherwise.
		fmt.Fprintln(out, "clauduct: update refused:", err)
		fmt.Fprintln(out, "          the installation was not changed")
		return 1
	}

	leftovers, err := Apply(dir, files)
	if err != nil {
		fmt.Fprintln(out, "clauduct: update failed:", err)
		fmt.Fprintln(out, "          the previous binaries were put back")
		return 1
	}

	fmt.Fprintln(out, "updated to", release.Tag)
	for _, path := range leftovers {
		// The running binary holds its own predecessor open until this process exits.
		fmt.Fprintln(out, "leftover ", path, "(delete after this process exits)")
	}
	return 0
}

// digests reads the release's SHA256SUMS.
func digests(ctx context.Context, client *http.Client, release Release) (map[string]string, error) {
	asset, ok := release.Asset(SumsAsset)
	if !ok {
		return nil, ErrMissingFile
	}
	raw, err := Fetch(ctx, client, asset.URL)
	if err != nil {
		return nil, err
	}
	sums := Sums(raw)
	for _, name := range Binaries {
		if sums[name] == "" {
			return nil, fmt.Errorf("%w: %s has no line for %s", ErrMissingFile, SumsAsset, name)
		}
	}
	return sums, nil
}

// confirmed asks once. Anything that is not a yes is a no, including end of input: an
// update that proceeds because nobody was there to object is not one anybody approved.
func confirmed(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "replace these three files? [y/N] ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		fmt.Fprintln(out)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
