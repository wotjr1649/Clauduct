package main

import (
	"strings"
	"testing"
)

// usageCounts is the whole cache probe: whatever it fails to read, the probe reports as
// absent, and "the backend has no cache field" is the finding that would close the
// question. So it is read against frames rather than trusted, before anything is spent.
func TestUsageCountsReadsWhatTheBackendActuallySends(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  string
		want map[string]int64
	}{
		"what this build reads today": {
			`{"type":"response.completed","response":{"usage":{"input_tokens":5,"output_tokens":2}}}`,
			map[string]int64{"input_tokens": 5, "output_tokens": 2},
		},
		// The shape a cached count is most likely to arrive in, and the one a struct with
		// two int fields cannot see at all.
		"a nested cached count": {
			`{"type":"response.completed","response":{"usage":{"input_tokens":900,` +
				`"input_tokens_details":{"cached_tokens":768},"output_tokens":2}}}`,
			map[string]int64{"input_tokens": 900, "input_tokens_details.cached_tokens": 768,
				"output_tokens": 2},
		},
		"no usage object": {
			`{"type":"response.completed","response":{}}`, map[string]int64{},
		},
		// Non-integer members are skipped rather than failing the read: a usage object that
		// gains a string would otherwise hide every count beside it.
		"a member that is not a count": {
			`{"type":"response.completed","response":{"usage":{"input_tokens":5,"model":"x"}}}`,
			map[string]int64{"input_tokens": 5},
		},
		// This map is printed. A name the backend controls is not.
		"a field name that is not a field name": {
			`{"type":"response.completed","response":{"usage":{"IN-PUT":5,"ok_1":7}}}`,
			map[string]int64{"ok_1": 7},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := usageCounts([]byte(tc.raw))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for key, want := range tc.want {
				if got[key] != want {
					t.Errorf("%s = %d, want %d (full: %v)", key, got[key], want, got)
				}
			}
		})
	}
}

// The verdict is what a reader acts on, so each of its findings has to be reachable -- and
// "no field" has to stay distinguishable from "a field that read zero".
func TestTheCacheVerdictSeparatesAbsentFromZero(t *testing.T) {
	ok := func(usage map[string]int64) outcome { return outcome{ok: true, usage: usage} }

	for name, tc := range map[string]struct {
		cold, warm outcome
		want       string
	}{
		"nothing was measured": {outcome{}, outcome{}, "inconclusive"},
		"no field at all": {
			ok(map[string]int64{"input_tokens": 900}),
			ok(map[string]int64{"input_tokens": 900}),
			"no cache field at all",
		},
		"a field that never moved": {
			ok(map[string]int64{"input_tokens_details.cached_tokens": 0}),
			ok(map[string]int64{"input_tokens_details.cached_tokens": 0}),
			"stayed zero",
		},
		"a field that moved": {
			ok(map[string]int64{"input_tokens_details.cached_tokens": 0}),
			ok(map[string]int64{"input_tokens_details.cached_tokens": 768}),
			"discarding it",
		},
		// The shape actually measured 2026-09-18, and the one the first verdict read as
		// "stayed zero": the hit landed on the first call because its prefix overlapped an
		// earlier run. A non-zero anywhere in the pair closes the question.
		"a field that moved on the first call": {
			ok(map[string]int64{"input_tokens_details.cached_tokens": 3840}),
			ok(map[string]int64{"input_tokens_details.cached_tokens": 0}),
			"discarding it",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := cacheVerdict(tc.cold, tc.warm); !strings.Contains(got, tc.want) {
				t.Fatalf("verdict = %q, want it to say %q", got, tc.want)
			}
		})
	}
}

// Both requests have to carry the same bytes or there is nothing for a cache to hit.
func TestTheCachePrefixIsStableAndLongEnoughToBeWorthCaching(t *testing.T) {
	first, second := cachePrefix(cacheRepeats), cachePrefix(cacheRepeats)
	if first != second {
		t.Fatal("the prefix varies between calls; the second request cannot hit the first")
	}
	// Roughly four characters to a token, and every cache implementation known has a
	// minimum in the high hundreds of tokens. Short enough to be under it is a probe that
	// reports "no caching" about its own prompt size.
	if len(first) < 4*1024 {
		t.Fatalf("prefix is %d characters, likely under the backend's minimum", len(first))
	}
}
