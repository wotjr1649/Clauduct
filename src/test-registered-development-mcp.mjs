import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { publicBudgetTask } from '../verification/fixtures/registered-budget-task.mjs';
import { registerDevelopmentTask } from '../verification/registered-development-tasks.mjs';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
const record = publicBudgetTask(randomUUID()), text = JSON.stringify(record) + '\n';
const taskId = registerDevelopmentTask(text, sourceHash(text)), fixture = createDevelopmentFixture({ taskId, waitMode: 'check' });
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
let checks = 0, mcpChildren = 0, permissionWarnings = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
function invoke(requests) {
  const result = spawnSync(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true, encoding: 'utf8',
    input: requests.map(row => JSON.stringify(row)).join('\n') + '\n', timeout: 10000, maxBuffer: 32768 });
  equal(result.error, undefined); equal(result.status, 0); equal(result.signal, null);
  assert.doesNotMatch(result.stderr, /DEVELOPMENT_FIXTURE_FAILED|ERR_ACCESS_DENIED/); checks++;
  if (result.stderr) {
    assert.match(result.stderr, /^\(node:\d+\) (?:\[[A-Z0-9]+\] )?SecurityWarning: The flag --allow-child-process [^\r\n]+\r?\n\(Use `node --trace-warnings \.\.\.` to show where the warning was created\)\r?\n$/);
    permissionWarnings++;
  }
  mcpChildren++;
  return result.stdout.trim().split('\n').map(JSON.parse);
}
const baseline = invoke([call(1, 'read_task'), call(2, 'run_tests')]);
equal(JSON.parse(baseline[1].result.content[0].text).passed, false);
equal(JSON.parse(baseline[1].result.content[0].text).checks, 37);
const pending = invoke([call(3, 'write_source', { code: record.localFixtureSource }), call(4, 'run_tests')]);
equal(JSON.parse(pending[1].result.content[0].text).testsRun, false);
equal(JSON.parse(pending[1].result.content[0].text).reason, 'SOURCE_REVIEW_REQUIRED');
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ sha256: sourceHash(record.localFixtureSource), approved: true }));
const completed = invoke([call(5, 'run_tests')]);
equal(JSON.parse(completed[0].result.content[0].text).passed, true);
equal(sourceHash(readFileSync(join(fixture.control, 'oracle.mjs'))), fixture.oracleHash);
const denied = invoke([call(6, 'write_source', { code: 'export function remainingExecutionBudget(value) { return process.env; }' })]);
equal(denied[0].error.message, 'DEVELOPMENT_SOURCE_REJECTED');
equal(readFileSync(join(fixture.work, fixture.task.sourceFile), 'utf8'), record.localFixtureSource);
console.log(JSON.stringify({ suite: 'registered-development-mcp', checks, taskId, root: fixture.root,
  mcpChildren, permissionWarnings, oracleCases: 37, inertCodeStringTested: true,
  baselineFailed: true, unreviewedExecutionRejected: true, fixedOraclePassed: true,
  actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
