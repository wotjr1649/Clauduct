import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { initializeRecovery, runRecovery, readRecoveryJournal, recoveryOracle } from '../verification/unattended-recovery.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const manager = join(project, 'verification', 'unattended-recovery.mjs');
const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const fixture = (mode = 'queryable') => {
  const root = temporaryDir(project, 'recovery-wait-'); initializeRecovery(root, { mode }); return root;
};
const run = root => {
  const result = spawnSync(process.execPath, [manager, root], { cwd: project, env, windowsHide: true,
    encoding: 'utf8', timeout: 12000, maxBuffer: 16384 });
  assert.equal(result.error, undefined); assert.equal(result.stderr, '');
  return { code: result.status, value: JSON.parse(result.stdout) };
};
let checks = 0;
{
  const root = fixture(), retryAtMs = Date.now() + 1000;
  const wait = { retryAtMs, category: 'UPSTREAM_RETRY_DEFERRED', httpStatus: 503 };
  const first = await runRecovery(root, { execute: async () => ({ completed: false, exitCode: 1, wait }) });
  assert.equal(first.state, 'WAITING'); assert.equal(first.taskCompleted, false); assert.equal(first.retryAtMs, retryAtMs);
  const journal = readFileSync(join(root, 'journal.jsonl'), 'utf8');
  const early = run(root);
  assert.equal(early.code, 2); assert.equal(early.value.state, 'WAITING'); assert.equal(early.value.retryAtMs, retryAtMs);
  assert.equal(readFileSync(join(root, 'journal.jsonl'), 'utf8'), journal);
  assert.equal(readRecoveryJournal(root).filter(row => row.event === 'WORKER').length, 0);
  assert.equal(existsSync(join(root, 'work', 'operation.json')), false);
  await new Promise(done => setTimeout(done, Math.max(0, retryAtMs - Date.now()) + 10));
  const final = run(root);
  assert.equal(final.code, 0); assert.equal(final.value.taskCompleted, true); assert.equal(recoveryOracle(root), true);
  const workers = readRecoveryJournal(root).filter(row => row.event === 'WORKER');
  assert.deepEqual(workers.map(row => row.phase), ['effect', 'finish']);
  for (const row of workers) assert.throws(() => process.kill(row.pid, 0), error => error.code === 'ESRCH');
  checks++;
}
for (const wait of [
  null, {}, { retryAtMs: true, category: 'UPSTREAM_HTTP_ERROR', httpStatus: 503 },
  { retryAtMs: 1.5, category: 'UPSTREAM_HTTP_ERROR', httpStatus: 503 },
  { retryAtMs: -1, category: 'UPSTREAM_HTTP_ERROR', httpStatus: 503 },
  { retryAtMs: Date.now(), category: 'UPSTREAM_TLS_ERROR', httpStatus: 503 },
  { retryAtMs: Date.now(), category: 'UPSTREAM_HTTP_ERROR', httpStatus: 403 },
  { retryAtMs: Date.now(), category: 'RATE_LIMITED', httpStatus: 503 },
  { retryAtMs: Date.now(), category: 'UPSTREAM_RETRY_DEFERRED', httpStatus: '503' },
  { retryAtMs: Date.now(), category: 'UPSTREAM_HTTP_ERROR', httpStatus: 503, message: 'SYNTHETIC_PRIVATE' }
]) {
  const root = fixture();
  await assert.rejects(runRecovery(root, { execute: async () => ({ completed: false, exitCode: 1, wait }) }), error => error.code === 'WORKER_FAILED');
  const journal = readFileSync(join(root, 'journal.jsonl'), 'utf8');
  assert.ok(!journal.includes('SYNTHETIC_PRIVATE')); assert.ok(!journal.includes('WAITING'));
  assert.equal(existsSync(join(root, 'work', 'operation.json')), false); checks++;
}
for (const result of [{ completed: 'true', exitCode: 0 }, { completed: true, exitCode: 1 },
  { completed: false, exitCode: { message: 'SYNTHETIC_PRIVATE' } },
  { completed: true, exitCode: 0, wait: { retryAtMs: Date.now(), category: 'RATE_LIMITED', httpStatus: 429 } }]) {
  const root = fixture();
  await assert.rejects(runRecovery(root, { execute: async () => result }), error => error.code === 'WORKER_FAILED');
  assert.ok(!readFileSync(join(root, 'journal.jsonl'), 'utf8').includes('SYNTHETIC_PRIVATE')); checks++;
}
{
  const root = fixture(), retryAtMs = Date.now() + 60000;
  await runRecovery(root, { execute: async () => ({ completed: false, exitCode: 1,
    wait: { retryAtMs, category: 'RATE_LIMITED', httpStatus: 429 } }) });
  const rows = readRecoveryJournal(root);
  // A shorter later deadline or unrelated row cannot erase the server minimum.
  writeFileSync(join(root, 'journal.jsonl'), [
    { seq: rows.length + 1, event: 'INTERRUPTED', at: Date.now(), exitCode: 1 },
    { seq: rows.length + 2, event: 'WAITING', at: Date.now(), retryAtMs: Date.now() + 1000,
      retryCategory: 'UPSTREAM_HTTP_ERROR', retryStatus: 503 },
    { seq: rows.length + 3, event: 'FIX_NEEDED', at: Date.now() }
  ].map(row => JSON.stringify(row) + '\n').join(''), { flag: 'a' });
  const early = run(root); assert.equal(early.value.state, 'WAITING'); assert.equal(early.value.retryAtMs, retryAtMs);
  assert.equal(readRecoveryJournal(root).filter(row => row.event === 'WORKER').length, 0); checks++;
}
for (const change of ['missing-status', 'boolean-time', 'wrong-category', 'extra-field', 'missing-predecessor', 'invalid-exit-code']) {
  const root = fixture();
  const rows = [{ seq: 1, event: 'INTENT', at: Date.now() }, { seq: 2, event: 'INTERRUPTED', at: Date.now(), exitCode: 1 },
    { seq: 3, event: 'WAITING', at: Date.now(), retryAtMs: Date.now() + 60000,
      retryCategory: 'UPSTREAM_HTTP_ERROR', retryStatus: 503 }];
  if (change === 'missing-status') delete rows[2].retryStatus;
  if (change === 'boolean-time') rows[2].retryAtMs = true;
  if (change === 'wrong-category') rows[2].retryCategory = 'UNAUTHENTICATED';
  if (change === 'extra-field') rows[2].pid = 1;
  if (change === 'missing-predecessor') rows[1].event = 'FIX_NEEDED';
  if (change === 'invalid-exit-code') rows[2].exitCode = 'SYNTHETIC_PRIVATE';
  writeFileSync(join(root, 'journal.jsonl'), rows.map(row => JSON.stringify(row) + '\n').join(''));
  const result = run(root); assert.equal(result.code, 1); assert.equal(result.value.state, 'JOURNAL_CORRUPT');
  assert.equal(existsSync(join(root, 'work', 'operation.json')), false); checks++;
}
{
  const root = fixture('opaque');
  await runRecovery(root, { execute: async () => ({ completed: false, exitCode: 1,
    wait: { retryAtMs: Date.now() + 60000, category: 'UPSTREAM_HTTP_ERROR', httpStatus: 503 } }) });
  const result = run(root); assert.equal(result.value.state, 'UNKNOWN_EFFECT'); assert.equal(result.value.taskCompleted, false);
  assert.equal(readRecoveryJournal(root).filter(row => row.event === 'WORKER').length, 0); checks++;
}
{
  const root = fixture(), retryAtMs = Date.now() + 60000;
  // Exit after the manager committed the failure but before the caller got a result.
  const script = 'import { runRecovery } from "./verification/unattended-recovery.mjs";'
    + 'await runRecovery(process.argv[1], { execute: async () => ({ completed: false, exitCode: 1,'
    + 'wait: { retryAtMs: Number(process.argv[2]), category: "UPSTREAM_RETRY_DEFERRED", httpStatus: 503 } }) });'
    + 'process.exit(71);';
  const crashed = spawnSync(process.execPath, ['--input-type=module', '-e', script, root, String(retryAtMs)],
    { cwd: project, env, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 8192 });
  assert.equal(crashed.error, undefined); assert.equal(crashed.status, 71); assert.equal(crashed.stdout, ''); assert.equal(crashed.stderr, '');
  const rows = readRecoveryJournal(root);
  assert.deepEqual(rows.map(row => row.event), ['INTENT', 'WAITING']);
  assert.equal(rows[1].exitCode, 1); assert.equal(rows[1].retryAtMs, retryAtMs);
  const early = run(root); assert.equal(early.value.state, 'WAITING'); assert.equal(early.value.retryAtMs, retryAtMs);
  assert.equal(existsSync(join(root, 'work', 'operation.json')), false); checks++;
}
console.log(JSON.stringify({ suite: 'recovery-wait', checks, actualCredentialReads: 0, externalRequests: 0,
  actualClaudeExecutions: 0, actualDefaultWorkers: 2 }));
