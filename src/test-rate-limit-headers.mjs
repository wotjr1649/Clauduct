import assert from 'node:assert/strict';
import { observeRateLimitHeaders, rateLimitEvidence } from './rate-limit-observation.mjs';

const names = ['x-codex-primary-used-percent', 'x-codex-primary-window-minutes', 'x-codex-primary-reset-at',
  'x-codex-secondary-used-percent', 'x-codex-secondary-window-minutes', 'x-codex-secondary-reset-at'];
const raw = () => names.flatMap((name, index) => [name, ['12.5', '300', '1800000000', '37', '10080', '1800500000'][index]]);
const observed = observeRateLimitHeaders(raw());
let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
equal(observed, { state: 'observed', primary: { usedPercent: 12.5, windowMinutes: 300, resetAtSeconds: 1800000000 },
  secondary: { usedPercent: 37, windowMinutes: 10080, resetAtSeconds: 1800500000 }, otherLimitFamilies: 0 });
equal(Object.isFrozen(observed) && Object.isFrozen(observed.primary) && Object.isFrozen(observed.secondary), true);
equal(observeRateLimitHeaders([]), { state: 'missing', primary: null, secondary: null, otherLimitFamilies: 0 });
equal(observeRateLimitHeaders([names[0], '0']).primary, { usedPercent: 0, windowMinutes: null, resetAtSeconds: null });
equal(observeRateLimitHeaders([names[0], '0']).state, 'partial');
equal(observeRateLimitHeaders(raw().map((value, index) => index % 2 ? ` \t${value}\t ` : value.toUpperCase())), observed);
equal(observeRateLimitHeaders(names.flatMap(name => [name, '0'])), { state: 'observed',
  primary: { usedPercent: 0, windowMinutes: 0, resetAtSeconds: 0 },
  secondary: { usedPercent: 0, windowMinutes: 0, resetAtSeconds: 0 }, otherLimitFamilies: 0 });
for (const value of ['', ' ', '-1', '+1', 'NaN', 'Infinity', '1e2', '0x10', '.5', '1.', '12\n', '12\r', '12\r\n', '12\u00a0',
  '1, 2', 'SYNTHETIC_PRIVATE', '1'.repeat(65), ['12'], 12, null]) {
  equal(observeRateLimitHeaders([names[0], value]).state, 'invalid');
}
for (const [name, value] of [[names[0], '100.001'], [names[1], '1.5'], [names[1], '150119987580'],
  [names[2], '9007199254741'], [names[2], '9007199254740993']]) {
  equal(observeRateLimitHeaders([name, value]).state, 'invalid');
}
for (const input of [null, {}, ['odd'], Array(1026).fill('x'), [null, '1'], ['x'.repeat(257), '1']]) {
  equal(observeRateLimitHeaders(input).state, 'invalid');
}
for (const key of names) {
  equal(observeRateLimitHeaders([...raw(), key, '1']).state, 'invalid');
  equal(observeRateLimitHeaders([...raw(), key.toUpperCase(), '1']).state, 'invalid');
}
const extras = observeRateLimitHeaders([...raw(), 'x-public-SYNTHETIC_PRIVATE-primary-used-percent', 'SYNTHETIC_PRIVATE',
  'x-codex-credits-balance', 'SYNTHETIC_PRIVATE', 'set-cookie', 'SYNTHETIC_PRIVATE']);
equal(extras.otherLimitFamilies, 1);
equal(JSON.stringify(extras).includes('SYNTHETIC_PRIVATE'), false);
const malformedOptional = raw(); malformedOptional[11] = 'SYNTHETIC_PRIVATE';
equal(observeRateLimitHeaders(malformedOptional).primary, observed.primary);
equal(observeRateLimitHeaders(malformedOptional).secondary, { usedPercent: 37, windowMinutes: 10080, resetAtSeconds: null });
equal(observeRateLimitHeaders(malformedOptional).state, 'invalid');
const signedReset = raw(); signedReset[11] = '-1';
equal(observeRateLimitHeaders(signedReset).secondary.resetAtSeconds, -1);
equal(observeRateLimitHeaders(signedReset).state, 'observed');
for (const [input, reason, field] of [[null, 'header-shape', null],
  [[...raw(), names[4], '10'], 'duplicate', 'secondary.windowMinutes'],
  [[names[2], 'SYNTHETIC_PRIVATE'], 'numeric-format', 'primary.resetAtSeconds'],
  [[names[0], '100.01'], 'numeric-range', 'primary.usedPercent']]) {
  const value = observeRateLimitHeaders(input);
  equal(value.invalidReason, reason); equal(value.invalidField, field);
  equal(JSON.stringify(value).includes('SYNTHETIC_PRIVATE'), false);
}
const record = (number = 1, observation = observed) => ({ at: 1800000000000, event: 'RESPONSE_LIMITS',
  requestAttempt: number, kind: 'responses', httpStatus: 200, observation });
