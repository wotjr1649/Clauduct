import assert from 'node:assert/strict';
import { writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { createDevelopmentFixture } from '../verification/development-fixture.mjs';
import { publicDevelopmentEvents } from '../verification/fixtures/development-responses.mjs';
import { verifyNativeDevelopment, verifyDevelopmentIncompleteRecovery } from '../verification/verify-native-development.mjs';

let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
for (const [model, effort] of [['sol', 'low'], ['luna', 'max']]) {
  const events = (serial, variant, finish = false) => publicDevelopmentEvents(`gpt-5.6-${model}`, effort,
    serial, finish, 'retry-delay-window', variant);
  const read = events(1, 'early').at(-1).response.output[0];
  const ending = events(2, 'early').at(-1).response.output[0];
  equal(read.name, 'mcp__fixture__read_task');
  equal(ending.type, 'message');
  equal(ending.content[0].text, 'PUBLIC_TASK_READ_ONLY');
  equal(events(3, 'retry').at(-1).response.output[0].name, 'mcp__fixture__write_source');
  equal(events(5, 'retry').at(-1).response.output[0].content[0].text, 'CLAUDUCT_DEVELOPMENT_DONE');
  equal(read.call_id === events(1, 'retry').at(-1).response.output[0].call_id, false);
  for (const action of [() => events(3, 'early'), () => events(0, 'early'), () => events(1, 'unknown'),
    () => events(1, 'early', true), () => events(1, 'retry', true)]) {
    assert.throws(action, { message: 'DEVELOPMENT_STIMULUS_INVALID' }); checks++;
  }
}
const base = { model: 'sol', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 };
for (const extra of [{ earlyExitAfterRead: 'true' }, { earlyExitAfterRead: true, localNative: false },
  { earlyExitAfterRead: true, cutOutputAfterPass: true }, { earlyExitAfterRead: true, resumeRoot: 'public' },
  { earlyExitAfterRead: true, resumeIncompleteRoot: 'public' }, { earlyExitAfterRead: true, continueFrom: 'public' },
  { resumeIncompleteRoot: 1 }, { resumeIncompleteRoot: 'public', resumeRoot: 'public' },
  { resumeIncompleteRoot: 'public', cutOutputAfterPass: true }, { resumeIncompleteRoot: 'public', continueFrom: 'public' }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...extra }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
await assert.rejects(verifyNativeDevelopment({ ...base, resumeIncompleteRoot: 'C:\\PublicFixture\\outside-root' }),
  { message: 'RESUME_ROOT_INVALID' }); checks++;
for (const localNative of [false, undefined, 'true']) {
  await assert.rejects(verifyDevelopmentIncompleteRecovery({ ...base, localNative }),
    { message: 'LOCAL_INCOMPLETE_STIMULUS_REQUIRED' }); checks++;
}
// These fresh public records exercise the new admission conditions. A valid
// record still names this live test process, so the next owner check must deny
// it before an intent or a native child can be created.
const admission = async change => {
  const fixture = createDevelopmentFixture({ waitMode: 'check', taskId: 'retry-delay-window' });
  const first = { taskIncompleteBeforeEffect: true, failure: 'DEVELOPMENT_TASK_INCOMPLETE', nativeJson: true,
    root: fixture.root, model: 'sol', localNative: true, taskId: 'retry-delay-window', phase: 'development',
    sessionId: '11111111-1111-1111-1111-111111111111', sourceUnchanged: true, recordedNativeStopped: true,
    nativeError: false, exitCode: 0, cleanupComplete: true, output: { resultCount: 1, statusCount: 1, stdoutEnded: true, stderrEnded: true },
    ledgerMatched: true, attemptsComplete: true, usageUnobservedAttempts: 0, passed: false, tree: { stopped: true } };
  change?.(first);
  writeFileSync(join(fixture.root, 'budget.json'), JSON.stringify({ model: 'sol', localNative: true,
    taskId: 'retry-delay-window', cutOutputAfterPass: false }), { flag: 'wx' });
  writeFileSync(join(fixture.root, 'result.json'), JSON.stringify(first), { flag: 'wx' });
  writeFileSync(join(fixture.root, 'process.json'), JSON.stringify({ sessionId: '11111111-1111-1111-1111-111111111111' }), { flag: 'wx' });
  writeFileSync(join(fixture.root, 'transport-development.jsonl'), JSON.stringify({ event: 'NATIVE_STARTED', pid: process.pid }) + '\n', { flag: 'wx' });
  await assert.rejects(verifyNativeDevelopment({ ...base, taskId: 'retry-delay-window', resumeIncompleteRoot: fixture.root }),
    { message: change ? 'RESUME_EVIDENCE_INVALID' : 'RESUME_OWNER_UNVERIFIED' }); checks++;
  equal(existsSync(join(fixture.root, 'retry-intent.json')), false);
  equal(existsSync(join(fixture.root, 'process-retry.json')), false);
};
for (const change of [row => { row.taskIncompleteBeforeEffect = false; }, row => { row.failure = null; },
  row => { row.root = 'C:\\PublicFixture\\outside-root'; }, row => { row.model = 'luna'; }, row => { row.localNative = false; },
  row => { row.taskId = 'retry-after-seconds'; }, row => { row.phase = 'development-retry'; }, row => { row.sessionId = 'other'; },
  row => { row.sourceUnchanged = false; }, row => { row.recordedNativeStopped = false; },
  row => { row.nativeJson = false; }, row => { row.nativeError = true; }, row => { row.exitCode = 1; },
  row => { row.cleanupComplete = false; }, row => { row.output.resultCount = 0; }, row => { row.output.statusCount = 2; },
  row => { row.output.stdoutEnded = false; }, row => { row.output.stderrEnded = false; },
  row => { row.ledgerMatched = false; }, row => { row.attemptsComplete = false; },
  row => { row.usageUnobservedAttempts = null; }, row => { row.usageUnobservedAttempts = 1; },
  row => { row.passed = true; }, row => { row.tree.stopped = false; }]) await admission(change);
await admission(null);
console.log(JSON.stringify({ suite: 'development-incomplete', checks, actualNativeExecutions: 0,
  actualModelRequests: 0, actualCredentialReads: 0 }));
