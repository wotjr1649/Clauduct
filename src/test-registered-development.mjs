import assert from 'node:assert/strict';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash, randomUUID } from 'node:crypto';
import { registerDevelopmentTask, registeredDevelopmentTaskPath, readRegisteredDevelopmentTask } from '../verification/registered-development-tasks.mjs';
import { developmentTask, developmentOracleSource, developmentTaskWorkKey } from '../verification/development-tasks.mjs';
import { publicDevelopmentSource } from '../verification/fixtures/development-responses.mjs';
import { publicBudgetTask } from '../verification/fixtures/registered-budget-task.mjs';
import { checkDevelopmentSource } from '../verification/development-source-policy.mjs';
import { createManagedPlan } from '../verification/managed-development.mjs';
import { createManagedLedgerAccount } from '../verification/managed-ledger-account.mjs';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
const encode = value => JSON.stringify(value) + '\n', hash = value => createHash('sha256').update(value).digest('hex');
const fixture = () => publicBudgetTask(randomUUID());
const register = value => { const text = encode(value); return registerDevelopmentTask(text, hash(text)); };
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const rejected = (record, message = 'REGISTERED_DEVELOPMENT_TASK_INVALID') => {
  const text = encode(record), id = `registered-${hash(text)}`;
  assert.throws(() => registerDevelopmentTask(text, hash(text)), { message }); checks++;
  equal(existsSync(registeredDevelopmentTaskPath(id)), false);
};
const record = fixture(), taskId = register(record), task = developmentTask(taskId);
equal(task.registered, true); equal(task.functionName, record.functionName); equal(task.checks, 37);
equal(task.sourceFile, 'implementation.mjs'); equal(task.oracleSource, developmentOracleSource(taskId));
equal(task.localFixtureSource, publicDevelopmentSource(taskId)); equal(checkDevelopmentSource(task.localFixtureSource, taskId), true);
equal(Object.isFrozen(task.members), true); equal(Object.isFrozen(task.members.used), true);
equal(register(record), taskId); equal(readFileSync(registeredDevelopmentTaskPath(taskId), 'utf8'), encode(record));
const withoutAnswer = register(publicBudgetTask(randomUUID(), { localFixture: false }));
equal(readRegisteredDevelopmentTask(withoutAnswer).localFixtureSource, null);
assert.throws(() => publicDevelopmentSource(withoutAnswer), { message: 'DEVELOPMENT_LOCAL_SOURCE_REQUIRED' }); checks++;
const localAccount = createManagedLedgerAccount({ ...createPublicLegacyLedger(), localNative: true });
assert.throws(() => createManagedPlan(localAccount.root, { model: 'sol', localNative: true, taskIds: [withoutAnswer], maxDurationMs: 180000 }),
  { message: 'DEVELOPMENT_LOCAL_SOURCE_REQUIRED' }); checks++;
equal(existsSync(join(localAccount.root, 'development-plan.json')), false);
const relabeled = publicBudgetTask(randomUUID(), { localFixture: false });
relabeled.cases.reverse(); relabeled.cases[0].name = 'PUBLIC_RENAMED';
const relabeledId = register(relabeled);
equal(developmentTaskWorkKey(withoutAnswer), developmentTaskWorkKey(relabeledId));
const publicLive = createManagedLedgerAccount({ ...createPublicLegacyLedger(), localNative: false,
  scopeRoot: dirname(dirname(fileURLToPath(import.meta.url))) });
assert.throws(() => createManagedPlan(publicLive.root, { model: 'sol', localNative: false,
  taskIds: [withoutAnswer, relabeledId], maxDurationMs: 1320000 }), { message: 'MANAGED_PLAN_REPEATED_TASK' }); checks++;
equal(existsSync(join(publicLive.root, 'development-plan.json')), false);
equal(createManagedPlan(publicLive.root, { model: 'sol', localNative: false,
  taskIds: [withoutAnswer], maxDurationMs: 1320000 }).account.entries.length, 0);
for (const patch of [{ functionName: 'constructor' }, { functionName: 'then' }, { functionName: 'default' }, { functionName: 'arguments' },
  { fields: ['constructor'] }, { fields: ['__proto__'] }, { fields: ['value'] }, { fields: ['isSafeInteger'] },
  { numbers: ['0x10'] }, { numbers: ['9007199254740992'] }, { numbers: [] }, { strings: ['\\x70rocess'] },
  { globals: ['process'] }, { oracleFile: '../outside.mjs' }, { sourceFile: '../outside.mjs' },
  { cases: [] }, { localFixtureSource: 1 }]) rejected({ ...fixture(), ...patch });
for (const text of ['return process.env;', 'return value.constructor;', 'return value["used"];',
  'while (true) {}', 'return remainingExecutionBudget(value);', 'Number.isSafeInteger = value;']) {
  rejected({ ...fixture(), baseline: `export function remainingExecutionBudget(value) { ${text} }` }, 'DEVELOPMENT_SOURCE_REJECTED');
}
const duplicate = fixture(); duplicate.cases[1].name = duplicate.cases[0].name; rejected(duplicate);
const badKind = fixture(); badKind.cases[0].inputKind = 'eval'; rejected(badKind);
const extraCase = fixture(); extraCase.cases[0].command = 'PUBLIC_INERT'; rejected(extraCase);
const changed = fixture(), changedId = register(changed), changedPath = registeredDevelopmentTaskPath(changedId);
writeFileSync(changedPath, encode({ ...changed, requirements: 'PUBLIC_CHANGED' }));
assert.throws(() => readRegisteredDevelopmentTask(changedId), { message: 'REGISTERED_DEVELOPMENT_TASK_INVALID' }); checks++;
assert.throws(() => register(changed), { message: 'REGISTERED_DEVELOPMENT_TASK_INVALID' }); checks++;
equal(readFileSync(changedPath, 'utf8'), encode({ ...changed, requirements: 'PUBLIC_CHANGED' }));
for (const text of ['{', '[]\n', ' '.repeat(65537), encode(record).replace('"version":1', '"version":1,"version":1')]) {
  assert.throws(() => registerDevelopmentTask(text, hash(text)), { message: 'REGISTERED_DEVELOPMENT_TASK_INVALID' }); checks++;
}
console.log(JSON.stringify({ suite: 'registered-development', checks, taskId, oracleCases: task.checks,
  fixedCapabilitiesPreserved: true, nativeStarts: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
