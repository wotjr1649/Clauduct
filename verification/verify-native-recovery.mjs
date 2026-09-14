import { mkdirSync, writeFileSync, readFileSync, existsSync, lstatSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { initializeRecovery, runRecovery, recordRecoveryWorker, recoveryOracle } from './unattended-recovery.mjs';
import { readVerificationLedger } from './verification-ledger.mjs';
import { createNativeOutputCapture, nativeOutputCompleted } from './native-output.mjs';
import { temporaryDir } from './temporary-dir.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const entry = join(project, 'verification', 'native-recovery-entry.mjs');
const mcp = join(project, 'verification', 'fixtures', 'native-recovery-mcp.mjs');
const helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
const combinations = { luna: 'max', sol: 'low' };
const sleep = ms => new Promise(done => setTimeout(done, ms));
const need = (ok, label) => { if (!ok) throw new Error(label); };
const read = path => {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= 65536, 'EVIDENCE_SHAPE');
  return readFileSync(path, 'utf8');
};
const write = (path, value) => writeFileSync(path, JSON.stringify(value) + '\n', { flag: 'wx', flush: true });
const hash = path => createHash('sha256').update(readFileSync(path)).digest('hex');

export function nativeVerificationEnvironment(root) {
  const env = {};
  for (const key of ['SystemRoot', 'WINDIR', 'SystemDrive', 'ComSpec', 'PATH', 'PATHEXT', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH',
    'APPDATA', 'LOCALAPPDATA', 'ProgramData', 'ProgramFiles', 'ProgramFiles(x86)', 'OS', 'PROCESSOR_ARCHITECTURE', 'CODEX_HOME',
    'NODE_DEBUG', 'NODE_OPTIONS', 'NODE_USE_ENV_PROXY', 'NODE_TLS_REJECT_UNAUTHORIZED']) if (process.env[key]) env[key] = process.env[key];
  Object.assign(env, { CLAUDE_CONFIG_DIR: join(root, 'config'), CLAUDE_CODE_TMPDIR: join(root, 'temp'), TEMP: join(root, 'temp'), TMP: join(root, 'temp'),
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1', CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY: '1' });
  return env;
}

export async function verifyNativeRecovery({ model, powershell, priorAttempts, priorElapsedMs, phaseMs = 120000, requestLimit = 16, failureMode = 'crash' }) {
  need(Object.hasOwn(combinations, model) && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'), 'INVALID_ARGUMENTS');
  need(['crash', 'service-signal'].includes(failureMode), 'INVALID_ARGUMENTS');
  const serviceSignal = failureMode === 'service-signal';
  const selectedEntry = serviceSignal ? join(project, 'verification', 'native-service-live-entry.mjs') : entry;
  need(Number.isSafeInteger(priorAttempts) && priorAttempts >= 0 && Number.isSafeInteger(priorElapsedMs) && priorElapsedMs >= 0, 'INVALID_PRIOR_USAGE');
  need(phaseMs === 120000 && requestLimit === 16, 'INVALID_BUDGET');
  const root = temporaryDir(project, 'native-recovery-');
  const manifest = initializeRecovery(root), work = join(root, 'work'), config = join(root, 'config'), temp = join(root, 'temp');
  mkdirSync(config); mkdirSync(temp);
  const budget = { model, effort: combinations[model], phaseMs, requestLimit, maxPhases: 2, maxTurns: 8, failureMode,
    maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768,
    outputBytes: 1024 * 1024, priorAttempts, priorElapsedMs, cumulativeReservedAttempts: priorAttempts + 2 * requestLimit,
    cumulativeReservedMs: priorElapsedMs + 2 * phaseMs, mainConcurrency: 1,
    basis: 'Observed text runs used one attempt and 2.87-5.12s. Up to eight tool turns plus the existing retry allowance fit 16 attempts; two phases are reserved.',
    hashes: Object.fromEntries([selectedEntry, mcp, helper, ...['verification/verify-native-recovery.mjs',
      'verification/unattended-recovery.mjs', 'verification/fixture-tool-policy.mjs', 'verification/development-source-policy.mjs',
      'verification/development-tasks.mjs', 'verification/verification-ledger.mjs', 'verification/native-output.mjs',
      'verification/manual-http-probe.mjs', 'verification/auth-store-selection.mjs', 'src/clauduct.mjs', 'src/native-transport.mjs', 'src/native-protocol.mjs',
      'src/native-gateway.mjs', 'src/models.mjs', 'src/client-version.mjs', 'poc/user-session.mjs', 'poc/adapter.mjs']
      .map(name => join(project, name))].map(path => [path.slice(project.length + 1), hash(path)])) };
  write(join(root, 'budget.json'), budget);
  write(join(work, '.mcp.json'), { mcpServers: { fixture: { type: 'stdio', command: process.execPath,
    args: ['--permission', `--allow-fs-read=${mcp}`, `--allow-fs-read=${work}`, `--allow-fs-write=${work}`,
      mcp, work, manifest.operationId, ...(serviceSignal ? ['return', 'audit'] : ['hold'])], env: { ANTHROPIC_AUTH_TOKEN: '', ANTHROPIC_BASE_URL: '',
      ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '' } } } });
  const env = nativeVerificationEnvironment(root);
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'ProgramFiles', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]]));
  const phases = [];
  const started = Date.now();
  const execute = async (stateRoot, state, records, phase) => {
    need(phases.length < 2 && Date.now() - started < phaseMs * 2, 'RUN_BUDGET');
    const prompt = serviceSignal ? phase === 'effect'
      ? 'Complete this public verification task in separate tool turns. First call mcp__fixture__apply_effect exactly once with {}. Wait for its result before completing the report. Do not claim completion before the report exists. The MCP tools are already available; do not search for tools.'
      : 'Continue the same interrupted public verification task. First call mcp__fixture__effect_status exactly once with {}. Wait for its result. Then call mcp__fixture__complete_report exactly once with {}. Wait for its success and reply exactly CLAUDUCT_RECOVERY_DONE. Never call apply_effect again. Use one tool per response. The MCP tools are already available.'
      : phase === 'effect'
      ? 'Complete this public verification task: first call mcp__fixture__apply_effect, then call mcp__fixture__complete_report, then reply exactly CLAUDUCT_RECOVERY_DONE. Use ToolSearch to discover these tools if needed. Do not claim completion before the report tool succeeds.'
      : 'Continue the original interrupted public verification task in this same session. The manager independently found its effect receipt. First use mcp__fixture__effect_status to reconcile it, then mcp__fixture__complete_report to finish the original report. Never call apply_effect again. After the report succeeds, reply exactly CLAUDUCT_RECOVERY_DONE.';
    const args = ['--model', model, '--effort', combinations[model], '--verify-model-route', '--verify-request-limit', String(requestLimit),
      '-p', '--output-format', 'json', '--tools', serviceSignal ? '' : 'ToolSearch', '--allowedTools',
      serviceSignal ? phase === 'effect' ? 'mcp__fixture__apply_effect' : 'mcp__fixture__effect_status,mcp__fixture__complete_report'
        : phase === 'effect' ? 'ToolSearch,mcp__fixture__apply_effect,mcp__fixture__complete_report' : 'ToolSearch,mcp__fixture__effect_status,mcp__fixture__complete_report',
      '--max-turns', '8', ...(phase === 'effect' ? ['--session-id', state.operationId] : ['--resume', state.operationId, '--disallowedTools', 'mcp__fixture__apply_effect']), '--', prompt];
    writeFileSync(join(root, `transport-${phase}.jsonl`), '', { flag: 'wx' });
    if (serviceSignal) mkdirSync(join(root, `usage-${phase}`));
    const phaseStart = Date.now();
    const child = fork(selectedEntry, [root, phase, ...args], { cwd: work, env, execArgv: [], windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    const capture = createNativeOutputCapture({ maxBytes: budget.outputBytes, maxLineBytes: budget.outputBytes, maxRecords: 4096 });
    let finished = false, exitCode, failure = null;
    const stopped = new Promise(done => {
      child.once('error', () => { failure = 'PROCESS_START_FAILED'; });
      child.once('close', code => { exitCode = code; finished = true; done(); });
    });
    capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
    recordRecoveryWorker(stateRoot, records, child.pid, phase);
    child.send({ start: true });
    let stopAttempted = false;
    function tree(stop) {
      if (stop) { need(!stopAttempted, 'TREE_STOP_UNVERIFIED'); stopAttempted = true; }
      const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(child.pid),
        '-StartedAfterMs', String(phaseStart), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
      { cwd: project, env: controlEnv, windowsHide: true, timeout: 12000, maxBuffer: 32768, encoding: 'utf8' });
      need(!result.error && result.status === 0, 'TREE_STOP_UNVERIFIED');
      return JSON.parse(result.stdout);
    }
    async function waitStopped() {
      if (finished) return;
      let deadline;
      try {
        await Promise.race([stopped, new Promise((_, reject) => {
          deadline = setTimeout(() => reject(new Error('PROCESS_STOP_UNVERIFIED')), 5000);
        })]);
      } finally { clearTimeout(deadline); }
    }
    let crashInjected = false, treeEvidence;
    try {
      while (!finished) {
        if (!serviceSignal && phase === 'effect' && existsSync(join(work, 'operation.json'))) {
          need(read(join(work, 'operation.json')) === JSON.stringify({ operationId: state.operationId, count: 1 }), 'RECEIPT_MISMATCH');
          need(!existsSync(join(work, 'report.json')), 'CRASH_BOUNDARY_MISSED');
          treeEvidence = tree(true); crashInjected = true; break;
        }
        if (capture.evidence().failure || Date.now() - phaseStart >= phaseMs) {
          failure = capture.evidence().failure ?? 'PHASE_TIMEOUT'; treeEvidence = tree(true); break;
        }
        await sleep(25);
      }
      await waitStopped();
      treeEvidence ??= tree(false);
      need(treeEvidence.stopped, 'ORPHAN_PROCESS');
    } catch (error) {
      failure = ['RECEIPT_MISMATCH', 'CRASH_BOUNDARY_MISSED', 'TREE_STOP_UNVERIFIED', 'PROCESS_STOP_UNVERIFIED', 'ORPHAN_PROCESS'].includes(error.message)
        ? error.message : 'NATIVE_RECOVERY_FAILED';
      if (!finished && !stopAttempted) { treeEvidence = tree(true); await waitStopped(); }
    }
    const output = capture.snapshot(), result = output.nativeResult, status = output.status;
    if (!crashInjected && !output.valid) failure ??= output.evidence.failure ?? 'OUTPUT_INCOMPLETE';
    const rows = read(join(root, `transport-${phase}.jsonl`)).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
    const requests = rows.filter(row => row.event === 'REQUEST_STARTED');
    const settled = rows.filter(row => row.event === 'REQUEST_SETTLED');
    const attempts = Math.max(0, ...settled.map(row => row.attempts));
    const routeMatched = requests.length > 0 && requests.every(row => row.model === `gpt-5.6-${model}` && row.effort === combinations[model]);
    const usage = rows.filter(row => row.event === 'USAGE').map(row => row.usage);
    const signals = rows.filter(row => row.event === 'SERVICE_SIGNAL_INJECTED');
    const usageLedger = serviceSignal ? readVerificationLedger(join(root, `usage-${phase}`, 'tool-usage.jsonl')) : null;
    const withinUsageBudget = usage.reduce((sum, row) => sum + (row.input_tokens ?? 0), 0) <= budget.maxObservedInputTokens
      && usage.reduce((sum, row) => sum + (row.output_tokens ?? 0), 0) <= budget.maxObservedOutputTokens;
    const clean = status?.cleanup && Object.keys(status.cleanup).length === 9 && Object.values(status.cleanup).every(value => value === true);
    const completed = !failure && !crashInjected && nativeOutputCompleted(output, { exitCode,
      sessionId: state.operationId, resultText: 'CLAUDUCT_RECOVERY_DONE', oraclePassed: recoveryOracle(root) })
      && routeMatched && withinUsageBudget && treeEvidence?.stopped;
    const summary = { phase, elapsedMs: Date.now() - phaseStart, exitCode, failure, crashInjected, completed: completed === true,
      routeMatched, attempts, attemptsComplete: requests.length === settled.length, usage, withinUsageBudget,
      outputBytes: output.evidence.bytes, outputCapture: output.evidence,
      nativeJson: result !== null, nativeError: result?.is_error ?? null, sameSession: result?.session_id === state.operationId,
      cleanupComplete: clean === true, tree: treeEvidence };
    if (serviceSignal) Object.assign(summary, { syntheticServiceSignals: signals.length, usageLedger });
    phases.push(summary); write(join(root, `result-${phase}.json`), summary);
    need(!failure && routeMatched && withinUsageBudget && requests.length === settled.length && treeEvidence?.stopped, 'PHASE_FAILED');
    if (serviceSignal) {
      need(usageLedger.finalRecorded && !usageLedger.truncatedTail && usageLedger.requestAttempts === attempts
        && usageLedger.completions === usage.length && usage.length === requests.length - signals.length
        && usageLedger.inputTokens === usage.reduce((sum, row) => sum + row.input_tokens, 0)
        && usageLedger.outputTokens === usage.reduce((sum, row) => sum + row.output_tokens, 0), 'SERVICE_USAGE_UNVERIFIED');
      const audit = read(join(work, 'mcp-events.jsonl')).trim().split('\n').map(line => JSON.parse(line));
      const expectedCalls = phase === 'effect' ? ['apply_effect'] : ['apply_effect', 'effect_status', 'complete_report'];
      need(audit.length === expectedCalls.length && audit.every((row, index) => Object.keys(row).length === 1 && row.name === expectedCalls[index]), 'SERVICE_EFFECT_UNVERIFIED');
      if (phase === 'effect') need(!completed && exitCode !== 0 && result?.is_error === true && result.session_id === state.operationId
        && status?.requestOutcome === 'has-failures' && clean && !recoveryOracle(root) && signals.length === 1
        && signals[0].category === 'UPSTREAM_HTTP_ERROR' && signals[0].status === 503 && signals[0].backendAttempted === false
        && status.lifetime.failed === 1 && status.recentRequests.filter(row => row.success === false).length === 1
        && status.recentRequests.at(-1).failureCategory === 'UPSTREAM_HTTP_ERROR'
        && status.recentRequests.at(-1).attempts.length === 0, 'SERVICE_SIGNAL_NOT_VERIFIED');
      else need(completed && signals.length === 0, 'SERVICE_RESUME_UNVERIFIED');
    } else if (phase === 'effect') need(crashInjected && !recoveryOracle(root), 'CRASH_NOT_VERIFIED');
    return { completed: completed === true, exitCode };
  };
  let first = null, final = null, error = null;
  try {
    first = await runRecovery(root, { execute });
    need(first.state === 'RECOVERING' && !first.taskCompleted
      && (serviceSignal ? phases[0]?.syntheticServiceSignals === 1 : phases[0]?.crashInjected), 'RECOVERY_BASELINE_FAILED');
    // The manager chooses resume from its preserved receipt/journal, without a user turn.
    final = await runRecovery(root, { execute });
  } catch (caught) { error = ['PHASE_FAILED', 'RECOVERY_BASELINE_FAILED', 'CRASH_NOT_VERIFIED', 'SERVICE_USAGE_UNVERIFIED',
    'SERVICE_EFFECT_UNVERIFIED', 'SERVICE_SIGNAL_NOT_VERIFIED', 'SERVICE_RESUME_UNVERIFIED'].includes(caught.message) ? caught.message : 'RECOVERY_FAILED'; }
  const sourceUnchanged = Object.entries(budget.hashes).every(([path, expected]) => hash(join(project, path)) === expected);
  const passed = !error && final?.taskCompleted === true && recoveryOracle(root) && phases.length === 2 && sourceUnchanged;
  const summary = { suite: 'native-recovery-live', root, model, effort: combinations[model], failureMode, passed, error, first, final, sourceUnchanged,
    elapsedMs: Date.now() - started, phases, attempts: phases.reduce((sum, row) => sum + row.attempts, 0),
    cumulativeAttempts: priorAttempts + phases.reduce((sum, row) => sum + row.attempts, 0),
    cumulativeNativeElapsedMs: priorElapsedMs + phases.reduce((sum, row) => sum + row.elapsedMs, 0) };
  write(join(root, 'result.json'), summary); return summary;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [live, model, powershell, priorAttempts, priorElapsedMs, mode] = process.argv.slice(2);
    need(live === '--live' && (process.argv.length === 7 || process.argv.length === 8 && mode === '--service-signal'), 'LIVE_ARGUMENTS_REQUIRED');
    const result = await verifyNativeRecovery({ model, powershell, priorAttempts: Number(priorAttempts), priorElapsedMs: Number(priorElapsedMs),
      failureMode: mode === '--service-signal' ? 'service-signal' : 'crash' });
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-recovery-live', passed: false, error: 'VERIFIER_FAILED' })); process.exitCode = 1; }
}
