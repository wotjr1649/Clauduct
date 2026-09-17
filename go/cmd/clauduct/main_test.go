package main

import "testing"

// The owned option is recognised only where it cannot be a prompt.
//
// Every argument is a string, so an option matched anywhere in the vector fires on a prompt
// that merely mentions it. For --update that would replace binaries; for this it would
// print an account instead of starting the session the user asked for. First and alone is
// the rule, and it is the same one internal/update applies.
func TestTheUsageOptionIsOnlyTheWholeArgumentVector(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want bool
	}{
		{"alone", []string{"--usage"}, true},
		{"case", []string{"--USAGE"}, true},
		{"inside a prompt", []string{"-p", "what does --usage show"}, false},
		{"after other arguments", []string{"-p", "hi", "--usage"}, false},
		{"with anything after it", []string{"--usage", "-p", "hi"}, false},
		{"attached value", []string{"--usage=weekly"}, false},
		{"nothing", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := asked(c.args, usageOption); got != c.want {
				t.Fatalf("asked(%q) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
