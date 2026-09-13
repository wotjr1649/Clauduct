import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, readdirSync, lstatSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID, createHash } from 'node:crypto';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';
import { createNativeOutputCapture, nativeOutputCompleted } from './native-output.mjs';
import { createBackgroundTaskBindings } from './background-task-id.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const need = (ok, code) => { if (!ok) throw new Error(code); };
const write = (path, value) => writeFileSync(path, JSON.stringify(value) + '\n', { flag: 'wx', flush: true });
const read = (path, maximum = 65536) => {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= maximum, 'TASK_ARTIFACT_BOUNDARY');
  return readFileSync(path, 'utf8');
};

function toolEvidence(root, manifest) {
  const projects = join(root, 'config', 'projects'), paths = [];
  for (const item of readdirSync(projects, { withFileTypes: true })) if (item.isDirectory()) {
    const path = join(projects, item.name, manifest.sessionId + '.jsonl');
    try { const stat = lstatSync(path); if (stat.isFile() && !stat.isSymbolicLink()) paths.push(path); } catch { }
  }
  need(paths.length === 1, 'TASK_TRANSCRIPT_UNVERIFIED');
  const calls = [], results = [], bindings = createBackgroundTaskBindings();
  for (const line of read(paths[0], 2097152).trim().split('\n')) {
    need(line.length <= 1048576, 'TASK_TRANSCRIPT_UNVERIFIED');
    const row = JSON.parse(line);
    for (const block of Array.isArray(row.message?.content) ? row.message.content : []) {
      if (row.type === 'assistant' && block.type === 'tool_use') calls.push(block);
      if (row.type === 'user' && block.type === 'tool_result') results.push(block);
    }
  }
  need(calls.length === 4 && results.length === 4, 'TASK_TRANSCRIPT_UNVERIFIED');
  const ids = {};
  for (const [index, name] of ['alpha', 'beta'].entries()) {
    const call = calls[index], result = results.find(item => item.tool_use_id === call.id);
    const command = `node --permission --allow-fs-read=worker.mjs --allow-fs-write=${name}-events.jsonl worker.mjs ${name} ${manifest.cancelled}`;
    need(call.name === 'Bash' && call.input.command === command && call.input.run_in_background === true, 'TASK_COMMAND_UNVERIFIED');
    const content = typeof result?.content === 'string' ? [{ type: 'text', text: result.content }] : result?.content;
    need(Array.isArray(content), 'TASK_TRANSCRIPT_UNVERIFIED');
    ids[name] = bindings.observe({ input: [{ type: 'function_call_output', call_id: call.id,
      output: content.map(block => ({ type: block.type === 'text' ? 'input_text' : 'other', text: block.text })) }] }, call.id);
  }
  const cancelled = manifest.cancelled, survivor = cancelled === 'alpha' ? 'beta' : 'alpha';
  const targetMatched = calls[2].name === 'TaskStop' && Object.keys(calls[2].input).length === 1
    && calls[2].input.task_id === ids[cancelled] && calls[3].name === 'TaskOutput'
    && Object.keys(calls[3].input).length === 3 && calls[3].input.task_id === ids[survivor]
    && calls[3].input.block === true && calls[3].input.timeout === 7000;
  return { calls: calls.length, results: results.length, toolErrors: results.filter(item => item.is_error === true).length,
    distinctTasks: bindings.count(), targetMatched, commandsMatched: true };
}

