import assert from 'node:assert/strict';
import { developmentUsageEvidence } from '../verification/verify-native-development.mjs';

const observed = { attempts: 5, attemptsComplete: true, ledgerMatched: true, usageUnobservedAttempts: 0 };
const missing = { attempts: 3, attemptsComplete: true, ledgerMatched: true, usageUnobservedAttempts: 2 };
let checks = 0;
function expect(first, next, failure, count, uncertain = false) {
  assert.deepEqual(developmentUsageEvidence(first, next, failure),
    { usageUnobservedAttempts: count, continuationUsageUnobserved: uncertain });
  checks++;
}
expect(observed, observed, null, 0);
expect(observed, missing, null, 2);
expect(missing, { ...missing, usageUnobservedAttempts: 1 }, null, 3);
expect(observed, null, null, 0); // No second phase was started.
expect(missing, null, null, 2);
expect(observed, null, 'DEVELOPMENT_RESUME_FAILED', null, true);
expect(observed, null, 'CONTINUATION_FAILED', null, true);
expect(observed, null, 'RESUME_OWNER_UNVERIFIED', null, true);
expect(observed, observed, 'CONTINUATION_FAILED', null, true);
expect(observed, observed, undefined, null, true);
for (const value of [null, undefined, -1, 6, 1.5, '0', false, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1]) {
  expect({ ...observed, usageUnobservedAttempts: value }, observed, null, null);
  expect(observed, { ...observed, usageUnobservedAttempts: value }, null, null);
}
for (const phase of [null, {}, { ...observed, attemptsComplete: false }, { ...observed, attemptsComplete: 1 },
  { ...observed, ledgerMatched: false }, { ...observed, ledgerMatched: undefined },
  { ...observed, attempts: -1 }, { ...observed, attempts: '5' }, { ...observed, attempts: 1.5 }]) {
  expect(phase, observed, null, null);
  if (phase !== null) expect(observed, phase, null, null);
}
expect(observed, undefined, null, null);
const maximum = { ...observed, attempts: Number.MAX_SAFE_INTEGER, usageUnobservedAttempts: Number.MAX_SAFE_INTEGER };
expect(maximum, null, null, Number.MAX_SAFE_INTEGER);
expect(maximum, { ...observed, usageUnobservedAttempts: 1 }, null, null);
expect({ ...observed, attempts: 0 }, observed, null, 0);
console.log(JSON.stringify({ suite: 'development-usage-summary', checks, actualNativeExecutions: 0,
  actualModelRequests: 0, actualCredentialReads: 0 }));
