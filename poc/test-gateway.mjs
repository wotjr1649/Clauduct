import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { connect } from 'node:net';
import { PassThrough } from 'node:stream';
import { setTimeout as delay } from 'node:timers/promises';
import { startGateway, deliverSse } from './gateway.mjs';
import { createLoopbackCodexTransport, createCodexTransport } from './codex-transport.mjs';
import { ALIAS, MODEL, EFFORT, FIXTURE_PATH, LIMITS, readTool, probeProfile } from './adapter.mjs';

const marker = 'CLAUDUCT_GATEWAY_FIXTURE';
const suiteTimeout = setTimeout(() => { process.stderr.write('GATEWAY_TEST_TIMEOUT\n'); process.exit(1); },
  process.argv.includes('--lifetime-timeout') ? 250000 : 60000);
const initial = { model: ALIAS, stream: true, system: 'Synthetic gateway instructions.',
  messages: [{ role: 'user', content: 'SYNTHETIC_PRIVATE_VALUE' }], tools: [readTool()], tool_choice: { type: 'auto' } };
function wire(kind, number, text = marker) {
  const tool = kind === 'tool';
  const value = tool ? JSON.stringify({ file_path: FIXTURE_PATH }) : text;
  const item = tool ? { id: `fc_${number}`, type: 'function_call', call_id: `call_${number}`, name: 'Read',
    arguments: value, status: 'completed' } : { id: `msg_${number}`, type: 'message', role: 'assistant',
    content: [{ type: 'output_text', text: value, annotations: [] }], status: 'completed' };
  const prefix = tool ? 'response.function_call_arguments' : 'response.output_text';
  const position = { item_id: item.id, output_index: 0, ...(tool ? {} : { content_index: 0 }) };
  const events = [
    { type: 'response.created', response: { id: `resp_${number}`, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress',
      ...(tool ? { arguments: '' } : { content: [] }) } },
    { type: `${prefix}.delta`, ...position, delta: value },
    { type: `${prefix}.done`, ...position, ...(tool ? { arguments: value } : { text: value }) },
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: `resp_${number}`, status: 'completed', model: MODEL,
      reasoning: { effort: EFFORT }, output: [item], usage: { input_tokens: 28, output_tokens: 5, total_tokens: 33 } } }
  ];
  return Buffer.from(events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join(''));
}
function frames(response) { return response.body.trim().split('\n\n').map(frame => JSON.parse(frame.split('\ndata: ')[1])); }
function follow(response, isError = false) {
  const events = frames(response), block = events.find(e => e.type === 'content_block_start').content_block;
  block.input = JSON.parse(events.find(e => e.type === 'content_block_delta').delta.partial_json);
  return { ...structuredClone(initial), messages: [...structuredClone(initial.messages),
    { role: 'assistant', content: [block] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: block.id, content: marker, is_error: isError }] }] };
}
function sanitized(value) {
  assert.equal(JSON.stringify(value).includes('SYNTHETIC_PRIVATE_VALUE'), false);
  assert.equal(JSON.stringify(value).includes('Bearer '), false);
  assert.equal(JSON.stringify(value).includes('accessToken'), false);
}
async function until(predicate, timeout = 1500) {
  const started = performance.now();
  while (!predicate()) { assert.ok(performance.now() - started < timeout); await delay(5); }
}
async function backend(modes = ['text']) {
  const sockets = new Set(), requests = [];
  let invalidRequests = 0;
  const server = createServer((req, res) => {
    req.on('error', () => {}); res.on('error', () => {});
    if (req.url !== '/backend-api/codex/responses' || req.method !== 'POST'
      || req.headers.authorization !== 'Bearer synthetic' || req.headers['chatgpt-account-id'] !== 'synthetic'
      || req.headers.version !== '0.153.4' || req.headers['x-api-key'] !== undefined) {
      invalidRequests++; res.writeHead(400); res.end(); return;
    }
    let raw = '';
    req.on('data', chunk => { raw += chunk.toString(); if (Buffer.byteLength(raw) > LIMITS.requestBytes) req.destroy(); });
    req.on('end', () => {
      let doc;
      try { doc = JSON.parse(raw); } catch { invalidRequests++; res.writeHead(400); res.end(); return; }
      requests.push(doc);
      const mode = modes[requests.length - 1] ?? 'unexpected';
      const result = doc.input.at(-1);
      const data = wire(mode === 'tool' ? 'tool' : 'text', requests.length,
        result?.type === 'function_call_output' && JSON.parse(result.output).is_error ? 'DENIED' : marker);
      res.setHeader('Connection', 'close');
      if (mode !== 'missing') res.setHeader('Content-Type', mode === 'empty' ? '' : mode === 'wrong_type' ? 'application/json' : 'text/event-stream');
      if (Number.isInteger(mode)) {
        res.statusCode = mode;
        if (mode >= 300 && mode < 400) res.setHeader('Location', '/must-not-follow');
        res.end('SYNTHETIC_PRIVATE_VALUE'); return;
      }
      if (mode === 'stall_headers') return;
      if (mode === 'stall_body') { res.write(data.subarray(0, 17)); return; }
      if (mode === 'disconnect') { res.write(data.subarray(0, 17)); setTimeout(() => res.destroy(), 10); return; }
      if (mode === 'truncated') { res.setHeader('Content-Length', data.length + 10); res.end(data); return; }
      if (mode === 'oversize') { res.end(Buffer.alloc(LIMITS.responseBytes + 1, 65)); return; }
      if (mode === 'utf8') { res.end(Buffer.from([0xff])); return; }
      if (mode === 'incomplete') { res.end(data.subarray(0, data.lastIndexOf('event: response.completed'))); return; }
      if (mode === 'compressed') { res.setHeader('Content-Encoding', 'gzip'); res.end(data); return; }
      if (mode === 'duplicate_type') { res.setHeader('Content-Type', ['text/event-stream', 'application/json']); res.end(data); return; }
      if (mode === 'malformed') { res.end('SYNTHETIC_PRIVATE_VALUE\n\n'); return; }
      if (mode === 'sequence') {
        res.end(data.toString().replace('"type":"response.created"', '"type":"response.created","sequence_number":7'));
        return;
      }
      res.write(data.subarray(0, 17)); res.end(data.subarray(17));
    });
  });
  server.maxConnections = 4;
  server.on('connection', socket => { sockets.add(socket); socket.on('error', () => {}); socket.once('close', () => sockets.delete(socket)); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return { port: server.address().port, requests, async close() {
    const closed = new Promise(resolve => server.close(resolve));
    const pending = [...sockets].map(socket => new Promise(resolve => socket.once('close', resolve)));
    for (const socket of sockets) socket.destroy();
    await Promise.all([closed, ...pending]);
    assert.equal(sockets.size, 0); assert.equal(invalidRequests, 0);
  } };
}
let passed = 0, failed = 0, closedGateways = 0;
const totals = { gatewayRequests: 0, upstreamRequests: 0 };
async function test(name, action) {
  try { await action(); passed++; }
  catch (error) {
    failed++;
    const classifications = new Set(['COMPLETE', 'DISCONNECTED', 'TIMEOUT', 'REQUEST_TIMEOUT', 'CANCELLED',
      'TOOL_RESULT_TIMEOUT', 'WAIT_TOOL_RESULT', 'CLEANUP_FAILED', 'INVALID_UTF8', 'PROTOCOL_REJECTED',
      'SESSION_TIMEOUT', 'CLOSED_BY_CALLER', 'NONE']);
    const numbers = [error.actual, error.expected].every(v => typeof v === 'number' || typeof v === 'boolean' || classifications.has(v))
      ? { actual: error.actual, expected: error.expected } : {};
    const line = Number(/test-gateway\.mjs:(\d+):/.exec(error.stack ?? '')?.[1] ?? 0);
    process.stderr.write(JSON.stringify({ failure: name, line, ...numbers }) + '\n');
  }
}
async function usingGateway(action, { modes, headerPolicy, limits = {}, transportTimeout = 2000, profile = 'astra-xhigh' } = {}) {
  const upstream = await backend(modes);
  const transport = createLoopbackCodexTransport(upstream.port, { timeoutMs: transportTimeout, profile });
  let gateway;
  try {
    gateway = await startGateway({ transport, headerPolicy, profile,
      limits: { lifetimeMs: 5000, upstreamMs: 2000, toolResultMs: 2000, requestMs: 500, deliveryMs: 500, ...limits } });
    return await action(gateway, upstream, transport);
  } finally {
    try { if (gateway) {
      const result = await gateway.close();
      assert.equal(result.activeJobs, 0); assert.equal(result.activeSockets, 0); assert.equal(result.activeTimers, 0);
      assert.equal(result.activeDeliveries, 0);
      assert.equal(result.busy, false); assert.equal(result.session.timerActive, false); assert.equal(result.session.buffered, false);
      assert.equal(result.transport.activeSockets, 0); assert.equal(result.transport.activeRequests, 0);
      assert.equal(result.transport.credentialWrites, 0); assert.equal(result.transport.retries, 0);
      assert.equal(result.localSessionSecretCleared, true); assert.equal(result.toolExecutions, 0);
      assert.equal(result.counts.internalErrors, 0);
      assert.throws(() => gateway.clientHeaders(), error => error.code === 'GATEWAY_CLOSED');
      sanitized(result); closedGateways++;
      totals.gatewayRequests += result.counts.receivedRequests;
      totals.upstreamRequests += upstream.requests.length;
    } else await transport.close(); }
    finally { await upstream.close(); }
  }
}
function send(gateway, { body = initial, path = '/v1/messages?beta=true', method = 'POST', headers = {},
  auth = true, signal = AbortSignal.timeout(3000), chunked = false } = {}) {
  const bytes = Buffer.isBuffer(body) ? body : Buffer.from(typeof body === 'string' ? body : JSON.stringify(body));
  return new Promise((resolve, reject) => {
    const req = request({ hostname: '127.0.0.1', port: gateway.port, path, method, agent: false, signal,
      headers: { ...(auth ? gateway.clientHeaders() : { 'Content-Type': 'application/json' }),
        ...(chunked ? {} : { 'Content-Length': bytes.length }), ...headers } }, res => {
      let raw = '';
      res.on('error', reject);
      res.on('data', chunk => { raw += chunk.toString(); if (raw.length > 512 * 1024) req.destroy(); });
      res.on('end', () => resolve({ status: res.statusCode, type: res.headers['content-type'], body: raw }));
    });
    req.on('error', reject);
    if (chunked) { req.write(bytes.subarray(0, 17)); req.end(bytes.subarray(17)); } else req.end(bytes);
  });
}
function tcp(gateway, packet, timeout = 1500) {
  return new Promise(resolve => {
    const socket = connect({ host: '127.0.0.1', port: gateway.port });
    let result = '';
    socket.setTimeout(timeout, () => socket.destroy()); socket.on('error', () => {});
    socket.on('data', chunk => { result += chunk.toString(); });
    socket.once('connect', () => socket.write(packet)); socket.once('close', () => resolve(result));
  });
}

await test('http_text_roundtrip', () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway);
  assert.equal(response.status, 200); assert.equal(response.type, 'text/event-stream; charset=utf-8');
  assert.equal(frames(response).find(e => e.type === 'content_block_delta').delta.text, marker);
  assert.equal(frames(response).at(-1).type, 'message_stop');
  const result = await gateway.done;
  assert.equal(result.reason, 'COMPLETE'); assert.equal(result.counts.responses, 1);
  assert.equal(upstream.requests.length, 1); assert.equal(result.transport.requestAttempts, 1);
  assert.equal(result.transport.connectionAttempts, 1); assert.equal(result.transport.synthetic, true);
  assert.equal(upstream.requests[0].model, MODEL); assert.equal(upstream.requests[0].reasoning.effort, EFFORT);
}));
for (const isError of [false, true]) await test(`http_tool_roundtrip_${isError ? 'denied' : 'ok'}`, () => usingGateway(async (gateway, upstream) => {
  const first = await send(gateway);
  assert.equal(first.status, 200); assert.equal(gateway.diagnostics().session.state, 'WAIT_TOOL_RESULT');
  const second = await send(gateway, { body: follow(first, isError) });
  assert.equal(second.status, 200);
  assert.equal(frames(second).find(e => e.type === 'content_block_delta').delta.text, isError ? 'DENIED' : marker);
  const request2 = upstream.requests[1];
  assert.equal(request2.tool_choice, 'none'); assert.equal(request2.parallel_tool_calls, false);
  assert.equal(request2.input.at(-2).id, 'fc_1'); assert.equal(request2.input.at(-2).call_id, 'call_1');
  assert.equal(request2.input.at(-1).call_id, 'call_1');
  assert.deepEqual(JSON.parse(request2.input.at(-1).output), { is_error: isError, content: marker });
  assert.equal((await gateway.done).transport.requestAttempts, 2); assert.equal(upstream.requests.length, 2);
}, { modes: ['tool', 'text'] }));
await test('missing_header_opt_in', () => usingGateway(async gateway => {
  assert.equal((await send(gateway)).status, 200);
  assert.equal((await gateway.done).counts.compatibilityApplied, 1);
}, { modes: ['missing'], headerPolicy: 'codex-missing-content-type' }));
await test('protocol_failure_keeps_its_actual_category', () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway);
  assert.equal(response.status, 502);
  assert.equal(JSON.parse(response.body).error.message, 'SEQUENCE_MISMATCH');
  const result = await gateway.done;
  assert.equal(result.counts.responses, 0); assert.equal(upstream.requests.length, 1);
  assert.equal(result.transport.httpStatus, 200); assert.equal(result.transport.contentTypeState, 'event-stream');
  assert.equal(result.transport.httpComplete, true); assert.equal(result.session.phase, 'sse-framing');
  assert.equal(result.session.eventKind, 'created'); assert.equal(result.session.eventNumber, 1);
}, { modes: ['sequence'] }));

