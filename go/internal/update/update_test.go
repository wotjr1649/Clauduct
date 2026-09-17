package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// release stands in for GitHub: the metadata endpoint, the digests file and the binaries.
//
// Written as a server rather than as injected values so the test drives the same code the
// product does, request bounds and status handling included.
type release struct {
	tag     string
	bodies  map[string][]byte
	sums    string // overrides the computed SHA256SUMS when set
	digests map[string]string
}

func (r release) serve(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	sums := r.sums
	if sums == "" {
		var b strings.Builder
		for name, body := range r.bodies {
			fmt.Fprintf(&b, "%s  %s\n", Digest(body), name)
		}
		sums = b.String()
	}
	bodies := map[string][]byte{SumsAsset: []byte(sums)}
	for name, body := range r.bodies {
		bodies[name] = body
	}

	assets := make([]Asset, 0, len(bodies))
	for name := range bodies {
		asset := Asset{Name: name, URL: server.URL + "/a/" + name, Size: int64(len(bodies[name]))}
		if stated, ok := r.digests[name]; ok {
			asset.Digest = stated
		}
		assets = append(assets, asset)
	}
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{Tag: r.tag, Assets: assets})
	})
	mux.HandleFunc("/a/", func(w http.ResponseWriter, req *http.Request) {
		body, ok := bodies[strings.TrimPrefix(req.URL.Path, "/a/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	})
	return server, server.URL + "/latest"
}

// installation lays out a directory that looks like an install and returns it.
func installation(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range Binaries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents+name), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func newBodies(contents string) map[string][]byte {
	out := make(map[string][]byte, len(Binaries))
	for _, name := range Binaries {
		out[name] = []byte(contents + name)
	}
	return out
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// The option is recognised only where it cannot be a prompt.
//
// This is the difference between a wrong answer and a replaced installation: every argument
// is a string, and a session whose prompt happens to contain the option name must stay a
// session.
func TestTheUpdateOptionIsOnlyTheFirstArgument(t *testing.T) {
	for _, c := range []struct {
		name       string
		args       []string
		want, cons bool
	}{
		{"alone", []string{"--update"}, true, false},
		{"with consent", []string{"--update", "--yes"}, true, true},
		{"case", []string{"--UPDATE"}, true, false},
		{"inside a prompt", []string{"-p", "how do I --update this"}, false, false},
		{"after other arguments", []string{"-p", "hi", "--update"}, false, false},
		{"with anything else", []string{"--update", "-p", "hi"}, false, false},
		{"attached value", []string{"--update=now"}, false, false},
		{"nothing", nil, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, consented := Requested(c.args)
			if got != c.want || consented != c.cons {
				t.Fatalf("Requested(%q) = (%v, %v), want (%v, %v)",
					c.args, got, consented, c.want, c.cons)
			}
		})
	}
}

// A digest that does not match stops the update before anything is written.
func TestAMismatchedDigestChangesNothing(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{
		tag:    "v9.9.9",
		bodies: newBodies("new "),
		sums: "0000000000000000000000000000000000000000000000000000000000000000  clauduct.exe\n" +
			"0000000000000000000000000000000000000000000000000000000000000000  clauduct-hook.exe\n" +
			"0000000000000000000000000000000000000000000000000000000000000000  clauduct-dev.exe\n",
	}.serve(t)
	defer server.Close()

	var out strings.Builder
	if code := RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update", "--yes"}, strings.NewReader(""), &out); code == 0 {
		t.Fatalf("a mismatched digest was accepted: %s", out.String())
	}
	for _, name := range Binaries {
		if got := read(t, dir, name); got != "old "+name {
			t.Fatalf("%s was replaced despite the digest: %q", name, got)
		}
	}
	if !strings.Contains(out.String(), "was not changed") {
		t.Fatalf("the refusal does not say the installation is intact: %s", out.String())
	}
}

// Two statements about the same file, disagreeing, stop it too.
//
// SHA256SUMS is the release author's claim and the API digest is GitHub's. Agreement proves
// little, but a disagreement means one of them is describing a file that is not there.
func TestADisagreementBetweenTheTwoDigestsStops(t *testing.T) {
	dir := installation(t, "old ")
	bodies := newBodies("new ")
	server, api := release{
		tag:     "v9.9.9",
		bodies:  bodies,
		digests: map[string]string{"clauduct.exe": "sha256:" + Digest([]byte("something else"))},
	}.serve(t)
	defer server.Close()

	var out strings.Builder
	if code := RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update", "--yes"}, strings.NewReader(""), &out); code == 0 {
		t.Fatalf("the two sources disagreed and the update went ahead: %s", out.String())
	}
	if got := read(t, dir, "clauduct.exe"); got != "old clauduct.exe" {
		t.Fatalf("clauduct.exe was replaced: %q", got)
	}
}

// Without consent, nothing is downloaded and nothing is written.
func TestWithoutConsentNothingIsReplaced(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{tag: "v9.9.9", bodies: newBodies("new ")}.serve(t)
	defer server.Close()

	var out strings.Builder
	code := RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update"}, strings.NewReader("n\n"), &out)
	if code != 0 {
		t.Fatalf("declining is not a failure, got %d: %s", code, out.String())
	}
	for _, name := range Binaries {
		if got := read(t, dir, name); got != "old "+name {
			t.Fatalf("%s was replaced after a refusal: %q", name, got)
		}
	}
	// And the answer was asked for with the facts in hand: the tag and every digest.
	printed := out.String()
	if !strings.Contains(printed, "v9.9.9") {
		t.Fatalf("the tag was not shown: %s", printed)
	}
	for _, name := range Binaries {
		if !strings.Contains(printed, Digest([]byte("new "+name))) {
			t.Fatalf("the digest of %s was not shown before asking: %s", name, printed)
		}
	}
}

// End of input is not consent.
func TestEndOfInputIsNotConsent(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{tag: "v9.9.9", bodies: newBodies("new ")}.serve(t)
	defer server.Close()

	var out strings.Builder
	RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update"}, strings.NewReader(""), &out)
	if got := read(t, dir, "clauduct.exe"); got != "old clauduct.exe" {
		t.Fatalf("an update proceeded with nobody there to approve it: %q", got)
	}
}

// The whole path, consented: all three replaced together.
func TestAConsentedUpdateReplacesAllThree(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{tag: "v9.9.9", bodies: newBodies("new ")}.serve(t)
	defer server.Close()

	var out strings.Builder
	if code := RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update", "--yes"}, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("code = %d: %s", code, out.String())
	}
	for _, name := range Binaries {
		if got := read(t, dir, name); got != "new "+name {
			t.Fatalf("%s holds %q", name, got)
		}
	}
	if !strings.Contains(out.String(), "updated to v9.9.9") {
		t.Fatalf("the result was not reported: %s", out.String())
	}
}

