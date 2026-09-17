package gateway

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/upstream"
)

// unknownEvent is a response that stops on an event this build does not know.
func unknownEvent(eventType string) string {
	return sse(created, `{"type":"`+eventType+`"}`, completed, "[DONE]")
}

// D6. A response stopped by an unreadable event says which one.
//
// This is the failure whose cause is a name and whose fix is one constant. It has already
// happened once: a request carrying a tool definition was refused before any call came back
// because the function-call argument events were not in the list, and finding that took a
// live session. With the name in the account it takes one line of output.
func TestAnUnreadableEventIsNamed(t *testing.T) {
	g := startWith(t, &upstream.Fixture{SSE: unknownEvent("response.something_new.added")})
	resp := post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("an event this build cannot read was passed over; the unread one may be " +
			"the one carrying the result")
	}

	report := status(t, g).Events
	if report.Unsupported != 1 {
		t.Fatalf("unsupported = %d", report.Unsupported)
	}
	if strings.Join(report.Names, ",") != "response.something_new.added" {
		t.Fatalf("names = %v, want the one that stopped it", report.Names)
	}
	if len(report.Formats) != 0 {
		t.Errorf("formats = %v; a named event is already the better answer and counting it "+
			"again would make the totals disagree with the list", report.Formats)
	}
}

// What can be written down, and what gets a label instead.
func TestAnEventTypeIsJudgedBeforeItIsRecorded(t *testing.T) {
	for name, tc := range map[string]struct{ eventType, format, recorded string }{
		"an ordinary name":         {"response.output_text.refusal", bridge.EventNamed, "response.output_text.refusal"},
		"a name with no dots":      {"message_start", bridge.EventNamed, "message_start"},
		"five segments":            {"a.b.c.d.e", bridge.EventNamed, "a.b.c.d.e"},
		"six segments":             {"a.b.c.d.e.f", bridge.EventOther, ""},
		"a segment past its bound": {strings.Repeat("a", 25) + ".b", bridge.EventOther, ""},
		"longer than a name is":    {strings.Repeat("a.", 30) + "b", bridge.EventOversized, ""},
		"upper case":               {"Response.Created", bridge.EventOther, ""},
		"a url":                    {"https://example.invalid/x", bridge.EventOther, ""},
	} {
		t.Run(name, func(t *testing.T) {
			refusal, ok := bridgeRefusal(t, tc.eventType)
			if !ok {
				t.Fatalf("%q was not refused", tc.eventType)
			}
			if refusal.Format != tc.format {
				t.Errorf("format = %q, want %q", refusal.Format, tc.format)
			}
			if refusal.Name != tc.recorded {
				t.Errorf("name = %q, want %q", refusal.Name, tc.recorded)
			}
			// The invariant that makes the account readable: a name is recorded exactly
			// when the format says one was.
			if (refusal.Name != "") != (refusal.Format == bridge.EventNamed) {
				t.Errorf("name=%q format=%q disagree", refusal.Name, refusal.Format)
			}
		})
	}
}

// bridgeRefusal runs one event through the translator and returns the refusal.
func bridgeRefusal(t *testing.T, eventType string) (*bridge.UnsupportedEvent, bool) {
	t.Helper()
	g := startWith(t, &upstream.Fixture{SSE: unknownEvent(eventType)})
	resp := post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	if resp.StatusCode == http.StatusOK {
		return nil, false
	}
	report := status(t, g).Events
	if report.Unsupported != 1 {
		return nil, false
	}
	refusal := &bridge.UnsupportedEvent{}
	if len(report.Names) == 1 {
		refusal.Name, refusal.Format = report.Names[0], bridge.EventNamed
		return refusal, true
	}
	for label := range report.Formats {
		refusal.Format = label
	}
	return refusal, refusal.Format != ""
}

// The name is the backend's own string, and it reaches a file that outlives the session.
func TestAnEventTypeThatIsNotANameIsNotWrittenDown(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: unknownEvent("response.SECRET_MARKER/../../etc/passwd"),
	})
	post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)

	body := bodyText(t, do(t, g, request{method: http.MethodGet, path: statusPath}))
	if strings.Contains(body, "SECRET_MARKER") || strings.Contains(body, "passwd") {
		t.Fatalf("the account wrote down the backend's own string:\n%s", body)
	}
	if !strings.Contains(body, bridge.EventOther) {
		t.Errorf("nothing was said about it at all:\n%s", body)
	}
}

// An event the parser itself refuses never becomes an unsupported one.
//
// A frame with no usable type is refused with its own error before it reaches the
// translator, so the three formats the Node baseline has for that -- missing, non-string,
// empty -- cannot occur here. A reader still learns: the refusal carries INVALID_SSE.
func TestAFrameWithNoTypeIsNotAnUnsupportedEvent(t *testing.T) {
	for name, frame := range map[string]string{
		"no type at all":          `{"response":{"id":"resp_1"}}`,
		"a type that is a number": `{"type":7}`,
		"an empty type":           `{"type":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			g := startWith(t, &upstream.Fixture{SSE: sse(created, frame, completed, "[DONE]")})
			resp := post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
			  "messages":[{"role":"user","content":"x"}]}`)
			if resp.StatusCode == http.StatusOK {
				t.Fatal("a frame with no usable type was accepted")
			}
			if report := status(t, g).Events; report.Unsupported != 0 {
				t.Fatalf("it was counted as an unsupported event: %+v", report)
			}
			if body := bodyText(t, resp); !strings.Contains(body, "INVALID_SSE") {
				t.Errorf("the client was told %q", body)
			}
		})
	}
}

// Every event this build handles stays handled.
//
// The list is the whole mechanism, so a name falling out of it is the defect this records.
func TestTheKnownEventsAreNotRefused(t *testing.T) {
	g := startWith(t, &upstream.Fixture{
		SSE: sse(created,
			`{"type":"response.in_progress"}`,
			`{"type":"response.queued"}`,
			`{"type":"ping"}`,
			`{"type":"keepalive"}`,
			`{"type":"rate_limits.updated"}`,
			`{"type":"codex.rate_limits"}`,
			`{"type":"codex.response.metadata"}`,
			`{"type":"responsesapi.websocket_timing"}`,
			`{"type":"response.reasoning_text.delta","item_id":"r1","delta":"x"}`,
			delta("ok"), done("ok"), completed, "[DONE]"),
	})
	resp := post(t, g, `{"model":"claude-opus-5","max_tokens":16,"stream":true,
	  "messages":[{"role":"user","content":"x"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, bodyText(t, resp))
	}
	if report := status(t, g).Events; report.Unsupported != 0 {
		t.Fatalf("a known event was refused: %+v", report)
	}
}
