package providerprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// p3Native is a started native and the generation its starter linked to the pinned image: the
// direct native, or the Claude process a TUI driver started (tuiDiscover). pid and created are
// evidence, and RP01's owner (rp01_run.go); a start without a linked generation is not a p3Native.
type p3Native struct {
	*childprocess.Process
	pid     int
	created uint64 // Windows FILETIME ticks
}

// errP3PartialStart is a start refused after its process existed: the process was ended, and its
// Job is not confirmed empty (the run is unreaped).
var errP3PartialStart = errors.New("POST_SPAWN unverified: the started process was ended")

// pinnedStarter starts the real native and links the new process to the exact image it runs. Nothing
// is held against native's updater: the image is observed with full sharing before the spawn and
// the live process is linked to its mapped image afterwards (providercert.LinkProcess). The
// identity observed before the spawn is reported as the checkpoint only when the process runs
// exactly that file; a child that cannot be linked, or runs another image, is ended and
// reported, never accepted.
func pinnedStarter(report spawnReport) starter {
	return func(spec launch.Spec, stdin io.Reader, stdout, stderr io.Writer) (app.Process, error) {
		catalog, err := providercert.CatalogOf(spec.File)
		if err != nil {
			report(providercert.Identity{}, fmt.Errorf("PRE_SPAWN catalog: %w", err))
			return nil, err
		}
		before, err := providercert.Observe(spec.File)
		if err != nil {
			report(providercert.Identity{}, fmt.Errorf("PRE_SPAWN observe: %w", err))
			return nil, err
		}
		if _, ok := catalog.Version(before.SHA256); !ok {
			err := fmt.Errorf("%w: native bytes are not one catalogued version", providercert.ErrInconclusiveNativeIdentity)
			report(providercert.Identity{}, fmt.Errorf("PRE_SPAWN catalog: %w", err))
			return nil, err
		}
		process, err := startOSProcess(spec, stdin, stdout, stderr)
		if err != nil {
			return nil, err
		}
		owned, ok := process.(*childprocess.Process)
		if !ok {
			process.Stop()
			process.Wait()
			report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: unexpected process type"))
			return nil, errP3PartialStart
		}
		// The file was hashed before the spawn; the process is bound to that file object by its
		// mapped image, with no hash after the spawn, so a child that finishes quickly is not lost.
		switch _, err := providercert.LinkSpawned(uint32(owned.PID()), 0, before); {
		case err == nil:
			report(before, nil)
			if id, err := owned.Identity(); err == nil && id.InOwnedJob {
				return &p3Native{Process: owned, pid: id.PID, created: id.Creation}, nil
			}
		case errors.Is(err, providercert.ErrNativeChanged):
			// Ended and waited for, so its Job handle is released; the run stays unconfirmed.
			process.Stop()
			process.Wait()
			report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: %w", err))
			return nil, errP3PartialStart
		default:
			// Not linkable (it was gone before its image could be read): the diagnostic still
			// runs, but the checkpoint is missing, so the run is INCONCLUSIVE_NATIVE_IDENTITY and
			// cannot show P6. It is never reported as a verified image.
			report(providercert.Identity{}, fmt.Errorf("POST_SPAWN: %w", err))
		}
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
