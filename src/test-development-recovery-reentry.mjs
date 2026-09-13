import assert from 'node:assert/strict';
import { appendFileSync, readFileSync, writeFileSync, lstatSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { readDevelopmentSourceState, writeDevelopmentSource, recoverStoppedDevelopmentSource,
  verifyDevelopmentSourceWrites } from '../verification/development-source-files.mjs';
const taskId = 'retry-project', source = publicDevelopmentSource(taskId), parts = JSON.parse(source).files;
const journal = work => row => appendFileSync(join(work, 'events.jsonl'), JSON.stringify(row) + '\n');
if (process.argv[2] === '--write-first') {
  const work = process.argv[3];
  writeDevelopmentSource(work, taskId, source, row => {
    if (row.event === 'SOURCE_FILE_WRITTEN') process.exit(71);
    journal(work)(row);
  });
  throw new Error('SOURCE_FAULT_MISSED');
}
if (process.argv[2] === '--interrupt-recovery') {
  const [work, stage] = process.argv.slice(3), state = readDevelopmentSourceState(work, taskId);
  recoverStoppedDevelopmentSource(work, taskId, row => {
    if (stage === 'claim' && row.event === 'SOURCE_RECOVERY_STARTED'
      || stage === 'last-write' && row.event === 'SOURCE_FILE_WRITTEN') process.exit(72);
    journal(work)(row);
    if (stage === 'start' && row.event === 'SOURCE_RECOVERY_STARTED'
      || stage === 'first-confirm' && row.event === 'SOURCE_FILE_CONFIRMED'
      || stage === 'written' && row.event === 'SOURCE_WRITTEN') process.exit(72);
  }, state.batch.intentHash);
  throw new Error('RECOVERY_FAULT_MISSED');
}
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const roots = []; let checks = 0, children = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const launch = args => {
  const result = spawnSync(process.execPath, [fileURLToPath(import.meta.url), ...args],
    { env, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 8192 });
  children++; equal(result.error, undefined); equal(result.signal, null); equal(result.stdout, ''); equal(result.stderr, '');
  return result;
};
for (const stage of ['claim', 'start', 'first-confirm', 'last-write', 'written']) {
  const fixture = createDevelopmentFixture({ taskId, waitMode: 'check' }); roots.push(fixture.root);
  const original = launch(['--write-first', fixture.work]); equal(original.status, 71);
  const recoverer = launch(['--interrupt-recovery', fixture.work, stage]); equal(recoverer.status, 72);
  const state = readDevelopmentSourceState(fixture.work, taskId);
  equal(state.batch.done, false); equal(state.batch.recovery.ownerPid, recoverer.pid);
  const recoveryPath = join(fixture.work, 'source-write-1-recovery.json'), recoveryBytes = readFileSync(recoveryPath);
  const eventPath = join(fixture.work, 'events.jsonl'), beforeEvents = readFileSync(eventPath);
  const stamps = parts.map(part => lstatSync(join(fixture.work, part.path), { bigint: true }).mtimeNs);
  console.log(JSON.stringify({ stage, root: fixture.root, previousOwnerExited: true, beforeReentry: true }));
  const recovered = recoverStoppedDevelopmentSource(fixture.work, taskId, journal(fixture.work), state.batch.intentHash, sourceHash(recoveryBytes));
  const alreadyWritten = ['last-write', 'written'].includes(stage);
  equal(recovered, { source, writes: alreadyWritten ? 0 : 1, confirmed: alreadyWritten ? 2 : 1 });
  equal(lstatSync(join(fixture.work, parts[0].path), { bigint: true }).mtimeNs, stamps[0]);
  if (alreadyWritten) equal(lstatSync(join(fixture.work, parts[1].path), { bigint: true }).mtimeNs, stamps[1]);
  equal(readFileSync(recoveryPath), recoveryBytes);
  const afterEvents = readFileSync(eventPath); equal(afterEvents.subarray(0, beforeEvents.length), beforeEvents);
  verifyDevelopmentSourceWrites(source, afterEvents.toString().trim().split('\n').map(JSON.parse), taskId, fixture.work); checks++;
  for (const mutate of [rows => rows.filter(row => row.event !== 'SOURCE_RECOVERY_RESUMED'),
    rows => { const index = rows.findIndex(row => row.event === 'SOURCE_RECOVERY_RESUMED'); rows.splice(index, 0, rows[index]); return rows; },
    rows => { rows.find(row => row.event === 'SOURCE_RECOVERY_RESUMED').recoveryHash = 'a'.repeat(64); return rows; },
    rows => { rows.find(row => row.event === 'SOURCE_RECOVERY_RESUMED').resumeHash = 'a'.repeat(64); return rows; },
    rows => { rows.find(row => row.event === 'SOURCE_RECOVERY_RESUMED').extra = true; return rows; },
    rows => { rows.splice(rows.findIndex(row => row.event === 'SOURCE_FILE_CONFIRMED'), 1); return rows; }]) {
    assert.throws(() => verifyDevelopmentSourceWrites(source, mutate(afterEvents.toString().trim().split('\n').map(JSON.parse)), taskId, fixture.work),
      { message: 'DEVELOPMENT_SOURCE_WRITES_INCOMPLETE' }); checks++;
  }
  equal(recoverStoppedDevelopmentSource(fixture.work, taskId, () => { throw new Error('REPEATED_EFFECT'); }, state.batch.intentHash),
    { source, writes: 0, confirmed: 0 });
  equal(existsSync(join(fixture.work, 'source-write-1-recovery-resume.json')), true);
}
const rejected = [];
for (const label of ['wrong-claim-hash', 'active-owner', 'changed-events', 'changed-source', 'existing-resume', 'invalid-hash']) {
  const fixture = createDevelopmentFixture({ taskId, waitMode: 'check' }); roots.push(fixture.root);
  equal(launch(['--write-first', fixture.work]).status, 71);
  equal(launch(['--interrupt-recovery', fixture.work, 'claim']).status, 72);
  const state = readDevelopmentSourceState(fixture.work, taskId), recoveryPath = join(fixture.work, 'source-write-1-recovery.json');
  let recoveryHash = sourceHash(readFileSync(recoveryPath)), code = 'DEVELOPMENT_SOURCE_BATCH_INVALID';
  if (label === 'wrong-claim-hash') recoveryHash = 'a'.repeat(64);
  if (label === 'invalid-hash') recoveryHash = null;
  if (label === 'active-owner') {
    writeFileSync(recoveryPath, JSON.stringify({ intentHash: state.batch.intentHash, ownerPid: process.pid }) + '\n');
    recoveryHash = sourceHash(readFileSync(recoveryPath)); code = 'DEVELOPMENT_SOURCE_OWNER_UNVERIFIED';
  }
  if (label === 'changed-events') {
    const path = join(fixture.work, 'events.jsonl'), row = JSON.parse(readFileSync(path)); row.sha256 = 'a'.repeat(64);
    writeFileSync(path, JSON.stringify(row) + '\n'); code = 'DEVELOPMENT_SOURCE_WRITES_INCOMPLETE';
  }
  if (label === 'changed-source') {
    appendFileSync(join(fixture.work, parts[1].path), '\n'); code = 'DEVELOPMENT_SOURCE_EFFECT_UNKNOWN';
  }
  if (label === 'existing-resume') {
    const events = readFileSync(join(fixture.work, 'events.jsonl'));
    writeFileSync(join(fixture.work, 'source-write-1-recovery-resume.json'), JSON.stringify({ intentHash: state.batch.intentHash,
      recoveryHash, ownerPid: process.pid, eventsHash: sourceHash(events), eventBytes: events.length }) + '\n', { flag: 'wx' });
    code = 'DEVELOPMENT_SOURCE_RECOVERY_UNCERTAIN';
  }
  const snapshot = () => parts.map(part => ({ source: readFileSync(join(fixture.work, part.path), 'utf8'),
    mtime: lstatSync(join(fixture.work, part.path), { bigint: true }).mtimeNs }));
  const before = snapshot(), beforeEvents = readFileSync(join(fixture.work, 'events.jsonl'));
  assert.throws(() => recoverStoppedDevelopmentSource(fixture.work, taskId, journal(fixture.work), state.batch.intentHash, recoveryHash), { message: code }); checks++;
  equal(snapshot(), before); equal(readFileSync(join(fixture.work, 'events.jsonl')), beforeEvents);
  equal(existsSync(join(fixture.work, 'source-write-1-done.json')), false);
  equal(existsSync(join(fixture.work, 'source-write-1-recovery-resume.json')), label === 'existing-resume');
  rejected.push({ label, code });
}
console.log(JSON.stringify({ suite: 'development-recovery-reentry', checks, children, stages: 5,
  originalClaimAndEventsPreserved: true, confirmedFilesNotRewritten: true, actualModelRequests: 0,
  actualCredentialReads: 0, nativeStarts: 0, powerLossDurability: 'NOT_RUN', rejected, roots }));
