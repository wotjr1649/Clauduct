import { FAILURE_DIAGNOSTIC_CATEGORIES, EVENT_DIAGNOSTIC_TYPES, KEEPALIVE_SHAPES } from '../src/native-protocol.mjs';

const channels = ['stdout', 'stderr'];
const need = ok => { if (!ok) throw new Error('INVALID_OUTPUT_CAPTURE'); };

// Keep one pending line per pipe and only the final native result/status.
// Intermediate stream-json messages and raw diagnostics are never accumulated
// or written to disk. The caller still owns its watchdog and process cleanup.
export function createNativeOutputCapture({ maxBytes = 16777216, maxLineBytes = 1048576, maxRecords = 65536 } = {}) {
  need(Number.isSafeInteger(maxBytes) && maxBytes >= 1024 && maxBytes <= 268435456
    && Number.isSafeInteger(maxLineBytes) && maxLineBytes >= 256 && maxLineBytes <= 2097152 && maxLineBytes <= maxBytes
    && Number.isSafeInteger(maxRecords) && maxRecords >= 1 && maxRecords <= 262144);
  const pipes = Object.fromEntries(channels.map(name => [name,
    { buffer: Buffer.alloc(maxLineBytes), used: 0, bytes: 0, lines: 0, ended: false, watched: false }]));
  const decoder = new TextDecoder('utf-8', { fatal: true });
  let failure = null, bytes = 0, records = 0, peakBufferedBytes = 0, retainedBytes = 0;
  let nativeResult = null, status = null, resultCount = 0, statusCount = 0;
  function reject(code) { failure ??= code; }
  const pipeFor = name => { need(channels.includes(name)); return pipes[name]; };
  function line(name, pipe) {
    if (++records > maxRecords) { reject('OUTPUT_RECORD_LIMIT'); return; }
    pipe.lines++;
    let text;
    try { text = decoder.decode(pipe.buffer.subarray(0, pipe.used)); }
    catch { reject('OUTPUT_INVALID_UTF8'); return; }
    if (text.endsWith('\r')) text = text.slice(0, -1);
    if (name === 'stderr' && !text.startsWith('CLAUDUCT_REQUEST_STATUS ')) return;
    let value;
    try { value = JSON.parse(name === 'stderr' ? text.slice(24) : text); }
    catch { reject('OUTPUT_INVALID_JSON'); return; }
    if (!value || typeof value !== 'object' || Array.isArray(value)) { reject('OUTPUT_INVALID_SHAPE'); return; }
    if (name === 'stderr') {
      if (++statusCount !== 1) { reject('OUTPUT_DUPLICATE_STATUS'); return; }
      status = value; retainedBytes += pipe.used; return;
    }
    if (value.type === 'result') {
      if (++resultCount !== 1) { reject('OUTPUT_DUPLICATE_RESULT'); return; }
      if (typeof value.is_error !== 'boolean' || typeof value.session_id !== 'string'
        || !/^[0-9a-f-]{36}$/.test(value.session_id)
        || !value.is_error && typeof value.result !== 'string') { reject('OUTPUT_INVALID_RESULT'); return; }
      nativeResult = value; retainedBytes += pipe.used;
    } else if (resultCount) reject('OUTPUT_AFTER_RESULT');
    else if (typeof value.type !== 'string' || !/^[a-z_]{1,64}$/.test(value.type)) reject('OUTPUT_INVALID_SHAPE');
  }
  function push(name, chunk) {
    const pipe = pipeFor(name);
    need(Buffer.isBuffer(chunk));
    bytes = Math.min(Number.MAX_SAFE_INTEGER, bytes + chunk.length);
    pipe.bytes = Math.min(Number.MAX_SAFE_INTEGER, pipe.bytes + chunk.length);
    if (pipe.ended) reject('OUTPUT_AFTER_END');
    if (bytes > maxBytes) reject('OUTPUT_BYTE_LIMIT');
    if (failure) return;
    let offset = 0;
    while (offset < chunk.length && !failure) {
      const newline = chunk.indexOf(10, offset), end = newline < 0 ? chunk.length : newline;
      const length = end - offset;
      if (pipe.used + length > maxLineBytes) { reject('OUTPUT_LINE_LIMIT'); break; }
      chunk.copy(pipe.buffer, pipe.used, offset, end); pipe.used += length;
      peakBufferedBytes = Math.max(peakBufferedBytes, pipes.stdout.used + pipes.stderr.used);
      if (newline >= 0) { line(name, pipe); pipe.buffer.fill(0, 0, pipe.used); pipe.used = 0; }
      offset = newline < 0 ? chunk.length : newline + 1;
    }
  }
  function end(name) {
    const pipe = pipeFor(name);
    if (pipe.ended) { reject('OUTPUT_DUPLICATE_END'); return; }
    pipe.ended = true;
    if (pipe.used) reject('OUTPUT_TRUNCATED_LINE');
    pipe.buffer.fill(0); pipe.used = 0;
  }
  function watch(stream, name) {
    const pipe = pipeFor(name);
    need(stream && typeof stream.on === 'function' && !pipe.watched);
    pipe.watched = true;
    stream.on('data', chunk => push(name, chunk));
    stream.once('end', () => end(name));
    stream.on('error', () => reject('OUTPUT_PIPE_ERROR'));
    stream.once('close', () => { if (!pipe.ended) reject('OUTPUT_PIPE_CLOSED'); });
  }
  function evidence() {
    return { failure, bytes, records, resultCount, statusCount, retainedBytes, peakBufferedBytes,
      bufferCapacityBytes: 2 * maxLineBytes, maxBytes, maxLineBytes, maxRecords,
      stdoutEnded: pipes.stdout.ended, stderrEnded: pipes.stderr.ended,
      stdoutBytes: pipes.stdout.bytes, stderrBytes: pipes.stderr.bytes };
  }
  function snapshot() {
    return { nativeResult, status, valid: !failure && pipes.stdout.ended && pipes.stderr.ended
      && resultCount === 1 && statusCount === 1 && nativeResult !== null, evidence: evidence() };
  }
  return Object.freeze({ push, end, watch, evidence, snapshot });
}

