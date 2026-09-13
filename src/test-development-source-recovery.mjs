import assert from 'node:assert/strict';
import { appendFileSync, readFileSync, writeFileSync, lstatSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { developmentTask } from '../verification/development-tasks.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { readDevelopmentSource, readDevelopmentSourceBatches, writeDevelopmentSource, writeDevelopmentSourceWithRecovery,
  verifyDevelopmentSourceWrites } from '../verification/development-source-files.mjs';
import { readDevelopmentArtifactHashes, verifyDevelopmentArtifacts } from '../verification/development-artifacts.mjs';
import { verifyNativeDevelopment } from '../verification/verify-native-development.mjs';
const taskId = 'retry-project', task = developmentTask(taskId), source = publicDevelopmentSource(taskId), files = JSON.parse(source).files;
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let checks = 0, mcpChildren = 0, sourceWorkers = 0;
const roots = [], equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const fresh = options => { const value = createDevelopmentFixture({ taskId, waitMode: 'check', ...options }); roots.push(value.root); return value; };
const journal = fixture => row => appendFileSync(join(fixture.work, 'events.jsonl'), JSON.stringify(row) + '\n');
const events = fixture => readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
const stamp = path => lstatSync(path, { bigint: true }).mtimeNs;
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
function invoke(fixture, requests) {
  const result = spawnSync(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true, encoding: 'utf8',
    input: requests.map(JSON.stringify).join('\n') + '\n', timeout: 15000, maxBuffer: 32768 });
  mcpChildren++; equal(result.error, undefined); equal(result.status, 0);
  assert.doesNotMatch(result.stderr, /DEVELOPMENT_FIXTURE_FAILED/); checks++;
  return result.stdout.trim().split('\n').map(JSON.parse);
}
const value = reply => JSON.parse(reply.result.content[0].text);
const fixture = fresh();
equal(value(invoke(fixture, [call(1, 'read_task'), call(2, 'run_tests')])[1]).passed, false);
let beforeRecovery;
const record = journal(fixture);
const recovered = writeDevelopmentSourceWithRecovery(fixture.work, taskId, source, row => {
  if (row.event === 'SOURCE_WORKER_INTERRUPTED') {
    sourceWorkers++; beforeRecovery = stamp(join(fixture.work, files[0].path));
    equal(readFileSync(join(fixture.work, files[0].path), 'utf8'), files[0].code);
    equal(readFileSync(join(fixture.work, files[1].path), 'utf8'), developmentTask(task.parts[1].taskId).baseline);
    equal(events(fixture).filter(row => row.event === 'SOURCE_FILE_WRITTEN').length, 0);
  }
  record(row);
});
equal(recovered, { source, writes: 1, confirmed: 1 }); equal(stamp(join(fixture.work, files[0].path)), beforeRecovery);
equal(readDevelopmentSource(fixture.work, taskId), source);
equal(events(fixture).filter(row => row.event === 'SOURCE_FILE_CONFIRMED').map(row => row.path), [files[0].path]);
equal(events(fixture).filter(row => row.event === 'SOURCE_FILE_WRITTEN').map(row => row.path), [files[1].path]);
verifyDevelopmentSourceWrites(source, events(fixture), taskId, fixture.work); checks++;
const afterEvents = readFileSync(join(fixture.work, 'events.jsonl'), 'utf8'), secondStamp = stamp(join(fixture.work, files[1].path));
equal(writeDevelopmentSourceWithRecovery(fixture.work, taskId, source, record), { source, writes: 0, confirmed: 0 });
equal(readFileSync(join(fixture.work, 'events.jsonl'), 'utf8'), afterEvents); equal(stamp(join(fixture.work, files[1].path)), secondStamp);
equal(value(invoke(fixture, [call(1, 'run_tests')])[0]).testsRun, false);
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ sha256: sourceHash(source), approved: true }));
const tested = value(invoke(fixture, [call(1, 'run_tests')])[0]); equal(tested.passed, true); equal(tested.checks, 81);
const hashes = readDevelopmentArtifactHashes(fixture.root, taskId), result = { taskId, sourceSha256: sourceHash(source), artifactHashes: hashes };
const budget = { taskHash: fixture.taskHash, oracleHash: fixture.oracleHash, mcpHash: hashes.mcp };
verifyDevelopmentArtifacts(fixture.root, result, budget); checks++;
appendFileSync(join(fixture.work, 'source-write-1-recovery.json'), '\n');
assert.throws(() => verifyDevelopmentArtifacts(fixture.root, result, budget), { message: 'DEVELOPMENT_ARTIFACT_CHANGED' }); checks++;

const throughMcp = fresh({ recoverAfterFirstSourceWrite: true });
const mcp = invoke(throughMcp, [call(1, 'read_task'), call(2, 'run_tests'), call(3, 'write_source', JSON.parse(source)), call(4, 'run_tests')]);
equal(value(mcp[1]).passed, false); equal(value(mcp[2]).written, true); equal(value(mcp[3]).testsRun, false); sourceWorkers++;
writeFileSync(join(throughMcp.control, 'review.json'), JSON.stringify({ sha256: sourceHash(source), approved: true }));
equal(value(invoke(throughMcp, [call(1, 'run_tests')])[0]).passed, true);
verifyDevelopmentSourceWrites(source, events(throughMcp), taskId, throughMcp.work); checks++;
for (const mutate of [rows => rows.filter(row => row.event !== 'SOURCE_WRITE_STARTED'),
  rows => rows.filter(row => row.event !== 'SOURCE_RECOVERY_STARTED'),
  rows => { const at = rows.findIndex(row => row.event === 'SOURCE_RECOVERY_STARTED'); rows.splice(at, 0, rows[at]); return rows; },
  rows => { const at = rows.findIndex(row => row.event === 'SOURCE_FILE_WRITTEN'); rows.splice(at, 0, rows[at]); return rows; },
  rows => { rows.find(row => row.event === 'SOURCE_FILE_WRITTEN').path = files[0].path; return rows; },
  rows => { rows.find(row => row.event === 'SOURCE_FILE_CONFIRMED').sha256 = 'a'.repeat(64); return rows; },
  rows => { rows.find(row => row.event === 'SOURCE_FILE_CONFIRMED').extra = true; return rows; },
  rows => rows.filter(row => row.event !== 'SOURCE_WRITTEN')]) {
  assert.throws(() => verifyDevelopmentSourceWrites(source, mutate(events(throughMcp)), taskId, throughMcp.work),
    { message: 'DEVELOPMENT_SOURCE_WRITES_INCOMPLETE' }); checks++;
}

