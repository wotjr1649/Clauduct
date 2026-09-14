import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { projectLegacyExecutionLedger, readLegacyExecutionLedger, createManagedLedgerAccount, readManagedLedgerAccount, openManagedLedgerSource } from '../verification/managed-ledger-account.mjs';
import { createExecutionAccount, reserveExecutionAccount, closeExecutionAccount } from '../verification/execution-account.mjs';
import { runManagedDevelopment } from '../verification/managed-development.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';
const project = dirname(dirname(fileURLToPath(import.meta.url))), hash = bytes => createHash('sha256').update(bytes).digest('hex');
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const denied = (action, message) => { assert.throws(action, { message }); checks++; };
const input = createPublicLegacyLedger(), before = readLegacyExecutionLedger(input.sourceDirectory, input);
const { priorLedger, ...components } = input.ledger;
equal(before.projection.initial, { attempts: 37, inputTokens: 400216, outputTokens: 99504, elapsedMs: 64000 });
equal(before.projection.components, components);
equal(before.projection.withinLimits, true);
equal(Object.isFrozen(before.projection.initial), true);
const bound = createManagedLedgerAccount({ ...input, localNative: true });
equal(openManagedLedgerSource(input.sourceDirectory).root, bound.root);
equal(readManagedLedgerAccount(bound.root).account.initial, before.projection.initial);
denied(() => createManagedLedgerAccount({ ...input, localNative: true }), 'MANAGED_LEDGER_ALREADY_BOUND');
denied(() => readManagedLedgerAccount(bound.root, { localNative: false }), 'MANAGED_LEDGER_MODE_MISMATCH');
await assert.rejects(runManagedDevelopment({ root: bound.root, model: 'sol', localNative: false,
  powershell: 'C:\\PublicFixture\\pwsh.exe' }), { message: 'MANAGED_LEDGER_MODE_MISMATCH' }); checks++;
equal(readManagedLedgerAccount(bound.root).account.entries.length, 0);
const claim = reserveExecutionAccount(bound.root, { executionHash: hash('PUBLIC_ACCOUNTING_UNIT'),
  allowance: { attempts: 3, inputTokens: 3000, outputTokens: 500, elapsedMs: 1000 } });
closeExecutionAccount(claim.entry, { ownerNonce: claim.ownerNonce,
  observed: { attempts: 1, inputTokens: 10, outputTokens: 2, elapsedMs: 20 }, evidenceHash: hash('PUBLIC_OBSERVATION') });
const finished = readManagedLedgerAccount(bound.root);
equal(finished.account.charged, { attempts: 38, inputTokens: 400226, outputTokens: 99506, elapsedMs: 64020 });
equal(finished.components, components);
denied(() => createManagedLedgerAccount(input), 'MANAGED_LEDGER_SCOPE_REQUIRED');
denied(() => createManagedLedgerAccount({ ...input, localNative: true, scopeRoot: input.sourceDirectory }), 'MANAGED_LEDGER_SCOPE_INVALID');
const foreign = temporaryDir(project, 'managed-development-');
createExecutionAccount(foreign, { basisHash: before.basisHash, previous: before.projection.initial, limits: before.projection.limits });
await assert.rejects(runManagedDevelopment({ root: foreign, model: 'sol', localNative: false,
  powershell: 'C:\\PublicFixture\\pwsh.exe' }), { message: 'MANAGED_LEDGER_BINDING_REQUIRED' }); checks++;