// A result marker is only one part of completion. The independent task oracle,
// native exit and wrapper outcome must agree after both pipes actually end.
export function nativeOutputCompleted(snapshot, { exitCode, sessionId, resultText, oraclePassed }) {
  const cleanup = snapshot.status?.cleanup;
  const cleanupKeys = ['childClosed', 'gatewaySocketsClosed', 'gatewayJobsClosed', 'gatewayTimersClosed',
    'gatewayDeliveriesClosed', 'gatewayIdle', 'gatewayCleanupCompleted', 'transportSocketsClosed', 'transportRequestsClosed'];
  return snapshot.valid === true && exitCode === 0 && oraclePassed === true
    && snapshot.nativeResult?.is_error === false && snapshot.nativeResult.session_id === sessionId
    && snapshot.nativeResult.result === resultText && snapshot.status?.requestOutcome === 'all-succeeded'
    && cleanup && Object.keys(cleanup).length === cleanupKeys.length && cleanupKeys.every(key => cleanup[key] === true) ? true : false;
}

// Durable development evidence uses this projection, never the raw status,
// native result text, request history, event fields or arbitrary error strings.
export function nativeOutputDiagnostics(snapshot) {
  const status = snapshot?.status;
  const counter = value => Number.isSafeInteger(value) && value >= 0 ? value : null;
  const rows = Array.isArray(status?.failureHistory?.records) ? status.failureHistory.records.slice(0, 16) : [];
  const requestOutcome = ['all-succeeded', 'has-failures', 'no-requests', 'in-progress', 'not-observed'].includes(status?.requestOutcome)
    ? status.requestOutcome : 'not-observed';
  const failures = rows.map(row => ({
    category: FAILURE_DIAGNOSTIC_CATEGORIES.includes(row?.failureCategory) ? row.failureCategory : null,
    eventKind: EVENT_DIAGNOSTIC_TYPES.includes(row?.unsupportedEvent) ? row.unsupportedEvent : null,
    keepaliveShape: KEEPALIVE_SHAPES.includes(row?.keepaliveShape) ? row.keepaliveShape : null
  }));
  return { requestOutcome,
    started: counter(status?.lifetime?.started), succeeded: counter(status?.lifetime?.succeeded), failed: counter(status?.lifetime?.failed),
    failures,
    failure: requestOutcome === 'has-failures' ? failures.find(row => row.category)?.category ?? 'NATIVE_REQUEST_FAILED'
      : snapshot?.nativeResult?.is_error === true ? 'NATIVE_RESULT_ERROR' : null };
}
