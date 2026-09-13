import assert from 'node:assert/strict';
import { existsSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createDevelopmentFixture } from '../verification/development-fixture.mjs';
import { verifyNativeDevelopment, prepareDevelopmentInterruption, readDevelopmentInterruption } from '../verification/verify-native-development.mjs';

let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const base = { model: 'sol', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 };
if (process.argv[2] === '--reserve-only') {
  await verifyNativeDevelopment({ ...base, executionAccountHash: 'a'.repeat(64),
    onReservation: binding => { console.log(JSON.stringify(binding)); process.exit(73); } });
  throw new Error('RESERVATION_BOUNDARY_MISSED');
}
for (const extra of [{ holdAfterSourceWrite: 'true' }, { holdAfterSourceWrite: true, localNative: false },
  { holdAfterSourceWrite: true, cutOutputAfterPass: true }, { holdAfterSourceWrite: true, resumeRoot: 'public' },
  { holdAfterSourceWrite: true, continueFrom: 'public' }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...extra }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
let fixtureRoot;
await assert.rejects(verifyNativeDevelopment({ ...base, executionAccountHash: 'a'.repeat(64),
  onReservation: binding => {
    fixtureRoot = binding.root;
    prepareDevelopmentInterruption(binding.root, { model: 'sol', localNative: true, powershell: base.powershell,
      accountHash: 'a'.repeat(64), managerPid: process.pid });
  } }), { message: 'INTERRUPTION_MANAGER_ACTIVE' }); checks++;
equal(existsSync(join(fixtureRoot, 'process.json')), false);
equal(existsSync(join(fixtureRoot, 'interruption-stop-intent.json')), false);
equal(existsSync(join(fixtureRoot, 'interruption-evidence.json')), false);
for (const value of [null, [], {}, { version: 1, kind: 'manager-interruption', taskCompleted: true },
  { version: 1, kind: 'manager-interruption', originalResultAbsent: true, sourceReviewed: true, independentPassed: true, observed: {} }]) {
  const fixture = createDevelopmentFixture({ waitMode: 'check' });
  writeFileSync(join(fixture.root, 'interruption-evidence.json'), JSON.stringify(value) + '\n', { flag: 'wx' });
  assert.throws(() => readDevelopmentInterruption(fixture.root), { message: 'INTERRUPTION_RECORD_INVALID' }); checks++;
  equal(existsSync(join(fixture.root, 'finish-intent.json')), false);
}
assert.throws(() => readDevelopmentInterruption('C:\\PublicFixture\\outside-root'), { message: 'INTERRUPTION_ROOT_INVALID' }); checks++;
const interruptedRoots = [];
for (const missingGuard of [false, true]) {
  const child = spawnSync(process.execPath, [fileURLToPath(import.meta.url), '--reserve-only'], {
    env: Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]])),
    windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 16384
  });
  equal(Boolean(child.error), false); equal(child.status, 73); equal(child.stderr, '');
  const { root } = JSON.parse(child.stdout); interruptedRoots.push(root);
  assert.throws(() => process.kill(child.pid, 0), { code: 'ESRCH' }); checks++;
  equal(existsSync(join(root, 'process.json')), false);
  // The manager exited before native launch; this synthetic owner permits only
  // the stop-intent admission check, never an actual native termination.
  writeFileSync(join(root, 'process.json'), JSON.stringify({ pid: child.pid, startedAt: 0,
    sessionId: '11111111-1111-1111-1111-111111111111' }) + '\n', { flag: 'wx' });
  const recover = () => prepareDevelopmentInterruption(root, { model: 'sol', localNative: true,
    powershell: base.powershell, accountHash: 'a'.repeat(64), managerPid: child.pid });
  if (missingGuard) {
    equal(existsSync(base.powershell), false);
    assert.throws(recover, { message: 'INTERRUPTION_OWNER_UNVERIFIED' }); checks++;
    equal(existsSync(join(root, 'interruption-stop-intent.json')), true);
  } else writeFileSync(join(root, 'interruption-stop-intent.json'), '{', { flag: 'wx' });
  assert.throws(recover, { message: 'INTERRUPTION_STOP_UNVERIFIED' }); checks++;
  equal(existsSync(join(root, 'interruption-stop-result.json')), false);
  equal(existsSync(join(root, 'interruption-evidence.json')), false);
  equal(existsSync(join(root, 'finish-intent.json')), false);
}
console.log(JSON.stringify({ suite: 'development-interruption', checks, fixtureRoot,
  interruptedRoots, liveManagerRejectedBeforeNativeStart: true, lostStopResultNotRetried: true,
  actualManagerProcesses: 2, nativeStarts: 0, stopHelperStarts: 0, missingGuardAttempts: 1,
  actualModelRequests: 0, actualCredentialReads: 0 }));
