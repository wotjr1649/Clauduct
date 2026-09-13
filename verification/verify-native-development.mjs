import { readFileSync, writeFileSync, existsSync, lstatSync, mkdirSync, realpathSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createDevelopmentFixture, sourceHash } from './development-fixture.mjs';
import { developmentTask, developmentOracleSource, DEFAULT_DEVELOPMENT_TASK_ID } from './development-tasks.mjs';
import { checkDevelopmentSource } from './development-source-policy.mjs';
import { readDevelopmentArtifactHashes, verifyDevelopmentArtifacts } from './development-artifacts.mjs';
import { nativeVerificationEnvironment } from './verify-native-recovery.mjs';
import { createNativeOutputCapture, nativeOutputCompleted, nativeOutputDiagnostics } from './native-output.mjs';
import { readVerificationLedger } from './verification-ledger.mjs';
import { publicDevelopmentSource } from './fixtures/development-responses.mjs';
import { rateLimitEvidence } from '../src/rate-limit-observation.mjs';
import { createExecutionReservation, settleExecutionReservation, readExecutionReservation } from './execution-reservation.mjs';

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
    'verification/development-source-grammar.mjs', 'verification/registered-development-tasks.mjs', 'verification/development-artifacts.mjs', 'verification/development-change.mjs',
    'verification/native-output.mjs', 'verification/verification-ledger.mjs', 'verification/execution-reservation.mjs', 'verification/fixtures/development-oracle.mjs', 'verification/fixtures/development-window-oracle.mjs',
    'verification/execution-account.mjs', 'verification/managed-development.mjs', 'verification/managed-ledger-account.mjs', 'verification/managed-plan-entry.mjs',
    'verification/native-development-entry.mjs', 'verification/fixtures/development-responses.mjs',
    'verification/verify-native-recovery.mjs', 'src/models.mjs', 'src/client-version.mjs', 'poc/adapter.mjs',
    'src/clauduct.mjs', 'src/native-transport.mjs', 'src/rate-limit-observation.mjs', 'src/native-gateway.mjs', 'src/native-protocol.mjs', 'src/request-status.mjs',
    'poc/user-session.mjs', 'verification/manual-http-probe.mjs', 'verification/auth-store-selection.mjs'];
  return Object.fromEntries(files.map(name => {
    const path = join(project, name); return [path.slice(project.length + 1), sourceHash(readFileSync(path))];
  }));
}
export function nativeDevelopmentSourceHashes(localNative) {
  need(typeof localNative === 'boolean', 'INVALID_ARGUMENTS');
  return developmentHashes(dirname(dirname(fileURLToPath(import.meta.url))), localNative);
}
function verifyPriorReservation(root, result, code, allowOverrun = false) {
  try {
    const phase = result.phase;
    need(['development', 'development-retry', 'development-finish'].includes(phase), code);
    const usage = join(root, `usage-${phase}`), state = readExecutionReservation(usage);
    const observationPath = join(usage, 'execution-observation.json');
    const observation = JSON.parse(read(observationPath));
    need((result.executionReservationMatched === true || allowOverrun && result.reservationBeforeNative === true && (state.overrun || !state.withinLimits))
      && state.settlementState === 'recorded' && (allowOverrun || !state.overrun && state.withinLimits)
      && JSON.stringify(state) === JSON.stringify(result.executionReservation)
      && state.basisHash === sourceHash(readFileSync(join(root, `budget${phase.slice('development'.length)}.json`)))
      && state.evidenceHash === sourceHash(readFileSync(observationPath))
      && observation.version === 1 && observation.phase === phase && observation.sessionId === result.sessionId
      && observation.ownerStopped === true && observation.ledgerMatched === result.ledgerMatched
      && observation.usageUnobservedAttempts === result.usageUnobservedAttempts, code);
    return { state, observation };
  } catch { need(false, code); }
}

