import { readFileSync, writeFileSync, lstatSync, realpathSync, readdirSync, mkdirSync, existsSync } from 'node:fs';
import { join, resolve, dirname, basename } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';
import { createExecutionReservation, readExecutionReservation, settleExecutionReservation } from './execution-reservation.mjs';

const keys = ['attempts', 'inputTokens', 'outputTokens', 'elapsedMs'];
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const encode = value => JSON.stringify(value) + '\n';
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const shape = (value, fields) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === fields.length && fields.every(key => Object.hasOwn(value, key));
const need = (ok, code = 'EXECUTION_ACCOUNT_INVALID') => { if (!ok) throw new Error(code); };
const vector = value => shape(value, keys) && keys.every(key => Number.isSafeInteger(value[key]) && value[key] >= 0);
const same = (left, right) => keys.every(key => left[key] === right[key]);
function directory(path) {
  const root = resolve(path), info = lstatSync(root);
  need(info.isDirectory() && !info.isSymbolicLink() && realpathSync(root).toLowerCase() === root.toLowerCase(), 'EXECUTION_ACCOUNT_PATH');
  return root;
}
function read(path) {
  const info = lstatSync(path);
  need(info.isFile() && !info.isSymbolicLink() && info.size <= 16384, 'EXECUTION_ACCOUNT_PATH');
  const bytes = readFileSync(path), text = new TextDecoder('utf-8', { fatal: true }).decode(bytes), value = JSON.parse(text);
  need(encode(value) === text);
  return { value, hash: hash(bytes) };
}
function create(path, value) {
  const text = encode(value); need(Buffer.byteLength(text) <= 16384);
  writeFileSync(path, text, { flag: 'wx' });
}
function ownerStopped(pid) {
  try { process.kill(pid, 0); return false; }
  catch (error) { need(error.code === 'ESRCH', 'EXECUTION_ACCOUNT_OWNER_UNCERTAIN'); return true; }
}
function manifest(root) {
  const record = read(join(root, 'account.json')), value = record.value;
  need(shape(value, ['version', 'directoryHash', 'basisHash', 'previous', 'limits']) && value.version === 1
    && value.directoryHash === hash(root.toLowerCase()) && digest(value.basisHash)
    && vector(value.previous) && vector(value.limits) && keys.every(key => value.previous[key] <= value.limits[key]));
  return record;
}

// The initial balance includes earlier unobserved reservations. Closing a new
// entry can reconcile only that entry; it cannot reduce the initial balance.
export function createExecutionAccount(path, details) {
  const root = directory(path);
  need(shape(details, ['basisHash', 'previous', 'limits']) && digest(details.basisHash)
    && vector(details.previous) && vector(details.limits) && keys.every(key => details.previous[key] <= details.limits[key]));
  need(readdirSync(root).length === 0, 'EXECUTION_ACCOUNT_EXISTS');
  create(join(root, 'account.json'), { version: 1, directoryHash: hash(root.toLowerCase()), ...details });
  mkdirSync(join(root, 'executions'));
  return readExecutionAccount(root);
}

