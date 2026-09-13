import { developmentTask, DEFAULT_DEVELOPMENT_TASK_ID } from '../development-tasks.mjs';

// Reviewed public replacement used only by local native integration checks.
// Live development starts from BASELINE_SOURCE and receives no replacement.
export const PUBLIC_DEVELOPMENT_SOURCE = `export function parseRetryAfterSeconds(value) {
  if (typeof value !== 'string' || value.length > 128 || /[\\r\\n]/.test(value) || !/^[ \\t]*[0-9]+[ \\t]*$/.test(value)) return null;
  const milliseconds = Number(value) * 1000;
  return Number.isSafeInteger(milliseconds) ? milliseconds : null;
}
`;
const windowSource = `export function retryDelayWithinBudget(value) {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null;
  if (!Number.isSafeInteger(value.nowMs) || value.nowMs < 0 || !Number.isSafeInteger(value.retryAtMs) || value.retryAtMs < 0 || !Number.isSafeInteger(value.deadlineMs) || value.deadlineMs < 0) return null;
  if (value.deadlineMs <= value.nowMs || value.retryAtMs >= value.deadlineMs) return null;
  return value.retryAtMs <= value.nowMs ? 0 : value.retryAtMs - value.nowMs;
}
`;
export function publicDevelopmentSource(taskId = DEFAULT_DEVELOPMENT_TASK_ID) {
  developmentTask(taskId);
  return taskId === DEFAULT_DEVELOPMENT_TASK_ID ? PUBLIC_DEVELOPMENT_SOURCE : windowSource;
}
export function publicDevelopmentEvents(model, effort, serial, finish = false, taskId = DEFAULT_DEVELOPMENT_TASK_ID) {
  const source = publicDevelopmentSource(taskId);
  if (!['gpt-5.6-luna', 'gpt-5.6-sol'].includes(model) || effort !== (model.endsWith('luna') ? 'max' : 'low')
    || typeof finish !== 'boolean' || !Number.isSafeInteger(serial) || serial < 1 || serial > (finish ? 3 : 5)) throw new Error('DEVELOPMENT_STIMULUS_INVALID');
  const name = (finish ? ['mcp__fixture__read_task', 'mcp__fixture__run_tests', null]
    : ['mcp__fixture__read_task', 'mcp__fixture__run_tests', 'mcp__fixture__write_source', 'mcp__fixture__run_tests', null])[serial - 1];
  const suffix = (taskId === DEFAULT_DEVELOPMENT_TASK_ID ? '' : 'window_') + (finish ? `finish_${serial}` : String(serial));
  const marker = 'CLAUDUCT_DEVELOPMENT_DONE', responseId = `resp_public_${suffix}`, itemId = `item_public_${suffix}`;
  const item = name ? { type: 'function_call', id: itemId, call_id: `call_public_${suffix}`, name,
    arguments: JSON.stringify(!finish && serial === 3 ? { code: source } : {}), status: 'completed' }
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
