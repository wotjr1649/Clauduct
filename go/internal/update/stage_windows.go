package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	journalName = "clauduct.exe.update-journal"
	cleanupName = "clauduct.exe.update-cleanup"
	stagePrefix = "clauduct-update-"
	stageSuffix = ".update-new"
)

type stagedPair struct {
	Directory string
	Exe       string
	// The v0.6.19 updater could also journal a probe and its old hard link.
	Probe         string        `json:",omitempty"`
	PreviousProbe *fileIdentity `json:",omitempty"`
}

var writeStageFile = writeSynced

func stagePair(dir string, exe []byte) (pair stagedPair, err error) {
	if err := refuseUnmarkedStages(dir); err != nil {
		return pair, err
	}
	for _, name := range []string{journalName, cleanupName} {
		path := filepath.Join(dir, name)
		if mayExist(path) {
			return pair, recoveryError(fmt.Errorf("%s already exists", path))
		}
	}
	journal := filepath.Join(dir, journalName)
	work, err := os.MkdirTemp(dir, stagePrefix)
	if err != nil {
		return pair, err
	}
	// Before publication only this invocation knows which entries it created.
	var created []string
	defer func() {
		if err != nil {
			for _, path := range created {
				err = errors.Join(err, removeFile(path))
			}
			_ = removeStageDir(work)
		}
	}()

	pair.Directory = filepath.Base(work)
	pair.Exe = Digest(exe)
	if err = writeStageFile(pair.stage(dir, Binaries[0]), exe, &created); err != nil {
		return pair, err
	}
	record, err := json.Marshal(pair)
	if err != nil {
		return pair, err
	}
	recordPath := filepath.Join(work, journalName)
	if err = writeSynced(recordPath, record, &created); err != nil {
		return pair, err
	}
	// Publish only complete files and never replace a journal that appeared during staging.
	return pair, moveNewFile(recordPath, journal)
}

func writeSynced(path string, body []byte, created *[]string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	*created = append(*created, path)
	_, err = file.Write(body)
	return errors.Join(err, file.Sync(), file.Close())
}

func moveNewFile(from, to string) error {
	src, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	if err := syscall.MoveFile(src, dst); err != nil {
		return &os.LinkError{Op: "publish", Old: from, New: to, Err: err}
	}
	return nil
}

func refuseUnmarkedStages(dir string) error {
	for _, name := range []string{Binaries[0], Probe} {
		path := filepath.Join(dir, name+stageSuffix)
		if mayExist(path) {
			return recoveryError(fmt.Errorf("unrecognized staging entry %s", path))
		}
	}
	return nil
}

func readStagedPair(dir, recordName string) (stagedPair, error) {
	path := filepath.Join(dir, recordName)
	var pair stagedPair
	record, err := readRegularFile(path, 1024)
	if err != nil {
		return pair, err
	}
	if err := json.Unmarshal(record, &pair); err != nil {
		return pair, fmt.Errorf("unrecognized update journal %s: %w", path, err)
	}
	if !isStageDirectoryName(pair.Directory) ||
		!validSHA256(pair.Exe) || (pair.Probe != "" && !validSHA256(pair.Probe)) {
		return pair, fmt.Errorf("unrecognized update journal %s", path)
	}

	return pair, nil
}

func isStageDirectoryName(name string) bool {
	suffix, ok := strings.CutPrefix(name, stagePrefix)
	return ok && suffix != "" && strings.Trim(suffix, "0123456789") == ""
}

func (pair stagedPair) verify(dir string) error {
	work := filepath.Join(dir, pair.Directory)
	info, err := os.Lstat(work)
	if err != nil {
		return err
	}
	if !info.IsDir() || reparsePoint(info) {
		return fmt.Errorf("staging path is not a directory: %s", work)
	}
	entries, err := os.ReadDir(work)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var expected string
		switch entry.Name() {
		case Binaries[0] + stageSuffix:
			expected = pair.Exe
		case Probe + stageSuffix:
			expected = pair.Probe
		}
		path := filepath.Join(work, entry.Name())
		if expected == "" || !binaryMatches(path, expected) {
			return fmt.Errorf("unrecognized or damaged staging entry %s", path)
		}
	}
	return nil
}

func validSHA256(digest string) bool {
	return len(digest) == 64 && strings.Trim(digest, "0123456789abcdef") == ""
}

// Record the cleanup decision before removing any payloads needed to tell which swap ran.
func (pair stagedPair) discard(dir string) error {
	if err := pair.verify(dir); err != nil {
		return err
	}
	if err := moveNewFile(filepath.Join(dir, journalName), filepath.Join(dir, cleanupName)); err != nil {
		return err
	}
	return pair.cleanup(dir)
}

func (pair stagedPair) cleanup(dir string) error {
	if pair.Probe != "" {
		if err := removeFile(pair.stage(dir, Probe)); err != nil {
			return err
		}
	}
	if err := removeFile(pair.stage(dir, Binaries[0])); err != nil {
		return err
	}
	if err := removeFile(filepath.Join(dir, cleanupName)); err != nil {
		return err
	}
	_ = removeStageDir(filepath.Join(dir, pair.Directory))
	return nil
}

func removeStageDir(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.RemoveDirectory(name)
}

type fileIdentity struct {
	Volume, IndexHigh, IndexLow uint32
}

func reparsePoint(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func (pair stagedPair) stage(dir, name string) string {
	return filepath.Join(dir, pair.Directory, name+stageSuffix)
}

func binaryMatches(path, expected string) bool {
	data, err := readRegularFile(path, maxBinaryBytes)
	return err == nil && Digest(data) == expected
}

func readRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || reparsePoint(info) || info.Size() > limit {
		return nil, fmt.Errorf("not a regular file within size limit: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && int64(len(body)) > limit {
		return nil, fmt.Errorf("file exceeds size limit: %s", path)
	}
	return body, err
}
