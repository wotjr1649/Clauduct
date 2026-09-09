import assert from 'node:assert/strict';
import { request } from 'node:http';
import { connect } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import { inspectRequest, startInspector, FIXTURE_MARKER, INSPECT_LIMITS } from './request-inspector.mjs';
import { ALIAS, readTool } from './adapter.mjs';

const fixture = { model: ALIAS, stream: true, max_tokens: 1024, system: '합성 지침',
  messages: [{ role: 'user', content: 'SYNTHETIC_PRIVATE_VALUE' }],
  tools: [readTool()], tool_choice: { type: 'auto' } };
const raw = JSON.stringify(fixture);
let passed = 0, failed = 0, checks = 0, closedSessions = 0;
async function test(name, action) {
  checks++;
  try { await action(); passed++; }
  catch (e) {
    failed++;
    const code = ['ERR_ASSERTION', 'ERR_ACCESS_DENIED', 'ECONNRESET', 'ABORT_ERR'].includes(e.code) ? e.code : 'TEST_FAILURE';
    const values = [e.actual, e.expected].every(v => typeof v === 'number' || typeof v === 'boolean')
      ? { actual: e.actual, expected: e.expected } : {};
    process.stderr.write(JSON.stringify({ failure: name, code, ...values }) + '\n');
  }
}
function rejected(fn, code) { assert.throws(fn, e => e.code === code && e.message === code); }
function sanitized(value) {
  const serialized = JSON.stringify(value);
  for (const secret of ['SYNTHETIC_PRIVATE_VALUE', 'PRIVATE_TOOL_NAME', 'PRIVATE_FIELD',
    'PRIVATE_SESSION', 'PRIVATE_HEADER', 'private@example.invalid', 'D:\\private']) {
    assert.equal(serialized.includes(secret), false);
  }
}
async function usingProbe(action, options = {}) {
  const probe = await startInspector({ lifetimeMs: 2500, requestMs: 1000, observationMs: 1000, ...options });
  try { return await action(probe); }
  finally {
    const final = await probe.close();
    assert.equal(final.activeSockets, 0); assert.equal(final.activeRequestTimers, 0);
    assert.equal(final.activeBodies, 0);
    sanitized(final); closedSessions++;
  }
}
function send(port, { body = raw, path = '/v1/messages?beta=true', method = 'POST', headers = {}, chunked = false, bearer = false } = {}) {
  return new Promise((resolve, reject) => {
    const bytes = Buffer.from(body);
    const req = request({ hostname: '127.0.0.1', port, path, method, agent: false,
      signal: AbortSignal.timeout(1500), headers: { 'Content-Type': 'application/json',
        ...(bearer ? { Authorization: `Bearer ${FIXTURE_MARKER}` } : { 'x-api-key': FIXTURE_MARKER }),
        'anthropic-version': '2023-06-01',
        ...(chunked ? {} : { 'Content-Length': bytes.length }), ...headers } }, res => {
      const chunks = [];
      res.on('data', b => chunks.push(b));
      res.on('error', reject);
      res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(chunks).toString(), headers: res.headers }));
    });
    req.on('error', reject);
    if (chunked) { req.write(bytes.subarray(0, 11)); req.end(bytes.subarray(11)); }
    else req.end(bytes);
  });
}
function tcp(port, packet, { keepOpen = false, timeoutMs = 1500 } = {}) {
  return new Promise(resolve => {
    const client = connect({ host: '127.0.0.1', port });
    let data = '';
    client.setTimeout(timeoutMs, () => client.destroy());
    client.on('data', chunk => { data += chunk.toString(); });
    client.on('error', () => {});
    client.once('close', () => resolve(data));
    client.once('connect', () => { if (keepOpen) client.write(packet); else client.end(packet); });
  });
}