const negative = [
  ['source-path', 'DEVELOPMENT_SOURCE_BATCH_INVALID', (fixture, intent) => { intent.files[1].path = '../control/oracle.mjs'; }],
  ['source-owner', 'DEVELOPMENT_SOURCE_BATCH_INVALID', (fixture, intent) => { intent.ownerPid = process.pid; }],
  ['work-binding', 'DEVELOPMENT_SOURCE_BATCH_INVALID', (fixture, intent) => { intent.workHash = 'a'.repeat(64); }],
  ['changed-proposal', 'DEVELOPMENT_SOURCE_BATCH_INVALID', (fixture, intent) => {
    intent.source = intent.source.replace('value.retryAtMs >= value.deadlineMs', 'value.retryAtMs > value.deadlineMs');
  }],
  ['first-source', 'DEVELOPMENT_SOURCE_EFFECT_UNKNOWN', fixture => { appendFileSync(join(fixture.work, files[0].path), '\n'); }],
  ['second-source', 'DEVELOPMENT_SOURCE_EFFECT_UNKNOWN', fixture => { appendFileSync(join(fixture.work, files[1].path), '\n'); }],
  ['same-bytes-new-write', 'DEVELOPMENT_SOURCE_EFFECT_UNKNOWN', fixture => {
    const path = join(fixture.work, files[1].path); writeFileSync(path, readFileSync(path));
  }],
  ['uncertain-recovery', 'DEVELOPMENT_SOURCE_RECOVERY_UNCERTAIN', fixture => {
    const batch = readDevelopmentSourceBatches(fixture.work, taskId)[0];
    writeFileSync(join(fixture.work, 'source-write-1-recovery.json'), JSON.stringify({ intentHash: batch.intentHash, ownerPid: process.pid }) + '\n', { flag: 'wx' });
  }]
];
for (const [label, code, mutate] of negative) {
  const current = fresh(), write = journal(current); let afterMutation;
  assert.throws(() => writeDevelopmentSourceWithRecovery(current.work, taskId, source, row => {
    write(row);
    if (row.event === 'SOURCE_WORKER_INTERRUPTED') {
      sourceWorkers++;
      const path = join(current.work, 'source-write-1.json'), text = readFileSync(path, 'utf8'), intent = JSON.parse(text);
      mutate(current, intent);
      if (JSON.stringify(intent) + '\n' !== text) writeFileSync(path, JSON.stringify(intent) + '\n');
      afterMutation = task.parts.map(part => ({ bytes: readFileSync(join(current.work, part.path), 'utf8'), stamp: stamp(join(current.work, part.path)) }));
    }
  }), { message: code }, label); checks++;
  equal(task.parts.map(part => ({ bytes: readFileSync(join(current.work, part.path), 'utf8'), stamp: stamp(join(current.work, part.path)) })), afterMutation);
  equal(existsSync(join(current.work, 'source-write-1-done.json')), false);
  if (label !== 'uncertain-recovery') equal(existsSync(join(current.work, 'source-write-1-recovery.json')), false);
}
const normal = fresh(), write = journal(normal);
writeDevelopmentSource(normal.work, taskId, source, write);
const revised = source.replace('const milliseconds = Number(value) * 1000;', 'const ms = Number(value) * 1000;')
  .replace('Number.isSafeInteger(milliseconds) ? milliseconds', 'Number.isSafeInteger(ms) ? ms');
writeDevelopmentSource(normal.work, taskId, revised, write);
equal(readDevelopmentSourceBatches(normal.work, taskId).length, 2);
verifyDevelopmentSourceWrites(revised, events(normal), taskId, normal.work); checks++;
assert.throws(() => writeDevelopmentSource(normal.work, taskId, source, write), { message: 'DEVELOPMENT_SOURCE_BATCH_INVALID' }); checks++;
const base = { model: 'sol', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true, taskId,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0, recoverAfterFirstSourceWrite: true };
for (const options of [{ localNative: false }, { recoverAfterFirstSourceWrite: 'true' }, { taskId: 'retry-after-seconds' },
  { holdAfterTaskRead: true }, { holdAfterSourceWrite: true }, { cutOutputAfterPass: true }, { earlyExitAfterRead: true },
  { resumeRoot: 'public' }, { resumeIncompleteRoot: 'public' }, { resumeInterruptedRoot: 'public' }, { continueFrom: 'public' }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...options }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
console.log(JSON.stringify({ suite: 'development-source-recovery', checks, sourceWorkers, mcpChildren, oracleCases: 81,
  firstFileReceiptLost: true, firstFileNotRewritten: true, secondFileRecovered: true, unreviewedExecutionRejected: true,
  repeatedCompletedWriteEffects: 0, rejectedMutations: negative.length, actualNativeExecutions: 0, actualModelRequests: 0,
  actualCredentialReads: 0, managerCrashRecovery: false, powerLossDurability: 'NOT_RUN', roots }));
