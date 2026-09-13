import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { writeFileSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { PUBLIC_DEVELOPMENT_SOURCE } from '../verification/fixtures/development-responses.mjs';

const fixture = createDevelopmentFixture({ waitMode: 'check', holdAfterPass: true });
writeFileSync(join(fixture.work, 'retry-after-seconds.mjs'), PUBLIC_DEVELOPMENT_SOURCE);
writeFileSync(join(fixture.control, 'review.json'), JSON.stringify({ sha256: sourceHash(PUBLIC_DEVELOPMENT_SOURCE), approved: true }));
const env = Object.fromEntries(['SystemRoot','WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const child = spawn(process.execPath, fixture.args, { cwd: fixture.work, env, windowsHide: true, stdio: ['pipe','pipe','pipe'] });
let stdout = '', stderr = '', stopAttempted = false;
const watchdog = setTimeout(() => { if (!stopAttempted) { stopAttempted = true; child.kill(); } }, 8000);
child.stdout.on('data', chunk => { stdout += chunk; assert.ok(stdout.length <= 16384); });
child.stderr.on('data', chunk => { stderr += chunk; assert.ok(stderr.length <= 16384); });
const stopped = new Promise(done => child.once('close', code => { clearTimeout(watchdog); done(code); }));
child.stdin.end(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'run_tests', arguments: {} } }) + '\n');
const eventsPath = join(fixture.work, 'events.jsonl'), deadline = Date.now() + 3000;
let rows = [];
while (Date.now() < deadline) {
  if (existsSync(eventsPath)) {
    rows = readFileSync(eventsPath, 'utf8').trim().split('\n').map(JSON.parse);
    if (rows.some(row => row.event === 'OUTPUT_CUT_WAIT')) break;
  }
  await new Promise(done => setTimeout(done, 10));
}
assert.equal(rows.filter(row => row.event === 'TESTS_EXECUTED' && row.passed).length, 1);
assert.equal(rows.filter(row => row.event === 'OUTPUT_CUT_WAIT').length, 1);
assert.equal(stdout, '');
writeFileSync(join(fixture.control, 'output-cut-release.json'), JSON.stringify({ released: true }), { flag: 'wx' });
assert.equal(await stopped, 0); assert.equal(stopAttempted, false); assert.doesNotMatch(stderr, /DEVELOPMENT_FIXTURE_FAILED/);
const reply = JSON.parse(stdout), outcome = JSON.parse(reply.result.content[0].text);
assert.equal(outcome.testsRun, true); assert.equal(outcome.passed, true); assert.equal(outcome.checks, 28);
console.log(JSON.stringify({ suite: 'development-output-barrier', checks: 8, fixtureRoot: fixture.root,
  actualPublicChildren: 1, actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
