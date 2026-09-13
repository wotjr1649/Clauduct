import { openSync, closeSync, writeSync, readFileSync, lstatSync, realpathSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';

const dimensions = ['attempts', 'inputTokens', 'outputTokens', 'elapsedMs'];
const hash = value => createHash('sha256').update(value).digest('hex');
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const integer = value => Number.isSafeInteger(value) && value >= 0;
const shape = (value, keys) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const need = (ok, code = 'EXECUTION_RESERVATION_INVALID') => { if (!ok) throw new Error(code); };
function vector(value, nullable = false) {
  need(shape(value, dimensions) && dimensions.every(key => integer(value[key]) || nullable && value[key] === null));
  return Object.fromEntries(dimensions.map(key => [key, value[key]]));
}
function directory(path) {
  const root = resolve(path), info = lstatSync(root);
  need(info.isDirectory() && !info.isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase(), 'EXECUTION_RESERVATION_PATH');
  return root;
}
function read(path) {
  const info = lstatSync(path);
  need(info.isFile() && !info.isSymbolicLink() && info.size <= 16384, 'EXECUTION_RESERVATION_PATH');
  const text = new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(path));
  const value = JSON.parse(text);
  need(JSON.stringify(value) + '\n' === text, 'EXECUTION_RESERVATION_ENCODING');
  return { value, sha256: hash(text) };
}
function create(path, value) {
  const bytes = Buffer.from(JSON.stringify(value) + '\n');
  need(bytes.length <= 16384);
  const fd = openSync(path, 'wx');
  try { need(writeSync(fd, bytes) === bytes.length, 'EXECUTION_RESERVATION_IO'); }
  finally { closeSync(fd); }
  return hash(bytes);
}
function reservation(root) {
  const record = read(join(root, 'execution-reservation.json')), value = record.value;
  need(shape(value, ['version', 'id', 'directoryHash', 'basisHash', 'previous', 'limits', 'allowance'])
    && value.version === 1 && typeof value.id === 'string' && /^[a-f0-9-]{36}$/.test(value.id)
    && value.directoryHash === hash(root.toLowerCase()) && digest(value.basisHash));
  const previous = vector(value.previous), limits = vector(value.limits), allowance = vector(value.allowance);
  need(allowance.attempts > 0 && allowance.elapsedMs > 0 && dimensions.every(key =>
    Number.isSafeInteger(previous[key] + allowance[key]) && previous[key] + allowance[key] <= limits[key]), 'EXECUTION_RESERVATION_LIMIT');
  return { ...record, previous, limits, allowance };
}

// One immutable reservation per owned execution directory. The caller retains
// the global run-owner/admission lock and independently verifies any settlement.
// These files preserve process-crash evidence, not power-loss durability.
export function createExecutionReservation(path, details) {
  const root = directory(path);
  need(shape(details, ['basisHash', 'previous', 'limits', 'allowance']) && digest(details.basisHash));
  const previous = vector(details.previous), limits = vector(details.limits), allowance = vector(details.allowance);
  need(allowance.attempts > 0 && allowance.elapsedMs > 0 && dimensions.every(key =>
    Number.isSafeInteger(previous[key] + allowance[key]) && previous[key] + allowance[key] <= limits[key]), 'EXECUTION_RESERVATION_LIMIT');
  need(!existsSync(join(root, 'execution-settlement.json')), 'EXECUTION_RESERVATION_CONFLICT');
  const value = { version: 1, id: randomUUID(), directoryHash: hash(root.toLowerCase()), basisHash: details.basisHash,
    previous, limits, allowance };
  create(join(root, 'execution-reservation.json'), value);
  return readExecutionReservation(root);
}
export function settleExecutionReservation(path, details) {
  const root = directory(path), original = reservation(root);
  need(shape(details, ['observed', 'evidenceHash']) && digest(details.evidenceHash));
  const observed = vector(details.observed, true);
  need(dimensions.every(key => observed[key] === null || Number.isSafeInteger(original.previous[key] + observed[key])));
  create(join(root, 'execution-settlement.json'), { version: 1, reservationHash: original.sha256,
    evidenceHash: details.evidenceHash, observed });
  return readExecutionReservation(root);
}
export function readExecutionReservation(path) {
  const root = directory(path), original = reservation(root);
  let observed = null, evidenceHash = null, settlementState = 'missing';
  if (existsSync(join(root, 'execution-settlement.json'))) {
    try {
      const value = read(join(root, 'execution-settlement.json')).value;
      need(shape(value, ['version', 'reservationHash', 'evidenceHash', 'observed']) && value.version === 1
        && value.reservationHash === original.sha256 && digest(value.evidenceHash));
      const candidate = vector(value.observed, true);
      need(dimensions.every(key => candidate[key] === null || Number.isSafeInteger(original.previous[key] + candidate[key])));
      observed = candidate; evidenceHash = value.evidenceHash; settlementState = 'recorded';
    } catch { settlementState = 'invalid'; }
  }
  const charged = {}, retained = {};
  for (const key of dimensions) {
    retained[key] = observed?.[key] == null;
    charged[key] = original.previous[key] + (retained[key] ? original.allowance[key] : observed[key]);
  }
  const overrun = observed !== null && dimensions.some(key => observed[key] !== null && observed[key] > original.allowance[key]);
  return { reservationHash: original.sha256, basisHash: original.value.basisHash,
    previous: original.previous, limits: original.limits, allowance: original.allowance, charged, retained,
    settlementState, evidenceHash, overrun, withinLimits: dimensions.every(key => charged[key] <= original.limits[key]),
    observationsIndependentlyVerified: false, authorizesExecution: false, powerLossDurability: 'NOT_RUN' };
}
