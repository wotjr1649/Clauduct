import assert from 'node:assert/strict';
import { createNativeResponse, nativeResponse, prepareNative } from './native-protocol.mjs';
import { mergeReasoning } from '../poc/adapter.mjs';

const tool = (name, defer_loading) => ({ name, description: 'Synthetic tool', ...(defer_loading ? { defer_loading: true } : {}),
  input_schema: { type: 'object', properties: { value: { type: 'string' } }, required: ['value'], additionalProperties: false } });
const doc = (tools = [tool('Read')], messages = [{ role: 'user', content: 'SYNTHETIC_PROMPT' }]) => ({
  model: 'astra', max_tokens: 100, stream: true, messages, tools
});
const usage = { input_tokens: 4, output_tokens: 3, total_tokens: 7 };
function responseEvents(prepared, { reasoning = false, failure = false, annotations = [] } = {}) {
  const events = [{ type: 'response.created', response: { id: 'resp_protocol', status: 'in_progress' } }];
  let index = 0;
  const output = [];
  if (reasoning) {
    const item = { type: 'reasoning', id: 'rs_protocol', status: 'in_progress', summary: [], encrypted_content: 'OPAQUE_REASONING' };
    const done = { ...item, status: 'completed' };
    events.push({ type: 'response.output_item.added', output_index: index, item },
      { type: 'response.output_item.done', output_index: index, item: done });
    output.push(done); index++;
  }
  const message = { type: 'message', id: 'msg_protocol', role: 'assistant', status: 'completed', phase: 'final_answer',
    content: [{ type: 'output_text', text: 'hello', annotations }] };
  const firstMessage = { ...message, status: 'in_progress', content: [] };
  events.push({ type: 'response.output_item.added', output_index: index, item: firstMessage },
    { type: 'response.output_text.delta', output_index: index, item_id: message.id, content_index: 0, delta: 'he' },
    { type: 'response.output_text.delta', output_index: index, item_id: message.id, content_index: 0, delta: 'llo' },
    { type: 'response.output_text.done', output_index: index, item_id: message.id, content_index: 0, text: 'hello' },
    { type: 'response.output_item.done', output_index: index, item: message });
  output.push(message); index++;
  const call = { type: 'function_call', id: 'fc_protocol', call_id: 'call_protocol', name: 'Read',
    arguments: JSON.stringify({ value: 'SYNTHETIC_VALUE' }), status: 'completed' };
  const firstCall = { ...call, status: 'in_progress', arguments: '' };
  events.push({ type: 'response.output_item.added', output_index: index, item: firstCall },
    { type: 'response.function_call_arguments.delta', output_index: index, item_id: call.id, delta: call.arguments },
    { type: 'response.function_call_arguments.done', output_index: index, item_id: call.id, arguments: call.arguments, name: call.name },
    { type: 'response.output_item.done', output_index: index, item: call });
  output.push(call);
  events.push({ type: 'response.completed', response: { id: 'resp_protocol', status: failure ? 'incomplete' : 'completed',
    model: prepared.selected.model, reasoning: { effort: prepared.selected.effort }, output, usage } });
  return events;
}

function collect(parser, events) {
  const frames = [];
  for (const event of events) frames.push(...parser.push(event));
  return frames;
}

assert.throws(() => prepareNative({ ...doc(), tool_choice: { type: 'auto', disable_parallel_tool_use: 'false' } }));
assert.equal(prepareNative({ ...doc(), tool_choice: { type: 'auto', disable_parallel_tool_use: false } }).body.parallel_tool_calls, true);
assert.equal(prepareNative({ ...doc(), tool_choice: { type: 'auto', disable_parallel_tool_use: true } }).body.parallel_tool_calls, false);

const deferredInitial = prepareNative(doc([tool('ToolSearch'), tool('Read', true)]));
assert.deepEqual(deferredInitial.body.tools.map(value => value.name), ['ToolSearch']);
assert.equal(deferredInitial.names.has('Read'), false);
const deferredFollowup = prepareNative(doc([tool('ToolSearch'), tool('Read', true)], [
  { role: 'user', content: 'SYNTHETIC_PROMPT' },
  { role: 'assistant', content: [{ type: 'tool_use', id: 'search_call', name: 'ToolSearch', input: { value: 'Read' } }] },
  { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'search_call', content: [{ type: 'tool_reference', tool_name: 'Read' }] }] }
]));
assert.deepEqual(deferredFollowup.body.tools.map(value => value.name), ['ToolSearch', 'Read']);
assert.throws(() => prepareNative(doc([tool('ToolSearch'), tool('Read', true)], [
  { role: 'user', content: 'SYNTHETIC_PROMPT' },
  { role: 'assistant', content: [{ type: 'tool_use', id: 'search_call', name: 'ToolSearch', input: { value: 'Read' } }] },
  { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'search_call', content: [{ type: 'tool_reference', tool_name: 'Missing' }] }] }
])));
assert.throws(() => prepareNative(doc([tool('Read', true)])));

