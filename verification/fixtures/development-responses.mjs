// Reviewed public replacement used only by local native integration checks.
// Live development starts from BASELINE_SOURCE and receives no replacement.
export const PUBLIC_DEVELOPMENT_SOURCE = `export function parseRetryAfterSeconds(value) {
  if (typeof value !== 'string' || value.length > 128 || /[\\r\\n]/.test(value) || !/^[ \\t]*[0-9]+[ \\t]*$/.test(value)) return null;
  const milliseconds = Number(value) * 1000;
  return Number.isSafeInteger(milliseconds) ? milliseconds : null;
}
`;
export function publicDevelopmentEvents(model, effort, serial) {
  if (!['gpt-5.6-luna', 'gpt-5.6-sol'].includes(model) || effort !== (model.endsWith('luna') ? 'max' : 'low')
    || !Number.isSafeInteger(serial) || serial < 1 || serial > 5) throw new Error('DEVELOPMENT_STIMULUS_INVALID');
  const name = ['mcp__fixture__read_task', 'mcp__fixture__run_tests', 'mcp__fixture__write_source', 'mcp__fixture__run_tests', null][serial - 1];
  const marker = 'CLAUDUCT_DEVELOPMENT_DONE', responseId = `resp_public_${serial}`, itemId = `item_public_${serial}`;
  const item = name ? { type: 'function_call', id: itemId, call_id: `call_public_${serial}`, name,
    arguments: JSON.stringify(serial === 3 ? { code: PUBLIC_DEVELOPMENT_SOURCE } : {}), status: 'completed' }
    : { type: 'message', id: itemId, role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: marker, annotations: [] }] };
  return [{ type: 'response.created', response: { id: responseId, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0,
      item: name ? { ...item, status: 'in_progress', arguments: '' } : { ...item, status: 'in_progress', content: [] } },
    ...(name ? [
      { type: 'response.function_call_arguments.delta', output_index: 0, item_id: itemId, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: itemId, arguments: item.arguments }
    ] : [
      { type: 'response.output_text.delta', output_index: 0, item_id: itemId, content_index: 0, delta: marker },
      { type: 'response.output_text.done', output_index: 0, item_id: itemId, content_index: 0, text: marker }
    ]), { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: responseId, status: 'completed', model, reasoning: { effort },
      output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
}