// A release with no binaries in it is named, not guessed at.
//
// This is the state of the repository's only published release today: it carries the Node
// implementation's archive and no Go binaries, so an update against it has to say what is
// missing rather than fail as though something went wrong.
func TestAReleaseWithoutBinariesSaysWhichAreMissing(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{tag: "v0.1.0", bodies: map[string][]byte{
		"Clauduct-windows-x64.zip": []byte("node"),
	}}.serve(t)
	defer server.Close()

	var out strings.Builder
	if code := RunIn(context.Background(), server.Client(), api, dir,
		[]string{"--update", "--yes"}, strings.NewReader(""), &out); code == 0 {
		t.Fatal("an incomplete release was treated as installable")
	}
	for _, name := range Binaries {
		if !strings.Contains(out.String(), name) {
			t.Fatalf("%s is missing from the report: %s", name, out.String())
		}
	}
	if got := read(t, dir, "clauduct.exe"); got != "old clauduct.exe" {
		t.Fatalf("clauduct.exe was touched: %q", got)
	}
}

// A replacement that fails part way puts back what it moved.
func TestAFailedReplacementRestoresWhatItMoved(t *testing.T) {
	dir := installation(t, "old ")
	// The third file is missing from the set, so Apply fails after replacing two.
	files := map[string][]byte{Binaries[0]: []byte("new one"), Binaries[1]: []byte("new two")}

	if _, err := Apply(dir, files); err == nil {
		t.Fatal("an incomplete set was applied")
	}
	for _, name := range Binaries {
		if got := read(t, dir, name); got != "old "+name {
			t.Fatalf("%s was left as %q after a failed update", name, got)
		}
	}
}