export function readExecutionAccount(path) {
  const root = directory(path), base = manifest(root), executions = directory(join(root, 'executions'));
  const names = readdirSync(executions).sort();
  need(names.length <= 4096 && names.every((name, index) => name === String(index + 1).padStart(6, '0')));
  let charged = base.value.previous;
  const entries = [];
  for (const [index, name] of names.entries()) {
    const entry = directory(join(executions, name));
    let intent, state;
    try {
      intent = read(join(entry, 'intent.json'));
      need(shape(intent.value, ['version', 'accountHash', 'index', 'executionHash', 'ownerPid', 'ownerNonce'])
        && intent.value.version === 1 && intent.value.accountHash === base.hash && intent.value.index === index + 1
        && digest(intent.value.executionHash) && Number.isSafeInteger(intent.value.ownerPid) && intent.value.ownerPid > 0
        && typeof intent.value.ownerNonce === 'string' && /^[a-f0-9-]{36}$/.test(intent.value.ownerNonce));
      state = readExecutionReservation(entry);
      need(state.basisHash === intent.hash && same(state.previous, charged) && same(state.limits, base.value.limits));
    } catch { need(false, 'EXECUTION_ACCOUNT_ENTRY_INCOMPLETE'); }
    const closed = existsSync(join(entry, 'closure.json'));
    if (closed) {
      const closure = read(join(entry, 'closure.json')).value;
      need(shape(closure, ['version', 'reservationHash', 'stateHash', 'evidenceHash']) && closure.version === 1
        && state.settlementState === 'recorded' && closure.reservationHash === state.reservationHash && closure.stateHash === hash(encode(state))
        && digest(closure.evidenceHash) && closure.evidenceHash === state.evidenceHash, 'EXECUTION_ACCOUNT_CLOSURE_INVALID');
    }
    need(closed || index === names.length - 1, 'EXECUTION_ACCOUNT_PENDING_PREDECESSOR');
    // An unclosed entry has no acknowledged reconciliation. Retain its full
    // reservation even when its settlement file was written before a crash.
    charged = closed ? state.charged : Object.fromEntries(keys.map(key =>
      [key, Math.max(state.charged[key], state.previous[key] + state.allowance[key])]));
    entries.push({ entry, index: index + 1, ownerPid: intent.value.ownerPid, executionHash: intent.value.executionHash,
      closed, charged, reservation: state });
  }
  return { root, accountHash: base.hash, basisHash: base.value.basisHash, initial: base.value.previous, limits: base.value.limits,
    charged, entries, pending: entries.at(-1)?.closed === false ? entries.at(-1).entry : null,
    withinLimits: keys.every(key => charged[key] <= base.value.limits[key]), powerLossDurability: 'NOT_RUN' };
}

export function reserveExecutionAccount(path, { allowance, executionHash }) {
  const state = readExecutionAccount(path);
  need(state.pending === null, 'EXECUTION_ACCOUNT_PENDING');
  need(vector(allowance) && allowance.attempts > 0 && allowance.elapsedMs > 0 && digest(executionHash));
  need(state.entries.length < 4096 && keys.every(key => Number.isSafeInteger(state.charged[key] + allowance[key])
    && state.charged[key] + allowance[key] <= state.limits[key]), 'EXECUTION_ACCOUNT_LIMIT');
  const index = state.entries.length + 1, entry = join(state.root, 'executions', String(index).padStart(6, '0'));
  // This exclusive directory claim is also the execution-owner claim. Two
  // readers of the same previous balance cannot both reserve the next slot.
  try { mkdirSync(entry); }
  catch (error) { if (error.code === 'EEXIST') need(false, 'EXECUTION_ACCOUNT_BUSY'); throw error; }
  const ownerNonce = randomUUID();
  create(join(entry, 'intent.json'), { version: 1, accountHash: state.accountHash, index, executionHash, ownerPid: process.pid, ownerNonce });
  const reservation = createExecutionReservation(entry, { basisHash: read(join(entry, 'intent.json')).hash,
    previous: state.charged, limits: state.limits, allowance });
  return { entry, ownerNonce, previous: state.charged, reservation };
}

// The application must independently verify the native owner and observations.
// A dead manager PID alone never proves its native children have stopped.
export function closeExecutionAccount(entryPath, { ownerNonce, observed, evidenceHash }) {
  const entry = directory(entryPath), root = dirname(dirname(entry));
  need(basename(dirname(entry)) === 'executions');
  const account = readExecutionAccount(root), current = account.entries.at(-1);
  need(current?.entry === entry && account.pending === entry, 'EXECUTION_ACCOUNT_NOT_PENDING');
  const intent = read(join(entry, 'intent.json')).value;
  need(intent.ownerPid === process.pid ? ownerNonce === intent.ownerNonce : ownerStopped(intent.ownerPid), 'EXECUTION_ACCOUNT_OWNER_ACTIVE');
  need(digest(evidenceHash));
  let state = readExecutionReservation(entry);
  if (state.settlementState === 'missing') state = settleExecutionReservation(entry, { observed, evidenceHash });
  else {
    need(state.settlementState === 'recorded' && state.evidenceHash === evidenceHash
      && shape(observed, keys) && keys.every(key => observed[key] === null ? state.retained[key]
        : !state.retained[key] && Number.isSafeInteger(observed[key]) && observed[key] >= 0
          && state.charged[key] === state.previous[key] + observed[key]), 'EXECUTION_ACCOUNT_SETTLEMENT_CONFLICT');
  }
  create(join(entry, 'closure.json'), { version: 1, reservationHash: state.reservationHash, stateHash: hash(encode(state)), evidenceHash });
  return readExecutionAccount(root);
}
