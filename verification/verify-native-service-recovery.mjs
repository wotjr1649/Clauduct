import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, lstatSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { initializeRecovery, runRecovery, recordRecoveryWorker, recoveryOracle } from './unattended-recovery.mjs';
import { SERVICE_FAULTS, SERVICE_MARKER } from './fixtures/service-faults.mjs';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const entry = join(project, 'verification', 'native-service-entry.mjs');
const mcp = join(project, 'verification', 'fixtures', 'native-recovery-mcp.mjs');
const helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
const need = (ok, label) => { if (!ok) throw new Error(label); };
const write = (path, value) => writeFileSync(path, JSON.stringify(value) + '\n', { flag: 'wx', flush: true });
const read = (path, maximum = 65536) => {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= maximum, 'SERVICE_EVIDENCE_BOUNDARY');
  return readFileSync(path, 'utf8');
};
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const sleep = ms => new Promise(done => setTimeout(done, ms));

export async function verifyNativeServiceRecovery({ kind, model, powershell }) {
  need(SERVICE_FAULTS.includes(kind) && ['luna', 'sol'].includes(model)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'), 'SERVICE_ARGUMENTS');
  const root = mkdtempSync(join(project, '.tmp', 'native-service-recovery-'));
  const manifest = initializeRecovery(root, { taskId: 'public-service-task' });
  const work = join(root, 'work');
  for (const name of ['config', 'temp']) mkdirSync(join(root, name));
  const sourceNames = ['verification/native-service-entry.mjs', 'verification/verify-native-service-recovery.mjs',
    'verification/fixtures/service-faults.mjs', 'verification/fixtures/native-recovery-mcp.mjs',
    'verification/unattended-recovery.mjs', 'verification/stop-owned-native-tree.ps1',
    'src/native-transport.mjs', 'src/native-protocol.mjs', 'src/native-gateway.mjs', 'src/clauduct.mjs'];
  const sourceHashes = Object.fromEntries(sourceNames.map(name => [name, hash(join(project, name))]));
  write(join(root, 'budget.json'), { kind, model, effort: model === 'luna' ? 'max' : 'low', phaseMs: 30000,
    maxPhases: 2, requestLimit: 16, maxTurns: 8, outputBytes: 1048576, treeCheckMs: 12000,
    actualModelRequests: 0, credentialReads: 0, sourceHashes,
    basis: 'Earlier local native Workflow/MCP probes completed within 3 seconds. The default five-retry loop adds at most 3.1 seconds per request; 30 seconds bounds each phase.' });
  write(join(work, '.mcp.json'), { mcpServers: { fixture: { type: 'stdio', command: process.execPath,
    args: ['--permission', `--allow-fs-read=${mcp}`, `--allow-fs-read=${work}`, `--allow-fs-write=${work}`,
      mcp, work, manifest.operationId, 'return', 'audit'],
    env: { ANTHROPIC_AUTH_TOKEN: '', ANTHROPIC_BASE_URL: '', ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '' } } } });
  const env = nativeVerificationEnvironment(root);
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'ProgramFiles', 'TEMP', 'TMP']
    .filter(name => env[name]).map(name => [name, env[name]]));
  const phases = [], started = Date.now();
  const execute = async (stateRoot, state, records, phase) => {
    need(phases.length < 2, 'SERVICE_PHASE_LIMIT');
    const phaseStart = Date.now();
    const child = fork(entry, [root, phase, kind, model], { cwd: work, env, execArgv: [],
      windowsHide: true, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    let stdout = '', stderr = '', outputBytes = 0, finished = false, exitCode = null, failure = null;
    const stopped = new Promise(done => {
      child.once('error', () => { failure = 'SERVICE_PROCESS_START_FAILED'; });
      child.once('close', code => { finished = true; exitCode = code; done(); });
    });
    child.stdout.on('data', chunk => { outputBytes += chunk.length; if (outputBytes <= 1048576) stdout += chunk.toString(); });
    child.stderr.on('data', chunk => { outputBytes += chunk.length; if (outputBytes <= 1048576) stderr += chunk.toString(); });
    let stopAttempted = false;
    const tree = stop => {
      if (stop) { need(!stopAttempted, 'SERVICE_TREE_UNVERIFIED'); stopAttempted = true; }
      const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(child.pid),
        '-StartedAfterMs', String(phaseStart), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
      { cwd: project, env: controlEnv, windowsHide: true, timeout: 12000, maxBuffer: 32768, encoding: 'utf8' });
      need(!result.error && result.status === 0, 'SERVICE_TREE_UNVERIFIED');
      const value = JSON.parse(result.stdout); need(value.stopped === true, 'SERVICE_TREE_UNVERIFIED'); return value;
    };
    const waitStopped = async () => {
      if (finished) return;
      let deadline;
      try {
        await Promise.race([stopped, new Promise((_, reject) => {
          deadline = setTimeout(() => reject(new Error('SERVICE_STOP_UNVERIFIED')), 5000);
        })]);
      } finally { clearTimeout(deadline); }
    };
    let treeEvidence;
    try {
      recordRecoveryWorker(stateRoot, records, child.pid, phase);
      child.send({ start: true });
      while (!finished && outputBytes <= 1048576 && Date.now() - phaseStart < 30000) await sleep(25);
      if (!finished) {
        failure = outputBytes > 1048576 ? 'SERVICE_OUTPUT_LIMIT' : 'SERVICE_PHASE_TIMEOUT';
        treeEvidence = tree(true);
      }
      await waitStopped();
      treeEvidence ??= tree(false);
    } finally {
      if (!finished && !stopAttempted) {
        // No resume is attempted unless the original process and descendants are proven stopped.
        treeEvidence = tree(true);
        await waitStopped();
      }
    }
    let result, status, service;
    try { result = JSON.parse(stdout); } catch { failure ??= 'SERVICE_NATIVE_JSON_MISSING'; }
    const markers = stderr.split(/\r?\n/).filter(line => line.startsWith('CLAUDUCT_REQUEST_STATUS '));
    try { need(markers.length === 1, 'STATUS'); status = JSON.parse(markers[0].slice(24)); }
    catch { failure ??= 'SERVICE_STATUS_MISSING'; }
    try { service = JSON.parse(read(join(root, `service-${phase}.json`))); }
    catch { failure ??= 'SERVICE_WIRE_EVIDENCE_MISSING'; }
    const cleanup = status?.cleanup && Object.keys(status.cleanup).length === 9
      && Object.values(status.cleanup).every(value => value === true);
    const completed = !failure && exitCode === 0 && result?.is_error === false && result.result === SERVICE_MARKER
      && result.session_id === state.operationId && status?.requestOutcome === 'all-succeeded' && cleanup === true;
    const audit = read(join(work, 'mcp-events.jsonl'), 16384).trim().split('\n').map(line => JSON.parse(line));
    need(audit.every(row => Object.keys(row).length === 1 && ['apply_effect', 'effect_status', 'complete_report'].includes(row.name)), 'SERVICE_AUDIT_INVALID');
    const calls = Object.fromEntries(['apply_effect', 'effect_status', 'complete_report'].map(name => [name, audit.filter(row => row.name === name).length]));
    const summary = { phase, elapsedMs: Date.now() - phaseStart, exitCode, failure, completed: completed === true,
      nativeError: result?.is_error ?? null, sameSession: result?.session_id === state.operationId,
      markerMatched: result?.result === SERVICE_MARKER, requestOutcome: status?.requestOutcome ?? null,
      cleanupComplete: cleanup === true, outputBytes, calls, service, status, tree: treeEvidence };
    phases.push(summary); write(join(root, `result-${phase}.json`), summary);
    need(!failure && cleanup && treeEvidence.stopped && service?.failure === null
      && service.routeMatched && service.serverClosed && service.socketsRemaining === 0, 'SERVICE_PHASE_FAILED');
    if (phase === 'effect') {
      need(!completed && exitCode !== 0 && result.is_error === true && status.requestOutcome === 'has-failures'
        && service.faultAfterReceipt && calls.apply_effect === 1 && calls.complete_report === 0
        && calls.effect_status === 0 && !recoveryOracle(root), 'SERVICE_FAILURE_NOT_PRESERVED');
      const failures = status.recentRequests.filter(row => row.success === false);
      need(failures.length === 1 && status.lifetime.failed === 1, 'SERVICE_RETRY_AMPLIFICATION');
      const expected = kind === 'flapping-503' ? 'UPSTREAM_HTTP_ERROR' : kind === 'error-200' ? 'UPSTREAM_ERROR_EVENT'
        : kind === 'truncated' ? 'TRUNCATED_STREAM' : kind === 'invalid-utf8' ? 'INVALID_UTF8' : 'SEQUENCE_MISMATCH';
      need(failures[0].failureCategory === expected && failures[0].attempts.length === (kind === 'flapping-503' ? 6 : 1), 'SERVICE_WRONG_FAILURE');
      need(kind === 'flapping-503' ? service.serviceFailures === 8 && service.partialFailures === 0
        : service.serviceFailures === 0 && service.partialFailures === 1 && failures[0].firstTextDeltaMs !== null, 'SERVICE_WRONG_STIMULUS');
    } else need(completed && calls.apply_effect === 1 && calls.effect_status === 1 && calls.complete_report === 1, 'SERVICE_RESUME_FAILED');
    return { completed: completed === true, exitCode };
  };
  let first = null, final = null, error = null;
  try {
    first = await runRecovery(root, { execute });
    need(first.state === 'RECOVERING' && !first.taskCompleted, 'SERVICE_BASELINE_FAILED');
    final = await runRecovery(root, { execute });
  } catch (caught) {
    const labels = ['SERVICE_PHASE_FAILED', 'SERVICE_FAILURE_NOT_PRESERVED', 'SERVICE_RETRY_AMPLIFICATION',
      'SERVICE_WRONG_FAILURE', 'SERVICE_WRONG_STIMULUS', 'SERVICE_RESUME_FAILED', 'SERVICE_BASELINE_FAILED',
      'SERVICE_AUDIT_INVALID', 'SERVICE_TREE_UNVERIFIED', 'SERVICE_STOP_UNVERIFIED', 'SERVICE_EVIDENCE_BOUNDARY'];
    error = labels.includes(caught.message) ? caught.message : 'SERVICE_VERIFICATION_FAILED';
  }
  const sourceUnchanged = sourceNames.every(name => hash(join(project, name)) === sourceHashes[name]);
  const passed = !error && final?.taskCompleted === true && recoveryOracle(root) && phases.length === 2 && sourceUnchanged;
  const result = { suite: 'native-service-recovery-local', root, kind, model, passed, error, first, final,
    elapsedMs: Date.now() - started, sourceUnchanged, actualModelRequests: 0, credentialReads: 0, phases };
  write(join(root, 'result.json'), result); return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [flag, kind, model, powershell] = process.argv.slice(2);
    need(flag === '--local-native' && process.argv.length === 6, 'SERVICE_ARGUMENTS');
    const result = await verifyNativeServiceRecovery({ kind, model, powershell });
    console.log(JSON.stringify({ suite: result.suite, root: result.root, kind, model, passed: result.passed,
      error: result.error, elapsedMs: result.elapsedMs, phases: result.phases.length, actualModelRequests: 0, credentialReads: 0 }));
    process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-service-recovery-local', passed: false, error: 'SERVICE_VERIFIER_FAILED' })); process.exitCode = 1; }
}
