package auth

import "testing"

func read(t *testing.T, config string) (string, error) {
	t.Helper()
	return readStoreSetting(config)
}

func mustRead(t *testing.T, config, want string) {
	t.Helper()
	got, err := read(t, config)
	if err != nil {
		t.Fatalf("readStoreSetting(%q): %v", config, err)
	}
	if got != want {
		t.Fatalf("readStoreSetting(%q) = %q, want %q", config, got, want)
	}
}

func mustReject(t *testing.T, config string) {
	t.Helper()
	if got, err := read(t, config); err == nil {
		t.Fatalf("readStoreSetting(%q) = %q, want a refusal", config, got)
	}
}

func TestTheSettingIsFoundInOrdinaryConfigs(t *testing.T) {
	for name, tc := range map[string]struct{ config, want string }{
		"absent":            {"model = \"gpt-5\"\n", ""},
		"empty file":        {"", ""},
		"plain":             {`cli_auth_credentials_store = "file"`, "file"},
		"with whitespace":   {"  cli_auth_credentials_store   =   \"keyring\"  \n", "keyring"},
		"single quoted":     {`cli_auth_credentials_store = 'auto'`, "auto"},
		"trailing comment":  {`cli_auth_credentials_store = "file" # why`, "file"},
		"after other keys":  {"model = \"gpt-5\"\ncli_auth_credentials_store = \"ephemeral\"\n", "ephemeral"},
		"after a comment":   {"# a note\ncli_auth_credentials_store = \"file\"\n", "file"},
		"crlf line endings": {"model = \"x\"\r\ncli_auth_credentials_store = \"file\"\r\n", "file"},
		"quoted key":        {`"cli_auth_credentials_store" = "file"`, "file"},
		"before a table":    {"cli_auth_credentials_store = \"file\"\n[tui]\nfoo = 1\n", "file"},
	} {
		t.Run(name, func(t *testing.T) { mustRead(t, tc.config, tc.want) })
	}
}

// The setting is a root key. One with the same name inside a table is a different setting,
// and reading it as the root one would apply a value the user wrote about something else.
//
// Added after a mutation run: dropping the root-table condition left the suite green.
func TestASettingUnderATableIsNotTheRootOne(t *testing.T) {
	for name, config := range map[string]string{
		"under a table":        "[profiles]\ncli_auth_credentials_store = \"keyring\"\n",
		"under a nested table": "[profiles.work]\ncli_auth_credentials_store = \"keyring\"\n",
		"under an array table": "[[profiles]]\ncli_auth_credentials_store = \"keyring\"\n",
		"after any table":      "model = \"x\"\n[tui]\nfoo = 1\ncli_auth_credentials_store = \"keyring\"\n",
	} {
		t.Run(name, func(t *testing.T) { mustRead(t, config, "") })
	}
}

// Two readings of the same setting is not something to pick a winner from.
//
// Added after a mutation run: accepting the second occurrence left the suite green.
func TestARepeatedSettingIsRefused(t *testing.T) {
	mustReject(t, "cli_auth_credentials_store = \"file\"\ncli_auth_credentials_store = \"keyring\"\n")
	mustReject(t, "cli_auth_credentials_store = \"keyring\"\ncli_auth_credentials_store = \"file\"\n")
}

// A value outside the known set may name a store that holds the real credentials. Reading
// auth.json anyway would report no credentials while theirs sit somewhere else.
//
// Added after a mutation run: returning the unknown value left the suite green.
func TestAnUnknownStoreValueIsRefused(t *testing.T) {
	for _, value := range []string{"vault", "1password", "File", "FILE", "", "file ", " file"} {
		mustReject(t, `cli_auth_credentials_store = "`+value+`"`)
	}
}

// A dotted path, a table header by that name, or a non-string value are all shapes this
// reader does not claim to understand, and guessing at one decides where a credential
// comes from.
func TestUnreadableShapesAreRefused(t *testing.T) {
	for name, config := range map[string]string{
		"table header by that name": "[cli_auth_credentials_store]\nkind = \"file\"\n",
		"dotted path":               `cli_auth_credentials_store.kind = "file"`,
		"not a string":              `cli_auth_credentials_store = 1`,
		"an array":                  `cli_auth_credentials_store = ["file"]`,
		"a boolean":                 `cli_auth_credentials_store = true`,
		"no value":                  `cli_auth_credentials_store =`,
		"no equals":                 `cli_auth_credentials_store "file"`,
		"unterminated string":       `cli_auth_credentials_store = "file`,
		"trailing junk on the line": `cli_auth_credentials_store = "file" junk`,
	} {
		t.Run(name, func(t *testing.T) { mustReject(t, config) })
	}
}