for (const [mode, expected, policy] of [
  ['missing', 'MISSING_CONTENT_TYPE'], ['empty', 'EMPTY_CONTENT_TYPE', 'codex-missing-content-type'],
  ['wrong_type', 'UNSUPPORTED_CONTENT_TYPE'], [401, 'UNAUTHENTICATED'], [403, 'FORBIDDEN'],
  [301, 'REDIRECT_REJECTED'], [302, 'REDIRECT_REJECTED'], [303, 'REDIRECT_REJECTED'],
  [307, 'REDIRECT_REJECTED'], [308, 'REDIRECT_REJECTED'], [421, 'HTTP_ERROR'], [429, 'RATE_LIMITED'],
  ['oversize', 'RESPONSE_TOO_LARGE'], ['utf8', 'INVALID_UTF8'], ['incomplete', 'INCOMPLETE_RESPONSE'],
  ['truncated', 'UPSTREAM_IO_ERROR'], ['disconnect', 'UPSTREAM_IO_ERROR'],
  ['compressed', 'UNSUPPORTED_UPSTREAM_ENCODING'], ['duplicate_type', 'DUPLICATE_UPSTREAM_HEADER'],
  ['malformed', 'UNSUPPORTED_SSE_FIELD']
]) await test(`upstream_${mode}`, () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway);
  assert.equal(response.status, 502);
  assert.equal(JSON.parse(response.body).error.message, expected);
  assert.equal(response.body.includes('message_stop'), false); sanitized(response);
  const result = await gateway.done;
  assert.equal(result.transport.requestAttempts, 1); assert.equal(upstream.requests.length, 1);
  assert.equal(result.counts.responses, 0);
  assert.equal(result.transport.httpStatus, Number.isInteger(mode) ? mode : 200);
  if (mode === 'missing') assert.equal(result.transport.contentTypeState, 'missing');
  if (mode === 'empty') assert.equal(result.transport.contentTypeState, 'empty');
  if (mode === 'duplicate_type') assert.equal(result.transport.contentTypeState, 'multiple');
}, { modes: [mode], headerPolicy: policy }));

