package settingsfile

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/platform"
	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// Sync refusals. Each message is a fixed code plus, where one exists, the backup path; no
// message carries file contents.
var (
	// ErrSyncTarget means settings.json is not a regular file inside the home directory
	// (a directory, a link, a reparse point) or could not be read.
	ErrSyncTarget = errors.New("CLAUDUCT_SETTINGS_NOT_REGULAR")
	// ErrSyncInvalid means the document is not one strict JSON object, or is too large.
	ErrSyncInvalid = errors.New("CLAUDUCT_SETTINGS_INVALID")
	// ErrSyncVersion means "version" is absent or is not the integer 1.
	ErrSyncVersion = errors.New("CLAUDUCT_SETTINGS_UNSUPPORTED_VERSION")
	// ErrSyncBusy means another process holds the settings lock.
	ErrSyncBusy = errors.New("CLAUDUCT_SETTINGS_BUSY")
	// ErrSyncConflict means settings.json changed after it was read; it was not replaced.
	ErrSyncConflict = errors.New("CLAUDUCT_SETTINGS_CHANGED_DURING_SYNC")
	// ErrSyncWrite means the backup or the replacement could not be written or verified.
	ErrSyncWrite = errors.New("CLAUDUCT_SETTINGS_SYNC_WRITE_FAILED")
)

// SyncResult says what Sync did. A zero value with a nil error means nothing changed.
type SyncResult struct {
	Added   []string // top-level keys appended, in defaults order
	Backup  string   // absolute path of the copy of the original bytes; empty when none was made
	Created bool     // settings.json was absent and was created from the defaults
}

const (
	settingsDir    = ".clauduct"
	settingsTarget = settingsDir + "/settings.json"
	settingsLock   = settingsDir + "/settings.lock"
	// maxSettingsBytes matches the launcher's loader: a larger file is refused there too.
	maxSettingsBytes = 1 << 20
)

// testHook, when set, runs at the two moments a concurrent editor matters: "locked", just
// after the lock is taken and before the file is read again, and "replace", just before
// the comparison read that precedes the rename. Nil in the product.
var testHook func(point string)

// backupSuffix makes a backup name unique within one second. A variable so a test can
// force a collision and show that an existing file is never overwritten.
var backupSuffix = func() string { return strings.ToLower(rand.Text()[:8]) }

type member struct {
	key string
	raw json.RawMessage
}

// defaultMembers is the defaults document's top level in file order, values verbatim.
var defaultMembers = sync.OnceValues(func() ([]member, error) {
	dec := json.NewDecoder(strings.NewReader(defaults))
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		return nil, ErrSyncInvalid
	}
	var members []member
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, ErrSyncInvalid
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, ErrSyncInvalid
		}
		members = append(members, member{key, raw})
	}
	return members, nil
})

// Sync brings an existing settings.json up to the current set of top-level keys.
//
// A key the defaults have and the file lacks is appended with the defaults' value; nothing
// that is present is touched, whatever its value or type, and nested objects are not
// filled in. A file that is already complete is not opened for writing, locked or backed
// up. When something is added, the original bytes are first published as a new backup
// file, and the replacement is refused if settings.json changed since it was read.
//
// An absent file is created exactly as Ensure creates it.
func Sync(home string) (SyncResult, error) {
	var result SyncResult
	if !filepath.IsAbs(home) {
		return result, ErrSyncTarget
	}
	members, err := defaultMembers()
	if err != nil {
		return result, err
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return result, ErrSyncTarget
	}
	defer root.Close()

	if _, err := root.Lstat(settingsTarget); errors.Is(err, os.ErrNotExist) {
		if err := Ensure(home); err != nil {
			return result, err
		}
		// Carry on rather than return: if another writer published first, its file is
		// what is there now, and it gets the same treatment as any existing file.
		result.Created = true
	}

	original, missing, err := inspect(root, members)
	if err != nil || len(missing) == 0 {
		return result, err
	}

	lock, err := root.OpenFile(settingsLock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return result, ErrSyncBusy
	}
	defer lock.Close() // Closing releases the lock.
	if info, err := lock.Stat(); err != nil || !info.Mode().IsRegular() {
		return result, ErrSyncBusy
	}
	if err := platform.LockFile(lock); err != nil {
		return result, ErrSyncBusy
	}

	if testHook != nil {
		testHook("locked")
	}
	// The first read was a question; this one, under the lock, is the answer.
	original, missing, err = inspect(root, members)
	if err != nil || len(missing) == 0 {
		return result, err
	}

	backup, err := publishBackup(root, original)
	if err != nil {
		return result, err
	}
	result.Backup = filepath.Join(home, filepath.FromSlash(backup))

	updated := appendMembers(original, missing)
	fields, err := wire.Fields(updated, nil)
	if err != nil || len(updated) > maxSettingsBytes {
		return result, fmt.Errorf("%w: the completed document would not load; settings.json was not replaced; backup %s", ErrSyncInvalid, result.Backup)
	}
	for _, m := range missing {
		if _, ok := fields[m.key]; !ok {
			return result, fmt.Errorf("%w: settings.json was not replaced; backup %s", ErrSyncWrite, result.Backup)
		}
	}

	temp, err := writeTemp(root, updated)
	if err != nil {
		return result, fmt.Errorf("%w: settings.json was not replaced; backup %s", ErrSyncWrite, result.Backup)
	}
	defer root.Remove(temp) // A no-op once the rename has consumed it.

	if testHook != nil {
		testHook("replace")
	}
	if now, err := readRegular(root); err != nil || !bytes.Equal(now, original) {
		return result, fmt.Errorf("%w: settings.json was not replaced; backup %s", ErrSyncConflict, result.Backup)
	}
	// If the rename fails the destination keeps its previous contents.
	if err := root.Rename(temp, settingsTarget); err != nil {
		return result, fmt.Errorf("%w: settings.json was not replaced; backup %s", ErrSyncWrite, result.Backup)
	}
	if after, err := readRegular(root); err != nil || !bytes.Equal(after, updated) {
		return result, fmt.Errorf("%w: settings.json does not hold the completed document; original bytes in backup %s", ErrSyncWrite, result.Backup)
	}
	for _, m := range missing {
		result.Added = append(result.Added, m.key)
	}
	return result, nil
}

