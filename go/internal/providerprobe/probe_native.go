package providerprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/wotjr1649/Clauduct/go/internal/app"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wotjr1649/Clauduct/go/internal/childprocess"
	"github.com/wotjr1649/Clauduct/go/internal/launch"
	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/providercert"
)

var queryImageName = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

// imagePath is the executable a live process was created from.
func imagePath(pid int) (string, error) {
	h, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if ok, _, err := queryImageName.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return "", err
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

// pinnedStarter starts the real native under the contract's spawn pin: the executable is held
// against write, delete and rename from the last hash until the new process's image is
// confirmed to be the pinned file. The pinned identity is reported as a checkpoint; a child
// whose image cannot be confirmed is ended and reported, never accepted.
func pinnedStarter(report spawnReport) starter {
	return func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (app.Process, error) {
		pin, err := providercert.PinFile(spec.File)
		if err != nil {
			report(providercert.Identity{}, fmt.Errorf("PRE_SPAWN pin: %w", err))
			return nil, err
		}
		defer pin.Close()
		process, err := startOSProcess(spec, stdin, stdout, stderr)
		if err != nil {
			return nil, err
		}
		owned, ok := process.(*childprocess.Process)
		if !ok {
			process.Stop()
			report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: unexpected process type"))
			return nil, fmt.Errorf("POST_SPAWN unverified")
		}
		image, err := imagePath(owned.PID())
		if err == nil {
			var same bool
			if same, err = pin.SameFile(image); err == nil && !same {
				err = fmt.Errorf("image is not the pinned executable")
			}
		}
		if err != nil {
			process.Stop()
			report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: %w", err))
			return nil, fmt.Errorf("POST_SPAWN unverified")
		}
		report(pin.Identity, nil)
		return process, nil
	}
}

// userStateStat records only the size and modification time of the user's real Claude Code
// auth and settings files and the Anthropic profile directory, never their contents, so a run
// can show it changed none of them.
func userStateStat(t TB) map[string]string {
	home, err := platform.OSUserHome()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for name, path := range map[string]string{
		"claude/.credentials.json": filepath.Join(home, ".claude", ".credentials.json"),
		"claude/settings.json":     filepath.Join(home, ".claude", "settings.json"),
		".claude.json":             filepath.Join(home, ".claude.json"),
		"APPDATA/Anthropic":        filepath.Join(os.Getenv("APPDATA"), "Anthropic"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			out[name] = "absent"
			continue
		}
		out[name] = fmt.Sprintf("%d/%d", info.Size(), info.ModTime().UnixNano())
	}
	return out
}

// p3BuildIdentity is the Clauduct build under test: the product binary the hooks and the
// confirmation helper run (clauductBinarySHA256 in the contract's cache key) and its source.
func p3BuildIdentity(t TB) map[string]any {
	product, err := executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(product)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	dirty, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=no", "--", ".").Output()
	return map[string]any{"clauductBinarySHA256": hex.EncodeToString(sum[:]), "sourceCommit": strings.TrimSpace(string(commit)), "trackedChanges": len(strings.TrimSpace(string(dirty))) > 0}
}
