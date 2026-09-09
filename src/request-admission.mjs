import { freemem, totalmem } from 'node:os';
import { NativeError, need } from './native-protocol.mjs';

const MiB = 1024 * 1024;
// Reserve for body decoding, request conversion and a bounded response in addition to OS headroom.
export function createAdmission({ freeBytes = () => Math.min(freemem(), process.availableMemory?.() ?? Infinity),
  headroomBytes = Math.max(256 * MiB, Math.min(totalmem() / 10, 1024 * MiB)),
  requestReserveBytes = 128 * MiB, maxQueued = 128, pollMs = 250 } = {}) {
  need(typeof freeBytes === 'function' && Number.isFinite(headroomBytes) && headroomBytes >= 0
    && Number.isFinite(requestReserveBytes) && requestReserveBytes > 0
    && Number.isInteger(maxQueued) && maxQueued > 0 && Number.isInteger(pollMs) && pollMs > 0, 'INVALID_ADMISSION_POLICY');
  const queue = [];
  let active = 0, closed = false, timer, queuedTotal = 0;
  function room() {
    const free = freeBytes();
    return Number.isFinite(free) && free - active * requestReserveBytes >= headroomBytes + requestReserveBytes;
  }
  function pump() {
    while (!closed && queue.length && room()) {
      const next = queue.shift();
      next.signal.removeEventListener('abort', next.abort);
      active++;
      let released = false;
      next.resolve(() => {
        if (released) return;
        released = true; active--; pump();
      });
    }
    if (queue.length && !timer && !closed) timer = setInterval(pump, pollMs);
    else if (!queue.length && timer) { clearInterval(timer); timer = undefined; }
  }
  function acquire(signal) {
    need(signal instanceof AbortSignal, 'INVALID_SIGNAL');
    if (closed || signal.aborted) return Promise.reject(new NativeError('CANCELLED'));
    if (queue.length >= maxQueued) return Promise.reject(new NativeError('MEMORY_QUEUE_FULL'));
    if (!room() || queue.length) queuedTotal++;
    return new Promise((resolve, reject) => {
      const entry = { resolve, reject, signal, abort: () => {
        const index = queue.indexOf(entry);
        if (index !== -1) queue.splice(index, 1);
        signal.removeEventListener('abort', entry.abort);
        reject(new NativeError('CANCELLED')); pump();
      } };
      signal.addEventListener('abort', entry.abort, { once: true });
      queue.push(entry); pump();
    });
  }
  function close() {
    closed = true;
    clearInterval(timer); timer = undefined;
    for (const entry of queue.splice(0)) {
      entry.signal.removeEventListener('abort', entry.abort);
      entry.reject(new NativeError('CANCELLED'));
    }
  }
  return { acquire, close, diagnostics: () => ({ active, queued: queue.length, queuedTotal,
    reservedBytes: active * requestReserveBytes, headroomBytes, requestReserveBytes,
    timerActive: timer !== undefined, closed }) };
}
