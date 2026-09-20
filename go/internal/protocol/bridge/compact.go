package bridge

import (
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic"
)

// Recognising the client's compaction template for diagnostics. Compaction retains
// the resolved model and effort; matching text must never change the selection.
//
// This is template compatibility and nothing more. It is not an authenticated origin, not a
// permission decision, and not a claim about who sent the request -- and the prompt itself is
// never rewritten. Only the last user turn is examined: historical summaries, assistant text
// and tool output are never read to decide where a request runs.

// compactPrefix and compactSuffix are the exact boundaries Claude 2.1.263 wraps a compaction
// in. Read off the client rather than invented, which is the only way this can be right.
const compactPrefix = "CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.\n\n" +
	"- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.\n" +
	"- You already have all the context you need in the conversation above.\n" +
	"- Tool calls will be REJECTED and will waste your only turn — you will fail the task.\n" +
	"- Your entire response must be plain text: an <analysis> block followed by a <summary> block.\n\n"

const compactSuffix = "\n\nREMINDER: Do NOT call any tools. Respond with plain text only — " +
	"an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task."

// A summary preference, not a truncation or output-token cap. Explicit retention
// requirements take precedence, and the exact preflight counts this instruction.
const CompactEfficiencyInstruction = " For this compaction, keep any requested analysis brief. In the summary preserve every explicit retention requirement, exact task data, decisions, constraints, agent/run IDs, verified outcomes, failures, and next actions. State each fact once; omit repeated narration, duplicate reports and unexecuted example scripts unless explicitly requested for retention. Aim for at most 1200 words when those requirements fit; exceed that target whenever needed to preserve required information. Keep the native summary format."

// folded collapses runs of whitespace so the comparison is about wording.
//
// A terminal's paste indentation and the client's own line wrapping both change the bytes
// without changing the text, and matching on bytes would make this work until the day
// somebody resizes a window.
func folded(value string) string { return strings.Join(strings.Fields(value), " ") }

// IsCompactTemplate reports whether this request is the client compacting its own context.
func IsCompactTemplate(messages []Message) bool {
	text, ok := lastUserText(messages)
	if !ok {
		return false
	}
	body := folded(text)
	prefix, suffix := folded(compactPrefix), folded(compactSuffix)
	// The two boundaries with something between them. A request that is the boundaries and
	// nothing else is not a compaction of anything.
	return strings.HasPrefix(body, prefix+" ") &&
		strings.HasSuffix(body, " "+suffix) &&
		len(body) > len(prefix)+len(suffix)+2
}

// IsCompaction recognizes the native template, never authorizes its execution.
func IsCompaction(request *anthropic.Request) bool { return IsCompactTemplate(textOf(request)) }

// Message is the part of a turn this rule looks at. Declared here rather than taking the
// anthropic type so the rule stays a rule about text and cannot reach anything else in a
// request.
type Message struct {
	Role   string
	Blocks []string
}

// lastUserText finds the text a compaction would be in.
func lastUserText(messages []Message) (string, bool) {
	last := -1
	for i, message := range messages {
		if message.Role != "system" {
			last = i
		}
	}
	if last < 0 || messages[last].Role != "user" {
		return "", false
	}

	// The client appends a newline when it merges adjacent user messages, and that merged
	// turn can still carry earlier reminders. Those are the client talking to itself.
	var texts []string
	for _, text := range messages[last].Blocks {
		trimmed := strings.TrimRight(text, " \t\r\n")
		if strings.HasPrefix(trimmed, "<system-reminder>") && strings.HasSuffix(trimmed, "</system-reminder>") {
			continue
		}
		texts = append(texts, trimmed)
	}
	if len(texts) == 0 {
		return "", false
	}
	return texts[len(texts)-1], true
}

// textOf reduces a request to the turns and text this rule is allowed to look at.
//
// Text blocks only, and nothing else travels: no tool result, no image, no identifier. A
// rule about wording that could reach a tool's output would be a routing decision made from
// something a tool wrote.
func textOf(request *anthropic.Request) []Message {
	messages := make([]Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		entry := Message{Role: message.Role}
		for _, block := range message.Blocks {
			if block.Type == "text" {
				entry.Blocks = append(entry.Blocks, block.Text)
			}
		}
		messages = append(messages, entry)
	}
	return messages
}
