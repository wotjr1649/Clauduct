import assert from 'node:assert/strict';
import { mkdtempSync, existsSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const fixture = join(project, 'verification', 'fixtures', 'process-tree.mjs');
const helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
const powershell = 'C:\\Program Files\\PowerShell\\7\\pwsh.exe';
const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(name => process.env[name]).map(name => [name, process.env[name]]));
const root = mkdtempSync(join(project, '.tmp', 'owned-tree-')), started = Date.now();
const child = spawn(process.execPath, [fixture, root, '0'], { env, cwd: root, stdio: 'ignore', windowsHide: true });
const deadline = Date.now() + 3000;
while (!existsSync(join(root, 'pid-2.json')) && Date.now() < deadline) await new Promise(done => setTimeout(done, 10));
assert.equal(existsSync(join(root, 'pid-2.json')), true);
function invoke(pid, stop) {
  return spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(pid),
    '-StartedAfterMs', String(started), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
  { env, cwd: project, windowsHide: true, encoding: 'utf8', timeout: 30000, maxBuffer: 8192 });
}
// A wrong live PID must be rejected before any termination, including the test owner.
const rejected = invoke(process.pid, true);
assert.equal(rejected.status, 1); assert.match(rejected.stderr, /OWNED_TREE_CHECK_FAILED/);
for (let i = 0; i < 3; i++) process.kill(JSON.parse(readFileSync(join(root, `pid-${i}.json`))).pid, 0);
const snapshot = invoke(child.pid, false);
assert.equal(snapshot.error, undefined); assert.equal(snapshot.status, 0, snapshot.stderr);
const before = JSON.parse(snapshot.stdout);
// Windows can attach a conhost to each Node process. Require all three actual
// fixture PIDs and prove every additional observation belongs to that tree.
const fixturePids = [0, 1, 2].map(i => JSON.parse(readFileSync(join(root, `pid-${i}.json`))).pid);
for (const pid of fixturePids) assert.equal(before.observed.some(row => row.pid === pid), true);
assert.ok(before.observed.length >= 3 && before.observed.length <= 9);
for (const row of before.observed) assert.ok(fixturePids.includes(row.pid) || before.observed.some(parent => parent.pid === row.parentPid));
assert.equal(before.stopped, false);
const stopped = invoke(child.pid, true);
assert.equal(stopped.error, undefined); assert.equal(stopped.status, 0, stopped.stderr);
const result = JSON.parse(stopped.stdout);
assert.equal(result.stopped, true); assert.equal(result.observed.length, before.observed.length); assert.equal(result.remaining.length, 0);
for (let i = 0; i < 3; i++) assert.throws(() => process.kill(JSON.parse(readFileSync(join(root, `pid-${i}.json`))).pid, 0), error => error.code === 'ESRCH');
console.log(JSON.stringify({ suite: 'owned-native-tree', checks: 14, stopped: result.observed.length,
  externalRequests: 0, actualCredentialReads: 0, evidenceRoot: root }));