export function readDevelopmentAccountingEvidence(root, phase) {
  const code = 'DEVELOPMENT_ACCOUNTING_INVALID', project = dirname(dirname(fileURLToPath(import.meta.url)));
  need(typeof root === 'string' && ['development', 'development-retry', 'development-finish'].includes(phase), code);
  root = resolve(root);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(root.slice(root.lastIndexOf('\\') + 1))
    && !lstatSync(root).isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase(), code);
  const suffix = phase.slice('development'.length), resultPath = join(root, `result${suffix}.json`);
  const result = JSON.parse(read(resultPath)), budget = JSON.parse(read(join(root, `budget${suffix}.json`)));
  need(result.root === root && result.phase === phase && budget.phase === phase
    && typeof result.localNative === 'boolean' && result.localNative === budget.localNative
    && result.model === budget.model && result.effort === budget.effort && result.taskId === budget.taskId
    && result.sourceUnchanged === true && result.ledgerMatched === true && result.attemptsComplete === true
    && result.recordedNativeStopped === true && Number.isSafeInteger(result.elapsedMs) && result.elapsedMs >= 0
    && JSON.stringify(budget.hashes) === JSON.stringify(developmentHashes(project, result.localNative)), code);
  const task = developmentTask(budget.taskId), control = join(root, 'control');
  need(budget.taskHash === sourceHash(task.task) && sourceHash(read(join(control, 'TASK.md'))) === budget.taskHash
    && budget.oracleHash === sourceHash(developmentOracleSource(budget.taskId))
    && sourceHash(read(join(control, 'oracle.mjs'))) === budget.oracleHash, code);
  const rows = read(join(root, `transport-${phase}.jsonl`)).trim().split('\n').filter(Boolean).map(JSON.parse);
  need(nativeClientsStopped(rows) && rows.filter(row => row.event === 'REQUEST_STARTED').length
    === rows.filter(row => row.event === 'REQUEST_SETTLED').length, code);
  const owner = JSON.parse(read(join(root, `process${suffix}.json`), 1024));
  need(Number.isSafeInteger(owner.pid) && owner.pid > 0 && owner.sessionId === result.sessionId, code);
  try { process.kill(owner.pid, 0); need(false, code); } catch (error) { need(error.code === 'ESRCH', code); }
  if (result.passed === true) verifyDevelopmentArtifacts(root, result, budget);
  const usage = readVerificationLedger(join(root, `usage-${phase}`, 'tool-usage.jsonl'));
  need(usage.version === 2 && usage.finalRecorded && !usage.truncatedTail
    && usage.requestAttempts === result.attempts && usage.inputTokens === result.inputTokens && usage.outputTokens === result.outputTokens
    && Number.isSafeInteger(result.usageUnobservedAttempts) && result.usageUnobservedAttempts >= 0
    && result.usageUnobservedAttempts === Math.max(0, usage.requestAttempts - usage.completions), code);
  const { observation } = verifyPriorReservation(root, result, code, true);
  const observed = { attempts: result.attempts, inputTokens: result.usageUnobservedAttempts === 0 ? usage.inputTokens : null,
    outputTokens: result.usageUnobservedAttempts === 0 ? usage.outputTokens : null, elapsedMs: result.elapsedMs };
  need(JSON.stringify(observation.observed) === JSON.stringify(observed), code);
  return { root, phase, model: result.model, effort: result.effort, taskId: result.taskId, localNative: result.localNative,
    observed, evidenceHash: sourceHash(readFileSync(resultPath)), passed: result.passed === true, failure: result.failure };
}
function developmentConfiguration(root, budget, owner, code) {
  const project = dirname(dirname(fileURLToPath(import.meta.url))), configRoot = resolve(budget.configRoot ?? join(root, 'config'));
  const origin = dirname(configRoot), info = lstatSync(configRoot);
  need(configRoot === join(origin, 'config') && dirname(origin).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(origin.slice(origin.lastIndexOf('\\') + 1))
    && info.isDirectory() && !info.isSymbolicLink() && realpathSync(configRoot).toLowerCase() === configRoot.toLowerCase(), code);
  const originBudget = JSON.parse(read(join(origin, 'budget.json'))), originOwner = JSON.parse(read(join(origin, 'process.json'), 1024));
  need(originBudget.model === budget.model && originBudget.localNative === budget.localNative && originOwner.sessionId === owner.sessionId, code);
  return configRoot;
}
export function classifyDevelopmentInterruption(events, taskId) {
  const code = 'INTERRUPTION_EVIDENCE_INVALID', task = developmentTask(taskId);
  need(Array.isArray(events) && events.length > 0 && events.every(row => row && typeof row === 'object' && !Array.isArray(row))
    && events[0].event === 'TASK_READ', code);
  const writes = events.filter(row => row.event === 'SOURCE_WRITTEN');
  if (writes.length) {
    need(writes.length === 1 && events.some(row => row.event === 'TESTS_EXECUTED' && row.passed === false
      && events.indexOf(row) < events.indexOf(writes[0])), code);
    return 'after-write';
  }
  const baselineHash = sourceHash(task.baseline);
  need(events.filter(row => row.event === 'TASK_READ').length === 1
    && events.filter(row => row.event === 'TASK_READ_WAIT').length <= 1
    && events.filter(row => row.event === 'TESTS_EXECUTED').length <= 1
    && events.every(row => ['TASK_READ', 'TASK_READ_WAIT'].includes(row.event) ? Object.keys(row).length === 1
      : row.event === 'TESTS_EXECUTED' && Object.keys(row).length === 5 && row.sha256 === baselineHash
        && row.passed === false && row.checks === task.checks && Array.isArray(row.failures) && row.failures.length > 0
        && row.failures.every(value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(value))), code);
  return 'before-write';
}
function interruptedDevelopmentState(root, model, localNative, accountHash, eventBytes) {
  const code = 'INTERRUPTION_EVIDENCE_INVALID', project = dirname(dirname(fileURLToPath(import.meta.url)));
  need(typeof root === 'string' && typeof localNative === 'boolean' && typeof model === 'string' && Object.hasOwn({ luna: 'max', sol: 'low' }, model)
    && typeof accountHash === 'string' && /^[a-f0-9]{64}$/.test(accountHash), code);
  root = resolve(root);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(root.slice(root.lastIndexOf('\\') + 1))
    && realpathSync(root).toLowerCase() === root.toLowerCase(), code);
  need(!existsSync(join(root, 'result.json')), 'INTERRUPTION_RESULT_PRESENT');
  const budget = JSON.parse(read(join(root, 'budget.json'))), owner = JSON.parse(read(join(root, 'process.json'), 1024));
  need(budget.phase === 'development' && budget.model === model && budget.localNative === localNative
    && budget.executionAccountHash === accountHash && budget.cutOutputAfterPass === false
    && JSON.stringify(budget.hashes) === JSON.stringify(developmentHashes(project, localNative))
    && Number.isSafeInteger(owner.pid) && owner.pid > 0 && /^[a-f0-9-]{36}$/.test(owner.sessionId), code);
  try { process.kill(owner.pid, 0); need(false, 'INTERRUPTION_OWNER_ACTIVE'); }
  catch (error) { need(error.code === 'ESRCH', 'INTERRUPTION_OWNER_ACTIVE'); }
  const transport = read(join(root, 'transport-development.jsonl'));
  const rows = transport.trim().split('\n').filter(Boolean).map(JSON.parse);
  need(nativeClientsStopped(rows), 'INTERRUPTION_OWNER_ACTIVE');
  const reservation = readExecutionReservation(join(root, 'usage-development'));
  need(reservation.basisHash === sourceHash(readFileSync(join(root, 'budget.json'))), code);
  const seen = rows.filter(row => row.event === 'RESERVATION_OBSERVED');
  need(seen.length === 1 && seen[0].reservationHash === reservation.reservationHash
    && rows.indexOf(seen[0]) < rows.findIndex(row => row.event === 'NATIVE_STARTED'), code);
  const task = developmentTask(budget.taskId), work = join(root, 'work'), control = join(root, 'control');
  need(budget.taskHash === sourceHash(task.task) && sourceHash(read(join(control, 'TASK.md'))) === budget.taskHash
    && sourceHash(read(join(work, '.mcp.json'))) === budget.mcpHash
    && sourceHash(read(join(control, 'oracle.mjs'))) === budget.oracleHash
    && budget.oracleHash === sourceHash(developmentOracleSource(budget.taskId)), code);
  const source = read(join(work, task.sourceFile)); checkDevelopmentSource(source, budget.taskId);
  const eventsPath = join(work, 'events.jsonl'); read(eventsPath, 32768);
  const currentSourceHash = sourceHash(source), allEvents = readFileSync(eventsPath);
  const fullText = new TextDecoder('utf-8', { fatal: true }).decode(allEvents);
  need(allEvents.length <= 32768 && fullText.endsWith('\n'), code);
  const fullWrites = fullText.trim().split('\n').map(JSON.parse).filter(row => row.event === 'SOURCE_WRITTEN');
  const length = eventBytes ?? allEvents.length;
  need(Number.isSafeInteger(length) && length > 0 && length <= allEvents.length, code);
  const prefix = allEvents.subarray(0, length), text = new TextDecoder('utf-8', { fatal: true }).decode(prefix);
  need(text.endsWith('\n'), code);
  const events = text.trim().split('\n').map(JSON.parse), writes = events.filter(row => row.event === 'SOURCE_WRITTEN');
  const stage = classifyDevelopmentInterruption(events, budget.taskId);
  const sha256 = stage === 'before-write' ? sourceHash(task.baseline) : currentSourceHash;
  if (stage === 'before-write') {
    if (existsSync(join(root, 'result-retry.json'))) {
      // The immutable prefix describes the original baseline. A changed current
      // source is accepted only through a completed retry bound to that proof.
      need(eventBytes !== undefined, code);
      const retryBudget = JSON.parse(read(join(root, 'budget-retry.json')));
      const retryResult = JSON.parse(read(join(root, 'result-retry.json')));
      const completion = readDevelopmentAccountingEvidence(root, 'development-retry');
      need(completion.passed && completion.model === model && completion.localNative === localNative && completion.taskId === budget.taskId
        && retryResult.sessionId === owner.sessionId && retryResult.independentPassed === true
        && retryBudget.configRoot === (budget.configRoot ?? join(root, 'config'))
        && retryResult.sourceSha256 === currentSourceHash && fullWrites.length === 1 && fullWrites[0].sha256 === currentSourceHash
        && retryBudget.recoveredFromInterruption === true && retryBudget.recoveryStage === stage
        && retryBudget.recoveryProofHash === sourceHash(readFileSync(join(root, 'interruption-evidence.json'))), code);
    } else need(currentSourceHash === sha256 && fullWrites.length === 0 && allEvents.length === length, code);
  } else need(fullWrites.length === 1 && writes[0].sha256 === sha256, code);
  const usagePath = join(root, 'usage-development', 'tool-usage.jsonl'), usage = readVerificationLedger(usagePath);
  need(usage.version === 2 && usage.requestAttempts <= budget.requestLimit
    && usage.inputTokens <= budget.maxObservedInputTokens && usage.outputTokens <= budget.maxObservedOutputTokens,
  'INTERRUPTION_USAGE_INVALID');
  const configRoot = developmentConfiguration(root, budget, owner, code);
  const stopPath = join(root, 'interruption-stop-result.json'), stopRecord = JSON.parse(read(stopPath, 8192));
  need(stopRecord.version === 1 && stopRecord.managerPid === budget.managerPid
    && stopRecord.budgetHash === sourceHash(readFileSync(join(root, 'budget.json'))) && stopRecord.tree?.stopped === true, code);
  const snapshot = { root, model, localNative, taskId: budget.taskId, phase: 'development', sessionId: owner.sessionId, stage,
    accountHash, budgetHash: sourceHash(readFileSync(join(root, 'budget.json'))), sourceHash: sha256,
    eventsHash: sourceHash(prefix), eventBytes: length, transportHash: sourceHash(transport),
    usageHash: sourceHash(readFileSync(usagePath)), ownerHash: sourceHash(readFileSync(join(root, 'process.json'))), configRoot,
    stopHash: sourceHash(readFileSync(stopPath)) };
  return { project, root, taskId: budget.taskId, task, budget, owner, work, control, source, currentSourceHash, stage, snapshot };
}