const prepared = prepareNative(doc());
assert.equal(prepared.outputTokenLimitPolicy, 'usage-enforced-completion');
const parser = createNativeResponse(prepared);
const textFrames = collect(parser, responseEvents(prepared));
assert.ok(textFrames.some(frame => frame.type === 'content_block_delta'));
assert.equal(textFrames.some(frame => JSON.stringify(frame).includes('tool_use')), false);
const finished = parser.finish();
assert.deepEqual(finished.message.content.map(block => block.type), ['text', 'tool_use']);
assert.ok(finished.frames.some(frame => JSON.stringify(frame).includes('tool_use')));
assert.equal(finished.frames.at(-1).type, 'message_stop');
assert.equal(textFrames.find(frame => frame.type === 'content_block_delta').index, 0);
const streamedFramesBeforeFinish = collect(createNativeResponse(prepared), responseEvents(prepared));
const serializedTextFrames = streamedFramesBeforeFinish.map(frame => JSON.stringify(frame));
assert.ok(serializedTextFrames.some(frame => frame.includes('text_delta')));
const usageParser = createNativeResponse(prepared);
const earlyFrames = collect(usageParser, responseEvents(prepared));
const earlyWire = earlyFrames.map(frame => `event: ${frame.type}\ndata: ${JSON.stringify(frame)}\n\n`).join('');
assert.ok(earlyWire.includes('text_delta'));
const usageFinish = usageParser.finish();
const lateWire = usageFinish.frames.map(frame => `event: ${frame.type}\ndata: ${JSON.stringify(frame)}\n\n`).join('');
const usageDelta = JSON.parse(lateWire.split('\n\n').find(frame => frame.startsWith('event: message_delta')).split('\ndata: ')[1]);
assert.deepEqual(usageDelta.usage, { input_tokens: 4, output_tokens: 3, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 });

const failedParser = createNativeResponse(prepared);
const failedFrames = collect(failedParser, responseEvents(prepared, { failure: true }));
assert.throws(() => failedParser.finish());
assert.ok(failedFrames.some(frame => frame.type === 'content_block_delta'));
assert.equal(failedFrames.some(frame => JSON.stringify(frame).includes('tool_use')), false);

const reasoningPrepared = prepareNative(doc());
const reasoningParser = createNativeResponse(reasoningPrepared);
const reasoningEvents = responseEvents(reasoningPrepared, { reasoning: true });
const reasoningTextFrames = collect(reasoningParser, reasoningEvents);
assert.equal(reasoningTextFrames.find(frame => frame.type === 'content_block_delta').index, 0);
const reasoningFinished = reasoningParser.finish();
assert.deepEqual(reasoningFinished.message.content.map(block => block.type), ['text', 'redacted_thinking', 'tool_use']);
assert.equal(reasoningFinished.frames.find(frame => frame.type === 'content_block_start').index, 1);
assert.equal(reasoningFinished.frames.at(-2).type, 'message_delta');
assert.deepEqual(reasoningFinished.frames.at(-2).usage,
  { input_tokens: 4, output_tokens: 3, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 });
// Native Workflow 2.1.266 extracts text from the last yielded assistant block.
// A trailing opaque reasoning block must not replace the final text result.
function workflowEvents(options = {}) {
  const events = responseEvents(reasoningPrepared, { reasoning: true, ...options });
  const filtered = events.filter(event => !event.type.startsWith('response.function_call')
    && event.item?.type !== 'function_call');
  filtered.at(-1).response.output = filtered.at(-1).response.output.filter(item => item.type !== 'function_call');
  return filtered;
}
const workflowParser = createNativeResponse(reasoningPrepared, { deferText: true });
assert.deepEqual(collect(workflowParser, workflowEvents()), []);
const workflowFinished = workflowParser.finish();
assert.deepEqual(workflowFinished.message.content.map(block => block.type), ['redacted_thinking', 'text']);
assert.equal(workflowFinished.message.content.at(-1).text, 'hello');
assert.equal(workflowFinished.message.stop_reason, 'end_turn');
assert.deepEqual(workflowFinished.frames.filter(frame => frame.type === 'content_block_start').map(frame => frame.index), [0, 1]);
assert.equal(workflowFinished.frames.filter(frame => frame.type === 'content_block_delta').map(frame => frame.delta.text ?? '').join(''), 'hello');
const restoredWorkflow = prepareNative(doc([], [{ role: 'user', content: 'SYNTHETIC_PROMPT' },
  { role: 'assistant', content: workflowFinished.message.content }, { role: 'user', content: 'NEXT' }]));
