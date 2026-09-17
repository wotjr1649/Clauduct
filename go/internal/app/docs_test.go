package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// REL09: a citation has to point at what it says it points at.
//
// This lived as a script in a scratch directory for most of the redesign, which meant it ran
// when somebody remembered. A check that runs when somebody remembers is not a check, and
// the design documents are the evidence record -- a citation that has drifted is a claim
// with nothing behind it.
//
// It lives in this package because moduleRoot is here and because internal/app already
// holds the other repository-wide scan. A package of its own would be a directory for one
// file.

// citation matches `path/to/file.ext:12` and `path/to/file.ext:12-20` in backticks.
var citation = regexp.MustCompile(
	"`((?:src|verification|poc|bin|docs|go|\\.github)/[A-Za-z0-9._/-]+\\.[A-Za-z0-9]+):(\\d+)(?:-(\\d+))?`")

// link matches an ordinary markdown link target.
var link = regexp.MustCompile(`\[[^\]\n]*\]\(\s*(?:<([^>\n]*)>|([^)<>\s]+))(?:\s+"[^"\n]*")?\s*\)`)

// fenced code is stripped before scanning: an example of a citation is not a citation.
var fenced = regexp.MustCompile("(?s)```.*?```")

func TestDesignDocumentCitationsPointAtWhatTheyClaim(t *testing.T) {
	repo := filepath.Dir(moduleRoot(t))
	docs, err := filepath.Glob(filepath.Join(repo, "docs", "v2", "*.md"))
	if err != nil || len(docs) == 0 {
		t.Fatalf("no design documents found under docs/v2: %v", err)
	}
	tracked := trackedFiles(t, repo)

	citations, links := 0, 0
	for _, doc := range docs {
		raw, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		text := fenced.ReplaceAllString(string(raw), "")
		name := filepath.Base(doc)

		for _, match := range citation.FindAllStringSubmatch(text, -1) {
			citations++
			checkCitation(t, repo, tracked, name, match)
		}
		for _, match := range link.FindAllStringSubmatch(text, -1) {
			target := match[1]
			if target == "" {
				target = match[2]
			}
			// A URL or an anchor is not this test's business.
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
				continue
			}
			links++
			path := filepath.Join(filepath.Dir(doc), strings.SplitN(target, "#", 2)[0])
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s links to %q, which does not exist", name, target)
			}
		}
	}
	if citations == 0 {
		t.Fatal("no citations found; the pattern has stopped matching what the documents write")
	}
	t.Logf("checked %d citations and %d links across %d documents", citations, links, len(docs))
}

func checkCitation(t *testing.T, repo string, tracked map[string]bool, doc string, match []string) {
	t.Helper()
	target, from := match[1], match[2]
	to := match[3]
	if to == "" {
		to = from
	}

	if tracked != nil && !tracked[target] {
		t.Errorf("%s cites %s, which is not tracked; an untracked file can vanish "+
			"without the citation noticing", doc, target)
		return
	}
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(target)))
	if err != nil {
		t.Errorf("%s cites %s: %v", doc, target, err)
		return
	}
	lines := strings.Split(string(raw), "\n")

	first, _ := strconv.Atoi(from)
	last, _ := strconv.Atoi(to)
	if first < 1 || last < first || last > len(lines) {
		t.Errorf("%s cites %s:%s-%s but the file has %d lines", doc, target, from, to, len(lines))
		return
	}
	// A citation landing on a blank line is one that has drifted: whatever it meant to
	// point at has moved, and the number is no longer evidence of anything.
	for _, n := range []int{first, last} {
		if strings.TrimSpace(lines[n-1]) == "" {
			t.Errorf("%s cites %s:%d, which is a blank line", doc, target, n)
		}
	}
}

// trackedFiles lists what git knows about, or nil when git cannot answer. Nil means the
// tracked check is skipped rather than silently passing everything.
func trackedFiles(t *testing.T, repo string) map[string]bool {
	t.Helper()
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = repo
	raw, err := cmd.Output()
	if err != nil {
		t.Logf("git ls-files unavailable (%v); citations are checked on disk only", err)
		return nil
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out[line] = true
		}
	}
	return out
}
