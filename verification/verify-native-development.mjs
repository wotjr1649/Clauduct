import { readFileSync, writeFileSync, existsSync, lstatSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createDevelopmentFixture, sourceHash } from './development-fixture.mjs';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';
import { createNativeOutputCapture, nativeOutputCompleted } from './native-output.mjs';
import { readVerificationLedger } from './verification-ledger.mjs';
import { PUBLIC_DEVELOPMENT_SOURCE } from './fixtures/development-responses.mjs';

const need = (ok, label) => { if (!ok) throw new Error(label); };
const sleep = ms => new Promise(done => setTimeout(done, ms));
const write = (path, data) => writeFileSync(path, JSON.stringify(data) + '\n', { flag: 'wx', flush: true });
function read(path, limit = 65536) {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= limit, 'EVIDENCE_PATH');
  return readFileSync(path, 'utf8');
}
export async function verifyNativeDevelopment({ model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens, localNative = false }) {
  const effort = { luna: 'max', sol: 'low' }[model];
  need(effort && typeof powershell === 'string' && powershell.endsWith('pwsh.exe') && typeof localNative === 'boolean', 'INVALID_ARGUMENTS');
  for (const value of [priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens]) need(Number.isSafeInteger(value) && value >= 0, 'INVALID_PRIOR_USAGE');
  const fixture = createDevelopmentFixture(), { project, root, work, control } = fixture;
  const entry = join(project, 'verification', localNative ? 'native-development-local-entry.mjs' : 'native-development-entry.mjs'), helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
  const phaseMs = localNative ? 30000 : 600000;
  const budget = { model, effort, localNative, phaseMs, requestLimit: 16, maxTurns: 8, maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768,
    outputBytes: 1048576, mainConcurrency: 1, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
    cumulativeReservedAttempts: priorAttempts + 16, cumulativeReservedMs: priorElapsedMs + phaseMs,
    cumulativeObservedInputLimit: priorInputTokens + 131072, cumulativeObservedOutputLimit: priorOutputTokens + 32768,
    basis: 'Prior recovery used four requests and 10.6-12.5s. This eight-turn code task reserves 16 attempts and the authorized initial ten-minute development unit, including source review.',
    oracleHash: fixture.oracleHash, taskHash: fixture.taskHash,
    hashes: Object.fromEntries([entry, helper, fixture.script, fileURLToPath(import.meta.url), ...[
      'verification/development-fixture.mjs', 'verification/development-source-policy.mjs', 'verification/fixture-tool-policy.mjs',
      'verification/native-output.mjs', 'verification/verification-ledger.mjs', 'verification/fixtures/development-oracle.mjs',
      'verification/native-development-entry.mjs', 'verification/fixtures/development-responses.mjs',
      'src/clauduct.mjs', 'src/native-transport.mjs', 'src/native-gateway.mjs', 'src/native-protocol.mjs',
      'poc/user-session.mjs', 'verification/manual-http-probe.mjs'].map(path => join(project, path))]
      .map(path => [path.slice(project.length + 1), sourceHash(readFileSync(path))])) };
  write(join(root, 'budget.json'), budget); writeFileSync(join(root, 'transport-development.jsonl'), '', { flag: 'wx' });
  mkdirSync(join(root, 'usage-development'));
  const env = nativeVerificationEnvironment(root);
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]]));
  const sessionId = randomUUID(), phaseStart = Date.now();
  const args = ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '16', '-p',
    '--output-format', 'stream-json', '--verbose', '--tools', '', '--allowedTools', 'mcp__fixture__read_task,mcp__fixture__write_source,mcp__fixture__run_tests',
    '--max-turns', '8', '--session-id', sessionId, '--',
    'Read the complete TASK.md with mcp__fixture__read_task once and implement its bounded code change. Run the prescribed tests on the baseline, fix the code, then run the tests again. Use one fixture tool per response; the three MCP tools are already available. Follow the task completion marker only after tests pass.'];
  const child = fork(entry, [root, 'development', ...args], { cwd: work, env, windowsHide: true, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  const capture = createNativeOutputCapture({ maxBytes: budget.outputBytes, maxLineBytes: budget.outputBytes, maxRecords: 4096 });
  let finished = false, exitCode = null, failure = null, treeEvidence = null, treeStopAttempted = false;
  const stopped = new Promise(done => { child.once('error', () => { failure = 'PROCESS_START_FAILED'; }); child.once('close', code => { exitCode = code; finished = true; done(); }); });
  capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
  write(join(root, 'process.json'), { pid: child.pid, startedAt: phaseStart, sessionId });
  child.send({ start: true });
  console.log(JSON.stringify({ event: 'DEVELOPMENT_STARTED', root, model, effort, pid: child.pid }));
  function tree(stop) {
    if (stop) { need(!treeStopAttempted, 'TREE_STOP_UNVERIFIED'); treeStopAttempted = true; }
    const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(child.pid),
      '-StartedAfterMs', String(phaseStart), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
    { cwd: project, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!result.error && result.status === 0, 'TREE_STOP_UNVERIFIED'); return JSON.parse(result.stdout);
  }
  let reportedReview = '';
  try {
    while (!finished) {
      if (capture.evidence().failure || Date.now() - phaseStart >= budget.phaseMs) {
        failure = capture.evidence().failure ?? 'PHASE_TIMEOUT'; treeEvidence = tree(true); break;
      }
      const reviewPath = join(work, 'review-request.json');
      if (existsSync(reviewPath)) {
        const requested = JSON.parse(read(reviewPath, 1024));
        const reviewed = JSON.parse(read(join(control, 'review.json'), 1024));
        if (requested.sha256 !== reviewed.sha256 && requested.sha256 !== reportedReview) {
          reportedReview = requested.sha256;
          if (localNative) {
            need(requested.sha256 === sourceHash(PUBLIC_DEVELOPMENT_SOURCE)
              && read(join(work, 'retry-after-seconds.mjs')) === PUBLIC_DEVELOPMENT_SOURCE, 'LOCAL_SOURCE_MISMATCH');
            writeFileSync(join(control, 'review.json'), JSON.stringify({ sha256: requested.sha256, approved: true }), { flush: true });
          } else console.log(JSON.stringify({ event: 'SOURCE_REVIEW_PENDING', root, sourcePath: join(work, 'retry-after-seconds.mjs'), sha256: requested.sha256 }));
        }
      }
      await sleep(50);
    }
    const deadline = Date.now() + 5000;
    while (!finished && Date.now() < deadline) await sleep(25);
    need(finished, 'PROCESS_STOP_UNVERIFIED'); await stopped;
    treeEvidence ??= tree(false); need(treeEvidence.stopped, 'ORPHAN_PROCESS');
  } catch (error) {
    failure = ['TREE_STOP_UNVERIFIED', 'PROCESS_STOP_UNVERIFIED', 'ORPHAN_PROCESS'].includes(error.message) ? error.message : 'DEVELOPMENT_EXECUTION_FAILED';
    // A failed stop is not silently retried through another mechanism.
    if (!finished && !treeStopAttempted && failure !== 'TREE_STOP_UNVERIFIED') {
      try { treeEvidence = tree(true); } catch { failure = 'TREE_STOP_UNVERIFIED'; }
    }
  }
  const output = capture.snapshot(), result = output.nativeResult, status = output.status;
  failure ??= output.evidence.failure;
  const rows = read(join(root, 'transport-development.jsonl')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const requests = rows.filter(row => row.event === 'REQUEST_STARTED'), settled = rows.filter(row => row.event === 'REQUEST_SETTLED');
  const attempts = Math.max(0, ...settled.map(row => row.attempts));
  const routeMatched = requests.length > 0 && requests.every(row => row.model === `gpt-5.6-${model}` && row.effort === effort);
  const inputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.input_tokens ?? 0), 0);
  const outputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.output_tokens ?? 0), 0);
  let usageLedger = null;
  try { usageLedger = readVerificationLedger(join(root, 'usage-development', 'tool-usage.jsonl')); }
  catch { failure ??= 'VERIFICATION_LEDGER_INVALID'; }
  const ledgerMatched = usageLedger?.finalRecorded === true && usageLedger.truncatedTail === false
    && usageLedger.requestAttempts === attempts && usageLedger.inputTokens === inputTokens && usageLedger.outputTokens === outputTokens;
  const sourceUnchanged = Object.entries(budget.hashes).every(([path, hash]) => sourceHash(readFileSync(join(project, path))) === hash);
  const events = existsSync(join(work, 'events.jsonl')) ? read(join(work, 'events.jsonl')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
  const source = join(work, 'retry-after-seconds.mjs'), sha256 = sourceHash(read(source));
  const review = JSON.parse(read(join(control, 'review.json'), 1024));
  const oracleUnchanged = sourceHash(read(join(control, 'oracle.mjs'))) === fixture.oracleHash;
  let independentPassed = false;
  if (finished && !failure && oracleUnchanged && review.sha256 === sha256 && review.approved === true) {
    const oracle = join(control, 'oracle.mjs');
    const check = spawnSync(process.execPath, ['--permission', `--allow-fs-read=${oracle}`, `--allow-fs-read=${source}`, oracle, source],
      { cwd: work, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 16384 });
    try { const verdict = JSON.parse(check.stdout); independentPassed = !check.error && check.status === 0 && verdict.passed && verdict.checks === 28 && verdict.failures.length === 0; } catch { }
  }
  const tests = events.filter(row => row.event === 'TESTS_EXECUTED');
  const baselineFailed = tests.length >= 2 && tests[0].passed === false;
  const revisedPassed = tests.at(-1)?.passed === true && tests.at(-1)?.sha256 === sha256 && events.some(row => row.event === 'SOURCE_WRITTEN');
  const cleanupComplete = status?.cleanup && Object.keys(status.cleanup).length === 9 && Object.values(status.cleanup).every(value => value === true);
  const passed = !failure && nativeOutputCompleted(output, { exitCode, sessionId, resultText: 'CLAUDUCT_DEVELOPMENT_DONE', oraclePassed: independentPassed })
    && ledgerMatched && sourceUnchanged && treeEvidence?.stopped
    && routeMatched && requests.length === settled.length && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens
    && events[0]?.event === 'TASK_READ' && baselineFailed && revisedPassed && independentPassed && oracleUnchanged;
  const summary = { suite: localNative ? 'native-development-local' : 'native-development-live', root, model, effort, localNative,
    actualModelRequests: localNative ? 0 : attempts, passed: passed === true, failure, exitCode,
    elapsedMs: Date.now() - phaseStart, attempts, attemptsComplete: requests.length === settled.length, inputTokens, outputTokens,
    cumulativeAttempts: priorAttempts + attempts, cumulativeElapsedMs: priorElapsedMs + Date.now() - phaseStart,
    cumulativeInputTokens: priorInputTokens + inputTokens, cumulativeOutputTokens: priorOutputTokens + outputTokens,
    routeMatched, nativeJson: result !== null, nativeError: result?.is_error ?? null, cleanupComplete: cleanupComplete === true,
    baselineFailed, revisedPassed, independentPassed, oracleUnchanged, testsExecuted: tests.length, sourceSha256: sha256,
    sourceUnchanged, ledgerMatched, usageLedger, output: output.evidence, tree: treeEvidence };
  write(join(root, 'result.json'), summary); return summary;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [live, model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens] = process.argv.slice(2);
    const localNative = live === '--local-native';
    need(localNative ? process.argv.length === 5 : live === '--live' && process.argv.length === 9, 'LIVE_ARGUMENTS_REQUIRED');
    const result = await verifyNativeDevelopment({ model, powershell, localNative, priorAttempts: localNative ? 0 : Number(priorAttempts),
      priorElapsedMs: localNative ? 0 : Number(priorElapsedMs), priorInputTokens: localNative ? 0 : Number(priorInputTokens),
      priorOutputTokens: localNative ? 0 : Number(priorOutputTokens) });
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-development-live', passed: false, failure: 'VERIFIER_FAILED' })); process.exitCode = 1; }
}
