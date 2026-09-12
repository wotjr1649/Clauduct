import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { userInfo } from 'node:os';
import { spawnSync } from 'node:child_process';
import { CLAUDE_EXE, TASK_ROOT } from '../poc/claude-inspection.mjs';
import { FIXTURE_PATH } from '../poc/adapter.mjs';
import { interactiveLaunch } from './clauduct.mjs';

const self = fileURLToPath(import.meta.url), root = dirname(dirname(self));
assert.equal(resolve(TASK_ROOT), resolve(root), 'the fixture root must follow this checkout');
assert.equal(FIXTURE_PATH, join(root, 'poc', 'fixture.txt'));
assert.equal(CLAUDE_EXE, join(userInfo().homedir, '.local', 'bin', 'claude.exe'));
const { CODEX_ROOT, CODEX_EXE } = await import('./runtime-paths.mjs');
assert.equal(CODEX_ROOT, join(userInfo().homedir, '.codex'));
assert.equal(CODEX_EXE, join(userInfo().homedir, 'AppData', 'Local', 'Programs', 'OpenAI', 'Codex', 'bin', 'codex.exe'));
const launch = interactiveLaunch({ port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) }, {}, root);
const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
assert.equal(Object.hasOwn(settings, 'statusLine'), false, 'native user configuration owns the status line');
if (process.argv[2] !== '--synthetic-profile') {
  const child = spawnSync(process.execPath, [self, '--synthetic-profile'], { cwd: root,
    env: { ...process.env, USERPROFILE: 'D:/SYNTHETIC_UNTRUSTED_PROFILE' },
    encoding: 'utf8', timeout: 5000, maxBuffer: 16384, shell: false, windowsHide: true });
  assert.equal(child.error, undefined); assert.equal(child.status, 0, child.stderr);
  console.log(JSON.stringify({ suite: 'runtime-paths', checks: 7, externalRequests: 0, actualCredentialReads: 0 }));
}
