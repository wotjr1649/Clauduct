import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createPublicLegacyLedger } from '../verification/fixtures/legacy-ledger.mjs';
import { createManagedLedgerAccount } from '../verification/managed-ledger-account.mjs';
import { readManagedPlan } from '../verification/managed-development.mjs';
const worker = fileURLToPath(new URL('../verification/fixtures/plan-worker.mjs', import.meta.url));
const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const fresh = () => createManagedLedgerAccount({ ...createPublicLegacyLedger(), localNative: true });
let checks = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
function start(root, mode = 'create') {
  return new Promise((done, reject) => {
    const child = spawn(process.execPath, [worker, mode, root], { env, windowsHide: true, timeout: 5000, stdio: ['ignore', 'pipe', 'pipe'] });
    let output = '', errors = '';
    child.stdout.on('data', chunk => { output += chunk; if (output.length > 4096) child.kill(); });
    child.stderr.on('data', chunk => { errors += chunk; if (errors.length > 4096) child.kill(); });
    child.once('error', reject); child.once('close', (code, signal) => done({ pid: child.pid, code, signal, output, errors }));
  });
}
const account = fresh(), race = await Promise.all([start(account.root), start(account.root), start(account.root)]);
for (const result of race) { equal(result.code, 0); equal(result.errors, ''); equal(result.signal, null); }
const rows = race.map(result => JSON.parse(result.output));
equal(rows.filter(row => row.created).length, 1); equal(rows.filter(row => row.reason === 'PLAN_EXISTS').length, 2);
equal(readManagedPlan(account.root).planHash, rows.find(row => row.created).planHash);
const interrupted = fresh(), lost = await start(interrupted.root, 'exit-after-ready');
equal(lost.code, 73); equal(lost.output, ''); equal(lost.errors, ''); equal(lost.signal, null);
const reopened = readManagedPlan(interrupted.root);
equal(reopened.completedSteps, 0); equal(reopened.account.entries.length, 0);
equal(reopened.plan.deadlineMs - reopened.plan.createdAtMs, 180000);
for (const result of [...race, lost]) { assert.throws(() => process.kill(result.pid, 0), { code: 'ESRCH' }); checks++; }
console.log(JSON.stringify({ suite: 'managed-plan-processes', checks, actualPublicChildren: 4, stoppedChildren: 4,
  accountRoots: [account.root, interrupted.root], planCreators: 1, lostResultReopened: true,
  nativeStarts: 0, actualModelRequests: 0, powerLossDurability: 'NOT_RUN' }));