await test('fixture_shape_and_adapter_reuse', () => {
  const shape = inspectRequest(raw);
  assert.equal(shape.modelMatchesAlias, true); assert.equal(shape.streamTrue, true);
  assert.equal(shape.maxTokens, 1024); assert.equal(shape.readCount, 1);
  assert.equal(shape.fixtureSchemaMatches, true);
  assert.deepEqual(shape.adapter, { accepted: true, category: 'SUPPORTED_FIXTURE' });
  assert.equal(shape.transportTokenLimit, 'explicit-unsupported');
  assert.equal(shape.endConversationCount, 0);
  sanitized(shape);
});
await test('omitted_token_limit_is_not_live_success', () => {
  const doc = structuredClone(fixture); delete doc.max_tokens;
  const shape = inspectRequest(JSON.stringify(doc));
  assert.equal(shape.transportTokenLimit, 'not-requested');
  assert.equal(shape.adapter.accepted, true); sanitized(shape);
});
await test('invalid_explicit_limits_are_not_reported_as_omitted', () => {
  for (const max_tokens of [null, 0, -1, 4097, 262145, 'SYNTHETIC_PRIVATE_VALUE']) {
    const shape = inspectRequest(JSON.stringify({ ...fixture, max_tokens }));
    assert.equal(shape.transportTokenLimit, 'explicit-unsupported');
    assert.equal(shape.adapter.accepted, false); sanitized(shape);
  }
});
await test('end_conversation_is_counted_without_accepting_extra_tools', () => {
  const doc = structuredClone(fixture);
  doc.tools.push({ name: 'EndConversation', description: 'SYNTHETIC_PRIVATE_VALUE', input_schema: {} },
    { name: 'PRIVATE_TOOL_NAME', input_schema: {} });
  const shape = inspectRequest(JSON.stringify(doc));
  assert.equal(shape.toolCount, 3); assert.equal(shape.readCount, 1);
  assert.equal(shape.otherToolCount, 2); assert.equal(shape.endConversationCount, 1);
  assert.deepEqual(shape.adapter, { accepted: false, category: 'UNSUPPORTED_TOOLS' }); sanitized(shape);
});
await test('realistic_shape_rejected_without_silent_drops', () => {
  const doc = structuredClone(fixture);
  doc.max_tokens = 32000;
  doc.system = [{ type: 'text', text: 'SYNTHETIC_PRIVATE_VALUE', cache_control: { type: 'ephemeral' } }];
  doc.metadata = { user_id: 'private@example.invalid' };
  doc.thinking = { type: 'adaptive' };
  doc.tools[0].input_schema = { type: 'object', properties: {
    file_path: { type: 'string', description: 'D:\\private' },
    offset: { type: 'number', description: 'SYNTHETIC_PRIVATE_VALUE' },
    limit: { type: 'number' }, pages: { type: 'string' }
  }, required: ['file_path'] };
  const result = inspectRequest(JSON.stringify(doc));
  assert.equal(result.maxTokens, 32000); assert.equal(result.cacheControlCount, 1);
  assert.equal(result.thinking, 'adaptive'); assert.equal(result.fixtureSchemaMatches, false);
  assert.equal(result.readSchema.properties.offset.type, 'number');
  assert.equal(result.readSchema.properties.pages.present, true);
  assert.equal(result.readSchema.additionalProperties, 'absent');
  assert.deepEqual(result.adapter, { accepted: false, category: 'UNSUPPORTED_FIELDS' });
  sanitized(result);
});
await test('private_keys_values_do_not_escape', () => {
  const doc = structuredClone(fixture);
  doc.PRIVATE_FIELD = 'SYNTHETIC_PRIVATE_VALUE'; doc.model = 'SYNTHETIC_PRIVATE_VALUE';
  doc.tools.push({ name: 'PRIVATE_TOOL_NAME', input_schema: {} });
  doc.tools[0].input_schema.properties.PRIVATE_FIELD = { type: 'SYNTHETIC_PRIVATE_VALUE' };
  const shape = inspectRequest(JSON.stringify(doc));
  assert.equal(shape.unknownFieldCount, 1); assert.equal(shape.otherToolCount, 1);
  assert.equal(shape.readSchema.unknownPropertyCount, 1); sanitized(shape);
});
await test('prototype_keys_are_data', () => {
  const shape = inspectRequest('{"__proto__":{"PRIVATE_FIELD":"SYNTHETIC_PRIVATE_VALUE"},"constructor":{}}');
  assert.equal(shape.unknownFieldCount, 2); assert.equal({}.PRIVATE_FIELD, undefined); sanitized(shape);
});
await test('duplicate_read_schema_ambiguous', () => {
  const doc = structuredClone(fixture); doc.tools.push(readTool());
  const result = inspectRequest(JSON.stringify(doc));
  assert.equal(result.readCount, 2); assert.equal(result.readSchema, null);
  assert.equal(result.fixtureSchemaMatches, false);
});
await test('malformed_schema_no_exception_leak', () => {
  const doc = structuredClone(fixture); doc.tools[0].input_schema = { properties: [] };
  const result = inspectRequest(JSON.stringify(doc));
  assert.equal(result.readSchema.rootIsObject, false); sanitized(result);
});
await test('unknown_block_types_counted', () => {
  const doc = structuredClone(fixture);
  doc.messages = [{ role: 'SYNTHETIC_PRIVATE_VALUE', content: [{ type: 'PRIVATE_FIELD' }, { type: 'image' }] }];
  const shape = inspectRequest(JSON.stringify(doc));
  assert.equal(shape.roles.other, 1); assert.equal(shape.blocks.other, 1);
  assert.equal(shape.blocks.image, 1); sanitized(shape);
});
await test('max_tokens_untrusted_number', () => {
  const doc = { ...fixture, max_tokens: 'SYNTHETIC_PRIVATE_VALUE' };
  assert.equal(inspectRequest(JSON.stringify(doc)).maxTokens, null);
});
await test('role_order_uses_only_fixed_names_and_is_bounded', () => {
  const doc = structuredClone(fixture);
  doc.messages = ['system', 'developer', 'user', 'assistant', 'tool', 'function', 'PRIVATE_FIELD', undefined, 'user']
    .map(role => ({ role, content: 'SYNTHETIC_PRIVATE_VALUE' }));
  const shape = inspectRequest(JSON.stringify(doc));
  assert.deepEqual(shape.messageRoles, ['system', 'developer', 'user', 'assistant', 'tool', 'function', 'other', 'absent']);
  assert.equal(shape.messageRolesTruncated, true); sanitized(shape);
});
await test('observed_options_classified_without_accepting_them', () => {
  const doc = structuredClone(fixture);
  Object.assign(doc, { max_tokens: 64000, thinking: { type: 'adaptive' },
    metadata: { user_id: 'private@example.invalid' }, output_config: { effort: 'high' },
    context_management: { edits: [{ type: 'clear_thinking_20251015', keep: 'all',
      trigger: { type: 'PRIVATE_FIELD', value: 12 } }] } });
  const shape = inspectRequest(JSON.stringify(doc));
  assert.equal(shape.optionDetails.effort, 'high');
  assert.equal(shape.optionDetails.metadata.fields.user_id, 'string');
  assert.equal(shape.optionDetails.contextEdits[0].fields.trigger, 'object');
  assert.equal(shape.optionDetails.contextEdits[0].strategy, 'clear_thinking_20251015');
  assert.equal(shape.optionDetails.contextEdits[0].keepAll, true);
  assert.equal(shape.optionDetails.contextEdits[0].thresholds.trigger.type, 'other');
  assert.equal(shape.optionDetails.thinking.unknownFieldCount, 0);
  assert.equal(shape.adapter.category, 'UNSUPPORTED_FIELDS'); sanitized(shape);
});
await test('schema_numeric_constraints_and_root_keywords', () => {
  const doc = structuredClone(fixture), schema = doc.tools[0].input_schema;
  schema.$schema = 'SYNTHETIC_PRIVATE_VALUE';
  schema.properties.offset = { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER };
  schema.properties.limit = { type: 'integer', minimum: 'SYNTHETIC_PRIVATE_VALUE', default: -1, PRIVATE_FIELD: 1 };
  const shape = inspectRequest(JSON.stringify(doc));
  assert.deepEqual(shape.readSchema.properties.offset.constraints.minimum, { kind: 'number', value: 1 });
  assert.equal(shape.readSchema.properties.offset.constraints.maximum.value, Number.MAX_SAFE_INTEGER);
  assert.equal(shape.readSchema.properties.offset.unknownKeywordCount, 0);
  assert.equal(shape.readSchema.properties.limit.unknownKeywordCount, 1);
  assert.deepEqual(shape.readSchema.properties.limit.constraints.minimum, { kind: 'string', value: null });
  assert.equal(shape.readSchema.properties.limit.constraints.default.value, null);
  assert.equal(shape.readSchema.extraKeywords.fields.$schema, 'string'); sanitized(shape);
});
await test('malformed_options_and_unknown_effort_stay_private', () => {
  const shape = inspectRequest(JSON.stringify({ ...fixture, thinking: null,
    metadata: ['SYNTHETIC_PRIVATE_VALUE'], output_config: { effort: 'SYNTHETIC_PRIVATE_VALUE', PRIVATE_FIELD: 'PRIVATE_HEADER' },
    context_management: { edits: Array(10).fill('SYNTHETIC_PRIVATE_VALUE') } }));
  assert.equal(shape.optionDetails.thinking.kind, 'null');
  assert.equal(shape.optionDetails.metadata.kind, 'array');
  assert.equal(shape.optionDetails.effort, 'other');
  assert.equal(shape.optionDetails.outputConfig.unknownFieldCount, 1);
  assert.equal(shape.optionDetails.contextEdits.length, 8);
  assert.equal(shape.optionDetails.contextEditsTruncated, true); sanitized(shape);
});
await test('invalid_json_fixed_error', () => rejected(() => inspectRequest('SYNTHETIC_PRIVATE_VALUE'), 'INVALID_JSON'));
await test('array_not_request', () => rejected(() => inspectRequest('[]'), 'INVALID_REQUEST'));
await test('body_limit_over', () => rejected(() => inspectRequest(' '.repeat(INSPECT_LIMITS.bodyBytes + 1)), 'BODY_TOO_LARGE'));
await test('deep_tree_bounded', () => rejected(() => inspectRequest('{"x":'.repeat(34) + '0' + '}'.repeat(34)), 'STRUCTURE_LIMIT'));
await test('wide_tree_bounded', () => rejected(() => inspectRequest(JSON.stringify({ x: Array(20001).fill(0) })), 'STRUCTURE_LIMIT'));
await test('body_limit_exact', () => {
  const shape = inspectRequest(raw + ' '.repeat(INSPECT_LIMITS.bodyBytes - Buffer.byteLength(raw)));
  assert.equal(shape.adapter.category, 'INPUT_TOO_LARGE');
});

