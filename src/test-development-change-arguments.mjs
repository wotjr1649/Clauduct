import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { prepareDevelopmentChange, readDevelopmentChange } from '../verification/development-change.mjs';
import { readManagedDevelopmentResult } from '../verification/managed-development.mjs';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { createManagedLedgerAccount } from '../verification/managed-ledger-account.mjs';
import { readExecutionAccount, reserveExecutionAccount } from '../verification/execution-account.mjs';

let checks = 0;
const denied = (action, message) => { assert.throws(action, { message }); checks++; };
for (const value of [null, {}, [], { root: 'PUBLIC' },
  { accountRoot: 'PUBLIC', entryIndex: 1, targetRoot: 'PUBLIC', targetPath: 'source.mjs', expectedBeforeHash: 'bad', expectedAfterHash: '0'.repeat(64) },
  { accountRoot: 'PUBLIC', entryIndex: 1, targetRoot: 'PUBLIC', targetPath: 'source.mjs', expectedBeforeHash: '0'.repeat(64), expectedAfterHash: 'bad' }]) {
  denied(() => prepareDevelopmentChange(value), 'DEVELOPMENT_CHANGE_INVALID');
}
const account = createManagedLedgerAccount({ ...createPublicLegacyLedger(), localNative: true });
for (const index of [-1, 0, 1, 1.5, '1', null]) denied(() => readManagedDevelopmentResult(account.root, index), 'MANAGED_DEVELOPMENT_ENTRY_INVALID');
const pending = reserveExecutionAccount(account.root, { executionHash: 'a'.repeat(64),
  allowance: { attempts: 1, inputTokens: 1, outputTokens: 1, elapsedMs: 1000 } });
const before = readExecutionAccount(account.root);
denied(() => readManagedDevelopmentResult(account.root, 1), 'MANAGED_DEVELOPMENT_ENTRY_PENDING');
assert.deepEqual(readExecutionAccount(account.root), before); checks++;
assert.equal(existsSync(join(pending.entry, 'closure.json')), false); checks++;
denied(() => readDevelopmentChange(account.root), 'DEVELOPMENT_CHANGE_INVALID');
console.log(JSON.stringify({ suite: 'development-change-arguments', checks, accountRoot: account.root,
  pendingReservationUnchanged: true, nativeStarts: 0, testProcessesStarted: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
