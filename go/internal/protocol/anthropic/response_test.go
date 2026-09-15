package anthropic

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// A response has a ceiling. Without one, a backend that keeps sending holds memory here
// for as long as it likes, and the request that is meant to be bounded is not.
//
// Added after a mutation run: deleting the ceiling left the suite green, so nothing was
// checking it.
func TestResponseByteCeilingIsEnforced(t *testing.T) {
	builder := NewBuilder("m")
	chunk := strings.Repeat("x", 1<<20) // 1 MiB

	var err error
	for i := 0; i < 32 && err == nil; i++ {
		_, err = builder.AppendText(0, chunk)
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want RESPONSE_TOO_LARGE before 32 MiB", err)
	}
}

// The number of content blocks is bounded too. A backend opening a new index each time
// would otherwise grow the part table without limit.
func TestContentPartCeilingIsEnforced(t *testing.T) {
	builder := NewBuilder("m")
	var err error
	for i := 0; i < maxTextParts+2 && err == nil; i++ {
		_, err = builder.AppendText(i, "x")
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want RESPONSE_TOO_LARGE past the part ceiling", err)
	}
}

// Nothing may be added after the message has ended. A client that has already read
// message_stop is not reading any more, so a late delta is content that silently vanishes
// — and a late one that did arrive would contradict a message it has already closed.
//
// Added after a mutation run: removing the guard left the suite green.
func TestNothingIsAcceptedAfterCompletion(t *testing.T) {
	builder := NewBuilder("m")
	if _, err := builder.AppendText(0, "before"); err != nil {
		t.Fatalf("AppendText: %v", err)
	}
	if _, err := builder.Complete(Usage{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if _, err := builder.AppendText(0, "after"); !errors.Is(err, ErrStreamOrder) {
		t.Errorf("AppendText after completion: err = %v, want STREAM_ORDER", err)
	}
	if _, err := builder.AppendText(1, "after in a new block"); !errors.Is(err, ErrStreamOrder) {
		t.Errorf("AppendText in a new block after completion: err = %v, want STREAM_ORDER", err)
	}
	if _, err := builder.FinishText(0, "before"); !errors.Is(err, ErrStreamOrder) {
		t.Errorf("FinishText after completion: err = %v, want STREAM_ORDER", err)
	}
	if _, err := builder.Complete(Usage{}); !errors.Is(err, ErrStreamOrder) {
		t.Errorf("a second Complete: err = %v, want STREAM_ORDER", err)
	}
	if got := builder.Text(); got != "before" {
		t.Errorf("Text = %q; a refused append must not have been recorded either", got)
	}
}

// A block that has been closed by its snapshot does not reopen.
func TestClosedBlockDoesNotReopen(t *testing.T) {
	builder := NewBuilder("m")
	if _, err := builder.AppendText(0, "done"); err != nil {
		t.Fatalf("AppendText: %v", err)
	}
	if _, err := builder.FinishText(0, "done"); err != nil {
		t.Fatalf("FinishText: %v", err)
	}
	if _, err := builder.AppendText(0, "more"); !errors.Is(err, ErrStreamOrder) {
		t.Fatalf("err = %v, want STREAM_ORDER", err)
	}
	if _, err := builder.FinishText(0, "done"); !errors.Is(err, ErrStreamOrder) {
		t.Fatalf("a second FinishText: err = %v, want STREAM_ORDER", err)
	}
}

// A negative content index is not a position.
func TestNegativeContentIndexIsRefused(t *testing.T) {
	builder := NewBuilder("m")
	if _, err := builder.AppendText(-1, "x"); !errors.Is(err, ErrStreamOrder) {
		t.Fatalf("err = %v, want STREAM_ORDER", err)
	}
}

// The response id must be settled before the message opens, because message_start carries
// it and that frame is emitted once.
func TestResponseIDCannotChangeAfterTheMessageStarts(t *testing.T) {
	builder := NewBuilder("m")
	if err := builder.SetResponseID("resp_first"); err != nil {
		t.Fatalf("SetResponseID: %v", err)
	}
	frames, err := builder.AppendText(0, "x")
	if err != nil {
		t.Fatalf("AppendText: %v", err)
	}
	if !strings.Contains(string(frames[0].Data), "resp_first") {
		t.Fatalf("message_start = %s", frames[0].Data)
	}
	if err := builder.SetResponseID("resp_second"); !errors.Is(err, ErrStreamOrder) {
		t.Fatalf("err = %v, want STREAM_ORDER", err)
	}
}

// With no identifier from the backend, the placeholder must be visibly local so it cannot
// be mistaken for one in a transcript.
func TestMissingResponseIDGetsAVisiblyLocalPlaceholder(t *testing.T) {
	builder := NewBuilder("m")
	id := builder.ResponseID()
	if id == "" {
		t.Fatal("no identifier at all")
	}
	if !strings.Contains(id, "clauduct") {
		t.Fatalf("placeholder %q does not say where it came from", id)
	}
}

// Blocks close in the order they opened. Go's map iteration is deliberately random, so a
// sequence built from one would differ between runs and a client reading sequentially
// would see a different order each time.
func TestCompletionClosesBlocksInIndexOrder(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		builder := NewBuilder("m")
		for index := 0; index < 5; index++ {
			if _, err := builder.AppendText(index, "x"); err != nil {
				t.Fatalf("AppendText: %v", err)
			}
		}
		frames, err := builder.Complete(Usage{})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		var order []string
		for _, frame := range frames {
			if frame.Type == "content_block_stop" {
				order = append(order, string(frame.Data))
			}
		}
		if len(order) != 5 {
			t.Fatalf("got %d stops, want 5", len(order))
		}
		for index, data := range order {
			if !strings.Contains(data, `"index":`+string(rune('0'+index))) {
				t.Fatalf("attempt %d: stop %d = %s, want index %d", attempt, index, data, index)
			}
		}
	}
}

// An error frame carries a fixed category and nothing from the backend's body.
func TestErrorFrameCarriesOnlyTheCategory(t *testing.T) {
	frame := ErrorFrame("UPSTREAM_RESPONSE_FAILED")
	if frame.Type != "error" {
		t.Fatalf("Type = %q", frame.Type)
	}
	data := string(frame.Data)
	if !strings.Contains(data, "UPSTREAM_RESPONSE_FAILED") {
		t.Fatalf("data = %s", data)
	}
	var body strings.Builder
	frame.WriteTo(&body)
	if !strings.HasPrefix(body.String(), "event: error\ndata: ") {
		t.Fatalf("framing = %q", body.String())
	}
}

// The ceiling applies to tool arguments too. A backend sending enormous arguments holds
// memory here just as text does, and the arguments are additionally about to be handed to
// something that will act on them.
//
// Added after a mutation run: the ceiling check exists in two places and only one was
// under test, so deleting the other left the suite green.
func TestToolArgumentsCountAgainstTheResponseCeiling(t *testing.T) {
	builder := NewBuilder("m")
	builder.SetCallable(func(string) bool { return true })

	// A megabyte of arguments per call, each a well-formed object.
	arguments := []byte(`{"path":"` + strings.Repeat("x", 1<<20) + `"}`)

	var err error
	for i := 0; i < 32 && err == nil; i++ {
		err = builder.AddToolCall("call_"+strconv.Itoa(i), "Read", arguments)
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want RESPONSE_TOO_LARGE before 32 MiB of arguments", err)
	}
}

// And the count of calls is bounded, not only their size.
func TestToolCallCountIsBounded(t *testing.T) {
	builder := NewBuilder("m")
	builder.SetCallable(func(string) bool { return true })

	var err error
	for i := 0; i < maxTextParts+2 && err == nil; i++ {
		err = builder.AddToolCall("call_"+strconv.Itoa(i), "Read", []byte(`{}`))
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want RESPONSE_TOO_LARGE past the call ceiling", err)
	}
}