export function prepareDevelopmentInterruption(root, { model, localNative, accountHash, powershell, managerPid }) {
  const project = dirname(dirname(fileURLToPath(import.meta.url)));
  need(typeof root === 'string' && typeof powershell === 'string' && powershell.endsWith('pwsh.exe')
    && Number.isSafeInteger(managerPid) && managerPid > 0, 'INTERRUPTION_ARGUMENTS');
  root = resolve(root);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(root.slice(root.lastIndexOf('\\') + 1))
    && realpathSync(root).toLowerCase() === root.toLowerCase(), 'INTERRUPTION_ROOT_INVALID');
  const budgetPath = join(root, 'budget.json'), budget = JSON.parse(read(budgetPath));
  need(budget.managerPid === managerPid && budget.model === model && budget.localNative === localNative
    && budget.executionAccountHash === accountHash && budget.phase === 'development'
    && JSON.stringify(budget.hashes) === JSON.stringify(developmentHashes(project, localNative)), 'INTERRUPTION_EVIDENCE_INVALID');
  need(!existsSync(join(root, 'result.json')), 'INTERRUPTION_RESULT_PRESENT');
  try { process.kill(managerPid, 0); need(false, 'INTERRUPTION_MANAGER_ACTIVE'); }
  catch (error) { need(error.code === 'ESRCH', 'INTERRUPTION_MANAGER_ACTIVE'); }
  need(readExecutionReservation(join(root, 'usage-development')).basisHash === sourceHash(readFileSync(budgetPath)), 'INTERRUPTION_EVIDENCE_INVALID');
  const owner = JSON.parse(read(join(root, 'process.json'), 1024));
  const stopIntent = join(root, 'interruption-stop-intent.json'), stopResult = join(root, 'interruption-stop-result.json');
  const stopping = !existsSync(stopResult);
  if (stopping) {
    need(!existsSync(stopIntent), 'INTERRUPTION_STOP_UNVERIFIED');
    write(stopIntent, { managerPid, budgetHash: sourceHash(readFileSync(budgetPath)), ownerHash: sourceHash(readFileSync(join(root, 'process.json'))) });
  }
  const helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
  const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
  const check = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(owner.pid),
    '-StartedAfterMs', String(owner.startedAt), '-RunRoot', root, ...(stopping ? ['-Stop'] : [])],
  { cwd: project, env, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
  need(!check.error && check.status === 0 && check.stderr === '' && JSON.parse(check.stdout).stopped === true, 'INTERRUPTION_OWNER_UNVERIFIED');
  if (stopping) write(stopResult, { version: 1, managerPid, budgetHash: sourceHash(readFileSync(budgetPath)), tree: JSON.parse(check.stdout) });
  const state = interruptedDevelopmentState(root, model, localNative, accountHash);
  if (localNative && state.stage === 'after-write') {
    need(state.source === publicDevelopmentSource(state.taskId), 'INTERRUPTION_LOCAL_SOURCE_INVALID');
    writeFileSync(join(state.control, 'review.json'), JSON.stringify({ sha256: state.snapshot.sourceHash, approved: true }), { flush: true });
  }
  const review = JSON.parse(read(join(state.control, 'review.json'), 1024));
  need(review.sha256 === state.snapshot.sourceHash && review.approved === true, 'INTERRUPTION_SOURCE_REVIEW_REQUIRED');
  const oracle = join(state.control, 'oracle.mjs'), source = join(state.work, state.task.sourceFile);
  const tested = spawnSync(process.execPath, ['--permission', `--allow-fs-read=${oracle}`, `--allow-fs-read=${source}`, oracle, source],
    { cwd: state.work, env, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 16384 });
  let verdict; try { verdict = JSON.parse(tested.stdout); } catch { }
  const afterWrite = state.stage === 'after-write';
  need(!tested.error && tested.status === (afterWrite ? 0 : 1) && tested.stderr === '' && verdict?.passed === afterWrite
    && verdict.checks === state.task.checks && Array.isArray(verdict.failures)
    && (afterWrite ? verdict.failures.length === 0 : verdict.failures.length > 0), 'INTERRUPTION_SOURCE_TESTS_FAILED');
  const proof = { version: 2, kind: 'manager-interruption', ...state.snapshot, originalResultAbsent: true, taskCompleted: false,
    sourceReviewed: true, independentPassed: afterWrite, observed: { attempts: null, inputTokens: null, outputTokens: null, elapsedMs: null } };
  write(join(state.root, 'interruption-evidence.json'), proof);
  return readDevelopmentInterruption(state.root);
}

