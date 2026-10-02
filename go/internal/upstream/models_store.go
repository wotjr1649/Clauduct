package upstream

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// The last model list that validated, kept per Clauduct home so a failed read can fall back
// to it. It holds the account key, the client version, the time and the reduced models --
// nothing else from the reply, and never a token or an account id.

const (
	accountModelsDir     = ".clauduct"
	accountModelsFile    = accountModelsDir + "/account-models.json"
	accountModelsVersion = 1
	maxStoredModelsBytes = 1 << 20
)

var accountKeyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Refusals from the last-good store. A caller treats every one of them as "no list"; they
// are distinct so a diagnostic can say which.
var (
	// ErrAccountModelsUnavailable means nothing is stored.
	ErrAccountModelsUnavailable = errors.New("ACCOUNT_MODELS_UNAVAILABLE")
	// ErrAccountModelsOtherAccount means what is stored belongs to a different account, or
	// to one that cannot be confirmed.
	ErrAccountModelsOtherAccount = errors.New("ACCOUNT_MODELS_OTHER_ACCOUNT")
	// ErrAccountModelsVersion means the file is a format this build does not read.
	ErrAccountModelsVersion = errors.New("ACCOUNT_MODELS_VERSION")
	// ErrAccountModelsInvalid means the file is not a document this build wrote: malformed,
	// repeated keys, oversized, not a regular file, or model data that does not validate.
	ErrAccountModelsInvalid = errors.New("ACCOUNT_MODELS_INVALID")
	// ErrAccountModelsStore means the list could not be saved.
	ErrAccountModelsStore = errors.New("ACCOUNT_MODELS_STORE_FAILED")
)

type storedAccountModels struct {
	Version       int                  `json:"version"`
	Account       string               `json:"account"`
	ClientVersion string               `json:"clientVersion"`
	FetchedAt     string               `json:"fetchedAt"`
	Models        []storedAccountModel `json:"models"`
}

type storedAccountModel struct {
	ID      string   `json:"id"`
	Efforts []string `json:"efforts"`
	Default string   `json:"default"`
	Visible bool     `json:"visible"`
}

// SaveAccountModels replaces the stored list with m, under home/.clauduct. The file is
// written whole to a new temporary file, synced, and renamed over the old one; a target that
// is not a regular file is refused rather than replaced.
func SaveAccountModels(home string, m AccountModels) error {
	if !filepath.IsAbs(home) || !accountKeyShape.MatchString(m.Account) ||
		!storedWhole.MatchString(m.ClientVersion) || m.FetchedAt.IsZero() ||
		validateAccountModels(m.Models) != nil {
		return ErrAccountModelsStore
	}
	doc := storedAccountModels{
		Version:       accountModelsVersion,
		Account:       m.Account,
		ClientVersion: m.ClientVersion,
		FetchedAt:     m.FetchedAt.UTC().Format(time.RFC3339),
		Models:        make([]storedAccountModel, 0, len(m.Models)),
	}
	for _, model := range m.Models {
		efforts := append(make([]string, 0, len(model.Efforts)), model.Efforts...)
		doc.Models = append(doc.Models, storedAccountModel{ID: model.ID, Efforts: efforts, Default: model.Default, Visible: model.Visible})
	}
	encoded, err := json.Marshal(doc)
	if err != nil || len(encoded) > maxStoredModelsBytes {
		return ErrAccountModelsStore
	}

	root, err := os.OpenRoot(home)
	if err != nil {
		return ErrAccountModelsStore
	}
	defer root.Close()
	if err := root.MkdirAll(accountModelsDir, 0o700); err != nil {
		return ErrAccountModelsStore
	}
	if info, err := root.Lstat(accountModelsDir); err != nil || !info.IsDir() {
		return ErrAccountModelsStore
	}
	if err := refuseIrregular(root); err != nil {
		return err
	}
	temp := accountModelsDir + "/account-models-" + rand.Text() + ".tmp"
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ErrAccountModelsStore
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(encoded)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrAccountModelsStore
	}
	// Checked again just before the replace: a target that turned into something else
	// since is still refused.
	if err := refuseIrregular(root); err != nil {
		return err
	}
	if err := root.Rename(temp, accountModelsFile); err != nil {
		return ErrAccountModelsStore
	}
	return nil
}

// refuseIrregular allows a missing target or a regular file, nothing else.
func refuseIrregular(root *os.Root) error {
	info, err := root.Lstat(accountModelsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrAccountModelsStore
	}
	return nil
}

// LoadAccountModels returns the stored list for account, an AccountKey. A list is returned
// only when the file is one this build wrote and names that account.
func LoadAccountModels(home string, account string) (AccountModels, error) {
	if !filepath.IsAbs(home) {
		return AccountModels{}, ErrAccountModelsUnavailable
	}
	if !accountKeyShape.MatchString(account) {
		return AccountModels{}, ErrAccountModelsOtherAccount
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return AccountModels{}, ErrAccountModelsUnavailable
	}
	defer root.Close()
	info, err := root.Lstat(accountModelsFile)
	if errors.Is(err, os.ErrNotExist) {
		return AccountModels{}, ErrAccountModelsUnavailable
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxStoredModelsBytes {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	file, err := root.Open(accountModelsFile)
	if err != nil {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maxStoredModelsBytes {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxStoredModelsBytes+1))
	if err != nil || len(raw) > maxStoredModelsBytes {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	return decodeStoredAccountModels(raw, account)
}

func decodeStoredAccountModels(raw []byte, account string) (AccountModels, error) {
	fields, err := wire.Fields(raw, []string{"version", "account", "clientVersion", "fetchedAt", "models"})
	if err != nil {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	var version int
	if !storedValue(fields, "version", &version) {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	if version != accountModelsVersion {
		return AccountModels{}, ErrAccountModelsVersion
	}
	var stored string
	if !storedValue(fields, "account", &stored) || !accountKeyShape.MatchString(stored) {
		return AccountModels{}, ErrAccountModelsOtherAccount
	}
	if stored != account {
		return AccountModels{}, ErrAccountModelsOtherAccount
	}
	out := AccountModels{Account: stored}
	var fetched string
	if !storedValue(fields, "clientVersion", &out.ClientVersion) || !storedWhole.MatchString(out.ClientVersion) ||
		!storedValue(fields, "fetchedAt", &fetched) {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	if out.FetchedAt, err = time.Parse(time.RFC3339, fetched); err != nil {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	var entries []json.RawMessage
	if !storedValue(fields, "models", &entries) {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	for _, entry := range entries {
		modelFields, err := wire.Fields(entry, []string{"id", "efforts", "default", "visible"})
		var model AccountModel
		if err != nil || !storedValue(modelFields, "id", &model.ID) || !storedValue(modelFields, "efforts", &model.Efforts) ||
			!storedValue(modelFields, "default", &model.Default) || !storedValue(modelFields, "visible", &model.Visible) {
			return AccountModels{}, ErrAccountModelsInvalid
		}
		out.Models = append(out.Models, model)
	}
	if validateAccountModels(out.Models) != nil {
		return AccountModels{}, ErrAccountModelsInvalid
	}
	return out, nil
}

// storedValue decodes a required, non-null field into target.
func storedValue(fields map[string]json.RawMessage, name string, target any) bool {
	value, presence := wire.Of(fields, name)
	if presence != wire.Present {
		return false
	}
	return json.Unmarshal(value, target) == nil
}
