package app

import "testing"

func TestNativeConservativeDispatchContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		bg   bool
		verb string
		info bool
	}{
		{"background", []string{"prompt", "--bg"}, true, "", false},
		{"background takes precedence", []string{"stop", "deadbeef", "--background", "--help"}, true, "", false},
		{"option value is a background token", []string{"--name", "--bg", "--help"}, true, "", false},
		{"background after separator", []string{"--", "--bg"}, true, "", false},
		{"attached option value", []string{"--name=--bg", "--help"}, false, "", true},
		{"leading rm", []string{"rm", "deadbeef", "--verbose"}, false, "rm", false},
		{"leading stop", []string{"stop", "deadbeef"}, false, "stop", false},
		{"leading logs", []string{"logs", "deadbeef"}, false, "logs", false},
		{"leading option", []string{"--verbose", "rm", "deadbeef"}, false, "", false},
		{"leading kill", []string{"kill", "deadbeef"}, false, "", false},
		{"unknown option before help", []string{"--metadata=name=value", "-h"}, false, "", false},
		{"version", []string{"--version"}, false, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := BackgroundRequested(tc.args); got != tc.bg {
				t.Errorf("BackgroundRequested(%q) = %t, want %t", tc.args, got, tc.bg)
			}
			if got := nativeControl(tc.args); got != tc.verb {
				t.Errorf("nativeControl(%q) = %q, want %q", tc.args, got, tc.verb)
			}
			if got := nativeInformation(tc.args); got != tc.info {
				t.Errorf("nativeInformation(%q) = %t, want %t", tc.args, got, tc.info)
			}
		})
	}
}