equal(rateLimitEvidence([]), { responseCount: 0, counts: { missing: 0, partial: 0, observed: 0, invalid: 0 },
  latest: null, authorizesAdditionalRequests: false });
const evidence = rateLimitEvidence([record(), record(2, observeRateLimitHeaders([]))]);
equal(evidence.counts, { missing: 1, partial: 0, observed: 1, invalid: 0 });
equal(evidence.latest.observation.primary, null);
equal(evidence.authorizesAdditionalRequests, false);
equal(rateLimitEvidence([record(1, observeRateLimitHeaders(malformedOptional))]).latest.observation.primary, observed.primary);
equal(rateLimitEvidence([record(1, observeRateLimitHeaders(signedReset))]).latest.observation.secondary.resetAtSeconds, -1);
const duplicateAgain = observeRateLimitHeaders([...raw(), names[0], '20', names[0], '30']);
equal(duplicateAgain.primary.usedPercent, null);
equal(rateLimitEvidence([record(1, duplicateAgain)]).latest.observation.state, 'invalid');
equal(rateLimitEvidence([record(1, observeRateLimitHeaders([names[0], 'bad']))]).latest.observation.invalidReason, 'numeric-format');
equal(rateLimitEvidence([record(1, { state: 'invalid', primary: null, secondary: null, otherLimitFamilies: 1 })]).latest.observation.invalidReason, null);
for (const override of [{ invalidReason: 'SYNTHETIC_PRIVATE' }, { invalidField: 'SYNTHETIC_PRIVATE' }, { invalidField: null },
  { invalidReason: 'header-shape', invalidField: 'primary.usedPercent' }]) {
  const value = { ...observeRateLimitHeaders([names[0], 'bad']), ...override };
  assert.throws(() => rateLimitEvidence([record(1, value)]), { message: 'RATE_LIMIT_EVIDENCE_INVALID' }); checks++;
}
// Completion order may differ from attempt order; preserve receipt order and
// bind each observation to its own attempt rather than the current total.
equal(rateLimitEvidence([record(2), record(1)]).latest.requestAttempt, 1);
for (const change of [row => { row.extra = 'SYNTHETIC_PRIVATE'; }, row => { row.at = NaN; },
  row => { row.kind = 'SYNTHETIC_PRIVATE'; }, row => { row.httpStatus = 99; },
  row => { row.requestAttempt = 0; }, row => { row.observation.extra = 'SYNTHETIC_PRIVATE'; },
  row => { row.observation.state = 'SYNTHETIC_PRIVATE'; }, row => { row.observation.primary = null; },
  row => { row.observation.primary.usedPercent = '0'; }, row => { row.observation.primary.resetAtSeconds = -9007199254741; },
  row => { row.observation.primary.windowMinutes = 1.5; }, row => { row.observation.otherLimitFamilies = 513; },
  row => { row.observation.state = 'missing'; }, row => { row.observation.state = 'partial'; }]) {
  const row = structuredClone(record()); change(row);
  assert.throws(() => rateLimitEvidence([row]), { message: 'RATE_LIMIT_EVIDENCE_INVALID' }); checks++;
}
for (const rows of [null, {}, [record(), record()], Array(4097).fill(record())]) {
  assert.throws(() => rateLimitEvidence(rows), { message: 'RATE_LIMIT_EVIDENCE_INVALID' }); checks++;
}
console.log(JSON.stringify({ suite: 'rate-limit-headers', checks, rawHeadersPersisted: false,
  externalRequests: 0, actualCredentialReads: 0 }));
