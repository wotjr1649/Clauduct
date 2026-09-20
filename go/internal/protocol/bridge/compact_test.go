package bridge

import (
	"strings"
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// compaction builds the client's own compaction turn around some transcript.
func compaction(body string) string { return compactPrefix + body + compactSuffix }

// text is one user turn made of text blocks.
func text(role string, blocks ...string) anthropic.Message {
	message := anthropic.Message{Role: role}
	for _, block := range blocks {
		message.Blocks = append(message.Blocks, anthropic.Block{Type: "text", Text: block})
	}
	return message
}

func TestACompactionPreservesEffort(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		request := &anthropic.Request{
			Model:    "claude-opus-5",
			Effort:   effort,
			Messages: []anthropic.Message{text("user", compaction("everything so far"))},
		}
		built, err := BuildRequest(request)
		if err != nil {
			t.Fatalf("BuildRequest at %s: %v", effort, err)
		}
		if built.Effort.Effort != effort {
			t.Errorf("a compaction at %s ran at %s, want %s",
				effort, built.Effort.Effort, effort)
		}
		if built.Source == "compact" {
			t.Errorf("source = %q at %s", built.Source, effort)
		}
		// Compaction must also preserve the resolved model.
		if built.Model != "gpt-5.6-sol" {
			t.Errorf("the compaction was moved to %s", built.Model)
		}
	}
}

func TestCompactionKeepsTheAgentOverride(t *testing.T) {
	for _, route := range Catalogue() {
		for _, effort := range Efforts {
			route.Effort, route.Source = effort, "agent"
			rq := &anthropic.Request{Model: "gpt-6-astra", Effort: "low",
				Messages: []anthropic.Message{text("user", compaction("synthetic transcript"))}}
			got, err := BuildRequest(rq, route)
			if err != nil {
				t.Fatal(err)
			}
			if got.Model != route.Model || got.Effort.Effort != effort || got.Source != "agent" {
				t.Fatalf("override %v changed during compaction: %+v", route, got)
			}
		}
	}
}

// A session already cheaper than medium keeps what it had.
func TestACheapSessionIsNotMadeDearerByCompacting(t *testing.T) {
	request := &anthropic.Request{
		Model:    "claude-opus-5",
		Effort:   "low",
		Messages: []anthropic.Message{text("user", compaction("everything so far"))},
	}
	built, err := BuildRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if built.Effort.Effort != "low" {
		t.Fatalf("a compaction on a low session ran at %s", built.Effort.Effort)
	}
	if built.Source == "compact" {
		t.Errorf("source = %q for a request the rule did not change", built.Source)
	}
}

// What is and is not the client compacting its own context.
func TestWhatCountsAsACompaction(t *testing.T) {
	for name, tc := range map[string]struct {
		messages []anthropic.Message
		want     bool
	}{
		"the client's own template": {
			messages: []anthropic.Message{text("user", compaction("a transcript"))},
			want:     true,
		},
		// The terminal's paste indentation and the client's line wrapping both change the
		// bytes without changing the text.
		"the same wording, wrapped differently": {
			messages: []anthropic.Message{text("user",
				strings.ReplaceAll(compaction("a transcript"), " ", "\n   "))},
			want: true,
		},
		// The client appends a reminder to the merged user turn. That is the client talking
		// to itself and must not hide the compaction behind it.
		"with a system reminder after it": {
			messages: []anthropic.Message{text("user",
				compaction("a transcript"), "<system-reminder>be careful</system-reminder>")},
			want: true,
		},
		"a system turn after it": {
			messages: []anthropic.Message{
				text("user", compaction("a transcript")), text("system", "anything")},
			want: true,
		},
		"the boundaries with nothing between them": {
			messages: []anthropic.Message{text("user", compactPrefix+compactSuffix)},
			want:     false,
		},
		// Long enough that the length check cannot be what rejects it. Both boundaries
		// have to be there, and a test that only ever fails on length proves neither.
		"only the opening": {
			messages: []anthropic.Message{text("user",
				compactPrefix+strings.Repeat("a transcript ", 60))},
			want: false,
		},
		"only the closing": {
			messages: []anthropic.Message{text("user",
				strings.Repeat("a transcript ", 60)+compactSuffix)},
			want: false,
		},
		// The model's own output is not a routing signal. If it were, anything that quoted
		// the template back could move where the next request runs.
		"the assistant saying it": {
			messages: []anthropic.Message{
				text("user", "hello"), text("assistant", compaction("a transcript"))},
			want: false,
		},
		"a user quoting the template at us": {
			messages: []anthropic.Message{
				text("user", compaction("a transcript")), text("assistant", "done")},
			want: false,
		},
		"an ordinary turn": {
			messages: []anthropic.Message{text("user", "summarise this for me")},
			want:     false,
		},
		"nothing at all": {want: false},
		"a turn with no text in it": {
			messages: []anthropic.Message{{Role: "user"}},
			want:     false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := &anthropic.Request{Messages: tc.messages}
			if got := IsCompactTemplate(textOf(request)); got != tc.want {
				t.Fatalf("IsCompactTemplate = %v, want %v", got, tc.want)
			}
		})
	}
}

// Only text is read, and only from the last user turn.
//
// A rule about wording that could reach a tool's output would be a routing decision made
// from something a tool wrote, and historical summaries are the transcript rather than the
// request.
func TestNothingButTheLastUserTurnsTextIsRead(t *testing.T) {
	// A block that is not a text block, carrying the template in its Text field. No decoder
	// produces this today -- only the text block path fills Text -- and that is exactly why
	// the guard is worth having and worth testing directly: the day a block type reuses the
	// field, routing must not start reading it.
	for _, kind := range []string{"tool_result", "thinking", "redacted_thinking", "image"} {
		hidden := anthropic.Message{Role: "user", Blocks: []anthropic.Block{
			{Type: kind, Text: compaction("a transcript")},
		}}
		if IsCompactTemplate(textOf(&anthropic.Request{Messages: []anthropic.Message{hidden}})) {
			t.Errorf("a %s block decided where the request runs", kind)
		}
	}

	// And an older turn that was a compaction does not make this one one.
	older := []anthropic.Message{
		text("user", compaction("a transcript")),
		text("assistant", "here is the summary"),
		text("user", "now carry on"),
	}
	if IsCompactTemplate(textOf(&anthropic.Request{Messages: older})) {
		t.Error("a past compaction in the transcript decided where this request runs")
	}
}