// Skipping a value has to understand where it ends. A string or a nested structure
// containing a newline, a bracket or the key name again must not end the line early or
// make the reader find the wrong setting.
func TestValuesAreSkippedStructurally(t *testing.T) {
	for name, tc := range map[string]struct{ config, want string }{
		"a string containing the key name": {
			"note = \"cli_auth_credentials_store = \\\"keyring\\\"\"\ncli_auth_credentials_store = \"file\"\n", "file"},
		"a multi-line string": {
			"note = \"\"\"\nline one\ncli_auth_credentials_store = \"keyring\"\nline three\n\"\"\"\n" +
				"cli_auth_credentials_store = \"file\"\n", "file"},
		"a literal multi-line string": {
			"note = '''\ncli_auth_credentials_store = 'keyring'\n'''\ncli_auth_credentials_store = \"file\"\n", "file"},
		"an array spanning lines": {
			"items = [\n  1,\n  2,\n]\ncli_auth_credentials_store = \"file\"\n", "file"},
		"a nested inline table": {
			"t = { a = { b = [1, 2] } }\ncli_auth_credentials_store = \"file\"\n", "file"},
		"an escape sequence": {
			"note = \"a\\\\b\\\"c\"\ncli_auth_credentials_store = \"file\"\n", "file"},
		"a unicode escape": {
			"note = \"\\u0041\\U0001F600\"\ncli_auth_credentials_store = \"file\"\n", "file"},
		"a comment containing the key": {
			"# cli_auth_credentials_store = \"keyring\"\ncli_auth_credentials_store = \"file\"\n", "file"},
	} {
		t.Run(name, func(t *testing.T) { mustRead(t, tc.config, tc.want) })
	}
}

func TestMalformedValuesAreRefusedRatherThanSkippedPast(t *testing.T) {
	for name, config := range map[string]string{
		"unbalanced bracket":  "items = [1, 2\ncli_auth_credentials_store = \"file\"\n",
		"unbalanced brace":    "t = { a = 1\ncli_auth_credentials_store = \"file\"\n",
		"mismatched closers":  "items = [1, 2}\ncli_auth_credentials_store = \"file\"\n",
		"bad unicode escape":  "note = \"\\uZZZZ\"\ncli_auth_credentials_store = \"file\"\n",
		"surrogate half":      "note = \"\\uD800\"\ncli_auth_credentials_store = \"file\"\n",
		"unknown escape":      "note = \"\\q\"\ncli_auth_credentials_store = \"file\"\n",
		"raw control char":    "note = \"a\x01b\"\ncli_auth_credentials_store = \"file\"\n",
		"newline in a string": "note = \"a\nb\"\ncli_auth_credentials_store = \"file\"\n",
	} {
		t.Run(name, func(t *testing.T) { mustReject(t, config) })
	}
}

// A config far larger than a config file is refused rather than walked.
func TestOversizedConfigIsRefused(t *testing.T) {
	big := make([]byte, maxConfigBytes+1)
	for i := range big {
		big[i] = '#'
	}
	mustReject(t, string(big))
}

// A file that ends in a blank with no newline is still a file this can read.
//
// Found by review. space() walks past the last byte, at() answers zero, nothing matches,
// and the default branch reads an assignment out of the end of the file -- so the store was
// CONFIG_UNSUPPORTED and every request in the session became a 400 that looked, to the
// user, like their credential store was broken. The existing whitespace tests all ended in
// a newline and never walked that path.
func TestAConfigEndingInBlankSpaceIsStillRead(t *testing.T) {
	for name, text := range map[string]string{
		"a trailing space":       "[x]\nkey = \"value\" ",
		"a trailing tab":         "[x]\nkey = \"value\"\t",
		"blanks after a newline": "[x]\nkey = \"value\"\n   ",
		"nothing but blanks":     "   ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readStoreSetting(text); err != nil {
				t.Fatalf("read: %v", err)
			}
		})
	}
}
