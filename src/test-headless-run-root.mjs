import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
const project = dirname(dirname(fileURLToPath(import.meta.url)));
const root = mkdtempSync(join(project, '.tmp', 'headless-root-public-'));
mkdirSync(join(root, 'verification')); mkdirSync(join(root, '.tmp'));
for (const name of ['verify-native-headless.ps1', 'read-transport-progress.ps1']) {
  copyFileSync(join(project, 'verification', name), join(root, 'verification', name));
}
// The shadow project contains no executable native entry or authentication
// implementation. An incorrect early guard cannot start the real application.
assert.equal(existsSync(join(root, 'verification', 'guarded-headless-entry.mjs')), false);
assert.equal(existsSync(join(root, 'src', 'clauduct.mjs')), false);
const id = 'a'.repeat(32), occupied = join(root, '.tmp', 'native-headless-' + id);
mkdirSync(occupied); writeFileSync(join(occupied, 'public-marker.txt'), 'PRESERVE_PUBLIC', { flag: 'wx' });
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
env.TEMP = join(root, '.tmp'); env.TMP = env.TEMP;
let checks = 2;
for (const [value, message] of [[id, 'VERIFICATION_RUN_EXISTS'], ['../PUBLIC', 'RunId'], ['A'.repeat(32), 'RunId'], ['b'.repeat(31), 'RunId']]) {
  const result = spawnSync('C:\\Program Files\\PowerShell\\7\\pwsh.exe', ['-NoProfile', '-NonInteractive', '-File',
    join(root, 'verification', 'verify-native-headless.ps1'), '-Live', '-Model', 'sol', '-Effort', 'low', '-RunId', value],
  { env, cwd: root, windowsHide: true, encoding: 'utf8', timeout: 7000, maxBuffer: 8192 });
  assert.equal(result.error, undefined); assert.equal(result.status, 1); assert.equal(result.signal, null);
  assert(result.stderr.includes(message)); assert.equal(result.stdout, ''); checks += 5;
}
assert.equal(readFileSync(join(occupied, 'public-marker.txt'), 'utf8'), 'PRESERVE_PUBLIC'); checks++;
assert.equal(existsSync(join(occupied, 'config')), false); checks++;
console.log(JSON.stringify({ suite: 'headless-run-root', checks, root, existingRootPreserved: true,
  actualPublicPowerShellChildren: 4, actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
