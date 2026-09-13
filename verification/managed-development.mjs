import { readFileSync, writeFileSync, lstatSync, realpathSync, existsSync } from 'node:fs';
import { join, resolve, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { sourceHash } from './development-fixture.mjs';
import { developmentTask, developmentTaskWorkKey } from './development-tasks.mjs';
import { readExecutionAccount, reserveExecutionAccount, closeExecutionAccount } from './execution-account.mjs';
import { readManagedLedgerAccount } from './managed-ledger-account.mjs';
import { publicDevelopmentSource } from './fixtures/development-responses.mjs';
import { verifyNativeDevelopment, readDevelopmentAccountingEvidence, prepareDevelopmentInterruption, readDevelopmentInterruption, nativeDevelopmentSourceHashes } from './verify-native-development.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const need = (ok, code = 'MANAGED_DEVELOPMENT_INVALID') => { if (!ok) throw new Error(code); };
const encode = value => JSON.stringify(value) + '\n';
const shape = (value, fields) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === fields.length && fields.every(key => Object.hasOwn(value, key));
function read(path) {
  const info = lstatSync(path);
  need(info.isFile() && !info.isSymbolicLink() && info.size <= 16384);
  const text = new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(path)), value = JSON.parse(text);
  need(encode(value) === text);
  return value;
}
function create(path, value) { writeFileSync(path, encode(value), { flag: 'wx' }); }
function rootPath(path) {
  need(typeof path === 'string');
  const root = resolve(path);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
    && /^managed-development-[A-Za-z0-9]{6}$/.test(basename(root))
    && !lstatSync(root).isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase());
  return root;
}
function evidenceFor(entry) {
  const task = read(join(entry.entry, 'task.json')), binding = read(join(entry.entry, 'binding.json'));
  need(sourceHash(encode(task)) === entry.executionHash && task.version === 1 && ['development', 'development-finish', 'development-retry'].includes(binding.phase));
  const evidence = binding.phase === 'development' && existsSync(join(binding.root, 'interruption-evidence.json'))
    ? readDevelopmentInterruption(binding.root).evidence : readDevelopmentAccountingEvidence(binding.root, binding.phase);
  const suffix = binding.phase.slice('development'.length);
  const budgetPath = join(evidence.root, `budget${suffix}.json`), budget = read(budgetPath);
  need(evidence.model === task.model && evidence.effort === task.effort && evidence.localNative === task.localNative
    && evidence.taskId === task.taskId && budget.executionAccountHash === entry.reservation.reservationHash
    && binding.budgetHash === sourceHash(readFileSync(budgetPath))
    && binding.reservationHash === sourceHash(readFileSync(join(evidence.root, `usage-${binding.phase}`, 'execution-reservation.json'))));
  need(!entry.closed || entry.reservation.evidenceHash === evidence.evidenceHash, 'MANAGED_DEVELOPMENT_EVIDENCE_CHANGED');
  return evidence;
}

export function reconcileManagedDevelopment(path, ownerNonce) {
  const root = rootPath(path), before = existsSync(join(root, 'legacy-ledger.json'))
    ? readManagedLedgerAccount(root).account : readExecutionAccount(root), entry = before.entries.at(-1);
  need(entry, 'MANAGED_DEVELOPMENT_EMPTY');
  const evidence = evidenceFor(entry);
  const account = entry.closed ? before : closeExecutionAccount(entry.entry,
    { ownerNonce, observed: evidence.observed, evidenceHash: evidence.evidenceHash });
  return { suite: 'managed-development', root, nativeRoot: evidence.root, passed: evidence.passed && account.withinLimits,
    nativePassed: evidence.passed, failure: evidence.failure, account, evidence };
}

