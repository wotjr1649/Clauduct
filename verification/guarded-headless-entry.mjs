import { readFileSync, lstatSync } from 'node:fs';
import { dirname } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { createFixtureToolPolicy, guardFixtureTransport } from './fixture-tool-policy.mjs';
import { createVerificationLedger } from './verification-ledger.mjs';
import { NativeError } from '../src/native-protocol.mjs';

let guarded, ledger, progressTimer, progressFailed = false, progressSamples = 0;
const progressStarted = performance.now();
const checkProgress = () => { if (progressFailed) throw new NativeError('VERIFICATION_PROGRESS_FAILED'); };
function emitProgress() {
  if (!guarded) return;
  if (progressSamples >= 128) throw new NativeError('VERIFICATION_PROGRESS_FAILED');
  const record = { version: 1, sequence: ++progressSamples, elapsedMs: Math.round(performance.now() - progressStarted), ...guarded.fixtureProgress() };
  process.stderr.write(`CLAUDUCT_FIXTURE_PROGRESS ${JSON.stringify(record)}\n`);
}
try {
  const [policyPath, ...args] = process.argv.slice(2);
  const stat = lstatSync(policyPath);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 16384) throw new Error('INVALID_POLICY');
  const policy = JSON.parse(readFileSync(policyPath, 'utf8'));
  createFixtureToolPolicy(policy);
  ledger = createVerificationLedger(dirname(policyPath));
  await main({ args, openTransport: options => {
    const factory = options.transportFactory;
    const transport = openUserTransport({ ...options, transportFactory: configuration => factory({ ...configuration,
      onAttempt: value => { checkProgress(); ledger.record('attempt', { ...guarded?.fixtureUsage(), requestAttempts: value.requestAttempts }); } }) });
    guarded = guardFixtureTransport(transport, policy, {
      onUsage: value => { checkProgress(); ledger.record('usage', value); },
      onUsageUnobserved: value => { checkProgress(); ledger.record('usage-unobserved', value); }
    });
    emitProgress();
    progressTimer = setInterval(() => {
      try { emitProgress(); }
      catch { progressFailed = true; clearInterval(progressTimer); }
    }, 1000);
    return guarded;
  } });
} catch (error) { process.stderr.write(`GUARDED_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1; }
finally {
  clearInterval(progressTimer);
  try { checkProgress(); emitProgress(); }
  catch { process.stderr.write('GUARDED_ENTRY VERIFICATION_PROGRESS_FAILED\n'); process.exitCode = 1; }
  if (ledger) {
    try { ledger.record('final', guarded?.fixtureUsage() ?? {}); }
    catch { process.stderr.write('GUARDED_ENTRY VERIFICATION_LEDGER_FAILED\n'); process.exitCode = 1; }
    finally { ledger.close(); }
  }
  if (guarded) process.stderr.write(`CLAUDUCT_FIXTURE_USAGE ${JSON.stringify(guarded.fixtureUsage())}\n`);
}