// Digest lines this does not understand are skipped rather than guessed at.
func TestSumsReadsWhatItUnderstands(t *testing.T) {
	digest := Digest([]byte("x"))
	sums := Sums([]byte(
		digest + "  clauduct.exe\n" +
			"not-a-digest  clauduct-hook.exe\n" +
			strings.ToUpper(digest) + " *clauduct-dev.exe\n" +
			"\n# a comment\n"))
	if sums["clauduct.exe"] != digest {
		t.Fatalf("clauduct.exe = %q", sums["clauduct.exe"])
	}
	if _, ok := sums["clauduct-hook.exe"]; ok {
		t.Fatal("a line with no digest produced an entry")
	}
	// Uppercase and the binary-mode asterisk are the same file.
	if sums["clauduct-dev.exe"] != digest {
		t.Fatalf("clauduct-dev.exe = %q", sums["clauduct-dev.exe"])
	}
}

// The rate limit is reported as itself.
//
// Measured against the real endpoint on 2026-09-17: an unauthenticated read returned 403
// with x-ratelimit-remaining 0, and the first version of this command called that "no
// release to update from" -- sending a reader to look for a release that is published and
// fine. The limit is per address, so it is shared with everything else on the machine.
func TestTheRateLimitIsNamedRatherThanCalledAMissingRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(20*time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := Latest(context.Background(), server.Client(), server.URL)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want %v", err, ErrRateLimited)
	}
	if !strings.Contains(err.Error(), "try again in") {
		t.Fatalf("the reader is not told when to come back: %v", err)
	}

	// A 403 that is not the limit stays what it is: a private repository answers the same
	// status and must not be reported as a rate limit.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer other.Close()
	if _, err := Latest(context.Background(), other.Client(), other.URL); errors.Is(err, ErrRateLimited) {
		t.Fatalf("a plain 403 was reported as a rate limit: %v", err)
	}
}

// The command does not call a rate limit a missing release.
func TestTheCommandReportsTheLimitAsItself(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(9*time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	var out strings.Builder
	RunIn(context.Background(), server.Client(), server.URL, t.TempDir(),
		[]string{"--update"}, strings.NewReader(""), &out)
	if strings.Contains(out.String(), "no release") {
		t.Fatalf("a rate limit was reported as a missing release: %s", out.String())
	}
	if !strings.Contains(out.String(), "RATE_LIMITED") {
		t.Fatalf("the limit was not named: %s", out.String())
	}
}

// Reading the digests does not spend the download's time budget.
//
// Found by review: one deadline covered the whole command, prompt included, so a reader who
// took longer than it to check three digests pressed y and was answered with a context
// deadline. Printing them and then punishing someone for reading them is the wrong way
// round.
//
// The deadline here is short and the answer is slow, which puts the expiry exactly where a
// person would have put it: after the metadata and the digests, before the download.
func TestTheTimeSpentDecidingIsNotChargedToTheDownload(t *testing.T) {
	dir := installation(t, "old ")
	server, api := release{tag: "v9.9.9", bodies: newBodies("new ")}.serve(t)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var out strings.Builder
	if code := RunIn(ctx, server.Client(), api, dir,
		[]string{"--update"}, &slowReader{after: 150 * time.Millisecond, answer: "y" + "\n"}, &out); code != 0 {
		t.Fatalf("code = %d, want the update to proceed after a slow yes: %s", code, out.String())
	}
	for _, name := range Binaries {
		if got := read(t, dir, name); got != "new "+name {
			t.Fatalf("%s holds %q", name, got)
		}
	}
}

// slowReader answers once, after a wait, the way a person reading three digests does.
type slowReader struct {
	after  time.Duration
	answer string
	done   bool
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	time.Sleep(r.after)
	r.done = true
	return copy(p, r.answer), nil
}