for (const [name, options, category] of [
  ['no_auth', { auth: false }, 'LOCAL_SESSION_REQUIRED'],
  ['public_marker', { headers: { Authorization: 'Bearer clauduct-public-local-inspection' } }, 'LOCAL_SESSION_REQUIRED'],
  ['real_token', { headers: { Authorization: 'Bearer SYNTHETIC_PRIVATE_VALUE' } }, 'LOCAL_SESSION_REQUIRED'],
  ['cookie', { headers: { Cookie: 'SYNTHETIC_PRIVATE_VALUE' } }, 'UNEXPECTED_CREDENTIAL_SOURCE'],
  ['api_key', { headers: { 'x-api-key': 'SYNTHETIC_PRIVATE_VALUE' } }, 'UNEXPECTED_CREDENTIAL_SOURCE'],
  ['proxy_auth', { headers: { 'Proxy-Authorization': 'SYNTHETIC_PRIVATE_VALUE' } }, 'UNEXPECTED_CREDENTIAL_SOURCE'],
  ['origin', { headers: { Origin: 'http://example.invalid' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['host', { headers: { Host: 'example.invalid' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['forwarded', { headers: { 'X-Forwarded-For': '127.0.0.1' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['subagent', { headers: { 'x-claude-code-agent-id': 'fixture' } }, 'SUBAGENT_REJECTED'],
  ['beta', { headers: { 'anthropic-beta': 'SYNTHETIC_PRIVATE_VALUE' } }, 'UNSUPPORTED_CLIENT_VERSION_OR_BETA'],
  ['version', { headers: { 'anthropic-version': 'wrong' } }, 'UNSUPPORTED_CLIENT_VERSION_OR_BETA'],
  ['encoding', { headers: { 'Content-Encoding': 'gzip' } }, 'UNSUPPORTED_BODY_ENCODING'],
  ['route', { path: '/elsewhere' }, 'UNSUPPORTED_ROUTE'],
  ['absolute_target', { path: 'http://example.invalid/' }, 'INVALID_TARGET'],
  ['declared_size', { headers: { 'Content-Length': LIMITS.requestBytes + 1 } }, 'INPUT_TOO_LARGE']
]) await test(`client_${name}`, () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway, options);
  assert.equal(JSON.parse(response.body).error.message, category); sanitized(response);
  assert.equal(upstream.requests.length, 0); assert.equal(gateway.diagnostics().transport.requestAttempts, 0);
}));
await test('local_session_isolation', () => usingGateway(async first => usingGateway(async (second, upstream) => {
  const response = await send(second, { headers: first.clientHeaders() });
  assert.equal(response.status, 401); assert.equal(upstream.requests.length, 0);
})));
await test('head_and_count_tokens_no_upstream', () => usingGateway(async (gateway, upstream) => {
  assert.equal((await send(gateway, { path: '/api/hello', method: 'HEAD', body: '', auth: false })).status, 204);
  const response = await send(gateway, { path: '/v1/messages/count_tokens' });
  assert.equal(response.status, 404); assert.equal(response.body.includes('input_tokens'), false);
  assert.equal(upstream.requests.length, 0);
}));
await test('tool_id_mismatch_never_sent', () => usingGateway(async (gateway, upstream) => {
  const next = follow(await send(gateway)); next.messages.at(-1).content[0].tool_use_id = 'wrong';
  const response = await send(gateway, { body: next });
  assert.equal(response.status, 400); assert.equal(JSON.parse(response.body).error.message, 'INVALID_TOOL_RESULT');
  await gateway.done; assert.equal(upstream.requests.length, 1);
}, { modes: ['tool'] }));
await test('changed_session_id_never_sent', () => usingGateway(async (gateway, upstream) => {
  const first = await send(gateway, { headers: { 'x-claude-code-session-id': 'first' } });
  const response = await send(gateway, { body: follow(first), headers: { 'x-claude-code-session-id': 'second' } });
  assert.equal(response.status, 409); assert.equal(upstream.requests.length, 1);
}, { modes: ['tool'] }));
await test('unsupported_body_never_sent', () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway, { body: { ...initial, metadata: 'SYNTHETIC_PRIVATE_VALUE' } });
  assert.equal(response.status, 400); assert.equal(JSON.parse(response.body).error.message, 'UNSUPPORTED_FIELDS');
  await gateway.done; assert.equal(upstream.requests.length, 0); sanitized(response);
}));
await test('explicit_token_limit_is_not_silently_discarded', () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway, { body: { ...initial, max_tokens: 1024 } });
  assert.equal(response.status, 502); assert.equal(JSON.parse(response.body).error.message, 'TOKEN_LIMIT_UNSUPPORTED');
  const result = await gateway.done;
  assert.equal(result.transport.requestAttempts, 0); assert.equal(upstream.requests.length, 0);
}));
await test('concurrency_does_not_start_second_upstream', () => usingGateway(async (gateway, upstream) => {
  const controller = new AbortController();
  const first = send(gateway, { signal: controller.signal }).then(() => false, () => true);
  await until(() => upstream.requests.length === 1);
  const second = await send(gateway); assert.equal(second.status, 409);
  controller.abort(); assert.equal(await first, true);
  const final = await gateway.done; assert.equal(final.reason, 'DISCONNECTED');
  assert.equal(final.counts.clientDisconnects, 1); assert.equal(upstream.requests.length, 1);
}, { modes: ['stall_body'] }));
for (const mode of ['stall_headers', 'stall_body']) await test(`upstream_timeout_${mode}`, () => usingGateway(async (gateway, upstream) => {
  const response = await send(gateway);
  assert.equal(response.status, 502); assert.equal(JSON.parse(response.body).error.message, 'TIMEOUT');
  await gateway.done; assert.equal(upstream.requests.length, 1);
}, { modes: [mode], limits: { upstreamMs: 80 }, transportTimeout: 1000 }));
await test('transport_timeout_propagates', () => usingGateway(async gateway => {
  const response = await send(gateway);
  assert.equal(JSON.parse(response.body).error.message, 'TIMEOUT'); await gateway.done;
}, { modes: ['stall_body'], transportTimeout: 60 }));
await test('tool_result_deadline_closes_session', () => usingGateway(async gateway => {
  assert.equal((await send(gateway)).status, 200);
  assert.equal((await gateway.done).reason, 'TOOL_RESULT_TIMEOUT');
}, { modes: ['tool'], limits: { toolResultMs: 60 } }));
await test('idle_gateway_deadline', () => usingGateway(async gateway => {
  assert.equal((await gateway.done).reason, 'SESSION_TIMEOUT');
}, { limits: { lifetimeMs: 60 } }));
await test('body_deadline', () => usingGateway(async gateway => {
  const auth = gateway.clientHeaders().Authorization;
  await tcp(gateway, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\nAuthorization: ${auth}\r\nanthropic-version: 2023-06-01\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{`);
  const result = await gateway.done;
  if (result.reason !== 'REQUEST_TIMEOUT') process.stderr.write(JSON.stringify({ diagnostic: 'body_deadline', counts: result.counts }) + '\n');
  assert.equal(result.reason, 'REQUEST_TIMEOUT');
}, { limits: { requestMs: 60 } }));
await test('request_budget_no_upstream', () => usingGateway(async gateway => {
  await send(gateway, { auth: false }); await send(gateway, { auth: false });
  const response = await send(gateway, { auth: false });
  assert.equal(response.status, 429); assert.equal((await gateway.done).reason, 'REQUEST_BUDGET');
}, { limits: { requests: 2 } }));
await test('unknown_transport_error_sanitized', async () => {
  const transport = { async send() { throw new Error('SYNTHETIC_PRIVATE_VALUE'); }, async close() {}, diagnostics: () => ({}) };
  const gateway = await startGateway({ transport, limits: { lifetimeMs: 2000 } });
  try {
    const response = await send(gateway);
    assert.equal(response.status, 502); assert.equal(JSON.parse(response.body).error.message, 'PROTOCOL_REJECTED');
    sanitized(response); await gateway.done;
  } finally { await gateway.close(); }
});
await test('live_version_pin_before_any_send', () => {
  assert.throws(() => createCodexTransport({ credential: {}, clientVersion: 'unreviewed' }), error => error.code === 'CLI_VERSION_CHANGED');
});
await test('loopback_transport_limit_cannot_expand', () => {
  assert.throws(() => createLoopbackCodexTransport(1, { timeoutMs: 45001 }));
  assert.throws(() => createLoopbackCodexTransport(0));
  assert.throws(() => createLoopbackCodexTransport(1, { profile: 'unknown' }), e => e.code === 'INVALID_PROFILE');
  assert.throws(() => createCodexTransport({ credential: {}, clientVersion: '0.153.4', profile: 'unknown' }),
    e => e.code === 'INVALID_PROFILE');
});

for (const profile of ['astra-low', 'luna-low']) await test(`transport_profile_pin_${profile}`, () => usingGateway(async (_gateway, upstream, transport) => {
  const selected = probeProfile(profile), sink = { begin() {}, push() {} }, signal = new AbortController().signal;
  const base = { model: selected.model, reasoning: { effort: 'low' }, stream: true, store: false, input: [] };
  for (const wrong of [{ ...base, model: selected.model === MODEL ? 'gpt-5.6-luna' : MODEL },
    { ...base, reasoning: { effort: 'xhigh' } }]) {
    await assert.rejects(transport.send(JSON.stringify(wrong), sink, signal), e => e.code === 'INVALID_UPSTREAM_REQUEST');
  }
  const controller = new AbortController(); controller.abort();
  await assert.rejects(transport.send(JSON.stringify(base), sink, controller.signal), e => e.code === 'CANCELLED');
  assert.equal(transport.diagnostics().requestAttempts, 0); assert.equal(upstream.requests.length, 0);
}, { profile }));

const prepared = JSON.stringify({ model: MODEL, reasoning: { effort: EFFORT }, stream: true, store: false, input: [] });
await test('transport_request_budget_measured', () => usingGateway(async (_gateway, upstream, transport) => {
  const sink = { begin() {}, push() {} }, signal = new AbortController().signal;
  await transport.send(prepared, sink, signal); await transport.send(prepared, sink, signal);
  await assert.rejects(transport.send(prepared, sink, signal), error => error.code === 'REQUEST_BUDGET');
  assert.equal(upstream.requests.length, 2); assert.equal(transport.diagnostics().requestAttempts, 2);
}, { modes: ['text', 'text'] }));
await test('transport_cancelled_before_send', () => usingGateway(async (_gateway, upstream, transport) => {
  const controller = new AbortController(); controller.abort();
  await assert.rejects(transport.send(prepared, { begin() {}, push() {} }, controller.signal), error => error.code === 'CANCELLED');
  assert.equal(transport.diagnostics().requestAttempts, 0); assert.equal(upstream.requests.length, 0);
}));
await test('client_disconnect_before_upstream_headers', () => usingGateway(async (gateway, upstream) => {
  const controller = new AbortController();
  const pending = send(gateway, { signal: controller.signal }).then(() => false, () => true);
  await until(() => upstream.requests.length === 1);
  controller.abort(); assert.equal(await pending, true);
  assert.equal((await gateway.done).reason, 'DISCONNECTED');
  assert.equal(upstream.requests.length, 1);
}, { modes: ['stall_headers'] }));
await test('duplicate_auth_header_rejected', () => usingGateway(async (gateway, upstream) => {
  const header = gateway.clientHeaders().Authorization;
  const response = await tcp(gateway, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\nAuthorization: ${header}\r\nAuthorization: SYNTHETIC_PRIVATE_VALUE\r\nContent-Length: 0\r\n\r\n`);
  assert.equal(response.includes('DUPLICATE_HEADER'), true); assert.equal(upstream.requests.length, 0);
  assert.equal(response.includes('SYNTHETIC_PRIVATE_VALUE'), false);
}));
await test('http_framing_conflict_rejected', () => usingGateway(async (gateway, upstream) => {
  await tcp(gateway, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n`);
  assert.ok(gateway.diagnostics().counts.malformedHttp > 0); assert.equal(upstream.requests.length, 0);
}));
await test('expect_rejected', () => usingGateway(async (gateway, upstream) => {
  const response = await tcp(gateway, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\nExpect: 100-continue\r\nContent-Length: 1\r\n\r\n`);
  assert.equal(response.includes('417'), true); assert.equal(response.includes('100 Continue'), false);
  assert.equal((await gateway.done).reason, 'EXPECT_REJECTED'); assert.equal(upstream.requests.length, 0);
}));
await test('upgrade_not_proxied', () => usingGateway(async (gateway, upstream) => {
  await tcp(gateway, `GET /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n`);
  assert.equal((await gateway.done).reason, 'UPGRADE_REJECTED'); assert.equal(upstream.requests.length, 0);
}));
await test('pipeline_cannot_start_upstream', () => usingGateway(async (gateway, upstream) => {
  const head = `HEAD /api/hello HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\n\r\n`;
  await tcp(gateway, head + head);
  assert.equal((await gateway.done).reason, 'PIPELINE_REJECTED'); assert.equal(upstream.requests.length, 0);
}));
await test('connection_budget_enforced', () => usingGateway(async gateway => {
  for (let i = 0; i < 3; i++) await tcp(gateway, `HEAD /api/hello HTTP/1.1\r\nHost: 127.0.0.1:${gateway.port}\r\n\r\n`);
  const result = await gateway.done;
  assert.equal(result.reason, 'CONNECTION_BUDGET'); assert.equal(result.counts.connections, 3);
}, { limits: { connections: 2 } }));
await test('chunked_body_limit_enforced', () => usingGateway(async (gateway, upstream) => {
  await send(gateway, { body: ' '.repeat(LIMITS.requestBytes + 1), chunked: true }).catch(() => null);
  assert.equal((await gateway.done).reason, 'INPUT_TOO_LARGE'); assert.equal(upstream.requests.length, 0);
}));

const realTimeouts = [];
await test('writer_resumes_after_backpressure', async () => {
  const stream = new PassThrough({ highWaterMark: 1024 });
  const input = '한글🙂'.repeat(12000), chunks = [];
  try {
    const pending = deliverSse(stream, input, new AbortController().signal);
    assert.equal(stream.writableNeedDrain, true);
    await delay(20);
    stream.on('data', chunk => chunks.push(chunk));
    const result = await pending;
    assert.ok(result.backpressurePauses > 0); assert.equal(Buffer.concat(chunks).toString(), input);
    assert.equal(stream.listenerCount('drain'), 0);
  } finally { stream.destroy(); }
});
await test('writer_cancel_while_backpressured', async () => {
  const stream = new PassThrough({ highWaterMark: 1024 }), controller = new AbortController();
  try {
    const pending = deliverSse(stream, 'x'.repeat(40000), controller.signal);
    controller.abort(); await assert.rejects(pending, error => error.code === 'CANCELLED');
    assert.equal(stream.listenerCount('drain'), 0); assert.equal(stream.listenerCount('finish'), 0);
  } finally { stream.destroy(); }
});
await test('writer_disconnect_while_backpressured', async () => {
  const stream = new PassThrough({ highWaterMark: 1024 });
  const pending = deliverSse(stream, 'x'.repeat(40000), new AbortController().signal);
  stream.destroy(); await assert.rejects(pending, error => error.code === 'DISCONNECTED');
  assert.equal(stream.listenerCount('drain'), 0);
});
await test('default_delivery_deadline', async () => {
  const stream = new PassThrough({ highWaterMark: 1024 }), started = performance.now();
  try {
    await assert.rejects(deliverSse(stream, 'x'.repeat(40000), new AbortController().signal), error => error.code === 'DELIVERY_TIMEOUT');
    const elapsedMs = Math.round(performance.now() - started);
    assert.ok(elapsedMs >= 4980 && elapsedMs < 7500);
    assert.equal(stream.listenerCount('drain'), 0);
    realTimeouts.push({ kind: 'delivery_writer', elapsedMs, category: 'DELIVERY_TIMEOUT' });
  } finally { stream.destroy(); }
});
if (process.argv.includes('--lifetime-timeout')) await test('default_gateway_lifetime', () => usingGateway(async gateway => {
  const started = performance.now();
  const result = await gateway.done, elapsedMs = Math.round(performance.now() - started);
  assert.equal(result.reason, 'SESSION_TIMEOUT'); assert.ok(elapsedMs >= 179980 && elapsedMs < 185000);
  realTimeouts.push({ kind: 'gateway_lifetime', elapsedMs, category: result.reason });
}, { limits: { lifetimeMs: 180000 } }));
if (process.argv.includes('--real-timeouts')) await test('default_gateway_upstream_deadline', () => usingGateway(async (gateway, upstream) => {
  const startedAt = performance.now();
  const response = await send(gateway, { signal: AbortSignal.timeout(50000) });
  const result = await gateway.done, elapsedMs = Math.round(performance.now() - startedAt);
  assert.equal(response.status, 502); assert.equal(result.reason, 'TIMEOUT');
  assert.ok(elapsedMs >= 44980 && elapsedMs < 48000);
  assert.equal(upstream.requests.length, 1);
  realTimeouts.push({ kind: 'gateway_upstream', elapsedMs, category: result.reason });
}, { modes: ['stall_body'], limits: { lifetimeMs: 50000, upstreamMs: 45000 }, transportTimeout: 45000 }));

clearTimeout(suiteTimeout);
process.stdout.write(JSON.stringify({ suite: 'gateway', passed, failed, closedGateways, ...totals, realTimeouts,
  syntheticUpstreamOnly: true, externalRequests: 0, credentialReads: 0, toolExecutions: 0 }) + '\n');
process.exitCode = failed ? 1 : 0;
