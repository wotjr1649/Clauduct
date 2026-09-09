import { isDeepStrictEqual } from 'node:util';

export const ENDPOINT = 'https://chatgpt.com/backend-api/codex/responses';
export const MODEL = 'gpt-6-astra';
export const EFFORT = 'xhigh';
const profiles = Object.freeze({
  'astra-xhigh': Object.freeze({ model: MODEL, effort: EFFORT }),
  'astra-low': Object.freeze({ model: 'gpt-6-astra', effort: 'low' }),
  'luna-low': Object.freeze({ model: 'gpt-5.6-luna', effort: 'low' })
});
export function probeProfile(name = 'astra-xhigh') {
  requireThat(typeof name === 'string' && Object.hasOwn(profiles, name), 'INVALID_PROFILE');
  return profiles[name];
}
export const ALIAS = 'clauduct-poc';
export const FIXTURE_PATH = 'D:\\AIDEV\\Clauduct\\poc\\fixture.txt';
export const LIMITS = Object.freeze({ requestBytes: 65536, responseBytes: 262144,
  argumentBytes: 8192, events: 512, timeoutMs: 45000, requests: 2 });
export const CHAT_REQUESTS = 32;

// Closed diagnostic vocabulary shared with both HTTP error boundaries. Never emit arbitrary messages.
export const PROTOCOL_ERROR_CODES = Object.freeze([
  'NO_TOOL_CALL', 'READ_RESULT_MISMATCH', 'TOOL_EXECUTION_DENIED', 'FINAL_MARKER_MISMATCH', 'OUTPUT_TOKEN_LIMIT_EXCEEDED',
  'UPSTREAM_TOKEN_LIMIT_REJECTED',
  'ARGUMENTS_TOO_LARGE', 'CANCELLED', 'DISCONNECTED', 'DUPLICATE_RESPONSE_ID',
  'EMPTY_CONTENT_TYPE', 'EMPTY_REPLY', 'ENDPOINT_MISMATCH', 'EVENT_AFTER_COMPLETION',
  'FORBIDDEN', 'HISTORY_MISMATCH', 'HTTP_ERROR', 'INCOMPLETE_RESPONSE',
  'INPUT_TOO_LARGE', 'INVALID_HEADER_METADATA', 'INVALID_JSON', 'INVALID_POLICY',
  'INVALID_PROFILE', 'INVALID_PROTOCOL', 'INVALID_RESPONSE_START', 'INVALID_SSE', 'INVALID_STATE',
  'INVALID_TIMEOUT', 'INVALID_TOOL_RESULT', 'INVALID_USAGE', 'INVALID_UTF8', 'ITEM_INDEX_MISMATCH',
  'MISSING_CONTENT_TYPE', 'MISSING_RESPONSE_START', 'MISSING_TOOL_SNAPSHOT', 'MODEL_EFFORT_MISMATCH',
  'RATE_LIMITED', 'REDIRECT_REJECTED', 'REFUSAL', 'REQUEST_BUDGET',
  'RESPONSE_TOO_LARGE', 'SEQUENCE_MISMATCH', 'SNAPSHOT_MISMATCH', 'STREAM_ORDER',
  'TEXT_MISMATCH', 'TIMEOUT', 'TOOL_PATH_REJECTED', 'TOO_MANY_EVENTS',
  'TRUNCATED_STREAM', 'UNAUTHENTICATED', 'UNSUPPORTED_CACHED_USAGE', 'UNSUPPORTED_CONTENT',
  'UNSUPPORTED_CONTENT_TYPE', 'UNSUPPORTED_EVENT', 'UNSUPPORTED_FIELDS', 'UNSUPPORTED_MAX_TOKENS',
  'UNSUPPORTED_MESSAGE_PHASE', 'UNSUPPORTED_MESSAGES', 'UNSUPPORTED_OUTPUT', 'UNSUPPORTED_REQUEST', 'UNSUPPORTED_SSE_FIELD',
  'UNSUPPORTED_TOOLS', 'UNSUPPORTED_TOOL_CALL', 'UPSTREAM_FAILURE'
]);
const phases = ['not-started', 'request-validation', 'headers', 'body-decoding', 'sse-framing', 'response-validation', 'completion'];
const eventKinds = ['not-observed', 'created', 'in-progress', 'item-added', 'item-done', 'completed',
  'failed', 'incomplete', 'error', 'refusal', 'reasoning', 'tool-delta', 'tool-done',
  'text-delta', 'text-done', 'part-added', 'part-done', 'other'];
const itemKinds = ['not-observed', 'message', 'function_call', 'reasoning', 'other'];
const itemStatuses = ['not-observed', 'not-provided', 'in_progress', 'completed', 'incomplete', 'failed', 'other'];
const messageKeys = ['id', 'type', 'status', 'role', 'content'];
const messagePhases = ['not-observed', 'not-provided', 'null', 'commentary', 'final_answer', 'other'];
const messageExtraFields = ['not-observed', 'none', 'phase-only', 'other-only', 'phase-and-other'];
const snapshotChecks = ['RESPONSE_ID', 'REASONING_ID_FORMAT', 'REASONING_TYPE', 'REASONING_STATUS',
  'REASONING_ITEM_ID', 'REASONING_SUMMARY_COUNT', 'REASONING_SUMMARY_STREAM',
  'REASONING_CONTENT_COUNT', 'REASONING_CONTENT_STREAM', 'REASONING_INITIAL_ENCRYPTED',
  'REASONING_FINAL_ID', 'REASONING_FINAL_SUMMARY', 'REASONING_FINAL_CONTENT', 'REASONING_FINAL_ENCRYPTED',
  'PAYLOAD_ID_TYPE_STATUS', 'TOOL_NAME_CALL_ARGUMENTS', 'ARGUMENTS_DONE_NAME'];
