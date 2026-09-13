import { lstatSync, realpathSync, openSync, fstatSync, readSync, closeSync, writeFileSync, mkdirSync, mkdtempSync, existsSync } from 'node:fs';
import { join, resolve, dirname, basename, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { createExecutionAccount, readExecutionAccount } from './execution-account.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const encode = value => JSON.stringify(value) + '\n';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const need = (ok, code = 'MANAGED_LEDGER_INVALID') => { if (!ok) throw new Error(code); };
const shape = (value, fields) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === fields.length && fields.every(key => Object.hasOwn(value, key));
const wholeFields = ['attemptLower', 'attemptUpper', 'knownPreConnectionFaultAttempts', 'knownPreRequestServiceSignals',
  'knownInputTokens', 'knownOutputTokens', 'nativePhaseElapsedMs', 'unobservedNativeDurationRuns',
  'unobservedNativeDurationReservationMs', 'inFlightUsageUnobservedRuns', 'inFlightInputReservation',
  'inFlightOutputReservation', 'earlierFullInputReservation', 'earlierFullOutputReservation'];
const flagFields = ['nativePhaseElapsedMsIsPartial', 'earlierTimeoutAttemptRangeUnresolved', 'firstFailureUsageUnobserved', 'searchTokenUsageNotReported'];
const capFields = { attempts: 'cumulativeAttemptUpper', inputTokens: 'cumulativeKnownAndReservedInputTokens',
  outputTokens: 'cumulativeKnownAndReservedOutputTokens', elapsedMs: 'cumulativeKnownAndReservedNativeMs' };
const ledgerFields = new Set([...wholeFields, ...flagFields, 'priorLedger', 'completedRuns', 'allPassed', 'releaseVerdict', 'results', 'priorReconciliation']);
const manifestFields = new Set(['version', 'endpoint', 'taskId', 'candidateBase', 'sourceHashes', 'priorLedger', 'priorLedgerHash',
  'models', 'efforts', 'phaseMs', 'wrapperMs', 'requestLimit', 'maxTurns', 'maxObservedInputTokens', 'maxObservedOutputTokens',
  ...Object.values(capFields), 'basis', 'intent', 'prelaunchReservation']);
function directory(path) {
  need(typeof path === 'string', 'MANAGED_LEDGER_PATH');
  const root = resolve(path), info = lstatSync(root);
  need(info.isDirectory() && !info.isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase(), 'MANAGED_LEDGER_PATH');
  return root;
}
function readJson(path) {
  let fd;
  try {
    const before = lstatSync(path, { bigint: true });
    need(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.size > 0n && before.size <= 65536n
      && realpathSync(path).toLowerCase() === resolve(path).toLowerCase());
    fd = openSync(path, 'r');
    const opened = fstatSync(fd, { bigint: true });
    need(opened.dev === before.dev && opened.ino === before.ino && opened.size === before.size);
    const buffer = Buffer.alloc(65537); let length = 0;
    while (length < buffer.length) {
      const count = readSync(fd, buffer, length, buffer.length - length, length);
      if (count === 0) break;
      length += count;
    }
    const after = fstatSync(fd, { bigint: true });
    need(BigInt(length) === before.size && after.size === before.size && after.mtimeNs === before.mtimeNs);
    const bytes = buffer.subarray(0, length), text = new TextDecoder('utf-8', { fatal: true }).decode(bytes), value = JSON.parse(text);
    need(value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length <= 128);
    // Accounting fields are top-level. Reject duplicate spellings, including
    // escaped aliases, before selecting these fields; nested payloads are inert.
    const seen = new Set(); let depth = 0;
    for (let index = 0; index < text.length; index++) {
      if (text[index] === '{' || text[index] === '[') depth++;
      else if (text[index] === '}' || text[index] === ']') depth--;
      else if (text[index] === '"') {
        const start = index++;
        while (text[index] !== '"') { if (text[index] === '\\') index++; index++; }
        let next = index + 1; while (/\s/.test(text[next] ?? '') && next < text.length) next++;
        if (depth === 1 && text[next] === ':') {
          const key = JSON.parse(text.slice(start, index + 1)); need(!seen.has(key)); seen.add(key);
        }
      }
    }
    return { value, hash: hash(bytes) };
  } catch { throw new Error('MANAGED_LEDGER_RECORD_INVALID'); }
  finally { if (fd !== undefined) closeSync(fd); }
}
function write(path, value) { writeFileSync(path, encode(value), { flag: 'wx' }); }
function sum(...values) {
  const total = values.reduce((a, b) => a + b, 0);
  need(Number.isSafeInteger(total), 'MANAGED_LEDGER_COUNTER_INVALID'); return total;
}
export function projectLegacyExecutionLedger(ledger, manifest) {
  need(ledger && manifest && !Array.isArray(ledger) && !Array.isArray(manifest), 'MANAGED_LEDGER_COUNTER_INVALID');
  need(Object.keys(ledger).every(key => ledgerFields.has(key)) && Object.keys(manifest).every(key => manifestFields.has(key)), 'MANAGED_LEDGER_FIELDS_UNSUPPORTED');
  for (const key of wholeFields) need(Object.hasOwn(ledger, key) && Number.isSafeInteger(ledger[key]) && ledger[key] >= 0, 'MANAGED_LEDGER_COUNTER_INVALID');
  for (const key of flagFields) need(Object.hasOwn(ledger, key) && typeof ledger[key] === 'boolean', 'MANAGED_LEDGER_COUNTER_INVALID');
  need(manifest.version === 1 && ledger.attemptLower <= ledger.attemptUpper
    && (!ledger.earlierTimeoutAttemptRangeUnresolved || ledger.attemptLower < ledger.attemptUpper)
    && (ledger.inFlightUsageUnobservedRuns === 0 ? ledger.inFlightInputReservation === 0 && ledger.inFlightOutputReservation === 0
      : ledger.inFlightInputReservation > 0 && ledger.inFlightOutputReservation > 0)
    && (ledger.unobservedNativeDurationRuns === 0 ? ledger.unobservedNativeDurationReservationMs === 0
      : ledger.nativePhaseElapsedMsIsPartial && ledger.unobservedNativeDurationReservationMs > 0)
    && (!(ledger.firstFailureUsageUnobserved || ledger.searchTokenUsageNotReported)
      || ledger.earlierFullInputReservation > 0 && ledger.earlierFullOutputReservation > 0), 'MANAGED_LEDGER_COUNTER_INVALID');
  const initial = { attempts: ledger.attemptUpper,
    inputTokens: sum(ledger.knownInputTokens, ledger.inFlightInputReservation, ledger.earlierFullInputReservation),
    outputTokens: sum(ledger.knownOutputTokens, ledger.inFlightOutputReservation, ledger.earlierFullOutputReservation),
    elapsedMs: sum(ledger.nativePhaseElapsedMs, ledger.unobservedNativeDurationReservationMs) };
  const limits = Object.fromEntries(Object.entries(capFields).map(([key, field]) => {
    need(Object.hasOwn(manifest, field) && Number.isSafeInteger(manifest[field]) && manifest[field] >= 0, 'MANAGED_LEDGER_COUNTER_INVALID');
    return [key, manifest[field]];
  }));
  const components = Object.fromEntries([...wholeFields, ...flagFields].map(key => [key, ledger[key]]));
  return Object.freeze({ initial: Object.freeze(initial), limits: Object.freeze(limits), components: Object.freeze(components),
    withinLimits: Object.keys(limits).every(key => initial[key] <= limits[key]) });
}
export function readLegacyExecutionLedger(sourceDirectory, { ledgerHash, manifestHash }) {
  need(digest(ledgerHash) && digest(manifestHash), 'MANAGED_LEDGER_HASH_INVALID');
  const source = directory(sourceDirectory), ledger = readJson(join(source, 'result.json')), manifest = readJson(join(source, 'manifest.json'));
  need(ledger.hash === ledgerHash && manifest.hash === manifestHash, 'MANAGED_LEDGER_SOURCE_CHANGED');
  const projection = projectLegacyExecutionLedger(ledger.value, manifest.value);
  return { sourceDirectory: source, ledgerHash, manifestHash, projection,
    basisHash: hash(encode({ ledgerHash, manifestHash, projection })) };
}
function accountPath(path) {
  const root = directory(path);
  need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase() && /^managed-development-[A-Za-z0-9]{6}$/.test(basename(root)), 'MANAGED_LEDGER_PATH');
  return root;
}
function scoped(scope, path) {
  const prefix = scope.replace(/[\\/]+$/, '').toLowerCase() + sep;
  return path.toLowerCase() === scope.toLowerCase() || path.toLowerCase().startsWith(prefix);
}
export function createManagedLedgerAccount({ sourceDirectory, ledgerHash, manifestHash, localNative = false, scopeRoot }) {
  need(typeof localNative === 'boolean');
  need(scopeRoot !== undefined || localNative, 'MANAGED_LEDGER_SCOPE_REQUIRED');
  const source = readLegacyExecutionLedger(sourceDirectory, { ledgerHash, manifestHash });
  const scope = directory(scopeRoot ?? project);
  need(scoped(scope, project) && scoped(scope, source.sourceDirectory), 'MANAGED_LEDGER_SCOPE_INVALID');
  need(source.projection.withinLimits, 'MANAGED_LEDGER_LIMIT');
  const temp = join(project, '.tmp'); if (!existsSync(temp)) mkdirSync(temp); directory(temp);
  const claimRoot = join(source.sourceDirectory, '.managed-execution-account');
  need(!existsSync(claimRoot), 'MANAGED_LEDGER_ALREADY_BOUND');
  const scopeTemp = join(scope, '.tmp'); if (!existsSync(scopeTemp)) mkdirSync(scopeTemp); directory(scopeTemp);
  const registry = join(scopeTemp, 'managed-ledger-registry'); if (!existsSync(registry)) {
    try { mkdirSync(registry); } catch (error) { if (error.code !== 'EEXIST') throw error; }
  }
  directory(registry);
  const registryClaim = join(registry, source.basisHash);
  try { mkdirSync(registryClaim); } catch (error) { if (error.code === 'EEXIST') throw new Error('MANAGED_LEDGER_ALREADY_BOUND'); throw error; }
  try { mkdirSync(claimRoot); } catch (error) { if (error.code === 'EEXIST') throw new Error('MANAGED_LEDGER_ALREADY_BOUND'); throw error; }
  const root = mkdtempSync(join(temp, 'managed-development-'));
  const claim = { version: 1, scopeRoot: scope, project, root, ledgerHash, manifestHash, localNative, basisHash: source.basisHash };
  write(join(claimRoot, 'claim.json'), claim);
  const account = createExecutionAccount(root, { basisHash: source.basisHash, previous: source.projection.initial, limits: source.projection.limits });
  const provenance = { version: 1, scopeRoot: scope, sourceDirectory: source.sourceDirectory, ledgerHash, manifestHash, localNative,
    basisHash: source.basisHash, components: source.projection.components };
  write(join(root, 'legacy-ledger.json'), provenance);
  const ready = { version: 1, claimHash: hash(encode(claim)), accountHash: account.accountHash, provenanceHash: hash(encode(provenance)) };
  write(join(claimRoot, 'ready.json'), ready); write(join(registryClaim, 'ready.json'), ready);
  return readManagedLedgerAccount(root, { localNative });
}
export function readManagedLedgerAccount(path, { localNative } = {}) {
  const root = accountPath(path);
  need(existsSync(join(root, 'legacy-ledger.json')), 'MANAGED_LEDGER_BINDING_REQUIRED');
  const record = readJson(join(root, 'legacy-ledger.json')), provenance = record.value;
  need(shape(provenance, ['version', 'scopeRoot', 'sourceDirectory', 'ledgerHash', 'manifestHash', 'localNative', 'basisHash', 'components'])
    && provenance.version === 1 && typeof provenance.localNative === 'boolean');
  need(localNative === undefined || localNative === provenance.localNative, 'MANAGED_LEDGER_MODE_MISMATCH');
  const source = readLegacyExecutionLedger(provenance.sourceDirectory, provenance);
  const scope = directory(provenance.scopeRoot);
  need(scoped(scope, project) && scoped(scope, source.sourceDirectory), 'MANAGED_LEDGER_SCOPE_INVALID');
  const claimRoot = directory(join(source.sourceDirectory, '.managed-execution-account'));
  const claim = readJson(join(claimRoot, 'claim.json')), ready = readJson(join(claimRoot, 'ready.json')).value;
  const registry = directory(join(scope, '.tmp', 'managed-ledger-registry'));
  const registered = readJson(join(directory(join(registry, source.basisHash)), 'ready.json')).value;
  const account = readExecutionAccount(root);
  need(shape(ready, ['version', 'claimHash', 'accountHash', 'provenanceHash']) && ready.version === 1
    && ready.claimHash === claim.hash && ready.accountHash === account.accountHash && ready.provenanceHash === record.hash
    && encode(registered) === encode(ready)
    && encode(claim.value) === encode({ version: 1, scopeRoot: scope, project, root, ledgerHash: source.ledgerHash,
      manifestHash: source.manifestHash, localNative: provenance.localNative, basisHash: source.basisHash })
    && provenance.basisHash === source.basisHash && account.basisHash === source.basisHash
    && encode(provenance.components) === encode(source.projection.components)
    && encode(account.initial) === encode(source.projection.initial) && encode(account.limits) === encode(source.projection.limits), 'MANAGED_LEDGER_BINDING_INVALID');
  return { root, sourceDirectory: source.sourceDirectory, localNative: provenance.localNative, account,
    components: source.projection.components, ledgerHash: source.ledgerHash, manifestHash: source.manifestHash };
}
export function openManagedLedgerSource(sourceDirectory) {
  const source = directory(sourceDirectory), claim = readJson(join(directory(join(source, '.managed-execution-account')), 'claim.json')).value;
  const opened = readManagedLedgerAccount(claim.root);
  need(opened.sourceDirectory === source, 'MANAGED_LEDGER_BINDING_INVALID'); return opened;
}
