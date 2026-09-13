import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createExecutionAccount, readExecutionAccount, reserveExecutionAccount, closeExecutionAccount } from '../verification/execution-account.mjs';
import { settleExecutionReservation } from '../verification/execution-reservation.mjs';

const project = fileURLToPath(new URL('../', import.meta.url)), root = mkdtempSync(join(project, '.tmp', 'execution-account-'));
const previous = { attempts: 10, inputTokens: 100, outputTokens: 20, elapsedMs: 1000 };
const limits = { attempts: 20, inputTokens: 5000, outputTokens: 2000, elapsedMs: 50000 };
const allowance = { attempts: 6, inputTokens: 1000, outputTokens: 200, elapsedMs: 20000 };
const observed = { attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 };
const options = { executionHash: 'b'.repeat(64), allowance };
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const deny = (action, code) => { assert.throws(action, code ? { message: code } : undefined); checks++; };
function fixture(name) {
  const path = join(root, name); mkdirSync(path);
  createExecutionAccount(path, { basisHash: 'a'.repeat(64), previous, limits }); return path;
}
const normal = fixture('normal'), empty = readExecutionAccount(normal);
equal(empty.charged, previous); equal(empty.pending, null);
deny(() => createExecutionAccount(normal, { basisHash: 'a'.repeat(64), previous, limits }), 'EXECUTION_ACCOUNT_EXISTS');
const first = reserveExecutionAccount(normal, options);
equal(readExecutionAccount(normal).charged, { attempts: 16, inputTokens: 1100, outputTokens: 220, elapsedMs: 21000 });
equal(readExecutionAccount(normal).pending, first.entry);
deny(() => reserveExecutionAccount(normal, options), 'EXECUTION_ACCOUNT_PENDING');
equal(readdirSync(join(normal, 'executions')).length, 1);
deny(() => closeExecutionAccount(first.entry, { ownerNonce: 'PUBLIC_WRONG_NONCE', observed, evidenceHash: 'c'.repeat(64) }), 'EXECUTION_ACCOUNT_OWNER_ACTIVE');
const partial = { ...observed, attempts: 2, inputTokens: null };
const once = closeExecutionAccount(first.entry, { ownerNonce: first.ownerNonce, observed: partial, evidenceHash: 'c'.repeat(64) });
equal(once.pending, null); equal(once.charged, { attempts: 12, inputTokens: 1100, outputTokens: 50, elapsedMs: 3000 });
equal(once.initial, previous);
const originalClosure = readFileSync(join(first.entry, 'closure.json'));
deny(() => closeExecutionAccount(first.entry, { ownerNonce: first.ownerNonce, observed: partial, evidenceHash: 'c'.repeat(64) }), 'EXECUTION_ACCOUNT_NOT_PENDING');
equal(readFileSync(join(first.entry, 'closure.json')), originalClosure);
const second = reserveExecutionAccount(normal, options);
equal(second.previous, once.charged);
const twice = closeExecutionAccount(second.entry, { ownerNonce: second.ownerNonce, observed, evidenceHash: 'd'.repeat(64) });
equal(twice.charged, { attempts: 17, inputTokens: 1250, outputTokens: 80, elapsedMs: 5000 });
equal(twice.initial, previous);
deny(() => reserveExecutionAccount(normal, options), 'EXECUTION_ACCOUNT_LIMIT');
equal(readdirSync(join(normal, 'executions')).length, 2);
for (const [name, value] of [['settled-before-closure', observed], ['overrun-before-closure', { ...observed, attempts: 11 }]]) {
  const path = fixture(name), claim = reserveExecutionAccount(path, options);
  settleExecutionReservation(claim.entry, { observed: value, evidenceHash: 'c'.repeat(64) });
  const pending = readExecutionAccount(path);
  equal(pending.charged.attempts, name.startsWith('overrun') ? 21 : 16);
  equal(pending.charged.inputTokens, 1100);
  equal(pending.pending, claim.entry);
  const closed = closeExecutionAccount(claim.entry, { ownerNonce: claim.ownerNonce, observed: value, evidenceHash: 'c'.repeat(64) });
  equal(closed.charged.attempts, previous.attempts + value.attempts);
  equal(closed.withinLimits, !name.startsWith('overrun'));
}
const torn = fixture('torn'), pending = reserveExecutionAccount(torn, options);
writeFileSync(join(pending.entry, 'execution-settlement.json'), '{"version":', { flag: 'wx' });
equal(readExecutionAccount(torn).charged, { attempts: 16, inputTokens: 1100, outputTokens: 220, elapsedMs: 21000 });
deny(() => closeExecutionAccount(pending.entry, { ownerNonce: pending.ownerNonce, observed, evidenceHash: 'c'.repeat(64) }), 'EXECUTION_ACCOUNT_SETTLEMENT_CONFLICT');
equal(existsSync(join(pending.entry, 'closure.json')), false);
const changed = fixture('changed-closure'), claim = reserveExecutionAccount(changed, options);
closeExecutionAccount(claim.entry, { ownerNonce: claim.ownerNonce, observed, evidenceHash: 'c'.repeat(64) });
const closure = JSON.parse(readFileSync(join(claim.entry, 'closure.json'), 'utf8')); closure.stateHash = 'f'.repeat(64);
writeFileSync(join(claim.entry, 'closure.json'), JSON.stringify(closure) + '\n');
deny(() => readExecutionAccount(changed), 'EXECUTION_ACCOUNT_CLOSURE_INVALID');
const changedEvidence = fixture('changed-evidence'), evidenceClaim = reserveExecutionAccount(changedEvidence, options);
closeExecutionAccount(evidenceClaim.entry, { ownerNonce: evidenceClaim.ownerNonce, observed, evidenceHash: 'c'.repeat(64) });
const evidenceClosure = JSON.parse(readFileSync(join(evidenceClaim.entry, 'closure.json'), 'utf8'));
evidenceClosure.evidenceHash = 'f'.repeat(64);
writeFileSync(join(evidenceClaim.entry, 'closure.json'), JSON.stringify(evidenceClosure) + '\n');
deny(() => readExecutionAccount(changedEvidence), 'EXECUTION_ACCOUNT_CLOSURE_INVALID');
for (const slot of ['000001', '000002']) {
  const path = fixture(`incomplete-${slot}`); mkdirSync(join(path, 'executions', slot));
  deny(() => readExecutionAccount(path));
  deny(() => reserveExecutionAccount(path, options));
  equal(readdirSync(join(path, 'executions')), [slot]);
}
console.log(JSON.stringify({ suite: 'execution-account', checks, evidenceRoot: root,
  actualModelRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN' }));
