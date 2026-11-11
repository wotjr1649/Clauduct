package app

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

// What the dedicated config folder shares with the user's own (#379), and how. This table is the
// one place that decides it: moving plugins from a junction to native's CLAUDE_CODE_PLUGIN_CACHE_DIR
// (open question Q10) is a change here. Anything not named is not shared: login, supervisor
// state and the caches that belong to an account stay in the dedicated folder only.
type sharedKind int

const (
	sharedJunction sharedKind = iota // a directory junction to the same name in the user's folder
	sharedHardLink                   // a file hard link (a junction cannot name a file)
)

type sharedEntry struct {
	name string
	kind sharedKind
}

var sharedEntries = []sharedEntry{
	{"rules", sharedJunction}, {"agents", sharedJunction}, {"commands", sharedJunction},
	{"skills", sharedJunction}, {"output-styles", sharedJunction},
	{"workflows", sharedJunction}, {"themes", sharedJunction}, {"agent-memory", sharedJunction},
	{"plans", sharedJunction}, {"tasks", sharedJunction}, {"todos", sharedJunction},
	{"plugins", sharedJunction},
	{"projects", sharedJunction}, {"file-history", sharedJunction}, {"paste-cache", sharedJunction},
	{"history.jsonl", sharedHardLink},
}

// ErrIsolationLink refuses a shared name that is already something else in the dedicated folder.
// Nothing there is moved or deleted: it may be the user's data.
type ErrIsolationLink struct{ Name string }

func (e ErrIsolationLink) Error() string {
	return fmt.Sprintf("ISOLATION_LINK_CONFLICT: %s in the isolated configuration folder is not the link isolated mode makes; nothing was changed", e.Name)
}

// linkReport says what is shared, by name only.
type linkReport struct {
	Shared  []string
	History string // "linked", "absent" (nothing to link yet) or "unlinked" (a separate file)
}

const (
	fsctlSetReparsePoint = 0x000900A4
	ioReparseTagMount    = 0xA0000003
)

// isJunction reports whether path is a directory junction and, if so, where it points.
func isJunction(path string) (target string, ok bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular == 0 {
		return "", false
	}
	target, err = os.Readlink(path)
	return target, err == nil
}

// makeJunction creates link as a junction to the existing directory target.
func makeJunction(link, target string) error {
	if _, err := junctionTarget(target); err != nil {
		return err
	}
	if err := os.Mkdir(link, 0o700); err != nil {
		return err
	}
	if err := setJunction(link, target); err != nil {
		os.Remove(link)
		return err
	}
	return nil
}

// ErrIsolationLinkTarget refuses a junction target that Windows cannot make a junction to.
var ErrIsolationLinkTarget = errors.New("ISOLATION_LINK_TARGET_UNSUPPORTED: your configuration folder is on a network share or another path a directory junction cannot point to; isolated mode needs it on a local drive (a drive-letter path)")

