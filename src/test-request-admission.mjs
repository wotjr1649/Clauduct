import assert from 'node:assert/strict';
import { createAdmission } from './request-admission.mjs';

const signal = () => new AbortController();
const tick = () => new Promise(resolve => setTimeout(resolve, 15));
let free = 100;
const gate = createAdmission({ freeBytes: () => free, headroomBytes: 20, requestReserveBytes: 40, maxQueued: 2, pollMs: 5 });
try {
  const first = await gate.acquire(signal().signal);
  const second = await gate.acquire(signal().signal);
  let admitted = false;
  const third = gate.acquire(signal().signal).then(release => { admitted = true; return release; });
  await tick(); assert.equal(admitted, false); assert.equal(gate.diagnostics().active, 2);
  free = 0; first(); await tick();
  assert.equal(gate.diagnostics().active, 1); assert.equal(admitted, false); // Existing work is not evicted.
  free = 100; const releaseThird = await third;
  assert.equal(gate.diagnostics().active, 2);
  second(); releaseThird(); releaseThird(); assert.equal(gate.diagnostics().active, 0);
  free = 0;
  const cancelled = signal();
  const waiting = gate.acquire(cancelled.signal);
  const rejection = assert.rejects(waiting, { code: 'CANCELLED' }); cancelled.abort(); await rejection;
  assert.equal(gate.diagnostics().queued, 0);
  const one = gate.acquire(signal().signal), two = gate.acquire(signal().signal);
  await assert.rejects(gate.acquire(signal().signal), { code: 'MEMORY_QUEUE_FULL' });
  const closed = Promise.all([assert.rejects(one, { code: 'CANCELLED' }), assert.rejects(two, { code: 'CANCELLED' })]);
  gate.close(); await closed;
  assert.equal(gate.diagnostics().queued, 0); assert.equal(gate.diagnostics().timerActive, false);
  await assert.rejects(gate.acquire(signal().signal), { code: 'CANCELLED' });
  console.log(JSON.stringify({ suite: 'request-admission', passed: true, evictionCount: 0, queuedAfterClose: 0 }));
} finally { gate.close(); }
