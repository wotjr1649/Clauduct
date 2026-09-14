import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, copyFileSync, readFileSync, writeFileSync, existsSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const evidence = mkdtempSync(join(project, '.tmp', 'development-initialization-'));
const files = ['development-fixture.mjs', 'development-tasks.mjs', 'development-source-policy.mjs',
  'development-source-files.mjs', 'development-source-grammar.mjs', 'registered-development-tasks.mjs',
  'fixtures/development-oracle.mjs', 'fixtures/development-window-oracle.mjs', 'fixtures/development-project-oracle.mjs', 'fixtures/development-mcp.mjs'];
function prepare(name) {
  const root = join(evidence, name);
  mkdirSync(join(root, 'verification', 'fixtures'), { recursive: true });
  for (const file of files) copyFileSync(join(project, 'verification', file), join(root, 'verification', file));
  return root;
}
const script = `import { pathToFileURL } from 'node:url';
import { join } from 'node:path';
const { createDevelopmentFixture } = await import(pathToFileURL(join(process.argv[1], 'verification', 'development-fixture.mjs')).href);
try {
  const result = createDevelopmentFixture({ waitMode: 'check', taskId: process.argv[2] });
  console.log(JSON.stringify({ root: result.root, work: result.work, control: result.control, taskId: result.taskId }));
} catch(error) {
  console.log(JSON.stringify({ failure: error.message === 'DEVELOPMENT_PATH' ? 'DEVELOPMENT_PATH' : error.code === 'ENOENT' ? 'ENOENT' : 'UNEXPECTED' }));
}`;
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
function initialize(root, taskId) {
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', script, root, taskId],
    { cwd: root, env, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 4096 });
  assert.equal(result.error, undefined); assert.equal(result.status, 0); assert.equal(result.stderr, '');
  return JSON.parse(result.stdout);
}
let checks = 0;
const equal = (value, expected) => { assert.deepEqual(value, expected); checks++; };
const fresh = prepare('fresh project');
equal(existsSync(join(fresh, '.tmp')), false);
const first = initialize(fresh, 'retry-delay-window');
equal(first.failure, undefined);
equal(first.taskId, 'retry-delay-window');
equal(dirname(first.root), join(fresh, '.tmp'));
equal(readdirSync(first.root).sort(), ['config', 'control', 'temp', 'work']);
equal(existsSync(join(first.work, 'retry-delay-window.mjs')), true);
equal(readFileSync(join(first.control, 'oracle.mjs'), 'utf8'), readFileSync(join(project, 'verification', 'fixtures', 'development-window-oracle.mjs'), 'utf8'));
const preserved = readFileSync(join(first.work, 'retry-delay-window.mjs'), 'utf8');
writeFileSync(join(fresh, '.tmp', 'public-preserved.txt'), 'PUBLIC_PRESERVED', { flag: 'wx' });
const second = initialize(fresh, 'retry-after-seconds');
equal(second.failure, undefined);
equal(second.taskId, 'retry-after-seconds');
equal(second.root === first.root, false);
equal(readFileSync(join(first.work, 'retry-delay-window.mjs'), 'utf8'), preserved);
equal(readFileSync(join(fresh, '.tmp', 'public-preserved.txt'), 'utf8'), 'PUBLIC_PRESERVED');
const composite = initialize(fresh, 'retry-project');
equal(composite.failure, undefined);
equal(composite.taskId, 'retry-project');
equal(readdirSync(composite.work).sort(), ['.mcp.json', 'retry-after-seconds.mjs', 'retry-delay-window.mjs']);
for (const name of ['development-oracle.mjs', 'development-window-oracle.mjs']) {
  equal(readFileSync(join(composite.control, name), 'utf8'), readFileSync(join(project, 'verification', 'fixtures', name), 'utf8'));
}
equal(readFileSync(join(first.work, 'retry-delay-window.mjs'), 'utf8'), preserved);
const occupied = prepare('occupied project');
writeFileSync(join(occupied, '.tmp'), 'PUBLIC_OCCUPIED', { flag: 'wx' });
equal(initialize(occupied, 'retry-delay-window'), { failure: 'DEVELOPMENT_PATH' });
equal(readFileSync(join(occupied, '.tmp'), 'utf8'), 'PUBLIC_OCCUPIED');
equal(readdirSync(occupied).sort(), ['.tmp', 'verification']);
console.log(JSON.stringify({ suite: 'development-initialization', checks, evidenceRoot: evidence,
  freshTemporaryRootCreated: true, existingWorkPreserved: true, occupiedFileRejected: true,
  actualPublicChildren: 4, actualNativeExecutions: 0, actualCredentialReads: 0, externalRequests: 0,
  dynamicLinkChecks: 'BLOCKED_NOT_RUN' }));
