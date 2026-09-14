import { openSync, closeSync, writeSync, readFileSync, lstatSync, realpathSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { NativeError } from '../src/native-protocol.mjs';

const maximumBytes = 262144;
const legacyCounters = ['requestAttempts', 'inputTokens', 'outputTokens', 'completions', 'imageFormatMask'];
const counters = [...legacyCounters, 'unobservedCompletions'];
const kinds = ['start', 'attempt', 'usage', 'usage-unobserved', 'final'];
const fail = () => { throw new NativeError('VERIFICATION_LEDGER_INVALID'); };
const zero = () => Object.fromEntries(counters.map(key => [key, 0]));
function snapshot(value, previous) {
  if (!value || typeof value !== 'object' || Array.isArray(value)
    || Object.keys(value).some(key => ![...counters, 'maxInputTokens', 'maxOutputTokens'].includes(key))) fail();
  const next = {};
  for (const key of counters) {
    const current = Object.hasOwn(value, key) ? value[key] : previous[key];
    if (!Number.isSafeInteger(current) || current < previous[key]) fail();
    next[key] = current;
  }
  if (next.requestAttempts > 4096 || next.completions + next.unobservedCompletions > next.requestAttempts || next.imageFormatMask > 15
    || (next.imageFormatMask & previous.imageFormatMask) !== previous.imageFormatMask) fail();
  return next;
}

// This records process-crash evidence, not power-loss durability. Each bounded
// write completes before a network attempt or tool completion may be released.
export function createVerificationLedger(directory) {
  const root = resolve(directory);
  if (!lstatSync(root).isDirectory() || realpathSync(root).toLowerCase() !== root.toLowerCase()) fail();
  const path = join(root, 'tool-usage.jsonl');
  const fd = openSync(path, 'wx');
  let state = zero(), sequence = 0, bytes = 0, closed = false, failed = false, final = false;
  function record(kind, value = state) {
    if (closed || failed || final || !kinds.includes(kind) || (kind === 'start') !== (sequence === 0)) fail();
    const next = snapshot(value, state);
    if (kind === 'attempt' && next.requestAttempts !== state.requestAttempts + 1) fail();
    if (kind !== 'attempt' && next.requestAttempts !== state.requestAttempts) fail();
    if (kind === 'usage' ? next.completions !== state.completions + 1 : next.completions !== state.completions) fail();
    if (kind === 'usage-unobserved' ? next.unobservedCompletions !== state.unobservedCompletions + 1
      : next.unobservedCompletions !== state.unobservedCompletions) fail();
    if (kind === 'usage-unobserved' && (next.inputTokens !== state.inputTokens || next.outputTokens !== state.outputTokens)) fail();
    const line = Buffer.from(JSON.stringify({ version: 2, sequence: sequence + 1, kind, ...next }) + '\n');
    if (bytes + line.length > maximumBytes) fail();
    try {
      if (writeSync(fd, line) !== line.length) throw new Error();
    } catch { failed = true; throw new NativeError('VERIFICATION_LEDGER_IO'); }
    bytes += line.length; sequence++; state = next; final = kind === 'final';
  }
  const close = () => { if (!closed) { closed = true; closeSync(fd); } };
  try { record('start'); } catch (error) { close(); throw error; }
  return Object.freeze({ path, record, close });
}

export function readVerificationLedger(path) {
  const info = lstatSync(path);
  if (!info.isFile() || info.isSymbolicLink() || info.size > maximumBytes
    || realpathSync(path).toLowerCase() !== resolve(path).toLowerCase()) fail();
  let text;
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(path)); } catch { fail(); }
  const boundary = text.lastIndexOf('\n');
  if (boundary < 0) fail();
  const truncatedTail = boundary !== text.length - 1;
  const rows = text.slice(0, boundary).split('\n');
  let state = zero(), finalRecorded = false, version = null;
  for (let index = 0; index < rows.length; index++) {
    let row;
    try { row = JSON.parse(rows[index]); } catch { fail(); }
    const rowCounters = row?.version === 1 ? legacyCounters : counters;
    if (!row || ![1, 2].includes(row.version) || version !== null && version !== row.version
      || row.sequence !== index + 1 || !kinds.includes(row.kind) || row.version === 1 && row.kind === 'usage-unobserved' || finalRecorded
      || Object.keys(row).length !== rowCounters.length + 3
      || Object.keys(row).some(key => ![...rowCounters, 'version', 'sequence', 'kind'].includes(key))
      || (row.kind === 'start') !== (index === 0)) fail();
    version = row.version;
    const next = snapshot(Object.fromEntries(rowCounters.map(key => [key, row[key]])), state);
    if (row.kind === 'attempt' ? next.requestAttempts !== state.requestAttempts + 1 : next.requestAttempts !== state.requestAttempts) fail();
    if (row.kind === 'usage' ? next.completions !== state.completions + 1 : next.completions !== state.completions) fail();
    if (row.kind === 'usage-unobserved' ? next.unobservedCompletions !== state.unobservedCompletions + 1
      : next.unobservedCompletions !== state.unobservedCompletions) fail();
    if (row.kind === 'usage-unobserved' && (next.inputTokens !== state.inputTokens || next.outputTokens !== state.outputTokens)) fail();
    if (index === 0 && counters.some(key => next[key] !== 0)) fail();
    state = next; finalRecorded = row.kind === 'final';
  }
  if (truncatedTail && finalRecorded) fail();
  return { ...state, version, recordCount: rows.length, finalRecorded, truncatedTail };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.stdout.write(JSON.stringify(readVerificationLedger(process.argv[2])) + '\n'); }
  catch { process.stderr.write('VERIFICATION_LEDGER_UNAVAILABLE\n'); process.exitCode = 1; }
}
