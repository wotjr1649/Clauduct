import assert from 'node:assert/strict';
import { test } from 'node:test';
import { guardFixtureTransport } from '../verification/fixture-tool-policy.mjs';

const policy = { version: 1, kind: 'websearch', workingRoot: 'SYNTHETIC_WORK' };
const completed = usage => ({ type: 'response.completed', response: { output: [], usage } });
const signal = new AbortController().signal;

for (const streaming of [false, true]) {
  for (const [label, usage] of [
    ['missing', undefined], ['null', null], ['array', []], ['empty', {}],
    ['missing-output', { input_tokens: 1 }], ['negative', { input_tokens: -1, output_tokens: 1 }],
    ['fractional', { input_tokens: 1.5, output_tokens: 1 }], ['string', { input_tokens: '1', output_tokens: 1 }],
    ['unsafe', { input_tokens: Number.MAX_SAFE_INTEGER + 1, output_tokens: 1 }]
  ]) test(`${streaming ? 'stream' : 'buffer'} rejects ${label} usage before delivery and further egress`, async () => {
    let requests = 0, delivered = 0, observations = 0;
    const transport = {
      diagnostics: () => ({ requestAttempts: requests }),
      send: async (_body, _signal, options) => {
        requests++;
        const event = completed(usage);
        if (options.onEvent) await options.onEvent(event);
        return [event];
      },
      search: async () => { requests++; }
    };
    const guarded = guardFixtureTransport(transport, policy, { onUsageUnobserved: value => {
      observations++;
      assert.equal(value.unobservedCompletions, 1);
      assert.equal(value.completions, 0);
    } });
    const options = streaming ? { onEvent: () => { delivered++; } } : {};
    await assert.rejects(guarded.send({}, signal, options), error => error.code === 'INVALID_USAGE');
    await assert.rejects(guarded.send({}, signal, options), error => error.code === 'INVALID_USAGE');
    await assert.rejects(guarded.search({}, signal), error => error.code === 'INVALID_USAGE');
    assert.equal(requests, 1);
    assert.equal(delivered, 0);
    assert.equal(observations, 1);
    assert.equal(guarded.fixtureUsage().unobservedCompletions, 1);
    assert.equal(guarded.fixtureProgress().activeCount, 0);
  });
}

test('explicit zero usage remains observed and does not stop the next request', async () => {
  let requests = 0;
  const guarded = guardFixtureTransport({ diagnostics: () => ({ requestAttempts: requests }),
    send: async () => { requests++; return [completed({ input_tokens: 0, output_tokens: 0 })]; } }, policy);
  await guarded.send({}, signal); await guarded.send({}, signal);
  assert.equal(requests, 2);
  assert.equal(guarded.fixtureUsage().completions, 2);
  assert.equal(guarded.fixtureUsage().unobservedCompletions, 0);
});

test('observed budget overrun is recorded before rejecting tool delivery', async () => {
  let observed = null, delivered = 0;
  const guarded = guardFixtureTransport({ send: async (_body, _signal, options) => {
    await options.onEvent(completed({ input_tokens: 131073, output_tokens: 1 }));
  } }, policy, { onUsage: value => { observed = value; } });
  await assert.rejects(guarded.send({}, signal, { onEvent: () => { delivered++; } }), error => error.code === 'REQUEST_BUDGET');
  assert.equal(observed.inputTokens, 131073);
  assert.equal(observed.completions, 1);
  assert.equal(delivered, 0);
});

test('a failed missing-usage observer still leaves the next request stopped', async () => {
  let requests = 0;
  const guarded = guardFixtureTransport({ send: async () => { requests++; return [completed(null)]; } }, policy,
    { onUsageUnobserved: () => { throw new Error('PUBLIC_OBSERVER_FAILURE'); } });
  await assert.rejects(guarded.send({}, signal), /PUBLIC_OBSERVER_FAILURE/);
  await assert.rejects(guarded.send({}, signal), error => error.code === 'INVALID_USAGE');
  assert.equal(requests, 1);
});