assert.equal(restoredWorkflow.body.input.find(item => item.type === 'reasoning').encrypted_content, 'OPAQUE_REASONING');
const rejectedWorkflow = createNativeResponse(reasoningPrepared, { deferText: true });
assert.deepEqual(collect(rejectedWorkflow, workflowEvents({ failure: true })), []);
assert.throws(() => rejectedWorkflow.finish(), /INCOMPLETE_RESPONSE/);
const toolWorkflow = createNativeResponse(reasoningPrepared, { deferText: true });
assert.deepEqual(collect(toolWorkflow, responseEvents(reasoningPrepared, { reasoning: true })), []);
assert.deepEqual(toolWorkflow.finish().message.content.map(block => block.type), ['redacted_thinking', 'text', 'tool_use']);
const multipleTextEvents = workflowEvents();
const textDonePosition = multipleTextEvents.findIndex(event => event.type === 'response.output_item.done' && event.item.type === 'message');
multipleTextEvents.splice(textDonePosition, 0,
  { type: 'response.output_text.delta', output_index: 1, item_id: 'msg_protocol', content_index: 1, delta: 'SECOND' },
  { type: 'response.output_text.done', output_index: 1, item_id: 'msg_protocol', content_index: 1, text: 'SECOND' });
multipleTextEvents.find(event => event.type === 'response.output_item.done' && event.item.type === 'message').item.content.push({ type: 'output_text', text: 'SECOND' });
const multipleText = createNativeResponse(reasoningPrepared, { deferText: true });
assert.deepEqual(collect(multipleText, multipleTextEvents), []);
assert.equal(multipleText.finish().message.content.at(-1).text, 'hello\nSECOND');
const limitedWorkflow = createNativeResponse({ ...reasoningPrepared, outputLimit: 1 }, { deferText: true });
assert.deepEqual(collect(limitedWorkflow, workflowEvents()), []);
assert.throws(() => limitedWorkflow.finish(), /OUTPUT_TOKEN_LIMIT_EXCEEDED/);
const mismatchedWorkflow = workflowEvents();
mismatchedWorkflow.at(-1).response.output[1] = { ...mismatchedWorkflow.at(-1).response.output[1], content: [{ type: 'output_text', text: 'WRONG' }] };
const mismatchedParser = createNativeResponse(reasoningPrepared, { deferText: true });
assert.deepEqual(collect(mismatchedParser, mismatchedWorkflow), []);
assert.throws(() => mismatchedParser.finish(), /SNAPSHOT_MISMATCH/);

const mergedEvents = responseEvents(reasoningPrepared, { reasoning: true });
const mergedDone = mergedEvents.find(event => event.type === 'response.output_item.done' && event.item.type === 'reasoning').item;
mergedDone.summary = [{ type: 'summary_text', text: 'SYNTHETIC_SUMMARY' }];
mergedEvents.at(-1).response.output[0] = { ...mergedDone };
delete mergedEvents.at(-1).response.output[0].summary;
const mergedNative = nativeResponse(mergedEvents, reasoningPrepared);
const mergedData = mergedNative.message.content.find(block => block.type === 'redacted_thinking').data;
assert.equal(JSON.parse(Buffer.from(mergedData.slice('clauduct-reasoning-v1:'.length), 'base64url').toString()).summary[0].text,
  'SYNTHETIC_SUMMARY');

const interleavedEvents = [{ type: 'response.created', response: { id: 'resp_interleaved', status: 'in_progress' } },
  { type: 'response.output_item.added', output_index: 0, item: { type: 'function_call', id: 'fc_a', call_id: 'call_a', name: 'Read', arguments: '', status: 'in_progress' } },
  { type: 'response.output_item.added', output_index: 1, item: { type: 'function_call', id: 'fc_b', call_id: 'call_b', name: 'Read', arguments: '', status: 'in_progress' } },
  { type: 'response.function_call_arguments.delta', output_index: 0, item_id: 'fc_a', delta: '{"value":"a"}' },
  { type: 'response.function_call_arguments.delta', output_index: 1, item_id: 'fc_b', delta: '{"value":"b"}' },
  { type: 'response.function_call_arguments.done', output_index: 0, item_id: 'fc_a', arguments: '{"value":"a"}' },
  { type: 'response.function_call_arguments.done', output_index: 1, item_id: 'fc_b', arguments: '{"value":"b"}' },
  { type: 'response.output_item.done', output_index: 0, item: { type: 'function_call', id: 'fc_a', call_id: 'call_a', name: 'Read', arguments: '{"value":"a"}', status: 'completed' } },
  { type: 'response.output_item.done', output_index: 1, item: { type: 'function_call', id: 'fc_b', call_id: 'call_b', name: 'Read', arguments: '{"value":"b"}', status: 'completed' } },
  { type: 'response.completed', response: { id: 'resp_interleaved', status: 'completed', model: prepared.selected.model,
    reasoning: { effort: prepared.selected.effort }, output: [
      { type: 'function_call', id: 'fc_a', call_id: 'call_a', name: 'Read', arguments: '{"value":"a"}', status: 'completed' },
      { type: 'function_call', id: 'fc_b', call_id: 'call_b', name: 'Read', arguments: '{"value":"b"}', status: 'completed' }
    ], usage } }];