const diagnosticIndex = number => Number.isSafeInteger(number) && Math.abs(number) <= 1000000 ? number : null;
const eventNames = Object.freeze({
  'response.created': 'created', 'response.in_progress': 'in-progress',
  'response.output_item.added': 'item-added', 'response.output_item.done': 'item-done',
  'response.completed': 'completed', 'response.failed': 'failed', 'response.incomplete': 'incomplete', error: 'error',
  'response.function_call_arguments.delta': 'tool-delta', 'response.function_call_arguments.done': 'tool-done',
  'response.output_text.delta': 'text-delta', 'response.output_text.done': 'text-done',
  'response.content_part.added': 'part-added', 'response.content_part.done': 'part-done'
});
function noteEvent(event, number, observation) {
  const type = typeof event?.type === 'string' ? event.type : '';
  observation.eventNumber = Math.min(number, LIMITS.events + 1);
  observation.eventKind = Object.hasOwn(eventNames, type) ? eventNames[type]
    : type.startsWith('response.reasoning') ? 'reasoning' : type.includes('refusal') ? 'refusal' : 'other';
  observation.itemKind = event?.item === undefined ? 'not-observed'
    : ['message', 'function_call', 'reasoning'].includes(event.item?.type) ? event.item.type : 'other';
  observation.itemStatus = event?.item === undefined ? 'not-observed' : event.item?.status === undefined ? 'not-provided'
    : ['in_progress', 'completed', 'incomplete', 'failed'].includes(event.item.status) ? event.item.status : 'other';
  observation.sequenceNumber = diagnosticIndex(event?.sequence_number);
  observation.outputIndex = diagnosticIndex(event?.output_index);
}
export function protocolDiagnostics(value) {
  const count = number => Number.isSafeInteger(number) && number >= 0 && number <= LIMITS.events + 1 ? number : 0;
  return { phase: phases.includes(value?.phase) ? value.phase : 'not-started',
    category: PROTOCOL_ERROR_CODES.includes(value?.category) ? value.category : value?.category === 'NONE' ? 'NONE' : 'UNKNOWN',
    headersAccepted: value?.headersAccepted === true, headerCompatibilityUsed: value?.compatibilityApplied === true,
    parsedEvents: count(value?.parsedEvents), eventNumber: count(value?.eventNumber),
    eventKind: eventKinds.includes(value?.eventKind) ? value.eventKind : 'not-observed',
    itemKind: itemKinds.includes(value?.itemKind) ? value.itemKind : 'not-observed',
    itemStatus: itemStatuses.includes(value?.itemStatus) ? value.itemStatus : 'not-observed',
    messagePhase: messagePhases.includes(value?.messagePhase) ? value.messagePhase : 'not-observed',
    messageExtraFields: messageExtraFields.includes(value?.messageExtraFields) ? value.messageExtraFields : 'not-observed',
    reasoningItemCount: count(value?.reasoningItemCount),
    reasoningEncryptedUpdates: count(value?.reasoningEncryptedUpdates),
    snapshotCheck: snapshotChecks.includes(value?.snapshotCheck) ? value.snapshotCheck : 'not-observed',
    sequenceNumber: diagnosticIndex(value?.sequenceNumber), outputIndex: diagnosticIndex(value?.outputIndex) };
}


// This schema describes a synthetic fixture, not Claude Code's installed Read schema.
export function readTool() {
  return { name: 'Read', description: 'Read the single synthetic PoC fixture.',
    input_schema: { type: 'object', properties: { file_path: { type: 'string', enum: [FIXTURE_PATH] } },
      required: ['file_path'], additionalProperties: false } };
}

