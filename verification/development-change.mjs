import { writeFileSync, existsSync, lstatSync, realpathSync, mkdtempSync, openSync, readSync, fstatSync, closeSync } from 'node:fs';
import { join, dirname, basename, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { sourceHash } from './development-fixture.mjs';
import { developmentTask, developmentOracleSource } from './development-tasks.mjs';
import { readManagedDevelopmentResult } from './managed-development.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const encode = value => JSON.stringify(value) + '\n';
const need = (ok, code = 'DEVELOPMENT_CHANGE_INVALID') => { if (!ok) throw new Error(code); };
const shape = (value, keys) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
function directory(path, prefix) {
  need(typeof path === 'string'); const root = resolve(path), info = lstatSync(root);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
    && new RegExp(`^${prefix}-[A-Za-z0-9]{6}$`).test(basename(root))
    && info.isDirectory() && !info.isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase());
  return root;
}
function bytes(path, limit) {
  const before = lstatSync(path, { bigint: true });
  need(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.size <= BigInt(limit)
    && realpathSync(path).toLowerCase() === resolve(path).toLowerCase());
  const fd = openSync(path, 'r');
  try {
    const opened = fstatSync(fd, { bigint: true });
    need(opened.dev === before.dev && opened.ino === before.ino && opened.size === before.size && opened.nlink === 1n);
    const value = Buffer.alloc(limit + 1); let length = 0;
    while (length < value.length) { const count = readSync(fd, value, length, value.length - length, length); if (!count) break; length += count; }
    const after = fstatSync(fd, { bigint: true });
    need(BigInt(length) === before.size && before.size === after.size && before.mtimeNs === after.mtimeNs);
    return value.subarray(0, length);
  } finally { closeSync(fd); }
}
function read(path, limit = 16384) {
  const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes(path, limit)), value = JSON.parse(text);
  need(encode(value) === text); return value;
}
function create(path, value) { writeFileSync(path, encode(value), { flag: 'wx' }); }
function targetPath(root, path) {
  root = directory(root, 'development-target');
  need(typeof path === 'string' && path.length <= 160 && /^(?:[A-Za-z0-9_-]+\/)*[A-Za-z0-9_-]+\.mjs$/.test(path)
    && path.split('/').every(part => !['node_modules', 'hooks'].includes(part)));
  return join(root, path);
}
function sourceResult(accountRoot, entryIndex) {
  const managed = readManagedDevelopmentResult(accountRoot, entryIndex), evidence = managed.evidence;
  const suffix = evidence.phase.slice('development'.length);
  const result = read(join(evidence.root, `result${suffix}.json`), 65536);
  const task = developmentTask(evidence.taskId), source = bytes(join(evidence.root, 'work', task.sourceFile), 8192);
  need(sourceHash(source) === result.sourceSha256 && result.baselineFailed === true && result.revisedPassed === true
    && result.independentPassed === true && result.cleanupComplete === true, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  return { managed, result, task, source };
}

// The caller reviews the source and selects the target. Hashes identify the
// reviewed bytes; they do not authorize an effect. This writes a proposal only.
export function prepareDevelopmentChange(options) {
  need(shape(options, ['accountRoot', 'entryIndex', 'targetRoot', 'targetPath', 'expectedBeforeHash', 'expectedAfterHash'])
    && digest(options.expectedBeforeHash) && digest(options.expectedAfterHash));
  const { managed, task, source } = sourceResult(options.accountRoot, options.entryIndex);
  const target = targetPath(options.targetRoot, options.targetPath), before = bytes(target, 8192);
  need(sourceHash(before) === options.expectedBeforeHash && options.expectedBeforeHash === sourceHash(task.baseline), 'DEVELOPMENT_CHANGE_TARGET_CHANGED');
  need(sourceHash(source) === options.expectedAfterHash && options.expectedAfterHash !== options.expectedBeforeHash, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  const root = mkdtempSync(join(project, '.tmp', 'development-change-'));
  const record = { version: 1, rootHash: sourceHash(root.toLowerCase()), accountRoot: managed.root, entryIndex: managed.entryIndex,
    accountHash: managed.accountHash, executionHash: managed.executionHash, evidenceHash: managed.evidence.evidenceHash,
    taskId: managed.evidence.taskId, localNative: managed.evidence.localNative,
    targetRoot: resolve(options.targetRoot), targetPath: options.targetPath,
    beforeHash: options.expectedBeforeHash, afterHash: options.expectedAfterHash, oracleHash: sourceHash(developmentOracleSource(managed.evidence.taskId)) };
  writeFileSync(join(root, 'before.mjs'), before, { flag: 'wx' });
  writeFileSync(join(root, 'proposed.mjs'), source, { flag: 'wx' });
  create(join(root, 'change.json'), record);
  create(join(root, 'ready.json'), { version: 1, changeHash: sourceHash(encode(record)) });
  return readDevelopmentChange(root);
}

export function readDevelopmentChange(path) {
  const root = directory(path, 'development-change'), record = read(join(root, 'change.json')), ready = read(join(root, 'ready.json'));
  const changeHash = sourceHash(encode(record));
  need(shape(ready, ['version', 'changeHash']) && ready.version === 1 && ready.changeHash === changeHash
    && shape(record, ['version', 'rootHash', 'accountRoot', 'entryIndex', 'accountHash', 'executionHash', 'evidenceHash', 'taskId',
      'localNative', 'targetRoot', 'targetPath', 'beforeHash', 'afterHash', 'oracleHash'])
    && record.version === 1 && record.rootHash === sourceHash(root.toLowerCase()), 'DEVELOPMENT_CHANGE_RECORD_CHANGED');
  const { managed, source, task } = sourceResult(record.accountRoot, record.entryIndex);
  need(managed.accountHash === record.accountHash && managed.executionHash === record.executionHash
    && managed.evidence.evidenceHash === record.evidenceHash && managed.evidence.taskId === record.taskId
    && managed.evidence.localNative === record.localNative && sourceHash(source) === record.afterHash
    && sourceHash(developmentOracleSource(record.taskId)) === record.oracleHash, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  need(sourceHash(bytes(join(root, 'before.mjs'), 8192)) === record.beforeHash && record.beforeHash === sourceHash(task.baseline)
    && sourceHash(bytes(join(root, 'proposed.mjs'), 8192)) === record.afterHash, 'DEVELOPMENT_CHANGE_RECORD_CHANGED');
  const target = targetPath(record.targetRoot, record.targetPath), targetHash = sourceHash(bytes(target, 8192));
  need([record.beforeHash, record.afterHash].includes(targetHash), 'DEVELOPMENT_CHANGE_TARGET_CHANGED');
  const applied = targetHash === record.afterHash;
  const intentPresent = existsSync(join(root, 'verification-intent.json'));
  if (intentPresent) {
    const intent = read(join(root, 'verification-intent.json'));
    need(shape(intent, ['version', 'changeHash', 'ownerPid']) && intent.version === 1 && intent.changeHash === changeHash
      && Number.isSafeInteger(intent.ownerPid) && intent.ownerPid > 0, 'DEVELOPMENT_CHANGE_VERIFICATION_CHANGED');
  }
  let verification = null;
  if (existsSync(join(root, 'verification-ready.json'))) {
    const seal = read(join(root, 'verification-ready.json'));
    verification = read(join(root, 'verification.json'));
    need(intentPresent && shape(seal, ['version', 'verificationHash']) && seal.version === 1 && seal.verificationHash === sourceHash(encode(verification))
      && shape(verification, ['version', 'changeHash', 'targetHash', 'passed', 'checks', 'failures', 'exitCode', 'timedOut',
        'stderrPresent', 'unchanged', 'localNative', 'actualModelRequests', 'actualCredentialReads', 'wholeProjectVerified', 'longStageEvidence'])
      && verification.version === 1 && verification.changeHash === changeHash && verification.targetHash === record.afterHash,
    'DEVELOPMENT_CHANGE_VERIFICATION_CHANGED');
    need(typeof verification.passed === 'boolean' && typeof verification.timedOut === 'boolean'
      && typeof verification.stderrPresent === 'boolean' && typeof verification.unchanged === 'boolean'
      && (verification.exitCode === null || Number.isSafeInteger(verification.exitCode))
      && [verification.checks, verification.failures].every(value => value === null || Number.isSafeInteger(value) && value >= 0 && value <= 128)
      && verification.localNative === record.localNative && verification.actualModelRequests === 0 && verification.actualCredentialReads === 0
      && verification.wholeProjectVerified === false && verification.longStageEvidence === false
      && (!verification.passed || verification.checks === task.checks && verification.failures === 0 && verification.exitCode === 0
        && !verification.timedOut && !verification.stderrPresent && verification.unchanged), 'DEVELOPMENT_CHANGE_VERIFICATION_CHANGED');
    need(applied, 'DEVELOPMENT_CHANGE_TARGET_CHANGED');
  }
  const uncertain = verification === null && (intentPresent || existsSync(join(root, 'verification.json')));
  return { root, record, changeHash, target, applied, verification, state: uncertain ? 'verification-uncertain'
    : verification ? verification.passed === true ? 'verified' : 'failed' : applied ? 'applied-unverified' : 'not-applied' };
}

export function verifyDevelopmentChange(path) {
  const current = readDevelopmentChange(path);
  need(current.state !== 'verification-uncertain', 'DEVELOPMENT_CHANGE_VERIFICATION_UNCERTAIN');
  need(current.applied, 'DEVELOPMENT_CHANGE_NOT_APPLIED');
  if (current.verification) return { ...current, testsStarted: 0 };
  create(join(current.root, 'verification-intent.json'), { version: 1, changeHash: current.changeHash, ownerPid: process.pid });
  const managed = readManagedDevelopmentResult(current.record.accountRoot, current.record.entryIndex), task = developmentTask(current.record.taskId);
  const oracle = join(managed.evidence.root, 'control', 'oracle.mjs');
  need(sourceHash(bytes(oracle, 65536)) === current.record.oracleHash, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
  const result = spawnSync(process.execPath, ['--permission', `--allow-fs-read=${oracle}`, `--allow-fs-read=${current.target}`, oracle, current.target],
    { cwd: current.record.targetRoot, env, encoding: 'utf8', timeout: 5000, maxBuffer: 16384, windowsHide: true });
  let outcome = null;
  try { outcome = JSON.parse(result.stdout); } catch { }
  const unchanged = sourceHash(bytes(current.target, 8192)) === current.record.afterHash
    && sourceHash(bytes(oracle, 65536)) === current.record.oracleHash;
  const passed = !result.error && result.signal === null && result.status === 0 && result.stderr === '' && unchanged
    && outcome?.passed === true && outcome.checks === task.checks && Array.isArray(outcome.failures) && outcome.failures.length === 0;
  const verification = { version: 1, changeHash: current.changeHash, targetHash: current.record.afterHash, passed,
    checks: Number.isSafeInteger(outcome?.checks) ? outcome.checks : null,
    failures: Array.isArray(outcome?.failures) ? outcome.failures.length : null, exitCode: result.status,
    timedOut: result.error?.code === 'ETIMEDOUT', stderrPresent: result.stderr !== '', unchanged,
    localNative: current.record.localNative, actualModelRequests: 0, actualCredentialReads: 0,
    wholeProjectVerified: false, longStageEvidence: false };
  create(join(current.root, 'verification.json'), verification);
  create(join(current.root, 'verification-ready.json'), { version: 1, verificationHash: sourceHash(encode(verification)) });
  return { ...readDevelopmentChange(current.root), testsStarted: 1 };
}
