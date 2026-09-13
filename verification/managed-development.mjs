import { readFileSync, writeFileSync, lstatSync, realpathSync, existsSync } from 'node:fs';
import { join, resolve, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { sourceHash } from './development-fixture.mjs';
import { developmentTask } from './development-tasks.mjs';
import { readExecutionAccount, reserveExecutionAccount, closeExecutionAccount } from './execution-account.mjs';
import { verifyNativeDevelopment, readDevelopmentAccountingEvidence, prepareDevelopmentInterruption, readDevelopmentInterruption } from './verify-native-development.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const need = (ok, code = 'MANAGED_DEVELOPMENT_INVALID') => { if (!ok) throw new Error(code); };
const encode = value => JSON.stringify(value) + '\n';
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
  need(sourceHash(encode(task)) === entry.executionHash && task.version === 1 && ['development', 'development-finish'].includes(binding.phase));
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
  const root = rootPath(path), before = readExecutionAccount(root), entry = before.entries.at(-1);
  need(entry, 'MANAGED_DEVELOPMENT_EMPTY');
  const evidence = evidenceFor(entry);
  const account = entry.closed ? before : closeExecutionAccount(entry.entry,
    { ownerNonce, observed: evidence.observed, evidenceHash: evidence.evidenceHash });
  return { suite: 'managed-development', root, nativeRoot: evidence.root, passed: evidence.passed && account.withinLimits,
    nativePassed: evidence.passed, failure: evidence.failure, account, evidence };
}

export async function runManagedDevelopment({ root, model, powershell, taskId = 'retry-after-seconds', localNative = false,
  continuation = false, interruptAfterNativeResult = false, recoverInterrupted = false, holdAfterSourceWrite = false }) {
  root = rootPath(root);
  need(Object.hasOwn({ luna: 'max', sol: 'low' }, model) && typeof model === 'string'
    && typeof localNative === 'boolean' && typeof continuation === 'boolean'
    && typeof recoverInterrupted === 'boolean' && (!recoverInterrupted || !continuation && !interruptAfterNativeResult)
    && typeof holdAfterSourceWrite === 'boolean' && (!holdAfterSourceWrite || localNative && !continuation && !recoverInterrupted && !interruptAfterNativeResult)
    && typeof interruptAfterNativeResult === 'boolean' && (!interruptAfterNativeResult || localNative)
    && typeof powershell === 'string' && powershell.endsWith('pwsh.exe'));
  developmentTask(taskId);
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
    continuedFrom: previous?.root ?? null, ...(interruptedRoot ? { recoveredFrom: interruptedRoot } : {}) };
  const claim = reserveExecutionAccount(root, { executionHash: sourceHash(encode(task)), allowance: {
    attempts: 6, inputTokens: 131072, outputTokens: 32768, elapsedMs: localNative ? 60000 : 660000 } });
  create(join(claim.entry, 'task.json'), task);
  const prior = claim.previous;
  await verifyNativeDevelopment({ model, powershell, taskId, localNative, requestLimit: 6, holdAfterSourceWrite,
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

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [mode, root, model, powershell, taskId] = process.argv.slice(2);
    need(mode === '--reconcile' ? process.argv.length === 4
      : ['--local-task', '--local-continue', '--local-interrupt-after-result', '--local-recover', '--local-hold-after-source'].includes(mode) && process.argv.length === 7);
    const result = mode === '--reconcile' ? reconcileManagedDevelopment(root)
      : await runManagedDevelopment({ root, model, powershell, taskId, localNative: true,
        continuation: mode === '--local-continue', interruptAfterNativeResult: mode === '--local-interrupt-after-result',
        recoverInterrupted: mode === '--local-recover', holdAfterSourceWrite: mode === '--local-hold-after-source' });
    console.log(JSON.stringify({ suite: result.suite, root: result.root, nativeRoot: result.nativeRoot, passed: result.passed,
      nativePassed: result.nativePassed, failure: result.failure, entries: result.account.entries.length,
      pending: result.account.pending !== null, initial: result.account.initial, charged: result.account.charged,
      withinLimits: result.account.withinLimits, observed: result.evidence.observed,
      actualModelRequests: result.evidence.localNative ? 0 : result.evidence.observed.attempts }));
    process.exitCode = result.passed ? 0 : 1;
  } catch (error) {
    const known = /^(?:EXECUTION_ACCOUNT_[A-Z_]+|MANAGED_DEVELOPMENT_[A-Z_]+|INTERRUPTION_[A-Z_]+|DEVELOPMENT_ACCOUNTING_INVALID)$/;
    console.log(JSON.stringify({ suite: 'managed-development', passed: false,
      failure: known.test(error.message) ? error.message : 'MANAGED_DEVELOPMENT_FAILED' }));
    process.exitCode = 1;
  }
}
