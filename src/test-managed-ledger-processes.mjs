import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { openManagedLedgerSource, createManagedLedgerAccount } from '../verification/managed-ledger-account.mjs';
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
const worker = fileURLToPath(new URL('../verification/fixtures/legacy-ledger-worker.mjs', import.meta.url));
const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
function start(input, mode = 'create') {
  return new Promise((done, reject) => {
    const child = spawn(process.execPath, [worker, mode, input.sourceDirectory, input.ledgerHash, input.manifestHash],
      { env, windowsHide: true, timeout: 5000, stdio: ['ignore', 'pipe', 'pipe'] });
    let output = '', errors = '';
    child.stdout.on('data', chunk => { output += chunk; if (output.length > 4096) child.kill(); });
    child.stderr.on('data', chunk => { errors += chunk; if (errors.length > 4096) child.kill(); });
    child.once('error', reject);
    child.once('close', (code, signal) => done({ pid: child.pid, code, signal, output, errors }));
  });
}
const shared = createPublicLegacyLedger();
const raced = await Promise.all([start(shared), start(shared), start(shared)]);
for (const result of raced) { equal(result.code, 0); equal(result.signal, null); equal(result.errors, ''); }
const outcomes = raced.map(result => JSON.parse(result.output));
equal(outcomes.filter(result => result.created).length, 1);
equal(outcomes.filter(result => result.reason === 'MANAGED_LEDGER_ALREADY_BOUND').length, 2);
const one = openManagedLedgerSource(shared.sourceDirectory);
equal(one.root, outcomes.find(result => result.created).root);
equal(one.account.entries.length, 0);
const interrupted = createPublicLegacyLedger(), exited = await start(interrupted, 'exit-after-ready');
equal(exited.code, 73); equal(exited.signal, null); equal(exited.output, ''); equal(exited.errors, '');
for (const result of [...raced, exited]) {
  assert.throws(() => process.kill(result.pid, 0), { code: 'ESRCH' }); checks++;
}
const recovered = openManagedLedgerSource(interrupted.sourceDirectory);
equal(recovered.account.initial, one.account.initial);
equal(recovered.account.entries.length, 0);
assert.throws(() => createManagedLedgerAccount({ ...interrupted, localNative: true }), { message: 'MANAGED_LEDGER_ALREADY_BOUND' }); checks++;
console.log(JSON.stringify({ suite: 'managed-ledger-processes', checks, sourceDirectories: [shared.sourceDirectory, interrupted.sourceDirectory],
  actualPublicChildren: 4, stoppedChildren: 4, initializersPerSource: 1, nativeStarts: 0, actualModelRequests: 0,
  lostResultRecoveredWithoutReinitialization: true, powerLossDurability: 'NOT_RUN' }));
