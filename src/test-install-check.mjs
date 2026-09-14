import assert from 'node:assert/strict';
import { checkInstallation } from './install-check.mjs';
const runtime = { node: process.execPath, claude: { file: 'C:\\Public\\claude.exe', args: [], found: true },
  codex: { file: process.execPath, args: ['C:\\Public\\codex.js'], found: true } };
const calls = [];
const run = (file, args, options) => { calls.push({ file, args, options }); return { status: 0,
  stdout: file === runtime.claude.file ? '2.1.270 (Claude Code)\n' : 'codex-cli 0.154.0\n' }; };
assert.equal(checkInstallation({ runtime, run }).passed, true);
assert.deepEqual(calls.map(call => call.args), [['--version'], ['C:\\Public\\codex.js', '--version']]);
assert.ok(calls.every(call => call.options.shell === false && call.options.timeout === 5000));
assert.equal(Object.hasOwn(calls[0].options.env, 'NODE_OPTIONS'), false);
for (const name of ['claude', 'codex']) {
  assert.throws(() => checkInstallation({ runtime: { ...runtime, [name]: { ...runtime[name], found: false } }, run }), /NOT_INSTALLED/);
}
for (const output of [{ status: 0, stdout: 'unrelated tool' }, { status: 1, stdout: '2.1.270 (Claude Code)' },
  { status: 0, stdout: '2.1.270 (Claude Code)', signal: 'SIGTERM' }, { status: 0, stdout: '2.1.270\nPRIVATE_EXTRA' }]) {
  assert.throws(() => checkInstallation({ runtime, run: () => output }), /VERSION_UNAVAILABLE/);
}
console.log(JSON.stringify({ suite: 'install-check', passed: true, checks: 10, actualCliExecutions: 0, credentialReads: 0, externalRequests: 0 }));
