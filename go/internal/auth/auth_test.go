package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A synthetic token. It is a real JWT shape with a real payload so the expiry path is
// exercised, and a signature segment that is deliberately nonsense — nothing here is
// signed and nothing here verifies a signature.
func syntheticToken(expiry time.Time) string {
	payload, _ := json.Marshal(map[string]any{"exp": expiry.Unix()})
	return "eyJhbGciOiJub25lIn0." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".SYNTHETIC-SIGNATURE-NOT-VERIFIED"
}

func authFile(token, account string) string {
	body, _ := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens":    map[string]any{"access_token": token, "account_id": account},
	})
	return string(body)
}

// store writes a synthetic Codex home and returns a provider reading it.
func store(t *testing.T, files map[string]string) *Provider {
	t.Helper()
	home := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return &Provider{Home: home, Environ: map[string]string{}, Synthetic: true}
}

func valid(t *testing.T) *Provider {
	t.Helper()
	return store(t, map[string]string{
		"auth.json": authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_synthetic_1"),
	})
}

func mustRefuse(t *testing.T, p *Provider, want string) {
	t.Helper()
	_, err := p.Credential()
	if err == nil {
		t.Fatalf("accepted a credential that should have been refused with %s", want)
	}
	if got := CategoryOf(err); got != want {
		t.Fatalf("category = %s, want %s", got, want)
	}
}

func TestValidCredentialIsRead(t *testing.T) {
	credential, err := valid(t).Credential()
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if credential.Account != "acct_synthetic_1" {
		t.Fatalf("Account = %q", credential.Account)
	}
	if credential.Token() == "" {
		t.Fatal("no token")
	}
	if !credential.Synthetic {
		t.Fatal("a credential from a synthetic store must be marked as one")
	}
}