export async function runManagedDevelopment({ root, model, powershell, taskId = 'retry-after-seconds', localNative = false,
  continuation = false, interruptAfterNativeResult = false, recoverInterrupted = false, holdAfterSourceWrite = false, holdAfterTaskRead = false,
  plannedStep }) {
  root = rootPath(root);
  need(Object.hasOwn({ luna: 'max', sol: 'low' }, model) && typeof model === 'string'
    && typeof localNative === 'boolean' && typeof continuation === 'boolean'
    && typeof recoverInterrupted === 'boolean' && (!recoverInterrupted || !continuation && !interruptAfterNativeResult)
    && typeof holdAfterSourceWrite === 'boolean' && typeof holdAfterTaskRead === 'boolean' && !(holdAfterSourceWrite && holdAfterTaskRead)
    && (!(holdAfterSourceWrite || holdAfterTaskRead) || localNative && !continuation && !recoverInterrupted && !interruptAfterNativeResult)
    && typeof interruptAfterNativeResult === 'boolean' && (!interruptAfterNativeResult || localNative)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'));
  developmentTask(taskId);
  if (localNative) publicDevelopmentSource(taskId);
  if (!localNative || existsSync(join(root, 'legacy-ledger.json'))) readManagedLedgerAccount(root, { localNative });
  if (existsSync(join(root, 'development-plan.json')) || plannedStep !== undefined) {
    const state = readManagedPlan(root, { localNative });
    need(shape(plannedStep, ['planHash', 'index']) && plannedStep.planHash === state.planHash
      && plannedStep.index === state.completedSteps && state.plan.taskIds[plannedStep.index] === taskId
      && state.plan.model === model && !state.failed, 'MANAGED_PLAN_STEP_INVALID');
    need(state.plan.deadlineMs - Date.now() >= (localNative ? 60000 : 660000), 'MANAGED_PLAN_DEADLINE');
    need(recoverInterrupted ? state.account.pending !== null || state.recoveryRoot !== null
      : state.account.pending === null && state.recoveryRoot === null, 'MANAGED_PLAN_PENDING');
    need(continuation === (!recoverInterrupted && plannedStep.index > 0
      && state.plan.taskIds[plannedStep.index - 1] !== taskId), 'MANAGED_PLAN_STEP_INVALID');
  }
  let account = readExecutionAccount(root), interruptedRoot;
  if (recoverInterrupted) {
    const last = account.entries.at(-1); need(last, 'MANAGED_DEVELOPMENT_EMPTY');
    const before = read(join(last.entry, 'task.json')), binding = read(join(last.entry, 'binding.json'));
    need(before.model === model && before.localNative === localNative && before.taskId === taskId
      && binding.phase === 'development' && sourceHash(encode(before)) === last.executionHash, 'MANAGED_DEVELOPMENT_PREDECESSOR');
    if (!existsSync(join(binding.root, 'interruption-evidence.json'))) {
      prepareDevelopmentInterruption(binding.root, { model, localNative, powershell, managerPid: last.ownerPid,
        accountHash: last.reservation.reservationHash });
    }
    const previous = reconcileManagedDevelopment(root);
    need(previous.evidence.failure === 'MANAGER_INTERRUPTED', 'MANAGED_DEVELOPMENT_PREDECESSOR');
    account = previous.account; interruptedRoot = previous.nativeRoot;
  }
  need(account.pending === null, 'EXECUTION_ACCOUNT_PENDING');
  let previous;
  if (continuation) {
    need(account.entries.length > 0, 'MANAGED_DEVELOPMENT_EMPTY');
    previous = evidenceFor(account.entries.at(-1));
    need(previous.passed && previous.model === model && previous.localNative === localNative && previous.taskId !== taskId,
      'MANAGED_DEVELOPMENT_PREDECESSOR');
  }
  const task = { version: 1, model, effort: { luna: 'max', sol: 'low' }[model], localNative, taskId, requestLimit: 6,
    continuedFrom: previous?.root ?? null, ...(interruptedRoot ? { recoveredFrom: interruptedRoot } : {}),
    ...(plannedStep ? { planHash: plannedStep.planHash, planStep: plannedStep.index } : {}) };
  const claim = reserveExecutionAccount(root, { executionHash: sourceHash(encode(task)), allowance: {
    attempts: 6, inputTokens: 131072, outputTokens: 32768, elapsedMs: localNative ? 60000 : 660000 } });
  create(join(claim.entry, 'task.json'), task);
  const prior = claim.previous;
  await verifyNativeDevelopment({ model, powershell, taskId, localNative, requestLimit: 6, holdAfterSourceWrite, holdAfterTaskRead,
    ...(previous ? { continueFrom: previous.root } : {}),
    ...(interruptedRoot ? { resumeInterruptedRoot: interruptedRoot } : {}),
    priorAttempts: prior.attempts, priorInputTokens: prior.inputTokens, priorOutputTokens: prior.outputTokens, priorElapsedMs: prior.elapsedMs,
    executionAccountHash: claim.reservation.reservationHash,
    onReservation: binding => { create(join(claim.entry, 'binding.json'), binding); } });
  // A bounded, explicitly local fault point. The native result and usage are
  // already on disk; the new manager must verify them before reconciliation.
  if (interruptAfterNativeResult) process.exit(73);
  return reconcileManagedDevelopment(root, claim.ownerNonce);
}

export function createManagedPlan(path, { model, localNative, taskIds, maxDurationMs }) {
  const root = rootPath(path);
  need(typeof model === 'string' && Object.hasOwn({ sol: 'low', luna: 'max' }, model)
    && typeof localNative === 'boolean' && Array.isArray(taskIds) && taskIds.length > 0 && taskIds.length <= 256
    && Number.isSafeInteger(maxDurationMs) && maxDurationMs >= (localNative ? 60000 : 660000)
    && maxDurationMs <= 72 * 3600000, 'MANAGED_PLAN_INVALID');
  for (let index = 0; index < taskIds.length; index++) {
    need(Object.hasOwn(taskIds, index) && typeof taskIds[index] === 'string', 'MANAGED_PLAN_INVALID');
    developmentTask(taskIds[index]);
    if (localNative) publicDevelopmentSource(taskIds[index]);
  }
  need(localNative || new Set(taskIds.map(developmentTaskWorkKey)).size === taskIds.length, 'MANAGED_PLAN_REPEATED_TASK');
  if (!localNative || existsSync(join(root, 'legacy-ledger.json'))) readManagedLedgerAccount(root, { localNative });
  const account = readExecutionAccount(root);
  need(account.entries.length === 0, 'MANAGED_PLAN_ACCOUNT_NOT_EMPTY');
  const createdAtMs = Date.now(), plan = { version: 1, accountHash: account.accountHash, model,
    effort: { sol: 'low', luna: 'max' }[model], localNative, taskIds: [...taskIds], createdAtMs,
    deadlineMs: createdAtMs + maxDurationMs, sourceHashes: nativeDevelopmentSourceHashes(localNative) };
  need(Buffer.byteLength(encode(plan)) <= 16384, 'MANAGED_PLAN_INVALID');
  create(join(root, 'development-plan.json'), plan);
  create(join(root, 'development-plan-ready.json'), { version: 1, planHash: sourceHash(encode(plan)), accountHash: account.accountHash });
  return readManagedPlan(root, { localNative });
}

export function readManagedPlan(path, { localNative } = {}) {
  const root = rootPath(path), plan = read(join(root, 'development-plan.json'));
  const ready = read(join(root, 'development-plan-ready.json')), planHash = sourceHash(encode(plan));
  need(shape(ready, ['version', 'planHash', 'accountHash']) && ready.version === 1
    && ready.planHash === planHash && ready.accountHash === plan.accountHash, 'MANAGED_PLAN_CHANGED');
  need(shape(plan, ['version', 'accountHash', 'model', 'effort', 'localNative', 'taskIds', 'createdAtMs', 'deadlineMs', 'sourceHashes'])
    && plan.version === 1 && typeof plan.model === 'string' && Object.hasOwn({ sol: 'low', luna: 'max' }, plan.model)
    && plan.effort === { sol: 'low', luna: 'max' }[plan.model] && typeof plan.localNative === 'boolean'
    && Array.isArray(plan.taskIds) && plan.taskIds.length > 0 && plan.taskIds.length <= 256
    && Number.isSafeInteger(plan.createdAtMs) && plan.createdAtMs > 0 && Number.isSafeInteger(plan.deadlineMs)
    && plan.deadlineMs - plan.createdAtMs >= (plan.localNative ? 60000 : 660000)
    && plan.deadlineMs - plan.createdAtMs <= 72 * 3600000, 'MANAGED_PLAN_INVALID');
  need(localNative === undefined || localNative === plan.localNative, 'MANAGED_PLAN_MODE_MISMATCH');
  need(Date.now() >= plan.createdAtMs, 'MANAGED_PLAN_CLOCK');
  for (const task of plan.taskIds) {
    need(typeof task === 'string', 'MANAGED_PLAN_INVALID');
    developmentTask(task);
    if (plan.localNative) publicDevelopmentSource(task);
  }
  need(plan.localNative || new Set(plan.taskIds.map(developmentTaskWorkKey)).size === plan.taskIds.length, 'MANAGED_PLAN_REPEATED_TASK');
  need(encode(plan.sourceHashes) === encode(nativeDevelopmentSourceHashes(plan.localNative)), 'MANAGED_PLAN_SOURCE_CHANGED');
  const account = !plan.localNative || existsSync(join(root, 'legacy-ledger.json'))
    ? readManagedLedgerAccount(root, { localNative: plan.localNative }).account : readExecutionAccount(root);
  need(account.accountHash === plan.accountHash, 'MANAGED_PLAN_ACCOUNT_MISMATCH');
  let completedSteps = 0, recoveryRoot = null, previousRoot = null, failed = false;
  const completed = [];
  for (const entry of account.entries) {
    const task = read(join(entry.entry, 'task.json'));
    need(!failed && completedSteps < plan.taskIds.length && task.planHash === planHash && task.planStep === completedSteps
      && task.model === plan.model && task.effort === plan.effort && task.localNative === plan.localNative
      && task.taskId === plan.taskIds[completedSteps] && sourceHash(encode(task)) === entry.executionHash
      && (task.recoveredFrom ?? null) === recoveryRoot, 'MANAGED_PLAN_ENTRY_MISMATCH');
    const continuedFrom = recoveryRoot === null && completedSteps > 0 && plan.taskIds[completedSteps - 1] !== task.taskId ? previousRoot : null;
    need(task.continuedFrom === continuedFrom, 'MANAGED_PLAN_ENTRY_MISMATCH');
    if (!entry.closed) break;
    const evidence = evidenceFor(entry);
    if (evidence.passed) {
      completed.push({ index: completedSteps, taskId: task.taskId, root: evidence.root, evidenceHash: evidence.evidenceHash });
      completedSteps++; previousRoot = evidence.root; recoveryRoot = null;
    } else if (evidence.failure === 'MANAGER_INTERRUPTED' && recoveryRoot === null) recoveryRoot = evidence.root;
    else failed = true;
  }
  return { root, plan, planHash, account, completedSteps, completed, recoveryRoot, failed,
    done: completedSteps === plan.taskIds.length && account.pending === null && !failed };
}

export async function runManagedPlan({ root, powershell, localNative, interruptAfterCompleted = 0 }) {
  need(typeof powershell === 'string' && powershell.endsWith('pwsh.exe') && typeof localNative === 'boolean'
    && Number.isSafeInteger(interruptAfterCompleted) && interruptAfterCompleted >= 0 && interruptAfterCompleted <= 256
    && (interruptAfterCompleted === 0 || localNative), 'MANAGED_PLAN_INVALID');
  let state = readManagedPlan(root, { localNative });
  for (let iteration = 0; iteration <= state.plan.taskIds.length * 2; iteration++) {
    need(!state.failed, 'MANAGED_PLAN_TASK_FAILED');
    if (state.done) return { suite: 'managed-development-plan', root: state.root, passed: true,
      completedSteps: state.completedSteps, totalSteps: state.plan.taskIds.length, account: state.account,
      localNative, longStageEvidence: false, releaseVerdict: 'HOLD' };
    let recoverInterrupted = state.recoveryRoot !== null;
    if (state.account.pending !== null) {
      const binding = read(join(state.account.pending, 'binding.json'));
      need(['development', 'development-retry', 'development-finish'].includes(binding.phase), 'MANAGED_PLAN_ENTRY_MISMATCH');
      const suffix = binding.phase.slice('development'.length);
      if (existsSync(join(binding.root, `result${suffix}.json`))) {
        reconcileManagedDevelopment(root); state = readManagedPlan(root, { localNative }); continue;
      }
      need(binding.phase === 'development', 'MANAGED_PLAN_RECOVERY_UNAVAILABLE');
      recoverInterrupted = true;
    }
    const index = state.completedSteps;
    await runManagedDevelopment({ root, powershell, model: state.plan.model, localNative, taskId: state.plan.taskIds[index],
      continuation: !recoverInterrupted && index > 0 && state.plan.taskIds[index - 1] !== state.plan.taskIds[index],
      recoverInterrupted, plannedStep: { planHash: state.planHash, index } });
    state = readManagedPlan(root, { localNative });
    if (interruptAfterCompleted > 0 && state.completedSteps === interruptAfterCompleted) process.exit(73);
  }
  throw new Error('MANAGED_PLAN_NO_PROGRESS');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, root, model, powershell, taskId] = process.argv.slice(2);
    need(mode === '--reconcile' ? process.argv.length === 4
      : ['--live-task', '--live-continue', '--live-recover', '--local-task', '--local-continue', '--local-interrupt-after-result', '--local-recover', '--local-hold-after-source', '--local-hold-after-read'].includes(mode) && process.argv.length === 7);
    const result = mode === '--reconcile' ? reconcileManagedDevelopment(root)
      : await runManagedDevelopment({ root, model, powershell, taskId, localNative: mode.startsWith('--local-'),
        continuation: mode.endsWith('-continue'), interruptAfterNativeResult: mode === '--local-interrupt-after-result',
        recoverInterrupted: mode.endsWith('-recover'), holdAfterSourceWrite: mode === '--local-hold-after-source', holdAfterTaskRead: mode === '--local-hold-after-read' });
    console.log(JSON.stringify({ suite: result.suite, root: result.root, nativeRoot: result.nativeRoot, passed: result.passed,
      nativePassed: result.nativePassed, failure: result.failure, entries: result.account.entries.length,
      pending: result.account.pending !== null, initial: result.account.initial, charged: result.account.charged,
      withinLimits: result.account.withinLimits, observed: result.evidence.observed,
      actualModelRequests: result.evidence.localNative ? 0 : result.evidence.observed.attempts }));
    process.exitCode = result.passed ? 0 : 1;
  } catch (error) {
    const known = /^(?:EXECUTION_ACCOUNT_[A-Z_]+|MANAGED_LEDGER_[A-Z_]+|MANAGED_PLAN_[A-Z_]+|MANAGED_DEVELOPMENT_[A-Z_]+|INTERRUPTION_[A-Z_]+|DEVELOPMENT_ACCOUNTING_INVALID|REGISTERED_DEVELOPMENT_TASK_INVALID|DEVELOPMENT_LOCAL_SOURCE_REQUIRED|DEVELOPMENT_SOURCE_REJECTED)$/;
    console.log(JSON.stringify({ suite: 'managed-development', passed: false,
      failure: known.test(error.message) ? error.message : 'MANAGED_DEVELOPMENT_FAILED' }));
    process.exitCode = 1;
  }
}
