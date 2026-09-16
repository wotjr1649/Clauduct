package gateway

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// withBetas is an inference request carrying an anthropic-beta header.
func withBetas(header string) request {
	rq := messages(strings.NewReader(`{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`))
	rq.headers["Anthropic-Beta"] = header
	return rq
}

// measuredBetas is what a real session sends, read off the wire on 2026-09-16.
const measuredBetas = "claude-code-20250219,interleaved-thinking-2025-05-14," +
	"thinking-token-count-2026-05-13,context-management-2025-06-27," +
	"prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07," +
	"mid-conversation-tool-changes-2026-07-01,effort-2025-11-24"

// D4. An ordinary session reports nothing, because everything it asks for works.
//
// This is the guard on the list itself. A name the client sends that drifts out of
// nativeBetas would show up here as an unknown on every single request, which is a
// diagnostic that cries wolf and stops being read.
func TestAnOrdinarySessionAsksForNothingUnusual(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	if resp := do(t, g, withBetas(measuredBetas)); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}

	report := status(t, g).Betas
	if report.Requests != 1 {
		t.Errorf("requests = %d", report.Requests)
	}
	if report.Malformed != 0 || len(report.Judged) != 0 ||
		len(report.Unknown) != 0 || len(report.ServerDependent) != 0 {
		t.Fatalf("a real session's own header was reported as unusual: %+v", report)
	}
}

// What a beta lands in, and what a reader gets told about it.
func TestABetaIsClassifiedAndNeverRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		header string
		want   BetaReport
	}{
		"one this project judged": {
			header: "claude-code-20250219,structured-outputs-2025-12-15",
			want:   BetaReport{Requests: 1, Judged: []string{"STRUCTURED_OUTPUTS"}},
		},
		"one that needs Anthropic's own servers": {
			header: "message-batches-2024-09-24",
			want: BetaReport{Requests: 1,
				ServerDependent: []string{"message-batches-2024-09-24"}},
		},
		"one nobody here has classified": {
			header: "some-future-beta-2027-01-01",
			want:   BetaReport{Requests: 1, Unknown: []string{"some-future-beta-2027-01-01"}},
		},
		"an empty item": {
			header: "claude-code-20250219,,effort-2025-11-24",
			want:   BetaReport{Requests: 1, Malformed: 1},
		},
		"the same name twice": {
			header: "claude-code-20250219,claude-code-20250219",
			want:   BetaReport{Requests: 1, Malformed: 1},
		},
		"something that is not a name at all": {
			header: "NOT A BETA NAME <script>",
			want:   BetaReport{Requests: 1, Malformed: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{
				SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
			})
			// Whatever the header says, the turn goes through. Refusing a beta name was
			// measured killing WebFetch in a real session, and refusing a malformed header
			// would kill a turn over text that changes no decision here.
			resp := do(t, g, withBetas(tc.header))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d for %q: %s", resp.StatusCode, tc.header, bodyText(t, resp))
			}

			got := status(t, g).Betas
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("report = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A name that is not shaped like one is never written down.
//
// The header is the client's own text and this record goes into a file that outlives the
// session. Whatever arrives, what leaves is a fixed label, a bounded name or a count.
func TestAnUnusableNameIsCountedAndNotRecorded(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	for _, header := range []string{
		"UPPERCASE-2026-01-01",
		"1-starts-with-a-digit",
		"has spaces in it",
		strings.Repeat("a", 300),
		"looks/like/a/path",
	} {
		do(t, g, withBetas(header))
	}

	resp := do(t, g, request{method: http.MethodGet, path: statusPath})
	body := bodyText(t, resp)
	for _, header := range []string{"UPPERCASE", "starts-with-a-digit", "has spaces",
		strings.Repeat("a", 300), "looks/like/a/path"} {
		if strings.Contains(body, header) {
			t.Errorf("the account wrote down %q", header)
		}
	}
	if report := status(t, g).Betas; report.Malformed != 5 || len(report.Unknown) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

// The bound holds, and it keeps the names the session met first.
//
// Evicting to make room would give a reader a window that slid rather than an answer, and
// the first unknown a session meets is the one worth having.
func TestTheUnknownListIsBounded(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	for i := 0; i < betaNames+3; i++ {
		do(t, g, withBetas(fmt.Sprintf("unknown-beta-%d-2027-01-01", i)))
	}

	report := status(t, g).Betas
	if len(report.Unknown) != betaNames {
		t.Fatalf("unknown = %d names, want %d: %v", len(report.Unknown), betaNames, report.Unknown)
	}
	if !strings.Contains(strings.Join(report.Unknown, " "), "unknown-beta-0-") {
		t.Errorf("the first name the session met was dropped: %v", report.Unknown)
	}
	for i := betaNames; i < betaNames+3; i++ {
		if strings.Contains(strings.Join(report.Unknown, " "), fmt.Sprintf("unknown-beta-%d-", i)) {
			t.Errorf("the list grew past its bound: %v", report.Unknown)
		}
	}
}

// A request with no header at all is not a request that asked for nothing.
func TestNoHeaderIsNotAnObservation(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created, delta("ok"), done("ok"), completed, "[DONE]"),
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	if report := status(t, g).Betas; report.Requests != 0 {
		t.Fatalf("report = %+v for a request that carried no header", report)
	}
}

// Every list is disjoint, and every judged label is a label rather than the name.
func TestTheBetaListsDoNotOverlap(t *testing.T) {
	for name := range nativeBetas {
		if judgedBetas[name] != "" || serverDependentBetas[name] {
			t.Errorf("%s is in more than one list", name)
		}
	}
	for name, label := range judgedBetas {
		if serverDependentBetas[name] {
			t.Errorf("%s is in more than one list", name)
		}
		if label == name || !betaNameShape.MatchString(name) {
			t.Errorf("%s -> %q: the label must be a label, not the header text", name, label)
		}
	}
	// The gate this build actually reads has to be a name it also considers workable.
	// Otherwise every request that enables tool changes reports asking for something odd.
	if !nativeBetas[betaToolChanges] {
		t.Errorf("%s is the beta this build reads and it is not in nativeBetas", betaToolChanges)
	}
}
