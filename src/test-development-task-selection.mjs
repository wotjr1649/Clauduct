import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { developmentTask } from '../verification/development-tasks.mjs';
import { checkDevelopmentSource } from '../verification/development-source-policy.mjs';
import { createFixtureToolPolicy } from '../verification/fixture-tool-policy.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { verifyNativeDevelopment } from '../verification/verify-native-development.mjs';

let checks = 0;
const taskId = 'retry-delay-window', task = developmentTask(taskId), source = publicDevelopmentSource(taskId);
const denied = callback => { assert.throws(callback); checks++; };
const equal = (value, expected) => { assert.deepEqual(value, expected); checks++; };
for (const id of ['', '../control/oracle.mjs', 'constructor', '__proto__', 'toString', null, ['retry-delay-window'], {}]) {
  denied(() => developmentTask(id));
  denied(() => createDevelopmentFixture({ taskId: id }));
}
denied(() => { task.members.isSafeInteger.push('value'); });
equal(checkDevelopmentSource(task.baseline, taskId), true);
equal(checkDevelopmentSource(source, taskId), true);
denied(() => checkDevelopmentSource(source));
denied(() => checkDevelopmentSource(publicDevelopmentSource(), taskId));
const body = text => `export function retryDelayWithinBudget(value) { ${text} }`;
for (const text of ['return process.env;', 'return globalThis;', 'return import("node:fs");',
  'return value.constructor;', 'return value["nowMs"];', 'return Object.keys(value);',
  'return Array.constructor;', 'return value.isSafeInteger(value);', 'return Array.isSafeInteger(value);',
  'return Number.isArray(value);', 'value.nowMs = 0; return null;', 'Number.isSafeInteger = value;',
  'const Array = value; return null;', 'return value.nowMs();', 'return (value.nowMs)();',
  'return (value)();', 'return retryDelayWithinBudget(value);', 'while (value) {}',
  'return 5000;', 'return "public";', 'return value.nowMs; } export const now = 0; {']) {
  denied(() => checkDevelopmentSource(body(text), taskId));
}

const event = (name, args = {}, id = `call_public_${checks}`) => ({ type: 'response.completed', response: {
  output: [{ type: 'function_call', name: `mcp__fixture__${name}`, arguments: JSON.stringify(args), call_id: id }] } });
const policy = createFixtureToolPolicy({ version: 1, kind: 'development', taskId, workingRoot: process.cwd() });
for (const value of [event('read_task', {}, 'call_read'), event('run_tests', {}, 'call_baseline'),
  event('write_source', { code: source }, 'call_write'), event('run_tests', {}, 'call_finish')]) {
  assert.doesNotThrow(() => policy(value)); checks++;
}
for (const invalid of [{ kind: 'none', taskId }, { kind: 'development', taskId: '../control' }]) {
  denied(() => createFixtureToolPolicy({ version: 1, workingRoot: process.cwd(), ...invalid }));
}
const wrong = createFixtureToolPolicy({ version: 1, kind: 'development', taskId, workingRoot: process.cwd() });
wrong(event('read_task', {}, 'call_wrong_read')); wrong(event('run_tests', {}, 'call_wrong_baseline'));
denied(() => wrong(event('write_source', { code: publicDevelopmentSource() }, 'call_wrong_write')));

const fixture = createDevelopmentFixture({ waitMode: 'check', taskId });
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
function invoke(requests) {
  const result = spawnSync(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true, encoding: 'utf8',
    input: requests.map(row => JSON.stringify(row)).join('\n') + '\n', timeout: 10000, maxBuffer: 16384 });
  assert.equal(result.error, undefined); assert.equal(result.status, 0);
  assert.doesNotMatch(result.stderr, /DEVELOPMENT_FIXTURE_FAILED/);
  return result.stdout.trim().split('\n').map(JSON.parse);
}
const decode = reply => JSON.parse(reply.result.content[0].text);
const first = invoke([call(1, 'read_task'), call(2, 'run_tests')]);
equal(decode(first[0]).source, task.baseline);
equal(decode(first[0]).task, task.task);
equal(decode(first[1]).checks, 32);
equal(decode(first[1]).passed, false);
equal(decode(first[1]).testsRun, true);
const proposed = invoke([call(1, 'write_source', { code: source }), call(2, 'run_tests')]);
equal(decode(proposed[1]).testsRun, false);
equal(decode(proposed[1]).reason, 'SOURCE_REVIEW_REQUIRED');
const sourcePath = join(fixture.work, task.sourceFile);
equal(readFileSync(sourcePath, 'utf8'), source);
const prohibited = invoke([call(1, 'write_source', { code: source, taskId: 'retry-after-seconds' }),
  call(2, 'write_source', { code: publicDevelopmentSource() }),
  call(3, 'read_task', { path: '../control' })]);
equal(prohibited.map(reply => reply.error.code), [-32602, -32602, -32602]);
equal(readFileSync(sourcePath, 'utf8'), source);
equal(existsSync(join(fixture.work, 'retry-after-seconds.mjs')), false);
equal(sourceHash(readFileSync(join(fixture.control, 'oracle.mjs'))), fixture.oracleHash);
// Only the test author's reviewed constant above may execute in this check.
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ sha256: sourceHash(source), approved: true }));
const final = decode(invoke([call(1, 'run_tests')])[0]);
equal({ passed: final.passed, testsRun: final.testsRun, checks: final.checks, failures: final.failures },
  { passed: true, testsRun: true, checks: 32, failures: [] });
const events = readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
equal(events.filter(row => row.event === 'SOURCE_WRITTEN').length, 1);
equal(events.filter(row => row.event === 'TESTS_EXECUTED').map(row => row.passed), [false, true]);
// Public invalid resume metadata: mismatch must fail before owner inspection,
// creating an intent, or starting a native child. These are not live evidence.
writeFileSync(join(fixture.root, 'budget.json'), JSON.stringify({ model: 'luna', localNative: true,
  taskId: 'retry-after-seconds', cutOutputAfterPass: true }), { flag: 'wx' });
writeFileSync(join(fixture.root, 'result.json'), JSON.stringify({ outputCutObserved: true, passed: false,
  tree: { stopped: true } }), { flag: 'wx' });
writeFileSync(join(fixture.root, 'process.json'), JSON.stringify({ sessionId: '00000000-0000-4000-8000-000000000001' }), { flag: 'wx' });
await assert.rejects(verifyNativeDevelopment({ model: 'luna', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true,
  taskId, resumeRoot: fixture.root, priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 }),
{ message: 'RESUME_EVIDENCE_INVALID' }); checks++;
equal(existsSync(join(fixture.root, 'finish-intent.json')), false);
equal(readFileSync(sourcePath, 'utf8'), source);
console.log(JSON.stringify({ suite: 'development-task-selection', checks, taskId, oracleChecks: 32,
  baselineFailed: true, reviewedSourcePassed: true, unreviewedExecutionRejected: true, evidenceRoot: fixture.root,
  actualModelRequests: 0, actualCredentialReads: 0, externalRequests: 0 }));
