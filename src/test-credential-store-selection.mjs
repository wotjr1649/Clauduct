import assert from 'node:assert/strict';
import { createNativeCredentialSupplier } from '../poc/user-session.mjs';

const cases = [
  ['default-file', 'model = "public-model"\n', true],
  ['bare-file', 'cli_auth_credentials_store = "file"\n', true],
  ['basic-key-file', '"cli_auth_credentials_store" = "file"\n', true],
  ['literal-key-file', "'cli_auth_credentials_store' = 'file'\n", true],
  ['basic-key-keyring', '"cli_auth_credentials_store" = "keyring"\n', false],
  ['literal-key-auto', "'cli_auth_credentials_store' = 'auto'\n", false],
  ['basic-key-ephemeral', '"cli_auth_credentials_store" = "ephemeral"\n', false],
  ['escaped-key-keyring', String.raw`"cli_auth_credentials_stor\u0065" = "keyring"`, false],
  ['duplicate-key', 'cli_auth_credentials_store = "file"\n"cli_auth_credentials_store" = "file"\n', false],
  ['table-in-multiline', 'instructions = """public\n[not-a-table]\n"""\ncli_auth_credentials_store = "keyring"\n', false],
  ['comment-not-setting', '# "cli_auth_credentials_store" = "keyring"\nmodel = "public"\n', true],
  ['nested-not-root-setting', '[public]\ncli_auth_credentials_store = "keyring"\n', true],
  ['escaped-key-file', String.raw`"cli_auth_credentials_stor\U00000065" = "file"`, true],
  ['escaped-value-file', String.raw`cli_auth_credentials_store = "fi\u006Ce"`, true],
  ['escaped-value-keyring', String.raw`cli_auth_credentials_store = "key\U00000072ing"`, false],
  ['multiline-value-file', 'cli_auth_credentials_store = """\nfile"""\n', true],
  ['literal-multiline-file', "cli_auth_credentials_store = '''\nfile'''\n", true],
  ['continued-value-file', 'cli_auth_credentials_store = """fi\\\n \t le"""\n', true],
  ['quoted-key-table', '["cli_auth_credentials_store"]\nvalue="file"\n', false],
  ['auth-table-after-other', '[public]\nvalue=1\n[cli_auth_credentials_store]\nvalue="file"\n', false],
  ['array-auth-table', '[[cli_auth_credentials_store]]\nvalue="file"\n', false],
  ['nested-table-key', '[public.cli_auth_credentials_store]\nvalue="keyring"\n', true],
  ['dotted-auth-key', 'cli_auth_credentials_store.value = "file"\n', false],
  ['dotted-unrelated-key', 'public.cli_auth_credentials_store = "keyring"\n', true],
  ['auth-boolean', 'cli_auth_credentials_store = false\n', false],
  ['auth-array', 'cli_auth_credentials_store = ["file"]\n', false],
  ['auth-inline-table', 'cli_auth_credentials_store = {value="file"}\n', false],
  ['text-not-setting', 'instructions = """\n[public]\ncli_auth_credentials_store="keyring"\n"""\ncli_auth_credentials_store="file"\n', true],
  ['literal-text-not-setting', "instructions = '''\n[public]\ncli_auth_credentials_store='keyring'\n'''\ncli_auth_credentials_store='file'\n", true],
  ['nested-array-before-store', 'public = [\n["#text"],\n# comment\n["item"]\n]\ncli_auth_credentials_store="keyring"\n', false],
  ['quoted-equals-in-key', '"public=field" = "value"\ncli_auth_credentials_store="file"\n', true],
  ['quoted-hash-in-key', '"#cli_auth_credentials_store" = "keyring"\n', true],
  ['invalid-unicode-key', String.raw`"cli_auth_credentials_stor\uD800" = "file"`, false],
  ['invalid-key-escape', String.raw`"cli_auth_credentials_stor\x65" = "file"`, false],
  ['unterminated-string', 'public = """\n[table]\ncli_auth_credentials_store="file"\n', false],
  ['unterminated-array', 'public = [\n["item"]\ncli_auth_credentials_store="file"\n', false],
  ['excessive-nesting', 'public = ' + '['.repeat(65) + '0' + ']'.repeat(65) + '\n', false],
  ['unknown-store', 'cli_auth_credentials_store = "PUBLIC_UNSUPPORTED_STORE"\n', false],
  ['empty-store', 'cli_auth_credentials_store = ""\n', false],
  ['oversized-config', '#'.repeat(65537), false]
];
const failures = [];
for (const [name, config, accept] of cases) {
  let credentialReads = 0, value, error;
  const supplier = createNativeCredentialSupplier({ readConfig: () => config,
    readCredential: () => { credentialReads++; return '{}'; },
    runtimeCheck: () => {}, homeCheck: () => {},
    credentialSelector: () => ({ accessToken: 'PUBLIC_SYNTHETIC_TOKEN', account: 'PUBLIC_SYNTHETIC_ACCOUNT' }) });
  try { value = await supplier(); } catch (caught) { error = caught; }
  try {
    if (accept) { assert.equal(error, undefined); assert.equal(credentialReads, 1); assert.ok(value); }
    else {
      assert.ok(error && ['CONFIG_UNSUPPORTED', 'CREDENTIAL_STORE_UNSUPPORTED'].includes(error.code));
      assert.equal(credentialReads, 0);
    }
  } catch { failures.push(name); }
}
console.log(JSON.stringify({ suite: 'credential-store-selection', checks: cases.length, failures,
  actualCredentialReads: 0, externalRequests: 0 }));
assert.deepEqual(failures, []);
