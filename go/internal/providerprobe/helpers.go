package providerprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"context"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/protocol/bridge"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

// TB is the part of a test the runner uses: *testing.T satisfies it, and the probe binary
// supplies its own for one run.
type TB interface {
	Helper()
	TempDir() string
	Cleanup(func())
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

// nativeAvailable is the claude.exe a session would resolve; a run without one cannot measure.
func nativeAvailable(t TB) string {
	t.Helper()
	path, found, err := platform.Resolver{}.Claude()
	if err != nil {
		t.Fatalf("resolving claude.exe: %v", err)
	}
	if !found {
		t.Fatalf("claude.exe not installed at %s", path)
	}
	return path
}

// workspace is a working directory with a parent nobody else writes to.
func workspace(t TB) (parent, cwd string) {
	t.Helper()
	parent = filepath.Join(t.TempDir(), "workspace")
	cwd = filepath.Join(parent, "project")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	return parent, cwd
}

func writeJSON(t TB, path string, v any) {
	t.Helper()
	b, _ := json.MarshalIndent(v, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Errorf("%v", err)
	}
}

// startOSProcess starts spec as the product does: in its own Job.
func startOSProcess(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (app.Process, error) {
	cmd := exec.Command(spec.File, spec.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Dir, spec.Env, stdin, stdout, stderr
	return childprocess.Start(cmd)
}

// productExecutable is the clauduct.exe this run certifies; the probe binary sets it.
var productExecutable = ""

func executable() (string, error) {
	if productExecutable == "" {
		return "", errors.New("no product executable set for this run")
	}
	return productExecutable, nil
}

var _ app.Process = (*childprocess.Process)(nil)

// measuredStream is the reply shape the real backend was observed to send (2026-09-15).
func measuredStream(text string) string {
	encoded, _ := json.Marshal(text)
	item := `{"id":"msg_synth","type":"message","content":[{"type":"output_text","text":` + string(encoded) + `}]}`
	return frames(
		`{"type":"response.created","response":{"id":"resp_synth"}}`,
		`{"type":"response.in_progress"}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_synth","type":"message"}}`,
		`{"type":"response.content_part.added","item_id":"msg_synth","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","item_id":"msg_synth","content_index":0,"delta":`+string(encoded)+`}`,
		`{"type":"response.output_text.done","item_id":"msg_synth","content_index":0,"text":`+string(encoded)+`}`,
		`{"type":"response.content_part.done","item_id":"msg_synth","output_index":0,"content_index":0,"part":{"type":"output_text","text":`+string(encoded)+`}}`,
		`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
		`{"type":"response.completed","response":{"id":"resp_synth","usage":{"input_tokens":9,"output_tokens":2,"total_tokens":11},"output":[]}}`,
	)
}

// toolStream is a backend turn that asks for one tool call, in the measured shape.
func toolStream(callID, name, arguments string) string {
	args, _ := json.Marshal(arguments)
	item := `{"id":"fc_1","type":"function_call","call_id":"` + callID + `","name":"` + name + `","arguments":` + string(args) + `}`
	return frames(
		`{"type":"response.created","response":{"id":"resp_tool"}}`,
		`{"type":"response.in_progress"}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":`+string(args)+`}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":`+string(args)+`}`,
		`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
		`{"type":"response.completed","response":{"id":"resp_tool","usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13},"output":[]}}`,
	)
}

func frames(bodies ...string) string {
	var b strings.Builder
	for _, body := range bodies {
		b.WriteString("data: " + body + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// processTable is one Toolhelp snapshot: image base name and parent of every process.
type processTable map[uint32]struct {
	image  string
	parent uint32
}

func snapshotProcesses() processTable {
	table := processTable{}
	h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return table
	}
	defer syscall.CloseHandle(h)
	e := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(h, &e); err == nil; err = syscall.Process32Next(h, &e) {
		table[e.ProcessID] = struct {
			image  string
			parent uint32
		}{strings.ToLower(syscall.UTF16ToString(e.ExeFile[:])), e.ParentProcessID}
	}
	return table
}

// descends reports whether pid descends from root in the table. A parent PID outlives its
// process, so the recorded parent of an unrelated process can be a PID a later process reuses:
// every edge, the one into root included, must name a parent created strictly before its
// child, and a creation time that cannot be read (createdOf returns the zero time) proves
// nothing. This is the per-edge rule providercert applies to a native's ancestry.
func (t processTable) descends(pid, root uint32, createdOf func(uint32) time.Time) bool {
	child := createdOf(pid)
	for range 64 {
		p, ok := t[pid]
		if !ok || p.parent == 0 || p.parent == pid || child.IsZero() {
			return false
		}
		parent := createdOf(p.parent)
		if parent.IsZero() || !parent.Before(child) {
			return false
		}
		if p.parent == root {
			return true
		}
		pid, child = p.parent, parent
	}
	return false
}

// processCreatedAt is when pid was created, or the zero time when the OS will not say.
func processCreatedAt(pid uint32) time.Time {
	created, err := providercert.ProcessCreated(pid)
	if err != nil || created <= 0 {
		return time.Time{}
	}
	return time.Unix(0, created)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// p3Catalogue is the probe's synthetic account model list (the private test harness's list).
var p3Catalogue = func() *bridge.Catalogue {
	all := []string{"low", "medium", "high", "xhigh", "max"}
	c, _, err := bridge.NewCatalogue([]bridge.AccountModel{
		{ID: "gpt-6-astra", Efforts: all, Default: "medium", Visible: true},
		{ID: "gpt-6.1-sol", Efforts: all, Default: "xhigh", Visible: true},
		{ID: "gpt-6-sol", Efforts: all, Default: "xhigh", Visible: true},
		{ID: "gpt-5.6-terra", Efforts: all, Default: "high", Visible: true},
		{ID: "gpt-6-luna", Efforts: all, Default: "max", Visible: true},
	})
	if err != nil {
		panic(err)
	}
	return c
}()

func p3ModelList(context.Context, string) (*bridge.Catalogue, app.ModelListFacts, error) {
	return p3Catalogue, app.ModelListFacts{Source: "account", Models: len(p3Catalogue.Models())}, nil
}
