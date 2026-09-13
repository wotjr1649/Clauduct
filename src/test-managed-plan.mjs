import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { createManagedLedgerAccount } from '../verification/managed-ledger-account.mjs';
import { createManagedPlan, readManagedPlan, runManagedPlan, runManagedDevelopment } from '../verification/managed-development.mjs';
import { readExecutionAccount, reserveExecutionAccount } from '../verification/execution-account.mjs';
import { sourceHash } from '../verification/development-fixture.mjs';
const project = dirname(dirname(fileURLToPath(import.meta.url)));
const powershell = 'C:\\PublicFixture\\pwsh.exe';
const args = { model: 'sol', localNative: true, taskIds: ['retry-after-seconds', 'retry-delay-window'], maxDurationMs: 180000 };
const fresh = () => createManagedLedgerAccount({ ...createPublicLegacyLedger(), localNative: true });
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const denied = (action, message) => { assert.throws(action, { message }); checks++; };
const account = fresh(), plan = createManagedPlan(account.root, args);
equal(plan.completedSteps, 0); equal(plan.done, false); equal(plan.account.charged, account.account.initial);
equal(plan.plan.deadlineMs - plan.plan.createdAtMs, 180000);
equal(Object.keys(plan.plan.sourceHashes).length, 37);
equal(plan.plan.sourceHashes[join('verification', 'fixtures', 'development-integration-oracle.mjs')],
  sourceHash(readFileSync(join(project, 'verification/fixtures/development-integration-oracle.mjs'))));
equal(readManagedPlan(account.root).planHash, plan.planHash);
assert.throws(() => createManagedPlan(account.root, args), { code: 'EEXIST' }); checks++;
denied(() => readManagedPlan(account.root, { localNative: false }), 'MANAGED_PLAN_MODE_MISMATCH');
await assert.rejects(runManagedDevelopment({ root: account.root, model: 'sol', localNative: true, powershell }),
  { message: 'MANAGED_PLAN_STEP_INVALID' }); checks++;
for (const plannedStep of [{ planHash: plan.planHash, index: 1 }, { planHash: 'a'.repeat(64), index: 0 },
  { planHash: plan.planHash, index: 0, extra: true }]) {
  await assert.rejects(runManagedDevelopment({ root: account.root, model: 'sol', localNative: true, powershell, plannedStep }),
    { message: 'MANAGED_PLAN_STEP_INVALID' }); checks++;
}
equal(readExecutionAccount(account.root).entries.length, 0);
for (const patch of [{ model: 'astra' }, { localNative: 'true' }, { taskIds: [] }, { taskIds: Array(257).fill('retry-after-seconds') },
  { taskIds: [undefined] }, { taskIds: [null] }, { taskIds: Array(1) }, { taskIds: ['retry-after-seconds', undefined] },
  { maxDurationMs: 59999 }, { maxDurationMs: 259200001 }, { maxDurationMs: Infinity }]) {
  const current = fresh(); denied(() => createManagedPlan(current.root, { ...args, ...patch }), 'MANAGED_PLAN_INVALID');
  equal(existsSync(join(current.root, 'development-plan.json')), false);
}
const live = createManagedLedgerAccount({ ...createPublicLegacyLedger(), scopeRoot: project, localNative: false });
denied(() => createManagedPlan(live.root, { ...args, localNative: false, maxDurationMs: 1320000,
  taskIds: ['retry-after-seconds', 'retry-after-seconds'] }), 'MANAGED_PLAN_REPEATED_TASK');
equal(createManagedPlan(live.root, { ...args, localNative: false, maxDurationMs: 1320000 }).account.entries.length, 0);
for (const mutate of [value => { value.deadlineMs++; }, value => { value.taskIds.reverse(); },
  value => { value.accountHash = 'a'.repeat(64); }, value => { value.sourceHashes = {}; }]) {
  const current = fresh(); createManagedPlan(current.root, args);
  const path = join(current.root, 'development-plan.json'), value = JSON.parse(readFileSync(path)); mutate(value);
  writeFileSync(path, JSON.stringify(value) + '\n');
  denied(() => readManagedPlan(current.root), 'MANAGED_PLAN_CHANGED');
  equal(readExecutionAccount(current.root).entries.length, 0);
}
const expired = fresh(); createManagedPlan(expired.root, args);
const path = join(expired.root, 'development-plan.json'), value = JSON.parse(readFileSync(path));
value.createdAtMs = Date.now() - 240000; value.deadlineMs = value.createdAtMs + 180000;
const text = JSON.stringify(value) + '\n'; writeFileSync(path, text);
writeFileSync(join(expired.root, 'development-plan-ready.json'), JSON.stringify({ version: 1,
  planHash: sourceHash(text), accountHash: expired.account.accountHash }) + '\n');
await assert.rejects(runManagedPlan({ root: expired.root, powershell, localNative: true }), { message: 'MANAGED_PLAN_DEADLINE' }); checks++;
equal(readExecutionAccount(expired.root).entries.length, 0);
const dirty = fresh(); reserveExecutionAccount(dirty.root, { executionHash: 'b'.repeat(64),
  allowance: { attempts: 1, inputTokens: 1, outputTokens: 1, elapsedMs: 100 } });
denied(() => createManagedPlan(dirty.root, args), 'MANAGED_PLAN_ACCOUNT_NOT_EMPTY');
const limited = createManagedLedgerAccount({ ...createPublicLegacyLedger({ manifestPatch: { cumulativeAttemptUpper: 41 } }), localNative: true });
createManagedPlan(limited.root, args);
await assert.rejects(runManagedPlan({ root: limited.root, powershell, localNative: true }), { message: 'EXECUTION_ACCOUNT_LIMIT' }); checks++;
equal(readExecutionAccount(limited.root).entries.length, 0);
const incomplete = fresh();
writeFileSync(join(incomplete.root, 'development-plan.json'), '{}\n', { flag: 'wx' });
assert.throws(() => readManagedPlan(incomplete.root), { code: 'ENOENT' }); checks++;
assert.throws(() => createManagedPlan(incomplete.root, args), { code: 'EEXIST' }); checks++;
equal(existsSync(join(incomplete.root, 'development-plan-ready.json')), false);
console.log(JSON.stringify({ suite: 'managed-plan', checks, nativeStarts: 0, actualModelRequests: 0,
  actualCredentialReads: 0, sourceDirectory: account.sourceDirectory, accountRoot: account.root,
  deadlineChangeRejected: true, syntheticExpiredPlan: true, longStageEvidence: false }));
