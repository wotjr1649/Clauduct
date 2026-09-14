import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { performance } from 'node:perf_hooks';
import { MODELS, ROLE_MODELS, selectModel, CONTEXT_POLICY } from './models.mjs';
import { prepareNative, nativeResponse, createNativeResponse, searchEnvelope } from './native-protocol.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { bindingFrom, registerBinding, contextFromEnvironment } from './agent-route.mjs';
import { classifyBetaNames } from './scan-native-features.mjs';
import { interactiveLaunch, launchOptions, runInteractive } from './clauduct.mjs';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { NATIVE_BETAS, betaFailure, judgedBetas } from './native-beta.mjs';

const tool = name => ({ name, description: 'Synthetic tool', input_schema: { type: 'object',
  properties: { value: { type: 'string' } }, required: ['value'], additionalProperties: false } });
function doc(model = 'astra') { return { model, max_tokens: 10000, stream: true, system: 'SYNTHETIC_SYSTEM',
  messages: [{ role: 'user', content: 'SYNTHETIC_PROMPT' }], tools: [tool('Read'), tool('Bash'), tool('Agent'), tool('mcp__test__read')] }; }
function events(prepared, types = ['text'], suffix = '') {
  const result = [{ type: 'response.created', response: { id: 'resp_' + suffix, status: 'in_progress' } }];
  const output = [];
  for (const [index, type] of types.entries()) {
    let item;
    if (type === 'reasoning') item = { type, id: `rs_${index}`, summary: [], encrypted_content: 'SYNTHETIC_OPAQUE' };
    else if (type === 'text') item = { type: 'message', id: `msg_${index}`, role: 'assistant', status: 'completed', phase: 'final_answer',
      content: [{ type: 'output_text', text: 'SYNTHETIC_REPLY', annotations: [] }] };
    else item = { type: 'function_call', id: `fc_${index}`, call_id: `call_${index}`, name: type,
      arguments: JSON.stringify({ value: 'SYNTHETIC_VALUE' }), status: 'completed' };
    const first = type === 'reasoning' ? item : item.type === 'message' ? { ...item, content: [], status: 'in_progress' }
      : { ...item, arguments: '', status: 'in_progress' };
    result.push({ type: 'response.output_item.added', output_index: index, item: first });
    if (type !== 'reasoning') {
      const functionCall = item.type === 'function_call', prefix = functionCall ? 'response.function_call_arguments' : 'response.output_text';
      const value = functionCall ? item.arguments : item.content[0].text;
      const position = { output_index: index, item_id: item.id, ...(functionCall ? {} : { content_index: 0 }) };
      result.push({ type: prefix + '.delta', ...position, delta: value },
        { type: prefix + '.done', ...position, ...(functionCall ? { arguments: value } : { text: value }) });
    }
    result.push({ type: 'response.output_item.done', output_index: index, item }); output.push(item);
  }
  result.push({ type: 'response.completed', response: { id: 'resp_' + suffix, status: 'completed', model: prepared.selected.model,
    reasoning: { effort: prepared.selected.effort }, output,
    usage: { input_tokens: 30, output_tokens: 5, total_tokens: 35, input_tokens_details: { cached_tokens: 20 } } } });
  return result;
}
function wire(body, types = ['text'], suffix = '') {
  return events({ selected: { model: body.model, effort: body.reasoning.effort } }, types, suffix)
    .map((event, sequence_number) => `event: ${event.type}\ndata: ${JSON.stringify({ ...event, sequence_number })}\n\n`).join('');
}
async function post(gateway, body, headers = {}) {
  const raw = JSON.stringify(body);
  return new Promise((done, reject) => {
    const req = request({ hostname: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST', agent: false,
      signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01',
        'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(raw), ...headers } }, res => {
      let text = ''; res.on('data', chunk => { text += chunk; }); res.on('error', reject);
      res.on('end', () => done({ status: res.statusCode, text }));
    }); req.on('error', reject); req.end(raw);
  });
}
async function fixture(action, { fault, onUnregisteredAgent, responseHeaders = [] } = {}) {
  let received = 0, invalid = false;
  const routes = [];
  const server = createServer((req, res) => {
    let raw = ''; req.on('data', chunk => { raw += chunk; }); req.on('end', () => {
      const body = JSON.parse(raw); received++;
      invalid ||= req.headers['anthropic-beta'] !== undefined || req.headers.authorization !== 'Bearer synthetic';
      routes.push({ model: body.model, effort: body.reasoning.effort });
      invalid ||= !Object.values(MODELS).some(item => item.model === body.model) || Object.hasOwn(body, 'max_output_tokens');
      if (fault === 'stall') return;
      if (body.input.some(item => JSON.stringify(item).includes('SYNTHETIC_RATE_LIMIT'))) { res.writeHead(429); res.end('{}'); return; }
      res.writeHead(200, responseHeaders); res.end(wire(body, ['text'], String(received)));
    });
  });
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  const transport = createNativeLoopbackTransport(server.address().port), gateway = await startNativeGateway({ transport, onUnregisteredAgent,
    admissionOptions: { freeBytes: () => 16 * 1024 * 1024 * 1024 } });
  let actionFailure;
  try { await action(gateway, () => received, routes); assert.equal(invalid, false); }
  catch (error) { actionFailure = error; throw error; }
  finally {
    const result = await gateway.close();
    try {
      assert.equal(result.activeJobs, 0); assert.equal(result.activeSockets, 0);
      assert.equal(result.transport.activeRequests, 0); assert.equal(result.registeredAgents, 0);
    } catch (error) {
      error.fixtureCleanup = { jobs: result.activeJobs, sockets: result.activeSockets, deliveries: result.activeDeliveries,
        pendingCloses: result.pendingConnectionCloses, transportRequests: result.transport.activeRequests,
        transportSockets: result.transport.activeSockets, cleanupFailed: result.cleanupFailed,
        recent: result.recentRequests.slice(-4).map(row => ({ success: row.success, category: row.failureCategory,
          admittedMs: row.admittedMs, transportStartedMs: row.transportStartedMs, transportFinishedMs: row.transportFinishedMs,
          firstEventMs: row.firstEventMs, finishedMs: row.finishedMs })),
        initialCode: ['ABORT_ERR', 'ERR_ASSERTION', 'ECONNRESET'].includes(actionFailure?.code) ? actionFailure.code : null };
      throw error;
    } finally { server.closeAllConnections(); await new Promise(done => server.close(done)); }
  }
}
let passed = 0, failed = 0;
const focused = process.argv.includes('--concurrency-only');
const seconds = Number(process.argv[process.argv.indexOf('--soak-seconds') + 1]) || 0;
assert.ok(seconds >= 0 && seconds <= 600);
const watchdog = setTimeout(() => { process.stderr.write('NATIVE_SUITE_TIMEOUT\n'); process.exit(1); }, (seconds + 40) * 1000);
async function test(name, fn) {
  if (focused && name !== '20_concurrent_agents_and_repeated_release') return;
  try { await fn(); passed++; }
  catch (error) {
    failed++;
    const codes = ['ERR_ASSERTION', 'ABORT_ERR', 'ECONNRESET', 'ECONNREFUSED', 'EPIPE', 'ETIMEDOUT'];
    const scalar = value => typeof value === 'boolean' || (typeof value === 'number' && Number.isFinite(value)) ? value : null;
    process.stderr.write(JSON.stringify({ failure: name, code: codes.includes(error?.code) ? error.code : 'OTHER',
      actual: scalar(error?.actual), expected: scalar(error?.expected), fixtureCleanup: error?.fixtureCleanup ?? null,
      fixtureConcurrency: error?.fixtureConcurrency ?? null }) + '\n');
  }
}
for (const [name, selected] of Object.entries(MODELS)) await test('model_' + name, () => {
  assert.deepEqual(prepareNative(doc(name)).selected, selected);
  assert.equal(prepareNative({ ...doc(name), output_config: { effort: 'low' } }).selected.effort, 'low');
  assert.equal(prepareNative({ ...doc(name), output_config: { effort: 'low' } }, { subagent: true }).selected.effort, selected.effort);
});
await test('model_prototype_rejected', () => assert.throws(() => selectModel('__proto__')));
await test('context_policy_covers_every_main_and_role_model_without_claude_identity', () => {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC_TOKEN' }) };
  const source = { CLAUDE_CODE_MAX_CONTEXT_TOKENS: '200000', CLAUDE_CODE_AUTO_COMPACT_WINDOW: '100000',
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '50', CLAUDE_CONFIG_DIR: 'SYNTHETIC_NATIVE_CONFIG' };
  const before = { ...source };
  for (const selected of [...Object.values(MODELS), ...Object.values(ROLE_MODELS)]) {
    const launch = interactiveLaunch(gateway, source, process.cwd(), selected);
    const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
    for (const env of [launch.options.env, settings.env]) {
      assert.equal(env.CLAUDE_CODE_MAX_CONTEXT_TOKENS, '400000');
      assert.equal(env.CLAUDE_CODE_AUTO_COMPACT_WINDOW, '400000');
      const effective = Number(env.CLAUDE_CODE_AUTO_COMPACT_WINDOW) - 20000;
      const trigger = Math.min(Math.floor(effective * (Number(env.CLAUDE_AUTOCOMPACT_PCT_OVERRIDE) / 100)), effective - 13000);
      assert.equal(trigger, 320000);
    }
    assert.ok(settings.modelPicker.options.every(row => !Object.hasOwn(row, 'behavesAs')));
    assert.equal(launch.options.env.CLAUDE_CONFIG_DIR, source.CLAUDE_CONFIG_DIR);
  }
  assert.deepEqual(source, before);
  assert.equal(CONTEXT_POLICY.compactAt, 320000);
});
await test('repeated_unused_response_headers_do_not_break_sse', () => fixture(async gateway => {
  const result = await post(gateway, doc());
  assert.equal(result.status, 200);
  assert.ok(result.text.includes('SYNTHETIC_REPLY'));
  assert.ok(!result.text.includes('SYNTHETIC_COOKIE'));
}, { responseHeaders: ['Set-Cookie', 'a=SYNTHETIC_COOKIE', 'set-cookie', 'b=SYNTHETIC_COOKIE',
  'Cache-Control', 'no-cache', 'cache-control', 'no-store', 'Vary', 'Origin', 'vary', 'Accept-Encoding',
  'Content-Type', 'text/event-stream'] }));