// AUTH01: absent, malformed and expired are three different situations leading a user to
// three different actions.
func TestAbsentMalformedAndExpiredAreDistinct(t *testing.T) {
	t.Run("no file at all", func(t *testing.T) {
		mustRefuse(t, store(t, nil), CategoryUnavailable)
	})
	t.Run("expired", func(t *testing.T) {
		mustRefuse(t, store(t, map[string]string{
			"auth.json": authFile(syntheticToken(time.Now().Add(-time.Hour)), "acct_1"),
		}), CategoryTokenExpired)
	})
	t.Run("expiring inside the margin", func(t *testing.T) {
		// A token that is valid now and expired before the request finishes is a failure
		// the user sees as a mystery, so the margin treats it as already expired.
		mustRefuse(t, store(t, map[string]string{
			"auth.json": authFile(syntheticToken(time.Now().Add(30*time.Second)), "acct_1"),
		}), CategoryTokenExpired)
	})

	// Every case below carries a token that would otherwise pass, so the field named in
	// the case is the only thing refusing it. The first version of this table used
	// throwaway tokens and a mutation run showed the token path catching all of them —
	// each case was green for a reason other than the one it claimed.
	good := syntheticToken(time.Now().Add(time.Hour))
	for name, content := range map[string]string{
		"not json":  `nope`,
		"empty":     ``,
		"no tokens": `{"auth_mode":"chatgpt"}`,

		"no access token": `{"tokens":{"account_id":"acct_1"}}`,
		"no account":      `{"tokens":{"access_token":"` + good + `"}}`,

		// The account shape is the only defect here.
		"account with a dot":   `{"tokens":{"access_token":"` + good + `","account_id":"acct.1"}}`,
		"account with a space": `{"tokens":{"access_token":"` + good + `","account_id":"acct 1"}}`,
		"empty account":        `{"tokens":{"access_token":"` + good + `","account_id":""}}`,

		// The auth mode is the only defect here.
		"wrong auth mode": `{"auth_mode":"apikey","tokens":{"access_token":"` + good + `","account_id":"acct_1"}}`,

		// The token's own shape is the only defect: the payload still decodes and still
		// carries a usable expiry, so nothing downstream would have objected.
		"space in the header segment": `{"tokens":{"access_token":"a b.` +
			strings.SplitN(good, ".", 3)[1] + `.sig","account_id":"acct_1"}}`,
		"plus sign in the signature": `{"tokens":{"access_token":"hdr.` +
			strings.SplitN(good, ".", 3)[1] + `.si+g","account_id":"acct_1"}}`,
		"two segments only": `{"tokens":{"access_token":"hdr.` +
			strings.SplitN(good, ".", 3)[1] + `","account_id":"acct_1"}}`,

		"token not a jwt":    `{"tokens":{"access_token":"not-a-jwt","account_id":"acct_1"}}`,
		"payload not base64": `{"tokens":{"access_token":"a.!!!.c","account_id":"acct_1"}}`,
		"no expiry claim":    `{"tokens":{"access_token":"a.e30.c","account_id":"acct_1"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, store(t, map[string]string{"auth.json": content}), CategoryInvalidCache)
		})
	}

	t.Run("token past the byte ceiling", func(t *testing.T) {
		// Well formed in every other way, and far too long to be a credential.
		long := "h." + strings.SplitN(good, ".", 3)[1] + "." + strings.Repeat("s", maxTokenBytes)
		mustRefuse(t, store(t, map[string]string{
			"auth.json": `{"tokens":{"access_token":"` + long + `","account_id":"acct_1"}}`,
		}), CategoryInvalidCache)
	})
}

// AUTH02: one session, one account. A credential file replaced mid-session by a different
// login is not a reason to continue under the new identity — the conversation, its history
// and its budget belong to the account it started with.
func TestAccountChangeMidSessionIsRefused(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "auth.json")
	token := syntheticToken(time.Now().Add(time.Hour))

	write := func(account string) {
		if err := os.WriteFile(path, []byte(authFile(token, account)), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	provider := &Provider{Home: home, Environ: map[string]string{}, Synthetic: true}

	write("acct_first")
	if _, err := provider.Credential(); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if provider.BoundAccount() != "acct_first" {
		t.Fatalf("bound to %q", provider.BoundAccount())
	}

	// A re-read after a 401 must not follow the file to a different account.
	write("acct_second")
	mustRefuse(t, provider, CategoryAccountChanged)

	// The same account re-read is fine: a refreshed token for the same login is ordinary.
	write("acct_first")
	if _, err := provider.Credential(); err != nil {
		t.Fatalf("re-read of the same account: %v", err)
	}
}

// AUTH04: nothing here writes. A provider that repaired, refreshed or rewrote the file
// would be changing state the user's own tool owns.
func TestNothingIsWritten(t *testing.T) {
	home := t.TempDir()
	authPath := filepath.Join(home, "auth.json")
	configPath := filepath.Join(home, "config.toml")
	content := authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1")
	os.WriteFile(authPath, []byte(content), 0o600)
	os.WriteFile(configPath, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600)

	before := map[string]os.FileInfo{}
	for _, path := range []string{authPath, configPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		before[path] = info
	}

	provider := &Provider{Home: home, Environ: map[string]string{}, Synthetic: true}
	for i := 0; i < 3; i++ {
		if _, err := provider.Credential(); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}

	for path, info := range before {
		after, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat after: %v", err)
		}
		if !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
			t.Errorf("%s changed: %v/%d -> %v/%d",
				filepath.Base(path), info.ModTime(), info.Size(), after.ModTime(), after.Size())
		}
	}
	// Nothing new appeared either — no lock file, no backup, no temp file.
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory contents changed: %v", names)
	}
}

// AUTH05: a credential must not reach a log, an error or a debugger dump. Go prints structs
// readily, which is exactly how this leaks if nothing stops it.
func TestCredentialIsRedactedEverywhereItCouldBePrinted(t *testing.T) {
	credential, err := valid(t).Credential()
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	token := credential.Token()
	if token == "" {
		t.Fatal("no token to check")
	}

	for name, rendered := range map[string]string{
		"String":       credential.String(),
		"%v":           fmt.Sprintf("%v", credential),
		"%s":           fmt.Sprintf("%s", credential),
		"%+v":          fmt.Sprintf("%+v", credential),
		"%#v":          fmt.Sprintf("%#v", credential),
		"inside slice": fmt.Sprintf("%v", []Credential{credential}),
		"inside map":   fmt.Sprintf("%v", map[string]Credential{"c": credential}),
		"pointer":      fmt.Sprintf("%v", &credential),
	} {
		if strings.Contains(rendered, token) {
			t.Errorf("%s leaked the token: %s", name, rendered)
		}
		if !strings.Contains(rendered, "redacted") {
			t.Errorf("%s does not say it is redacted: %s", name, rendered)
		}
	}
}

// Every refusal is a fixed category, so no failure path can assemble a message out of file
// content and carry a credential out with it.
func TestRefusalsCarryNoFileContent(t *testing.T) {
	const marker = "SECRET-LOOKING-CONTENT"
	_, err := store(t, map[string]string{
		"auth.json": `{"tokens":{"access_token":"` + marker + `","account_id":"acct_1"}}`,
	}).Credential()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("the refusal echoed file content: %s", err)
	}
}

// AUTH06: a synthetic credential and a real one must not be mixed up in either direction.
func TestSyntheticAndRealCredentialsAreDistinguishable(t *testing.T) {
	synthetic, err := valid(t).Credential()
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if !synthetic.Synthetic {
		t.Fatal("a test store produced a credential that claims to be real")
	}

	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "auth.json"),
		[]byte(authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1")), 0o600)
	real, err := (&Provider{Home: home, Environ: map[string]string{}}).Credential()
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if real.Synthetic {
		t.Fatal("a provider that was not marked synthetic produced a synthetic credential")
	}
}

// AUTH07: an unsupported store is named. Reading auth.json when the user's credentials
// live in the system keyring would report no credentials while theirs sit somewhere this
// build never looked.
func TestUnsupportedStoreIsNamed(t *testing.T) {
	auth := authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1")
	for name, config := range map[string]string{
		"keyring":   `cli_auth_credentials_store = "keyring"`,
		"auto":      `cli_auth_credentials_store = "auto"`,
		"ephemeral": `cli_auth_credentials_store = "ephemeral"`,
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, store(t, map[string]string{"auth.json": auth, "config.toml": config}),
				CategoryStoreUnsupported)
		})
	}

	t.Run("file is supported", func(t *testing.T) {
		p := store(t, map[string]string{"auth.json": auth, "config.toml": `cli_auth_credentials_store = "file"`})
		if _, err := p.Credential(); err != nil {
			t.Fatalf("the file store was refused: %v", err)
		}
	})
	t.Run("absent means the default", func(t *testing.T) {
		p := store(t, map[string]string{"auth.json": auth, "config.toml": "model = \"gpt-5\"\n"})
		if _, err := p.Credential(); err != nil {
			t.Fatalf("a config without the setting was refused: %v", err)
		}
	})
}

// A redirected Codex home is refused rather than followed. Following it would let anyone
// who can set one environment variable choose which credential this process sends.
func TestRedirectedCodexHomeIsRefused(t *testing.T) {
	p := valid(t)
	p.Environ = map[string]string{"CODEX_HOME": `D:\somewhere\else`}
	mustRefuse(t, p, CategoryUnexpectedHome)
}

// A runtime asking the TLS stack to write session keys makes the traffic decryptable by
// anything that can read the file. A credential is about to cross that traffic.
func TestTLSKeyLoggingIsRefused(t *testing.T) {
	p := valid(t)
	p.Environ = map[string]string{"SSLKEYLOGFILE": `C:\tmp\keys.log`}
	mustRefuse(t, p, CategoryRuntimeUnsupported)

	// An empty value is not a request to log anything.
	p2 := valid(t)
	p2.Environ = map[string]string{"SSLKEYLOGFILE": ""}
	if _, err := p2.Credential(); err != nil {
		t.Fatalf("an empty SSLKEYLOGFILE was treated as set: %v", err)
	}
}

// AUTH08: a file larger than a credential file is refused rather than read to find out.
func TestOversizedFilesAreRefused(t *testing.T) {
	t.Run("credential", func(t *testing.T) {
		mustRefuse(t, store(t, map[string]string{
			"auth.json": strings.Repeat("x", maxCredentialBytes+1),
		}), CategoryFileTooLarge)
	})
	t.Run("config", func(t *testing.T) {
		mustRefuse(t, store(t, map[string]string{
			"auth.json":   authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1"),
			"config.toml": strings.Repeat("#", maxConfigBytes+1),
		}), CategoryFileTooLarge)
	})
	t.Run("exactly at the limit is not too large", func(t *testing.T) {
		auth := authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1")
		padded := auth + strings.Repeat(" ", maxCredentialBytes-len(auth))
		p := store(t, map[string]string{"auth.json": padded})
		if _, err := p.Credential(); err != nil {
			t.Fatalf("a file exactly at the ceiling was refused: %v", err)
		}
	})
}

// AUTH08: a partial file — one being replaced as it is read — must not decode into
// something that looks valid.
func TestPartialFileIsRefusedNotInterpreted(t *testing.T) {
	full := authFile(syntheticToken(time.Now().Add(time.Hour)), "acct_1")
	for _, cut := range []int{1, len(full) / 3, len(full) / 2, len(full) - 1} {
		p := store(t, map[string]string{"auth.json": full[:cut]})
		if _, err := p.Credential(); err == nil {
			t.Fatalf("a file truncated at %d bytes was accepted", cut)
		}
	}
}

// Reading is lazy: a process that never sends a request never opens the credential file,
// so --help and --version work without a login.
func TestNothingIsReadUntilAskedFor(t *testing.T) {
	opened := []string{}
	p := &Provider{
		Home:    `C:\synthetic`,
		Environ: map[string]string{},
		ReadFile: func(path string) ([]byte, error) {
			opened = append(opened, path)
			return nil, os.ErrNotExist
		},
	}
	if len(opened) != 0 {
		t.Fatalf("constructing a provider opened %v", opened)
	}
	p.Credential()
	if len(opened) == 0 {
		t.Fatal("asking for a credential opened nothing")
	}
}

// Two requests reading a credential at once bind one account, not a race.
//
// Found by review. The provider is shared with the transport and the transport is used by
// every request goroutine the server starts, so the first two requests of a session read
// concurrently. The bind was an unguarded read-modify-write: a data race, and a rule that
// fails open when it loses one -- "this session is pinned to one account" is not a rule if
// two goroutines can each decide what it is.
//
// Run under -race this is the test that says so.
func TestConcurrentCredentialReadsBindOneAccount(t *testing.T) {
	provider := valid(t)

	const readers = 8
	var wg sync.WaitGroup
	accounts := make([]string, readers)
	errs := make([]error, readers)
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func(i int) {
			defer wg.Done()
			credential, err := provider.Credential()
			accounts[i], errs[i] = credential.Account, err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("reader %d: %v", i, err)
		}
		if accounts[i] != accounts[0] {
			t.Fatalf("reader %d bound %q, reader 0 bound %q", i, accounts[i], accounts[0])
		}
	}
	if provider.BoundAccount() != accounts[0] {
		t.Fatalf("bound = %q, readers saw %q", provider.BoundAccount(), accounts[0])
	}
}
