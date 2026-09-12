import { createServer } from 'node:http';
import { pathToFileURL } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { OfflineSession, ALIAS, readTool } from './adapter.mjs';

// Public fixture marker, NOT a credential or a product authentication mechanism.
export const FIXTURE_MARKER = 'clauduct-public-local-inspection';
export const INSPECT_LIMITS = Object.freeze({ bodyBytes: 512 * 1024, headerBytes: 8192,
  requests: 8, connections: 16, captures: 2, lifetimeMs: 60000, requestMs: 5000,
  observationMs: 5000, nodes: 20000, depth: 32 });
const fields = ['model', 'stream', 'max_tokens', 'system', 'messages', 'tools', 'tool_choice',
  'thinking', 'metadata', 'temperature', 'top_p', 'top_k', 'stop_sequences', 'service_tier',
  'output_config', 'context_management'];
const blockTypes = ['text', 'tool_use', 'tool_result', 'image', 'thinking', 'redacted_thinking',
  'tool_reference', 'server_tool_use'];
const schemaNames = ['file_path', 'offset', 'limit', 'pages'];
const schemaTypes = ['string', 'number', 'integer', 'boolean', 'object', 'array', 'null'];
const diagnosticBetas = ['prompt-caching-2024-07-31', 'token-efficient-tools-2025-02-19',
  'output-128k-2025-02-19', 'interleaved-thinking-2025-05-14', 'context-1m-2025-08-07',
  'context-management-2025-06-27', 'structured-outputs-2025-11-13', 'compact-2026-01-12'];
const requestErrors = new Set(['INVALID_JSON', 'INPUT_TOO_LARGE', 'UNSUPPORTED_FIELDS',
  'UNSUPPORTED_REQUEST', 'UNSUPPORTED_MAX_TOKENS', 'UNSUPPORTED_TOOLS', 'UNSUPPORTED_MESSAGES',
  'UNSUPPORTED_CONTENT', 'INVALID_PROTOCOL']);
const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const own = (v, k) => Object.hasOwn(v, k);
const blankCounts = names => Object.fromEntries(names.map(n => [n, 0]));
const kind = v => v === undefined ? 'absent' : v === null ? 'null' : Array.isArray(v) ? 'array' : typeof v;
const classified = (v, allowed) => allowed.includes(v) ? v : v === undefined ? 'absent' : 'other';
const schemaKeywords = ['type', 'enum', 'pattern', 'description', 'minimum', 'maximum',
  'exclusiveMinimum', 'exclusiveMaximum', 'default'];
function fieldKinds(value, names) {
  return { kind: kind(value), fields: Object.fromEntries(names.map(name => [name, kind(object(value) ? value[name] : undefined)])),
    unknownFieldCount: object(value) ? Object.keys(value).filter(name => !names.includes(name)).length : 0 };
}
function numericConstraint(value) {
  return { kind: kind(value), value: Number.isSafeInteger(value) && value >= 0 ? value : null };
}
class InspectionError extends Error { constructor(code) { super(code); this.code = code; } }
function ensure(ok, code) { if (!ok) throw new InspectionError(code); }