class ProtocolError extends Error {
  constructor(code, snapshotCheck) { super(code); this.code = code; this.snapshotCheck = snapshotCheck; }
}
function requireThat(ok, code, snapshotCheck) { if (!ok) throw new ProtocolError(code, snapshotCheck); }
function object(value) { return value !== null && typeof value === 'object' && !Array.isArray(value); }
function keys(value, allowed) {
  requireThat(object(value) && Object.keys(value).every(k => allowed.includes(k)), 'UNSUPPORTED_FIELDS');
}
function checkMessageFields(item, observation) {
  if (object(item) && item.type === 'message') {
    const hasPhase = Object.hasOwn(item, 'phase');
    const hasOther = Object.keys(item).some(key => key !== 'phase' && !messageKeys.includes(key));
    // Observe only the message currently being validated, never later parsed items or raw field names.
    observation.messagePhase = !hasPhase ? 'not-provided' : item.phase === null ? 'null'
      : ['commentary', 'final_answer'].includes(item.phase) ? item.phase : 'other';
    observation.messageExtraFields = hasPhase ? hasOther ? 'phase-and-other' : 'phase-only'
      : hasOther ? 'other-only' : 'none';
  }
  keys(item, [...messageKeys, 'phase']);
  // A single final message is supported; commentary needs a multi-message lifecycle.
  requireThat(!Object.hasOwn(item, 'phase') || item.phase === null || item.phase === 'final_answer',
    'UNSUPPORTED_MESSAGE_PHASE');
}
function parse(raw, limit = LIMITS.requestBytes) {
  requireThat(typeof raw === 'string', 'INVALID_JSON');
  requireThat(Buffer.byteLength(raw) <= limit, 'INPUT_TOO_LARGE');
  try { return JSON.parse(raw); } catch { throw new ProtocolError('INVALID_JSON'); }
}
function id(value) { return typeof value === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value); }
function cacheHint(value) {
  if (value === undefined) return;
  keys(value, ['type', 'ttl']);
  requireThat(value.type === 'ephemeral' && (value.ttl === undefined || ['5m', '1h'].includes(value.ttl)),
    'UNSUPPORTED_CONTENT');
}
function textParts(value, type, claude = false) {
  const parts = typeof value === 'string' ? [{ type: 'text', text: value }] : value;
  requireThat(Array.isArray(parts) && parts.length > 0, 'UNSUPPORTED_CONTENT');
  return parts.map(part => {
    keys(part, claude ? ['type', 'text', 'cache_control'] : ['type', 'text']);
    if (claude) cacheHint(part.cache_control);
    requireThat(part.type === 'text' && typeof part.text === 'string', 'UNSUPPORTED_CONTENT');
    return { type, text: part.text };
  });
}
// Claude moves cache breakpoints and serializes uncached single text blocks as strings.
// Compare validated text/roles, never raw cache metadata; do not trim or rewrite text.
function claudeTextMessage(message) {
  keys(message, ['role', 'content']);
  requireThat(['user', 'system'].includes(message.role), 'UNSUPPORTED_MESSAGES');
  return { role: message.role, content: textParts(message.content, 'text', true) };
}
function claudeOptions(doc) {
  const { messages, system, tools, ...options } = doc;
  return { ...options, system: textParts(system, 'text', true), tools: tools.map(tool => {
    const { cache_control, ...definition } = tool;
    cacheHint(cache_control);
    return definition;
  }) };
}
// Explicit offline bridge policy: no cache emulation or client identity forwarding.
// Only the full, fixed fixture Read is exposed upstream, never optional range/PDF reads.
function claudeRequest(doc, profile, chat = false) {
  keys(doc, ['model', 'stream', 'max_tokens', 'system', 'messages', 'tools', 'tool_choice',
    'thinking', 'metadata', 'output_config', 'context_management']);
  requireThat(doc.model === (chat ? profile.model : ALIAS) && doc.stream === true, 'UNSUPPORTED_REQUEST');
  requireThat(Number.isSafeInteger(doc.max_tokens) && doc.max_tokens > 0 && doc.max_tokens <= 64000,
    'UNSUPPORTED_MAX_TOKENS');
  if (!chat || doc.thinking !== undefined) {
    keys(doc.thinking, ['type', 'display']);
    requireThat(doc.thinking.type === 'adaptive' && (doc.thinking.display === 'omitted'
      || (chat && doc.thinking.display === undefined)), 'UNSUPPORTED_REQUEST');
  }
  if (!chat || doc.output_config !== undefined) {
    keys(doc.output_config, ['effort']);
    requireThat(doc.output_config.effort === profile.effort, 'MODEL_EFFORT_MISMATCH');
  }
  if (!chat || doc.metadata !== undefined) {
    keys(doc.metadata, ['user_id']);
    requireThat(typeof doc.metadata.user_id === 'string' && doc.metadata.user_id.length <= 512, 'UNSUPPORTED_REQUEST');
  }
  requireThat((chat && doc.context_management === undefined) || isDeepStrictEqual(doc.context_management,
    { edits: [{ type: 'clear_thinking_20251015', keep: 'all' }] }), 'UNSUPPORTED_REQUEST');
  requireThat(doc.tool_choice === undefined || isDeepStrictEqual(doc.tool_choice, { type: 'auto' }), 'UNSUPPORTED_TOOLS');
  if (chat) {
    requireThat(doc.tools === undefined || (Array.isArray(doc.tools) && doc.tools.length === 0), 'UNSUPPORTED_TOOLS');
    requireThat(Array.isArray(doc.messages) && doc.messages.length > 0 && doc.messages.length <= 128, 'UNSUPPORTED_MESSAGES');
    return doc;
  }
  requireThat(Array.isArray(doc.tools) && doc.tools.length === 1, 'UNSUPPORTED_TOOLS');
  const tool = doc.tools[0];
  keys(tool, ['name', 'description', 'input_schema', 'cache_control']);
  requireThat(tool.name === 'Read' && typeof tool.description === 'string', 'UNSUPPORTED_TOOLS');
  cacheHint(tool.cache_control);
  const schema = tool.input_schema;
  keys(schema, ['type', 'properties', 'required', 'additionalProperties', '$schema']);
  requireThat(schema.type === 'object' && schema.additionalProperties === false
    && isDeepStrictEqual(schema.required, ['file_path']), 'UNSUPPORTED_TOOLS');
  requireThat(schema.$schema === undefined || ['http://json-schema.org/draft-07/schema#',
    'https://json-schema.org/draft/2020-12/schema'].includes(schema.$schema), 'UNSUPPORTED_TOOLS');
  keys(schema.properties, ['file_path', 'offset', 'limit', 'pages']);
  const expected = { file_path: { type: 'string' }, pages: { type: 'string' },
    offset: { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER },
    limit: { type: 'integer', exclusiveMinimum: 0, maximum: Number.MAX_SAFE_INTEGER } };
  for (const [name, contract] of Object.entries(expected)) {
    const property = schema.properties[name];
    keys(property, [...Object.keys(contract), 'description']);
    const { description, ...shape } = property;
    requireThat((description === undefined || typeof description === 'string')
      && isDeepStrictEqual(shape, contract), 'UNSUPPORTED_TOOLS');
  }
  requireThat(Array.isArray(doc.messages) && [2, 4, 5].includes(doc.messages.length), 'UNSUPPORTED_MESSAGES');
  return doc;
}
function request(raw, claude = false, profile, observe, chat = false) {
  const doc = parse(raw);
  observe?.(doc);
  if (claude) return claudeRequest(doc, profile, chat);
  keys(doc, ['model', 'stream', 'max_tokens', 'system', 'messages', 'tools', 'tool_choice']);
  requireThat(doc.model === ALIAS && doc.stream === true, 'UNSUPPORTED_REQUEST');
  requireThat(doc.max_tokens === undefined || (Number.isSafeInteger(doc.max_tokens) && doc.max_tokens > 0 && doc.max_tokens <= 4096),
    'UNSUPPORTED_MAX_TOKENS');
  requireThat(isDeepStrictEqual(doc.tools, [readTool()])
    && isDeepStrictEqual(doc.tool_choice, { type: 'auto' }), 'UNSUPPORTED_TOOLS');
  requireThat(Array.isArray(doc.messages) && doc.messages.length > 0, 'UNSUPPORTED_MESSAGES');
  return doc;
}
function body(doc, input, following, profile) {
  return { model: profile.model, instructions: 'Follow the developer instructions in the conversation.', input, reasoning: { effort: profile.effort },
    tools: [{ type: 'function', name: 'Read', description: readTool().description,
      parameters: readTool().input_schema, strict: true }],
    tool_choice: following ? 'none' : 'auto', parallel_tool_calls: false,
    ...(doc.max_tokens === undefined ? {} : { max_output_tokens: doc.max_tokens }), stream: true, store: false };
}

function checkHeaders(meta, policy) {
  keys(meta, ['endpoint', 'httpStatus', 'contentTypePresent', 'contentType']);
  requireThat(meta.endpoint === ENDPOINT, 'ENDPOINT_MISMATCH');
  requireThat(meta.httpStatus === 200, meta.httpStatus === 401 ? 'UNAUTHENTICATED'
    : meta.httpStatus === 403 ? 'FORBIDDEN' : meta.httpStatus === 429 ? 'RATE_LIMITED'
      : meta.httpStatus >= 300 && meta.httpStatus < 400 ? 'REDIRECT_REJECTED' : 'HTTP_ERROR');
  requireThat(typeof meta.contentTypePresent === 'boolean' && typeof meta.contentType === 'string',
    'INVALID_HEADER_METADATA');
  if (!meta.contentTypePresent) {
    requireThat(meta.contentType === '', 'INVALID_HEADER_METADATA');
    requireThat(policy === 'codex-missing-content-type', 'MISSING_CONTENT_TYPE');
    return true;
  }
  requireThat(meta.contentType.trim() !== '', 'EMPTY_CONTENT_TYPE');
  requireThat(meta.contentType.split(';', 1)[0].trim().toLowerCase() === 'text/event-stream'
    && !/[,\x00-\x1f\x7f]/.test(meta.contentType), 'UNSUPPORTED_CONTENT_TYPE');
  return false;
}

