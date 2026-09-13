import { readFileSync, writeFileSync, existsSync, lstatSync, mkdirSync, realpathSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createDevelopmentFixture, sourceHash } from './development-fixture.mjs';
import { developmentTask, DEFAULT_DEVELOPMENT_TASK_ID } from './development-tasks.mjs';
import { checkDevelopmentSource } from './development-source-policy.mjs';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';
import { createNativeOutputCapture, nativeOutputCompleted } from './native-output.mjs';
import { readVerificationLedger } from './verification-ledger.mjs';
import { publicDevelopmentSource } from './fixtures/development-responses.mjs';

const need = (ok, label) => { if (!ok) throw new Error(label); };
const sleep = ms => new Promise(done => setTimeout(done, ms));
const write = (path, data) => writeFileSync(path, JSON.stringify(data) + '\n', { flag: 'wx', flush: true });
function read(path, limit = 65536) {
  const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= limit, 'EVIDENCE_PATH');
  return readFileSync(path, 'utf8');
}
function nativeClientsStopped(rows) {
  const started = rows.filter(row => row.event === 'NATIVE_STARTED');
  return started.length === 1 && Number.isSafeInteger(started[0].pid) && started[0].pid > 0
    && (() => { try { process.kill(started[0].pid, 0); return false; } catch (error) { return error.code === 'ESRCH'; } })();
}
function developmentHashes(project, localNative) {
  const files = [localNative ? 'verification/native-development-local-entry.mjs' : 'verification/native-development-entry.mjs',
    'verification/stop-owned-native-tree.ps1', 'verification/fixtures/development-mcp.mjs', 'verification/verify-native-development.mjs',
    'verification/development-fixture.mjs', 'verification/development-tasks.mjs', 'verification/development-source-policy.mjs', 'verification/fixture-tool-policy.mjs',
    'verification/native-output.mjs', 'verification/verification-ledger.mjs', 'verification/fixtures/development-oracle.mjs', 'verification/fixtures/development-window-oracle.mjs',
    'verification/native-development-entry.mjs', 'verification/fixtures/development-responses.mjs',
    'src/clauduct.mjs', 'src/native-transport.mjs', 'src/native-gateway.mjs', 'src/native-protocol.mjs',
    'poc/user-session.mjs', 'verification/manual-http-probe.mjs', 'verification/auth-store-selection.mjs'];
  return Object.fromEntries(files.map(name => {
    const path = join(project, name); return [path.slice(project.length + 1), sourceHash(readFileSync(path))];
  }));
}
function resumeDevelopmentFixture(root, model, localNative, taskId) {
  const task = developmentTask(taskId);
  const project = dirname(dirname(fileURLToPath(import.meta.url))), canonical = resolve(root);
  need(dirname(canonical).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(canonical.slice(canonical.lastIndexOf('\\') + 1))
    && realpathSync(canonical).toLowerCase() === canonical.toLowerCase(), 'RESUME_ROOT_INVALID');
  const priorBudget = JSON.parse(read(join(root, 'budget.json'))), first = JSON.parse(read(join(root, 'result.json')));
  const owner = JSON.parse(read(join(root, 'process.json')));
  need(priorBudget.model === model && priorBudget.localNative === localNative && priorBudget.taskId === taskId && priorBudget.cutOutputAfterPass === true
    && first.outputCutObserved === true && first.passed === false && first.tree?.stopped === true
    && /^[0-9a-f-]{36}$/.test(owner.sessionId), 'RESUME_EVIDENCE_INVALID');
  need(nativeClientsStopped(read(join(root, 'transport-development.jsonl')).trim().split('\n').filter(Boolean).map(JSON.parse)), 'RESUME_OWNER_UNVERIFIED');
  need(JSON.stringify(priorBudget.hashes) === JSON.stringify(developmentHashes(project, localNative)), 'RESUME_SOURCE_CHANGED');
  need(priorBudget.taskHash === sourceHash(task.task)
    && priorBudget.oracleHash === sourceHash(readFileSync(join(project, 'verification', 'fixtures', task.oracleFile))), 'RESUME_ARTIFACT_CHANGED');
  need(sourceHash(read(join(root, 'work', task.sourceFile))) === first.sourceSha256
    && sourceHash(read(join(root, 'work', '.mcp.json'))) === priorBudget.mcpHash
    && sourceHash(read(join(root, 'control', 'oracle.mjs'))) === priorBudget.oracleHash
    && sourceHash(read(join(root, 'control', 'TASK.md'))) === priorBudget.taskHash, 'RESUME_ARTIFACT_CHANGED');
  checkDevelopmentSource(read(join(root, 'work', task.sourceFile)), taskId);
  return { project, root, taskId, task, work: join(root, 'work'), control: join(root, 'control'),
    script: join(project, 'verification', 'fixtures', 'development-mcp.mjs'), oracleHash: priorBudget.oracleHash,
    taskHash: priorBudget.taskHash, priorOwner: owner, priorSourceHash: first.sourceSha256 };
}
export async function verifyNativeDevelopment({ model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
  localNative = false, cutOutputAfterPass = false, resumeRoot, taskId = DEFAULT_DEVELOPMENT_TASK_ID }) {
  const effort = { luna: 'max', sol: 'low' }[model];
  need(typeof model === 'string' && Object.hasOwn({ luna: 'max', sol: 'low' }, model)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe') && typeof localNative === 'boolean'
    && typeof cutOutputAfterPass === 'boolean'
    && (resumeRoot === undefined || typeof resumeRoot === 'string' && !cutOutputAfterPass), 'INVALID_ARGUMENTS');
  try { developmentTask(taskId); } catch { need(false, 'INVALID_ARGUMENTS'); }
  for (const value of [priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens]) need(Number.isSafeInteger(value) && value >= 0, 'INVALID_PRIOR_USAGE');
  const finishing = resumeRoot !== undefined, phase = finishing ? 'development-finish' : 'development';
  const fixture = finishing ? resumeDevelopmentFixture(resumeRoot, model, localNative, taskId) : createDevelopmentFixture({ holdAfterPass: cutOutputAfterPass, taskId });
  const { project, root, work, control, task } = fixture;
  const entry = join(project, 'verification', localNative ? 'native-development-local-entry.mjs' : 'native-development-entry.mjs'), helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
  const phaseMs = localNative ? 30000 : finishing ? 120000 : 600000;
  const budget = { model, effort, taskId, localNative, phase, cutOutputAfterPass, phaseMs, requestLimit: 16, maxTurns: 8, maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768,
    outputBytes: 1048576, mainConcurrency: 1, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
    cumulativeReservedAttempts: priorAttempts + 16, cumulativeReservedMs: priorElapsedMs + phaseMs,
    cumulativeObservedInputLimit: priorInputTokens + 131072, cumulativeObservedOutputLimit: priorOutputTokens + 32768,
    basis: 'Prior recovery used four requests and 10.6-12.5s. This eight-turn code task reserves 16 attempts and the authorized initial ten-minute development unit, including source review.',
    oracleHash: fixture.oracleHash, taskHash: fixture.taskHash, mcpHash: sourceHash(read(join(work, '.mcp.json'))),
    hashes: developmentHashes(project, localNative) };
  const env = nativeVerificationEnvironment(root);
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]]));
  if (finishing) {
    const owner = fixture.priorOwner;
    const previous = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(owner.pid),
      '-StartedAfterMs', String(owner.startedAt), '-RunRoot', root],
      { cwd: project, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!previous.error && previous.status === 0 && JSON.parse(previous.stdout).stopped === true, 'RESUME_OWNER_UNVERIFIED');
    // Exclusive, task-local intent is created before any new child or effect.
    write(join(root, 'finish-intent.json'), { sessionId: owner.sessionId, sourceSha256: fixture.priorSourceHash });
  }
  write(join(root, finishing ? 'budget-finish.json' : 'budget.json'), budget);
  writeFileSync(join(root, `transport-${phase}.jsonl`), '', { flag: 'wx' }); mkdirSync(join(root, `usage-${phase}`));
  const sessionId = finishing ? fixture.priorOwner.sessionId : randomUUID(), phaseStart = Date.now();
  const args = ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '16', '-p',
    '--output-format', 'stream-json', '--verbose', '--tools', '', '--allowedTools',
    finishing ? 'mcp__fixture__read_task,mcp__fixture__run_tests' : 'mcp__fixture__read_task,mcp__fixture__write_source,mcp__fixture__run_tests',
    '--max-turns', '8', finishing ? '--resume' : '--session-id', sessionId, '--', finishing
    ? 'Continue the same public development task after the output receiver lost the final result. The outer manager verified that your source was written once, reviewed and passed the fixed tests. Do not modify it. Read the original requirements and current source once with mcp__fixture__read_task, run mcp__fixture__run_tests once, and after success reply exactly CLAUDUCT_DEVELOPMENT_DONE. Use one tool per response. The original baseline/fix instructions are already fulfilled; only verification and this final report remain.'
    :
    'Read the complete TASK.md with mcp__fixture__read_task once and implement its bounded code change. Run the prescribed tests on the baseline, fix the code, then run the tests again. Use one fixture tool per response; the three MCP tools are already available. Follow the task completion marker only after tests pass.'];
  const child = fork(entry, [root, phase, ...args], { cwd: work, env, windowsHide: true, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  const capture = createNativeOutputCapture({ maxBytes: budget.outputBytes, maxLineBytes: budget.outputBytes, maxRecords: 4096 });
  let finished = false, exitCode = null, failure = null, treeEvidence = null, treeStopAttempted = false, outputCut = false, cutReleased = false;
  const stopped = new Promise(done => { child.once('error', () => { failure = 'PROCESS_START_FAILED'; }); child.once('close', code => { exitCode = code; finished = true; done(); }); });
  capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
  write(join(root, finishing ? 'process-finish.json' : 'process.json'), { pid: child.pid, startedAt: phaseStart, sessionId });
  child.send({ start: true });
  console.log(JSON.stringify({ event: 'DEVELOPMENT_STARTED', root, model, effort, taskId, pid: child.pid }));
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
      if (capture.evidence().failure && !outputCut || failure === 'OUTPUT_CUT_RELEASE_FAILED' || Date.now() - phaseStart >= budget.phaseMs) {
        failure ??= capture.evidence().failure ?? 'PHASE_TIMEOUT'; treeEvidence = tree(true); break;
      }
      const reviewPath = join(work, 'review-request.json');
      if (existsSync(reviewPath)) {
        const requested = JSON.parse(read(reviewPath, 1024));
        const reviewed = JSON.parse(read(join(control, 'review.json'), 1024));
        if (requested.sha256 !== reviewed.sha256 && requested.sha256 !== reportedReview) {
          reportedReview = requested.sha256;
          if (localNative) {
            need(requested.sha256 === sourceHash(publicDevelopmentSource(taskId))
              && read(join(work, task.sourceFile)) === publicDevelopmentSource(taskId), 'LOCAL_SOURCE_MISMATCH');
            writeFileSync(join(control, 'review.json'), JSON.stringify({ sha256: requested.sha256, approved: true }), { flush: true });
          } else console.log(JSON.stringify({ event: 'SOURCE_REVIEW_PENDING', root, taskId, sourcePath: join(work, task.sourceFile), sha256: requested.sha256 }));
        }
      }
      if (cutOutputAfterPass && !outputCut && existsSync(join(work, 'events.jsonl'))) {
        const events = read(join(work, 'events.jsonl')).trim().split('\n').filter(Boolean).map(JSON.parse);
        if (events.some(row => row.event === 'OUTPUT_CUT_WAIT')) {
          need(capture.evidence().resultCount === 0 && events.filter(row => row.event === 'SOURCE_WRITTEN').length === 1,
            'OUTPUT_CUT_BOUNDARY_INVALID');
          outputCut = true;
          child.stdout.once('close', () => {
            try {
              writeFileSync(join(control, 'output-cut-release.json'), JSON.stringify({ released: true }), { flag: 'wx', flush: true });
              cutReleased = true;
            } catch { failure = 'OUTPUT_CUT_RELEASE_FAILED'; }
          });
          child.stdout.destroy();
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
  const rows = read(join(root, `transport-${phase}.jsonl`)).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const recordedNativeStopped = nativeClientsStopped(rows);
  const requests = rows.filter(row => row.event === 'REQUEST_STARTED'), settled = rows.filter(row => row.event === 'REQUEST_SETTLED');
  const attempts = Math.max(0, ...settled.map(row => row.attempts));
  const routeMatched = requests.length > 0 && requests.every(row => row.model === `gpt-5.6-${model}` && row.effort === effort);
  const inputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.input_tokens ?? 0), 0);
  const outputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.output_tokens ?? 0), 0);
  let usageLedger = null;
  try { usageLedger = readVerificationLedger(join(root, `usage-${phase}`, 'tool-usage.jsonl')); }
  catch { failure ??= 'VERIFICATION_LEDGER_INVALID'; }
  const ledgerMatched = usageLedger?.finalRecorded === true && usageLedger.truncatedTail === false
    && usageLedger.requestAttempts === attempts && usageLedger.inputTokens === inputTokens && usageLedger.outputTokens === outputTokens;
  const sourceUnchanged = Object.entries(budget.hashes).every(([path, hash]) => sourceHash(readFileSync(join(project, path))) === hash);
  const events = existsSync(join(work, 'events.jsonl')) ? read(join(work, 'events.jsonl')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
  const source = join(work, task.sourceFile), sha256 = sourceHash(read(source));
  const review = JSON.parse(read(join(control, 'review.json'), 1024));
  const oracleUnchanged = sourceHash(read(join(control, 'oracle.mjs'))) === fixture.oracleHash;
  let independentPassed = false;
  if (finished && (!failure || outputCut && failure === 'OUTPUT_PIPE_CLOSED') && oracleUnchanged && review.sha256 === sha256 && review.approved === true) {
    const oracle = join(control, 'oracle.mjs');
    const check = spawnSync(process.execPath, ['--permission', `--allow-fs-read=${oracle}`, `--allow-fs-read=${source}`, oracle, source],
      { cwd: work, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 16384 });
    try { const verdict = JSON.parse(check.stdout); independentPassed = !check.error && check.status === 0 && verdict.passed && verdict.checks === task.checks && verdict.failures.length === 0; } catch { }
  }
  const tests = events.filter(row => row.event === 'TESTS_EXECUTED');
  const baselineFailed = tests.length >= 2 && tests[0].passed === false;
  const revisedPassed = tests.at(-1)?.passed === true && tests.at(-1)?.sha256 === sha256 && events.some(row => row.event === 'SOURCE_WRITTEN');
  const cleanupComplete = status?.cleanup && Object.keys(status.cleanup).length === 9 && Object.values(status.cleanup).every(value => value === true);
  const passed = !failure && nativeOutputCompleted(output, { exitCode, sessionId, resultText: 'CLAUDUCT_DEVELOPMENT_DONE', oraclePassed: independentPassed })
    && ledgerMatched && sourceUnchanged && treeEvidence?.stopped && recordedNativeStopped
    && routeMatched && requests.length === settled.length && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens
    && events[0]?.event === 'TASK_READ' && baselineFailed && revisedPassed && independentPassed && oracleUnchanged;
  const outputCutObserved = outputCut && cutReleased && output.evidence.failure === 'OUTPUT_PIPE_CLOSED'
    && output.evidence.resultCount === 0 && output.valid === false && treeEvidence?.stopped === true
    && sourceUnchanged && ledgerMatched && independentPassed && baselineFailed && revisedPassed && recordedNativeStopped
    && events.filter(row => row.event === 'SOURCE_WRITTEN').length === 1 && routeMatched && requests.length === settled.length
    && attempts <= budget.requestLimit && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens;
  const summary = { suite: localNative ? 'native-development-local' : 'native-development-live', root, model, effort, taskId, localNative, phase, sessionId,
    actualModelRequests: localNative ? 0 : attempts, passed: passed === true, failure, exitCode,
    elapsedMs: Date.now() - phaseStart, attempts, attemptsComplete: requests.length === settled.length, inputTokens, outputTokens,
    cumulativeAttempts: priorAttempts + attempts, cumulativeElapsedMs: priorElapsedMs + Date.now() - phaseStart,
    cumulativeInputTokens: priorInputTokens + inputTokens, cumulativeOutputTokens: priorOutputTokens + outputTokens,
    routeMatched, nativeJson: result !== null, nativeError: result?.is_error ?? null, cleanupComplete: cleanupComplete === true,
    baselineFailed, revisedPassed, independentPassed, oracleUnchanged, testsExecuted: tests.length, sourceSha256: sha256,
    sourceUnchanged, ledgerMatched, usageLedger, outputCut, cutReleased, outputCutObserved, recordedNativeStopped,
    output: output.evidence, tree: treeEvidence };
  write(join(root, finishing ? 'result-finish.json' : 'result.json'), summary); return summary;
}
export async function verifyDevelopmentOutputRecovery({ model, powershell, localNative, taskId = DEFAULT_DEVELOPMENT_TASK_ID,
  priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens }) {
  need(typeof localNative === 'boolean', 'INVALID_ARGUMENTS');
  const first = await verifyNativeDevelopment({ model, powershell, localNative, taskId, cutOutputAfterPass: true,
    priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens });
  let finish = null, resumeFailure = null;
  if (first.outputCutObserved && !first.passed) {
    try { finish = await verifyNativeDevelopment({ model, powershell, localNative, taskId,
      resumeRoot: first.root, priorAttempts: priorAttempts + first.attempts, priorElapsedMs: priorElapsedMs + first.elapsedMs,
      priorInputTokens: priorInputTokens + first.inputTokens, priorOutputTokens: priorOutputTokens + first.outputTokens }); }
    catch (error) {
      resumeFailure = ['RESUME_ROOT_INVALID', 'RESUME_EVIDENCE_INVALID', 'RESUME_SOURCE_CHANGED', 'RESUME_ARTIFACT_CHANGED',
        'RESUME_OWNER_UNVERIFIED'].includes(error.message) ? error.message : error.code === 'EEXIST' ? 'RESUME_ALREADY_CLAIMED' : 'DEVELOPMENT_RESUME_FAILED';
    }
  }
  const passed = first.outputCutObserved === true && first.passed === false && finish?.passed === true
    && first.sessionId === finish.sessionId && first.sourceSha256 === finish.sourceSha256;
  const attempts = first.attempts + (finish?.attempts ?? 0), elapsedMs = first.elapsedMs + (finish?.elapsedMs ?? 0);
  const result = { suite: localNative ? 'native-development-output-recovery-local' : 'native-development-output-recovery-live',
    root: first.root, model, localNative, passed, scenarioPassed: passed, taskCompleted: passed, first, finish, resumeFailure,
    attempts, elapsedMs, inputTokens: first.inputTokens + (finish?.inputTokens ?? 0), outputTokens: first.outputTokens + (finish?.outputTokens ?? 0),
    attemptsComplete: first.attemptsComplete && finish?.attemptsComplete === true,
    actualModelRequests: localNative ? 0 : attempts, ...(localNative ? { actualCredentialReads: 0 } : {}) };
  write(join(first.root, 'output-recovery-result.json'), result); return result;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [live, model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens] = process.argv.slice(2);
    if (['--local-output-recovery', '--live-output-recovery'].includes(live)) {
      const localNative = live === '--local-output-recovery';
      need(process.argv.length === (localNative ? 5 : 9), 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyDevelopmentOutputRecovery({ model, powershell, localNative,
        priorAttempts: localNative ? 0 : Number(priorAttempts), priorElapsedMs: localNative ? 0 : Number(priorElapsedMs),
        priorInputTokens: localNative ? 0 : Number(priorInputTokens), priorOutputTokens: localNative ? 0 : Number(priorOutputTokens) });
      console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    } else if (['--local-task', '--live-task'].includes(live)) {
      const localNative = live === '--local-task';
      const [, , , taskId, ...prior] = process.argv.slice(2);
      need(process.argv.length === (localNative ? 6 : 10), 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyNativeDevelopment({ model, powershell, localNative, taskId,
        priorAttempts: localNative ? 0 : Number(prior[0]), priorElapsedMs: localNative ? 0 : Number(prior[1]),
        priorInputTokens: localNative ? 0 : Number(prior[2]), priorOutputTokens: localNative ? 0 : Number(prior[3]) });
      console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    } else {
    const localNative = ['--local-native', '--local-output-cut'].includes(live), cutOutputAfterPass = live === '--local-output-cut';
    need(localNative ? process.argv.length === 5 : live === '--live' && process.argv.length === 9, 'LIVE_ARGUMENTS_REQUIRED');
    const result = await verifyNativeDevelopment({ model, powershell, localNative, cutOutputAfterPass, priorAttempts: localNative ? 0 : Number(priorAttempts),
      priorElapsedMs: localNative ? 0 : Number(priorElapsedMs), priorInputTokens: localNative ? 0 : Number(priorInputTokens),
      priorOutputTokens: localNative ? 0 : Number(priorOutputTokens) });
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    }
  } catch { console.log(JSON.stringify({ suite: 'native-development-live', passed: false, failure: 'VERIFIER_FAILED' })); process.exitCode = 1; }
}
