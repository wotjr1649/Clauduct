import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';

const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const roots = [];
for (const mode of ['sealed', 'changed-source', 'torn-seal']) {
  const fixture = createDevelopmentFixture({ waitMode: 'check' }); roots.push(fixture.root);
  const original = readFileSync(join(fixture.work, fixture.task.sourceFile));
  writeFileSync(join(fixture.control, 'source-sealed.json'), mode === 'torn-seal' ? '{"sha256":'
    : JSON.stringify({ sha256: mode === 'changed-source' ? 'f'.repeat(64) : sourceHash(original) }) + '\n', { flag: 'wx' });
  const request = id => JSON.stringify({ jsonrpc: '2.0', id, method: 'tools/call', params: {
    name: 'write_source', arguments: { code: publicDevelopmentSource() } } });
  // These calls only test write_source; they need no permission to spawn the
  // oracle or any other child process. Keep that capability absent here.
  const result = spawnSync(process.execPath, fixture.args.filter(argument => argument !== '--allow-child-process'), { cwd: fixture.work, env,
    input: request(1) + '\n' + (mode === 'sealed' ? request(2) + '\n' : ''),
    encoding: 'utf8', windowsHide: true, timeout: 3000, maxBuffer: 16384 });
  equal(result.error, undefined); equal(result.signal, null);
  equal(readFileSync(join(fixture.work, fixture.task.sourceFile)), original);
  if (mode === 'sealed') {
    equal(result.status, 0); equal(result.stderr, '');
    const rows = result.stdout.trim().split('\n').map(JSON.parse); equal(rows.length, 2);
    for (const row of rows) {
      equal(row.result.isError, true);
      equal(JSON.parse(row.result.content[0].text), { written: false, reason: 'DEVELOPMENT_SOURCE_SEALED' });
    }
    const events = readFileSync(join(fixture.work, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
    equal(events.map(row => row.event), ['SOURCE_WRITE_BLOCKED', 'SOURCE_WRITE_BLOCKED']);
  } else {
    equal(result.status, 1); equal(result.stdout, ''); equal(result.stderr.trim(), 'DEVELOPMENT_FIXTURE_FAILED');
  }
}
console.log(JSON.stringify({ suite: 'development-source-seal', checks, fixtureRoots: roots, actualMcpProcesses: 3,
  sourceWrites: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