function eventsFrom(text, observation) {
  const normalized = text.replace(/\r\n/g, '\n');
  requireThat(!normalized.includes('\r') && normalized.endsWith('\n\n'), 'TRUNCATED_STREAM');
  const events = [];
  let sentinel = false;
  for (const frame of normalized.split('\n\n')) {
    if (!frame) continue;
    const data = [];
    let event;
    for (const line of frame.split('\n')) {
      if (line.startsWith(':')) continue;
      const match = /^(data|event): ?(.*)$/.exec(line);
      requireThat(match, 'UNSUPPORTED_SSE_FIELD');
      if (match[1] === 'data') data.push(match[2]);
      else { requireThat(event === undefined, 'INVALID_SSE'); event = match[2]; }
    }
    if (!data.length) { requireThat(event === undefined, 'INVALID_SSE'); continue; }
    requireThat(!sentinel, 'EVENT_AFTER_COMPLETION');
    if (data.join('\n') === '[DONE]') {
      requireThat(events.at(-1)?.type === 'response.completed' && event === undefined, 'INCOMPLETE_RESPONSE');
      sentinel = true;
      continue;
    }
    const value = parse(data.join('\n'), LIMITS.responseBytes);
    observation.parsedEvents = Math.min(events.length + 1, LIMITS.events + 1);
    noteEvent(value, events.length + 1, observation);
    requireThat(object(value) && typeof value.type === 'string'
      && (event === undefined || event === value.type), 'INVALID_SSE');
    requireThat(events.length < LIMITS.events, 'TOO_MANY_EVENTS');
    requireThat(value.sequence_number === undefined || value.sequence_number === events.length,
      'SEQUENCE_MISMATCH');
    events.push(value);
  }
  return events;
}
function usage(value) {
  requireThat(object(value) && ['input_tokens', 'output_tokens', 'total_tokens']
    .every(k => Number.isSafeInteger(value[k]) && value[k] >= 0)
    && value.total_tokens === value.input_tokens + value.output_tokens, 'INVALID_USAGE');
  const details = value.input_tokens_details;
  const cached = details === undefined ? 0 : details?.cached_tokens;
  requireThat((details === undefined || object(details)) && Number.isSafeInteger(cached)
    && cached >= 0 && cached <= value.input_tokens, 'INVALID_USAGE');
  // OpenAI input includes cache reads; Anthropic reports them separately. Preserve the total.
  return { input_tokens: value.input_tokens - cached, output_tokens: value.output_tokens,
    ...(details === undefined ? {} : { cache_read_input_tokens: cached, cache_creation_input_tokens: 0 }) };
}
export function reasoningSnapshot(item, starting = false) {
  keys(item, ['id', 'type', 'status', 'summary', 'content', 'encrypted_content']);
  requireThat(id(item.id), 'SNAPSHOT_MISMATCH', 'REASONING_ID_FORMAT');
  requireThat(item.type === 'reasoning', 'SNAPSHOT_MISMATCH', 'REASONING_TYPE');
  requireThat(item.status === undefined || item.status === (starting ? 'in_progress' : 'completed'),
    'SNAPSHOT_MISMATCH', 'REASONING_STATUS');
  for (const [field, type] of [['summary', 'summary_text'], ['content', 'reasoning_text']]) {
    if (item[field] === undefined) continue;
    requireThat(Array.isArray(item[field]) && (!starting || item[field].length === 0), 'UNSUPPORTED_CONTENT');
    for (const part of item[field]) {
      keys(part, ['type', 'text']);
      requireThat(part.type === type && typeof part.text === 'string', 'UNSUPPORTED_CONTENT');
    }
  }
  requireThat(item.encrypted_content == null || typeof item.encrypted_content === 'string', 'UNSUPPORTED_CONTENT');
}
export function mergeReasoning(done, final) {
  reasoningSnapshot(final);
  requireThat(final.id === done.id, 'SNAPSHOT_MISMATCH', 'REASONING_FINAL_ID');
  const merged = { ...done, ...final };
  // A present final opaque value belongs to the final snapshot; never infer its plaintext identity.
  // Only an omitted stream field may fall back to the verified item.done value.
  for (const field of ['summary', 'content']) {
    const finalHasField = Object.hasOwn(final, field) && final[field] !== undefined;
    if (done[field] != null && finalHasField && final[field] != null) {
      requireThat(isDeepStrictEqual(done[field], final[field]), 'SNAPSHOT_MISMATCH',
        field === 'summary' ? 'REASONING_FINAL_SUMMARY' : 'REASONING_FINAL_CONTENT');
    }
    if (!finalHasField && done[field] != null) merged[field] = done[field];
  }
  return merged;
}
function noteEncryptedUpdate(before, after, observation) {
  if (typeof before.encrypted_content === 'string' && typeof after.encrypted_content === 'string'
    && before.encrypted_content !== after.encrypted_content) {
    observation.reasoningEncryptedUpdates = (observation.reasoningEncryptedUpdates ?? 0) + 1;
  }
}
export function reasoningEvent(state, event, index) {
  requireThat(event.output_index === index, 'STREAM_ORDER');
  if (event.type === 'response.output_item.done') {
    reasoningSnapshot(event.item);
    requireThat(event.item.id === state.item.id, 'SNAPSHOT_MISMATCH', 'REASONING_ITEM_ID');
    for (const field of ['summary', 'content']) {
      const parts = state[field];
      if (parts.length) {
        requireThat(event.item[field]?.length === parts.length, 'SNAPSHOT_MISMATCH',
          field === 'summary' ? 'REASONING_SUMMARY_COUNT' : 'REASONING_CONTENT_COUNT');
        parts.forEach((part, i) => requireThat(part.done && (!part.started || part.stopped)
          && event.item[field][i].text === part.text, 'SNAPSHOT_MISMATCH',
          field === 'summary' ? 'REASONING_SUMMARY_STREAM' : 'REASONING_CONTENT_STREAM'));
      }
    }
    // item.added is provisional. Only a completed snapshot may supply replayable opaque content.
    return event.item;
  }
  requireThat(event.item_id === state.item.id, 'ITEM_INDEX_MISMATCH');
  const partEvent = /^response\.reasoning_summary_part\.(added|done)$/.exec(event.type);
  const textEvent = /^response\.reasoning_(summary_text|text)\.(delta|done)$/.exec(event.type);
  requireThat(partEvent || textEvent, 'UNSUPPORTED_EVENT');
  const summary = !!partEvent || textEvent[1] === 'summary_text';
  const parts = state[summary ? 'summary' : 'content'];
  const position = summary ? event.summary_index : event.content_index;
  requireThat(Number.isSafeInteger(position) && position >= 0 && position <= parts.length, 'ITEM_INDEX_MISMATCH');
  if (position === parts.length) {
    const prior = parts.at(-1);
    requireThat(!prior || (prior.done && (!prior.started || prior.stopped)), 'STREAM_ORDER');
    parts.push({ text: '', deltas: 0, done: false, started: false, stopped: false });
  }
  const part = parts[position];
  if (partEvent) {
    keys(event.part, ['type', 'text']);
    const starting = partEvent[1] === 'added';
    requireThat(event.part.type === 'summary_text' && event.part.text === (starting ? '' : part.text), 'TEXT_MISMATCH');
    requireThat(starting ? !part.started && !part.deltas && !part.done : part.started && part.done && !part.stopped, 'STREAM_ORDER');
    if (starting) part.started = true; else part.stopped = true;
  } else {
    requireThat(!part.done, 'STREAM_ORDER');
    if (textEvent[2] === 'delta') {
      requireThat(typeof event.delta === 'string', 'TEXT_MISMATCH');
      part.text += event.delta; part.deltas++;
    } else {
      requireThat(typeof event.text === 'string' && event.text === part.text, 'TEXT_MISMATCH');
      part.done = true;
    }
  }
}
function validateResponse(events, following, observation, profile) {
  let responseId, added, itemDone, doneText, completed, text = '', deltas = 0;
  let partAdded = false, partDone = false, inProgress = false;
  const reasoning = [];
  let activeReasoning;
  function snapshot(item) {
    requireThat(object(item) && item.id === added.id && item.type === added.type
      && (item.status === undefined || item.status === 'completed'), 'SNAPSHOT_MISMATCH', 'PAYLOAD_ID_TYPE_STATUS');
    if (added.type === 'function_call') {
      keys(item, ['id', 'type', 'status', 'name', 'call_id', 'arguments']);
      requireThat(item.name === added.name && item.call_id === added.call_id
        && item.arguments === text, 'SNAPSHOT_MISMATCH', 'TOOL_NAME_CALL_ARGUMENTS');
    } else {
      checkMessageFields(item, observation);
      requireThat(item.role === 'assistant' && Array.isArray(item.content) && item.content.length === 1,
        'UNSUPPORTED_CONTENT');
      part(item.content[0], text);
    }
  }
  function part(value, expected) {
    keys(value, ['type', 'text', 'annotations', 'logprobs']);
    requireThat(value.type === 'output_text' && value.text === expected
      && (!value.annotations || isDeepStrictEqual(value.annotations, []))
      && (!value.logprobs || isDeepStrictEqual(value.logprobs, [])), 'TEXT_MISMATCH');
  }
  let eventNumber = 0;
  for (const e of events) {
    noteEvent(e, ++eventNumber, observation);
    requireThat(!completed, 'EVENT_AFTER_COMPLETION');
    requireThat(!['error', 'response.failed', 'response.incomplete'].includes(e.type), 'UPSTREAM_FAILURE');
    requireThat(!e.type.includes('refusal'), 'REFUSAL');
    if (e.type === 'response.created') {
      requireThat(responseId === undefined && !added && id(e.response?.id)
        && e.response.status === 'in_progress' && !e.response.error
        && (e.response_id === undefined || e.response_id === e.response.id), 'INVALID_RESPONSE_START');
      responseId = e.response.id;
      continue;
    }
    requireThat(responseId !== undefined, 'MISSING_RESPONSE_START');
    requireThat(e.response_id === undefined || e.response_id === responseId, 'SNAPSHOT_MISMATCH', 'RESPONSE_ID');
    if (e.type === 'response.in_progress') {
      requireThat(!inProgress && !added && !activeReasoning && !reasoning.length && e.response?.id === responseId
        && e.response.status === 'in_progress' && !e.response.error, 'STREAM_ORDER');
      inProgress = true;
    } else if (e.type === 'response.output_item.added') {
      requireThat(!added && !activeReasoning && e.output_index === reasoning.length && id(e.item?.id)
        && !reasoning.some(item => item.id === e.item.id)
        && (e.item.status === undefined || e.item.status === 'in_progress'), 'UNSUPPORTED_OUTPUT');
      if (e.item.type === 'reasoning') {
        reasoningSnapshot(e.item, true);
        activeReasoning = { item: e.item, summary: [], content: [] };
        continue;
      }
      added = e.item;
      if (added.type === 'function_call') {
        keys(added, ['id', 'type', 'status', 'name', 'call_id', 'arguments']);
        requireThat(!following && added.name === 'Read' && id(added.call_id)
          && added.arguments === '', 'UNSUPPORTED_TOOL_CALL');
      } else {
        checkMessageFields(added, observation);
        requireThat(added.type === 'message' && added.role === 'assistant'
          && isDeepStrictEqual(added.content, []), 'UNSUPPORTED_OUTPUT');
      }
    } else if (e.type === 'response.completed') {
      const r = e.response;
      requireThat(!activeReasoning && added && itemDone && doneText !== undefined && deltas > 0,
        'INCOMPLETE_RESPONSE');
      requireThat(object(r) && r.id === responseId && r.status === 'completed'
        && r.error == null && r.incomplete_details == null, 'INCOMPLETE_RESPONSE');
      requireThat(r.model === profile.model && r.reasoning?.effort === profile.effort, 'MODEL_EFFORT_MISMATCH');
      requireThat(Array.isArray(r.output) && (r.output.length === 0 || r.output.length === reasoning.length + 1), 'UNSUPPORTED_OUTPUT');
      // An empty final output may omit the duplicate; the completed item snapshot is still required.
      // Never replace a present final item or recover from arguments/deltas alone.
      if (r.output.length) {
        reasoning.forEach((item, index) => {
          reasoning[index] = mergeReasoning(item, r.output[index]);
          noteEncryptedUpdate(item, reasoning[index], observation);
        });
      }
      snapshot(r.output.length ? r.output[reasoning.length] : itemDone);
      completed = r;
    } else if (activeReasoning) {
      const item = reasoningEvent(activeReasoning, e, reasoning.length);
      if (item) {
        noteEncryptedUpdate(activeReasoning.item, item, observation);
        reasoning.push(item); activeReasoning = undefined;
        observation.reasoningItemCount = reasoning.length;
      }
    } else {
      requireThat(added && !itemDone && e.output_index === reasoning.length, 'STREAM_ORDER');
      const isTool = added.type === 'function_call';
      if (e.type === 'response.output_item.done') {
        requireThat(doneText !== undefined && (isTool || !partAdded || partDone), 'STREAM_ORDER');
        snapshot(e.item);
        itemDone = e.item;
        continue;
      }
      requireThat(e.item_id === added.id && (isTool || e.content_index === 0), 'ITEM_INDEX_MISMATCH');
      const prefix = isTool ? 'response.function_call_arguments' : 'response.output_text';
      if (e.type === `${prefix}.delta`) {
        requireThat(doneText === undefined && typeof e.delta === 'string', 'STREAM_ORDER');
        text += e.delta;
        deltas++;
        requireThat(!isTool || Buffer.byteLength(text) <= LIMITS.argumentBytes, 'ARGUMENTS_TOO_LARGE');
      } else if (e.type === `${prefix}.done`) {
        requireThat(doneText === undefined && deltas > 0, 'STREAM_ORDER');
        requireThat(!isTool || e.name === undefined || e.name === added.name, 'SNAPSHOT_MISMATCH', 'ARGUMENTS_DONE_NAME');
        doneText = isTool ? e.arguments : e.text;
        requireThat(typeof doneText === 'string' && doneText === text, 'TEXT_MISMATCH');
      } else if (!isTool && e.type === 'response.content_part.added') {
        requireThat(!partAdded && !deltas && doneText === undefined, 'STREAM_ORDER');
        part(e.part, '');
        partAdded = true;
      } else if (!isTool && e.type === 'response.content_part.done') {
        requireThat(partAdded && !partDone && doneText !== undefined, 'STREAM_ORDER');
        part(e.part, text);
        partDone = true;
      } else throw new ProtocolError('UNSUPPORTED_EVENT');
    }
  }
  requireThat(completed, 'INCOMPLETE_RESPONSE');
  let content;
  if (added.type === 'function_call') {
    const args = parse(text, LIMITS.argumentBytes);
    keys(args, ['file_path']);
    requireThat(args.file_path === FIXTURE_PATH, 'TOOL_PATH_REJECTED');
    content = { type: 'tool_use', id: added.call_id, name: 'Read', input: args };
  } else {
    requireThat(text.length > 0, 'EMPTY_REPLY');
    content = { type: 'text', text };
  }
  return { content, item: itemDone, reasoningItems: reasoning, id: responseId, usage: usage(completed.usage),
    reconstructed: completed.output.length === 0, eventCount: events.length };
}