// inspect reads and validates settings.json and lists the default members it lacks.
func inspect(root *os.Root, members []member) ([]byte, []member, error) {
	raw, err := readRegular(root)
	if err != nil {
		return nil, nil, err
	}
	fields, err := wire.Fields(raw, nil)
	if err != nil {
		return nil, nil, ErrSyncInvalid
	}
	var version int
	if field, ok := fields["version"]; !ok || json.Unmarshal(field, &version) != nil || version != 1 {
		return nil, nil, ErrSyncVersion
	}
	var missing []member
	for _, m := range members {
		if _, present := fields[m.key]; !present {
			missing = append(missing, m)
		}
	}
	return raw, missing, nil
}

// readRegular reads settings.json only if it is a regular file of at most the loader limit.
func readRegular(root *os.Root) ([]byte, error) {
	info, err := root.Lstat(settingsTarget)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrSyncTarget
	}
	file, err := root.Open(settingsTarget)
	if err != nil {
		return nil, ErrSyncTarget
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, ErrSyncTarget
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxSettingsBytes+1))
	if err != nil {
		return nil, ErrSyncTarget
	}
	if len(raw) > maxSettingsBytes {
		return nil, fmt.Errorf("%w: larger than 1 MiB", ErrSyncInvalid)
	}
	return raw, nil
}

// writeTemp writes body to a new, exclusively created file beside settings.json and
// returns its root-relative name. The caller removes it.
func writeTemp(root *os.Root, body []byte) (string, error) {
	temp := settingsDir + "/settings-" + rand.Text() + ".tmp"
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(body)
	syncErr, closeErr := file.Sync(), file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		root.Remove(temp)
		return "", err
	}
	return temp, nil
}

// publishBackup stores the original bytes under a new name. Link refuses an existing
// destination, so no earlier backup or other file is ever overwritten.
func publishBackup(root *os.Root, original []byte) (string, error) {
	temp, err := writeTemp(root, original)
	if err != nil {
		return "", ErrSyncWrite
	}
	defer root.Remove(temp)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for range 3 {
		name := settingsDir + "/settings.backup-" + stamp + "-" + backupSuffix() + ".json"
		err := root.Link(temp, name)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			break
		}
	}
	return "", ErrSyncWrite
}

// appendMembers inserts the missing members after the last value of the top-level object,
// so the whitespace before the closing brace and everything after it stay as they were.
// The caller has validated original as one JSON object with at least one member.
func appendMembers(original []byte, missing []member) []byte {
	const space = " \t\r\n"
	body := bytes.TrimRight(original, space)
	closing := len(body) - 1 // the top-level '}'
	at := len(bytes.TrimRight(body[:closing], space))

	newline := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		newline = "\r\n"
	}
	var insert bytes.Buffer
	for _, m := range missing {
		// The embedded document's own line endings depend on how it was checked out.
		value := strings.ReplaceAll(string(m.raw), "\r\n", "\n")
		value = strings.ReplaceAll(value, "\n", newline)
		key, _ := json.Marshal(m.key)
		insert.WriteString("," + newline + "  " + string(key) + ": " + value)
	}
	out := make([]byte, 0, len(original)+insert.Len())
	out = append(out, original[:at]...)
	out = append(out, insert.Bytes()...)
	return append(out, original[at:]...)
}