export function readDevelopmentInterruption(root) {
  const project = dirname(dirname(fileURLToPath(import.meta.url)));
  need(typeof root === 'string' && dirname(resolve(root)).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(resolve(root).slice(resolve(root).lastIndexOf('\\') + 1)), 'INTERRUPTION_ROOT_INVALID');
  root = resolve(root);
  const path = join(root, 'interruption-evidence.json'), proof = JSON.parse(read(path, 8192));
  need(proof && typeof proof === 'object' && !Array.isArray(proof) && Object.keys(proof).length === 24
    && proof.version === 2 && proof.kind === 'manager-interruption' && proof.originalResultAbsent === true
    && ['before-write', 'after-write'].includes(proof.stage) && proof.independentPassed === (proof.stage === 'after-write')
    && proof.taskCompleted === false && proof.sourceReviewed === true
    && JSON.stringify(proof.observed) === JSON.stringify({ attempts: null, inputTokens: null, outputTokens: null, elapsedMs: null }),
  'INTERRUPTION_RECORD_INVALID');
  const state = interruptedDevelopmentState(root, proof.model, proof.localNative, proof.accountHash, proof.eventBytes);
  need(Object.entries(state.snapshot).every(([key, value]) => proof[key] === value), 'INTERRUPTION_RECORD_CHANGED');
  const review = JSON.parse(read(join(state.control, 'review.json'), 1024));
  need(review.sha256 === state.currentSourceHash && review.approved === true, 'INTERRUPTION_SOURCE_REVIEW_REQUIRED');
  return { ...state, evidence: { root: state.root, phase: 'development', model: proof.model, effort: state.budget.effort,
    taskId: proof.taskId, localNative: proof.localNative, observed: proof.observed, evidenceHash: sourceHash(readFileSync(path)),
    passed: false, failure: 'MANAGER_INTERRUPTED', sourceHash: proof.sourceHash, stage: proof.stage },
  fixture: { project: state.project, root: state.root, taskId: state.taskId, task: state.task, work: state.work, control: state.control,
    script: join(state.project, 'verification', 'fixtures', 'development-mcp.mjs'), oracleHash: state.budget.oracleHash,
    taskHash: state.budget.taskHash, priorOwner: state.owner, priorSourceHash: proof.sourceHash, configRoot: state.snapshot.configRoot,
    minimumPrior: { priorAttempts: state.budget.priorAttempts + state.budget.requestLimit,
      priorInputTokens: state.budget.priorInputTokens + state.budget.maxObservedInputTokens,
      priorOutputTokens: state.budget.priorOutputTokens + state.budget.maxObservedOutputTokens,
      priorElapsedMs: state.budget.priorElapsedMs + state.budget.phaseMs + 30000 } } };
}
function completedDevelopmentContext(root, model, localNative, taskId) {
  const project = dirname(dirname(fileURLToPath(import.meta.url))), canonical = resolve(root);
  need(dirname(canonical).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(canonical.slice(canonical.lastIndexOf('\\') + 1))
    && realpathSync(canonical).toLowerCase() === canonical.toLowerCase(), 'CONTINUATION_ROOT_INVALID');
  const suffix = existsSync(join(canonical, 'result-finish.json')) ? '-finish' : existsSync(join(canonical, 'result-retry.json')) ? '-retry' : '';
  const phase = `development${suffix}`;
  const budget = JSON.parse(read(join(canonical, `budget${suffix}.json`))), result = JSON.parse(read(join(canonical, `result${suffix}.json`)));
  const owner = JSON.parse(read(join(canonical, `process${suffix}.json`))), previousTask = developmentTask(budget.taskId);
  need(budget.model === model && budget.localNative === localNative && budget.taskId !== taskId
    && budget.cutOutputAfterPass === false && budget.phase === phase && result.phase === phase && result.root === canonical
    && result.model === model && result.taskId === budget.taskId && result.localNative === localNative && result.passed === true
    && result.failure === null && result.tree?.stopped === true && result.recordedNativeStopped === true
    && result.sourceUnchanged === true && result.independentPassed === true && result.ledgerMatched === true
    && result.completedUsageObserved === true && result.usageUnobservedAttempts === 0
    && result.cleanupComplete === true && result.output?.resultCount === 1 && result.output?.statusCount === 1
    && result.output?.stdoutEnded === true && result.output?.stderrEnded === true
    && result.sessionId === owner.sessionId && /^[0-9a-f-]{36}$/.test(owner.sessionId), 'CONTINUATION_EVIDENCE_INVALID');
  need(!existsSync(join(canonical, 'successor-intent.json')), 'CONTINUATION_ALREADY_STARTED');
  need(JSON.stringify(budget.hashes) === JSON.stringify(developmentHashes(project, localNative)), 'CONTINUATION_SOURCE_CHANGED');
  verifyDevelopmentArtifacts(canonical, result, budget);
  const source = read(join(canonical, 'work', previousTask.sourceFile));
  checkDevelopmentSource(source, budget.taskId);
  const review = JSON.parse(read(join(canonical, 'control', 'review.json'), 1024));
  need(sourceHash(source) === result.sourceSha256 && review.sha256 === result.sourceSha256 && review.approved === true
    && sourceHash(read(join(canonical, 'work', '.mcp.json'))) === budget.mcpHash
    && sourceHash(read(join(canonical, 'control', 'TASK.md'))) === budget.taskHash && budget.taskHash === sourceHash(previousTask.task)
    && sourceHash(read(join(canonical, 'control', 'oracle.mjs'))) === budget.oracleHash
    && budget.oracleHash === sourceHash(developmentOracleSource(budget.taskId)), 'CONTINUATION_ARTIFACT_CHANGED');
  const usage = readVerificationLedger(join(canonical, `usage-${phase}`, 'tool-usage.jsonl'));
  need(usage.version === 2 && usage.finalRecorded && !usage.truncatedTail && usage.requestAttempts === result.attempts
    && usage.unobservedCompletions === 0 && usage.completions === usage.requestAttempts
    && usage.inputTokens === result.inputTokens && usage.outputTokens === result.outputTokens, 'CONTINUATION_LEDGER_INVALID');
  need(nativeClientsStopped(read(join(canonical, `transport-${phase}.jsonl`)).trim().split('\n').filter(Boolean).map(JSON.parse)), 'CONTINUATION_OWNER_UNVERIFIED');
  verifyPriorReservation(canonical, result, 'CONTINUATION_RESERVATION_INVALID');
  const configRoot = developmentConfiguration(canonical, budget, owner, 'CONTINUATION_CONFIG_INVALID');
  const minimumPrior = { priorAttempts: result.cumulativeAttempts, priorElapsedMs: result.cumulativeElapsedMs,
    priorInputTokens: result.cumulativeInputTokens, priorOutputTokens: result.cumulativeOutputTokens };
  need(Object.values(minimumPrior).every(value => Number.isSafeInteger(value) && value >= 0), 'CONTINUATION_LEDGER_INVALID');
  return { root: canonical, owner, configRoot, previousTaskId: budget.taskId, sourceSha256: result.sourceSha256, minimumPrior };
}
function resumeDevelopmentFixture(root, model, localNative, taskId, incomplete = false) {
  const task = developmentTask(taskId);
  const project = dirname(dirname(fileURLToPath(import.meta.url))), canonical = resolve(root);
  need(dirname(canonical).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^native-development-[A-Za-z0-9]{6}$/.test(canonical.slice(canonical.lastIndexOf('\\') + 1))
    && realpathSync(canonical).toLowerCase() === canonical.toLowerCase(), 'RESUME_ROOT_INVALID');
  const priorBudget = JSON.parse(read(join(root, 'budget.json'))), first = JSON.parse(read(join(root, 'result.json')));
  const owner = JSON.parse(read(join(root, 'process.json')));
  need(priorBudget.model === model && priorBudget.localNative === localNative && priorBudget.taskId === taskId
    && (incomplete ? priorBudget.cutOutputAfterPass === false && priorBudget.continuedFrom === undefined
      && first.root === canonical && first.model === model && first.localNative === localNative && first.taskId === taskId
      && first.phase === 'development' && first.sessionId === owner.sessionId && first.sourceUnchanged === true
      && first.recordedNativeStopped === true
      && first.taskIncompleteBeforeEffect === true && first.failure === 'DEVELOPMENT_TASK_INCOMPLETE'
      && first.nativeJson === true && first.nativeError === false && first.exitCode === 0 && first.cleanupComplete === true
      && first.output?.resultCount === 1 && first.output?.statusCount === 1 && first.output?.stdoutEnded === true && first.output?.stderrEnded === true
      && first.ledgerMatched === true && first.attemptsComplete === true && first.usageUnobservedAttempts === 0
      : priorBudget.cutOutputAfterPass === true && first.outputCutObserved === true)
    && first.passed === false && first.tree?.stopped === true
    && /^[0-9a-f-]{36}$/.test(owner.sessionId), 'RESUME_EVIDENCE_INVALID');
  need(nativeClientsStopped(read(join(root, 'transport-development.jsonl')).trim().split('\n').filter(Boolean).map(JSON.parse)), 'RESUME_OWNER_UNVERIFIED');
  need(JSON.stringify(priorBudget.hashes) === JSON.stringify(developmentHashes(project, localNative)), 'RESUME_SOURCE_CHANGED');
  need(priorBudget.taskHash === sourceHash(task.task)
    && priorBudget.oracleHash === sourceHash(developmentOracleSource(taskId)), 'RESUME_ARTIFACT_CHANGED');
  need(sourceHash(read(join(root, 'work', task.sourceFile))) === first.sourceSha256
    && sourceHash(read(join(root, 'work', '.mcp.json'))) === priorBudget.mcpHash
    && sourceHash(read(join(root, 'control', 'oracle.mjs'))) === priorBudget.oracleHash
    && sourceHash(read(join(root, 'control', 'TASK.md'))) === priorBudget.taskHash, 'RESUME_ARTIFACT_CHANGED');
  checkDevelopmentSource(read(join(root, 'work', task.sourceFile)), taskId);
  verifyPriorReservation(root, first, 'RESUME_RESERVATION_INVALID');
  if (incomplete) {
    const events = read(join(root, 'work', 'events.jsonl')).trim().split('\n').filter(Boolean).map(JSON.parse);
    const review = JSON.parse(read(join(root, 'control', 'review.json'), 1024));
    const usage = readVerificationLedger(join(root, 'usage-development', 'tool-usage.jsonl'));
    need(events.length === 1 && events[0].event === 'TASK_READ' && Object.keys(events[0]).length === 1
      && first.sourceSha256 === sourceHash(task.baseline) && review.sha256 === first.sourceSha256 && review.approved === true
      && usage.version === 2 && usage.finalRecorded && !usage.truncatedTail && usage.requestAttempts === first.attempts
      && usage.completions === usage.requestAttempts && usage.unobservedCompletions === 0
      && usage.inputTokens === first.inputTokens && usage.outputTokens === first.outputTokens, 'RESUME_EVIDENCE_INVALID');
    for (const [priorKey, current, cumulativeKey] of [['priorAttempts', usage.requestAttempts, 'cumulativeAttempts'],
      ['priorElapsedMs', first.elapsedMs, 'cumulativeElapsedMs'], ['priorInputTokens', usage.inputTokens, 'cumulativeInputTokens'],
      ['priorOutputTokens', usage.outputTokens, 'cumulativeOutputTokens']]) {
      need(Number.isSafeInteger(priorBudget[priorKey]) && priorBudget[priorKey] >= 0 && Number.isSafeInteger(current) && current >= 0
        && Number.isSafeInteger(priorBudget[priorKey] + current) && first[cumulativeKey] === priorBudget[priorKey] + current,
      'RESUME_USAGE_REGRESSION');
    }
  }
  return { project, root, taskId, task, work: join(root, 'work'), control: join(root, 'control'),
    configRoot: developmentConfiguration(root, priorBudget, owner, 'RESUME_CONFIG_INVALID'),
    script: join(project, 'verification', 'fixtures', 'development-mcp.mjs'), oracleHash: priorBudget.oracleHash,
    taskHash: priorBudget.taskHash, priorOwner: owner, priorSourceHash: first.sourceSha256,
    ...(incomplete ? { minimumPrior: { priorAttempts: first.cumulativeAttempts, priorElapsedMs: first.cumulativeElapsedMs,
      priorInputTokens: first.cumulativeInputTokens, priorOutputTokens: first.cumulativeOutputTokens } } : {}) };
}
export async function verifyNativeDevelopment({ model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
  localNative = false, cutOutputAfterPass = false, resumeRoot, taskId = DEFAULT_DEVELOPMENT_TASK_ID, continueFrom, requestLimit = 16,
  earlyExitAfterRead = false, resumeIncompleteRoot, onReservation, executionAccountHash, resumeInterruptedRoot, holdAfterSourceWrite = false, holdAfterTaskRead = false }) {
  const effort = { luna: 'max', sol: 'low' }[model];
  need(typeof model === 'string' && Object.hasOwn({ luna: 'max', sol: 'low' }, model)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe') && typeof localNative === 'boolean'
    && typeof cutOutputAfterPass === 'boolean' && Number.isSafeInteger(requestLimit) && requestLimit >= 1 && requestLimit <= 16
    && typeof earlyExitAfterRead === 'boolean'
    && typeof holdAfterSourceWrite === 'boolean' && typeof holdAfterTaskRead === 'boolean' && !(holdAfterSourceWrite && holdAfterTaskRead)
    && (!(holdAfterSourceWrite || holdAfterTaskRead) || localNative && !cutOutputAfterPass && !earlyExitAfterRead
      && resumeRoot === undefined && resumeIncompleteRoot === undefined && resumeInterruptedRoot === undefined && continueFrom === undefined)
    && (onReservation === undefined || typeof onReservation === 'function')
    && (executionAccountHash === undefined || typeof executionAccountHash === 'string' && /^[a-f0-9]{64}$/.test(executionAccountHash))
    && (resumeInterruptedRoot === undefined || typeof resumeInterruptedRoot === 'string' && resumeRoot === undefined
      && resumeIncompleteRoot === undefined && continueFrom === undefined && !cutOutputAfterPass && !earlyExitAfterRead)
    && (!earlyExitAfterRead || localNative === true && !cutOutputAfterPass && resumeRoot === undefined && resumeIncompleteRoot === undefined && continueFrom === undefined)
    && (resumeRoot === undefined || typeof resumeRoot === 'string' && !cutOutputAfterPass)
    && (resumeIncompleteRoot === undefined || typeof resumeIncompleteRoot === 'string' && resumeRoot === undefined && !cutOutputAfterPass)
    && (continueFrom === undefined || typeof continueFrom === 'string' && resumeRoot === undefined && resumeIncompleteRoot === undefined && !cutOutputAfterPass), 'INVALID_ARGUMENTS');
  try { developmentTask(taskId); } catch { need(false, 'INVALID_ARGUMENTS'); }
  if (localNative) publicDevelopmentSource(taskId);
  for (const value of [priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens]) need(Number.isSafeInteger(value) && value >= 0, 'INVALID_PRIOR_USAGE');
  const recovering = resumeInterruptedRoot !== undefined;
  const interrupted = recovering ? readDevelopmentInterruption(resumeInterruptedRoot) : null;
  const beforeWrite = interrupted?.stage === 'before-write';
  const finishing = resumeRoot !== undefined || recovering && !beforeWrite, retrying = resumeIncompleteRoot !== undefined || beforeWrite, resuming = finishing || retrying;
  const suffix = finishing ? '-finish' : retrying ? '-retry' : '', phase = `development${suffix}`;
  const context = continueFrom === undefined ? null : completedDevelopmentContext(continueFrom, model, localNative, taskId);
  if (context) {
    const supplied = { priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens };
    need(Object.entries(context.minimumPrior).every(([key, value]) => supplied[key] >= value), 'CONTINUATION_USAGE_REGRESSION');
  }
  if (interrupted) need(interrupted.evidence.model === model && interrupted.evidence.localNative === localNative
    && interrupted.evidence.taskId === taskId, 'RESUME_INTERRUPTION_INVALID');
  const fixture = interrupted?.fixture ?? (resuming ? resumeDevelopmentFixture(resumeRoot ?? resumeIncompleteRoot, model, localNative, taskId, retrying)
    : createDevelopmentFixture({ holdAfterPass: cutOutputAfterPass, holdAfterSourceWrite, holdAfterTaskRead, taskId }));
  const { project, root, work, control, task } = fixture;
  if (retrying || recovering) {
    const supplied = { priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens };
    need(Object.entries(fixture.minimumPrior).every(([key, value]) => Number.isSafeInteger(value) && value >= 0
      && supplied[key] >= value), 'RESUME_USAGE_REGRESSION');
  }
  const entry = join(project, 'verification', localNative ? 'native-development-local-entry.mjs' : 'native-development-entry.mjs'), helper = join(project, 'verification', 'stop-owned-native-tree.ps1');
  const phaseMs = localNative ? 30000 : finishing ? 120000 : 600000;
  const configRoot = context?.configRoot ?? fixture.configRoot ?? join(root, 'config');
  const budget = { model, effort, taskId, localNative, phase, cutOutputAfterPass, earlyExitAfterRead, phaseMs, requestLimit, maxTurns: 8, maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768,
    outputBytes: 1048576, mainConcurrency: 1, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens,
    cumulativeReservedAttempts: priorAttempts + requestLimit, cumulativeReservedMs: priorElapsedMs + phaseMs,
    cumulativeObservedInputLimit: priorInputTokens + 131072, cumulativeObservedOutputLimit: priorOutputTokens + 32768,
    basis: 'The initial eight-turn code task permits up to sixteen attempts and ten minutes, including source review. An explicitly smaller requestLimit reduces both the reservation and enforced transport budget; it never raises a cumulative cap.',
    oracleHash: fixture.oracleHash, taskHash: fixture.taskHash, mcpHash: sourceHash(read(join(work, '.mcp.json'))),
    ...(context ? { continuedFrom: context.root, previousTaskId: context.previousTaskId, previousSourceSha256: context.sourceSha256 } : {}),
    ...(executionAccountHash === undefined ? {} : { executionAccountHash }), configRoot, managerPid: process.pid, recoveredFromInterruption: recovering,
    ...(interrupted ? { recoveryStage: interrupted.stage, recoveryProofHash: interrupted.evidence.evidenceHash } : {}),
    hashes: developmentHashes(project, localNative) };
  const env = nativeVerificationEnvironment(root);
  env.CLAUDE_CONFIG_DIR = configRoot;
  const controlEnv = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => env[key]).map(key => [key, env[key]]));
  if (resuming) {
    const owner = fixture.priorOwner;
    const previous = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(owner.pid),
      '-StartedAfterMs', String(owner.startedAt), '-RunRoot', root],
      { cwd: project, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!previous.error && previous.status === 0 && JSON.parse(previous.stdout).stopped === true, 'RESUME_OWNER_UNVERIFIED');
    // Exclusive, task-local intent is created before any new child or effect.
    write(join(root, finishing ? 'finish-intent.json' : 'retry-intent.json'), { sessionId: owner.sessionId, sourceSha256: fixture.priorSourceHash });
    if (finishing) {
      const seal = join(control, 'source-sealed.json'), expected = { sha256: fixture.priorSourceHash };
      if (existsSync(seal)) need(JSON.stringify(JSON.parse(read(seal, 1024))) === JSON.stringify(expected), 'SOURCE_SEAL_INVALID');
      else write(seal, expected);
    }
  }
  if (context) {
    const owner = context.owner;
    const previous = spawnSync(powershell, ['-NoProfile', '-NonInteractive', '-File', helper, '-RootPid', String(owner.pid),
      '-StartedAfterMs', String(owner.startedAt), '-RunRoot', context.root],
      { cwd: project, env: controlEnv, windowsHide: true, encoding: 'utf8', timeout: 12000, maxBuffer: 32768 });
    need(!previous.error && previous.status === 0 && JSON.parse(previous.stdout).stopped === true, 'CONTINUATION_OWNER_UNVERIFIED');
    // New fixture files exist for inspection; only one successor may start a
    // native process. An interrupted handoff is not retried as a new task.
    write(join(context.root, 'successor-intent.json'), { root, taskId, sessionId: owner.sessionId, previousSourceSha256: context.sourceSha256 });
  }
  write(join(root, `budget${suffix}.json`), budget);
  writeFileSync(join(root, `transport-${phase}.jsonl`), '', { flag: 'wx' }); mkdirSync(join(root, `usage-${phase}`));
  // This reservation covers this phase only; the outer manager retains all
  // earlier cumulative reservations. Cleanup has its own bounded allowance.
  const reservationRoot = join(root, `usage-${phase}`);
  const phaseAllowance = { attempts: requestLimit, inputTokens: budget.maxObservedInputTokens,
    outputTokens: budget.maxObservedOutputTokens, elapsedMs: phaseMs + 30000 };
  const reservationProof = createExecutionReservation(reservationRoot, { basisHash: sourceHash(readFileSync(join(root, `budget${suffix}.json`))),
    previous: { attempts: 0, inputTokens: 0, outputTokens: 0, elapsedMs: 0 }, limits: phaseAllowance, allowance: phaseAllowance });
  need(onReservation?.(Object.freeze({ root, phase, budgetHash: reservationProof.basisHash,
    reservationHash: reservationProof.reservationHash })) === undefined, 'INVALID_RESERVATION_OBSERVER');
  const sessionId = resuming ? fixture.priorOwner.sessionId : context ? context.owner.sessionId : randomUUID(), phaseStart = Date.now();
  const args = ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', String(requestLimit), '-p',
    '--output-format', 'stream-json', '--verbose', '--tools', '', '--allowedTools',
    finishing ? 'mcp__fixture__read_task,mcp__fixture__run_tests' : 'mcp__fixture__read_task,mcp__fixture__write_source,mcp__fixture__run_tests',
    '--max-turns', '8', resuming || context ? '--resume' : '--session-id', sessionId, '--', finishing
    ? 'Continue the same public development task after the output receiver lost the final result. The outer manager verified that your source was written once, reviewed and passed the fixed tests. Do not modify it. Read the original requirements and current source once with mcp__fixture__read_task, run mcp__fixture__run_tests once, and after success reply exactly CLAUDUCT_DEVELOPMENT_DONE. Use one tool per response. The original baseline/fix instructions are already fulfilled; only verification and this final report remain.'
    : (retrying ? (beforeWrite
      ? 'Continue the same public task after its manager stopped. The outer manager verified that the source remains the original baseline and the previous owner stopped. '
      : 'Continue the same incomplete public task. Your previous response ended after reading the requirements. The outer manager verified that no baseline test or source write occurred and that the previous owner stopped. ')
      + 'Read the requirements once in this execution, then run the baseline tests, write the fix and run the tests. Continue through these tool calls without stopping at a readiness message. '
      : context ? 'The previous public development task is complete. Start the different task in this new working directory; its fixture tools, TASK.md and source are separate from the previous task. ' : '') +
    'Read the complete TASK.md with mcp__fixture__read_task once and implement its bounded code change. Run the prescribed tests on the baseline, fix the code, then run the tests again. Use one fixture tool per response; the three MCP tools are already available. Follow the task completion marker only after tests pass.'];
  const child = fork(entry, [root, phase, ...args], { cwd: work, env, windowsHide: true, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  const capture = createNativeOutputCapture({ maxBytes: budget.outputBytes, maxLineBytes: budget.outputBytes, maxRecords: 4096 });
  let finished = false, exitCode = null, failure = null, treeEvidence = null, treeStopAttempted = false, outputCut = false, cutReleased = false;
  const stopped = new Promise(done => { child.once('error', () => { failure = 'PROCESS_START_FAILED'; }); child.once('close', code => { exitCode = code; finished = true; done(); }); });
  capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
  write(join(root, `process${suffix}.json`), { pid: child.pid, startedAt: phaseStart, sessionId });
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
      // A finishing phase uses the already reviewed, sealed source. Its work
      // directory can still contain the earlier baseline's review request.
      if (!finishing && existsSync(reviewPath)) {
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
    failure = ['TREE_STOP_UNVERIFIED', 'PROCESS_STOP_UNVERIFIED', 'ORPHAN_PROCESS', 'LOCAL_SOURCE_MISMATCH'].includes(error.message) ? error.message : 'DEVELOPMENT_EXECUTION_FAILED';
    // A failed stop is not silently retried through another mechanism.
    if (!finished && !treeStopAttempted && failure !== 'TREE_STOP_UNVERIFIED') {
      try { treeEvidence = tree(true); } catch { failure = 'TREE_STOP_UNVERIFIED'; }
    }
  }
  const output = capture.snapshot(), result = output.nativeResult, status = output.status;
  failure ??= output.evidence.failure;
  const requestDiagnostics = nativeOutputDiagnostics(output);
  failure ??= requestDiagnostics.failure;
  const rows = read(join(root, `transport-${phase}.jsonl`)).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const contexts = rows.filter(row => row.event === 'CONTEXT_OBSERVED');
  const contextMatched = !context || contexts.length === 1 && contexts[0].previousTaskId === context.previousTaskId
    && contexts[0].previousFunctionObserved === true && contexts[0].previousCompletionObserved === true;
  const recordedNativeStopped = nativeClientsStopped(rows);
  const requests = rows.filter(row => row.event === 'REQUEST_STARTED'), settled = rows.filter(row => row.event === 'REQUEST_SETTLED');
  const reservations = rows.filter(row => row.event === 'RESERVATION_OBSERVED');
  const reservationBeforeNative = reservations.length === 1 && reservations[0].reservationHash === reservationProof.reservationHash
    && JSON.stringify(reservations[0].charged) === JSON.stringify(phaseAllowance)
    && rows.indexOf(reservations[0]) < rows.findIndex(row => row.event === 'NATIVE_STARTED');
  const attempts = Math.max(0, ...settled.map(row => row.attempts));
  const routeMatched = requests.length > 0 && requests.every(row => row.model === `gpt-5.6-${model}` && row.effort === effort);
  const inputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.input_tokens ?? 0), 0);
  const outputTokens = rows.filter(row => row.event === 'USAGE').reduce((sum, row) => sum + (row.usage.output_tokens ?? 0), 0);
  let usageLedger = null;
  try { usageLedger = readVerificationLedger(join(root, `usage-${phase}`, 'tool-usage.jsonl')); }
  catch { failure ??= 'VERIFICATION_LEDGER_INVALID'; }
  const ledgerMatched = usageLedger?.finalRecorded === true && usageLedger.truncatedTail === false
    && usageLedger.requestAttempts === attempts && usageLedger.inputTokens === inputTokens && usageLedger.outputTokens === outputTokens
    && usageLedger.completions === rows.filter(row => row.event === 'USAGE').length
    && usageLedger.unobservedCompletions === rows.filter(row => row.event === 'USAGE_UNOBSERVED').length;
  const completedUsageObserved = usageLedger?.version === 2 && usageLedger.unobservedCompletions === 0;
  const usageUnobservedAttempts = usageLedger ? usageLedger.requestAttempts - usageLedger.completions : null;
  let rateLimits = null;
  const limitRows = rows.filter(row => row.event === 'RESPONSE_LIMITS');
  try { rateLimits = rateLimitEvidence(limitRows); }
  catch { failure ??= 'RATE_LIMIT_EVIDENCE_INVALID'; }
  const rateLimitsMatched = rateLimits !== null && usageLedger !== null
    && rateLimits.responseCount >= usageLedger.completions + usageLedger.unobservedCompletions
    && rateLimits.responseCount <= usageLedger.requestAttempts
    && limitRows.every(row => row.requestAttempt <= usageLedger.requestAttempts);
  if (usageLedger?.unobservedCompletions > 0) failure ??= 'INVALID_USAGE';
  const sourceUnchanged = Object.entries(budget.hashes).every(([path, hash]) => sourceHash(readFileSync(join(project, path))) === hash);
  const eventsText = existsSync(join(work, 'events.jsonl')) ? read(join(work, 'events.jsonl')) : '';
  const events = eventsText.trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
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
  if (!contextMatched) failure ??= 'CONTINUATION_CONTEXT_MISSING';
  const elapsedMs = Date.now() - phaseStart;
  const ownerStopped = finished && treeEvidence?.stopped === true && recordedNativeStopped;
  const attemptsObserved = ownerStopped && ledgerMatched && usageLedger.version === 2 && requests.length === settled.length;
  const tokensObserved = attemptsObserved && completedUsageObserved && usageUnobservedAttempts === 0;
  const observation = { version: 1, phase, sessionId, ownerStopped: ownerStopped === true,
    ledgerMatched, usageUnobservedAttempts, observed: { attempts: attemptsObserved ? attempts : null,
      inputTokens: tokensObserved ? inputTokens : null, outputTokens: tokensObserved ? outputTokens : null,
      elapsedMs: ownerStopped ? elapsedMs : null } };
  const observationPath = join(reservationRoot, 'execution-observation.json');
  write(observationPath, observation);
  const observationHash = sourceHash(readFileSync(observationPath));
  const executionReservation = settleExecutionReservation(reservationRoot,
    { observed: observation.observed, evidenceHash: observationHash });
  const executionReservationMatched = executionReservation.settlementState === 'recorded'
    && executionReservation.evidenceHash === observationHash && !executionReservation.overrun && executionReservation.withinLimits
    && reservationBeforeNative;
  if (executionReservation.overrun || !executionReservation.withinLimits) failure ??= 'EXECUTION_RESERVATION_OVERRUN';
  if (!executionReservationMatched) failure ??= 'EXECUTION_RESERVATION_INVALID';
  const taskIncompleteBeforeEffect = !failure && !resuming && !context && !cutOutputAfterPass
    && output.valid === true && exitCode === 0 && result?.is_error === false && result.session_id === sessionId
    && requestDiagnostics.requestOutcome === 'all-succeeded' && cleanupComplete === true
    && ledgerMatched && completedUsageObserved && usageUnobservedAttempts === 0 && rateLimitsMatched && sourceUnchanged
    && treeEvidence?.stopped === true && recordedNativeStopped && routeMatched && requests.length === settled.length
    && events.length === 1 && events[0].event === 'TASK_READ' && Object.keys(events[0]).length === 1
    && sha256 === sourceHash(task.baseline) && oracleUnchanged;
  if (taskIncompleteBeforeEffect) failure = 'DEVELOPMENT_TASK_INCOMPLETE';
  let artifactHashes = null;
  if (independentPassed) {
    try {
      artifactHashes = readDevelopmentArtifactHashes(root, taskId);
      verifyDevelopmentArtifacts(root, { taskId, sourceSha256: sha256, artifactHashes }, budget);
      need(artifactHashes.events === sourceHash(eventsText), 'DEVELOPMENT_ARTIFACT_CHANGED');
    } catch { failure ??= 'DEVELOPMENT_ARTIFACT_CHANGED'; }
  }
  const passed = !failure && nativeOutputCompleted(output, { exitCode, sessionId, resultText: 'CLAUDUCT_DEVELOPMENT_DONE', oraclePassed: independentPassed })
    && ledgerMatched && completedUsageObserved && rateLimitsMatched && sourceUnchanged && treeEvidence?.stopped && recordedNativeStopped
    && routeMatched && requests.length === settled.length && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens
    && events[0]?.event === 'TASK_READ' && baselineFailed && revisedPassed && independentPassed && oracleUnchanged;
  const outputCutObserved = outputCut && cutReleased && output.evidence.failure === 'OUTPUT_PIPE_CLOSED'
    && executionReservationMatched
    && output.evidence.resultCount === 0 && output.valid === false && treeEvidence?.stopped === true
    && sourceUnchanged && ledgerMatched && rateLimitsMatched && independentPassed && baselineFailed && revisedPassed && recordedNativeStopped
    && events.filter(row => row.event === 'SOURCE_WRITTEN').length === 1 && routeMatched && requests.length === settled.length
    && attempts <= budget.requestLimit && inputTokens <= budget.maxObservedInputTokens && outputTokens <= budget.maxObservedOutputTokens;
  const summary = { suite: localNative ? 'native-development-local' : 'native-development-live', root, model, effort, taskId, localNative, phase, sessionId,
    continuedFrom: context?.root ?? null, contextMatched, contextEvidence: contexts[0] ?? null,
    actualModelRequests: localNative ? 0 : attempts, passed: passed === true, failure, exitCode,
    elapsedMs, attempts, attemptsComplete: requests.length === settled.length, inputTokens, outputTokens,
    cumulativeAttempts: priorAttempts + attempts, cumulativeElapsedMs: priorElapsedMs + elapsedMs,
    cumulativeInputTokens: priorInputTokens + inputTokens, cumulativeOutputTokens: priorOutputTokens + outputTokens,
    routeMatched, nativeJson: result !== null, nativeError: result?.is_error ?? null, cleanupComplete: cleanupComplete === true,
    baselineFailed, revisedPassed, independentPassed, oracleUnchanged, testsExecuted: tests.length, sourceSha256: sha256, artifactHashes,
    sourceUnchanged, ledgerMatched, completedUsageObserved, usageUnobservedAttempts, usageLedger, requestDiagnostics, rateLimits, rateLimitsMatched,
    executionReservation, executionReservationMatched, reservationBeforeNative,
    outputCut, cutReleased, outputCutObserved, taskIncompleteBeforeEffect: taskIncompleteBeforeEffect === true, recordedNativeStopped,
    output: output.evidence, tree: treeEvidence };
  write(join(root, `result${suffix}.json`), summary); return summary;
}
export function developmentUsageEvidence(first, continuation, continuationFailure) {
  const continuationUsageUnobserved = continuationFailure !== null;
  const phases = continuation === null ? [first] : [first, continuation];
  const countsKnown = phases.every(phase => phase?.attemptsComplete === true && phase.ledgerMatched === true
    && Number.isSafeInteger(phase.attempts) && phase.attempts >= 0
    && Number.isSafeInteger(phase.usageUnobservedAttempts) && phase.usageUnobservedAttempts >= 0
    && phase.usageUnobservedAttempts <= phase.attempts);
  const total = countsKnown ? phases.reduce((sum, phase) => sum + phase.usageUnobservedAttempts, 0) : null;
  // A continuation can start requests and then throw before returning evidence.
  // Its missing result cannot release the reserved usage as an observed zero.
  return { usageUnobservedAttempts: !continuationUsageUnobserved && Number.isSafeInteger(total) ? total : null,
    continuationUsageUnobserved };
}
export async function verifyDevelopmentSequence({ model, powershell, localNative,
  priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens }) {
  need(typeof localNative === 'boolean', 'INVALID_ARGUMENTS');
  const first = await verifyNativeDevelopment({ model, powershell, localNative,
    priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens });
  let second = null, continuationFailure = null;
  if (first.passed) {
    try {
      second = await verifyNativeDevelopment({ model, powershell, localNative, taskId: 'retry-delay-window', continueFrom: first.root,
        priorAttempts: first.cumulativeAttempts, priorElapsedMs: first.cumulativeElapsedMs,
        priorInputTokens: first.cumulativeInputTokens, priorOutputTokens: first.cumulativeOutputTokens });
    } catch (error) {
      continuationFailure = /^CONTINUATION_[A-Z_]+$/.test(error.message) ? error.message : 'CONTINUATION_FAILED';
    }
  }
  const originalSourceUnchanged = sourceHash(read(join(first.root, 'work', developmentTask().sourceFile))) === first.sourceSha256;
  const passed = first.passed && second?.passed === true && first.sessionId === second.sessionId
    && second.contextMatched === true && originalSourceUnchanged && continuationFailure === null;
  const result = { suite: localNative ? 'native-development-sequence-local' : 'native-development-sequence-live',
    root: first.root, model, localNative, passed, scenarioPassed: passed, taskCompleted: passed,
    first, second, continuationFailure, originalSourceUnchanged, sameSession: first.sessionId === second?.sessionId,
    attempts: first.attempts + (second?.attempts ?? 0), elapsedMs: first.elapsedMs + (second?.elapsedMs ?? 0),
    inputTokens: first.inputTokens + (second?.inputTokens ?? 0), outputTokens: first.outputTokens + (second?.outputTokens ?? 0),
    attemptsComplete: first.attemptsComplete && (first.passed ? second?.attemptsComplete === true : true) && continuationFailure === null,
    actualModelRequests: first.actualModelRequests + (second?.actualModelRequests ?? 0),
    ...developmentUsageEvidence(first, second, continuationFailure), longStageCounts: 0 };
  write(join(first.root, 'sequence-result.json'), result); return result;
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
    attemptsComplete: first.attemptsComplete && finish?.attemptsComplete === true && resumeFailure === null,
    ...developmentUsageEvidence(first, finish, resumeFailure),
    actualModelRequests: localNative ? 0 : attempts, ...(localNative ? { actualCredentialReads: 0 } : {}), longStageCounts: 0 };
  write(join(first.root, 'output-recovery-result.json'), result); return result;
}
export async function verifyDevelopmentIncompleteRecovery({ model, powershell, localNative,
  taskId = 'retry-delay-window', priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens }) {
  need(localNative === true, 'LOCAL_INCOMPLETE_STIMULUS_REQUIRED');
  const first = await verifyNativeDevelopment({ model, powershell, localNative, taskId, earlyExitAfterRead: true, requestLimit: 6,
    priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens });
  let retry = null, resumeFailure = null;
  if (first.taskIncompleteBeforeEffect && first.passed === false) {
    try {
      retry = await verifyNativeDevelopment({ model, powershell, localNative, taskId, resumeIncompleteRoot: first.root, requestLimit: 6,
        priorAttempts: first.cumulativeAttempts, priorElapsedMs: first.cumulativeElapsedMs,
        priorInputTokens: first.cumulativeInputTokens, priorOutputTokens: first.cumulativeOutputTokens });
    } catch (error) {
      resumeFailure = /^RESUME_[A-Z_]+$/.test(error.message) ? error.message : error.code === 'EEXIST'
        ? 'RESUME_ALREADY_CLAIMED' : 'DEVELOPMENT_RESUME_FAILED';
    }
  }
  const events = read(join(first.root, 'work', 'events.jsonl')).trim().split('\n').filter(Boolean).map(JSON.parse);
  const sourceWrites = events.filter(row => row.event === 'SOURCE_WRITTEN').length;
  const sameSession = first.sessionId === retry?.sessionId;
  const passed = first.taskIncompleteBeforeEffect === true && first.passed === false && retry?.passed === true
    && sameSession && sourceWrites === 1 && resumeFailure === null;
  const result = { suite: 'native-development-incomplete-recovery-local', root: first.root, model, localNative,
    passed, scenarioPassed: passed, taskCompleted: passed, first, retry, resumeFailure, sourceWrites, sameSession,
    attempts: first.attempts + (retry?.attempts ?? 0), elapsedMs: first.elapsedMs + (retry?.elapsedMs ?? 0),
    inputTokens: first.inputTokens + (retry?.inputTokens ?? 0), outputTokens: first.outputTokens + (retry?.outputTokens ?? 0),
    attemptsComplete: first.attemptsComplete && retry?.attemptsComplete === true && resumeFailure === null,
    ...developmentUsageEvidence(first, retry, resumeFailure), actualModelRequests: 0, actualCredentialReads: 0, longStageCounts: 0 };
  write(join(first.root, 'incomplete-recovery-result.json'), result); return result;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [live, model, powershell, priorAttempts, priorElapsedMs, priorInputTokens, priorOutputTokens] = process.argv.slice(2);
    if (live === '--local-incomplete-recovery') {
      need(process.argv.length === 5, 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyDevelopmentIncompleteRecovery({ model, powershell, localNative: true,
        priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 });
      console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    } else if (['--local-sequence', '--live-sequence'].includes(live)) {
      const localNative = live === '--local-sequence';
      need(process.argv.length === (localNative ? 5 : 9), 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyDevelopmentSequence({ model, powershell, localNative,
        priorAttempts: localNative ? 0 : Number(priorAttempts), priorElapsedMs: localNative ? 0 : Number(priorElapsedMs),
        priorInputTokens: localNative ? 0 : Number(priorInputTokens), priorOutputTokens: localNative ? 0 : Number(priorOutputTokens) });
      console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    } else if (['--local-output-recovery', '--live-output-recovery'].includes(live)) {
      const localNative = live === '--local-output-recovery';
      need(process.argv.length === (localNative ? 5 : 9), 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyDevelopmentOutputRecovery({ model, powershell, localNative,
        priorAttempts: localNative ? 0 : Number(priorAttempts), priorElapsedMs: localNative ? 0 : Number(priorElapsedMs),
        priorInputTokens: localNative ? 0 : Number(priorInputTokens), priorOutputTokens: localNative ? 0 : Number(priorOutputTokens) });
      console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
    } else if (['--local-task', '--live-task'].includes(live)) {
      const localNative = live === '--local-task';
      const [, , , taskId, ...prior] = process.argv.slice(2);
      need([localNative ? 6 : 10, localNative ? 7 : 11].includes(process.argv.length), 'LIVE_ARGUMENTS_REQUIRED');
      const result = await verifyNativeDevelopment({ model, powershell, localNative, taskId,
        requestLimit: localNative ? prior.length === 0 ? 16 : Number(prior[0]) : prior.length === 4 ? 16 : Number(prior[4]),
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
