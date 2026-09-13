import assert from 'node:assert/strict';
import { copyFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { createManagedLedgerAccount, openManagedLedgerSource } from '../verification/managed-ledger-account.mjs';
let checks = 0;
const source = createPublicLegacyLedger(), original = createManagedLedgerAccount({ ...source, localNative: true });
const clone = createPublicLegacyLedger();
for (const name of ['result.json', 'manifest.json']) copyFileSync(join(source.sourceDirectory, name), join(clone.sourceDirectory, name));
assert.throws(() => createManagedLedgerAccount({ sourceDirectory: clone.sourceDirectory, ledgerHash: source.ledgerHash,
  manifestHash: source.manifestHash, localNative: true }), { message: 'MANAGED_LEDGER_ALREADY_BOUND' }); checks++;
assert.equal(existsSync(join(clone.sourceDirectory, '.managed-execution-account')), false); checks++;
assert.equal(openManagedLedgerSource(source.sourceDirectory).root, original.root); checks++;
console.log(JSON.stringify({ suite: 'managed-ledger-snapshot', checks, sourceDirectory: source.sourceDirectory,
  duplicateSourceDirectory: clone.sourceDirectory, identicalSnapshotRejected: true, nativeStarts: 0, actualModelRequests: 0 }));
