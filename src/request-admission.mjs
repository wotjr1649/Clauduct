import { freemem, totalmem } from 'node:os';
import { NativeError, need } from './native-protocol.mjs';

const MiB = 1024 * 1024;
// Reserve for body decoding, request conversion and a bounded response in addition to OS headroom.
export function createAdmission({ freeBytes = () => Math.min(freemem(), process.availableMemory?.() ?? Infinity),
  headroomBytes = Math.max(256 * MiB, Math.min(totalmem() / 10, 1024 * MiB)),
  requestReserveBytes = 128 * MiB, maxQueued = 128, pollMs = 250, maxWaitMs = 30000 } = {}) {
  need(typeof freeBytes === 'function' && Number.isFinite(headroomBytes) && headroomBytes >= 0
    && Number.isFinite(requestReserveBytes) && requestReserveBytes > 0
    && Number.isInteger(maxQueued) && maxQueued > 0 && Number.isInteger(pollMs) && pollMs > 0
    && Number.isSafeInteger(maxWaitMs) && maxWaitMs > 0 && maxWaitMs <= 300000, 'INVALID_ADMISSION_POLICY');
  const queue = [];
  let active = 0, closed = false, timer, queuedTotal = 0, timedOutTotal = 0;
  function room() {
    const free = freeBytes();
    return Number.isFinite(free) && free - active * requestReserveBytes >= headroomBytes + requestReserveBytes;
  }
  function pump() {
    clearTimeout(timer); timer = undefined;
    while (!closed && queue.length) {
      const available = room(), expired = performance.now() >= queue[0].deadline;
      if (!expired && !available) break;
      const next = queue.shift();
      next.signal.removeEventListener('abort', next.abort);
      if (expired) { timedOutTotal++; next.reject(new NativeError('MEMORY_ADMISSION_TIMEOUT')); continue; }
      active++;
      let released = false;
      next.resolve(() => {
        if (released) return;
        released = true; active--; pump();
      });
    }
    // One timer owns both the memory poll and the earliest monotonic deadline.
    if (queue.length && !closed) timer = setTimeout(pump,
      Math.max(1, Math.min(pollMs, queue[0].deadline - performance.now())));
  }
  function acquire(signal) {
    need(signal instanceof AbortSignal, 'INVALID_SIGNAL');
    if (closed || signal.aborted) return Promise.reject(new NativeError('CANCELLED'));
    pump();
    if (queue.length >= maxQueued) return Promise.reject(new NativeError('MEMORY_QUEUE_FULL'));
    if (!room() || queue.length) queuedTotal++;
    return new Promise((resolve, reject) => {
      const entered = performance.now();
      const entry = { resolve, reject, signal, entered, deadline: entered + maxWaitMs, abort: () => {
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
    clearTimeout(timer); timer = undefined;
    for (const entry of queue.splice(0)) {
      entry.signal.removeEventListener('abort', entry.abort);
      entry.reject(new NativeError('CANCELLED'));
    }
  }
  return { acquire, close, diagnostics: () => ({ active, queued: queue.length, queuedTotal, timedOutTotal, maxWaitMs,
    oldestWaitMs: queue.length ? Math.max(0, performance.now() - queue[0].entered) : 0,
    reservedBytes: active * requestReserveBytes, headroomBytes, requestReserveBytes,
    timerActive: timer !== undefined, closed }) };
}
