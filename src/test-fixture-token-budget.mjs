import assert from 'node:assert/strict';
import { fixtureTokenBudget } from '../verification/fixture-token-budget.mjs';
import { guardFixtureTransport } from '../verification/fixture-tool-policy.mjs';
import { verifyNativeDevelopment } from '../verification/verify-native-development.mjs';
import { readdirSync, mkdirSync } from 'node:fs';

let checks = 0;
assert.deepEqual(fixtureTokenBudget(), { maxInputTokens: 131072, maxOutputTokens: 32768, outputTokenLimitPolicy: 'usage-enforced-completion' }); checks++;
assert.equal(fixtureTokenBudget({ maxOutputTokens: 30000 }).maxOutputTokens, 30000); checks++;
for (const options of [null, [], true, { maxOuputTokens: 30000 }, { maxInputTokens: 0 }, { maxInputTokens: 131073 },
  { maxInputTokens: '20' }, { maxOutputTokens: 0 }, { maxOutputTokens: -1 }, { maxOutputTokens: 32769 },
  { maxOutputTokens: 1.5 }, { maxOutputTokens: '30000' }, { requirePreGenerationLimit: 'false' }]) {
  assert.throws(() => fixtureTokenBudget(options), { code: 'VERIFICATION_TOKEN_BUDGET_INVALID' }); checks++;
}
let sends = 0, delivered = 0;
const transport = { send: async (_body, _signal, options) => {
  sends++;
  const event = { type: 'response.completed', response: { output: [], usage: { input_tokens: 2, output_tokens: 30001 } } };
  if (options.onEvent) await options.onEvent(event);
  return [event];
} };
const policy = { version: 1, kind: 'none', workingRoot: 'PUBLIC_FIXTURE', tokenBudget: { maxOutputTokens: 30000 } };
assert.throws(() => guardFixtureTransport(transport, { ...policy, tokenBudget: { ...policy.tokenBudget, requirePreGenerationLimit: true } }),
  { code: 'VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE' });
assert.equal(sends, 0); checks++;
for (const streaming of [false, true]) {
  let observed;
  const guarded = guardFixtureTransport(transport, policy, { onUsage: value => { observed = value; } });
  await assert.rejects(guarded.send({}, AbortSignal.timeout(1000), streaming ? { onEvent: () => { delivered++; } } : {}), { code: 'REQUEST_BUDGET' });
  assert.equal(observed.maxOutputTokens, 30000); assert.equal(observed.outputTokens, 30001);
  const before = sends;
  await assert.rejects(guarded.send({}, AbortSignal.timeout(1000)), { code: 'REQUEST_BUDGET' });
  assert.equal(sends, before); assert.equal(delivered, 0); checks++;
}
mkdirSync(new URL('../.tmp/', import.meta.url), { recursive: true });
const before = readdirSync(new URL('../.tmp/', import.meta.url));
let reservations = 0;
await assert.rejects(verifyNativeDevelopment({ model: 'sol', powershell: 'C:\\PUBLIC_FIXTURE\\pwsh.exe',
  priorAttempts: 327, priorElapsedMs: 3179404, priorInputTokens: 1910442, priorOutputTokens: 244298,
  maxObservedOutputTokens: 30000, requirePreGenerationLimit: true, onReservation: () => { reservations++; } }),
{ code: 'VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE' });
assert.equal(reservations, 0); assert.deepEqual(readdirSync(new URL('../.tmp/', import.meta.url)), before); checks++;
console.log(JSON.stringify({ checks, passed: true, actualBackendRequests: 0, credentialReads: 0, hardBudgetBeforeNative: true }));