// Only fixed field names, fixed classifications, bounded counts and booleans leave this function.
export function inspectRequest(raw) {
  ensure(typeof raw === 'string', 'INVALID_JSON');
  ensure(Buffer.byteLength(raw) <= INSPECT_LIMITS.bodyBytes, 'BODY_TOO_LARGE');
  let doc;
  try { doc = JSON.parse(raw); } catch { throw new InspectionError('INVALID_JSON'); }
  ensure(object(doc), 'INVALID_REQUEST');
  let nodes = 0, cacheControlCount = 0;
  const pending = [[doc, 0]];
  while (pending.length) {
    const [value, depth] = pending.pop();
    ensure(++nodes <= INSPECT_LIMITS.nodes && depth <= INSPECT_LIMITS.depth, 'STRUCTURE_LIMIT');
    if (value !== null && typeof value === 'object') {
      const entries = Object.entries(value);
      ensure(entries.length + pending.length + nodes <= INSPECT_LIMITS.nodes, 'STRUCTURE_LIMIT');
      for (const [key, child] of entries) {
        if (key === 'cache_control') cacheControlCount++;
        pending.push([child, depth + 1]);
      }
    }
  }
  const summary = { fields: Object.fromEntries(fields.map(k => [k, own(doc, k)])),
    unknownFieldCount: Object.keys(doc).filter(k => !fields.includes(k)).length,
    modelMatchesAlias: doc.model === ALIAS, streamTrue: doc.stream === true,
    maxTokens: Number.isSafeInteger(doc.max_tokens) && doc.max_tokens >= 0 && doc.max_tokens <= 262144
      ? doc.max_tokens : null,
    // The offline adapter's max_output_tokens mapping is not live transport support.
    transportTokenLimit: own(doc, 'max_tokens') ? 'explicit-unsupported' : 'not-requested',
    systemKind: typeof doc.system === 'string' ? 'string' : Array.isArray(doc.system) ? 'array' : 'other',
    systemBlocks: typeof doc.system === 'string' ? 1 : Array.isArray(doc.system) ? doc.system.length : 0,
    systemTextChars: 0, messageCount: Array.isArray(doc.messages) ? doc.messages.length : 0,
    roles: { user: 0, assistant: 0, other: 0 }, blocks: { ...blankCounts(blockTypes), other: 0 },
    toolCount: Array.isArray(doc.tools) ? doc.tools.length : 0, readCount: 0, otherToolCount: 0,
    endConversationCount: 0,
    cacheControlCount, readSchema: null, fixtureSchemaMatches: false,
    toolChoice: ['auto', 'any', 'tool', 'none'].includes(doc.tool_choice?.type) ? doc.tool_choice.type : 'other',
    thinking: ['enabled', 'disabled', 'adaptive'].includes(doc.thinking?.type) ? doc.thinking.type : 'other' };
  // Fixed vocabulary only; no text, role name, metadata value or schema string is echoed.
  summary.messageRoles = Array.isArray(doc.messages) ? doc.messages.slice(0, 8).map(message =>
    classified(message?.role, ['user', 'assistant', 'system', 'developer', 'tool', 'function'])) : [];
  summary.messageRolesTruncated = Array.isArray(doc.messages) && doc.messages.length > 8;
  summary.optionDetails = {
    thinking: fieldKinds(doc.thinking, ['type', 'budget_tokens', 'display']),
    thinkingBudget: numericConstraint(doc.thinking?.budget_tokens),
    metadata: fieldKinds(doc.metadata, ['user_id']),
    outputConfig: fieldKinds(doc.output_config, ['effort', 'format']),
    effort: classified(doc.output_config?.effort, ['low', 'medium', 'high', 'max', 'xhigh']),
    contextManagement: fieldKinds(doc.context_management, ['edits']),
    contextEdits: Array.isArray(doc.context_management?.edits) ? doc.context_management.edits.slice(0, 8).map(edit => ({
      ...fieldKinds(edit, ['type', 'trigger', 'keep', 'clear_at_least', 'clear_tool_inputs', 'exclude_tools']),
      strategy: classified(edit?.type, ['clear_tool_uses_20250919', 'clear_thinking_20251015']),
      keepAll: edit?.keep === 'all',
      thresholds: Object.fromEntries(['trigger', 'keep', 'clear_at_least'].map(name => [name, {
        type: classified(edit?.[name]?.type, ['input_tokens', 'tool_uses', 'thinking_turns']),
        value: numericConstraint(edit?.[name]?.value) }])) })) : [],
    contextEditsTruncated: Array.isArray(doc.context_management?.edits) && doc.context_management.edits.length > 8 };
  if (typeof doc.system === 'string') summary.systemTextChars = doc.system.length;
  else if (Array.isArray(doc.system)) for (const part of doc.system) {
    if (part?.type === 'text' && typeof part.text === 'string') summary.systemTextChars += part.text.length;
  }
  if (Array.isArray(doc.messages)) for (const message of doc.messages) {
    summary.roles[message?.role === 'user' ? 'user' : message?.role === 'assistant' ? 'assistant' : 'other']++;
    if (typeof message?.content === 'string') summary.blocks.text++;
    else if (Array.isArray(message?.content)) for (const part of message.content) {
      summary.blocks[blockTypes.includes(part?.type) ? part.type : 'other']++;
    } else summary.blocks.other++;
  }
  if (Array.isArray(doc.tools)) for (const tool of doc.tools) {
    if (tool?.name === 'EndConversation') summary.endConversationCount++;
    if (tool?.name !== 'Read') { summary.otherToolCount++; continue; }
    summary.readCount++;
    if (summary.readCount !== 1) { summary.readSchema = null; summary.fixtureSchemaMatches = false; continue; }
    const schema = tool.input_schema;
    summary.fixtureSchemaMatches = isDeepStrictEqual(schema, readTool().input_schema);
    if (!object(schema)) continue;
    const props = object(schema.properties) ? schema.properties : {};
    const required = Array.isArray(schema.required) ? schema.required : [];
    summary.readSchema = { rootIsObject: schema.type === 'object',
      properties: Object.fromEntries(schemaNames.map(name => {
        const p = props[name];
        return [name, { present: own(props, name), required: required.includes(name),
          type: schemaTypes.includes(p?.type) ? p.type : 'other',
          hasEnum: object(p) && own(p, 'enum'), hasPattern: object(p) && own(p, 'pattern'),
          hasDescription: object(p) && own(p, 'description'),
          constraints: Object.fromEntries(['minimum', 'maximum', 'exclusiveMinimum', 'exclusiveMaximum', 'default']
            .map(key => [key, numericConstraint(object(p) ? p[key] : undefined)])),
          unknownKeywordCount: object(p) ? Object.keys(p).filter(k => !schemaKeywords.includes(k)).length : 0 }];
      })), unknownPropertyCount: Object.keys(props).filter(k => !schemaNames.includes(k)).length,
      unknownRequiredCount: required.filter(k => !schemaNames.includes(k)).length,
      additionalProperties: schema.additionalProperties === false ? 'false'
        : schema.additionalProperties === true ? 'true' : own(schema, 'additionalProperties') ? 'other' : 'absent',
      extraKeywords: fieldKinds(schema, ['type', 'properties', 'required', 'additionalProperties', '$schema', 'description', 'title']),
      unknownKeywordCount: Object.keys(schema).filter(k => !['type', 'properties', 'required', 'additionalProperties'].includes(k)).length };
  }
  const session = new OfflineSession();
  try {
    session.prepare(raw);
    summary.adapter = { accepted: true, category: 'SUPPORTED_FIXTURE' };
  } catch (e) {
    summary.adapter = { accepted: false, category: requestErrors.has(e.code) ? e.code : 'INVALID_PROTOCOL' };
  } finally { session.cancel(); }
  return summary;
}

