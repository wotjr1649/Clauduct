import assert from 'node:assert/strict';
import { observeRateLimitHeaders, rateLimitEvidence } from './rate-limit-observation.mjs';

const family = (prefix, percent = '42') => ['primary', 'secondary'].flatMap(window =>
  ['used-percent', 'window-minutes', 'reset-at'].flatMap((suffix, index) =>
    [`x-${prefix}-${window}-${suffix}`, [percent, '300', '1800000000'][index]]));
const record = observation => ({ at: 1800000000000, event: 'RESPONSE_LIMITS', requestAttempt: 1,
  kind: 'responses', httpStatus: 200, observation });
let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const snapshot = { state: 'observed', primary: { usedPercent: 42, windowMinutes: 300, resetAtSeconds: 1800000000 },
  secondary: { usedPercent: 42, windowMinutes: 300, resetAtSeconds: 1800000000 } };
const extra = observeRateLimitHeaders(family('SYNTHETIC_PRIVATE'));
equal(extra.otherLimits, [snapshot]);
equal(extra.state, 'missing'); equal(extra.otherLimitFamilies, 1); equal(extra.otherLimitsTruncated, false);
equal(Object.isFrozen(extra.otherLimits) && Object.isFrozen(extra.otherLimits[0].primary), true);
equal(JSON.stringify(extra).includes('SYNTHETIC_PRIVATE'), false);
equal(rateLimitEvidence([record(extra)]).latest.observation, extra);
const multiple = observeRateLimitHeaders([...family('codex', '1'), ...family('public-z', '20'), ...family('public-a', '30')]);
equal(multiple.primary.usedPercent, 1);
equal(multiple.otherLimits.map(limit => limit.primary.usedPercent), [30, 20]);
const invalid = observeRateLimitHeaders([...family('public'), 'x-public-primary-used-percent', 'SYNTHETIC_PRIVATE']);
equal(invalid.otherLimits[0].state, 'invalid');
equal(invalid.otherLimits[0].primary.usedPercent, null);
equal(invalid.otherLimits[0].secondary.usedPercent, 42);
equal(invalid.otherLimits[0].invalidReason, 'duplicate');
equal(JSON.stringify(invalid).includes('SYNTHETIC_PRIVATE'), false);
equal(rateLimitEvidence([record(invalid)]).latest.observation, invalid);
const partial = observeRateLimitHeaders(['x-public-primary-used-percent', '0']);
equal(partial.otherLimits[0].state, 'partial');
equal(partial.otherLimits[0].primary, { usedPercent: 0, windowMinutes: null, resetAtSeconds: null });
const many = observeRateLimitHeaders(Array.from({ length: 9 }, (_, index) => family(`public-${index}`)).flat());
equal(many.otherLimitFamilies, 9); equal(many.otherLimits.length, 8); equal(many.otherLimitsTruncated, true);
equal(rateLimitEvidence([record(many)]).latest.observation.otherLimits.length, 8);
const legacy = { state: 'missing', primary: null, secondary: null, otherLimitFamilies: 1 };
equal(rateLimitEvidence([record(legacy)]).latest.observation, legacy);
for (const change of [value => { value.otherLimits = null; }, value => { value.otherLimits = []; },
  value => { value.otherLimitsTruncated = true; }, value => { delete value.otherLimitsTruncated; },
  value => { value.otherLimitFamilies = 0; }, value => { value.otherLimits.push(snapshot); },
  value => { value.otherLimits[0].name = 'SYNTHETIC_PRIVATE'; },
  value => { value.otherLimits[0].otherLimits = [snapshot]; },
  value => { value.otherLimits[0].primary.usedPercent = '42'; },
  value => { value.otherLimits[0].secondary = null; },
  value => { value.otherLimits[0].state = 'missing'; },
  value => { value.otherLimits[0].primary.windowMinutes = -1; }]) {
  const value = structuredClone(extra); change(value);
  assert.throws(() => rateLimitEvidence([record(value)]), { message: 'RATE_LIMIT_EVIDENCE_INVALID' }); checks++;
}
console.log(JSON.stringify({ suite: 'additional-rate-limits', checks, rawHeadersPersisted: false,
  externalRequests: 0, actualCredentialReads: 0 }));
