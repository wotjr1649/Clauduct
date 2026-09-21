package anthropic

import "testing"

// A delta for an earlier, still-open block after a later one opened is an accepted input
// shape: content_block_start never requires the previous block closed, and
// content_block_delta never requires the newest index. It used to reach a strings.Builder
// that append had moved, and Builder.copyCheck panics on that -- net/http recovers it per
// connection, so the client saw a reset socket rather than the ErrStreamOrder every other
// malformed shape returns. Blocks are pointers now, so the sequence is merely accepted.
//
// Two blocks are the minimum that reproduces it: append grows capacity 1 to 2 there, which
// is what relocates block 0 while its Builder still records the old address.
func TestADeltaForAnEarlierBlockDoesNotPanic(t *testing.T) {
	b := NewBuilder("public-model")
	var m ResponseMessage
	first, err := b.AppendText("part-a", 0, "가")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Add(first); err != nil {
		t.Fatal(err)
	}
	second, err := b.AppendText("part-b", 1, "나")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Add(second); err != nil {
		t.Fatal(err)
	}
	// Panics before the fix; the frame is ordinary bytes either way.
	if err = m.Add([]Frame{contentBlockDelta(0, "다")}); err != nil {
		t.Fatalf("interleaved delta rejected outright: %v", err)
	}
	if got := m.blocks[0].data.String(); got != "가다" {
		t.Fatalf("earlier block lost its delta: %q", got)
	}
	if got := m.blocks[1].data.String(); got != "나" {
		t.Fatalf("later block disturbed: %q", got)
	}
}
