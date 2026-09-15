// Package auth reads the Codex credential the user's own login produced.
//
// Read-only, and that is a contract rather than a convention: this package never writes to
// the credential file, never writes to the config, and never refreshes or re-logs-in.
// Keeping a credential valid belongs to the user and to the Codex tool. What belongs here
// is reading one without damaging it, refusing one that cannot be trusted, and never
// letting its bytes reach a log, an error or a response.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Bounds. A credential file larger than this is not a credential file.
const (
	maxConfigBytes     = 64 * 1024
	maxCredentialBytes = 64 * 1024
	maxTokenBytes      = 16000
	maxKeyParts        = 64
	maxValueBytes      = 1024
	maxNesting         = 64
	storeKey           = "cli_auth_credentials_store"

	// A token about to expire is treated as expired. Sixty seconds is the baseline's
	// margin: a request that starts valid and finishes expired is a failure the user sees
	// as a mystery.
	expiryMargin = 60 * time.Second
)

// Fixed categories. A caller branches on these; the text is never assembled from file
// content, so no category can carry a credential out with it.
const (
	CategoryStoreUnsupported   = "CREDENTIAL_STORE_UNSUPPORTED"
	CategoryConfigUnsupported  = "CONFIG_UNSUPPORTED"
	CategoryFileUnavailable    = "FILE_CACHE_UNAVAILABLE"
	CategoryFileTooLarge       = "FILE_TOO_LARGE"
	CategoryInvalidCache       = "INVALID_AUTH_CACHE"
	CategoryTokenExpired       = "TOKEN_EXPIRED"
	CategoryUnavailable        = "CREDENTIAL_UNAVAILABLE_OR_EXPIRED"
	CategoryAccountChanged     = "CREDENTIAL_ACCOUNT_CHANGED"
	CategoryUnexpectedHome     = "UNEXPECTED_CODEX_HOME"
	CategoryRuntimeUnsupported = "TRANSPORT_RUNTIME_UNSUPPORTED"
)

// Error is a refusal carrying a fixed category.
type Error struct{ Category string }

func (e *Error) Error() string { return e.Category }

func errorOf(category string) error { return &Error{Category: category} }

// CategoryOf reports the category of an error, or "" if it is not one of this package's.
func CategoryOf(err error) string {
	var refusal *Error
	if errors.As(err, &refusal) {
		return refusal.Category
	}
	return ""
}

// Credential is what a request needs to authenticate. It is deliberately small: the token,
// the account it belongs to, and when it stops being usable.
type Credential struct {
	accessToken string
	Account     string
	ExpiresAt   time.Time

	// Synthetic marks a credential that came from a test store. A transport that replays
	// fixtures refuses a real one and a real transport refuses a synthetic one, so the two
	// cannot be mixed by accident in either direction.
	Synthetic bool
}

// Token returns the secret. It is a method rather than a field so that every use is a
// visible call site, and so no struct printing can reach it.
func (c Credential) Token() string { return c.accessToken }

// String is redacted. A credential in a log line, an error, or a debugger dump is the
// failure this whole package exists to prevent, and Go prints structs readily.
func (c Credential) String() string {
	return fmt.Sprintf("auth.Credential{account:%s expires:%s synthetic:%t token:<redacted %d bytes>}",
		c.Account, c.ExpiresAt.UTC().Format(time.RFC3339), c.Synthetic, len(c.accessToken))
}

// GoString keeps %#v redacted too.
func (c Credential) GoString() string { return c.String() }

