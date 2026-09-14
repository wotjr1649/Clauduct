import assert from 'node:assert/strict';
import { createNativeResponse, nativeResponse, prepareNative } from './native-protocol.mjs';

const prepared = prepareNative({ model: 'luna', stream: true, max_tokens: 100,
  messages: [{ role: 'user', content: 'PUBLIC_KEEPALIVE' }],
  tools: [{ name: 'Read', input_schema: { type: 'object', properties: {} } }] });
const call = { id: 'public_call', type: 'function_call', call_id: 'public_call', name: 'Read', arguments: '{}', status: 'completed' };
const events = () => [
  { type: 'response.created', response: { id: 'public_response', status: 'in_progress' } },
  { type: 'response.output_item.added', output_index: 0, item: { ...call, arguments: '', status: 'in_progress' } },
  { type: 'response.function_call_arguments.delta', output_index: 0, item_id: call.id, delta: '{}' },
  { type: 'response.function_call_arguments.done', output_index: 0, item_id: call.id, arguments: '{}' },
  { type: 'response.output_item.done', output_index: 0, item: call },
  { type: 'response.completed', response: { id: 'public_response', status: 'completed', model: prepared.selected.model,
    reasoning: { effort: prepared.selected.effort }, output: [call], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }
];
let checks = 0;
const baseline = nativeResponse(events(), prepared);
for (const index of [1, 2, 3, 4, 5]) {
  const withHeartbeat = events(); withHeartbeat.splice(index, 0, { type: 'keepalive' });
  assert.deepEqual(nativeResponse(withHeartbeat, prepared), baseline); checks++;
}
const parser = createNativeResponse(prepared);
for (const event of events().slice(0, -1)) assert.deepEqual(parser.push(event), []);
for (let index = 0; index < 20; index++) assert.deepEqual(parser.push({ type: 'keepalive' }), []);
assert.throws(() => parser.finish(), error => error.code === 'INCOMPLETE_RESPONSE'); checks++;
for (const event of [
  { type: 'keepalive', sequence_number: 1 }, { type: 'keepalive', response_id: 'public_response' },
  { type: 'keepalive', output: [] }, { type: 'keepalive', response: {} }, { type: 'keepalive', error: {} },
  { type: 'keepalive', metadata: {} }, { type: 'keepalive', delta: 'SYNTHETIC_PRIVATE' },
  { type: 'keepalive', arbitrary: 'SYNTHETIC_PRIVATE' }, { type: 'keepalive.other' }, { type: 'KeepAlive' }
]) {
  const parser = createNativeResponse(prepared);
  parser.push(events()[0]);
  assert.throws(() => parser.push(event), error => error.code === 'UNSUPPORTED_EVENT');
  assert.throws(() => parser.finish(), error => error.code === 'UNSUPPORTED_EVENT'); checks++;
}
const early = createNativeResponse(prepared);
assert.throws(() => early.push({ type: 'keepalive' }), error => error.code === 'MISSING_RESPONSE_START'); checks++;
const late = createNativeResponse(prepared);
for (const event of events()) late.push(event);
assert.throws(() => late.push({ type: 'keepalive' }), error => error.code === 'EVENT_AFTER_COMPLETION'); checks++;
const failed = createNativeResponse(prepared);
failed.push(events()[0]); failed.push({ type: 'keepalive' });
assert.throws(() => failed.push({ type: 'error', error: { code: 'server_error' } }), error => error.code === 'UPSTREAM_ERROR_EVENT');
assert.throws(() => failed.finish(), error => error.code === 'UPSTREAM_ERROR_EVENT'); checks++;
console.log(JSON.stringify({ suite: 'keepalive', checks, acceptedShape: 'type-only-after-created-before-completed',
  toolDeliveryRequiresCompletion: true, externalRequests: 0, actualCredentialReads: 0 }));
