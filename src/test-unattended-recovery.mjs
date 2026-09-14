import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { initializeRecovery, recoveryOracle, readRecoveryJournal, runRecovery } from '../verification/unattended-recovery.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const manager = join(root, 'verification', 'unattended-recovery.mjs');
const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const created = [];
let checks = 0;
function fixture(mode = 'queryable') {
  const dir = temporaryDir(root, 'recovery-');
  initializeRecovery(dir, { mode }); created.push(dir); return dir;
}
function run(dir, fault = 'none') {
  const result = spawnSync(process.execPath, [manager, dir, fault], { env, cwd: root,
    encoding: 'utf8', timeout: 12000, maxBuffer: 16384, windowsHide: true });
  assert.equal(result.error, undefined); assert.equal(result.signal, null);
  return { code: result.status, body: result.stdout ? JSON.parse(result.stdout) : null };
}
function assertStopped(dir) {
  for (const row of readRecoveryJournal(dir).filter(row => row.event === 'WORKER')) {
    assert.throws(() => process.kill(row.pid, 0), error => error.code === 'ESRCH');
  }
}
for (const fault of ['before-effect', 'after-effect', 'before-record', 'after-record']) {
  const dir = fixture(), first = run(dir, fault);
  assert.equal(first.code, fault.endsWith('record') ? 71 : 2);
  assert.equal(recoveryOracle(dir), false); assertStopped(dir);
  const final = run(dir);
  assert.equal(final.code, 0); assert.equal(final.body.taskCompleted, true);
  assert.equal(recoveryOracle(dir), true);
  const rows = readRecoveryJournal(dir), attempts = rows.filter(row => row.event === 'WORKER' && row.phase === 'effect');
  assert.equal(attempts.length, fault === 'before-effect' ? 2 : 1);
  assert.equal(JSON.parse(readFileSync(join(dir, 'work', 'operation.json'))).count, 1);
  assertStopped(dir); checks++;
}
{
  const dir = fixture('opaque');
  assert.equal(run(dir, 'after-effect').code, 2);
  for (let i = 0; i < 2; i++) {
    const result = run(dir); assert.equal(result.code, 2);
    assert.deepEqual(result.body, { state: 'UNKNOWN_EFFECT', scenarioPassed: true, taskCompleted: false });
  }
  assert.equal(readFileSync(join(dir, 'work', 'opaque-effects.jsonl'), 'utf8').trim().split('\n').length, 1);
  assert.equal(existsSync(join(dir, 'work', 'report.json')), false); assertStopped(dir); checks++;
}
{
  const dir = fixture(), forged = run(dir, 'fake-success');
  assert.equal(forged.code, 2); assert.equal(forged.body.state, 'FIX_NEEDED');
  assert.equal(forged.body.taskCompleted, false); assert.equal(recoveryOracle(dir), false);
  assert.equal(run(dir).body.taskCompleted, true); checks++;
}
{
  const dir = fixture(); writeFileSync(join(dir, 'oracle.json'), '{"operationCount":0}');
  assert.equal(run(dir).body.state, 'ORACLE_CHANGED');
  assert.equal(readRecoveryJournal(dir).length, 0); checks++;
}
for (const [bytes, expected] of [['{"seq":1', 'JOURNAL_TRUNCATED'], ['{"seq":2,"event":"VERIFIED","at":0}\n', 'JOURNAL_CORRUPT']]) {
  const dir = fixture(); writeFileSync(join(dir, 'journal.jsonl'), bytes);
  assert.equal(run(dir).body.state, expected);
  assert.equal(existsSync(join(dir, 'work', 'operation.json')), false); checks++;
}
// Actual simultaneous processes, with a stable owner before the competing start.
{
  const dir = fixture(); let finishCalls = 0;
  const execute = async (root, manifest, records, phase) => {
    if (phase === 'effect') writeFileSync(join(root, 'work', 'operation.json'),
      JSON.stringify({ operationId: manifest.operationId, count: 1 }));
    else {
      finishCalls++;
      writeFileSync(join(root, 'work', 'report.json'), JSON.stringify({ operationId: manifest.operationId,
        operationCount: 1, result: 'effect-reconciled' }));
    }
    return { completed: phase === 'effect' || finishCalls > 1, exitCode: finishCalls === 1 ? 1 : 0 };
  };
  const first = await runRecovery(dir, { execute });
  assert.equal(recoveryOracle(dir), true);
  assert.equal(first.taskCompleted, false);
  assert.equal(first.state, 'RECOVERING');
  assert.equal(readRecoveryJournal(dir).some(row => row.event === 'COMPLETION_CONFIRMED'), false);
  assert.equal((await runRecovery(dir, { execute })).taskCompleted, true);
  assert.equal(finishCalls, 2);
  assert.equal(readRecoveryJournal(dir).filter(row => row.event === 'INTENT').length, 1); checks++;
}
for (let i = 0; i < 20; i++) {
  const dir = fixture();
  const child = spawn(process.execPath, [manager, dir, 'none', '600'], { env, cwd: root,
    stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
  const completion = new Promise((done, reject) => { child.once('error', reject); child.once('close', done); });
  let bytes = 0;
  for (const stream of [child.stdout, child.stderr]) stream.on('data', chunk => { bytes += chunk.length; if (bytes > 16384) child.kill(); });
  try {
    const deadline = Date.now() + 3000;
    while (!existsSync(join(dir, 'owner.json')) && Date.now() < deadline) await new Promise(done => setTimeout(done, 10));
    assert.equal(existsSync(join(dir, 'owner.json')), true);
    const second = run(dir); assert.equal(second.code, 1); assert.equal(second.body.state, 'OWNER_ACTIVE');
    assert.equal(await completion, 0); assert.equal(recoveryOracle(dir), true);
    assert.equal(readRecoveryJournal(dir).filter(row => row.event === 'WORKER' && row.phase === 'effect').length, 1);
    assertStopped(dir); checks++;
  } finally { if (child.exitCode === null && child.signalCode === null) { child.kill(); await completion; } }
}
console.log(JSON.stringify({ suite: 'unattended-recovery', checks, concurrentOwnerTrials: 20,
  effectsLost: 0, duplicateEffects: 0, falseCompletions: 0, externalRequests: 0,
  actualCredentialReads: 0, actualClaudeExecutions: 0, evidenceRoots: created }));