export async function verifyNativeTaskIsolation({ model, cancelled, powershell }) {
  need(['luna', 'sol'].includes(model) && ['alpha', 'beta'].includes(cancelled)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'), 'TASK_ARGUMENTS');
  const root = mkdtempSync(join(project, '.tmp', 'native-task-isolation-'));
  for (const name of ['work', 'config', 'temp']) mkdirSync(join(root, name));
  const names = ['verification/native-task-entry.mjs', 'verification/verify-native-task-isolation.mjs',
    'verification/fixtures/native-task-worker.mjs', 'verification/background-task-id.mjs',
    'verification/native-output.mjs', 'verification/stop-owned-native-tree.ps1', 'verification/verify-native-recovery.mjs',
    'src/clauduct.mjs', 'src/native-transport.mjs', 'src/native-protocol.mjs', 'src/native-gateway.mjs'];
  const sourceHashes = Object.fromEntries(names.map(name => [name, hash(join(project, name))]));
  const manifest = { model, effort: model === 'luna' ? 'max' : 'low', cancelled, sessionId: randomUUID(),
    nativeMs: 30000, outputBytes: 1048576, requestLimit: 8, maxTurns: 8, treeMs: 12000,
    workers: 2, targetMs: 15000, survivorMs: 5000, sourceHashes, actualModelRequests: 0, credentialReads: 0 };
  write(join(root, 'manifest.json'), manifest);
  copyFileSync(join(project, 'verification', 'fixtures', 'native-task-worker.mjs'), join(root, 'work', 'worker.mjs'));
  const env = nativeVerificationEnvironment(root), started = Date.now();
  const child = spawn(process.execPath, [join(project, 'verification', 'native-task-entry.mjs'), root],
    { cwd: join(root, 'work'), env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  const capture = createNativeOutputCapture({ maxBytes: manifest.outputBytes, maxLineBytes: manifest.outputBytes, maxRecords: 256 });
  capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
  let finished = false, exitCode = null, failure = null, treeEvidence = null, stopAttempted = false;
  const stopped = new Promise(done => {
    child.once('error', () => { failure = 'TASK_START_FAILED'; });
    child.once('close', code => { finished = true; exitCode = code; done(); });
  });
  function tree(stop) {
    if (stop) { need(!stopAttempted, 'TASK_TREE_UNVERIFIED'); stopAttempted = true; }
    const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', join(project, 'verification', 'stop-owned-native-tree.ps1'),
      '-RootPid', String(child.pid), '-StartedAfterMs', String(started), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
    { cwd: project, env: Object.fromEntries(['SystemRoot', 'WINDIR', 'ProgramFiles', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]])),
      windowsHide: true, timeout: manifest.treeMs, maxBuffer: 32768, encoding: 'utf8' });
    need(!result.error && result.status === 0, 'TASK_TREE_UNVERIFIED'); return JSON.parse(result.stdout);
  }
  try {
    while (!finished) {
      if (capture.evidence().failure || Date.now() - started >= manifest.nativeMs) {
        failure = capture.evidence().failure ?? 'TASK_TIMEOUT'; treeEvidence = tree(true); break;
      }
      await new Promise(done => setTimeout(done, 25));
    }
    let timer;
    try { await Promise.race([stopped, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('TASK_STOP_UNVERIFIED')), 5000); })]); }
    finally { clearTimeout(timer); }
    treeEvidence ??= tree(false);
    need(treeEvidence.stopped, 'TASK_TREE_UNVERIFIED');
  } catch { failure ??= 'TASK_CONTROL_FAILED'; }
  let effects = null, tools = null;
  try { effects = JSON.parse(read(join(root, 'task-evidence.json'))); tools = toolEvidence(root, manifest); }
  catch { failure ??= 'TASK_EVIDENCE_UNVERIFIED'; }
  const survivor = cancelled === 'alpha' ? 'beta' : 'alpha';
  const oraclePassed = effects?.failure === null && effects.routeMatched && effects.requests === 5
    && effects.stopIssuedAt < effects.targetStoppedAt && effects.survivorAliveAfterStop
    && effects.workers[cancelled]?.finished === false && effects.workers[cancelled].alive === false
    && effects.workers[survivor]?.finished === true && effects.workers[survivor].alive === false
    && effects.workers[survivor].finishedAt > effects.targetStoppedAt
    && tools?.targetMatched && tools.distinctTasks === 2 && tools.toolErrors === 0;
  const sourceUnchanged = names.every(name => hash(join(project, name)) === sourceHashes[name]);
  const output = capture.snapshot();
  const passed = !failure && treeEvidence?.stopped && sourceUnchanged && nativeOutputCompleted(output,
    { exitCode, sessionId: manifest.sessionId, resultText: 'PUBLIC_TASK_ISOLATION_COMPLETE', oraclePassed });
  const result = { suite: 'native-task-isolation-local', root, model, cancelled, passed: passed === true,
    failure, exitCode, elapsedMs: Date.now() - started, sourceUnchanged, effects, tools,
    output: output.evidence, tree: treeEvidence, actualModelRequests: 0, credentialReads: 0 };
  write(join(root, 'result.json'), result); return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, model, cancelled, powershell] = process.argv.slice(2);
    need(mode === '--local-native' && process.argv.length === 6, 'TASK_ARGUMENTS');
    const result = await verifyNativeTaskIsolation({ model, cancelled, powershell });
    console.log(JSON.stringify({ suite: result.suite, root: result.root, model, cancelled, passed: result.passed,
      failure: result.failure, elapsedMs: result.elapsedMs, actualModelRequests: 0, credentialReads: 0 }));
    process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-task-isolation-local', passed: false, failure: 'TASK_VERIFIER_FAILED' })); process.exitCode = 1; }
}
