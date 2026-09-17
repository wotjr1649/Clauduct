package main

import (
	"strings"
	"testing"
)

// The subcommand reaches the shared view.
//
// Thin on purpose: what the view says is fixed in internal/app, where it lives. What this
// checks is that `clauduct-dev usage` is wired to it at all, which the switch could lose.
func TestTheUsageSubcommandReachesTheView(t *testing.T) {
	var out, errOut strings.Builder
	run([]string{"usage"}, &out, &errOut)
	if !strings.Contains(out.String(), "quota") && !strings.Contains(out.String(), "no reading yet") {
		t.Fatalf("usage printed neither a reading nor the absence of one: %q %q",
			out.String(), errOut.String())
	}
	if strings.Contains(errOut.String(), "usage: clauduct-dev") {
		t.Fatal("the subcommand fell through to the usage banner")
	}
}