var (
	// The shape of a JWT: three base64url segments. Checked before the payload is decoded
	// so a malformed value is refused rather than partially interpreted.
	jwtShape     = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
	accountShape = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

// Provider reads a credential on demand.
//
// It is lazy by design: a process that only ran --help must not have opened a credential
// file, and a session that never sends a request must not need a login.
type Provider struct {
	// Home is the Codex home directory. Empty resolves it from the OS user.
	Home string
	// Now supplies the current time. Zero uses the clock.
	Now func() time.Time
	// ReadFile reads a file. Zero uses a bounded reader over the real filesystem.
	ReadFile func(path string) ([]byte, error)
	// Environ is the environment to inspect for unsupported runtime settings.
	Environ map[string]string
	// Synthetic marks every credential this provider produces as a test credential.
	Synthetic bool

	// boundAccount is the account of the first credential read. A later read that reports
	// a different one is refused rather than followed.
	boundAccount string
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// OSCodexHome is the Codex directory belonging to the OS user.
//
// It deliberately ignores CODEX_HOME. Following that variable would let anyone who can set
// one environment value choose which credential file this process reads, and the file it
// points at is the thing being sent to a backend.
func OSCodexHome() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(u.HomeDir, ".codex"), nil
}

func (p *Provider) home() (string, error) {
	if p.Home != "" {
		return p.Home, nil
	}
	return OSCodexHome()
}

// CheckRuntime refuses to proceed when the process environment could compromise the
// transport that is about to carry a credential.
//
// SSLKEYLOGFILE is the one that matters most: it asks the TLS stack to write session keys
// to a file, and anything holding that file can decrypt the traffic afterwards. A proxy is
// left alone — the handoff treats a user's proxy and CA configuration as their own network,
// and this build never relaxes certificate verification whatever the network looks like.
func (p *Provider) CheckRuntime() error {
	for name, value := range p.environ() {
		if !strings.EqualFold(name, "SSLKEYLOGFILE") {
			continue
		}
		// An empty value is not a request to log anything.
		if value != "" {
			return errorOf(CategoryRuntimeUnsupported)
		}
	}
	return nil
}

func (p *Provider) environ() map[string]string {
	if p.Environ != nil {
		return p.Environ
	}
	out := make(map[string]string, len(os.Environ()))
	for _, entry := range os.Environ() {
		if name, value, ok := strings.Cut(entry, "="); ok {
			out[name] = value
		}
	}
	return out
}

// CheckHome refuses a redirected Codex home rather than silently reading a different
// account's credential.
func (p *Provider) CheckHome() error {
	expected, err := OSCodexHome()
	if err != nil {
		return errorOf(CategoryUnavailable)
	}
	configured := p.environ()["CODEX_HOME"]
	if configured == "" {
		return nil
	}
	if !samePath(configured, expected) {
		return errorOf(CategoryUnexpectedHome)
	}
	return nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// Credential reads, validates and returns the current credential.
//
// Every failure has its own category. "There is no file", "the file is not a credential",
// "the token expired" and "this is a different account than the one this session started
// with" lead a user to four different actions, and collapsing them into one message would
// send them guessing.
func (p *Provider) Credential() (Credential, error) {
	if err := p.CheckRuntime(); err != nil {
		return Credential{}, err
	}
	if err := p.CheckHome(); err != nil {
		return Credential{}, err
	}
	home, err := p.home()
	if err != nil {
		return Credential{}, errorOf(CategoryUnavailable)
	}

	// The store setting is read first. Reading auth.json when the user selected the system
	// keyring would report no credentials while theirs sit somewhere this build never
	// looked, which is a confusing failure rather than an honest one.
	config, err := p.readBounded(filepath.Join(home, "config.toml"), maxConfigBytes)
	switch {
	case err == nil:
		store, err := readStoreSetting(config)
		if err != nil {
			return Credential{}, err
		}
		if store != "" && store != "file" {
			return Credential{}, errorOf(CategoryStoreUnsupported)
		}
	case CategoryOf(err) == CategoryFileUnavailable:
		// No config at all means the default, which is the file store.
	default:
		return Credential{}, err
	}

	raw, err := p.readBounded(filepath.Join(home, "auth.json"), maxCredentialBytes)
	if err != nil {
		if CategoryOf(err) == CategoryFileUnavailable {
			return Credential{}, errorOf(CategoryUnavailable)
		}
		return Credential{}, err
	}

	credential, err := p.selectCredential(raw)
	if err != nil {
		return Credential{}, err
	}

	// One session, one account. A credential file replaced mid-session by a different
	// login is not a reason to continue under the new identity: the conversation, its
	// history and its budget all belong to the account it started with.
	if p.boundAccount == "" {
		p.boundAccount = credential.Account
	} else if p.boundAccount != credential.Account {
		return Credential{}, errorOf(CategoryAccountChanged)
	}
	return credential, nil
}

// BoundAccount reports the account this session is pinned to, if one has been read.
func (p *Provider) BoundAccount() string { return p.boundAccount }

func (p *Provider) selectCredential(raw string) (Credential, error) {
	var doc struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return Credential{}, errorOf(CategoryInvalidCache)
	}
	if doc.AuthMode != "" && doc.AuthMode != "chatgpt" {
		return Credential{}, errorOf(CategoryInvalidCache)
	}

	token := doc.Tokens.AccessToken
	account := doc.Tokens.AccountID
	if len(token) == 0 || len(token) > maxTokenBytes || !jwtShape.MatchString(token) {
		return Credential{}, errorOf(CategoryInvalidCache)
	}
	if !accountShape.MatchString(account) {
		return Credential{}, errorOf(CategoryInvalidCache)
	}

	expiry, err := tokenExpiry(token)
	if err != nil {
		return Credential{}, err
	}
	// A local sanity check, not signature verification. The server authenticates the
	// token; this only avoids sending one that is already known to be useless.
	if !expiry.After(p.now().Add(expiryMargin)) {
		return Credential{}, errorOf(CategoryTokenExpired)
	}

	return Credential{
		accessToken: token,
		Account:     account,
		ExpiresAt:   expiry,
		Synthetic:   p.Synthetic,
	}, nil
}

func tokenExpiry(token string) (time.Time, error) {
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return time.Time{}, errorOf(CategoryInvalidCache)
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return time.Time{}, errorOf(CategoryInvalidCache)
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == nil {
		return time.Time{}, errorOf(CategoryInvalidCache)
	}
	seconds := *claims.Exp
	// Not a finite second count is not an expiry.
	if seconds != seconds || seconds > 1e18 || seconds < -1e18 {
		return time.Time{}, errorOf(CategoryInvalidCache)
	}
	return time.Unix(int64(seconds), 0), nil
}

// readBounded reads a file with a ceiling, refusing anything larger rather than reading it
// to find out. It never opens a file for writing and never creates one.
func (p *Provider) readBounded(path string, limit int) (string, error) {
	if p.ReadFile != nil {
		raw, err := p.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "", errorOf(CategoryFileUnavailable)
			}
			return "", errorOf(CategoryFileUnavailable)
		}
		if len(raw) > limit {
			return "", errorOf(CategoryFileTooLarge)
		}
		return string(raw), nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", errorOf(CategoryFileUnavailable)
	}
	defer file.Close()

	// One byte past the ceiling, so "exactly at the limit" and "too large" are
	// distinguishable without reading an unbounded file to find out which.
	buffer := make([]byte, limit+1)
	total := 0
	for total < len(buffer) {
		n, err := file.Read(buffer[total:])
		total += n
		if err != nil {
			break
		}
		if n == 0 {
			break
		}
	}
	if total > limit {
		zero(buffer)
		return "", errorOf(CategoryFileTooLarge)
	}
	text := string(buffer[:total])
	// The buffer held a credential. Clearing it does not make Go's garbage collector
	// forget the copy in text, and this package does not claim otherwise — it removes the
	// one copy it can account for.
	zero(buffer)
	return text, nil
}

func zero(buffer []byte) {
	for i := range buffer {
		buffer[i] = 0
	}
}
