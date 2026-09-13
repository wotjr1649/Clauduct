import assert from 'node:assert/strict';
import { appendFileSync, readFileSync, writeFileSync, existsSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { classifyDevelopmentInterruption, verifyNativeDevelopment, readDevelopmentInterruption } from '../verification/verify-native-development.mjs';
import { developmentTask } from '../verification/development-tasks.mjs';
import { sourceHash, createDevelopmentFixture } from '../verification/development-fixture.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { writeDevelopmentSource, readDevelopmentSource, readDevelopmentSourceState, recoverStoppedDevelopmentSource,
  verifyDevelopmentSourceWrites } from '../verification/development-source-files.mjs';
const task = developmentTask('retry-project');
const rows = [{ event: 'TASK_READ' }, { event: 'TESTS_EXECUTED', sha256: sourceHash(task.baseline), passed: false,
  checks: task.checks, failures: ['PUBLIC_BASELINE_FAILURE'] },
{ event: 'SOURCE_WRITE_STARTED', sha256: 'a'.repeat(64), fileCount: 2, intentHash: 'b'.repeat(64) },
{ event: 'SOURCE_FIRST_FILE_WAIT' }];
let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
equal(classifyDevelopmentInterruption(rows, 'retry-project'), 'partial-write');
for (const mutate of [value => value.slice(1), value => value.concat(value[3]), value => value.slice(0, 3),
  value => { value[1].passed = true; return value; }, value => { value[1].checks = 1; return value; },
  value => { value[1].sha256 = 'c'.repeat(64); return value; }, value => { value[1].failures = []; return value; },
  value => { value[2].intentHash = {}; return value; }, value => { value[2].fileCount = 1; return value; },
  value => { value[2].extra = true; return value; }, value => { value[3].extra = true; return value; }]) {
  assert.throws(() => classifyDevelopmentInterruption(mutate(structuredClone(rows)), 'retry-project'),
    { message: 'INTERRUPTION_EVIDENCE_INVALID' }); checks++;
}
const taskId = 'retry-project', source = publicDevelopmentSource(taskId), roots = [];
const fresh = options => { const value = createDevelopmentFixture({ taskId, waitMode: 'check', ...options }); roots.push(value.root); return value; };
const record = fixture => row => appendFileSync(join(fixture.work, 'events.jsonl'), JSON.stringify(row) + '\n');
const owned = fresh();
assert.throws(() => writeDevelopmentSource(owned.work, taskId, source, row => {
  record(owned)(row); if (row.event === 'SOURCE_FILE_WRITTEN') throw new Error('PUBLIC_PARTIAL_POINT');
}), { message: 'PUBLIC_PARTIAL_POINT' }); checks++;
const active = readDevelopmentSourceState(owned.work, taskId), before = readDevelopmentSource(owned.work, taskId);
equal(active.states, ['applied', 'pending']);
assert.throws(() => recoverStoppedDevelopmentSource(owned.work, taskId, record(owned), active.batch.intentHash),
  { message: 'DEVELOPMENT_SOURCE_OWNER_UNVERIFIED' }); checks++;
equal(readDevelopmentSource(owned.work, taskId), before); equal(existsSync(join(owned.work, 'source-write-1-recovery.json')), false);

const fixture = fresh({ holdAfterFirstSourceWrite: true });
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
const input = [call(1, 'read_task'), call(2, 'run_tests'), call(3, 'write_source', JSON.parse(source))].map(JSON.stringify).join('\n') + '\n';
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const child = spawnSync(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true,
  input, encoding: 'utf8', timeout: 10000, maxBuffer: 16384 });
equal(child.error, undefined); equal(child.status, 71); equal(child.signal, null);
assert.doesNotMatch(child.stderr, /DEVELOPMENT_FIXTURE_FAILED/); checks++;
const events = () => readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
equal(classifyDevelopmentInterruption(events(), taskId), 'partial-write');
const state = readDevelopmentSourceState(fixture.work, taskId);
equal(state.batch.intent.ownerPid, child.pid); equal(state.states, ['applied', 'pending']);
const first = join(fixture.work, task.parts[0].path), firstStamp = lstatSync(first, { bigint: true }).mtimeNs;
const recovered = recoverStoppedDevelopmentSource(fixture.work, taskId, record(fixture), state.batch.intentHash);
equal(recovered, { source, writes: 1, confirmed: 1 }); equal(lstatSync(first, { bigint: true }).mtimeNs, firstStamp);
verifyDevelopmentSourceWrites(source, events(), taskId, fixture.work); checks++;
equal(recoverStoppedDevelopmentSource(fixture.work, taskId, record(fixture), state.batch.intentHash), { source, writes: 0, confirmed: 0 });

