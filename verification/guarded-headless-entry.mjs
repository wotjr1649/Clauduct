import { readFileSync, lstatSync } from 'node:fs';
import { dirname } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { createFixtureToolPolicy, guardFixtureTransport } from './fixture-tool-policy.mjs';
import { createVerificationLedger } from './verification-ledger.mjs';

let guarded, ledger;
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
      onAttempt: value => ledger.record('attempt', { ...guarded?.fixtureUsage(), requestAttempts: value.requestAttempts }) }) });
    return (guarded = guardFixtureTransport(transport, policy, { onUsage: value => ledger.record('usage', value) }));
  } });
} catch (error) { process.stderr.write(`GUARDED_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1; }
finally {
  if (ledger) {
    try { ledger.record('final', guarded?.fixtureUsage() ?? {}); }
    catch { process.stderr.write('GUARDED_ENTRY VERIFICATION_LEDGER_FAILED\n'); process.exitCode = 1; }
    finally { ledger.close(); }
  }
  if (guarded) process.stderr.write(`CLAUDUCT_FIXTURE_USAGE ${JSON.stringify(guarded.fixtureUsage())}\n`);
}
