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
} finally { gate.close(); }

// A deadline must wake a queue even when its ordinary memory poll is slower.
const blocked = createAdmission({ freeBytes: () => 0, maxWaitMs: 40, pollMs: 1000 });
let deadlineTimer, deadlineResult, elapsed;
try {
  const started = performance.now();
  const pending = blocked.acquire(signal().signal).then(release => { release(); return 'ADMITTED'; }, error => error.code);
  deadlineResult = await Promise.race([pending, new Promise(done => { deadlineTimer = setTimeout(() => done('UNSETTLED'), 300); })]);
  elapsed = performance.now() - started;
} finally { clearTimeout(deadlineTimer); blocked.close(); }
assert.equal(deadlineResult, 'MEMORY_ADMISSION_TIMEOUT');
assert.ok(elapsed >= 40 && elapsed < 300);
assert.equal(blocked.diagnostics().timedOutTotal, 1);
assert.equal(blocked.diagnostics().timerActive, false);
assert.equal(blocked.diagnostics().queued, 0);

for (const maxWaitMs of [0, -1, 1.5, NaN, Infinity, '30', 300001]) {
  assert.throws(() => createAdmission({ maxWaitMs }), { code: 'INVALID_ADMISSION_POLICY' });
}

// Expiring a waiter leaves admitted work intact; recovered capacity admits a
// subsequent request without resurrecting the expired one.
let recoveredBytes = 60;
const recovering = createAdmission({ freeBytes: () => recoveredBytes, headroomBytes: 20,
  requestReserveBytes: 40, maxWaitMs: 30, pollMs: 5 });
try {
  const held = await recovering.acquire(signal().signal);
  await assert.rejects(recovering.acquire(signal().signal), { code: 'MEMORY_ADMISSION_TIMEOUT' });
  assert.equal(recovering.diagnostics().active, 1);
  recoveredBytes = 100;
  const next = await recovering.acquire(signal().signal);
  assert.equal(recovering.diagnostics().active, 2);
  next(); held(); assert.equal(recovering.diagnostics().active, 0);
  assert.equal(recovering.diagnostics().timedOutTotal, 1);
} finally { recovering.close(); }

// Delaying the event loop must not admit an expired entry when memory returns
// before the pending deadline callback can run.
let available = 0;
const overdue = createAdmission({ freeBytes: () => available, headroomBytes: 0,
  requestReserveBytes: 1, maxWaitMs: 20, pollMs: 1000 });
try {
  const old = overdue.acquire(signal().signal).then(release => { release(); return 'ADMITTED'; }, error => error.code);
  const until = performance.now() + 30;
  while (performance.now() < until) { /* finite scheduling pressure */ }
  available = 10;
  const release = await overdue.acquire(signal().signal);
  assert.equal(await old, 'MEMORY_ADMISSION_TIMEOUT');
  release(); assert.equal(overdue.diagnostics().timerActive, false);
} finally { overdue.close(); }
console.log(JSON.stringify({ suite: 'request-admission', passed: true, evictionCount: 0, queuedAfterClose: 0,
  actualDeadlineExpirations: 3, pressureRecovery: true }));
