import { readFileSync, lstatSync } from 'node:fs';
import { main } from '../src/clauduct.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { createFixtureToolPolicy, guardFixtureTransport } from './fixture-tool-policy.mjs';

let guarded;
try {
  const [policyPath, ...args] = process.argv.slice(2);
  const stat = lstatSync(policyPath);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 16384) throw new Error('INVALID_POLICY');
  const policy = JSON.parse(readFileSync(policyPath, 'utf8'));
  createFixtureToolPolicy(policy);
  await main({ args, openTransport: options => (guarded = guardFixtureTransport(openUserTransport(options), policy)) });
} catch (error) { process.stderr.write(`GUARDED_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1; }
finally { if (guarded) process.stderr.write(`CLAUDUCT_FIXTURE_USAGE ${JSON.stringify(guarded.fixtureUsage())}\n`); }
