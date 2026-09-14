import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { developmentTask } from '../verification/development-tasks.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { developmentSourceFromArguments, developmentSourceArguments } from '../verification/development-source-policy.mjs';
import { readDevelopmentSource, readDevelopmentOracle, writeDevelopmentSource, verifyDevelopmentSourceWrites } from '../verification/development-source-files.mjs';
import { readDevelopmentArtifactHashes, verifyDevelopmentArtifacts } from '../verification/development-artifacts.mjs';
import { classifyDevelopmentInterruption } from '../verification/verify-native-development.mjs';
const taskId = 'retry-project', task = developmentTask(taskId), source = publicDevelopmentSource(taskId);
const fixture = createDevelopmentFixture({ taskId, waitMode: 'check' });
let checks = 0, children = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
equal(readDevelopmentSource(fixture.work, taskId), task.baseline);
const input = developmentSourceArguments(source, taskId);
equal(input.files.length, 2);
equal(developmentSourceFromArguments({ files: input.files.map(file => ({ code: file.code, path: file.path })) }, taskId), source);
for (const mutate of [value => { value.files.pop(); }, value => { value.files.push(value.files[0]); },
  value => { value.files.reverse(); }, value => { value.files[1].path = value.files[0].path; },
  value => { value.files[1].path = '../control/oracle.mjs'; }, value => { value.files[0].path = 'RETRY-AFTER-SECONDS.mjs'; },
  value => { value.files[1].code = "export function retryDelayWithinBudget(value) { return process.env; }"; },
  value => { value.files[1].extra = true; }, value => { value.extra = true; }, value => { value.files[1].code = 'x'.repeat(8193); }]) {
  const value = structuredClone(input); mutate(value);
  assert.throws(() => developmentSourceFromArguments(value, taskId), { message: 'DEVELOPMENT_SOURCE_REJECTED' }); checks++;
  equal(readDevelopmentSource(fixture.work, taskId), task.baseline);
}
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
function invoke(target, requests) {
  const result = spawnSync(process.execPath, target.args, { cwd: target.work, env, windowsHide: true, encoding: 'utf8',
    input: requests.map(row => JSON.stringify(row)).join('\n') + '\n', timeout: 10000, maxBuffer: 32768 });
  children++; equal(result.error, undefined); equal(result.status, 0);
  assert.doesNotMatch(result.stderr, /DEVELOPMENT_FIXTURE_FAILED/); checks++;
  return result.stdout.trim().split('\n').map(JSON.parse);
}
const value = reply => JSON.parse(reply.result.content[0].text);
const first = invoke(fixture, [call(1, 'read_task'), call(2, 'run_tests')]);
equal(value(first[0]).files.length, 2); equal(value(first[1]).checks, 81); equal(value(first[1]).passed, false);
const bad = structuredClone(input); bad.files[1].code = 'export function retryDelayWithinBudget(value) { return globalThis; }';
equal(invoke(fixture, [call(1, 'write_source', bad)])[0].error.message, 'DEVELOPMENT_SOURCE_REJECTED');
equal(readDevelopmentSource(fixture.work, taskId), task.baseline);
const pending = invoke(fixture, [call(1, 'write_source', input), call(2, 'run_tests')]);
equal(value(pending[0]).written, true); equal(value(pending[1]).testsRun, false); equal(readDevelopmentSource(fixture.work, taskId), source);
const events = () => readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
equal(events().filter(row => row.event === 'SOURCE_FILE_WRITTEN').map(row => row.path), task.parts.map(part => part.path));
verifyDevelopmentSourceWrites(source, events(), taskId, fixture.work); checks++;
for (const rows of [events().filter(row => row.event !== 'SOURCE_WRITTEN'), events().filter(row => row.event !== 'SOURCE_FILE_WRITTEN')]) {
  assert.throws(() => verifyDevelopmentSourceWrites(source, rows, taskId, fixture.work), { message: 'DEVELOPMENT_SOURCE_WRITES_INCOMPLETE' }); checks++;
}
// This constant public source was reviewed above. Approval binds both files.
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ approved: true, sha256: sourceHash(source) }));
const tested = value(invoke(fixture, [call(1, 'run_tests')])[0]); equal(tested.passed, true); equal(tested.checks, 81);
const hashes = readDevelopmentArtifactHashes(fixture.root, taskId);
const result = { taskId, sourceSha256: sourceHash(source), artifactHashes: hashes };
const budget = { taskHash: fixture.taskHash, oracleHash: fixture.oracleHash, mcpHash: hashes.mcp };
verifyDevelopmentArtifacts(fixture.root, result, budget); checks++;
writeFileSync(join(fixture.control, 'development-window-oracle.mjs'), readFileSync(join(fixture.control, 'development-window-oracle.mjs'), 'utf8') + '\n');
assert.notEqual(sourceHash(readDevelopmentOracle(fixture.control, taskId)), fixture.oracleHash); checks++;
assert.throws(() => verifyDevelopmentArtifacts(fixture.root, result, budget), { message: 'DEVELOPMENT_ARTIFACT_CHANGED' }); checks++;
// No modified oracle executes. A changed second file also invalidates approval.
writeFileSync(join(fixture.work, task.parts[1].path), developmentTask(task.parts[1].taskId).baseline);
equal(value(invoke(fixture, [call(1, 'run_tests')])[0]).testsRun, false);
const untouched = createDevelopmentFixture({ taskId, waitMode: 'check' }), journal = [];
const incomplete = JSON.stringify({ files: input.files.slice(0, 1) }) + '\n';
assert.throws(() => writeDevelopmentSource(untouched.work, taskId, incomplete, row => journal.push(row)), { message: 'DEVELOPMENT_SOURCE_REJECTED' }); checks++;
equal(journal.length, 0); equal(readDevelopmentSource(untouched.work, taskId), task.baseline);
const partial = createDevelopmentFixture({ taskId, waitMode: 'check' }), partialEvents = [];
assert.throws(() => writeDevelopmentSource(partial.work, taskId, source, row => {
  partialEvents.push(row);
  if (row.event === 'SOURCE_FILE_WRITTEN') throw new Error('PUBLIC_RECORD_FAILURE');
}), { message: 'PUBLIC_RECORD_FAILURE' }); checks++;
const partialFiles = JSON.parse(readDevelopmentSource(partial.work, taskId)).files;
equal(partialFiles[0].code, input.files[0].code); equal(partialFiles[1].code, developmentTask(task.parts[1].taskId).baseline);
assert.throws(() => verifyDevelopmentSourceWrites(readDevelopmentSource(partial.work, taskId), partialEvents, taskId, partial.work),
  { message: 'DEVELOPMENT_SOURCE_WRITES_INCOMPLETE' }); checks++;
assert.throws(() => classifyDevelopmentInterruption([{ event: 'TASK_READ' }, ...partialEvents], taskId),
  { message: 'INTERRUPTION_EVIDENCE_INVALID' }); checks++;
console.log(JSON.stringify({ suite: 'development-project', checks, mcpChildren: children, oracleCases: 81,
  allFilesValidatedBeforeWrite: true, aggregateApprovalChecked: true, oracleDependencyChangeRejected: true,
  partialWritePreserved: true, partialWriteReplayAttempts: 0,
  actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0, roots: [fixture.root, untouched.root, partial.root] }));
