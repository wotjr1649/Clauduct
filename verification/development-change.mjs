import { writeFileSync, existsSync, lstatSync, realpathSync, mkdtempSync, openSync, readSync, fstatSync, closeSync } from 'node:fs';
import { join, dirname, basename, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { sourceHash } from './development-fixture.mjs';
import { developmentTask, developmentOracleSource } from './development-tasks.mjs';
import { readManagedDevelopmentResult } from './managed-development.mjs';
import { readDevelopmentSource } from './development-source-files.mjs';

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
function sourceResult(accountRoot, entryIndex, sourcePath) {
  const managed = readManagedDevelopmentResult(accountRoot, entryIndex), evidence = managed.evidence;
  const suffix = evidence.phase.slice('development'.length);
  const result = read(join(evidence.root, `result${suffix}.json`), 65536);
  const originalTask = developmentTask(evidence.taskId), aggregate = readDevelopmentSource(join(evidence.root, 'work'), evidence.taskId);
  need(sourceHash(aggregate) === result.sourceSha256 && result.baselineFailed === true && result.revisedPassed === true
    && result.independentPassed === true && result.cleanupComplete === true, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  const part = originalTask.parts?.find(part => part.path === sourcePath);
  need(originalTask.parts ? part !== undefined : sourcePath === undefined, 'DEVELOPMENT_CHANGE_SOURCE_PART');
  const sourceTaskId = part?.taskId ?? evidence.taskId, task = developmentTask(sourceTaskId);
  const source = Buffer.from(part ? JSON.parse(aggregate).files.find(file => file.path === sourcePath).code : aggregate);
  return { managed, result, task, source, sourceTaskId };
}

// The caller reviews the source and selects the target. Hashes identify the
// reviewed bytes; they do not authorize an effect. This writes a proposal only.
export function prepareDevelopmentChange(options) {
  need(shape(options, ['accountRoot', 'entryIndex', 'targetRoot', 'targetPath', 'expectedBeforeHash', 'expectedAfterHash',
    ...(options && Object.hasOwn(options, 'sourcePath') ? ['sourcePath'] : [])])
    && digest(options.expectedBeforeHash) && digest(options.expectedAfterHash));
  const { managed, task, source, sourceTaskId } = sourceResult(options.accountRoot, options.entryIndex, options.sourcePath);
  const target = targetPath(options.targetRoot, options.targetPath), before = bytes(target, 8192);
  need(sourceHash(before) === options.expectedBeforeHash && options.expectedBeforeHash === sourceHash(task.baseline), 'DEVELOPMENT_CHANGE_TARGET_CHANGED');
  need(sourceHash(source) === options.expectedAfterHash && options.expectedAfterHash !== options.expectedBeforeHash, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
  const root = mkdtempSync(join(project, '.tmp', 'development-change-'));
  const record = { version: options.sourcePath === undefined ? 1 : 2, rootHash: sourceHash(root.toLowerCase()), accountRoot: managed.root, entryIndex: managed.entryIndex,
    accountHash: managed.accountHash, executionHash: managed.executionHash, evidenceHash: managed.evidence.evidenceHash,
    taskId: managed.evidence.taskId, localNative: managed.evidence.localNative,
    ...(options.sourcePath === undefined ? {} : { sourcePath: options.sourcePath, sourceTaskId }),
    targetRoot: resolve(options.targetRoot), targetPath: options.targetPath,
    beforeHash: options.expectedBeforeHash, afterHash: options.expectedAfterHash, oracleHash: sourceHash(developmentOracleSource(sourceTaskId)) };
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
      'localNative', 'targetRoot', 'targetPath', 'beforeHash', 'afterHash', 'oracleHash', ...(record.version === 2 ? ['sourcePath', 'sourceTaskId'] : [])])
    && [1, 2].includes(record.version) && record.rootHash === sourceHash(root.toLowerCase()), 'DEVELOPMENT_CHANGE_RECORD_CHANGED');
  const { managed, source, task, sourceTaskId } = sourceResult(record.accountRoot, record.entryIndex, record.sourcePath);
  need(managed.accountHash === record.accountHash && managed.executionHash === record.executionHash
    && managed.evidence.evidenceHash === record.evidenceHash && managed.evidence.taskId === record.taskId
    && managed.evidence.localNative === record.localNative && sourceHash(source) === record.afterHash
    && (record.version === 1 || record.sourceTaskId === sourceTaskId)
    && sourceHash(developmentOracleSource(sourceTaskId)) === record.oracleHash, 'DEVELOPMENT_CHANGE_SOURCE_CHANGED');
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
  const managed = readManagedDevelopmentResult(current.record.accountRoot, current.record.entryIndex), task = developmentTask(current.record.sourceTaskId ?? current.record.taskId);
  const oracle = join(managed.evidence.root, 'control', current.record.version === 2 ? task.oracleFile : 'oracle.mjs');
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

function integrationSpecification(definition, changes) {
  need(shape(definition, ['version', 'steps', 'cases']) && definition.version === 1
    && Buffer.byteLength(encode(definition)) <= 32768 && Array.isArray(definition.steps)
    && definition.steps.length >= 2 && definition.steps.length <= 32
    && Array.isArray(definition.cases) && definition.cases.length >= 1 && definition.cases.length <= 128,
  'DEVELOPMENT_INTEGRATION_INVALID');
  const invalid = () => need(false, 'DEVELOPMENT_INTEGRATION_INVALID');
  function json(value, depth = 0, count = { nodes: 0 }) {
    if (depth > 8 || ++count.nodes > 4096) invalid();
    if (value === null || typeof value === 'boolean' || typeof value === 'number' && Number.isFinite(value)) return;
    if (typeof value === 'string') { if (value.length > 4096) invalid(); return; }
    if (!value || typeof value !== 'object') invalid();
    for (const [key, item] of Object.entries(value)) { if (key.length > 128) invalid(); json(item, depth + 1, count); }
  }
  const inputCount = definition.cases[0]?.inputs?.length, names = new Set();
  if (!Number.isSafeInteger(inputCount) || inputCount < 1 || inputCount > 16) invalid();
  for (const test of definition.cases) {
    if (!shape(test, ['name', 'inputs', 'expected']) || typeof test.name !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(test.name)
      || names.has(test.name) || !Array.isArray(test.inputs) || test.inputs.length !== inputCount) invalid();
    names.add(test.name); json(test.inputs); json(test.expected);
  }
  function expression(value, step, depth = 0, count = { nodes: 0 }) {
    if (!value || depth > 8 || ++count.nodes > 256) invalid();
    if (['input', 'result'].includes(value.kind)) {
      if (!shape(value, ['kind', 'index']) || !Number.isSafeInteger(value.index) || value.index < 0
        || value.index >= (value.kind === 'input' ? inputCount : step)) invalid();
    } else if (value.kind === 'literal') {
      if (!shape(value, ['kind', 'value']) || value.value !== null && !['string', 'boolean', 'number'].includes(typeof value.value)) invalid();
      json(value.value);
    } else if (value.kind === 'sum') {
      if (!shape(value, ['kind', 'left', 'right'])) invalid();
      expression(value.left, step, depth + 1, count); expression(value.right, step, depth + 1, count);
    } else if (value.kind === 'object') {
      if (!shape(value, ['kind', 'fields']) || !value.fields || typeof value.fields !== 'object' || Array.isArray(value.fields)
        || Object.keys(value.fields).length > 16) invalid();
      for (const [key, child] of Object.entries(value.fields)) {
        if (!/^[A-Za-z][A-Za-z0-9_]{0,47}$/.test(key) || ['constructor', 'prototype', 'then'].includes(key)) invalid();
        expression(child, step, depth + 1, count);
      }
    } else invalid();
  }
  const used = new Set(), steps = definition.steps.map((step, index) => {
    if (!shape(step, ['targetPath', 'argument'])) invalid();
    const moduleIndex = changes.findIndex(change => change.record.targetPath === step.targetPath);
    if (moduleIndex < 0) invalid();
    expression(step.argument, index); used.add(moduleIndex);
    return { moduleIndex, argument: step.argument };
  });
  if (used.size !== changes.length) invalid();
  return { version: 1, modules: changes.map(change => ({ path: change.target, exportName: developmentTask(change.record.sourceTaskId ?? change.record.taskId).functionName })),
    steps, cases: definition.cases };
}
function changeSetMembers(roots) {
  need(Array.isArray(roots) && roots.length >= 2 && roots.length <= 16 && roots.every(root => typeof root === 'string'), 'DEVELOPMENT_CHANGE_SET_INVALID');
  const changes = roots.map(readDevelopmentChange), paths = new Set(), entries = new Set();
  for (const change of changes) {
    const sourceKey = `${change.record.entryIndex}:${change.record.sourcePath ?? ''}`;
    need(change.record.accountRoot === changes[0].record.accountRoot && change.record.targetRoot === changes[0].record.targetRoot
      && !paths.has(change.record.targetPath.toLowerCase()) && !entries.has(sourceKey), 'DEVELOPMENT_CHANGE_SET_MEMBERS');
    paths.add(change.record.targetPath.toLowerCase()); entries.add(sourceKey);
  }
  return changes;
}

// A set binds existing reviewed proposals and an integration specification.
// It never applies source files or turns partial application into completion.
export function prepareDevelopmentChangeSet(options) {
  need(shape(options, ['changeRoots', 'integration']), 'DEVELOPMENT_CHANGE_SET_INVALID');
  const changes = changeSetMembers(options.changeRoots);
  need(changes.every(change => change.state === 'not-applied'), 'DEVELOPMENT_CHANGE_SET_BASELINE');
  const compiled = integrationSpecification(options.integration, changes);
  const oracle = bytes(join(project, 'verification', 'fixtures', 'development-integration-oracle.mjs'), 16384);
  const root = mkdtempSync(join(project, '.tmp', 'development-change-set-'));
  const record = { version: 1, rootHash: sourceHash(root.toLowerCase()), accountRoot: changes[0].record.accountRoot,
    targetRoot: changes[0].record.targetRoot, changes: changes.map(change => ({ root: change.root, changeHash: change.changeHash,
      targetPath: change.record.targetPath })), definitionHash: sourceHash(encode(options.integration)),
    integrationHash: sourceHash(encode(compiled)), oracleHash: sourceHash(oracle) };
  create(join(root, 'definition.json'), options.integration); create(join(root, 'integration.json'), compiled);
  writeFileSync(join(root, 'oracle.mjs'), oracle, { flag: 'wx' });
  create(join(root, 'set.json'), record); create(join(root, 'ready.json'), { version: 1, setHash: sourceHash(encode(record)) });
  return readDevelopmentChangeSet(root);
}

export function readDevelopmentChangeSet(path) {
  const root = directory(path, 'development-change-set'), record = read(join(root, 'set.json')), ready = read(join(root, 'ready.json'));
  const setHash = sourceHash(encode(record));
  need(shape(record, ['version', 'rootHash', 'accountRoot', 'targetRoot', 'changes', 'definitionHash', 'integrationHash', 'oracleHash'])
    && record.version === 1 && record.rootHash === sourceHash(root.toLowerCase()) && shape(ready, ['version', 'setHash'])
    && ready.version === 1 && ready.setHash === setHash && Array.isArray(record.changes)
    && record.changes.every(item => shape(item, ['root', 'changeHash', 'targetPath'])), 'DEVELOPMENT_CHANGE_SET_RECORD_CHANGED');
  const changes = changeSetMembers(record.changes.map(change => change.root));
  need(changes.every((change, index) => change.changeHash === record.changes[index].changeHash
    && change.record.targetPath === record.changes[index].targetPath && change.record.accountRoot === record.accountRoot
    && change.record.targetRoot === record.targetRoot), 'DEVELOPMENT_CHANGE_SET_MEMBERS');
  const definition = read(join(root, 'definition.json'), 32768), compiled = integrationSpecification(definition, changes);
  need(sourceHash(encode(definition)) === record.definitionHash && sourceHash(encode(compiled)) === record.integrationHash
    && sourceHash(bytes(join(root, 'integration.json'), 49152)) === record.integrationHash
    && sourceHash(bytes(join(root, 'oracle.mjs'), 16384)) === record.oracleHash
    && sourceHash(bytes(join(project, 'verification', 'fixtures', 'development-integration-oracle.mjs'), 16384)) === record.oracleHash,
  'DEVELOPMENT_CHANGE_SET_INTEGRATION_CHANGED');
  const appliedCount = changes.filter(change => change.applied).length, intentPresent = existsSync(join(root, 'verification-intent.json'));
  if (intentPresent) {
    const intent = read(join(root, 'verification-intent.json'));
    need(shape(intent, ['version', 'setHash', 'ownerPid']) && intent.version === 1 && intent.setHash === setHash
      && Number.isSafeInteger(intent.ownerPid) && intent.ownerPid > 0, 'DEVELOPMENT_CHANGE_SET_VERIFICATION_CHANGED');
  }
  let verification = null;
  if (existsSync(join(root, 'verification-ready.json'))) {
    const seal = read(join(root, 'verification-ready.json')); verification = read(join(root, 'verification.json'));
    need(intentPresent && shape(seal, ['version', 'verificationHash']) && seal.version === 1 && seal.verificationHash === sourceHash(encode(verification))
      && shape(verification, ['version', 'setHash', 'entries', 'integration', 'passed', 'unchanged', 'wholeProjectVerified', 'longStageEvidence'])
      && verification.version === 1 && verification.setHash === setHash && typeof verification.passed === 'boolean'
      && typeof verification.unchanged === 'boolean' && verification.wholeProjectVerified === false && verification.longStageEvidence === false
      && Array.isArray(verification.entries) && verification.entries.length >= 1 && verification.entries.length <= changes.length
      && verification.entries.every((entry, index) => shape(entry, ['changeHash', 'verificationHash']) && changes[index].verification
        && entry.changeHash === changes[index].changeHash && entry.verificationHash === sourceHash(encode(changes[index].verification))),
    'DEVELOPMENT_CHANGE_SET_VERIFICATION_CHANGED');
    const integration = verification.integration;
    if (integration !== null) need(shape(integration, ['passed', 'checks', 'failures', 'exitCode', 'timedOut', 'stderrPresent'])
      && typeof integration.passed === 'boolean' && typeof integration.timedOut === 'boolean' && typeof integration.stderrPresent === 'boolean'
      && [integration.checks, integration.failures].every(value => value === null || Number.isSafeInteger(value) && value >= 0 && value <= 128)
      && (integration.exitCode === null || Number.isSafeInteger(integration.exitCode))
      && (!integration.passed || integration.checks === definition.cases.length && integration.failures === 0
        && integration.exitCode === 0 && !integration.timedOut && !integration.stderrPresent), 'DEVELOPMENT_CHANGE_SET_VERIFICATION_CHANGED');
    need(!verification.passed || appliedCount === changes.length && verification.entries.length === changes.length
      && changes.every(change => change.verification?.passed) && integration?.passed === true && verification.unchanged,
    'DEVELOPMENT_CHANGE_SET_VERIFICATION_CHANGED');
  }
  const uncertain = verification === null && (intentPresent || existsSync(join(root, 'verification.json')));
  return { root, record, setHash, changes, appliedCount, definition, verification,
    state: uncertain ? 'verification-uncertain' : verification ? verification.passed ? 'verified' : 'failed'
      : appliedCount === 0 ? 'not-applied' : appliedCount < changes.length ? 'partially-applied' : 'applied-unverified' };
}

export function verifyDevelopmentChangeSet(path) {
  const current = readDevelopmentChangeSet(path);
  need(current.state !== 'verification-uncertain', 'DEVELOPMENT_CHANGE_SET_VERIFICATION_UNCERTAIN');
  need(current.appliedCount === current.changes.length, 'DEVELOPMENT_CHANGE_SET_NOT_APPLIED');
  if (current.verification) return { ...current, testsStarted: 0 };
  create(join(current.root, 'verification-intent.json'), { version: 1, setHash: current.setHash, ownerPid: process.pid });
  let testsStarted = 0;
  const entries = [];
  for (const change of current.changes) {
    const checked = verifyDevelopmentChange(change.root); testsStarted += checked.testsStarted;
    entries.push({ changeHash: checked.changeHash, verificationHash: sourceHash(encode(checked.verification)) });
    if (!checked.verification.passed) break;
  }
  let integration = null;
  const before = readDevelopmentChangeSet(current.root);
  if (entries.length === before.changes.length && before.changes.every(change => change.verification?.passed)) {
    const oracle = join(current.root, 'oracle.mjs'), specification = join(current.root, 'integration.json');
    const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
    const result = spawnSync(process.execPath, ['--permission', ...[oracle, specification, ...before.changes.map(change => change.target)]
      .map(file => `--allow-fs-read=${file}`), oracle, specification],
    { cwd: current.record.targetRoot, env, encoding: 'utf8', timeout: 5000, maxBuffer: 16384, windowsHide: true });
    testsStarted++;
    let outcome = null; try { outcome = JSON.parse(result.stdout); } catch { }
    integration = { passed: !result.error && result.signal === null && result.status === 0 && result.stderr === ''
      && outcome?.passed === true && outcome.checks === current.definition.cases.length && Array.isArray(outcome.failures) && outcome.failures.length === 0,
    checks: Number.isSafeInteger(outcome?.checks) ? outcome.checks : null, failures: Array.isArray(outcome?.failures) ? outcome.failures.length : null,
    exitCode: result.status, timedOut: result.error?.code === 'ETIMEDOUT', stderrPresent: result.stderr !== '' };
  }
  const after = readDevelopmentChangeSet(current.root), unchanged = after.setHash === current.setHash && after.appliedCount === after.changes.length;
  const verification = { version: 1, setHash: current.setHash, entries, integration, passed: integration?.passed === true && unchanged,
    unchanged, wholeProjectVerified: false, longStageEvidence: false };
  create(join(current.root, 'verification.json'), verification);
  create(join(current.root, 'verification-ready.json'), { version: 1, verificationHash: sourceHash(encode(verification)) });
  return { ...readDevelopmentChangeSet(current.root), testsStarted };
}