function bounded(value, maximum) {
  ensure(Number.isInteger(value) && value > 0 && value <= maximum, 'INVALID_LIMIT');
  return value;
}

// ponytail: a finite loopback inspection session, never an upstream proxy or a product server.
export async function startInspector({ lifetimeMs = INSPECT_LIMITS.lifetimeMs,
  requestMs = INSPECT_LIMITS.requestMs, observationMs = INSPECT_LIMITS.observationMs } = {}) {
  bounded(lifetimeMs, INSPECT_LIMITS.lifetimeMs);
  bounded(requestMs, INSPECT_LIMITS.requestMs);
  bounded(observationMs, INSPECT_LIMITS.observationMs);
  // Synthetic clients import only FIXTURE_MARKER from this module. Load the
  // server helper only when starting the inspector; their read scope stays poc/.
  const { installHttpClose } = await import('../src/http-close.mjs');
  const sockets = new Set(), timers = new Set();
  const connectionClosures = new WeakMap();
  let pendingConnectionCloses = 0;
  const counters = { receivedRequests: 0, acceptedConnections: 0, droppedConnections: 0, messages: 0, countTokens: 0,
    hello: 0, rejected: 0, truncated: 0, timeouts: 0, malformedHttp: 0, transportErrorEvents: 0, captured: 0 };
  const records = [];
  let port, deadlineTimer, observationTimer, closing = false, sessionId, closeReason, clearBody;
  let complete;
  const done = new Promise(resolve => { complete = resolve; });
  const server = createServer({ maxHeaderSize: INSPECT_LIMITS.headerBytes,
    headersTimeout: requestMs, requestTimeout: requestMs, connectionsCheckingInterval: Math.min(requestMs, 100) }, onRequest);
  server.maxConnections = 4;
  server.maxRequestsPerSocket = 1;
  server.setTimeout(requestMs, socket => { counters.timeouts++; socket.destroy(); });

  function result() {
    return { category: counters.captured > 0 ? 'REQUEST_SHAPE_CAPTURED' : 'NO_MESSAGE_CAPTURED',
      closeReason, counters: { ...counters }, records: structuredClone(records),
      activeSockets: sockets.size, activeRequestTimers: timers.size, activeBodies: clearBody ? 1 : 0, pendingConnectionCloses,
      upstreamRequests: 0, toolExecutions: 0, persistedBodies: 0,
      authenticatedProductSession: false };
  }
  function stop(reason) {
    if (closing) return done;
    closing = true; closeReason = reason; sessionId = undefined;
    clearBody?.();
    clearTimeout(deadlineTimer); clearTimeout(observationTimer);
    for (const timer of timers) clearTimeout(timer);
    timers.clear();
    const closedSockets = [...sockets].map(socket => new Promise(resolve => {
      socket.once('close', resolve);
      if (!connectionClosures.get(socket)?.pending) socket.resetAndDestroy();
    }));
    // Keep the final diagnostic reply intact while its bounded normal close
    // drains. server.close() would destroy those completed idle responses.
    void Promise.all(closedSockets).then(() => server.close(() => complete(result())));
    return done;
  }
  function reply(res, status, code) {
    if (res.destroyed || res.writableEnded) return;
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store',
      Connection: 'close', 'X-Content-Type-Options': 'nosniff' });
    res.end(JSON.stringify({ type: 'error', error: { type: 'invalid_request_error', message: code } }));
  }
  function reject(res, status, category) {
    counters.rejected++;
    if (records.length < INSPECT_LIMITS.requests) records.push({ route: 'rejected', category });
    reply(res, status, category);
  }
  function onRequest(req, res) {
    counters.receivedRequests++;
    req.on('error', () => { counters.transportErrorEvents++; });
    res.on('error', () => { counters.transportErrorEvents++; });
    if (closing) { reply(res, 400, 'INSPECTOR_CLOSED'); return; }
    if (counters.receivedRequests > INSPECT_LIMITS.requests) {
      reject(res, 400, 'REQUEST_BUDGET'); res.once('finish', () => stop('REQUEST_BUDGET')); return;
    }
    const names = req.rawHeaders.filter((_, i) => i % 2 === 0).map(v => v.toLowerCase());
    if (new Set(names).size !== names.length) { reject(res, 400, 'DUPLICATE_HEADER'); return; }
    if (req.socket.remoteAddress !== '127.0.0.1' || req.headers.host !== `127.0.0.1:${port}`
      || req.headers.origin !== undefined || req.headers['sec-fetch-site'] !== undefined
      || req.headers.forwarded !== undefined || names.some(n => n.startsWith('x-forwarded-'))) {
      reject(res, 400, 'LOCAL_BOUNDARY_REJECTED'); return;
    }
    const bearerMarker = req.headers.authorization === `Bearer ${FIXTURE_MARKER}`;
    const apiKeyMarker = req.headers['x-api-key'] === FIXTURE_MARKER;
    if ((req.headers.authorization !== undefined && !bearerMarker) || req.headers.cookie !== undefined
      || req.headers['proxy-authorization'] !== undefined
      || (req.headers['x-api-key'] !== undefined && !apiKeyMarker)) {
      reject(res, 400, 'REAL_CREDENTIALS_NOT_ACCEPTED'); return;
    }
    if (bearerMarker && apiKeyMarker) { reject(res, 400, 'AMBIGUOUS_FIXTURE_MARKER'); return; }
    if (req.headers['x-claude-code-agent-id'] !== undefined || req.headers['x-claude-code-parent-agent-id'] !== undefined) {
      reject(res, 400, 'SUBAGENT_REJECTED'); return;
    }
    const target = req.url;
    if (!target.startsWith('/') || target.startsWith('//')) { reject(res, 400, 'INVALID_TARGET'); return; }
    const path = target.split('?', 1)[0];
    if (req.method === 'HEAD' && path === '/api/hello') {
      counters.hello++; res.writeHead(204, { Connection: 'close', 'Cache-Control': 'no-store' }); res.end(); return;
    }
    if (req.method !== 'POST' || !['/v1/messages', '/v1/messages/count_tokens'].includes(path)) {
      reject(res, 404, 'UNSUPPORTED_ROUTE'); return;
    }
    if (!bearerMarker && !apiKeyMarker) { reject(res, 400, 'FIXTURE_MARKER_REQUIRED'); return; }
    if (String(req.headers['content-type'] ?? '').split(';', 1)[0].trim().toLowerCase() !== 'application/json'
      || (req.headers['content-encoding'] !== undefined && req.headers['content-encoding'] !== 'identity')) {
      reject(res, 415, 'UNSUPPORTED_BODY_ENCODING'); return;
    }
    const id = req.headers['x-claude-code-session-id'];
    if (id !== undefined && (typeof id !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(id))) {
      reject(res, 400, 'INVALID_SESSION_ID'); return;
    }
    // Bind presence as well as value, so a follow-up cannot drop a previously supplied ID.
    if (sessionId !== undefined && sessionId !== (id ?? '')) { reject(res, 400, 'SESSION_MISMATCH'); return; }
    const countRoute = path === '/v1/messages/count_tokens';
    if (countRoute) counters.countTokens++; else counters.messages++;
    if (!countRoute && counters.captured >= INSPECT_LIMITS.captures) {
      reject(res, 400, 'CAPTURE_BUDGET'); res.once('finish', () => stop('CAPTURE_BUDGET')); return;
    }
    if (Number(req.headers['content-length']) > INSPECT_LIMITS.bodyBytes) {
      reject(res, 413, 'BODY_TOO_LARGE'); return;
    }
    if (clearBody) { reject(res, 400, 'CONCURRENT_REQUEST'); return; }
    let bytes = 0, raw = '', finished = false;
    const decoder = new TextDecoder('utf-8', { fatal: true });
    const timer = setTimeout(() => {
      if (finished) return;
      counters.timeouts++; cleanup(); reject(res, 408, 'REQUEST_TIMEOUT'); req.socket.destroy();
    }, requestMs);
    timers.add(timer);
    function cleanup() { finished = true; clearTimeout(timer); timers.delete(timer); raw = ''; clearBody = undefined; }
    clearBody = cleanup;
    req.on('aborted', () => { if (!finished) { counters.truncated++; cleanup(); } });
    req.on('error', () => { if (!finished) { counters.truncated++; cleanup(); } });
    res.on('close', () => { if (!finished) cleanup(); });
    req.on('data', chunk => {
      if (finished) return;
      bytes += chunk.length;
      if (bytes > INSPECT_LIMITS.bodyBytes) { cleanup(); reject(res, 413, 'BODY_TOO_LARGE'); return; }
      try { raw += decoder.decode(chunk, { stream: true }); }
      catch { cleanup(); reject(res, 400, 'INVALID_UTF8'); }
    });
    req.on('end', () => {
      if (finished) return;
      try {
        try { raw += decoder.decode(); } catch { throw new InspectionError('INVALID_UTF8'); }
        const shape = inspectRequest(raw);
        const betas = typeof req.headers['anthropic-beta'] === 'string'
          ? req.headers['anthropic-beta'].split(',').map(value => value.trim()) : [];
        const betaValues = new URLSearchParams(target.includes('?')
          ? target.slice(target.indexOf('?') + 1) : '').getAll('beta');
        if (sessionId === undefined) sessionId = id ?? '';
        if (records.length < INSPECT_LIMITS.requests) records.push({
          route: countRoute ? 'count_tokens' : 'messages', category: 'SHAPE_INSPECTED', bodyBytes: bytes,
          betaQuery: betaValues.length === 1 && betaValues[0] === 'true',
          headers: { fixtureMarkerSource: bearerMarker ? 'bearer' : 'api_key',
            versionPresent: req.headers['anthropic-version'] !== undefined,
            versionMatches: req.headers['anthropic-version'] === '2023-06-01',
            betaPresent: req.headers['anthropic-beta'] !== undefined,
            betaCapabilities: Object.fromEntries(diagnosticBetas.map(name => [name, betas.includes(name)])),
            unknownBetaCount: betas.filter(value => !diagnosticBetas.includes(value)).length,
            sessionIdPresent: id !== undefined }, shape });
        if (!countRoute) {
          counters.captured++;
          if (!observationTimer) observationTimer = setTimeout(() => stop('OBSERVATION_WINDOW_ENDED'), observationMs);
        }
        cleanup();
        reply(res, countRoute ? 404 : 400, countRoute ? 'CLAUDUCT_COUNT_UNAVAILABLE' : 'CLAUDUCT_INSPECTION_COMPLETE');
      } catch (e) {
        cleanup(); reject(res, 400, e instanceof InspectionError ? e.code : 'INVALID_PROTOCOL');
      }
    });
  }
  server.on('connection', socket => {
    counters.acceptedConnections++;
    sockets.add(socket);
    socket.on('error', () => { counters.transportErrorEvents++; });
    socket.once('close', () => sockets.delete(socket));
    connectionClosures.set(socket, installHttpClose(socket, { onPending: change => { pendingConnectionCloses += change; } }));
    if (closing || counters.acceptedConnections > INSPECT_LIMITS.connections) {
      socket.destroy(); stop('CONNECTION_BUDGET');
    }
  });
  server.on('clientError', (_error, socket) => { counters.malformedHttp++; socket.destroy(); });
  function rejectExpectation(req, res) {
    counters.receivedRequests++;
    req.on('error', () => { counters.transportErrorEvents++; });
    res.on('error', () => { counters.transportErrorEvents++; });
    if (counters.receivedRequests > INSPECT_LIMITS.requests) res.once('finish', () => stop('REQUEST_BUDGET'));
    reject(res, 417, 'EXPECT_REJECTED');
  }
  function rejectUpgrade(_req, socket) {
    counters.receivedRequests++; counters.rejected++; socket.destroy();
    if (counters.receivedRequests > INSPECT_LIMITS.requests) stop('REQUEST_BUDGET');
  }
  server.on('checkContinue', rejectExpectation);
  server.on('checkExpectation', rejectExpectation);
  server.on('connect', rejectUpgrade);
  server.on('upgrade', rejectUpgrade);
  server.on('dropRequest', () => { counters.receivedRequests++; counters.rejected++; stop('PIPELINE_REJECTED'); });
  server.on('drop', () => { counters.droppedConnections++; stop('CONCURRENCY_LIMIT'); });
  await new Promise((resolve, rejectListen) => {
    server.once('error', () => rejectListen(new InspectionError('LISTEN_FAILED')));
    server.listen(0, '127.0.0.1', resolve);
  });
  port = server.address().port;
  server.on('error', () => stop('SERVER_ERROR'));
  deadlineTimer = setTimeout(() => stop('SESSION_TIMEOUT'), lifetimeMs);
  return { port, done, close: () => stop('CLOSED_BY_CALLER'), diagnostics: () => result() };
}

async function main() {
  if (process.argv.length !== 3 || process.argv[2] !== '--listen'
    || !process.stdin.isTTY || !process.stdout.isTTY) {
    process.stdout.write(JSON.stringify({ category: 'USER_TERMINAL_REQUIRED' }) + '\n');
    process.exitCode = 1; return;
  }
  if (process.env.NODE_OPTIONS || process.env.NODE_DEBUG || process.execArgv.length) {
    process.stdout.write(JSON.stringify({ category: 'DEBUG_RUNTIME_UNSUPPORTED' }) + '\n');
    process.exitCode = 1; return;
  }
  let probe;
  try {
    probe = await startInspector();
    const cancel = () => { void probe.close(); };
    process.once('SIGINT', cancel);
    process.stdout.write(JSON.stringify({ category: 'LOCAL_INSPECTOR_READY', host: '127.0.0.1',
      port: probe.port, lifetimeSeconds: 60 }) + '\n');
    const result = await probe.done;
    process.removeListener('SIGINT', cancel);
    process.stdout.write(JSON.stringify(result) + '\n');
  } catch {
    if (probe) await probe.close();
    process.stdout.write(JSON.stringify({ category: 'INSPECTOR_FAILED' }) + '\n');
    process.exitCode = 1;
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) await main();
