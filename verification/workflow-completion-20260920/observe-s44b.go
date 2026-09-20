// Read-only Windows OS evidence for a native TUI Bash ping test.
// Holds process handles across cancellation; never terminates a process.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type process struct {
	PID     uint32 `json:"pid"`
	Parent  uint32 `json:"parent"`
	Name    string `json:"name"`
	Created string `json:"created"`
	Exited  string `json:"exited,omitempty"`
	h       syscall.Handle
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func snapshot() map[uint32]process {
	h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	must(err)
	defer syscall.CloseHandle(h)
	e := syscall.ProcessEntry32{}
	e.Size = uint32(unsafe.Sizeof(e))
	out := map[uint32]process{}
	for err = syscall.Process32First(h, &e); err == nil; err = syscall.Process32Next(h, &e) {
		out[e.ProcessID] = process{PID: e.ProcessID, Parent: e.ParentProcessID, Name: syscall.UTF16ToString(e.ExeFile[:])}
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		must(err)
	}
	return out
}
func hold(p process) process {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE|0x1000, false, p.PID)
	must(err)
	p.h = h
	var c, e, k, u syscall.Filetime
	must(syscall.GetProcessTimes(h, &c, &e, &k, &u))
	p.Created = time.Unix(0, c.Nanoseconds()).UTC().Format(time.RFC3339Nano)
	state, err := syscall.WaitForSingleObject(h, 0)
	must(err)
	if state != syscall.WAIT_TIMEOUT {
		panic("NOT_ALIVE")
	}
	return p
}
func save(name string, value any) {
	b, err := json.MarshalIndent(value, "", "  ")
	must(err)
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	must(err)
	_, err = f.Write(append(b, '\n'))
	must(err)
	must(f.Sync())
	must(f.Close())
}
func main() {
	if len(os.Args) != 2 {
		panic("RUN_MANIFEST_REQUIRED")
	}
	self, err := os.Executable()
	must(err)
	manifest, err := filepath.Abs(os.Args[1])
	must(err)
	rel, err := filepath.Rel(filepath.Dir(self), manifest)
	must(err)
	if strings.HasPrefix(rel, "..") || !strings.HasPrefix(rel, "tui-") || filepath.Base(manifest) != "run.json" {
		panic("UNOWNED_MANIFEST")
	}
	b, err := os.ReadFile(manifest)
	must(err)
	var run struct {
		PID     uint32    `json:"pid"`
		Binary  string    `json:"binary"`
		Started time.Time `json:"started"`
	}
	must(json.Unmarshal(b, &run))
	if run.Binary != "D:/AIDEV/clauduct-s36-build/clauduct.exe" || run.PID == 0 || time.Since(run.Started) > 15*time.Minute {
		panic("UNREVIEWED_LAUNCHER")
	}
	dir := filepath.Join(filepath.Dir(manifest), "os-ping-observation")
	must(os.Mkdir(dir, 0700))
	root, ok := snapshot()[run.PID]
	if !ok || !strings.EqualFold(root.Name, "clauduct.exe") {
		panic("LAUNCHER_MISSING")
	}
	root = hold(root)
	defer syscall.CloseHandle(root.h)
	created, err := time.Parse(time.RFC3339Nano, root.Created)
	must(err)
	if created.Before(run.Started.Add(-time.Second)) || created.After(run.Started.Add(10*time.Second)) {
		panic("LAUNCHER_IDENTITY")
	}
	var targets []process
	var ancestry []process
	deadline := time.Now().Add(120 * time.Second)
	for len(targets) == 0 && time.Now().Before(deadline) {
		all := snapshot()
		for _, p := range all {
			if !strings.EqualFold(p.Name, "ping.exe") {
				continue
			}
			chain := []process{p}
			seen := map[uint32]bool{p.PID: true}
			ancestor := p
			for len(chain) < 32 && ancestor.Parent != run.PID {
				next, exists := all[ancestor.Parent]
				if !exists || seen[next.PID] {
					break
				}
				seen[next.PID] = true
				chain = append(chain, next)
				ancestor = next
			}
			if ancestor.Parent != run.PID {
				continue
			}
			if len(targets) > 0 || len(chain) < 2 {
				panic("AMBIGUOUS_PING_TREE")
			}
			parent := chain[1]
			if parent.Name != "bash.exe" && parent.Name != "sh.exe" && parent.Name != "cmd.exe" {
				panic("UNEXPECTED_SHELL_PARENT")
			}
			// Observe the complete command-specific shell chain, stopping at native.
			for _, item := range chain {
				if strings.EqualFold(item.Name, "claude.exe") {
					break
				}
				if item.PID != p.PID && item.Name != "bash.exe" && item.Name != "sh.exe" && item.Name != "cmd.exe" {
					panic("UNEXPECTED_COMMAND_ANCESTOR")
				}
				targets = append(targets, hold(item))
			}
			ancestry = append(chain, root)
		}
		if len(targets) == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if len(targets) == 0 {
		panic("PING_START_TIMEOUT")
	}
	for _, p := range targets {
		defer syscall.CloseHandle(p.h)
	}
	observed := time.Now().UTC()
	save(filepath.Join(dir, "before.json"), map[string]any{"observed": observed, "targets": targets, "ancestry": ancestry, "observerCanTerminate": false, "identity": "held_process_handles"})
	fmt.Printf("OS_STARTED %s ping=%d shell=%d\n", observed.Format(time.RFC3339Nano), targets[0].PID, targets[1].PID)
	deadline = time.Now().Add(200 * time.Second)
	for time.Now().Before(deadline) {
		remaining := 0
		for i := range targets {
			if targets[i].Exited != "" {
				continue
			}
			state, err := syscall.WaitForSingleObject(targets[i].h, 0)
			must(err)
			if state == syscall.WAIT_OBJECT_0 {
				var c, e, k, u syscall.Filetime
				must(syscall.GetProcessTimes(targets[i].h, &c, &e, &k, &u))
				targets[i].Exited = time.Unix(0, e.Nanoseconds()).UTC().Format(time.RFC3339Nano)
			} else if state == syscall.WAIT_TIMEOUT {
				remaining++
			} else {
				panic("UNEXPECTED_WAIT_STATE")
			}
		}
		if remaining == 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	allExited := true
	for _, p := range targets {
		allExited = allExited && p.Exited != ""
	}
	save(filepath.Join(dir, "after.json"), map[string]any{"observed": time.Now().UTC(), "targets": targets, "allExited": allExited, "observerCanTerminate": false})
	fmt.Printf("OS_OBSERVATION_COMPLETE allExited=%v\n", allExited)
	if !allExited {
		os.Exit(1)
	}
}
