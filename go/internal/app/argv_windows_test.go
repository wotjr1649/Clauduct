//go:build windows

package app

import (
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
)

// The shapes the offline argv tests carry, sent this time to a binary that is not Go.
var hostileArgs = []string{
	"",
	" leading and trailing ",
	`he said "hi"`,
	`C:\path\with\trailing\`,
	`a"b\"c`,
	"&", "|", "<", ">", "^", "%PATH%", "!DELAYED!",
	"--append-system-prompt", "--model is a string",
	"한국어", "🙂", "tab\there",
}

// ARG09. The argv fixtures elsewhere are Go handing arguments to Go.
//
// That leaves the question the ID asks unanswered. Go joins the arguments into one
// command-line string and the child takes it apart again, and a Go child takes it apart
// with Go's own implementation of the rules. Agreeing with yourself is not evidence.
//
// node.exe is the right second party, not a convenient one: the client this launcher
// spawns is a Node binary, so the rules node applies to a command line are the rules the
// real child applies. A shape that survives Go's quoting on the way out and node's parsing
// on the way back in is a shape this launcher can carry.
func TestHostileArgumentsSurviveARealNonGoParser(t *testing.T) {
	node, err := exec.LookPath("node.exe")
	if err != nil {
		t.Skipf("node.exe not found, and this measurement needs a non-Go parser: %v", err)
	}

	// Built the way the product builds it, so this measures the launcher's own argv and
	// not a list assembled for the occasion.
	spec := launch.Build(node,
		append([]string{"-e", "process.stdout.write(JSON.stringify(process.argv.slice(1)))"},
			hostileArgs...),
		nil, t.TempDir(), launch.Overlay{})

	cmd := exec.Command(spec.File, spec.Args...)
	cmd.Dir = spec.Dir
	out, err := outputWithin(t, cmd, 30*time.Second)
	if err != nil {
		t.Fatalf("node: %v (%s)", err, out)
	}

	var got []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if !reflect.DeepEqual(got, hostileArgs) {
		t.Fatalf("a non-Go parser did not see what was sent\n got: %#v\nwant: %#v",
			got, hostileArgs)
	}
	t.Logf("%d hostile shapes round-tripped through node.exe unchanged", len(hostileArgs))
}

// And the other half of ARG09: which shim this launcher supports.
//
// None, and that is the answer rather than an omission. The resolver looks for claude.exe
// and nothing else, so no .cmd, .bat or .ps1 ever sits between this process and the
// client — which matters because those are re-parsed by cmd.exe or PowerShell on the way
// through, under rules that are not the ones Go quoted for.
//
// The hazard is real and was measured, just not here. cscript.exe was tried first as the
// non-Go party above and mangled every quote: `he said "hi"` arrived as `he said \hi\`,
// because the Windows Script Host does not parse a command line the way the C runtime
// does. A launcher that resolved a shim would be exposed to exactly that class of thing.
func TestNoShimIsEverResolvedInPlaceOfTheExecutable(t *testing.T) {
	dir := t.TempDir()
	shims := map[string]bool{
		`C:\bin\claude.cmd`: true,
		`C:\bin\claude.bat`: true,
		`C:\bin\claude.ps1`: true,
		`C:\bin\claude`:     true,
	}
	resolver := platform.Resolver{
		Home:   dir,
		Cwd:    dir,
		Env:    map[string]string{"PATH": `C:\bin`},
		IsFile: func(path string) bool { return shims[path] },
	}

	path, found, err := resolver.Claude()
	if err != nil {
		t.Fatalf("Claude: %v", err)
	}
	if found {
		t.Fatalf("resolved %q. A shim is re-parsed by cmd.exe or PowerShell before the "+
			"client sees it, under rules Go did not quote for.", path)
	}
}

// outputWithin runs cmd and gives up rather than hanging the suite.
func outputWithin(t *testing.T, cmd *exec.Cmd, limit time.Duration) (string, error) {
	t.Helper()
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := cmd.Output()
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		return string(r.out), r.err
	case <-time.After(limit):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", errors.New("the child did not finish in time")
	}
}