// junctionTarget is the absolute drive-letter path a junction can name. A junction (a mount-point
// reparse point) holds a local volume path only: a UNC target (\\server\share\...) has no valid
// \??\UNC\... form that Windows accepts for one, so it is refused with a code rather than made into
// a link that cannot be opened. An extended-length local path (\\?\C:\...) is the same folder as its
// plain form and is accepted.
func junctionTarget(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if rest, extended := strings.CutPrefix(abs, `\\?\`); extended {
		abs = rest
	}
	if len(abs) < 3 || abs[1] != ':' || abs[2] != '\\' || !(abs[0] >= 'A' && abs[0] <= 'Z' || abs[0] >= 'a' && abs[0] <= 'z') {
		return "", ErrIsolationLinkTarget
	}
	return abs, nil
}

func setJunction(link, target string) error {
	target, err := junctionTarget(target)
	if err != nil {
		return err
	}
	name := utf16.Encode([]rune(`\??\` + target))
	shown := utf16.Encode([]rune(target))
	// REPARSE_DATA_BUFFER for a mount point: tag, data length, reserved, then four name fields and
	// the two NUL-terminated names.
	path := make([]uint16, 0, len(name)+len(shown)+2)
	path = append(append(append(append(path, name...), 0), shown...), 0)
	data := 8 + len(path)*2
	buf := make([]byte, 8+data)
	binary.LittleEndian.PutUint32(buf[0:], ioReparseTagMount)
	binary.LittleEndian.PutUint16(buf[4:], uint16(data))
	binary.LittleEndian.PutUint16(buf[8:], 0)
	binary.LittleEndian.PutUint16(buf[10:], uint16(len(name)*2))
	binary.LittleEndian.PutUint16(buf[12:], uint16(len(name)*2+2))
	binary.LittleEndian.PutUint16(buf[14:], uint16(len(shown)*2))
	for i, c := range path {
		binary.LittleEndian.PutUint16(buf[16+i*2:], c)
	}
	p, err := syscall.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	var returned uint32
	return syscall.DeviceIoControl(h, fsctlSetReparsePoint, &buf[0], uint32(len(buf)), nil, 0, &returned, nil)
}

// linkShared makes the dedicated folder share what sharedEntries names with source. It is
// idempotent. A name that is already a different thing in the dedicated folder is a conflict, and
// a user's folder that lacks a shared directory gets an empty one, so what an isolated session
// creates there lands in the user's folder rather than in the dedicated one.
func linkShared(dedicated, source string) (linkReport, error) {
	report := linkReport{History: "absent"}
	if err := os.MkdirAll(source, 0o700); err != nil {
		return report, err
	}
	for _, entry := range sharedEntries {
		from, to := filepath.Join(source, entry.name), filepath.Join(dedicated, entry.name)
		switch entry.kind {
		case sharedJunction:
			if info, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
				if err := os.Mkdir(from, 0o700); err != nil {
					return report, err
				}
			} else if err != nil || !info.IsDir() {
				return report, ErrIsolationLink{entry.name}
			}
			if target, ok := isJunction(to); ok {
				if !sameFolder(target, from) {
					return report, ErrIsolationLink{entry.name}
				}
			} else if _, err := os.Lstat(to); err == nil {
				return report, ErrIsolationLink{entry.name}
			} else if !errors.Is(err, os.ErrNotExist) {
				return report, err
			} else if err := makeJunction(to, from); err != nil {
				return report, err
			}
			report.Shared = append(report.Shared, entry.name)
		case sharedHardLink:
			report.History = linkHistory(from, to)
		}
	}
	return report, nil
}

// linkHistory hard-links the prompt history. It never merges two files: one that has already
// diverged is reported, not touched, and nothing is created in the user's folder for it.
func linkHistory(from, to string) string {
	src, err := os.Stat(from)
	if err != nil || !src.Mode().IsRegular() {
		if _, lerr := os.Lstat(to); lerr == nil {
			return "unlinked"
		}
		return "absent"
	}
	if dst, err := os.Stat(to); err == nil {
		if os.SameFile(src, dst) {
			return "linked"
		}
		return "unlinked"
	} else if !errors.Is(err, os.ErrNotExist) {
		return "unlinked"
	}
	if os.Link(from, to) != nil {
		return "unlinked"
	}
	return "linked"
}

// removeSharedLinks removes the links linkShared made and nothing else, so that deleting the
// dedicated folder afterwards cannot reach the user's folder. A junction is removed as a link
// (its target is never opened); a name that is a real directory is left alone and reported.
func removeSharedLinks(dedicated string) (left []string, err error) {
	for _, entry := range sharedEntries {
		to := filepath.Join(dedicated, entry.name)
		switch entry.kind {
		case sharedJunction:
			if _, ok := isJunction(to); ok {
				if rerr := os.Remove(to); rerr != nil {
					err = errors.Join(err, rerr)
				}
			} else if _, lerr := os.Lstat(to); lerr == nil {
				left = append(left, entry.name)
			}
		case sharedHardLink:
			// Removing one name of a hard link leaves the other; the user's file is untouched.
			if rerr := os.Remove(to); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
				err = errors.Join(err, rerr)
			}
		}
	}
	return left, err
}
