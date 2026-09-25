package upstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/wotjr1649/Clauduct/go/internal/wire"
)

// A harness creates one fresh directory with budget.json, then passes its absolute
// path to every product process. Only an absent setting disables this extra cap.
// The harness owns the directory; deleting claims or replacing the directory is
// not supported. No credential, prompt, endpoint or user path is written here.
const verificationBudgetEnv = "CLAUDUCT_VERIFICATION_BUDGET"

// VerificationBudgetVersion lets a harness reject older binaries before billing.
const VerificationBudgetVersion = 1

var errVerificationBudget = fmt.Errorf("VERIFICATION_BUDGET_INVALID: %w", ErrBudgetExhausted)
var verificationLabel = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

type verificationBudget struct {
	directory string
	plan      []byte // immutable during this ledger's lifetime
}

func verificationFromEnvironment() *verificationBudget {
	if directory, present := os.LookupEnv(verificationBudgetEnv); present {
		return &verificationBudget{directory: directory}
	}
	return nil
}

func (v *verificationBudget) reserve(a Attempt) error {
	if !filepath.IsAbs(v.directory) {
		return errVerificationBudget
	}
	root, err := os.OpenRoot(v.directory)
	if err != nil {
		return errVerificationBudget
	}
	defer root.Close()
	file, err := root.Open("budget.json")
	if err != nil {
		return errVerificationBudget
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 16<<10 {
		file.Close()
		return errVerificationBudget
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(raw) > 16<<10 || v.plan != nil && !bytes.Equal(v.plan, raw) {
		return errVerificationBudget
	}
	fields, err := wire.Fields(raw, []string{"version", "limit", "routes", "allowSearch"})
	if err != nil {
		return errVerificationBudget
	}
	var version, limit int
	var routes []json.RawMessage
	var allowSearch bool
	if value, present := fields["allowSearch"]; present {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &allowSearch) != nil {
			return errVerificationBudget
		}
	}
	if json.Unmarshal(fields["version"], &version) != nil || version != VerificationBudgetVersion ||
		json.Unmarshal(fields["limit"], &limit) != nil || limit < 1 || limit > 10000 ||
		json.Unmarshal(fields["routes"], &routes) != nil || len(routes) == 0 || len(routes) > 32 {
		return errVerificationBudget
	}
	authorised := false
	for _, route := range routes {
		fields, err := wire.Fields(route, []string{"model", "effort"})
		var model, effort string
		if err != nil || json.Unmarshal(fields["model"], &model) != nil || json.Unmarshal(fields["effort"], &effort) != nil ||
			!verificationLabel.MatchString(model) || !verificationLabel.MatchString(effort) {
			return errVerificationBudget
		}
		authorised = authorised || model == a.Model && effort == a.Effort
	}
	if a.Search {
		authorised = allowSearch
	}
	if !authorised {
		return ErrRouteNotAuthorised
	}
	v.plan = raw
	// O_EXCL is the cross-process reservation. A crash, partial write or failed
	// request never releases a slot. Sync and close finish before credentials/dial.
	// ponytail: at most 10000 slot probes; use a locked counter if that is measured slow.
	for slot := 1; slot <= limit; slot++ {
		claim, err := root.OpenFile(fmt.Sprintf("attempt-%05d.json", slot), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return errVerificationBudget
		}
		err = json.NewEncoder(claim).Encode(struct {
			Model, Effort string
			Count, Retry  bool
			PID           int
			Search        bool `json:",omitempty"`
		}{a.Model, a.Effort, a.CountOnly, a.Retry, os.Getpid(), a.Search})
		if err == nil {
			err = claim.Sync()
		}
		closeErr := claim.Close()
		if err != nil || closeErr != nil {
			return errVerificationBudget
		}
		return nil
	}
	return ErrBudgetExhausted
}

// Only explicit verification opt-in charges search to the shared total. The
// reservation precedes credentials, including the refresh read before a retry.
// Ordinary sessions retain separate search accounting and their existing policy.
func (l *Ledger) verificationSearch(retry bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.verification == nil {
		return nil
	}
	if !l.budget.authorises() || !l.budget.Unrestricted && l.attempts >= l.budget.Limit {
		l.refused++
		return ErrBudgetExhausted
	}
	if err := l.verification.reserve(Attempt{Search: true, Retry: retry}); err != nil {
		l.refused++
		return err
	}
	l.attempts++
	return nil
}