writeFileSync(join(foreign, 'legacy-ledger.json'), readFileSync(join(bound.root, 'legacy-ledger.json')), { flag: 'wx' });
denied(() => readManagedLedgerAccount(foreign), 'MANAGED_LEDGER_BINDING_INVALID');
for (const patch of [{ attemptUpper: 34 }, { knownInputTokens: -1 }, { knownOutputTokens: 0.5 },
  { inFlightInputReservation: undefined }, { inFlightUsageUnobservedRuns: 0 }, { inFlightInputReservation: 0 },
  { unobservedNativeDurationRuns: 0 }, { nativePhaseElapsedMsIsPartial: false }, { firstFailureUsageUnobserved: 'true' },
  { earlierFullOutputReservation: 0 }, { knownInputTokens: Number.MAX_SAFE_INTEGER }]) {
  denied(() => projectLegacyExecutionLedger({ ...input.ledger, ...patch }, input.manifest), 'MANAGED_LEDGER_COUNTER_INVALID');
}
const over = createPublicLegacyLedger({ manifestPatch: { cumulativeAttemptUpper: 36 } });
equal(readLegacyExecutionLedger(over.sourceDirectory, over).projection.withinLimits, false);
denied(() => createManagedLedgerAccount({ ...over, localNative: true }), 'MANAGED_LEDGER_LIMIT');
equal(existsSync(join(over.sourceDirectory, '.managed-execution-account')), false);
for (const alias of ['attemptUpper', 'attempt\\u0055pper']) {
  const fixture = createPublicLegacyLedger(), path = join(fixture.sourceDirectory, 'result.json');
  const text = `{"${alias}":1,` + readFileSync(path, 'utf8').slice(1);
  writeFileSync(path, text);
  denied(() => readLegacyExecutionLedger(fixture.sourceDirectory, { ...fixture, ledgerHash: hash(text) }), 'MANAGED_LEDGER_RECORD_INVALID');
}
for (const text of ['{"attemptUpper":', ' '.repeat(65537), '[]']) {
  const fixture = createPublicLegacyLedger(); writeFileSync(join(fixture.sourceDirectory, 'result.json'), text);
  denied(() => readLegacyExecutionLedger(fixture.sourceDirectory, { ...fixture, ledgerHash: hash(text) }), 'MANAGED_LEDGER_RECORD_INVALID');
}
const changed = createPublicLegacyLedger(), old = createManagedLedgerAccount({ ...changed, localNative: true });
writeFileSync(join(changed.sourceDirectory, 'result.json'), JSON.stringify({ ...changed.ledger, attemptUpper: 38 }));
denied(() => readManagedLedgerAccount(old.root), 'MANAGED_LEDGER_SOURCE_CHANGED');
const incomplete = createPublicLegacyLedger(); mkdirSync(join(incomplete.sourceDirectory, '.managed-execution-account'));
denied(() => createManagedLedgerAccount({ ...incomplete, localNative: true }), 'MANAGED_LEDGER_ALREADY_BOUND');
denied(() => openManagedLedgerSource(incomplete.sourceDirectory), 'MANAGED_LEDGER_RECORD_INVALID');
denied(() => projectLegacyExecutionLedger({ ...input.ledger, unknownReservation: 0 }, input.manifest), 'MANAGED_LEDGER_FIELDS_UNSUPPORTED');
denied(() => projectLegacyExecutionLedger(input.ledger, { ...input.manifest, unknownLimit: 0 }), 'MANAGED_LEDGER_FIELDS_UNSUPPORTED');
const inert = createPublicLegacyLedger({ ledgerPatch: { results: [{ command: 'PUBLIC_INERT_DATA', attempts: 0 }] } });
equal(readLegacyExecutionLedger(inert.sourceDirectory, inert).projection, before.projection);
const liveMetadata = createPublicLegacyLedger();
const liveBound = createManagedLedgerAccount({ ...liveMetadata, localNative: false, scopeRoot: project });
equal(readManagedLedgerAccount(liveBound.root, { localNative: false }).account.entries.length, 0);
equal(openManagedLedgerSource(liveMetadata.sourceDirectory).localNative, false);
console.log(JSON.stringify({ suite: 'managed-ledger-account', checks, sourceDirectory: input.sourceDirectory, root: bound.root,
  earlierReservationsPreserved: true, nativeStarts: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
