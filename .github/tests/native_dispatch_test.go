package app

import (
	"testing"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
)

func TestNative330DispatchContract(t *testing.T) {
	previous := nativeArgvVersion
	t.Cleanup(func() { nativeArgvVersion = previous })
	for _, version := range []string{launch.NativeDispatchVersion, "2.1.325", ""} {
		nativeArgvVersion = func() string { return version }
		pinned := version == launch.NativeDispatchVersion
		for _, tc := range []struct {
			args     []string
			wantInfo bool
		}{
			{[]string{"--metadata", "--bg", "--help"}, pinned},
			{[]string{"--metadata=name=value", "-h"}, pinned},
			{[]string{"--initialize-fields", "settings", "--version"}, pinned},
			{[]string{"--hold-repository-while-parked", "--help"}, pinned},
			{[]string{"--metadata", "--help"}, false},
			{[]string{"--initialize-fields", "--version"}, false},
			{[]string{"--metadata", "name=value", "--bg", "--help"}, false},
			{[]string{"--metadata", "name=value", "--", "--help"}, false},
			{[]string{"--unknown", "--help"}, false},
			{[]string{"--version"}, true},
		} {
			if got := nativeInformation(tc.args); got != tc.wantInfo {
				t.Errorf("native %q: nativeInformation(%q) = %t, want %t", version, tc.args, got, tc.wantInfo)
			}
		}
		args := []string{"--hold-repository-while-parked", "logs", "deadbeef"}
		want := ""
		if pinned {
			want = "logs"
		}
		if got := nativeControl(args); got != want {
			t.Errorf("native %q: nativeControl(%q) = %q, want %q", version, args, got, want)
		}
	}
}