function anthropic(result, model = ALIAS) {
  const tool = result.content.type === 'tool_use';
  const stop = tool ? 'tool_use' : 'end_turn';
  const message = { id: result.id, type: 'message', role: 'assistant', model,
    content: [result.content], stop_reason: stop, stop_sequence: null, usage: result.usage };
  const frames = [
    { type: 'message_start', message: { ...message, content: [], stop_reason: null,
      usage: { ...result.usage, output_tokens: 0 } } },
    { type: 'content_block_start', index: 0, content_block: tool
      ? { ...result.content, input: {} } : { type: 'text', text: '' } },
    { type: 'content_block_delta', index: 0, delta: tool
      ? { type: 'input_json_delta', partial_json: JSON.stringify(result.content.input) }
      : { type: 'text_delta', text: result.content.text } },
    { type: 'content_block_stop', index: 0 },
    { type: 'message_delta', delta: { stop_reason: stop, stop_sequence: null },
      usage: { output_tokens: result.usage.output_tokens } },
    { type: 'message_stop' }
  ];
  return { message, sse: frames.map(f => `event: ${f.type}\ndata: ${JSON.stringify(f)}\n\n`).join('') };
}

// ponytail: sequential reasoning items followed by one message/tool; no interleaved or parallel outputs.
// Buffer <=256 KiB before releasing any block; incremental client output needs a separate review.
export class OfflineSession {
  #state = 'NEW'; #failure; #count = 0; #policy; #profile; #timeout; #timer; #deadline;
  #initial; #input; #pending; #responseId; #decoder; #text = ''; #bytes = 0; #compat = false;
  #phase = 'not-started'; #headersAccepted = false;
  #observation = { parsedEvents: 0, eventNumber: 0, eventKind: 'not-observed', itemKind: 'not-observed' };
  #claude; #readMarker; #outputLimit; #readResults = 0; #readCalls = 0; #exactMarker = false;
  #requestShape;
  #chat; #chatOptions; #chatHistory = [];
  constructor({ headerPolicy = 'strict', timeoutMs = LIMITS.timeoutMs, profile = 'astra-xhigh', inputPolicy = 'fixture', readMarker } = {}) {
    requireThat(['fixture', 'claude-code-v2-offline', 'claude-code-read-once', 'claude-code-chat'].includes(inputPolicy), 'INVALID_POLICY');
    requireThat(inputPolicy === 'claude-code-read-once' ? typeof readMarker === 'string'
      && /^CLAUDUCT_READ_[A-F0-9]{32}$/.test(readMarker) : readMarker === undefined, 'INVALID_POLICY');
    this.#readMarker = readMarker;
    this.#claude = inputPolicy !== 'fixture';
    this.#chat = inputPolicy === 'claude-code-chat';
    this.#profile = probeProfile(profile);
    requireThat(['strict', 'codex-missing-content-type'].includes(headerPolicy), 'INVALID_POLICY');
    requireThat(Number.isInteger(timeoutMs) && timeoutMs > 0 && timeoutMs <= LIMITS.timeoutMs,
      'INVALID_TIMEOUT');
    this.#policy = headerPolicy; this.#timeout = timeoutMs;
  }
  get diagnostics() {
    return { state: this.#state, category: this.#failure ?? 'NONE', preparedRequests: this.#count,
      inputPolicy: this.#chat ? 'claude-code-chat' : this.#readMarker ? 'claude-code-read-once' : this.#claude ? 'claude-code-v2-offline' : 'fixture',
      readCalls: this.#readCalls, readResults: this.#readResults, exactMarker: this.#exactMarker,
      ...(this.#readMarker ? { requestShape: this.#requestShape ?? null } : {}),
      ...(this.#claude ? { cachePolicy: 'not-emulated', clientMetadataPolicy: 'local-only',
        readPolicy: this.#chat ? 'tools-disabled' : 'fixed-fixture-only' } : {}),
      responseBytes: this.#bytes, buffered: this.#text.length > 0,
      timerActive: this.#timer !== undefined, phase: this.#phase, headersAccepted: this.#headersAccepted,
      compatibilityApplied: this.#compat, ...this.#observation };
  }
  #clear() {
    clearTimeout(this.#timer); this.#timer = undefined;
    this.#text = ''; this.#decoder = undefined;
  }
  #arm() {
    clearTimeout(this.#timer);
    this.#deadline = performance.now() + this.#timeout;
    this.#timer = setTimeout(() => this.#fail('TIMEOUT'), this.#timeout);
    this.#timer.unref();
  }
  #fail(code) {
    this.#clear(); this.#state = code === 'CANCELLED' ? 'CANCELLED' : 'FAILED'; this.#failure = code;
    this.#initial = this.#input = this.#pending = undefined;
    this.#chatHistory = []; this.#chatOptions = undefined;
  }
  #run(action) {
    try {
      requireThat(!this.#failure, this.#failure);
      if (this.#timer !== undefined && performance.now() >= this.#deadline) throw new ProtocolError('TIMEOUT');
      return action();
    } catch (error) {
      const code = error instanceof ProtocolError ? error.code : 'INVALID_PROTOCOL';
      if (!this.#failure) this.#observation.snapshotCheck = code === 'SNAPSHOT_MISMATCH'
        && snapshotChecks.includes(error.snapshotCheck) ? error.snapshotCheck : 'not-observed';
      this.#fail(code); throw new ProtocolError(code);
    }
  }
  prepare(raw) {
    return this.#run(() => {
      this.#phase = 'request-validation';
      requireThat(['NEW', 'WAIT_TOOL_RESULT', ...(this.#chat ? ['READY'] : [])].includes(this.#state), 'INVALID_STATE');
      requireThat(this.#count < (this.#chat ? CHAT_REQUESTS : LIMITS.requests), 'REQUEST_BUDGET');
      const doc = request(raw, this.#claude, this.#profile, this.#readMarker ? value => {
        const messages = Array.isArray(value?.messages) ? value.messages : [];
        const previous = this.#initial?.messages;
        this.#requestShape = {
          following: this.#state === 'WAIT_TOOL_RESULT', messageCount: messages.length,
          truncated: messages.length > 12,
          previousPrefixMatches: previous ? isDeepStrictEqual(messages.slice(0, previous.length), previous) : null,
          messages: messages.slice(0, 12).map(m => ({
            role: ['user', 'assistant', 'system'].includes(m?.role) ? m.role : 'other',
            contentKind: typeof m?.content === 'string' ? 'string' : Array.isArray(m?.content) ? 'array' : 'other',
            blockCount: Array.isArray(m?.content) ? m.content.length : 0,
            blocks: Array.isArray(m?.content) ? m.content.slice(0, 12).map(b => ({
              type: ['text', 'tool_use', 'tool_result', 'thinking', 'redacted_thinking'].includes(b?.type) ? b.type : 'other',
              ...(b?.type === 'tool_result' ? { linked: typeof b.tool_use_id === 'string' && b.tool_use_id === this.#pending?.content.id,
                isError: b.is_error === true } : {})
            })) : []
          }))
        };
      } : undefined, this.#chat);
      if (this.#chat) return this.#prepareChat(doc);
      const following = this.#state === 'WAIT_TOOL_RESULT';
      if (!following) {
        this.#initial = doc;
        this.#input = doc.system === undefined ? []
          : [{ role: 'developer', content: textParts(doc.system, 'input_text', this.#claude) }];
        if (this.#claude) requireThat(doc.messages.length === 2, 'UNSUPPORTED_MESSAGES');
        doc.messages.forEach((m, index) => {
          keys(m, ['role', 'content']);
          requireThat(m.role === (this.#claude ? ['user', 'system'][index]
            : index % 2 === 0 ? 'user' : 'assistant'), 'UNSUPPORTED_MESSAGES');
          this.#input.push({ role: m.role === 'system' ? 'developer' : m.role, content: textParts(m.content,
            m.role === 'assistant' ? 'output_text' : 'input_text', this.#claude) });
        });
        requireThat(doc.messages.at(-1).role === (this.#claude ? 'system' : 'user'), 'UNSUPPORTED_MESSAGES');
      } else {
        const { messages, ...options } = doc;
        const { messages: previous, ...original } = this.#initial;
        const extraSystem = this.#claude && messages.length === previous.length + 3;
        const optionsMatch = this.#claude ? isDeepStrictEqual(claudeOptions(doc), claudeOptions(this.#initial))
          : isDeepStrictEqual(options, original);
        const prefixMatches = this.#claude
          ? isDeepStrictEqual(messages.slice(0, previous.length).map(claudeTextMessage), previous.map(claudeTextMessage))
          : isDeepStrictEqual(messages.slice(0, previous.length), previous);
        if (this.#requestShape) Object.assign(this.#requestShape, { optionsMatch, canonicalPrefixMatches: prefixMatches });
        requireThat(optionsMatch && (messages.length === previous.length + 2 || extraSystem)
          && prefixMatches, 'HISTORY_MISMATCH');
        let assistant = messages[previous.length];
        if (this.#claude) {
          keys(assistant, ['role', 'content']);
          requireThat(Array.isArray(assistant.content) && assistant.content.length === 1, 'HISTORY_MISMATCH');
          const { cache_control, ...tool } = assistant.content[0];
          cacheHint(cache_control);
          assistant = { ...assistant, content: [tool] };
        }
        requireThat(isDeepStrictEqual(assistant, { role: 'assistant', content: [this.#pending.content] }),
          'HISTORY_MISMATCH');
        const tail = extraSystem ? claudeTextMessage(messages.at(-1)) : undefined;
        requireThat(!tail || tail.role === 'system', 'HISTORY_MISMATCH');
        const m = messages[previous.length + 1];
        keys(m, ['role', 'content']);
        requireThat(m.role === 'user' && Array.isArray(m.content) && m.content.length === 1, 'INVALID_TOOL_RESULT');
        const result = m.content[0];
        keys(result, this.#claude ? ['type', 'tool_use_id', 'content', 'is_error', 'cache_control']
          : ['type', 'tool_use_id', 'content', 'is_error']);
        if (this.#claude) cacheHint(result.cache_control);
        requireThat(result.type === 'tool_result' && result.tool_use_id === this.#pending.content.id
          && (typeof result.content === 'string' || (this.#claude && Array.isArray(result.content)))
          && (result.is_error === undefined || typeof result.is_error === 'boolean'), 'INVALID_TOOL_RESULT');
        const content = typeof result.content === 'string' ? result.content : textParts(result.content, 'text', true);
        if (this.#readMarker) {
          requireThat(result.is_error !== true, 'TOOL_EXECUTION_DENIED');
          const visible = typeof content === 'string' ? content : content.map(part => part.text).join('\n');
          const markerLine = visible.split('\n').some(line => {
            const text = line.trim();
            if (text === this.#readMarker) return true;
            const separator = text.includes('→') ? text.indexOf('→') : text.indexOf('\t');
            return separator > 0 && /^[0-9]+$/.test(text.slice(0, separator).trim())
              && text.slice(separator + 1).trim() === this.#readMarker;
          });
          requireThat(visible.split(this.#readMarker).length === 2
            && markerLine,
          'READ_RESULT_MISMATCH');
          this.#readResults++;
        }
        this.#input.push(...this.#pending.reasoningItems, this.#pending.item, { type: 'function_call_output', call_id: result.tool_use_id,
          output: JSON.stringify({ is_error: result.is_error === true, content }) });
        if (tail) this.#input.push({ role: 'developer', content: tail.content.map(part => ({ ...part, type: 'input_text' })) });
        this.#pending = undefined;
      }
      const prepared = JSON.stringify(body(doc, this.#input, following, this.#profile));
      this.#outputLimit = doc.max_tokens;
      requireThat(Buffer.byteLength(prepared) <= LIMITS.requestBytes, 'INPUT_TOO_LARGE');
      this.#count++; this.#state = 'AWAIT_HEADERS'; this.#bytes = 0;
      this.#headersAccepted = this.#compat = false;
      this.#observation = { parsedEvents: 0, eventNumber: 0, eventKind: 'not-observed', itemKind: 'not-observed' };
      this.#arm();
      return prepared;
    });
  }
  #prepareChat(doc) {
    const { messages, system, tools, ...rest } = doc;
    const options = { ...rest, system: system === undefined ? [] : textParts(system, 'text', true) };
    const history = messages.map(message => {
      keys(message, ['role', 'content']);
      requireThat(['user', 'assistant', 'system'].includes(message.role), 'UNSUPPORTED_MESSAGES');
      return { role: message.role, content: textParts(message.content, 'text', true) };
    });
    const previous = this.#chatHistory.length;
    requireThat(!this.#chatOptions || isDeepStrictEqual(options, this.#chatOptions), 'HISTORY_MISMATCH');
    requireThat(isDeepStrictEqual(history.slice(0, previous), this.#chatHistory), 'HISTORY_MISMATCH');
    const added = history.slice(previous);
    requireThat(added.some(m => m.role === 'user') && added.every(m => m.role !== 'assistant'), 'UNSUPPORTED_MESSAGES');
    this.#input ??= options.system.length ? [{ role: 'developer', content: textParts(system, 'input_text', true) }] : [];
    for (const message of added) this.#input.push({ role: message.role === 'system' ? 'developer' : 'user',
      content: message.content.map(part => ({ type: 'input_text', text: part.text })) });
    const prepared = JSON.stringify({ ...body(doc, this.#input, true, this.#profile), tools: [] });
    requireThat(Buffer.byteLength(prepared) <= LIMITS.requestBytes, 'INPUT_TOO_LARGE');
    this.#chatOptions = options; this.#chatHistory = history; this.#outputLimit = doc.max_tokens;
    this.#count++; this.#state = 'AWAIT_HEADERS'; this.#bytes = 0; this.#headersAccepted = this.#compat = false;
    this.#observation = { parsedEvents: 0, eventNumber: 0, eventKind: 'not-observed', itemKind: 'not-observed' };
    this.#arm(); return prepared;
  }
  begin(meta) {
    return this.#run(() => {
      this.#phase = 'headers';
      requireThat(this.#state === 'AWAIT_HEADERS', 'INVALID_STATE');
      this.#compat = checkHeaders(meta, this.#policy);
      this.#headersAccepted = true;
      this.#state = 'RECEIVING'; this.#decoder = new TextDecoder('utf-8', { fatal: true });
    });
  }
  push(bytes) {
    return this.#run(() => {
      this.#phase = 'body-decoding';
      requireThat(this.#state === 'RECEIVING' && bytes instanceof Uint8Array, 'INVALID_STATE');
      this.#bytes += bytes.byteLength;
      requireThat(this.#bytes <= LIMITS.responseBytes, 'RESPONSE_TOO_LARGE');
      try { this.#text += this.#decoder.decode(bytes, { stream: true }); }
      catch { throw new ProtocolError('INVALID_UTF8'); }
    });
  }
  finish() {
    return this.#run(() => {
      requireThat(this.#state === 'RECEIVING', 'INVALID_STATE');
      try { this.#text += this.#decoder.decode(); }
      catch { throw new ProtocolError('INVALID_UTF8'); }
      this.#phase = 'sse-framing';
      const events = eventsFrom(this.#text, this.#observation);
      this.#phase = 'response-validation';
      const result = validateResponse(events, this.#chat || this.#count === 2, this.#observation, this.#profile);
      if (this.#chat) {
        requireThat(result.content.type === 'text', 'UNSUPPORTED_TOOL_CALL');
        requireThat(result.usage.output_tokens <= this.#outputLimit, 'OUTPUT_TOKEN_LIMIT_EXCEEDED');
      }
      if (this.#readMarker) {
        requireThat(result.usage.output_tokens <= this.#outputLimit, 'OUTPUT_TOKEN_LIMIT_EXCEEDED');
        if (this.#count === 1) {
          requireThat(result.content.type === 'tool_use', 'NO_TOOL_CALL'); this.#readCalls++;
        } else {
          requireThat(this.#readResults === 1 && result.content.type === 'text'
            && result.content.text === this.#readMarker, 'FINAL_MARKER_MISMATCH'); this.#exactMarker = true;
        }
      }
      this.#phase = 'completion';
      requireThat(performance.now() < this.#deadline, 'TIMEOUT');
      requireThat(result.id !== this.#responseId, 'DUPLICATE_RESPONSE_ID');
      this.#responseId = result.id;
      const output = anthropic(structuredClone(result), this.#chat ? this.#profile.model : ALIAS);
      this.#clear();
      if (this.#chat) {
        this.#input.push(...result.reasoningItems, result.item);
        this.#chatHistory.push({ role: 'assistant', content: [structuredClone(result.content)] });
        this.#state = 'READY';
      } else if (result.content.type === 'tool_use') {
        this.#pending = result; this.#state = 'WAIT_TOOL_RESULT'; this.#arm();
      }
      else { this.#state = 'COMPLETE'; this.#input = this.#initial = undefined; }
      return { ...output, diagnostics: { accepted: true, headerPolicy: this.#policy,
        compatibilityApplied: this.#compat, eventCount: result.eventCount,
        reconstructed: result.reconstructed } };
    });
  }
  cancel() { if (!['COMPLETE', 'FAILED', 'CANCELLED'].includes(this.#state)) this.#fail('CANCELLED'); }
  disconnect() { if (!['COMPLETE', 'FAILED', 'CANCELLED'].includes(this.#state)) this.#fail('DISCONNECTED'); }
}
