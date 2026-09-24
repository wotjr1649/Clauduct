// Package update replaces this installation with the binaries a tag published.
//
// Why a tag rather than a commit: a release is a decision someone made, and an installation
// that follows the newest green commit follows work in progress. The v0.1.0 release already
// carries the shape this reads -- binaries beside a SHA256SUMS file -- so the mechanism is
// not new, only its subject.
//
// Why the digests matter more than usual: nothing is signed, so what stands behind a
// downloaded binary is HTTPS to GitHub plus a digest the release author published. The
// build is byte-reproducible, which is what makes that digest a claim anyone can check
// rather than a number only the publisher can produce.
//
// The order is the safety. Everything is fetched and verified before anything on disk is
// touched, so a failed download or a digest that does not match leaves the installation
// exactly as it was.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repository is where releases come from. A constant, not a setting: a build that can be
// pointed at another origin is one where "where did this binary come from" has no answer.
const Repository = "wotjr1649/Clauduct"

// API is the release endpoint. The parameter exists so a test can stand in for it.
const API = "https://api.github.com/repos/" + Repository + "/releases/latest"

// Timeout bounds one update run.
const Timeout = 3 * time.Minute

// Binaries are what an installation consists of: one file since v0.4.0, which is also the
// hook, the PDF renderer and Clauduct's own commands (#112).
var Binaries = []string{"clauduct.exe"}

// Retired are the names a 0.3.x installation had beside clauduct.exe. Releases through
// v0.4.x still publish them as byte-identical copies, because a 0.3.x updater refuses a
// release without all three; a copy takes the role its name says. An update removes them
// once it has replaced clauduct.exe, and an uninstall removes them with the rest.
var Retired = []string{"clauduct-hook.exe", "clauduct-dev.exe"}

// SumsAsset is where the release states its digests.
const SumsAsset = "SHA256SUMS"

// Bounds on what a confused or hostile server can make this read.
const (
	maxBinaryBytes = 64 << 20
	maxSumsBytes   = 64 << 10
)

// Refusals this package produces.
var (
	ErrNoRelease   = errors.New("NO_RELEASE")
	ErrMissingFile = errors.New("RELEASE_INCOMPLETE")
	ErrDigest      = errors.New("DIGEST_MISMATCH")
	// ErrRateLimited is GitHub refusing an unauthenticated read, which is a condition a
	// user will meet: the limit is per address and shared with everything else on it.
	// Reported as itself because "no release to update from" would send someone looking
	// for a release that is there.
	ErrRateLimited = errors.New("RATE_LIMITED")
)

// Asset is one published file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest is what the API says the bytes hash to, when it says anything. It is checked
	// against the release's own SHA256SUMS: two independent statements about one file, and
	// a disagreement between them is a reason to stop rather than to pick one.
	Digest string `json:"digest"`
}

// Release is what one tag published.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// Asset finds a published file by name.
func (r Release) Asset(name string) (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

// Missing reports which files this release does not carry.
func (r Release) Missing() []string {
	var missing []string
	for _, name := range append(append([]string{}, Binaries...), SumsAsset) {
		if _, ok := r.Asset(name); !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// Latest reads the newest release.
//
// The request carries no credential and no query. This is a public repository and an
// unauthenticated read is all it takes; sending a token would put one on the wire for
// nothing.
func Latest(ctx context.Context, client *http.Client, api string) (Release, error) {
	raw, err := fetch(ctx, client, api, maxSumsBytes*8)
	if err != nil {
		return Release{}, err
	}
	var release Release
	if err := json.Unmarshal(raw, &release); err != nil || release.Tag == "" {
		return Release{}, ErrNoRelease
	}
	return release, nil
}

// Fetch reads one small asset, such as the digests file.
func Fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	return fetch(ctx, client, url, maxSumsBytes)
}

// Sums reads a SHA256SUMS file into name to digest.
//
// The usual format: a hex digest, spaces, a name. A line this does not understand is
// skipped rather than guessed at, and a binary that ends up without a digest is refused by
// Download rather than quietly installed.
func Sums(raw []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out
}

// Download fetches every binary and checks it, returning nothing unless all of them pass.
func Download(ctx context.Context, client *http.Client, release Release,
	sums map[string]string) (map[string][]byte, error) {

	files := make(map[string][]byte, len(Binaries))
	for _, name := range Binaries {
		asset, ok := release.Asset(name)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrMissingFile, name)
		}
		want, ok := sums[name]
		if !ok {
			return nil, fmt.Errorf("%w: %s is not in %s", ErrMissingFile, name, SumsAsset)
		}
		body, err := fetch(ctx, client, asset.URL, maxBinaryBytes)
		if err != nil {
			return nil, err
		}
		got := Digest(body)
		if got != want {
			return nil, fmt.Errorf("%w: %s hashes to %s, %s says %s",
				ErrDigest, name, got, SumsAsset, want)
		}
		// The API's own digest, when it states one. Two sources agreeing is worth little;
		// two sources disagreeing is worth stopping for.
		if stated := strings.TrimPrefix(asset.Digest, "sha256:"); stated != "" && stated != got {
			return nil, fmt.Errorf("%w: %s hashes to %s and the release API says %s",
				ErrDigest, name, got, stated)
		}
		files[name] = body
	}
	return files, nil
}

// Digest is the lowercase hex sha256 of body.
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Apply replaces the binary in dir, and puts the old one back if the new one cannot be
// written.
//
// A running executable cannot be overwritten on Windows but it can be renamed, which is
// what makes this possible at all: the old file is moved aside, the new one is written in
// its place, and the process doing this keeps running from the file it renamed.
//
// One binary since v0.4.0 (#112). A second would need the multi-file restore this carried
// until then: replacing one and failing on the next must not leave a mixed installation.
func Apply(dir string, files map[string][]byte) (leftovers []string, err error) {
	name := Binaries[0]
	body, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrMissingFile, name)
	}
	path := filepath.Join(dir, name)
	backup := path + ".old"
	os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, body, 0o755); err != nil {
		os.Rename(backup, path)
		return nil, err
	}

	// The binary running this is the file just replaced, so its predecessor is still open
	// and cannot be removed until this process exits. Reported rather than retried: a
	// leftover is a file the user can delete, and a retry loop here would be waiting for
	// itself.
	if os.Remove(backup) != nil {
		leftovers = append(leftovers, backup)
	}

	// Only after clauduct.exe is the new one, so a failure above leaves a 0.3.x installation
	// whole. A copy a session started before the update is running as its hook right now
	// cannot go, and is reported the same way.
	for _, name := range Retired {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			leftovers = append(leftovers, path)
		}
	}
	return leftovers, nil
}

func fetch(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream, application/json")
	request.Header.Set("User-Agent", "clauduct-update")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if limited(response) {
			return nil, fmt.Errorf("%w: GitHub allows a limited number of unauthenticated "+
				"reads per address%s", ErrRateLimited, resetsIn(response))
		}
		return nil, fmt.Errorf("%s: HTTP %d", url, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", url, limit)
	}
	return body, nil
}

// limited reports whether a refusal is the rate limit rather than something about the
// request. The status alone does not say: 403 is also what a private repository returns.
func limited(response *http.Response) bool {
	if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return false
	}
	return response.Header.Get("X-RateLimit-Remaining") == "0"
}

// resetsIn turns the reset header into something a person can act on, or into nothing when
// the server did not say.
func resetsIn(response *http.Response) string {
	seconds, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return ""
	}
	wait := time.Until(time.Unix(seconds, 0)).Round(time.Minute)
	if wait <= 0 {
		return "; try again now"
	}
	return "; try again in " + wait.String()
}
