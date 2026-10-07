package bridge

import (
	"regexp"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/codex"
	"github.com/wotjr1649/Clauduct/go/internal/stream"
)

// safetyLabel is what a backend-supplied use case or reason may be recorded as.
var safetyLabel = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// maxSafetyKeys bounds the distinct keys one response records.
const maxSafetyKeys = 16

// noteSafetyBuffering records the safety_buffering notice an event carries, if any, and
// refuses an active value of any other shape. The event itself is then read exactly as it
// would be without the notice: the backend only says it is holding output for extra checks,
// and a blocked result arrives through the ordinary terminal events. A response.metadata event
// is accepted only when it is such a notice (t.metadataNotice tells Accept so); any other
// response.metadata stays unsupported.
func (t *Translator) noteSafetyBuffering(event stream.Event) error {
	notice, err := codex.SafetyBufferingOf(event.Type, event.Raw)
	if err != nil {
		return err
	}
	if notice == nil {
		return nil
	}
	t.metadataNotice = event.Type == codex.MetadataEvent
	t.countSafety("notices")
	for _, name := range notice.UseCases {
		t.countSafety("use_case:" + safetyName(name))
	}
	for _, name := range notice.Reasons {
		t.countSafety("reason:" + safetyName(name))
	}
	return nil
}

func safetyName(name string) string {
	if safetyLabel.MatchString(name) {
		return name
	}
	return "<other>"
}

func (t *Translator) countSafety(key string) {
	if t.safety == nil {
		t.safety = map[string]int{}
	}
	if _, kept := t.safety[key]; !kept && len(t.safety) >= maxSafetyKeys {
		key = "<more>"
	}
	t.safety[key]++
}

// SafetyBuffering counts the notices the backend sent: "notices" is the number of events
// that carried one, and "use_case:<name>" and "reason:<name>" count the values they named.
// Names are shaped and bounded; no content is recorded.
func (t *Translator) SafetyBuffering() map[string]int { return t.safety }

// acceptsEventType lists the event types Accept dispatches (response.metadata, as a notice,
// whose invalid form is a shape error). It exists so that a bad safety_buffering value
// on a type this build does not know ends as that type's UnsupportedEvent, which the gateway
// ledger names, rather than as a shape error that hides the type. Keep it in step with the
// switch in Accept; TestUnknownEventTypeWithBadBufferingStaysUnsupported walks it.
var acceptsEventType = map[string]bool{
	codex.Created: true, codex.TextDelta: true, codex.OutputItemAdd: true, codex.OutputItemDone: true,
	codex.FuncArgsDelta: true, codex.FuncArgsDone: true, codex.TextDone: true, codex.Completed: true,
	codex.ContentPartAdd: true, codex.ContentPartDon: true, codex.InProgress: true, codex.Queued: true,
	codex.Keepalive: true, codex.Ping: true, codex.RateLimitsUpdated: true, codex.CodexRateLimits: true,
	codex.CodexMetadata: true, codex.WebsocketTiming: true, codex.MetadataEvent: true,
	codex.ReasoningSummaryAdd: true, codex.ReasoningSummaryDone: true, codex.ReasoningSummaryTxtD: true,
	codex.ReasoningSummaryTxtF: true, codex.ReasoningTextDelta: true, codex.ReasoningTextDone: true,
}
