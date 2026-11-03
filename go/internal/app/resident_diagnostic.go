package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/Clauduct/go/internal/hookcmd"
)

var errResidentDiagnostic = errors.New("RESIDENT_DIAGNOSTIC_UNAVAILABLE")

func residentFailureCategory(category string) string {
	switch category {
	case "RESIDENT_START_FAILED", "RESIDENT_RECORD_UNAVAILABLE", "RESIDENT_RELAY_FAILED", "INVALID_SESSION_TIMEOUT":
		return category
	}
	return hookcmd.DiagnosticCategory(category)
}

func residentDiagnosticPath(link string) (string, error) {
	if !residentLink.MatchString(link) {
		return "", errResidentDiagnostic
	}
	dir := filepath.Join(StatusDir(), "resident-restore")
	path := filepath.Join(dir, link+".txt")
	for _, name := range []string{StatusDir(), dir, path} {
		info, err := os.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 ||
			name == path && !info.Mode().IsRegular() || name != path && !info.IsDir() {
			return "", errResidentDiagnostic
		}
	}
	return path, nil
}

// WriteResidentRestoreFailure persists only an admitted category, including pre-native failures.
func WriteResidentRestoreFailure(link, category string) error {
	path, err := residentDiagnosticPath(link)
	if err != nil || residentFailureCategory(category) == "" {
		return errResidentDiagnostic
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return errResidentDiagnostic
	}
	file, err := os.CreateTemp(filepath.Dir(path), link+"-*.tmp")
	if err != nil {
		return errResidentDiagnostic
	}
	defer os.Remove(file.Name())
	_, writeErr := io.WriteString(file, category+"\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || os.Rename(file.Name(), path) != nil {
		return errResidentDiagnostic
	}
	return nil
}

// A diagnostic lasts until consumption, observed link success, or the next attempt for this link.
// There is no directory scan or age-based deletion.
func clearResidentRestoreFailure(link string) error {
	path, err := residentDiagnosticPath(link)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errResidentDiagnostic
	}
	return nil
}

// ReportResidentRestoreFailure consumes this link's file without echoing untrusted contents.
func ReportResidentRestoreFailure(link string, errOut io.Writer) {
	path, err := residentDiagnosticPath(link)
	if err == nil {
		var file *os.File
		file, err = os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err == nil {
			raw, readErr := io.ReadAll(io.LimitReader(file, 128))
			closeErr := file.Close()
			category := strings.TrimSuffix(string(raw), "\n")
			clearErr := clearResidentRestoreFailure(link)
			if readErr == nil && closeErr == nil && clearErr == nil && residentFailureCategory(category) != "" {
				fmt.Fprintln(errOut, "CLAUDUCT_RESIDENT_RESTORE_FAILED: "+category)
				return
			}
		}
	}
	fmt.Fprintln(errOut, "clauduct: RESIDENT_DIAGNOSTIC_UNAVAILABLE")
}