const oversized = fresh(), replacement = JSON.parse(source);
replacement.files[1].code = 'export function retryDelayWithinBudget(value) { return null; }\n';
const padding = 8192 - Buffer.byteLength(JSON.stringify(replacement) + '\n'); replacement.files[0].code += ' '.repeat(padding);
const boundedFinal = JSON.stringify(replacement) + '\n'; equal(Buffer.byteLength(boundedFinal), 8192);
assert.throws(() => writeDevelopmentSource(oversized.work, taskId, boundedFinal, record(oversized)),
  { message: 'DEVELOPMENT_SOURCE_BATCH_INVALID' }); checks++;
equal(readDevelopmentSource(oversized.work, taskId), task.baseline); equal(existsSync(join(oversized.work, 'source-write-1.json')), false);

const base = { model: 'sol', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true, taskId,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0, holdAfterFirstSourceWrite: true };
for (const options of [{ holdAfterFirstSourceWrite: 'true' }, { localNative: false }, { taskId: 'retry-after-seconds' },
  { holdAfterTaskRead: true }, { holdAfterSourceWrite: true }, { recoverAfterFirstSourceWrite: true }, { cutOutputAfterPass: true },
  { earlyExitAfterRead: true }, { resumeRoot: 'public' }, { resumeIncompleteRoot: 'public' }, { resumeInterruptedRoot: 'public' }, { continueFrom: 'public' }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...options }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
for (const options of [{ holdAfterFirstSourceWrite: 1 }, { taskId: 'retry-after-seconds' },
  { holdAfterPass: true }, { holdAfterSourceWrite: true }, { holdAfterTaskRead: true }, { recoverAfterFirstSourceWrite: true }]) {
  assert.throws(() => createDevelopmentFixture({ taskId, holdAfterFirstSourceWrite: true, ...options }),
    { message: 'INVALID_DEVELOPMENT_MODE' }); checks++;
}
// Invalid completion/usage claims must fail before any native or stop lookup.
// Positive v3 evidence is checked with actual manager interruption separately.
const partialProof = { version: 3, kind: 'manager-interruption', root: 'PUBLIC', model: 'sol', localNative: true,
  taskId, phase: 'development', sessionId: '11111111-1111-1111-1111-111111111111', stage: 'partial-write',
  accountHash: 'a'.repeat(64), budgetHash: 'a'.repeat(64), sourceHash: 'a'.repeat(64), eventsHash: 'a'.repeat(64), eventBytes: 1,
  transportHash: 'a'.repeat(64), usageHash: 'a'.repeat(64), ownerHash: 'a'.repeat(64), configRoot: 'PUBLIC', stopHash: 'a'.repeat(64),
  partialIntentHash: 'a'.repeat(64), originalResultAbsent: true, taskCompleted: false, sourceReviewed: true, independentPassed: false,
  observed: { attempts: null, inputTokens: null, outputTokens: null, elapsedMs: null } };
equal(Object.keys(partialProof).length, 25);
for (const mutate of [value => { value.version = 2; }, value => { value.stage = 'after-write'; },
  value => { value.taskCompleted = true; }, value => { value.independentPassed = true; },
  value => { value.originalResultAbsent = false; }, value => { value.sourceReviewed = false; },
  value => { value.observed.attempts = 0; }, value => { value.observed.elapsedMs = 0; },
  value => { value.observed.extra = null; }, value => { delete value.partialIntentHash; }, value => { value.extra = true; }]) {
  const invalid = fresh(), value = structuredClone(partialProof); mutate(value);
  writeFileSync(join(invalid.root, 'interruption-evidence.json'), JSON.stringify(value) + '\n', { flag: 'wx' });
  assert.throws(() => readDevelopmentInterruption(invalid.root), { message: 'INTERRUPTION_RECORD_INVALID' }); checks++;
  equal(existsSync(join(invalid.root, 'finish-intent.json')), false);
}
console.log(JSON.stringify({ suite: 'development-partial-interruption', checks, publicMcpChildren: 1, firstFileNotRewritten: true,
  remainingFileWrites: 1, originalOwnerActiveRejected: true, intermediateSizeRejectedBeforeWrite: true,
  nativeStarts: 0, actualModelRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN', roots }));
