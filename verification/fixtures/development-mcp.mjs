import { createInterface } from 'node:readline';
import { readFileSync, writeFileSync, appendFileSync, existsSync, lstatSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { checkDevelopmentSource, developmentSourceFromArguments } from '../development-source-policy.mjs';
import { developmentTask } from '../development-tasks.mjs';
import { readDevelopmentSource, writeDevelopmentSource, writeDevelopmentSourceWithRecovery, developmentOracleInvocation } from '../development-source-files.mjs';

const [directory, waitMode, taskId, holdMode] = process.argv.slice(2), root = resolve(directory);
if (!['wait', 'check'].includes(waitMode) || holdMode !== undefined && !['hold-after-pass', 'hold-after-source', 'hold-after-read', 'recover-after-first-file'].includes(holdMode)
  || typeof taskId !== 'string' || process.argv.length > 6) throw new Error('DEVELOPMENT_MODE');
const task = developmentTask(taskId);
if (holdMode === 'recover-after-first-file' && !task.parts) throw new Error('DEVELOPMENT_MODE');
const work = join(root, 'work'), control = join(root, 'control');
for (const path of [work, control]) {
  const stat = lstatSync(path); if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('DEVELOPMENT_PATH');
}
const readSource = () => readDevelopmentSource(work, taskId);
const digest = text => createHash('sha256').update(text).digest('hex');
function read(path, limit = 16384) {
  const stat = lstatSync(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > limit) throw new Error('DEVELOPMENT_PATH');
  return readFileSync(path, 'utf8');
}
function record(row) {
  const path = join(work, 'events.jsonl');
  if (existsSync(path) && lstatSync(path).size > 32768) throw new Error('DEVELOPMENT_LIMIT');
  appendFileSync(path, JSON.stringify(row) + '\n');
}
const tools = [
  { name: 'read_task', description: 'Read the complete reviewed TASK.md and the current source for this bounded development task.', properties: {} },
  { name: 'write_source', description: task.parts ? 'Replace the complete explicit file set in the declared order. Every source must pass its language check before any file is written.'
    : `Replace only ${task.sourceFile} with the proposed JavaScript source. No other file can be written.`,
  properties: task.parts ? { files: { type: 'array', minItems: task.parts.length, maxItems: task.parts.length,
    items: { type: 'object', properties: { path: { type: 'string', enum: task.parts.map(part => part.path) }, code: { type: 'string', maxLength: 8192 } },
      required: ['path', 'code'], additionalProperties: false } } } : { code: { type: 'string', maxLength: 8192 } } },
  { name: 'run_tests', description: `Run the fixed independent ${task.checks}-case Node test suite. A proposed source is reviewed by the outer developer before execution; this call waits for that review.`, properties: {} }
];
const textResult = value => ({ content: [{ type: 'text', text: JSON.stringify(value) }] });
async function respond(message) {
  if (!Object.hasOwn(message, 'id') || !['number', 'string'].includes(typeof message.id)) return;
  const reply = { jsonrpc: '2.0', id: message.id };
  if (message.method === 'initialize' && /^20\d\d-\d\d-\d\d$/.test(message.params?.protocolVersion)) return { ...reply, result: {
    protocolVersion: message.params.protocolVersion, capabilities: { tools: {} }, serverInfo: { name: 'clauduct-development-fixture', version: '1.0.0' } } };
  if (message.method === 'ping') return { ...reply, result: {} };
  if (message.method === 'tools/list') return { ...reply, result: { tools: tools.map(tool => ({ name: tool.name, description: tool.description,
    inputSchema: { type: 'object', properties: tool.properties, required: Object.keys(tool.properties), additionalProperties: false } })) } };
  const args = message.params?.arguments ?? {}, name = message.params?.name;
  if (message.method !== 'tools/call' || !tools.some(tool => tool.name === name) || !args || typeof args !== 'object' || Array.isArray(args)
    || Object.keys(args).sort().join(',') !== (name === 'write_source' ? task.parts ? 'files' : 'code' : '')) return { ...reply, error: { code: -32602, message: 'INVALID_DEVELOPMENT_REQUEST' } };
  if (name === 'read_task') {
    const currentSource = readSource(); checkDevelopmentSource(currentSource, taskId);
    record({ event: 'TASK_READ' });
    if (holdMode === 'hold-after-read') {
      record({ event: 'TASK_READ_WAIT' });
      await new Promise(done => setTimeout(done, 5000));
    }
    return { ...reply, result: textResult({ task: read(join(control, 'TASK.md')),
      ...(task.parts ? { files: JSON.parse(currentSource).files } : { source: currentSource }) }) };
  }
  if (name === 'write_source') {
    if (existsSync(join(control, 'source-sealed.json'))) {
      const seal = JSON.parse(read(join(control, 'source-sealed.json'), 1024));
      if (!seal || Object.keys(seal).length !== 1 || seal.sha256 !== digest(readSource())) throw new Error('SOURCE_SEAL_INVALID');
      record({ event: 'SOURCE_WRITE_BLOCKED', sha256: seal.sha256 });
      return { ...reply, result: { ...textResult({ written: false, reason: 'DEVELOPMENT_SOURCE_SEALED' }), isError: true } };
    }
    if (!task.parts && (typeof args.code !== 'string' || Buffer.byteLength(args.code) > 8192)) return { ...reply, error: { code: -32602, message: 'INVALID_SOURCE_SIZE' } };
    let proposed;
    try { proposed = developmentSourceFromArguments(args, taskId); }
    catch { return { ...reply, error: { code: -32602, message: 'DEVELOPMENT_SOURCE_REJECTED' } }; }
    if (holdMode === 'recover-after-first-file') {
      writeDevelopmentSourceWithRecovery(work, taskId, proposed, record);
    } else writeDevelopmentSource(work, taskId, proposed, record);
    if (!task.parts) record({ event: 'SOURCE_WRITTEN', sha256: digest(proposed) });
    if (holdMode === 'hold-after-source') {
      record({ event: 'SOURCE_WRITE_WAIT', sha256: digest(proposed) });
      await new Promise(done => setTimeout(done, 5000));
    }
    return { ...reply, result: textResult({ written: true }) };
  }
  const sha256 = digest(readSource());
  writeFileSync(join(work, 'review-request.json'), JSON.stringify({ sha256 }));
  const approval = join(control, 'review.json'), deadline = Date.now() + (waitMode === 'wait' ? 180000 : 0);
  let reviewed = false;
  do {
    if (existsSync(approval)) {
      const decision = JSON.parse(read(approval, 1024));
      if (decision.sha256 === sha256) { reviewed = decision.approved === true; break; }
    }
    if (Date.now() >= deadline) break;
    await new Promise(done => setTimeout(done, 100));
  } while (true);
  if (!reviewed || digest(readSource()) !== sha256) {
    record({ event: 'TESTS_UNRUN', sha256 });
    return { ...reply, result: { ...textResult({ passed: false, testsRun: false, reason: 'SOURCE_REVIEW_REQUIRED' }), isError: true } };
  }
  const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
  const invocation = developmentOracleInvocation(control, work, taskId);
  const result = spawnSync(process.execPath, ['--permission', ...invocation.readPaths.map(path => `--allow-fs-read=${path}`),
    invocation.oracle, invocation.argument], { cwd: work, env, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 16384 });
  let outcome = null;
  try { outcome = JSON.parse(result.stdout); } catch { }
  if (result.error || result.signal || !outcome || outcome.checks !== task.checks || typeof outcome.passed !== 'boolean'
    || !Array.isArray(outcome.failures) || outcome.failures.some(item => typeof item !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(item))) throw new Error('TEST_EXECUTION_FAILED');
  const passed = result.status === 0 && outcome.passed && outcome.failures.length === 0 && result.stderr === '';
  record({ event: 'TESTS_EXECUTED', sha256, passed, checks: outcome.checks, failures: outcome.failures });
  if (passed && holdMode === 'hold-after-pass') {
    // The controller closes native's final-result pipe while this tool is held,
    // then releases this fixed barrier. The model cannot write the control root.
    const release = join(control, 'output-cut-release.json'), stopAt = Date.now() + 5000;
    if (!existsSync(release)) record({ event: 'OUTPUT_CUT_WAIT', sha256 });
    for (;;) {
      if (existsSync(release)) {
        const value = JSON.parse(read(release, 1024));
        if (!value || Object.keys(value).length !== 1 || value.released !== true) throw new Error('OUTPUT_CUT_RELEASE_INVALID');
        break;
      }
      if (Date.now() >= stopAt) throw new Error('OUTPUT_CUT_RELEASE_MISSING');
      await new Promise(done => setTimeout(done, 25));
    }
  }
  return { ...reply, result: textResult({ ...outcome, passed, testsRun: true }) };
}
const lines = createInterface({ input: process.stdin });
try {
  for await (const line of lines) {
    if (Buffer.byteLength(line) > 16384) throw new Error('DEVELOPMENT_INPUT_LIMIT');
    const message = JSON.parse(line);
    if (!message || typeof message !== 'object' || Array.isArray(message)) throw new Error('DEVELOPMENT_MESSAGE');
    const reply = await respond(message); if (reply) process.stdout.write(JSON.stringify(reply) + '\n');
  }
} catch { process.stderr.write('DEVELOPMENT_FIXTURE_FAILED\n'); process.exitCode = 1; }
finally { lines.close(); }