await test('fixed_http_message', () => usingProbe(async p => {
  const response = await send(p.port);
  assert.equal(response.status, 400);
  assert.equal(JSON.parse(response.body).error.message, 'CLAUDUCT_INSPECTION_COMPLETE');
  assert.equal(response.headers['cache-control'], 'no-store');
  assert.equal(response.headers.connection, 'close');
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
  assert.equal(p.diagnostics().counters.acceptedConnections, 1);
  const record = p.diagnostics().records[0];
  assert.equal(record.betaQuery, true); assert.equal(record.shape.adapter.accepted, true);
  assert.equal(record.headers.versionMatches, true);
  assert.equal(record.headers.fixtureMarkerSource, 'api_key');
  assert.equal(record.shape.transportTokenLimit, 'explicit-unsupported');
  assert.equal(record.bodyBytes, Buffer.byteLength(raw));
  sanitized(response);
}));
await test('public_bearer_marker', () => usingProbe(async p => {
  const r = await send(p.port, { bearer: true });
  assert.equal(r.status, 400);
  assert.equal(JSON.parse(r.body).error.message, 'CLAUDUCT_INSPECTION_COMPLETE');
  assert.equal(p.diagnostics().records[0].headers.fixtureMarkerSource, 'bearer');
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
  assert.equal(p.diagnostics().counters.captured, 1);
  assert.equal(p.diagnostics().authenticatedProductSession, false);
  sanitized(r); sanitized(p.diagnostics());
}));
for (const [name, headers, category] of [
  ['both_markers', { 'x-api-key': FIXTURE_MARKER }, 'AMBIGUOUS_FIXTURE_MARKER'],
  ['real_api_key', { 'x-api-key': 'SYNTHETIC_PRIVATE_VALUE' }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['real_bearer', { Authorization: 'Bearer SYNTHETIC_PRIVATE_VALUE' }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['wrong_scheme', { Authorization: `Basic ${FIXTURE_MARKER}` }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['longer_marker', { Authorization: `Bearer ${FIXTURE_MARKER}-extra` }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['cookie', { Cookie: 'SYNTHETIC_PRIVATE_VALUE' }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['proxy_auth', { 'Proxy-Authorization': 'SYNTHETIC_PRIVATE_VALUE' }, 'REAL_CREDENTIALS_NOT_ACCEPTED']
]) await test(`bearer_${name}`, () => usingProbe(async p => {
  const r = await send(p.port, { bearer: true, headers });
  assert.equal(JSON.parse(r.body).error.message, category);
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
  assert.equal(p.diagnostics().counters.captured, 0);
  sanitized(r); sanitized(p.diagnostics());
}));
await test('chunked_http_message', () => usingProbe(async p => {
  const r = await send(p.port, { chunked: true, path: '/v1/messages' });
  assert.equal(r.status, 400); assert.equal(p.diagnostics().counters.captured, 1);
}));
for (const [name, query, expected] of [
  ['after_other_parameter', '?x=1&beta=true', true],
  ['encoded_value', '?beta=%74rue', true],
  ['longer_value', '?beta=trueevil', false],
  ['embedded_value', '?x=beta=true', false],
  ['conflicting_duplicate', '?beta=true&beta=false', false],
  ['repeated_value', '?beta=true&beta=true', false],
  ['absent', '', false]
]) await test(`beta_query_${name}`, () => usingProbe(async p => {
  await send(p.port, { path: `/v1/messages${query}` });
  assert.equal(p.diagnostics().records[0].betaQuery, expected);
  assert.equal(p.diagnostics().counters.captured, 1);
  sanitized(p.diagnostics());
}));
await test('head_hello', () => usingProbe(async p => {
  const r = await send(p.port, { method: 'HEAD', path: '/api/hello', body: '' });
  assert.equal(r.status, 204); assert.equal(r.body, '');
  assert.equal(p.diagnostics().counters.hello, 1); assert.equal(p.diagnostics().counters.captured, 0);
}));
await test('count_tokens_not_fabricated', () => usingProbe(async p => {
  const r = await send(p.port, { path: '/v1/messages/count_tokens' });
  assert.equal(r.status, 404); assert.equal(r.body.includes('input_tokens'), false);
  assert.equal(p.diagnostics().counters.countTokens, 1);
  assert.equal(p.diagnostics().counters.captured, 0);
  assert.equal(p.diagnostics().records[0].route, 'count_tokens');
}));
await test('observe_fallback_and_repeated_messages', () => usingProbe(async p => {
  await send(p.port, { path: '/v1/messages/count_tokens' });
  await send(p.port); await send(p.port);
  assert.deepEqual(p.diagnostics().records.map(r => r.route), ['count_tokens', 'messages', 'messages']);
  assert.equal(p.diagnostics().counters.acceptedConnections, 3);
  assert.equal(p.diagnostics().upstreamRequests, 0);
}));

const cases = [
  ['bad_host', { headers: { Host: 'example.invalid' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['origin', { headers: { Origin: 'http://example.invalid' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['null_origin', { headers: { Origin: 'null' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['browser', { headers: { 'Sec-Fetch-Site': 'same-origin' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['forwarded', { headers: { 'X-Forwarded-For': 'PRIVATE_HEADER' } }, 'LOCAL_BOUNDARY_REJECTED'],
  ['bearer_rejected', { headers: { Authorization: 'Bearer SYNTHETIC_PRIVATE_VALUE' } }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['cookie_rejected', { headers: { Cookie: 'SYNTHETIC_PRIVATE_VALUE' } }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['api_key_rejected', { headers: { 'x-api-key': 'SYNTHETIC_PRIVATE_VALUE' } }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['proxy_auth_rejected', { headers: { 'Proxy-Authorization': 'SYNTHETIC_PRIVATE_VALUE' } }, 'REAL_CREDENTIALS_NOT_ACCEPTED'],
  ['subagent', { headers: { 'x-claude-code-agent-id': 'PRIVATE_SESSION' } }, 'SUBAGENT_REJECTED'],
  ['parent_agent', { headers: { 'x-claude-code-parent-agent-id': 'PRIVATE_SESSION' } }, 'SUBAGENT_REJECTED'],
  ['wrong_type', { headers: { 'Content-Type': 'text/plain' } }, 'UNSUPPORTED_BODY_ENCODING'],
  ['compressed', { headers: { 'Content-Encoding': 'gzip' } }, 'UNSUPPORTED_BODY_ENCODING'],
  ['unknown_route', { path: '/PRIVATE_FIELD' }, 'UNSUPPORTED_ROUTE'],
  ['wrong_method', { method: 'PUT' }, 'UNSUPPORTED_ROUTE'],
  ['absolute_target', { path: 'http://example.invalid/v1/messages' }, 'INVALID_TARGET'],
  ['double_slash_target', { path: '//example.invalid/v1/messages' }, 'INVALID_TARGET'],
  ['invalid_body', { body: 'SYNTHETIC_PRIVATE_VALUE' }, 'INVALID_JSON'],
  ['invalid_utf8', { body: Buffer.from([0xff]) }, 'INVALID_UTF8'],
  ['truncated_utf8', { body: Buffer.from([0xe3, 0x81]) }, 'INVALID_UTF8'],
  ['declared_too_large', { body: 'x', headers: { 'Content-Length': INSPECT_LIMITS.bodyBytes + 1 } }, 'BODY_TOO_LARGE'],
  ['chunked_too_large', { body: 'x'.repeat(INSPECT_LIMITS.bodyBytes + 1), chunked: true }, 'BODY_TOO_LARGE']
];
for (const [name, options, code] of cases) await test(name, () => usingProbe(async p => {
  const r = await send(p.port, options);
  assert.equal(JSON.parse(r.body).error.message, code);
  assert.equal(p.diagnostics().counters.captured, 0);
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
  assert.equal(p.diagnostics().counters.acceptedConnections, 1); sanitized(r);
}));
await test('unknown_beta_value_not_exposed', () => usingProbe(async p => {
  await send(p.port, { headers: { 'anthropic-beta': 'PRIVATE_HEADER' } });
  assert.equal(p.diagnostics().records[0].headers.betaPresent, true); sanitized(p.diagnostics());
}));
await test('known_beta_flags_are_diagnostics_not_feature_acceptance', () => usingProbe(async p => {
  const response = await send(p.port, { headers: {
    'anthropic-beta': 'context-management-2025-06-27, PRIVATE_HEADER, interleaved-thinking-2025-05-14' } });
  const headers = p.diagnostics().records[0].headers;
  assert.equal(headers.betaCapabilities['context-management-2025-06-27'], true);
  assert.equal(headers.betaCapabilities['interleaved-thinking-2025-05-14'], true);
  assert.equal(headers.betaCapabilities['compact-2026-01-12'], false);
  assert.equal(headers.unknownBetaCount, 1);
  assert.equal(response.status, 400); sanitized(p.diagnostics());
}));
await test('body_limit_exact_http', () => usingProbe(async p => {
  const body = raw + ' '.repeat(INSPECT_LIMITS.bodyBytes - Buffer.byteLength(raw));
  const r = await send(p.port, { body });
  assert.equal(r.status, 400); assert.equal(p.diagnostics().counters.captured, 1);
  assert.equal(p.diagnostics().records[0].shape.adapter.category, 'INPUT_TOO_LARGE');
}));
await test('session_id_binding', () => usingProbe(async p => {
  await send(p.port, { headers: { 'x-claude-code-session-id': 'PRIVATE_SESSION' } });
  const r = await send(p.port, { headers: { 'x-claude-code-session-id': 'other-session' } });
  assert.equal(JSON.parse(r.body).error.message, 'SESSION_MISMATCH');
  assert.equal(p.diagnostics().counters.captured, 1); sanitized(p.diagnostics());
}));
await test('session_id_cannot_disappear', () => usingProbe(async p => {
  await send(p.port, { headers: { 'x-claude-code-session-id': 'PRIVATE_SESSION' } });
  const r = await send(p.port);
  assert.equal(JSON.parse(r.body).error.message, 'SESSION_MISMATCH');
}));
await test('capture_budget', () => usingProbe(async p => {
  await send(p.port); await send(p.port);
  const r = await send(p.port);
  assert.equal(JSON.parse(r.body).error.message, 'CAPTURE_BUDGET');
  const done = await p.done;
  assert.equal(done.closeReason, 'CAPTURE_BUDGET'); assert.equal(done.counters.captured, 2);
}));
await test('request_budget', () => usingProbe(async p => {
  for (let i = 0; i < INSPECT_LIMITS.requests; i++) await send(p.port, { method: 'GET', path: '/', body: '' });
  const r = await send(p.port, { method: 'GET', path: '/', body: '' });
  assert.equal(JSON.parse(r.body).error.message, 'REQUEST_BUDGET');
  assert.equal((await p.done).counters.receivedRequests, 9);
}));
await test('duplicate_sensitive_header', () => usingProbe(async p => {
  const packet = `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nHost: example.invalid\r\nContent-Length: 0\r\n\r\n`;
  const r = await tcp(p.port, packet);
  assert.equal(r.includes('DUPLICATE_HEADER'), true); assert.equal(p.diagnostics().counters.captured, 0);
}));
await test('conflicting_http_framing', () => usingProbe(async p => {
  const r = await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n`);
  assert.equal(r, ''); assert.ok(p.diagnostics().counters.malformedHttp >= 1);
}));
await test('oversized_http_headers', () => usingProbe(async p => {
  await tcp(p.port, `GET / HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nX-Large: ${'x'.repeat(INSPECT_LIMITS.headerBytes)}\r\n\r\n`);
  assert.ok(p.diagnostics().counters.malformedHttp >= 1);
}));
await test('expect_continue_rejected', () => usingProbe(async p => {
  const r = await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nExpect: 100-continue\r\nContent-Length: 1\r\n\r\n`, { keepOpen: true });
  assert.equal(r.includes('417'), true); assert.equal(r.includes('100 Continue'), false);
}));
await test('truncated_body_cleanup', () => usingProbe(async p => {
  await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nx-api-key: ${FIXTURE_MARKER}\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{`);
  await delay(10);
  assert.equal(p.diagnostics().counters.captured, 0); assert.equal(p.diagnostics().counters.truncated, 1);
}));
await test('request_timeout_cleanup', () => usingProbe(async p => {
  await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nx-api-key: ${FIXTURE_MARKER}\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{`, { keepOpen: true });
  assert.ok(p.diagnostics().counters.timeouts >= 1); assert.equal(p.diagnostics().counters.captured, 0);
}, { requestMs: 60 }));
await test('header_timeout_cleanup', () => usingProbe(async p => {
  await tcp(p.port, 'POST /v1/messages HTTP/1.1\r\n', { keepOpen: true });
  assert.equal(p.diagnostics().counters.captured, 0);
  assert.ok(p.diagnostics().counters.timeouts + p.diagnostics().counters.malformedHttp >= 1);
}, { requestMs: 60 }));
await test('lifetime_closes_idle_socket', () => usingProbe(async p => {
  await tcp(p.port, '', { keepOpen: true });
  const r = await p.done; assert.equal(r.closeReason, 'SESSION_TIMEOUT');
  assert.equal(r.activeSockets, 0);
}, { lifetimeMs: 60 }));
await test('observation_window_ends', () => usingProbe(async p => {
  await send(p.port); const r = await p.done;
  assert.equal(r.closeReason, 'OBSERVATION_WINDOW_ENDED'); assert.equal(r.counters.captured, 1);
}, { observationMs: 60 }));
await test('diagnostics_return_isolated', () => usingProbe(async p => {
  await send(p.port); const d = p.diagnostics(); d.records[0].shape.readCount = 99;
  assert.equal(p.diagnostics().records[0].shape.readCount, 1);
}));
await test('limits_cannot_expand', async () => {
  await assert.rejects(startInspector({ lifetimeMs: 60001 }), e => e.code === 'INVALID_LIMIT');
  await assert.rejects(startInspector({ requestMs: 0 }), e => e.code === 'INVALID_LIMIT');
});
await test('missing_fixture_marker', () => usingProbe(async p => {
  const r = await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nContent-Length: 0\r\n\r\n`);
  assert.equal(r.includes('FIXTURE_MARKER_REQUIRED'), true);
}));
await test('unknown_expectation_rejected', () => usingProbe(async p => {
  const r = await tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nExpect: private\r\nContent-Length: 0\r\n\r\n`);
  assert.equal(r.includes('EXPECT_REJECTED'), true);
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
}));
await test('connect_not_proxy', () => usingProbe(async p => {
  const r = await tcp(p.port, `CONNECT example.invalid:443 HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\n\r\n`);
  assert.equal(r, ''); assert.equal(p.diagnostics().counters.rejected, 1);
  assert.equal(p.diagnostics().counters.receivedRequests, 1);
}));
await test('websocket_upgrade_rejected', () => usingProbe(async p => {
  const r = await tcp(p.port, `GET /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n`);
  assert.equal(r, ''); assert.equal(p.diagnostics().counters.rejected, 1);
}));
await test('connection_budget', () => usingProbe(async p => {
  for (let i = 0; i <= INSPECT_LIMITS.connections; i++) await tcp(p.port, '');
  const r = await p.done;
  assert.equal(r.closeReason, 'CONNECTION_BUDGET');
  assert.equal(r.counters.acceptedConnections, 17);
}));
await test('concurrent_connections_bounded', () => usingProbe(async p => {
  await Promise.all(Array.from({ length: 5 }, () => tcp(p.port, '', { keepOpen: true })));
  const r = await p.done;
  assert.equal(r.closeReason, 'CONCURRENCY_LIMIT');
  assert.equal(r.counters.droppedConnections, 1);
}));
await test('concurrent_bodies_rejected_and_cancelled', () => usingProbe(async p => {
  const slow = tcp(p.port, `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nx-api-key: ${FIXTURE_MARKER}\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{`, { keepOpen: true });
  await delay(20);
  const r = await send(p.port);
  assert.equal(JSON.parse(r.body).error.message, 'CONCURRENT_REQUEST');
  assert.equal(p.diagnostics().activeBodies, 1);
  await p.close(); await slow;
}));
await test('pipelined_second_message_not_captured', () => usingProbe(async p => {
  const packet = `HEAD /api/hello HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\n\r\n`
    + `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nx-api-key: ${FIXTURE_MARKER}\r\nContent-Type: application/json\r\nContent-Length: ${Buffer.byteLength(raw)}\r\n\r\n${raw}`;
  await tcp(p.port, packet);
  assert.equal(p.diagnostics().counters.acceptedConnections, 1);
  assert.equal(p.diagnostics().counters.captured, 0);
  assert.ok(p.diagnostics().counters.receivedRequests <= 2);
}));
await test('closed_session_not_reusable', () => usingProbe(async p => {
  await p.close();
  await assert.rejects(send(p.port), e => e.code === 'ECONNREFUSED');
}));

const realTimeouts = [];
if (process.argv.includes('--real-timeouts')) {
  // Real defaults, no clock substitution. Run independent loopback sessions concurrently.
  await Promise.all(['lifetime', 'observation', 'body', 'headers'].map(kind => test(`default_timeout_${kind}`, async () => {
    const p = await startInspector();
    const startedAt = performance.now();
    const watchdog = setTimeout(() => { void p.close(); }, 65000);
    try {
      if (kind === 'observation') await send(p.port);
      else if (kind === 'body' || kind === 'headers') {
        const packet = kind === 'headers' ? 'POST /v1/messages HTTP/1.1\r\n'
          : `POST /v1/messages HTTP/1.1\r\nHost: 127.0.0.1:${p.port}\r\nx-api-key: ${FIXTURE_MARKER}\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{`;
        await tcp(p.port, packet, { keepOpen: true, timeoutMs: 8000 });
        const counters = p.diagnostics().counters;
        assert.ok(counters.timeouts + counters.malformedHttp >= 1);
        assert.equal(counters.captured, 0);
        await p.close();
      }
      const final = await p.done;
      const elapsedMs = Math.round(performance.now() - startedAt);
      const expectedMs = kind === 'lifetime' ? 60000 : 5000;
      assert.ok(elapsedMs >= expectedMs - 20 && elapsedMs < expectedMs + 3000);
      assert.equal(final.closeReason, kind === 'lifetime' ? 'SESSION_TIMEOUT'
        : kind === 'observation' ? 'OBSERVATION_WINDOW_ENDED' : 'CLOSED_BY_CALLER');
      realTimeouts.push({ kind, elapsedMs, closeReason: final.closeReason });
    } finally {
      clearTimeout(watchdog);
      const final = await p.close();
      assert.equal(final.activeSockets, 0); assert.equal(final.activeRequestTimers, 0);
      assert.equal(final.activeBodies, 0); sanitized(final); closedSessions++;
    }
  })));
}
process.stdout.write(JSON.stringify({ suite: 'request-inspector', checks, passed, failed, closedSessions, realTimeouts,
  syntheticClientsOnly: true, externalRequests: 0, credentialReads: 0, toolExecutions: 0 }) + '\n');
process.exitCode = failed ? 1 : 0;
