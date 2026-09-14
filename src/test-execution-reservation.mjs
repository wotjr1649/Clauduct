import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync, copyFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createExecutionReservation, settleExecutionReservation, readExecutionReservation } from '../verification/execution-reservation.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';

const project = fileURLToPath(new URL('../', import.meta.url));
const root = temporaryDir(project, 'execution-reservation-');
const folder = name => { const path = join(root, name); mkdirSync(path); return path; };
const details = () => ({ basisHash: 'a'.repeat(64),
  previous: { attempts: 10, inputTokens: 100, outputTokens: 20, elapsedMs: 1000 },
  limits: { attempts: 20, inputTokens: 2000, outputTokens: 500, elapsedMs: 30000 },
  allowance: { attempts: 6, inputTokens: 1000, outputTokens: 200, elapsedMs: 20000 } });
const observed = () => ({ attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 });
let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const deny = action => { assert.throws(action); checks++; };
const first = folder('normal'), reserved = createExecutionReservation(first, details());
equal(reserved.charged, { attempts: 16, inputTokens: 1100, outputTokens: 220, elapsedMs: 21000 });
equal(reserved.retained, { attempts: true, inputTokens: true, outputTokens: true, elapsedMs: true });
equal(reserved.settlementState, 'missing'); equal(reserved.authorizesExecution, false);
equal(reserved.observationsIndependentlyVerified, false);
const original = readFileSync(join(first, 'execution-reservation.json'));
deny(() => createExecutionReservation(first, details()));
const completed = settleExecutionReservation(first, { observed: observed(), evidenceHash: 'b'.repeat(64) });
equal(completed.charged, { attempts: 15, inputTokens: 250, outputTokens: 50, elapsedMs: 3000 });
equal(completed.settlementState, 'recorded'); equal(completed.overrun, false); equal(completed.withinLimits, true);
equal(readFileSync(join(first, 'execution-reservation.json')), original);
deny(() => settleExecutionReservation(first, { observed: observed(), evidenceHash: 'b'.repeat(64) }));
equal(readExecutionReservation(first).charged, completed.charged);
const partial = folder('partial'); createExecutionReservation(partial, details());
equal(settleExecutionReservation(partial, { observed: { ...observed(), inputTokens: null, outputTokens: null }, evidenceHash: 'b'.repeat(64) }).charged,
  { attempts: 15, inputTokens: 1100, outputTokens: 220, elapsedMs: 3000 });
const overrun = folder('overrun'); createExecutionReservation(overrun, details());
const exceeded = settleExecutionReservation(overrun, { observed: { ...observed(), attempts: 11 }, evidenceHash: 'b'.repeat(64) });
equal(exceeded.charged.attempts, 21); equal(exceeded.overrun, true); equal(exceeded.withinLimits, false);
for (const text of ['', '{"version":1', '{"version":1}\n', '{"SYNTHETIC_PRIVATE":true}\n']) {
  const target = folder(`torn-${checks}`), before = createExecutionReservation(target, details());
  writeFileSync(join(target, 'execution-settlement.json'), text, { flag: 'wx' });
  const after = readExecutionReservation(target);
  equal(after.settlementState, 'invalid'); equal(after.charged, before.charged); equal(after.authorizesExecution, false);
}
const replay = folder('replay');
copyFileSync(join(first, 'execution-reservation.json'), join(replay, 'execution-reservation.json'));
copyFileSync(join(first, 'execution-settlement.json'), join(replay, 'execution-settlement.json'));
deny(() => readExecutionReservation(replay));
const foreignSettlement = folder('foreign-settlement'), pending = createExecutionReservation(foreignSettlement, details());
copyFileSync(join(first, 'execution-settlement.json'), join(foreignSettlement, 'execution-settlement.json'));
equal(readExecutionReservation(foreignSettlement).settlementState, 'invalid');
equal(readExecutionReservation(foreignSettlement).charged, pending.charged);
for (const change of [value => { value.basisHash = 'SYNTHETIC_PRIVATE'; }, value => { value.extra = 'SYNTHETIC_PRIVATE'; },
  value => { value.previous.attempts = null; }, value => { value.previous.inputTokens = -1; }, value => { value.limits.outputTokens = '500'; },
  value => { value.allowance.attempts = 0; }, value => { value.allowance.elapsedMs = 0; }, value => { value.allowance.attempts = 11; },
  value => { value.previous.inputTokens = Number.MAX_SAFE_INTEGER; }, value => { value.allowance.inputTokens = NaN; }]) {
  const target = folder(`invalid-${checks}`), value = details(); change(value);
  deny(() => createExecutionReservation(target, value)); equal(existsSync(join(target, 'execution-reservation.json')), false);
}
const invalidResult = folder('invalid-result'), unchanged = createExecutionReservation(invalidResult, details());
for (const value of [undefined, '0', -1, 0.5, NaN, Infinity]) {
  deny(() => settleExecutionReservation(invalidResult, { observed: { ...observed(), inputTokens: value }, evidenceHash: 'b'.repeat(64) }));
  equal(readExecutionReservation(invalidResult).charged, unchanged.charged);
}
console.log(JSON.stringify({ suite: 'execution-reservation', checks, evidenceRoot: root,
  externalRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN' }));
