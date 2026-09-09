import { isDeepStrictEqual } from 'node:util';
import { win32, posix } from 'node:path';
import { fileURLToPath } from 'node:url';
import { selectModel } from './models.mjs';
import { reasoningSnapshot, reasoningEvent, mergeReasoning } from '../poc/adapter.mjs';
import { inspectCompactTemplate } from './compact-policy.mjs';

export const NATIVE_LIMITS = Object.freeze({ requestBytes: 32 * 1024 * 1024, responseBytes: 16 * 1024 * 1024 });
export const OUTPUT_TOKEN_LIMIT_POLICY = 'usage-enforced-completion';
// Diagnostic allowlist only, not a list of newly supported events. Never echo an
// arbitrary upstream type or body through an error or status response.
export const EVENT_DIAGNOSTIC_TYPES = Object.freeze(['other', 'invalid-event-object', 'missing-event-type', 'invalid-event-type',
  'unknown-response-event', 'ping', 'rate_limits.updated', 'codex.rate_limits', 'response.created', 'response.in_progress', 'response.queued',
  'response.completed', 'response.failed', 'response.incomplete', 'error',
  'response.output_item.added', 'response.output_item.done', 'response.content_part.added', 'response.content_part.done',
  'response.output_text.delta', 'response.output_text.done', 'response.output_text.annotation.added',
  'response.refusal.delta', 'response.refusal.done', 'response.function_call_arguments.delta', 'response.function_call_arguments.done',
  'response.reasoning_summary_part.added', 'response.reasoning_summary_part.done', 'response.reasoning_part.added', 'response.reasoning_part.done',
  'response.reasoning_summary_text.delta', 'response.reasoning_summary_text.done', 'response.reasoning_text.delta', 'response.reasoning_text.done',
  'response.custom_tool_call_input.delta', 'response.custom_tool_call_input.done']);
export class NativeError extends Error { constructor(code) { super(code); this.code = code; } }
export const need = (condition, code = 'UNSUPPORTED_REQUEST') => { if (!condition) throw new NativeError(code); };
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const id = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
const reviewPath = value => typeof value !== 'string' ? null : /^[A-Za-z]:[\\/]/.test(value)
  ? win32.normalize(value).toLowerCase() : value.startsWith('/') ? posix.normalize(value) : null;
const shellQuote = value => "'" + value.replaceAll("'", "'\\''") + "'";
export const fileReviewCommand = target => [process.execPath.replaceAll('\\', '/'),
  fileURLToPath(new URL('./review-diff.mjs', import.meta.url)).replaceAll('\\', '/'), target].map(shellQuote).join(' ');

