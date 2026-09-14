import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, lstatSync, existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';
import { createNativeOutputCapture, nativeOutputCompleted, nativeOutputDiagnostics } from './native-output.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const need = (ok, code) => { if (!ok) throw new Error(code); };
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const write = (path, value) => writeFileSync(path, JSON.stringify(value) + '\n', { flag: 'wx', flush: true });
const read = path => {
  const info = lstatSync(path); need(info.isFile() && !info.isSymbolicLink() && info.size <= 16384, 'PERMISSION_EVIDENCE');
  return JSON.parse(readFileSync(path, 'utf8'));
};

export async function verifyNativePermissions({ model, powershell }) {
  need(['luna', 'sol'].includes(model) && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'), 'PERMISSION_ARGUMENTS');
  const temporary = join(project, '.tmp');
  if (!existsSync(temporary)) mkdirSync(temporary);
  need(lstatSync(temporary).isDirectory() && !lstatSync(temporary).isSymbolicLink(), 'PERMISSION_ROOT');
  const root = mkdtempSync(join(temporary, 'native-permission-'));
  for (const directory of ['work', 'config', 'temp']) mkdirSync(join(root, directory));
  const files = ['verification/verify-native-permissions.mjs', 'verification/native-permission-entry.mjs',
    'verification/fixtures/permission-worker.mjs', 'verification/native-output.mjs', 'verification/verify-native-recovery.mjs',
    'verification/stop-owned-native-tree.ps1', 'src/clauduct.mjs', 'src/native-gateway.mjs', 'src/native-protocol.mjs',
    'src/native-transport.mjs', 'src/rate-limit-observation.mjs', 'src/models.mjs', 'src/request-status.mjs'];
  const sourceHashes = Object.fromEntries(files.map(path => [path, hash(join(project, path))]));
  const manifest = { model, effort: model === 'luna' ? 'max' : 'low', sessionId: randomUUID(), nativeMs: 30000,
    outputBytes: 1048576, requestLimit: 3, maxTurns: 4, sourceHashes, actualModelRequests: 0, credentialReads: 0 };
  write(join(root, 'manifest.json'), manifest);
  copyFileSync(join(project, 'verification/fixtures/permission-worker.mjs'), join(root, 'work/permission-worker.mjs'));
  const env = nativeVerificationEnvironment(root), started = Date.now();
  const child = spawn(process.execPath, [join(project, 'verification/native-permission-entry.mjs'), root],
    { cwd: join(root, 'work'), env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  const capture = createNativeOutputCapture({ maxBytes: manifest.outputBytes, maxLineBytes: manifest.outputBytes, maxRecords: 256 });
  capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
  let finished = false, exitCode = null, failure = null, treeEvidence = null, stopAttempted = false;
  const stopped = new Promise(done => {
    child.once('error', () => { failure = 'PERMISSION_START_FAILED'; });
    child.once('close', code => { finished = true; exitCode = code; done(); });
  });
  write(join(root, 'process.json'), { pid: child.pid, startedAt: started });
  console.log(JSON.stringify({ event: 'PERMISSION_STARTED', root, model }));
  function tree(stop) {
    if (stop) { need(!stopAttempted, 'PERMISSION_TREE_UNVERIFIED'); stopAttempted = true; }
    const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', join(project, 'verification/stop-owned-native-tree.ps1'),
      '-RootPid', String(child.pid), '-StartedAfterMs', String(started), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
    { cwd: project, env: Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]])),
      windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!result.error && result.status === 0, 'PERMISSION_TREE_UNVERIFIED'); return JSON.parse(result.stdout);
  }
  try {
    while (!finished) {
      if (capture.evidence().failure || Date.now() - started >= manifest.nativeMs) {
        failure = capture.evidence().failure ?? 'PERMISSION_TIMEOUT'; treeEvidence = tree(true); break;
      }
      await new Promise(done => setTimeout(done, 25));
    }
    let timer;
    try { await Promise.race([stopped, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('PERMISSION_STOP_UNVERIFIED')), 5000); })]); }
    finally { clearTimeout(timer); }
    treeEvidence ??= tree(false); need(treeEvidence.stopped, 'PERMISSION_TREE_UNVERIFIED');
  } catch { failure ??= 'PERMISSION_CONTROL_FAILED'; }
  let effects = null, allowed = null, workerStart = null;
  try {
    effects = read(join(root, 'permission-evidence.json')); allowed = read(join(root, 'work/allowed-effect.json'));
    workerStart = read(join(root, 'work/worker-started.jsonl'));
  }
  catch { failure ??= 'PERMISSION_EVIDENCE'; }
  const deniedEffectAbsent = !existsSync(join(root, 'work/denied-effect.json'));
  const workerUnchanged = hash(join(root, 'work/permission-worker.mjs')) === sourceHashes['verification/fixtures/permission-worker.mjs'];
  const sourceUnchanged = files.every(path => hash(join(project, path)) === sourceHashes[path]);
  // A denied worker could start and then fail before writing its effect. The
  // separate first action must contain exactly the allowed worker's one entry.
  const workerStartMatched = workerStart?.label === 'allowed' && workerStart.event === 'started' && Object.keys(workerStart).length === 2;
  const oraclePassed = effects?.failure === null && effects.requests === 3 && effects.routeMatched === true
    && effects.allowedResultObserved === true && effects.deniedResultObserved === true
    && effects.allowedEffectObserved === true && effects.deniedEffectAbsent === true
    && allowed?.label === 'allowed' && allowed.event === 'performed' && Object.keys(allowed).length === 2
    && deniedEffectAbsent && workerUnchanged && workerStartMatched;
  const output = capture.snapshot();
  const passed = !failure && sourceUnchanged && treeEvidence?.stopped && nativeOutputCompleted(output,
    { exitCode, sessionId: manifest.sessionId, resultText: 'PUBLIC_PERMISSION_DENIAL_VERIFIED', oraclePassed });
  const result = { suite: 'native-permission-local', root, model, effort: manifest.effort, passed: passed === true,
    failure, exitCode, elapsedMs: Date.now() - started, sourceUnchanged, workerUnchanged, workerStartMatched, deniedEffectAbsent,
    effects, output: output.evidence, requestDiagnostics: nativeOutputDiagnostics(output), tree: treeEvidence,
    actualModelRequests: 0, credentialReads: 0, longStageCounts: 0 };
  write(join(root, 'result.json'), result); return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, model, powershell] = process.argv.slice(2);
    need(mode === '--local-native' && process.argv.length === 5, 'PERMISSION_ARGUMENTS');
    const result = await verifyNativePermissions({ model, powershell });
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-permission-local', passed: false, failure: 'PERMISSION_VERIFIER_FAILED' })); process.exitCode = 1; }
}
