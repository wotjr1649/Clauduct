// Fixed public wire stimuli. This module has no credential or external transport.
export const SERVICE_FAULTS = Object.freeze(['flapping-503', 'error-200', 'truncated', 'invalid-utf8', 'sequence-gap']);
export const SERVICE_MARKER = 'PUBLIC_SERVICE_RECOVERY_COMPLETE';
export const frame = event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`;

export function publicServiceEvents(model, effort, serial, name, input = {}) {
  if (!['gpt-5.6-luna', 'gpt-5.6-sol'].includes(model) || !['max', 'low'].includes(effort)
    || !Number.isSafeInteger(serial) || serial < 1 || serial > 32
    || (name !== null && !['ToolSearch', 'mcp__fixture__apply_effect', 'mcp__fixture__effect_status',
      'mcp__fixture__complete_report'].includes(name))) throw new Error('SERVICE_STIMULUS_INVALID');
  const responseId = `resp_public_${serial}`, itemId = `item_public_${serial}`;
  const item = name ? { type: 'function_call', id: itemId, call_id: `call_public_${serial}`, name,
    arguments: JSON.stringify(input), status: 'completed' }
    : { type: 'message', id: itemId, role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: SERVICE_MARKER, annotations: [] }] };
  return [{ type: 'response.created', response: { id: responseId, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0,
      item: name ? { ...item, status: 'in_progress', arguments: '' } : { ...item, status: 'in_progress', content: [] } },
    ...(name ? [
      { type: 'response.function_call_arguments.delta', output_index: 0, item_id: itemId, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: itemId, arguments: item.arguments }
    ] : [
      { type: 'response.output_text.delta', output_index: 0, item_id: itemId, content_index: 0, delta: SERVICE_MARKER },
      { type: 'response.output_text.done', output_index: 0, item_id: itemId, content_index: 0, text: SERVICE_MARKER }
    ]),
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: responseId, status: 'completed', model,
      reasoning: { effort }, output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
}

export function serviceFailureWire(kind) {
  if (!SERVICE_FAULTS.includes(kind) || kind === 'flapping-503') throw new Error('SERVICE_STIMULUS_INVALID');
  const text = 'PUBLIC_REPORT_PENDING';
  const message = { type: 'message', id: 'item_public_partial', role: 'assistant', status: 'completed',
    content: [{ type: 'output_text', text, annotations: [] }] };
  const tool = { type: 'function_call', id: 'item_public_uncommitted', call_id: 'call_public_uncommitted',
    name: 'mcp__fixture__complete_report', arguments: '{}', status: 'completed' };
  const events = [
    { type: 'response.created', response: { id: 'resp_public_partial', status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...message, status: 'in_progress', content: [] } },
    { type: 'response.output_text.delta', output_index: 0, item_id: message.id, content_index: 0, delta: text },
    { type: 'response.output_text.done', output_index: 0, item_id: message.id, content_index: 0, text },
    { type: 'response.output_item.done', output_index: 0, item: message },
    { type: 'response.output_item.added', output_index: 1, item: { ...tool, status: 'in_progress', arguments: '' } },
    { type: 'response.function_call_arguments.delta', output_index: 1, item_id: tool.id, delta: '{}' },
    { type: 'response.function_call_arguments.done', output_index: 1, item_id: tool.id, arguments: '{}' },
    { type: 'response.output_item.done', output_index: 1, item: tool }
  ];
  const sequenced = kind === 'sequence-gap';
  const prefix = Buffer.from(events.map((event, index) => frame(sequenced ? { ...event, sequence_number: index } : event)).join(''));
  const tail = kind === 'error-200' ? Buffer.from(frame({ type: 'error', error: { code: 'server_error', type: 'server_error' } }))
    : kind === 'invalid-utf8' ? Buffer.from([0xff])
    : kind === 'sequence-gap' ? Buffer.from(frame({ type: 'response.in_progress', sequence_number: events.length + 1 }))
    : Buffer.alloc(0);
  return { prefix, tail, destroy: kind === 'truncated' };
}