for (const header of ['Content-Type', 'Content-Encoding', 'Content-Length', 'Transfer-Encoding']) {
  await test('duplicate_response_' + header, () => fixture(async gateway => {
    const result = await post(gateway, doc());
    assert.notEqual(result.status, 200);
    assert.ok(!result.text.includes('SYNTHETIC_REPLY'));
    assert.ok(/DUPLICATE_UPSTREAM_HEADER|UPSTREAM_IO_ERROR/.test(result.text));
  }, { responseHeaders: [header, header === 'Content-Type' ? 'text/event-stream' : header === 'Content-Encoding' ? 'identity'
    : header === 'Content-Length' ? '0' : 'chunked', header.toLowerCase(), header === 'Content-Type' ? 'application/json'
    : header === 'Content-Encoding' ? 'gzip' : header === 'Content-Length' ? '1' : 'chunked'] }));
}
await test('native_beta_headers_and_private_unknown', () => {
  assert.equal(betaFailure(NATIVE_BETAS.join(',')), null);
  assert.equal(betaFailure('per-turn-control-2026-07-01,per-turn-control-2026-07-01'), 'INVALID_BETA_HEADER');
  assert.equal(betaFailure(''), 'INVALID_BETA_HEADER');
  // A judged beta is recorded by label, not refused; only a malformed header is refused.
  assert.equal(betaFailure('context-hint-2026-04-09,SYNTHETIC_PRIVATE'), null);
  assert.deepEqual(judgedBetas('context-hint-2026-04-09,SYNTHETIC_PRIVATE'), ['CONTEXT_HINT']);
});
await test('verified_file_review_rejects_no_diff_completion', async () => {
  const gateway = await startNativeGateway({ agentSelection: {
    resolve: async () => ({ route: MODELS.luna, source: 'native-fork', sessionId: 'review_session', review: true }),
    remember: () => {}, begin: () => {}, delivered: () => {} }, transport: {
      send: async body => {
        assert.deepEqual(body.tool_choice, { type: 'function', name: 'Bash' });
        const bash = body.tools.find(tool => tool.name === 'Bash');
        assert.equal(bash.strict, true);
        assert.equal(bash.parameters.properties.command.enum.length, 1);
        assert.equal(bash.parameters.additionalProperties, false);
        return events({ selected: { model: body.model, effort: body.reasoning.effort } });
      }, close: async () => {}, diagnostics: () => ({}) },
    admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
  try {
    await registerBinding({ id: 'review_child', role: 'general-purpose', sessionId: 'review_session', stop: false },
      { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
    const request = doc('luna'); request.messages[0].content = 'Review target: `D:/SYNTHETIC_PROJECT/file.mjs`';
    const result = await post(gateway, request, { 'x-claude-code-session-id': 'review_session', 'x-claude-code-agent-id': 'review_child' });
    assert.match(result.text, /REVIEW_DIFF_REQUIRED/);
    assert.ok(!result.text.includes('event: message_stop'));
    assert.equal(gateway.diagnostics().recentRequests.at(-1).success, false);
    assert.equal(gateway.diagnostics().recentRequests.at(-1).reviewDiffMismatch, 'call-count');
  } finally { await gateway.close(); }
});
await test('beta_fixture_is_independent_of_implementation_list', () => {
  const header = 'claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,'
    + 'effort-2025-11-24,redact-thinking-2026-02-12,prompt-caching-scope-2026-01-05,'
    + 'mid-conversation-system-2026-04-07,thinking-token-count-2026-05-13,tool-search-tool-2025-10-19,oauth-2025-04-20';
  assert.equal(betaFailure(header), null);
  assert.equal(betaFailure('thinking-binding-controls-2026-08-01'), null);
  assert.deepEqual(judgedBetas('thinking-binding-controls-2026-08-01'), ['THINKING_BINDING']);
  assert.deepEqual(judgedBetas('cache-diagnosis-2026-04-07,SYNTHETIC_PRIVATE'), ['CACHE_DIAGNOSIS']);
});
await test('oauth_beta_does_not_replace_local_auth_or_reach_codex', () => fixture(async (gateway, received) => {
  const headers = { 'anthropic-beta': 'claude-code-20250219,oauth-2025-04-20' };
  assert.equal((await post(gateway, doc(), headers)).status, 200);
  assert.equal((await post(gateway, doc(), { ...headers, Authorization: 'Bearer SYNTHETIC_WRONG' })).status, 401);
  assert.equal((await post(gateway, doc(), { ...headers, Authorization: '', 'x-api-key': 'SYNTHETIC_WRONG' })).status, 400);
  assert.equal(received(), 1);
  // A malformed header is still refused before any upstream attempt.
  assert.equal((await post(gateway, doc(), { 'anthropic-beta': 'oauth-2025-04-20,oauth-2025-04-20' })).status, 400);
  assert.equal(received(), 1);
}));
await test('tool_search_result_and_turn_effort', () => {
  const request = doc(); request.tools.push(tool('ToolSearch'));
  request.tools[0].defer_loading = true;
  request.messages.push({ role: 'assistant', content: [{ type: 'tool_use', name: 'ToolSearch', id: 'search1', input: { value: 'Read' } }] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'search1', content: [{ type: 'tool_reference', tool_name: 'Read' }] }] },
    { role: 'system', content: 'SYNTHETIC_TURN', output_config: { effort: 'high' } });
  const prepared = prepareNative(request);
  assert.equal(prepared.selected.effort, 'high');
  assert.equal(prepared.body.tools.some(item => item.name === 'Read'), true);
  assert.equal(prepared.body.input.find(item => item.type === 'function_call_output').output[0].text,
    '{"type":"tool_reference","tool_name":"Read"}');
  request.messages[2].content[0].content[0].tool_name = '../bad';
  assert.throws(() => prepareNative(request));
});
await test('turn_tool_changes_preserve_current_availability', () => {
  const change = (type, name) => ({ role: 'system', content: [{ type, tool: { type: 'tool_reference', name } }] });
  for (const subagent of [false, true]) {
    const options = { turnToolChanges: true, subagent };
    const request = doc(); request.tools[0].defer_loading = true;
    assert.equal(prepareNative(request, options).names.has('Read'), false);
    request.messages.push(change('tool_addition', 'Read'));
    assert.equal(prepareNative(request, options).names.has('Read'), true);
    request.messages.push({ role: 'assistant', content: [{ type: 'tool_use', name: 'Read', id: 'history_read', input: {} }] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'history_read', content: 'SYNTHETIC' }] }, change('tool_removal', 'Read'));
    assert.equal(prepareNative(request, options).names.has('Read'), false);
    request.tool_choice = { type: 'tool', name: 'Read' };
    assert.throws(() => prepareNative(request, options)); delete request.tool_choice;
    request.messages.push(change('tool_addition', 'Read'));
    const prepared = prepareNative(request, options);
    assert.equal(prepared.names.has('Read'), true);
    assert.equal(prepared.body.input.filter(x => x.type === 'function_call_output').length, 1);
    assert.throws(() => prepareNative(request));
    request.messages.push(change('tool_addition', 'undeclared'));
    assert.throws(() => prepareNative(request, options)); request.messages.pop();
    request.messages.push({ ...change('tool_removal', 'Read'), role: 'user' });
    assert.throws(() => prepareNative(request, options)); request.messages.pop();
    request.messages.push({ role: 'system', content: [{ type: 'tool_addition', tool: { type: 'mcp_toolset_reference', server_name: 'test' } }] });
    assert.throws(() => prepareNative(request, options));
  }
});
await test('turn_tool_change_beta_http', () => fixture(async (gateway, received) => {
  const header = { 'anthropic-beta': 'mid-conversation-tool-changes-2026-07-01' };
  const request = doc(); request.tools[0].defer_loading = true;
  request.messages.push({ role: 'system', content: [{ type: 'tool_addition', tool: { type: 'tool_reference', name: 'Read' } }] });
  assert.equal((await post(gateway, request, header)).status, 200);
  assert.equal((await post(gateway, request)).status, 400);
  assert.equal((await post(gateway, request, { 'anthropic-beta': header['anthropic-beta'] + ',,' })).status, 400);
  assert.equal(received(), 1);
  assert.equal((await post(gateway, request, { 'anthropic-beta': header['anthropic-beta'] + ',brand-new-turn-2026-10-01' })).status, 200);
  assert.equal(received(), 2);
}));
await test('native_beta_http_accept_and_reject_without_upstream', () => fixture(async (gateway, received) => {
  assert.equal((await post(gateway, doc(), { 'anthropic-beta': NATIVE_BETAS.join(',') })).status, 200);
  // A judged beta passes and is recorded by its fixed label; a malformed header is refused.
  const malformed = await post(gateway, doc(), { 'anthropic-beta': 'files-api-2025-04-14,' });
  assert.equal(malformed.status, 400);
  assert.ok(malformed.text.includes('INVALID_BETA_HEADER')); assert.equal(received(), 1);
  assert.equal((await post(gateway, doc(), { 'anthropic-beta': 'files-api-2025-04-14,brand-new-feature-2026-10-01,SYNTHETIC_PRIVATE' })).status, 200);
  assert.equal(received(), 2);
  const state = gateway.diagnostics();
  assert.deepEqual(state.unknownBetaNames, ['brand-new-feature-2026-10-01']);
  assert.deepEqual(state.judgedBetaLabels, ['FILES_API']);
  assert.deepEqual(state.recentRequests.at(-1).judgedBetaLabels, ['FILES_API']);
  assert.ok(!JSON.stringify(state).includes('SYNTHETIC_PRIVATE'));
}));
await test('parallel_tools_reasoning_roundtrip_and_restart', () => {
  const request = doc(), first = prepareNative(request), result = nativeResponse(events(first, ['reasoning', 'text', 'Read', 'Bash', 'Agent', 'mcp__test__read']), first);
  assert.equal(result.message.stop_reason, 'tool_use'); assert.equal(result.message.usage.input_tokens, 10);
  const transcript = { ...request, messages: [...request.messages, { role: 'assistant', content: result.message.content },
    { role: 'user', content: result.message.content.filter(block => block.type === 'tool_use').map(block =>
      ({ type: 'tool_result', tool_use_id: block.id, content: 'SYNTHETIC_RESULT', is_error: block.name === 'Bash' })) }] };
  const resumed = prepareNative(JSON.parse(JSON.stringify(transcript)));
  assert.equal(resumed.body.input.filter(item => item.type === 'function_call_output').length, 4);
  assert.equal(resumed.body.input.filter(item => item.type === 'reasoning').length, 1);
  assert.equal(resumed.body.input.find(item => item.type === 'reasoning').encrypted_content, 'SYNTHETIC_OPAQUE');
  const final = nativeResponse(events(resumed, ['text'], 'second'), resumed);
  assert.equal(final.message.stop_reason, 'end_turn');
});
await test('compacted_transcript_and_model_switch', () => {
  const request = doc('sol'); request.messages = [{ role: 'user', content: 'SYNTHETIC_COMPACT_SUMMARY' },
    { role: 'system', content: 'SYNTHETIC_NATIVE_CONTEXT' }];
  const result = prepareNative(request); assert.equal(result.selected.model, MODELS.sol.model);
  assert.equal(result.body.input.length, 3);
});
await test('native_payload_exceeds_old_poc_size', () => {
  const request = doc(); request.messages[0].content = 'SYNTHETIC'.repeat(20000);
  const prepared = prepareNative(request); assert.ok(JSON.stringify(prepared.body).length > 65536);
});
for (const mutation of ['unknown-tool', 'changed-args', 'changed-done', 'missing-done', 'changed-model', 'bad-cache', 'incomplete']) {
  await test('reject_' + mutation, () => {
    const first = prepareNative(doc()), source = events(first, ['Read']);
    if (mutation === 'unknown-tool') { source[1].item.name = 'Unknown'; source[4].item.name = 'Unknown'; }
    if (mutation === 'changed-args') source[4].item.arguments = '{}';
    if (mutation === 'changed-done') source[3].arguments = '{}';
    if (mutation === 'missing-done') source.splice(3, 1);
    if (mutation === 'changed-model') source.at(-1).response.model = 'other';
    if (mutation === 'bad-cache') source.at(-1).response.usage.input_tokens_details.cached_tokens = 100;
    if (mutation === 'incomplete') source.pop();
    assert.throws(() => nativeResponse(source, first));
  });
}
await test('unlinked_results_rejected', () => {
  const request = doc(); request.messages.push({ role: 'user', content: [{ type: 'tool_result', tool_use_id: 'wrong', content: 'x' }] });
  assert.throws(() => prepareNative(request));
});
await test('native_launch_preserves_global_profile_and_statusline', () => {
  const fake = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC_TOKEN' }) };
  const env = { CLAUDE_CONFIG_DIR: 'SYNTHETIC_CONFIG', HTTPS_PROXY: 'SYNTHETIC_PROXY', CUSTOM_VALUE: 'VALUE' };
  Object.defineProperty(env, 'OPENAI_API_KEY', { enumerable: true, get() { throw new Error('MUST_NOT_READ'); } });
  const launch = interactiveLaunch(fake, env, process.cwd(), selectModel('sol'), ['--continue']);
  assert.equal(launch.options.env.CLAUDE_CONFIG_DIR, env.CLAUDE_CONFIG_DIR);
  assert.equal(env.CLAUDE_CONFIG_DIR, 'SYNTHETIC_CONFIG');
  assert.equal(launch.options.env.HTTPS_PROXY, env.HTTPS_PROXY);
  assert.equal(launch.args.includes('--tools'), false); assert.equal(launch.args.includes('--continue'), true);
  const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
  assert.equal(Object.hasOwn(settings.env, 'CLAUDE_CONFIG_DIR'), false);
  assert.equal(Object.hasOwn(settings, 'statusLine'), false); // Preserve native user configuration.
  assert.equal(settings.modelPicker.options.length, 4);
  assert.equal(settings.hooks.SubagentStart.length, 1);
  assert.equal(JSON.stringify(launch.args).includes('SYNTHETIC_TOKEN'), false);
  assert.equal(Object.hasOwn(settings, 'permissions'), false);
});
await test('all_models_and_resume_preserve_native_profile_discovery', () => {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC_TOKEN' }) };
  for (const name of Object.keys(MODELS)) {
    for (const source of [{}, { CLAUDE_CONFIG_DIR: 'SYNTHETIC_NATIVE_PROFILE' }]) {
      const before = { ...source };
      const launch = interactiveLaunch(gateway, source, 'D:/SYNTHETIC_OTHER_PROJECT', selectModel(name), ['--continue']);
      assert.equal(launch.options.env.CLAUDE_CONFIG_DIR, source.CLAUDE_CONFIG_DIR);
      assert.equal(Object.hasOwn(launch.options.env, 'CLAUDE_CONFIG_DIR'), Object.hasOwn(source, 'CLAUDE_CONFIG_DIR'));
      assert.deepEqual(source, before);
      assert.ok(launch.args.includes('--continue'));
    }
  }
});
await test('advisor_disabled_only_in_clauduct_child', () => {
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC_TOKEN' }) };
  for (const source of [{}, { CLAUDE_CODE_DISABLE_ADVISOR_TOOL: '0' }]) {
    const before = { ...source };
    const launch = interactiveLaunch(gateway, source, process.cwd());
    const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
    assert.equal(launch.options.env.CLAUDE_CODE_DISABLE_ADVISOR_TOOL, '1');
    assert.equal(settings.env.CLAUDE_CODE_DISABLE_ADVISOR_TOOL, '1');
    assert.deepEqual(source, before);
    assert.equal(Object.hasOwn(settings, 'advisorModel'), false);
    // Anthropic-side reporting is off for this child only; the caller's environment is untouched.
    assert.equal(launch.options.env.DISABLE_TELEMETRY, '1');
    assert.equal(launch.options.env.DISABLE_ERROR_REPORTING, '1');
    assert.equal(settings.env.DISABLE_TELEMETRY, '1');
  }
  assert.deepEqual(judgedBetas('advisor-tool-2026-03-01'), ['ADVISOR_TOOL']);
});
await test('web_search_is_never_sent_upstream', () => {
  const base = () => ({ model: 'astra', stream: true, max_tokens: 100,
    messages: [{ role: 'user', content: 'SYNTHETIC_PROMPT' }],
    tools: [{ type: 'web_search_20250305', name: 'web_search', allowed_domains: ['example.com'] }] });
  const prepared = prepareNative(base());
  // Accepted and recorded, never forwarded: the gateway answers that side query from the
  // backend's standalone search endpoint, and the reference client sends no such tool either.
  assert.equal(prepared.webSearch, true);
  assert.equal(prepared.names.size, 0);
  assert.deepEqual(prepared.body.tools, []);
  assert.equal(prepared.body.tool_choice, 'none');
  assert.equal(prepared.upstreamHeaders, undefined);
  assert.equal(JSON.stringify(prepared.body).includes('web_search'), false);
  assert.deepEqual(prepared.body.input.map(item => item.role ?? item.type), ['user']);
  // Client tools alongside it are still declared normally.
  const withTool = base();
  withTool.tools.push({ name: 'Bash', description: 'run', input_schema: { type: 'object', properties: {} } });
  const carried = prepareNative(withTool);
  assert.deepEqual(carried.body.tools.map(tool => tool.name), ['Bash']);
  assert.equal(carried.body.tool_choice, 'auto');
  // A choice naming the server tool is satisfied here only when this request is that side query.
  const chosen = { ...base(), tool_choice: { type: 'tool', name: 'web_search' } };
  assert.throws(() => prepareNative(chosen), error => error.code === 'UNSUPPORTED_TOOLS');
  assert.equal(prepareNative(chosen, { search: true }).body.tool_choice, 'none');
  // Turn identity for the search request is generated here: nothing is copied from the user's
  // codex install and no local path, repository or workspace is described.
  const metadata = JSON.parse(searchEnvelope().metadata);
  assert.equal(metadata.node_repl_disabled, true);
  assert.equal(Object.hasOwn(metadata, 'workspaces'), false);
  assert.ok(!searchEnvelope().metadata.includes(String.fromCharCode(92)));
  // Both domain lists at once, an unknown field, or a renamed tool stay rejected.
  for (const mutate of [d => { d.tools[0].blocked_domains = ['x.com']; }, d => { d.tools[0].private = 1; },
    d => { d.tools[0].name = 'other'; }, d => { d.tools.push({ ...d.tools[0] }); }]) {
    const doc = base(); mutate(doc); assert.throws(() => prepareNative(doc));
  }
  const search = { id: 'ws_1', type: 'web_search_call', status: 'completed',
    action: { type: 'search', query: 'SYNTHETIC_PRIVATE_QUERY' } };
  const message = { id: 'msg_0', type: 'message', role: 'assistant', status: 'completed',
    content: [{ type: 'output_text', text: 'SYNTHETIC_TEXT', annotations: [
      { type: 'url_citation', url: 'https://example.com', title: 'T', start_index: 0, end_index: 5 }] }] };
  const events = [
    { type: 'response.created', response: { id: 'resp_1', status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { id: 'ws_1', type: 'web_search_call', status: 'in_progress' } },
    { type: 'response.web_search_call.in_progress', output_index: 0, item_id: 'ws_1' },
    { type: 'response.web_search_call.searching', output_index: 0, item_id: 'ws_1' },
    { type: 'response.web_search_call.completed', output_index: 0, item_id: 'ws_1' },
    { type: 'response.output_item.done', output_index: 0, item: search },
    { type: 'response.output_item.added', output_index: 1, item: { ...message, content: [], status: 'in_progress' } },
    { type: 'response.output_text.delta', output_index: 1, item_id: 'msg_0', content_index: 0, delta: 'SYNTHETIC_TEXT' },
    { type: 'response.output_text.done', output_index: 1, item_id: 'msg_0', content_index: 0, text: 'SYNTHETIC_TEXT' },
    { type: 'response.output_item.done', output_index: 1, item: message },
    { type: 'response.completed', response: { id: 'resp_1', status: 'completed', model: prepared.body.model,
      reasoning: prepared.body.reasoning, output: [search, message],
      usage: { input_tokens: 5, output_tokens: 3, total_tokens: 8, input_tokens_details: { cached_tokens: 0 } } } }];
  const counted = createNativeResponse(prepared);
  for (const event of events.slice(0, -1)) counted.push(event);
  counted.push(events.at(-1));
  // Whether the backend actually searched is observable without any query or result.
  assert.equal(counted.webSearchCalls(), 1);
  assert.equal(createNativeResponse(prepared).webSearchCalls(), 0);
  const result = nativeResponse(events, prepared);
  // The search runs upstream; downstream sees the answer without the query or the source list.
  assert.deepEqual(result.message.content, [{ type: 'text', text: 'SYNTHETIC_TEXT' }]);
  assert.equal(result.message.stop_reason, 'end_turn');
  for (const output of [JSON.stringify(result.message), result.sse]) {
    assert.ok(!output.includes('SYNTHETIC_PRIVATE_QUERY'));
    assert.ok(!output.includes('example.com'));
    assert.ok(!output.includes('web_search'));
  }
  // Without the tool requested the same upstream items stay unsupported.
  const plain = prepareNative({ ...base(), tools: [] });
  assert.throws(() => nativeResponse(events, plain), error => error.code === 'UNSUPPORTED_OUTPUT');
});
await test('feature_scan_classifies_known_names', () => {
  const report = classifyBetaNames(['web-search-2025-03-05', 'files-api-2025-04-14',
    'mcp-tunnels-2026-06-22', 'foo-2025-01-01', 'pre-2026-07-28', 'brand-new-feature-2026-10-01']);
  assert.deepEqual(report.allowed, ['web-search-2025-03-05']);
  assert.deepEqual(report.judged, ['files-api-2025-04-14']);
  assert.deepEqual(report.serverDependent, ['mcp-tunnels-2026-06-22']);
  assert.deepEqual(report.unclassified, ['brand-new-feature-2026-10-01']);
});
await test('forward_native_resume_and_effort', () => {
  assert.equal(launchOptions(['--model', 'luna', '--effort', 'high', '--resume', 'SYNTHETIC_SESSION']).selected.effort, 'high');
  assert.deepEqual(launchOptions(['--c']).forward, ['--continue']);
  assert.throws(() => launchOptions(['--settings={}']));
});
await test('hook_payload_never_forwards_transcript', () => {
  assert.deepEqual(bindingFrom({ hook_event_name: 'SubagentStart', agent_id: 'a', agent_type: 'Explore',
    transcript_path: 'SYNTHETIC_PRIVATE', last_assistant_message: 'SYNTHETIC_PRIVATE' }), { id: 'a', role: 'Explore', stop: false });
  assert.throws(() => bindingFrom({ hook_event_name: 'SubagentStart', agent_id: '../x', agent_type: 'Explore' }));
});
await test('hook_context_reads_only_valid_numeric_policy', () => {
  const env = { CLAUDE_CODE_MAX_CONTEXT_TOKENS: '500000', CLAUDE_CODE_AUTO_COMPACT_WINDOW: '100000',
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '83.33333333333334', ANTHROPIC_AUTH_TOKEN: 'SYNTHETIC_PRIVATE' };
  assert.deepEqual(contextFromEnvironment(env), { window: 500000, autoCompactWindow: 100000, compactPercent: 83.33333333333334 });
  const input = { hook_event_name: 'SubagentStart', agent_id: 'context', agent_type: 'Explore' };
  assert.ok(!JSON.stringify(bindingFrom(input, env)).includes('SYNTHETIC_PRIVATE'));
  assert.equal(Object.hasOwn(bindingFrom({ ...input, hook_event_name: 'SubagentStop' }, env), 'contextPolicy'), false);
  for (const value of ['500k', 'Infinity', '-1', '1.5', '', 'SYNTHETIC_PRIVATE']) {
    assert.equal(contextFromEnvironment({ ...env, CLAUDE_CODE_MAX_CONTEXT_TOKENS: value }), null);
  }
});
await test('roles_parallel_errors_do_not_close_gateway', () => fixture(async gateway => {
  const source = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
  for (const [role] of Object.entries(ROLE_MODELS)) await registerBinding({ id: role, role, stop: false }, source);
  const responses = await Promise.all(Object.entries(ROLE_MODELS).map(async ([role, model]) => {
    const result = await post(gateway, doc(), { 'x-claude-code-agent-id': role });
    assert.equal(result.status, 200); assert.equal(result.text.includes('"model":"' + model.model + '"'), true);
    return result;
  })); assert.equal(responses.length, 3);
  for (const [role, model] of Object.entries(ROLE_MODELS)) {
    const timing = gateway.diagnostics().recentRequests.find(row => row.role === role);
    assert.equal(timing.roleRegistered, true); assert.equal(timing.subagent, true);
    assert.equal(timing.model, model.model); assert.equal(timing.effort, model.effort);
  }
  const bad = doc(); bad.messages[0].content = 'SYNTHETIC_RATE_LIMIT';
  assert.equal((await post(gateway, bad)).status, 429);
  assert.equal((await post(gateway, doc())).status, 200); assert.equal(gateway.diagnostics().closing, false);
  assert.equal((await post(gateway, doc(), { 'x-claude-code-agent-id': 'missing' })).status, 200);
  for (const [role] of Object.entries(ROLE_MODELS)) await registerBinding({ id: role, role, stop: true }, source);
  assert.equal(gateway.diagnostics().registeredAgents, 0);
}));
await test('missing_or_skipped_hook_preserves_requested_model_and_warns_once', async () => {
  let warnings = 0;
  await fixture(async (gateway, received, routes) => {
    for (const model of ['terra', 'sol']) {
      const result = await post(gateway, { ...doc(model), output_config: { effort: 'low' } },
        { 'x-claude-code-agent-id': 'unregistered' });
      assert.equal(result.status, 200); assert.ok(result.text.includes(MODELS[model].model));
    }
    assert.equal(warnings, 1); assert.equal(received(), 2);
    assert.deepEqual(routes, [MODELS.terra, MODELS.sol]);
    assert.equal(gateway.diagnostics().unregisteredAgentRequests, 2);
    assert.equal(gateway.diagnostics().registeredAgents, 0); assert.equal(gateway.diagnostics().closing, false);
    const main = await post(gateway, doc()); assert.equal(main.status, 200); assert.equal(warnings, 1);
  }, { onUnregisteredAgent: () => { warnings++; } });
});
await test('stopped_agent_can_resume_without_registration_and_later_rebind', () => fixture(async gateway => {
  const source = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
  const binding = { id: 'resumed', role: 'Plan', stop: false };
  await registerBinding(binding, source);
  const headers = { 'x-claude-code-agent-id': binding.id };
  assert.ok((await post(gateway, doc('terra'), headers)).text.includes(ROLE_MODELS.Plan.model));
  await registerBinding({ ...binding, stop: true }, source);
  assert.ok((await post(gateway, doc('terra'), headers)).text.includes(MODELS.terra.model));
  await registerBinding(binding, source);
  assert.ok((await post(gateway, doc('terra'), headers)).text.includes(ROLE_MODELS.Plan.model));
  assert.equal(gateway.diagnostics().unregisteredAgentRequests, 1);
}));
await test('unregistered_agent_still_requires_auth_valid_model_and_ids', () => fixture(async (gateway, received) => {
  assert.equal((await post(gateway, doc(), { Authorization: 'Bearer SYNTHETIC_WRONG', 'x-claude-code-agent-id': 'unknown' })).status, 401);
  assert.equal((await post(gateway, doc('unknown'), { 'x-claude-code-agent-id': 'unknown' })).status, 400);
  assert.equal((await post(gateway, doc(), { 'x-claude-code-agent-id': '../invalid' })).status, 400);
  assert.equal(received(), 0); assert.equal(gateway.diagnostics().unregisteredAgentRequests, 0);
}));
await test('invalid_binding_never_sets_role', () => fixture(async gateway => {
  const source = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
  await assert.rejects(registerBinding({ id: 123, role: 'Plan', stop: false }, source));
  await assert.rejects(registerBinding({ id: 'valid', role: '', stop: false }, source));
  await assert.rejects(registerBinding({ id: 'context', role: 'Plan', stop: false,
    contextPolicy: { window: 500000, autoCompactWindow: 100000, compactPercent: 101 } }, source));
  await assert.rejects(registerBinding({ id: 'context', role: 'Plan', stop: false,
    contextPolicy: { window: 500000, autoCompactWindow: 100000, compactPercent: 83, extra: 'SYNTHETIC_PRIVATE' } }, source));
  assert.equal(gateway.diagnostics().registeredAgents, 0);
}));
await test('actual_hook_process_filters_input_and_registers_role', () => fixture(async gateway => {
  const file = fileURLToPath(new URL('./agent-route.mjs', import.meta.url));
  const env = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
    ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7), CLAUDE_CODE_MAX_CONTEXT_TOKENS: '500000',
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: '500000', CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '83.33333333333334' };
  const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${file}`, file],
    { env, shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  let output = ''; child.stdout.on('data', b => { output += b; }); child.stderr.on('data', b => { output += b; });
  const done = new Promise(resolve => child.once('close', resolve));
  child.stdin.end(JSON.stringify({ hook_event_name: 'SubagentStart', agent_id: 'hook_agent', agent_type: 'Plan',
    transcript_path: 'SYNTHETIC_PRIVATE', last_assistant_message: 'SYNTHETIC_PRIVATE' }));
  assert.equal(await done, 0); assert.equal(output, '');
  const result = await post(gateway, doc(), { 'x-claude-code-agent-id': 'hook_agent' });
  assert.equal(result.status, 200); assert.ok(result.text.includes(ROLE_MODELS.Plan.model));
  assert.deepEqual(gateway.diagnostics().recentRequests.at(-1).agentContextPolicy,
    { window: 500000, autoCompactWindow: 500000, compactPercent: 83.33333333333334 });
}));
await test('20_concurrent_agents_and_repeated_release', () => fixture(async gateway => {
  const source = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
  for (let round = 0; round < 10; round++) {
    await Promise.all(Array.from({ length: 20 }, async (_, i) => {
      const binding = { id: `agent_${round}_${i}`, role: i % 2 ? 'Plan' : 'Explore', stop: false };
      let phase = 'register';
      try {
        await registerBinding(binding, source);
        phase = 'inference';
        assert.equal((await post(gateway, doc(), { 'x-claude-code-agent-id': binding.id })).status, 200);
        phase = 'unregister';
        await registerBinding({ ...binding, stop: true }, source);
      } catch (error) {
        const state = gateway.diagnostics();
        error.fixtureConcurrency = { round, worker: i, phase, jobs: state.activeJobs,
          registered: state.registeredAgents, pendingCloses: state.pendingConnectionCloses,
          closeTimeouts: state.connectionCloseTimeouts, upstreamAttempts: state.transport.requestAttempts,
          succeeded: state.lifetime.succeeded, failed: state.lifetime.failed };
        throw error;
      }
    }));
    assert.equal(gateway.diagnostics().registeredAgents, 0);
  }
}));
await test('cancel_inflight_keeps_server_available', () => fixture(async gateway => {
  const result = await runInteractive(gateway, () => spawn(process.execPath, ['-e', 'setTimeout(()=>{},5000)'],
    { env: {}, shell: false, stdio: 'ignore', windowsHide: true }), { signal: AbortSignal.timeout(100) });
  assert.equal(result.category, 'USER_CANCELLED'); assert.equal(result.resourcesClosed, true);
}));
await test('1000_requests_no_cumulative_cutoff_or_retained_history', () => fixture(async (gateway, received) => {
  for (let i = 0; i < 1000; i++) {
    const result = await post(gateway, doc(i % 2 ? 'luna' : 'astra')); assert.equal(result.status, 200);
    if (i % 100 === 0) assert.equal(gateway.diagnostics().closing, false);
  }
  await new Promise(done => setTimeout(done, 20));
  assert.equal(received(), 1000); assert.equal(gateway.diagnostics().requestBudget, null);
  assert.equal(gateway.diagnostics().sessionLifetime, null); assert.equal(gateway.diagnostics().activeJobs, 0);
}));
if (seconds) await test('wall_clock_soak', () => fixture(async (gateway, received) => {
  const start = performance.now(), rss = process.memoryUsage().rss;
  while (performance.now() - start < seconds * 1000) {
    const responses = await Promise.all(Array.from({ length: 4 }, () => post(gateway, doc())));
    assert.ok(responses.every(result => result.status === 200)); await new Promise(done => setTimeout(done, 50));
  }
  await new Promise(done => setTimeout(done, 30));
  const state = gateway.diagnostics(); assert.equal(state.activeJobs, 0); assert.equal(state.transport.activeRequests, 0);
  assert.equal(state.closing, false); assert.ok(process.memoryUsage().rss - rss < 256 * 1024 * 1024);
  process.stdout.write(JSON.stringify({ soakSeconds: Math.round((performance.now() - start) / 1000), requests: received(),
    rssGrowthBytes: process.memoryUsage().rss - rss, activeJobs: state.activeJobs }) + '\n');
}));
clearTimeout(watchdog);
process.stdout.write(JSON.stringify({ suite: 'native', passed, failed, realClaude: 0, credentialReads: 0, externalRequests: 0,
  notRun: focused ? ['other cases: focused concurrency diagnostic'] : [] }) + '\n');
process.exitCode = failed ? 1 : 0;
