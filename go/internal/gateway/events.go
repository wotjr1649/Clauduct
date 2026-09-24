package gateway

import (
	"sync"

	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
)

// Which backend event stopped a response.
//
// An event this build does not understand is refused rather than skipped, and that is the
// right call -- the unread event may be the one carrying the result, so continuing past it
// would produce a reply short by exactly the part nobody read. What was wrong was that the
// refusal said only UNSUPPORTED_EVENT. A backend that ships a new event name then breaks
// every turn in the session and the fix is one constant nobody can name.
//
// It has already happened: a request carrying a tool definition was refused before any call
// came back, because response.function_call_arguments.* was not in the list. That took a
// live session to find. With the name in the account it would have taken one line of output.
//
// The name is the backend's own string, so it is shaped and bounded before it is kept, and a
// type that does not fit the shape leaves a fixed label instead. See bridge.unsupportedEvent.

// eventNames is how many distinct names are kept.
const eventNames = 8

// EventReport is what the session could not read.
type EventReport struct {
	// Unsupported is how many responses were stopped by an event type.
	Unsupported int64 `json:"unsupported,omitempty"`
	// Names are the types, when they were safe to record.
	Names []string `json:"names,omitempty"`
	// Formats counts how each unrecordable type was judged, by label.
	Formats map[string]int64 `json:"formats,omitempty"`
	// OutputItems counts the output items the backend opened, by type, named by the same
	// rule (#85). At most itemTypes of them; the rest are counted under "<more>".
	OutputItems map[string]int64 `json:"outputItems,omitempty"`
}

type eventLedger struct {
	mu          sync.Mutex
	unsupported int64
	names       map[string]bool
	formats     map[string]int64
	items       map[string]int64
}

func newEventLedger() *eventLedger {
	return &eventLedger{names: map[string]bool{}, formats: map[string]int64{}, items: map[string]int64{}}
}

// observe records one refusal.
func (e *eventLedger) observe(refusal *bridge.UnsupportedEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.unsupported++
	if refusal.Name != "" {
		add(e.names, refusal.Name, eventNames)
		return
	}
	// Only the ones with no name are counted by label. A named one is already the better
	// answer, and counting it twice would make the totals disagree with the list.
	if len(e.formats) < eventNames || e.formats[refusal.Format] > 0 {
		e.formats[refusal.Format]++
	}
}

// itemTypes bounds how many distinct output item types are kept.
const itemTypes = 16

// observeItems adds one response's output item counts.
func (e *eventLedger) observeItems(types map[string]int) {
	if len(types) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for kind, n := range types {
		if _, kept := e.items[kind]; !kept && len(e.items) >= itemTypes {
			kind = "<more>"
		}
		e.items[kind] += int64(n)
	}
}

func (e *eventLedger) report() EventReport {
	e.mu.Lock()
	defer e.mu.Unlock()
	report := EventReport{Unsupported: e.unsupported, Names: sorted(e.names)}
	if len(e.items) > 0 {
		report.OutputItems = make(map[string]int64, len(e.items))
		for kind, n := range e.items {
			report.OutputItems[kind] = n
		}
	}
	if len(e.formats) > 0 {
		report.Formats = make(map[string]int64, len(e.formats))
		for label, count := range e.formats {
			report.Formats[label] = count
		}
	}
	return report
}