const interleavedParser = createNativeResponse(prepared);
assert.equal(collect(interleavedParser, interleavedEvents).length, 0);
const interleaved = interleavedParser.finish();
assert.deepEqual(interleaved.message.content.map(block => block.input.value), ['a', 'b']);

const invalidText = responseEvents(prepared);
const auxiliary = responseEvents(prepared);
auxiliary.splice(1, 0, { type: 'codex.response.metadata', response_id: 'resp_protocol',
  metadata: { synthetic: 'SYNTHETIC_PRIVATE_METADATA' }, sequence_number: 1 });
assert.deepEqual(nativeResponse(auxiliary, prepared), nativeResponse(responseEvents(prepared), prepared));
for (const metadataEvent of [
  { type: 'codex.response.metadata', metadata: null },
  { type: 'codex.response.metadata', metadata: {}, sequence_number: -1 },
  { type: 'codex.response.metadata', metadata: {}, response_id: 'wrong' },
  { type: 'codex.response.metadata', metadata: {}, output: [] },
  { type: 'codex.response.metadata', metadata: {}, error: {} }
]) {
  const invalid = responseEvents(prepared); invalid.splice(1, 0, metadataEvent);
  assert.throws(() => nativeResponse(invalid, prepared));
}
const prematureMetadata = responseEvents(prepared); prematureMetadata.unshift(auxiliary[1]);
assert.throws(() => nativeResponse(prematureMetadata, prepared), /MISSING_RESPONSE_START/);
const lateMetadata = responseEvents(prepared); lateMetadata.push(auxiliary[1]);
assert.throws(() => nativeResponse(lateMetadata, prepared), /EVENT_AFTER_COMPLETION/);
for (const [type, expected] of [['response.output_text.annotation.added', 'response.output_text.annotation.added'],
  ...['ping', 'rate_limits.updated', 'codex.rate_limits'].map(type => [type, type]),
  ['SYNTHETIC_PRIVATE_EVENT_VALUE', 'other']]) {
  const parser = createNativeResponse(prepared);
  parser.push({ type: 'response.created', response: { id: 'resp_diagnostic', status: 'in_progress' } });
  assert.throws(() => parser.push({ type, payload: 'SYNTHETIC_PRIVATE_BODY' }), error =>
    error.code === 'UNSUPPORTED_EVENT' && error.eventKind === expected && !JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
}
for (const [event, expected] of [[null, 'invalid-event-object'], [[], 'invalid-event-object'],
  [{ payload: 'SYNTHETIC_PRIVATE' }, 'missing-event-type'], [{ type: 7 }, 'invalid-event-type'],
  [{ type: 'response.SYNTHETIC_PRIVATE' }, 'unknown-response-event']]) {
  const parser = createNativeResponse(prepared);
  parser.push({ type: 'response.created', response: { id: 'resp_diagnostic', status: 'in_progress' } });
  assert.throws(() => parser.push(event), error => error.code === 'UNSUPPORTED_EVENT'
    && error.eventKind === expected && !JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
  assert.throws(() => parser.finish(), error => error.code === 'UNSUPPORTED_EVENT');
}
invalidText.find(event => event.type === 'response.output_text.done').text = undefined;
assert.throws(() => nativeResponse(invalidText, prepared));
const invalidAnnotations = responseEvents(prepared, { annotations: [{ type: 'url_citation' }] });
assert.throws(() => nativeResponse(invalidAnnotations, prepared));
const invalidSnapshot = responseEvents(prepared);
invalidSnapshot.find(event => event.type === 'response.output_item.done' && event.item.type === 'message').item.extra = true;
assert.throws(() => nativeResponse(invalidSnapshot, prepared));

const doneReasoning = { type: 'reasoning', id: 'rs_merge', status: 'completed',
  summary: [{ type: 'summary_text', text: 'SYNTHETIC_SUMMARY' }], encrypted_content: 'SYNTHETIC_DONE' };
const mergedReasoning = mergeReasoning(doneReasoning, { type: 'reasoning', id: doneReasoning.id, encrypted_content: 'SYNTHETIC_FINAL' });
assert.equal(mergedReasoning.summary[0].text, 'SYNTHETIC_SUMMARY');

process.stdout.write(JSON.stringify({ suite: 'native-protocol', passed: true, externalRequests: 0, credentialReads: 0 }) + '\n');
