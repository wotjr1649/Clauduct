import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, BASELINE_SOURCE, sourceHash } from '../verification/development-fixture.mjs';

const fixture = createDevelopmentFixture({ waitMode: 'check' });
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
function invoke(requests) {
  const result = spawnSync(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true, encoding: 'utf8',
    input: requests.map(row => JSON.stringify(row)).join('\n') + '\n', timeout: 10000, maxBuffer: 16384 });
  assert.equal(result.error, undefined); assert.equal(result.status, 0);
  assert.doesNotMatch(result.stderr, /DEVELOPMENT_FIXTURE_FAILED/);
  return result.stdout.trim().split('\n').map(line => JSON.parse(line));
}
const first = invoke([call(1, 'read_task'), call(2, 'run_tests'), call(3, 'write_source', { code: '', path: '../control/oracle.mjs' })]);
assert.equal(JSON.parse(first[0].result.content[0].text).source, BASELINE_SOURCE);
assert.equal(JSON.parse(first[1].result.content[0].text).passed, false);
assert.equal(JSON.parse(first[1].result.content[0].text).testsRun, true);
assert.equal(first[2].error.code, -32602);
assert.equal(readFileSync(join(fixture.work, 'retry-after-seconds.mjs'), 'utf8'), BASELINE_SOURCE);
const reviewedSource = `export function parseRetryAfterSeconds(value) {
  if (typeof value !== 'string' || value.length > 128 || /[\\r\\n]/.test(value) || !/^[ \\t]*[0-9]+[ \\t]*$/.test(value)) return null;
  const result = Number(value) * 1000;
  return Number.isSafeInteger(result) ? result : null;
}
`;
const pending = invoke([call(1, 'write_source', { code: reviewedSource }), call(2, 'run_tests')]);
assert.equal(JSON.parse(pending[1].result.content[0].text).testsRun, false);
assert.equal(JSON.parse(pending[1].result.content[0].text).reason, 'SOURCE_REVIEW_REQUIRED');
assert.equal(sourceHash(readFileSync(join(fixture.control, 'oracle.mjs'))), fixture.oracleHash);
const before = readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(line => JSON.parse(line));
assert.equal(before.filter(row => row.event === 'TESTS_EXECUTED').length, 1);
// The test author reviewed this constant pure function. Only that exact revision may execute.
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ sha256: sourceHash(reviewedSource), approved: true }));
const final = invoke([call(1, 'run_tests')]);
assert.equal(JSON.parse(final[0].result.content[0].text).passed, true);
assert.equal(JSON.parse(final[0].result.content[0].text).checks, 28);
const rejected = invoke([call(1, 'write_source', { code: 'export function parseRetryAfterSeconds(value) { return process.env; }' })]);
assert.equal(rejected[0].error.message, 'DEVELOPMENT_SOURCE_REJECTED');
assert.equal(readFileSync(join(fixture.work, 'retry-after-seconds.mjs'), 'utf8'), reviewedSource);
console.log(JSON.stringify({ suite: 'development-fixture', checks: 15, independentOracleCases: 28,
  baselineFailed: true, unreviewedExecutionRejected: true, reviewedImplementationPassed: true,
  externalRequests: 0, actualCredentialReads: 0, evidenceRoot: fixture.root }));