// Only a verified code-review fork receives this policy, never a prompt-name guess.
export function prepareFileReview(prepared, doc) {
  if (prepared.purpose === 'compact-template') return;
  const firstUser = doc.messages.find(message => message.role === 'user');
  const blocks = typeof firstUser?.content === 'string' ? [{ type: 'text', text: firstUser.content }] : firstUser?.content ?? [];
  const target = blocks.filter(block => block.type === 'text').map(block => /^Review target: `([^`\r\n]+)`/.exec(block.text)?.[1]).find(Boolean);
  if (!reviewPath(target) || !/\.[A-Za-z0-9]+$/.test(target)) return;
  const command = fileReviewCommand(target);
  const calls = new Set(); let ready = false;
  for (const message of doc.messages) for (const block of Array.isArray(message.content) ? message.content : []) {
    if (message.role === 'assistant' && block.type === 'tool_use' && block.name === 'Bash'
      && block.input?.command === command) calls.add(block.id);
    if (message.role === 'user' && block.type === 'tool_result' && calls.has(block.tool_use_id)) {
      need(!block.is_error, 'REVIEW_DIFF_FAILED');
      let result;
      const raw = typeof block.content === 'string' ? block.content
        : Array.isArray(block.content) ? block.content.filter(part => part.type === 'text').map(part => part.text).join('\n') : '';
      try { result = JSON.parse(raw); } catch { throw new NativeError('REVIEW_DIFF_FAILED'); }
      need(object(result) && result.diagnosticVersion === 1 && ['tracked-working-tree', 'untracked-added'].includes(result.kind)
        && reviewPath(result.target) === reviewPath(target) && typeof result.diff === 'string', 'REVIEW_DIFF_FAILED');
      ready = true;
    }
  }
  prepared.body.input.push({ role: 'developer', content: `Use the verified diff helper result as the review diff. untracked-added means the target is reviewed as a new file against an empty baseline; it does not require HEAD or main. Preserve the supplied review level: low reviews these hunks directly without full-file reads, other searches, or subagents. Other levels follow their own review body. Do not re-run diff collection after a successful result or invoke code-review again. First obtain the diff using this exact Bash command: ${command}` });
  if (ready) return;
  need(prepared.names.has('Bash') && prepared.body.tool_choice === 'auto', 'REVIEW_DIFF_UNAVAILABLE');
  prepared.body.tool_choice = { type: 'function', name: 'Bash' };
  prepared.body.parallel_tool_calls = false;
  // Constrain generation as well as checking it: the first call has no free-form shell arguments.
  prepared.body.tools = prepared.body.tools.map(tool => tool.name !== 'Bash' ? tool : {
    ...tool, strict: true, parameters: { type: 'object', properties: {
      command: { type: 'string', enum: [command] }
    }, required: ['command'], additionalProperties: false }
  });
  prepared.requiredReviewDiff = command;
}

export function verifyFileReviewStep(message, prepared) {
  if (!prepared.requiredReviewDiff) return;
  const calls = message.content.filter(block => block.type === 'tool_use');
  const mismatch = calls.length !== 1 ? 'call-count' : calls[0].name !== 'Bash' ? 'tool-name'
    : calls[0].input?.command !== prepared.requiredReviewDiff ? 'command'
    : calls[0].input.run_in_background === true ? 'background' : null;
  if (mismatch) {
    const error = new NativeError('REVIEW_DIFF_REQUIRED');
    error.reviewDiffMismatch = mismatch;
    throw error;
  }
}
function keys(value, allowed) { need(object(value) && Object.keys(value).every(key => allowed.includes(key))); }
function cache(value) {
  if (value === undefined) return;
  keys(value, ['type', 'ttl', 'scope']);
  need(value.type === 'ephemeral' && (value.ttl === undefined || ['5m', '1h'].includes(value.ttl)));
}
function text(value) {
  const blocks = typeof value === 'string' ? [{ type: 'text', text: value }] : value;
  need(Array.isArray(blocks));
  return blocks.map(block => {
    keys(block, ['type', 'text', 'cache_control']); cache(block.cache_control);
    need(block.type === 'text' && typeof block.text === 'string'); return block.text;
  }).join('\n');
}
function image(block) {
  keys(block, ['type', 'source', 'cache_control']); cache(block.cache_control);
  const source = block.source;
  keys(source, ['type', 'media_type', 'data']);
  need(source.type === 'base64' && ['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(source.media_type)
    && typeof source.data === 'string' && /^[A-Za-z0-9+/]*={0,2}$/.test(source.data), 'UNSUPPORTED_IMAGE');
  return { type: 'input_image', image_url: `data:${source.media_type};base64,${source.data}` };
}
// Opaque Codex reasoning travels in the native transcript, never a gateway disk cache.
// It is replay data, not instructions or a credential; backend validates encrypted_content.
const reasoningPrefix = 'clauduct-reasoning-v1:';
function decodeReasoning(value) {
  need(typeof value === 'string' && value.startsWith(reasoningPrefix), 'UNSUPPORTED_THINKING');
  let result;
  try { result = JSON.parse(Buffer.from(value.slice(reasoningPrefix.length), 'base64url').toString('utf8')); }
  catch { throw new NativeError('UNSUPPORTED_THINKING'); }
  keys(result, ['type', 'id', 'summary', 'encrypted_content']);
  need(result.type === 'reasoning' && id(result.id) && typeof result.encrypted_content === 'string'
    && Array.isArray(result.summary), 'UNSUPPORTED_THINKING');
  for (const part of result.summary) need(part.type === 'summary_text' && typeof part.text === 'string', 'UNSUPPORTED_THINKING');
  return result;
}
export function prepareNative(doc, { subagent = false, route, turnToolChanges = false } = {}) {
  keys(doc, ['model', 'messages', 'system', 'max_tokens', 'stream', 'tools', 'tool_choice', 'thinking',
    'metadata', 'output_config', 'context_management', 'temperature', 'top_p', 'stop_sequences']);
  need(doc.stream === true && Array.isArray(doc.messages) && doc.messages.length > 0);
  let selected = route ?? selectModel(doc.model, subagent ? undefined : doc.output_config?.effort);
  if (doc.output_config !== undefined) keys(doc.output_config, ['effort']);
  need(Number.isSafeInteger(doc.max_tokens) && doc.max_tokens > 0, 'INVALID_OUTPUT_LIMIT');
  if (doc.thinking !== undefined) {
    keys(doc.thinking, ['type', 'display', 'budget_tokens']);
    need(['adaptive', 'enabled', 'disabled'].includes(doc.thinking.type));
    need(doc.thinking.budget_tokens === undefined || (Number.isSafeInteger(doc.thinking.budget_tokens) && doc.thinking.budget_tokens > 0));
  }
  if (doc.context_management !== undefined) {
    keys(doc.context_management, ['edits']);
    // Only a semantic no-op is consumed; native local compaction arrives as a new transcript.
    need(isDeepStrictEqual(doc.context_management.edits, [{ type: 'clear_thinking_20251015', keep: 'all' }]), 'UNSUPPORTED_CONTEXT_EDIT');
  }
  need(doc.temperature === undefined && doc.top_p === undefined && doc.stop_sequences === undefined, 'UNSUPPORTED_SAMPLING');
  const definitions = new Map(), discovered = new Set(), removed = new Set();
  need(doc.tools === undefined || Array.isArray(doc.tools));
  for (const tool of doc.tools ?? []) {
    keys(tool, ['name', 'description', 'input_schema', 'cache_control', 'defer_loading']); cache(tool.cache_control);
    need(id(tool.name) && !definitions.has(tool.name) && object(tool.input_schema) && tool.input_schema.type === 'object', 'UNSUPPORTED_TOOLS');
    need(tool.description === undefined || typeof tool.description === 'string', 'UNSUPPORTED_TOOLS');
    need(tool.defer_loading === undefined || typeof tool.defer_loading === 'boolean', 'UNSUPPORTED_TOOLS');
    definitions.set(tool.name, tool);
  }
  let toolChoice = definitions.size ? 'auto' : 'none', parallel = true;
  if (doc.tool_choice !== undefined) {
    keys(doc.tool_choice, ['type', 'name', 'disable_parallel_tool_use']);
    need(['auto', 'any', 'none', 'tool'].includes(doc.tool_choice.type), 'UNSUPPORTED_TOOLS');
    need(doc.tool_choice.disable_parallel_tool_use === undefined
      || typeof doc.tool_choice.disable_parallel_tool_use === 'boolean', 'UNSUPPORTED_TOOLS');
    parallel = doc.tool_choice.disable_parallel_tool_use !== true;
    toolChoice = doc.tool_choice.type === 'any' ? 'required' : doc.tool_choice.type;
    if (toolChoice === 'tool') {
      need(definitions.has(doc.tool_choice.name), 'UNSUPPORTED_TOOLS');
      discovered.add(doc.tool_choice.name);
      toolChoice = { type: 'function', name: doc.tool_choice.name };
    }
  }
  const input = [], pending = new Set(), used = new Set();
  if (doc.system !== undefined) input.push({ role: 'developer', content: text(doc.system) });
  for (const message of doc.messages) {
    keys(message, ['role', 'content', 'output_config']);
    if (message.output_config !== undefined) {
      need(message.role === 'system'); keys(message.output_config, ['effort']);
      const turn = selectModel(selected.model, message.output_config.effort);
      if (!subagent && !route) selected = turn;
    }
    need(['user', 'assistant', 'system'].includes(message.role), 'UNSUPPORTED_MESSAGES');
    const blocks = typeof message.content === 'string' ? [{ type: 'text', text: message.content }] : message.content;
    need(Array.isArray(blocks), 'UNSUPPORTED_MESSAGES');
    for (const block of blocks) {
      cache(block.cache_control);
      if (block.type === 'text') {
        keys(block, ['type', 'text', 'cache_control']); need(typeof block.text === 'string');
        input.push({ role: message.role === 'system' ? 'developer' : message.role,
          content: [{ type: message.role === 'assistant' ? 'output_text' : 'input_text', text: block.text }] });
      } else if (block.type === 'tool_addition' || block.type === 'tool_removal') {
        need(turnToolChanges && message.role === 'system', 'UNSUPPORTED_TOOL_CHANGE');
        keys(block, ['type', 'tool', 'cache_control']);
        keys(block.tool, ['type', 'name']);
        need(block.tool.type === 'tool_reference' && id(block.tool.name)
          && definitions.has(block.tool.name), 'INVALID_TOOL_REFERENCE');
        if (block.type === 'tool_addition') {
          discovered.add(block.tool.name); removed.delete(block.tool.name);
        } else removed.add(block.tool.name);
      } else if (block.type === 'image') {
        need(message.role === 'user'); input.push({ role: 'user', content: [image(block)] });
      } else if (block.type === 'tool_use') {
        keys(block, ['type', 'id', 'name', 'input', 'cache_control']);
        need(message.role === 'assistant' && id(block.id) && id(block.name) && object(block.input)
          && definitions.has(block.name) && !used.has(block.id), 'INVALID_TOOL_CALL');
        discovered.add(block.name);
        pending.add(block.id); used.add(block.id);
        input.push({ type: 'function_call', call_id: block.id, name: block.name, arguments: JSON.stringify(block.input) });
      } else if (block.type === 'tool_result') {
        keys(block, ['type', 'tool_use_id', 'content', 'is_error', 'cache_control']);
        need(message.role === 'user' && id(block.tool_use_id) && pending.delete(block.tool_use_id)
          && (block.is_error === undefined || typeof block.is_error === 'boolean'), 'INVALID_TOOL_RESULT');
        const parts = typeof block.content === 'string' ? [{ type: 'text', text: block.content }] : block.content ?? [];
        need(Array.isArray(parts));
        const content = parts.map(part => {
          if (part.type === 'image') return image(part);
          if (part.type === 'tool_reference') {
            keys(part, ['type', 'tool_name', 'cache_control']); cache(part.cache_control);
            need(id(part.tool_name) && definitions.has(part.tool_name), 'INVALID_TOOL_REFERENCE');
            discovered.add(part.tool_name);
            // Claude supplies the available definitions in tools; retain the historical reference as data.
            return { type: 'input_text', text: JSON.stringify({ type: 'tool_reference', tool_name: part.tool_name }) };
          }
          return { type: 'input_text', text: text([part]) };
        });
        if (block.is_error) content.unshift({ type: 'input_text', text: 'Tool execution failed:' });
        input.push({ type: 'function_call_output', call_id: block.tool_use_id, output: content });
      } else if (block.type === 'redacted_thinking') {
        keys(block, ['type', 'data', 'cache_control']); need(message.role === 'assistant');
        input.push(decodeReasoning(block.data));
      } else throw new NativeError('UNSUPPORTED_CONTENT');
    }
  }
  need(pending.size === 0, 'MISSING_TOOL_RESULT');
  const deferred = [...definitions.values()].filter(tool => tool.defer_loading === true);
  const discoveryTool = definitions.get('ToolSearch');
  const discoveryVisible = discoveryTool !== undefined
    && (discoveryTool.defer_loading !== true || discovered.has(discoveryTool.name));
  need(turnToolChanges || !deferred.some(tool => !discovered.has(tool.name))
    || discoveryVisible, 'UNSUPPORTED_DEFERRED_TOOLS');
  const tools = [...definitions.values()].filter(tool => !removed.has(tool.name)
    && (tool.defer_loading !== true || discovered.has(tool.name)))
    .map(tool => ({ type: 'function', name: tool.name, description: tool.description ?? '', parameters: tool.input_schema, strict: false }));
  const names = new Set(tools.map(tool => tool.name));
  need(typeof toolChoice !== 'object' || names.has(toolChoice.name), 'UNSUPPORTED_TOOLS');
  need(toolChoice !== 'required' || names.size > 0, 'UNSUPPORTED_TOOLS');
  if (names.size === 0) toolChoice = 'none';
  const compactShape = inspectCompactTemplate(doc.messages);
  const purpose = compactShape.matches ? 'compact-template' : 'conversation';
  const requestedEffort = selected.effort;
  if (purpose === 'compact-template' && !['low', 'medium'].includes(selected.effort)) {
    selected = { ...selected, effort: 'medium' };
  }
  return { selected, names, purpose, compactShape, requestedEffort, outputLimit: doc.max_tokens, outputTokenLimitPolicy: OUTPUT_TOKEN_LIMIT_POLICY,
    body: { model: selected.model,
      instructions: 'Follow the developer instructions in the conversation.', input, tools, tool_choice: toolChoice,
      parallel_tool_calls: parallel, reasoning: { effort: selected.effort },
      include: ['reasoning.encrypted_content'], stream: true, store: false } };
}

export function nativeUsage(value) {
  need(object(value) && ['input_tokens', 'output_tokens', 'total_tokens'].every(key => Number.isSafeInteger(value[key]) && value[key] >= 0)
    && value.total_tokens === value.input_tokens + value.output_tokens, 'INVALID_USAGE');
  const cached = value.input_tokens_details === undefined ? 0 : value.input_tokens_details?.cached_tokens;
  need(Number.isSafeInteger(cached) && cached >= 0 && cached <= value.input_tokens, 'INVALID_USAGE');
  return { input_tokens: value.input_tokens - cached, output_tokens: value.output_tokens,
    cache_read_input_tokens: cached, cache_creation_input_tokens: 0 };
}

const MAX_ITEM_BYTES = Math.floor(NATIVE_LIMITS.responseBytes / 2);
const MAX_OUTPUT_PARTS = Math.floor(MAX_ITEM_BYTES / 32);
function encodedSize(value) {
  let json;
  try { json = JSON.stringify(value); } catch { throw new NativeError('RESPONSE_TOO_LARGE'); }
  return Buffer.byteLength(json ?? '');
}
function bounded(value, limit = NATIVE_LIMITS.responseBytes) {
  need(encodedSize(value) <= limit, 'RESPONSE_TOO_LARGE');
  return value;
}
function emptyArrayOrMissing(value) { return value === undefined || value === null || (Array.isArray(value) && value.length === 0); }
function outputTextPart(value) {
  keys(value, ['type', 'text', 'annotations', 'logprobs']);
  need(value.type === 'output_text' && typeof value.text === 'string', 'UNSUPPORTED_CONTENT');
  // These fields are not represented by Anthropic text blocks. Reject data rather than dropping it.
  need(emptyArrayOrMissing(value.annotations) && emptyArrayOrMissing(value.logprobs), 'UNSUPPORTED_CONTENT');
  return value;
}
function messageSnapshot(value, starting = false) {
  keys(value, ['id', 'type', 'status', 'role', 'content', 'phase']);
  need(id(value.id) && value.type === 'message' && value.role === 'assistant', 'INVALID_OUTPUT_ITEM');
  need(value.status === undefined || value.status === (starting ? 'in_progress' : 'completed'), 'SNAPSHOT_MISMATCH');
  need(Array.isArray(value.content) && value.content.length <= MAX_OUTPUT_PARTS, 'INVALID_OUTPUT_ITEM');
  if (starting) need(value.content.length === 0, 'INVALID_OUTPUT_ITEM');
  else value.content.forEach(outputTextPart);
  need(value.phase === undefined || typeof value.phase === 'string', 'INVALID_OUTPUT_ITEM');
  bounded(value, MAX_ITEM_BYTES);
  return value;
}
function functionSnapshot(value, starting = false) {
  keys(value, ['id', 'type', 'status', 'name', 'call_id', 'arguments']);
  need(id(value.id) && value.type === 'function_call' && id(value.name) && id(value.call_id)
    && typeof value.arguments === 'string', 'INVALID_OUTPUT_ITEM');
  need(value.status === undefined || value.status === (starting ? 'in_progress' : 'completed'), 'SNAPSHOT_MISMATCH');
  if (starting) need(value.arguments === '', 'INVALID_OUTPUT_ITEM');
  bounded(value, MAX_ITEM_BYTES);
  return value;
}

function frameStart(responseId, prepared, usage) {
  return { type: 'message_start', message: { id: responseId, type: 'message', role: 'assistant',
    model: prepared.selected.model, content: [], stop_reason: null, stop_sequence: null,
    usage: { ...usage, output_tokens: 0 } } };
}
const ADAPTER_ERROR_CODES = new Set(['ITEM_INDEX_MISMATCH', 'SNAPSHOT_MISMATCH', 'STREAM_ORDER', 'TEXT_MISMATCH',
  'UNSUPPORTED_CONTENT', 'UNSUPPORTED_EVENT', 'UNSUPPORTED_FIELDS', 'UNSUPPORTED_OUTPUT']);

// Validate streamed item snapshots while retaining only bounded item state, and expose text deltas early.
export function createNativeResponse(prepared) {
  const state = { responseId: undefined, completed: undefined, inProgress: false,
    items: new Map(), ids: new Set(), responseBytes: 0, nextBlockIndex: 0,
    messageStart: undefined, failed: undefined, finished: false };
  const fail = error => {
    const code = error instanceof NativeError ? error.code
      : ADAPTER_ERROR_CODES.has(error?.code) ? error.code : 'INVALID_REQUEST';
    state.failed ??= new NativeError(code);
    throw state.failed;
  };
  const charge = (item, bytes) => {
    need(Number.isSafeInteger(bytes) && bytes >= 0
      && state.responseBytes + bytes <= NATIVE_LIMITS.responseBytes
      && item.bytes + bytes <= MAX_ITEM_BYTES, 'RESPONSE_TOO_LARGE');
    state.responseBytes += bytes; item.bytes += bytes;
  };
  const active = index => {
    need(Number.isSafeInteger(index) && index >= 0, 'STREAM_ORDER');
    const item = state.items.get(index);
    need(item && !item.done, 'STREAM_ORDER');
    return item;
  };
  const startMessage = frames => {
    if (state.messageStart) return;
    state.messageStart = frameStart(state.responseId, prepared,
      { input_tokens: 0, output_tokens: 0, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 });
    frames.push(state.messageStart);
  };
  const textDelta = (event, item) => {
    need(item.kind === 'message' && id(event.item_id) && event.item_id === item.first.id
      && Number.isSafeInteger(event.content_index) && event.content_index >= 0 && typeof event.delta === 'string', 'STREAM_ORDER');
    let part = item.textParts[event.content_index], first = false;
    if (!part) {
      need(event.content_index === item.textParts.length, 'STREAM_ORDER');
      need(item.textParts.length < MAX_OUTPUT_PARTS, 'RESPONSE_TOO_LARGE');
      part = { text: '', done: false, deltas: 0, index: state.nextBlockIndex++ };
      item.textParts.push(part); first = true;
    }
    need(!part.done, 'STREAM_ORDER');
    charge(item, Buffer.byteLength(event.delta));
    part.text += event.delta; part.deltas++;
    const frames = [];
    startMessage(frames);
    if (first) frames.push({ type: 'content_block_start', index: part.index, content_block: { type: 'text', text: '' } });
    frames.push({ type: 'content_block_delta', index: part.index, delta: { type: 'text_delta', text: event.delta } });
    return frames;
  };
  const textDone = (event, item) => {
    need(item.kind === 'message' && id(event.item_id) && event.item_id === item.first.id
      && Number.isSafeInteger(event.content_index) && event.content_index >= 0 && typeof event.text === 'string', 'STREAM_ORDER');
    const part = item.textParts[event.content_index];
    need(part && !part.done && part.text === event.text, 'SNAPSHOT_MISMATCH');
    part.done = true;
    return [{ type: 'content_block_stop', index: part.index }];
  };
  const contentPart = (event, item) => {
    need(item.kind === 'message' && id(event.item_id) && event.item_id === item.first.id
      && Number.isSafeInteger(event.content_index) && event.content_index >= 0, 'STREAM_ORDER');
    if (event.type === 'response.content_part.added') {
      need(event.content_index === item.textParts.length && object(event.part), 'STREAM_ORDER');
      outputTextPart(event.part);
    } else {
      const part = item.textParts[event.content_index];
      need(part?.done === true && object(event.part) && outputTextPart(event.part).text === part.text, 'SNAPSHOT_MISMATCH');
    }
  };
  const addItem = event => {
    need(Number.isSafeInteger(event.output_index) && event.output_index >= 0
      && event.output_index === state.items.size && object(event.item), 'INVALID_OUTPUT_ITEM');
    const first = event.item;
    need(!state.ids.has(first.id), 'INVALID_OUTPUT_ITEM');
    let kind;
    if (first.type === 'message') { messageSnapshot(first, true); kind = 'message'; }
    else if (first.type === 'function_call') { functionSnapshot(first, true); kind = 'function_call'; }
    else if (first.type === 'reasoning') { reasoningSnapshot(first, true); kind = 'reasoning'; }
    else throw new NativeError('UNSUPPORTED_OUTPUT');
    const item = { index: event.output_index, first, kind, final: undefined, done: false, bytes: encodedSize(first),
      textParts: [], arguments: '', argumentsDone: false, reasoning: kind === 'reasoning'
        ? { item: first, summary: [], content: [] } : undefined, blockIndex: undefined };
    need(item.bytes <= MAX_ITEM_BYTES, 'RESPONSE_TOO_LARGE');
    state.ids.add(first.id); state.items.set(item.index, item);
  };
  const itemDone = event => {
    const item = active(event.output_index), final = event.item;
    need(object(final) && final.id === item.first.id && final.type === item.first.type, 'SNAPSHOT_MISMATCH');
    if (item.kind === 'function_call') {
      functionSnapshot(final);
      need(item.argumentsDone && final.arguments === item.arguments && final.name === item.first.name
        && final.call_id === item.first.call_id, 'SNAPSHOT_MISMATCH');
    } else if (item.kind === 'message') {
      messageSnapshot(final);
      need(final.content.length === item.textParts.length, 'SNAPSHOT_MISMATCH');
      final.content.forEach((part, index) => {
        const streamed = item.textParts[index];
        need(streamed && streamed.done && part.text === streamed.text, 'SNAPSHOT_MISMATCH');
      });
    } else {
      need(final.type === 'reasoning', 'UNSUPPORTED_OUTPUT');
      reasoningEvent(item.reasoning, event, event.output_index);
      bounded(final, MAX_ITEM_BYTES);
    }
    item.final = final; item.done = true;
  };
  const completedEvent = event => { state.completed = event.response; };
  const parse = event => {
    need(object(event) && typeof event.type === 'string', 'UNSUPPORTED_EVENT');
    need(!state.completed, 'EVENT_AFTER_COMPLETION');
    if (event.type === 'response.created') {
      need(state.responseId === undefined && id(event.response?.id)
        && (event.response.status === undefined || event.response.status === 'in_progress'), 'INVALID_RESPONSE_START');
      need(event.response_id === undefined || event.response_id === event.response.id, 'SNAPSHOT_MISMATCH');
      state.responseId = event.response.id; return [];
    }
    need(state.responseId, 'MISSING_RESPONSE_START');
    need(event.response_id === undefined || event.response_id === state.responseId, 'SNAPSHOT_MISMATCH');
    if (event.type === 'response.completed') { need(object(event.response), 'INCOMPLETE_RESPONSE'); completedEvent(event); return []; }
    if (event.type === 'response.failed' || event.type === 'response.incomplete' || event.type === 'error') {
      state.completed = object(event.response) ? event.response : { id: state.responseId, status: 'failed', error: true };
      return [];
    }
    if (event.type === 'response.in_progress') {
      need(!state.inProgress && state.items.size === 0
        && (event.response === undefined || (event.response.id === state.responseId
          && (event.response.status === undefined || event.response.status === 'in_progress'))), 'STREAM_ORDER');
      state.inProgress = true; return [];
    }
    if (event.type === 'response.output_item.added') { addItem(event); return []; }
    if (event.type === 'response.output_text.delta') return textDelta(event, active(event.output_index));
    if (event.type === 'response.output_text.done') return textDone(event, active(event.output_index));
    if (event.type === 'response.function_call_arguments.delta') {
      const item = active(event.output_index);
      need(item.kind === 'function_call' && event.item_id === item.first.id && typeof event.delta === 'string'
        && !item.argumentsDone, 'STREAM_ORDER');
      charge(item, Buffer.byteLength(event.delta)); item.arguments += event.delta; return [];
    }
    if (event.type === 'response.function_call_arguments.done') {
      const item = active(event.output_index);
      need(item.kind === 'function_call' && event.item_id === item.first.id && !item.argumentsDone
        && typeof event.arguments === 'string' && event.arguments === item.arguments
        && (event.name === undefined || event.name === item.first.name), 'SNAPSHOT_MISMATCH');
      item.argumentsDone = true; return [];
    }
    if (event.type === 'response.output_item.done') { itemDone(event); return []; }
    if (event.type === 'response.content_part.added' || event.type === 'response.content_part.done') {
      contentPart(event, active(event.output_index)); return [];
    }
    if (event.type.startsWith('response.reasoning')) {
      const item = active(event.output_index);
      need(item.kind === 'reasoning', 'STREAM_ORDER');
      if (event.type.endsWith('.delta')) {
        need(typeof event.delta === 'string' && item.reasoning.summary.length <= MAX_OUTPUT_PARTS
          && item.reasoning.content.length <= MAX_OUTPUT_PARTS, 'RESPONSE_TOO_LARGE');
        charge(item, Buffer.byteLength(event.delta));
      }
      reasoningEvent(item.reasoning, event, event.output_index); return [];
    }
    throw new NativeError('UNSUPPORTED_EVENT');
  };
  const finish = () => {
    if (state.failed) throw state.failed;
    try {
      need(!state.finished, 'INVALID_STATE');
      state.finished = true;
      need(state.responseId && object(state.completed) && state.completed.id === state.responseId
        && state.completed.status === 'completed' && !state.completed.error && !state.completed.incomplete_details,
      'INCOMPLETE_RESPONSE');
      need(Array.isArray(state.completed.output) && state.items.size > 0
        && [...state.items.values()].every(item => item.done), 'INCOMPLETE_RESPONSE');
      bounded(state.completed.output);
      need(state.completed.model === prepared.selected.model
        && (state.completed.reasoning === undefined || (object(state.completed.reasoning)
          && (state.completed.reasoning.effort === undefined || state.completed.reasoning.effort === prepared.selected.effort))),
      'MODEL_EFFORT_MISMATCH');
      const states = [...state.items.values()].sort((a, b) => a.index - b.index);
      const snapshots = states.map((item, index) => { need(item.index === index, 'STREAM_ORDER'); return item.final; });
      let output = snapshots;
      if (state.completed.output.length) {
        need(state.completed.output.length === snapshots.length, 'SNAPSHOT_MISMATCH');
        output = state.completed.output.map((item, index) => {
          if (item.type === 'reasoning') {
            need(snapshots[index]?.type === 'reasoning', 'SNAPSHOT_MISMATCH');
            bounded(item, MAX_ITEM_BYTES);
            const merged = mergeReasoning(snapshots[index], item);
            bounded(merged, MAX_ITEM_BYTES);
            states[index].final = merged;
            return merged;
          }
          if (item.type === 'message') messageSnapshot(item);
          else if (item.type === 'function_call') functionSnapshot(item);
          else throw new NativeError('UNSUPPORTED_OUTPUT');
          need(isDeepStrictEqual(item, snapshots[index]), 'SNAPSHOT_MISMATCH'); return item;
        });
      }
      const textBlocks = [], reasoningBlocks = [], toolBlocks = [], calls = new Set();
      output.forEach((item, index) => {
        const stateItem = states[index];
        if (item.type === 'reasoning') {
          if (typeof item.encrypted_content === 'string') {
            const saved = { type: 'reasoning', id: item.id, summary: item.summary ?? [], encrypted_content: item.encrypted_content };
            const data = reasoningPrefix + Buffer.from(JSON.stringify(saved)).toString('base64url');
            decodeReasoning(data);
            const block = { type: 'redacted_thinking', data };
            reasoningBlocks.push({ block, stateItem });
          } else {
            need(!item.content?.length && !item.summary?.length, 'MISSING_ENCRYPTED_REASONING');
          }
        } else if (item.type === 'function_call') {
          need(prepared.names.has(item.name) && id(item.call_id) && !calls.has(item.call_id), 'UNSUPPORTED_TOOL_CALL');
          let input;
          try { input = JSON.parse(item.arguments); } catch { throw new NativeError('INVALID_TOOL_CALL'); }
          need(object(input), 'INVALID_TOOL_CALL'); bounded(input, MAX_ITEM_BYTES); calls.add(item.call_id);
          const block = { type: 'tool_use', id: item.call_id, name: item.name, input };
          toolBlocks.push({ block, stateItem });
        } else {
          need(item.type === 'message' && item.role === 'assistant' && Array.isArray(item.content), 'UNSUPPORTED_OUTPUT');
          item.content.forEach((part, partIndex) => {
            outputTextPart(part);
            const streamed = stateItem.textParts[partIndex];
            need(streamed && streamed.done && streamed.text === part.text, 'STREAM_ORDER');
            textBlocks.push({ block: { type: 'text', text: part.text }, streamed });
          });
        }
      });
      const deferred = [], content = [];
      textBlocks.sort((a, b) => a.streamed.index - b.streamed.index);
      for (const { block, streamed } of textBlocks) {
        const index = streamed.index;
        need(index === content.length, 'STREAM_ORDER');
        content.push(block);
      }
      for (const entry of reasoningBlocks) {
        entry.stateItem.blockIndex = content.length;
        content.push(entry.block); deferred.push({ index: entry.stateItem.blockIndex, block: entry.block });
      }
      for (const entry of toolBlocks) {
        entry.stateItem.blockIndex = content.length;
        content.push(entry.block); deferred.push({ index: entry.stateItem.blockIndex, block: entry.block });
      }
      need(content.length > 0, 'EMPTY_REPLY');
      const usage = nativeUsage(state.completed.usage);
      need(usage.output_tokens <= prepared.outputLimit, 'OUTPUT_TOKEN_LIMIT_EXCEEDED');
      const message = { id: state.responseId, type: 'message', role: 'assistant', model: prepared.selected.model,
        content, stop_reason: calls.size ? 'tool_use' : 'end_turn', stop_sequence: null, usage };
      const frames = [];
      startMessage(frames);
      state.messageStart.message = { ...message, content: [], stop_reason: null, usage: { ...usage, output_tokens: 0 } };
      for (const { index, block } of deferred) {
        frames.push({ type: 'content_block_start', index, content_block: block.type === 'tool_use' ? { ...block, input: {} } : block });
        if (block.type === 'tool_use') frames.push({ type: 'content_block_delta', index,
          delta: { type: 'input_json_delta', partial_json: JSON.stringify(block.input) } });
        frames.push({ type: 'content_block_stop', index });
      }
      frames.push({ type: 'message_delta', delta: { stop_reason: message.stop_reason, stop_sequence: null },
        usage }, { type: 'message_stop' });
      return { message, frames };
    } catch (error) { return fail(error); }
  };
  const push = event => {
    if (state.failed) throw state.failed;
    if (state.finished) throw new NativeError('INVALID_STATE');
    try { return parse(event); } catch (error) {
      try { return fail(error); } catch (failure) {
        if (failure.code === 'UNSUPPORTED_EVENT') {
          failure.eventKind = !object(event) ? 'invalid-event-object'
            : !Object.hasOwn(event, 'type') ? 'missing-event-type'
            : typeof event.type !== 'string' ? 'invalid-event-type'
            : EVENT_DIAGNOSTIC_TYPES.includes(event.type) ? event.type
            : event.type.startsWith('response.') ? 'unknown-response-event' : 'other';
        }
        throw failure;
      }
    }
  };
  return { push, finish };
}

export function nativeResponse(events, prepared) {
  const parser = createNativeResponse(prepared), frames = [];
  for (const event of events) frames.push(...parser.push(event));
  const result = parser.finish(); frames.push(...result.frames);
  return { message: result.message,
    sse: frames.map(frame => `event: ${frame.type}\ndata: ${JSON.stringify(frame)}\n\n`).join('') };
}
