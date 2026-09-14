import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
const project = dirname(dirname(fileURLToPath(import.meta.url)));
mkdirSync(join(project, '.tmp'), { recursive: true });
const root = mkdtempSync(join(project, '.tmp', 'integration-public-'));
const module = join(root, 'identity.mjs'), oracle = join(project, 'verification', 'fixtures', 'development-integration-oracle.mjs');
writeFileSync(module, 'export function identity(value) { return value; }\n', { flag: 'wx' });
const input = index => ({ kind: 'input', index });
const specification = { version: 1, modules: [{ path: module, exportName: 'identity' }], steps: [
  { moduleIndex: 0, argument: input(0) },
  { moduleIndex: 0, argument: { kind: 'object', fields: { previous: { kind: 'result', index: 0 },
    total: { kind: 'sum', left: { kind: 'result', index: 0 }, right: input(1) }, constant: { kind: 'literal', value: 'PUBLIC' } } } }
], cases: [
  { name: 'sum', inputs: [5, 7], expected: { previous: 5, total: 12, constant: 'PUBLIC' } },
  { name: 'overflow', inputs: [Number.MAX_SAFE_INTEGER, 1], expected: { previous: Number.MAX_SAFE_INTEGER, total: null, constant: 'PUBLIC' } },
  { name: 'no-coercion', inputs: ['5', 7], expected: { previous: '5', total: null, constant: 'PUBLIC' } },
  { name: 'null', inputs: [null, 1], expected: { previous: null, total: null, constant: 'PUBLIC' } },
  { name: 'negative-sum', inputs: [-5, 7], expected: { previous: -5, total: 2, constant: 'PUBLIC' } },
  { name: 'fraction', inputs: [0.5, 1], expected: { previous: 0.5, total: null, constant: 'PUBLIC' } }
] };
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let checks = 0;
function run(name, expectedPass) {
  const path = join(root, name + '.json'); writeFileSync(path, JSON.stringify(specification) + '\n', { flag: 'wx' });
  const result = spawnSync(process.execPath, ['--permission', ...[oracle, module, path].map(file => `--allow-fs-read=${file}`), oracle, path],
    { cwd: root, env, windowsHide: true, encoding: 'utf8', timeout: 3000, maxBuffer: 4096 });
  assert.equal(result.error, undefined); assert.equal(result.signal, null); assert.equal(result.stderr, '');
  assert.equal(result.status, expectedPass ? 0 : 1);
  const report = JSON.parse(result.stdout); assert.equal(report.passed, expectedPass); assert.equal(report.checks, 6);
  assert.equal(report.failures.length, expectedPass ? 0 : 6); checks += 7;
}
run('valid', true);
specification.steps[1].argument.kind = 'unknown'; run('invalid-expression', false);
specification.steps[1].argument.kind = 'object'; specification.modules[0].exportName = 'absent';
const missing = join(root, 'missing.json'); writeFileSync(missing, JSON.stringify(specification) + '\n', { flag: 'wx' });
const absent = spawnSync(process.execPath, ['--permission', ...[oracle, module, missing].map(file => `--allow-fs-read=${file}`), oracle, missing],
  { cwd: root, env, windowsHide: true, encoding: 'utf8', timeout: 3000, maxBuffer: 4096 });
assert.equal(absent.error, undefined); assert.equal(absent.status, 1);
assert.deepEqual(JSON.parse(absent.stdout), { passed: false, checks: 0, failures: ['INTEGRATION_LOAD_FAILED'] }); checks += 3;
console.log(JSON.stringify({ suite: 'development-integration', checks, publicChildProcesses: 3,
  actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
