import assert from 'node:assert/strict';
import { classifyDevelopmentInterruption, verifyNativeDevelopment } from '../verification/verify-native-development.mjs';
import { developmentTask } from '../verification/development-tasks.mjs';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';

let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const rejects = action => { assert.throws(action, { message: 'INTERRUPTION_EVIDENCE_INVALID' }); checks++; };
for (const taskId of ['retry-after-seconds', 'retry-delay-window']) {
  const task = developmentTask(taskId), read = { event: 'TASK_READ' }, wait = { event: 'TASK_READ_WAIT' };
  const failed = { event: 'TESTS_EXECUTED', sha256: sourceHash(task.baseline), passed: false, checks: task.checks, failures: ['PUBLIC_BASELINE_FAILURE'] };
  equal(classifyDevelopmentInterruption([read], taskId), 'before-write');
  equal(classifyDevelopmentInterruption([read, wait], taskId), 'before-write');
  equal(classifyDevelopmentInterruption([read, failed], taskId), 'before-write');
  equal(classifyDevelopmentInterruption([read, failed, { event: 'SOURCE_WRITTEN', sha256: 'a'.repeat(64) }], taskId), 'after-write');
  for (const events of [null, [], [null], [wait], [read, read], [read, wait, wait], [read, failed, failed],
    [read, { event: 'TESTS_UNRUN' }], [read, { event: 'UNKNOWN_EFFECT' }], [read, { ...wait, ignored: true }],
    [read, { ...failed, passed: true }], [read, { ...failed, sha256: 'b'.repeat(64) }],
    [read, { ...failed, checks: 0 }], [read, { ...failed, failures: [] }],
    [read, { ...failed, failures: ['invalid\nlabel'] }], [read, { event: 'SOURCE_WRITTEN' }],
    [read, failed, { event: 'SOURCE_WRITTEN' }, { event: 'SOURCE_WRITTEN' }]]) {
    rejects(() => classifyDevelopmentInterruption(events, taskId));
  }
}
const base = { model: 'sol', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 };
for (const options of [{ holdAfterTaskRead: 'true' }, { holdAfterTaskRead: true, localNative: false },
  { holdAfterTaskRead: true, holdAfterSourceWrite: true }, { holdAfterTaskRead: true, earlyExitAfterRead: true },
  { holdAfterTaskRead: true, cutOutputAfterPass: true }, { holdAfterTaskRead: true, resumeRoot: 'public' },
  { holdAfterTaskRead: true, resumeIncompleteRoot: 'public' }, { holdAfterTaskRead: true, resumeInterruptedRoot: 'public' },
  { holdAfterTaskRead: true, continueFrom: 'public' }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...options }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
for (const options of [{ holdAfterTaskRead: 1 }, { holdAfterTaskRead: true, holdAfterPass: true },
  { holdAfterTaskRead: true, holdAfterSourceWrite: true }]) {
  assert.throws(() => createDevelopmentFixture(options), { message: 'INVALID_DEVELOPMENT_MODE' }); checks++;
}
console.log(JSON.stringify({ suite: 'development-before-write', checks, nativeStarts: 0, actualModelRequests: 0,
  baselineNotMarkedComplete: true, unknownOrRejectedEffectsNotResumed: true }));
