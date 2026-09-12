import { readFileSync, writeFileSync, existsSync, lstatSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createDevelopmentFixture, sourceHash } from './development-fixture.mjs';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';

const need = (ok, label) => { if (!ok) throw new Error(label); };
const sleep = ms => new Promise(done => setTimeout(done, ms));
const write = (path, data) => writeFileSync(path, JSON.stringify(data) + '\n', { flag: 'wx', flush: true });
function read(path, limit = 65536) {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= limit, 'EVIDENCE_PATH');
  return readFileSync(path, 'utf8');
}
export async function verifyNativeDevelopment({ model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens }) {
  const effort = { luna: 'max', sol: 'low' }[model];
  need(effort && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'), 'INVALID_ARGUMENTS');
  for (const value of [priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens]) need(Number.isSafeInteger(value) && value >= 0, 'INVALID_PRIOR_USAGE');
  const fixture = createDevelopmentFixture(), { project, root, work, control } = fixture;
  const entry = join(project, 'verification', 'native-recovery-entry.mjs'), helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
  const budget = { model, effort, phaseMs: 600000, requestLimit: 16, maxTurns: 8, maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768,
    outputBytes: 1048576, mainConcurrency: 1, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
    cumulativeReservedAttempts: priorAttempts + 16, cumulativeReservedMs: priorElapsedMs + 600000,
    cumulativeObservedInputLimit: priorInputTokens + 131072, cumulativeObservedOutputLimit: priorOutputTokens + 32768,
    basis: 'Prior recovery used four requests and 10.6-12.5s. This eight-turn code task reserves 16 attempts and the authorized initial ten-minute development unit, including source review.',
    oracleHash: fixture.oracleHash, taskHash: fixture.taskHash,
    hashes: Object.fromEntries([entry, helper, fixture.script, fileURLToPath(import.meta.url)].map(path => [path.slice(project.length + 1), sourceHash(readFileSync(path))])) };
  write(join(root, 'budget.json'), budget); writeFileSync(join(root, 'transport-development.jsonl'), '', { flag: 'wx' });
  const env = nativeVerificationEnvironment(root);
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]]));
  const sessionId = randomUUID(), phaseStart = Date.now();
  const args = ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '16', '-p',
    '--output-format', 'json', '--tools', 'ToolSearch', '--allowedTools', 'ToolSearch,mcp__fixture__read_task,mcp__fixture__write_source,mcp__fixture__run_tests',
    '--max-turns', '8', '--session-id', sessionId, '--',
    'Read the complete TASK.md with mcp__fixture__read_task and implement its bounded code change. Run the prescribed tests on the baseline, fix the code, then run the tests again. Use only the local fixture tools, discovering them with ToolSearch if needed. Follow the task completion marker only after tests pass.'];
  const child = fork(entry, [root, 'development', ...args], { cwd: work, env, windowsHide: true, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  let stdout = '', stderr = '', bytes = 0, finished = false, exitCode = null, failure = null, treeEvidence = null;
  const stopped = new Promise(done => { child.once('error', () => { failure = 'PROCESS_START_FAILED'; }); child.once('close', code => { exitCode = code; finished = true; done(); }); });
  child.stdout.on('data', chunk => { bytes += chunk.length; if (bytes <= budget.outputBytes) stdout += chunk.toString(); });
  child.stderr.on('data', chunk => { bytes += chunk.length; if (bytes <= budget.outputBytes) stderr += chunk.toString(); });
  write(join(root, 'process.json'), { pid: child.pid, startedAt: phaseStart, sessionId });
  child.send({ start: true });
  console.log(JSON.stringify({ event: 'DEVELOPMENT_STARTED', root, model, effort, pid: child.pid }));
  function tree(stop) {
    const result = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(child.pid),
      '-StartedAfterMs', String(phaseStart), '-RunRoot', root, ...(stop ? ['-Stop'] : [])],
    { cwd: project, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!result.error && result.status === 0, 'TREE_STOP_UNVERIFIED'); return JSON.parse(result.stdout);
  }
  let reportedReview = '';
  try {
    while (!finished) {
      if (bytes > budget.outputBytes || Date.now() - phaseStart >= budget.phaseMs) {
        failure = bytes > budget.outputBytes ? 'OUTPUT_LIMIT' : 'PHASE_TIMEOUT'; treeEvidence = tree(true); break;
      }
      const reviewPath = join(work, 'review-request.json');
      if (existsSync(reviewPath)) {
        const requested = JSON.parse(read(reviewPath, 1024));
        const reviewed = JSON.parse(read(join(control, 'review.json'), 1024));
        if (requested.sha256 !== reviewed.sha256 && requested.sha256 !== reportedReview) {
          reportedReview = requested.sha256;
          console.log(JSON.stringify({ event: 'SOURCE_REVIEW_PENDING', root, sourcePath: join(work, 'retry-after-seconds.mjs'), sha256: requested.sha256 }));
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
    if (!finished && failure !== 'TREE_STOP_UNVERIFIED') {
      try { treeEvidence = tree(true); } catch { failure = 'TREE_STOP_UNVERIFIED'; }
    }
  }
  let result = null, status = null;
  try { result = JSON.parse(stdout); } catch { }
  try { status = JSON.parse(stderr.split(/\r?\n/).filter(line => line.startsWith('CLAUDUCT_REQUEST_STATUS ')).at(-1)?.slice(24)); } catch { }
  const rows = read(join(root, 'transport-development.jsonl')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const requests = rows.filter(row => row.event === 'REQUEST_STARTED'), settled = rows.filter(row => row.event === 'REQUEST_SETTLED');
  const attempts = Math.max(0, ...settled.map(row => row.attempts));
  const routeMatched = requests.length > 0 && requests.every(row => row.model === `gpt-5.6-${model}` && row.effort === effort);
  const inputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.input_tokens ?? 0), 0);
  const outputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.output_tokens ?? 0), 0);
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
  const passed = !failure && exitCode === 0 && result?.is_error === false && result.result?.trim() === 'CLAUDUCT_DEVELOPMENT_DONE'
    && result.session_id === sessionId && status?.requestOutcome === 'all-succeeded' && cleanupComplete && treeEvidence?.stopped
    && routeMatched && requests.length === settled.length && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens
    && events[0]?.event === 'TASK_READ' && baselineFailed && revisedPassed && independentPassed && oracleUnchanged;
  const summary = { suite: 'native-development-live', root, model, effort, passed: passed === true, failure, exitCode,
    elapsedMs: Date.now() - phaseStart, attempts, attemptsComplete: requests.length === settled.length, inputTokens, outputTokens,
    cumulativeAttempts: priorAttempts + attempts, cumulativeElapsedMs: priorElapsedMs + Date.now() - phaseStart,
    cumulativeInputTokens: priorInputTokens + inputTokens, cumulativeOutputTokens: priorOutputTokens + outputTokens,
    routeMatched, nativeJson: result !== null, nativeError: result?.is_error ?? null, cleanupComplete: cleanupComplete === true,
    baselineFailed, revisedPassed, independentPassed, oracleUnchanged, testsExecuted: tests.length, sourceSha256: sha256, tree: treeEvidence };
  write(join(root, 'result.json'), summary); return summary;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [live, model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens] = process.argv.slice(2);
    need(live === '--live' && process.argv.length === 9, 'LIVE_ARGUMENTS_REQUIRED');
    const result = await verifyNativeDevelopment({ model, powershell, priorAttempts: Number(priorAttempts), priorElapsedMs: Number(priorElapsedMs),
      priorInputTokens: Number(priorInputTokens), priorOutputTokens: Number(priorOutputTokens) });
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
  } catch { console.log(JSON.stringify({ suite: 'native-development-live', passed: false, failure: 'VERIFIER_FAILED' })); process.exitCode = 1; }
}
