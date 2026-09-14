import assert from 'node:assert/strict';
import { publicDevelopmentEvents } from '../verification/fixtures/development-responses.mjs';
import { prepareNative } from './native-protocol.mjs';

let checks = 0;
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
for (const [model, effort] of [['sol', 'low'], ['luna', 'max']]) {
  const messages = [{ role: 'user', content: 'PUBLIC_DEVELOPMENT_SEQUENCE' }];
  const calls = new Set(), responses = new Set(), items = new Set();
  for (const [instanceId, taskId, finish, variant] of [
    ['first', 'retry-after-seconds', false, 'complete'],
    ['first', 'retry-after-seconds', true, 'complete'],
    ['second', 'retry-delay-window', false, 'complete'],
    ['third', 'retry-after-seconds', false, 'complete'],
    ['fourth', 'retry-delay-window', false, 'early'],
    ['fourth', 'retry-delay-window', false, 'retry']
  ]) {
    const count = variant === 'early' ? 2 : finish ? 3 : 5;
    for (let serial = 1; serial <= count; serial++) {
      const response = publicDevelopmentEvents(`gpt-5.6-${model}`, effort, serial, finish, taskId, variant, instanceId).at(-1).response;
      const item = response.output[0];
      equal(responses.has(response.id), false); responses.add(response.id);
      equal(items.has(item.id), false); items.add(item.id);
      if (item.type !== 'function_call') continue;
      equal(calls.has(item.call_id), false); calls.add(item.call_id);
      messages.push({ role: 'assistant', content: [{ type: 'tool_use', id: item.call_id, name: item.name, input: JSON.parse(item.arguments) }] },
        { role: 'user', content: [{ type: 'tool_result', tool_use_id: item.call_id, content: 'PUBLIC_RESULT' }] });
    }
  }
  const doc = { model, output_config: { effort }, stream: true, max_tokens: 1000, messages };
  const prepared = prepareNative(doc);
  equal(prepared.body.input.filter(item => item.type === 'function_call').length, calls.size);
  assert.throws(() => prepareNative({ ...doc, messages: [...messages, messages[1], messages[2]] }), { code: 'INVALID_TOOL_CALL' }); checks++;
}
for (const instanceId of [null, {}, '../outside', 'a b', 'x'.repeat(41)]) {
  assert.throws(() => publicDevelopmentEvents('gpt-5.6-sol', 'low', 1, false, 'retry-after-seconds', 'complete', instanceId),
    { message: 'DEVELOPMENT_STIMULUS_INVALID' }); checks++;
}
console.log(JSON.stringify({ suite: 'development-response-identity', checks, externalRequests: 0,
  repeatedTaskIds: true, duplicateHistoryRejected: true }));
