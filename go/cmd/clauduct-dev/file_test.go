package main

import (
	"bytes"
	"io"
	"testing"
)

// The scan has to survive the token arriving in pieces.
//
// This test exists because the first version of this probe looked for the token in the
// exchange the translator returns, and that buffer is cleared before it comes back. The
// probe reported "the reply does not carry the token" as a fact about the backend when it
// was a fact about the probe. A check that cannot fail for the right reason is not a check.
func TestASightingFindsATokenSplitAcrossReads(t *testing.T) {
	want := []byte("CLAUDUCT-PDF-7Q4M")

	whole := &sighting{want: want}
	if _, err := io.Copy(io.Discard, io.TeeReader(bytes.NewReader(
		[]byte(`data: {"delta":"CLAUDUCT-PDF-7Q4M"}`)), whole)); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if !whole.found {
		t.Fatal("a token arriving in one read was not seen")
	}

	// Split down the middle, one byte at a time, which is the shape a delta stream takes.
	pieces := &sighting{want: want}
	for _, b := range []byte(`data: {"delta":"CLAUDUCT-PDF-7Q4M"}`) {
		if _, err := pieces.Write([]byte{b}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if !pieces.found {
		t.Fatal("a token split across reads was not seen")
	}

	// And it must not see one that never arrived, or the probe would report every backend
	// as reading the file.
	absent := &sighting{want: want}
	if _, err := absent.Write([]byte(`data: {"delta":"I cannot read files."}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if absent.found {
		t.Fatal("a token that never arrived was reported as seen")
	}
}
