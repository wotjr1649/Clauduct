import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

test('Windows crypto starts with canonical system paths', () => {
  assert.ok(existsSync(process.env.SystemRoot));
  assert.equal(process.env.WINDIR, process.env.SystemRoot);
  assert.equal(randomBytes(16).length, 16);
});
test('temporary paths are task-local', () => {
  assert.equal(resolve(process.env.TEMP), resolve('.tmp'));
  assert.equal(process.env.TMP, process.env.TEMP);
});
test('unrelated environment is absent', () => {
  for (const key of ['CLAUDUCT_ENV_SENTINEL', 'HTTP_PROXY', 'NODE_OPTIONS', 'HOME', 'USERPROFILE',
    'ANTHROPIC_AUTH_TOKEN', 'OPENAI_API_KEY', 'PATH']) {
    assert.equal(process.env[key], undefined, key);
  }
});
