import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { installHttpClose } from './http-close.mjs';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import { OfflineSession, MODEL } from '../poc/adapter.mjs';
import { startGateway } from '../poc/gateway.mjs';
import { createLoopbackCodexTransport } from '../poc/codex-transport.mjs';
import { syntheticClaudeRequest } from '../poc/read-test-client.mjs';
import { interactiveLaunch, launchOptions, runInteractive } from './clauduct.mjs';

const self = fileURLToPath(import.meta.url), root = dirname(dirname(self));
function initial(model = MODEL) {
  return { ...syntheticClaudeRequest(), model, tools: [] };
}
function wire(number, { model = MODEL, overLimit = false } = {}) {
  const text = `SYNTHETIC_REPLY_${number}`;
  const item = { type: 'message', id: `msg_${number}`, role: 'assistant', status: 'completed', phase: 'final_answer',
    content: [{ type: 'output_text', text, annotations: [] }] };
  const tokens = overLimit ? 64001 : 5;
  const events = [
    { type: 'response.created', response: { id: `resp_${number}`, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
    { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: text },
    { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text },
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: `resp_${number}`, status: 'completed', model,
      reasoning: { effort: 'low' }, output: [item], usage: { input_tokens: 20, output_tokens: tokens,
        total_tokens: 20 + tokens, input_tokens_details: { cached_tokens: 12 } } } }
  ];
  return events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join('');
}
async function send(port, headers, body) {
  return new Promise((done, reject) => {
    const raw = JSON.stringify(body);
    const req = request({ hostname: '127.0.0.1', port, path: '/v1/messages?beta=true', method: 'POST', agent: false,
      signal: AbortSignal.timeout(3000), headers: { ...headers, 'Content-Length': Buffer.byteLength(raw),
        'anthropic-version': '2023-06-01', 'Content-Type': 'application/json',
        'anthropic-beta': 'claude-code-20250219,thinking-token-count-2026-05-13',
        'x-claude-code-session-id': 'synthetic-chat' } }, res => {
      let text = '';
      res.on('data', chunk => { text += chunk; if (text.length > 524288) req.destroy(); });
      res.on('error', reject);
      res.on('end', () => done({ status: res.statusCode, text }));
    });
    req.on('error', reject); req.end(raw);
  });
}
function answer(response) {
  assert.equal(response.status, 200);
  const frames = response.text.trim().split('\n\n').map(frame => JSON.parse(frame.split('\ndata: ')[1]));
  assert.equal(frames[0].message.model, MODEL);
  assert.equal(frames[0].message.usage.input_tokens + frames[0].message.usage.cache_read_input_tokens, 20);
  return { role: 'assistant', content: [{ type: 'text', text: frames[2].delta.text }] };
}
function next(doc, reply) {
  const result = structuredClone(doc);
  result.messages.push(reply, { role: 'user', content: 'SYNTHETIC_NEXT' },
    { role: 'system', content: [{ type: 'text', text: 'SYNTHETIC_TURN_SYSTEM', cache_control: { type: 'ephemeral' } }] });
  result.messages[1].content = 'SYNTHETIC_LATE_SYSTEM';
  return result;
}
async function fixture(action, { fault, requestBudget = 32, limits = {} } = {}) {
  let received = 0, invalid = false, gateway;
  const server = createServer((req, res) => {
    let raw = '';
    req.on('data', chunk => { raw += chunk; });
    req.on('end', () => {
      received++;
      const body = JSON.parse(raw);
      invalid ||= body.model !== MODEL || body.reasoning.effort !== 'low' || body.tool_choice !== 'none'
        || body.tools.length !== 0 || Object.hasOwn(body, 'max_output_tokens') || raw.includes('SYNTHETIC_PRIVATE_ID');
      if (received > 1) {
        const prior = body.input.filter(item => item.type === 'message');
        invalid ||= prior.length !== received - 1 || prior.at(-1).content[0].text !== `SYNTHETIC_REPLY_${received - 1}`;
      }
      if (fault === 'stall') return;
      res.writeHead(fault === 'http400' ? 400 : 200);
      res.end(fault === 'http400' ? '{}' : wire(received, { overLimit: fault === 'over-limit' }));
    });
  });
  server.on('connection', socket => installHttpClose(socket));
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  const transport = createLoopbackCodexTransport(server.address().port,
    { profile: 'astra-low', tokenLimitPolicy: 'backend-default', requestBudget });
  try {
    gateway = await startGateway({ transport, chat: true, profile: 'astra-low',
      headerPolicy: 'codex-missing-content-type', limits: { lifetimeMs: 8000, upstreamMs: 2000, ...limits } });
    await action(gateway, () => received);
    assert.equal(invalid, false);
  } finally {
    if (gateway) {
      const final = await gateway.close();
      assert.equal(final.activeSockets, 0); assert.equal(final.activeJobs, 0); assert.equal(final.activeTimers, 0);
      assert.equal(final.transport.activeRequests, 0); assert.equal(final.transport.activeSockets, 0);
    } else await transport.close();
    server.closeAllConnections(); await new Promise(done => server.close(done));
  }
}

async function client() {
  const port = Number(process.argv[3]);
  if (!Number.isInteger(port) || port < 1 || port > 65535) process.exit(2);
  const headers = { Authorization: `Bearer ${process.env.ANTHROPIC_AUTH_TOKEN}` };
  let doc = initial();
  for (let i = 1; i <= 3; i++) {
    const reply = answer(await send(port, headers, doc));
    assert.equal(reply.content[0].text, `SYNTHETIC_REPLY_${i}`); doc = next(doc, reply);
  }
}
async function tests() {
  let passed = 0, failed = 0;
  const watchdog = setTimeout(() => { process.stderr.write('CHAT_SUITE_TIMEOUT\n'); process.exit(1); }, 30000);
  async function test(name, action) {
    // A bare name sent seven failures into the baseline as "spawn is blocked in this shell",
    // which was wrong: one of them spawns nothing. Carry the reason, bounded and fixed-shape.
    try { await action(); passed++; } catch (error) {
      failed++;
      process.stderr.write(JSON.stringify({ failure: name, reason: String(error?.message ?? error).slice(0, 200) }) + '\n');
    }
  }
  await test('three_turns_keep_history_and_gateway_alive', () => fixture(async (gateway, received) => {
    let doc = initial();
    for (let turn = 1; turn <= 3; turn++) {
      const reply = answer(await send(gateway.port, gateway.clientHeaders(), doc));
      assert.equal(reply.content[0].text, `SYNTHETIC_REPLY_${turn}`); doc = next(doc, reply);
      assert.equal(gateway.diagnostics().closing, false); assert.equal(gateway.diagnostics().session.state, 'READY');
    }
    assert.equal(received(), 3);
  }));
  for (const fault of ['history', 'assistant', 'model', 'tools', 'options', 'replay']) {
    await test(`reject_${fault}`, () => fixture(async (gateway, received) => {
      let doc = initial();
      const reply = answer(await send(gateway.port, gateway.clientHeaders(), doc));
      if (fault !== 'replay') doc = next(doc, reply);
      if (fault === 'history') doc.messages[0].content[0].text = 'CHANGED';
      if (fault === 'assistant') doc.messages[2].content[0].text = 'FORGED';
      if (fault === 'model') doc.model = 'unavailable';
      if (fault === 'tools') doc.tools = syntheticClaudeRequest().tools;
      if (fault === 'options') doc.max_tokens = 100;
      const result = await send(gateway.port, gateway.clientHeaders(), doc);
      assert.equal(result.status, 400); assert.equal(received(), 1);
      assert.equal(JSON.stringify(gateway.diagnostics()).includes('CHANGED'), false);
    }));
  }
  for (const fault of ['http400', 'over-limit']) await test(fault, () => fixture(async gateway => {
    const result = await send(gateway.port, gateway.clientHeaders(), initial());
    assert.equal(result.status, 502);
    assert.equal(JSON.parse(result.text).error.message, fault === 'http400' ? 'HTTP_ERROR' : 'OUTPUT_TOKEN_LIMIT_EXCEEDED');
  }, { fault }));
  await test('transport_budget_still_enforced', () => fixture(async (gateway, received) => {
    let doc = initial();
    for (let i = 0; i < 2; i++) doc = next(doc, answer(await send(gateway.port, gateway.clientHeaders(), doc)));
    const result = await send(gateway.port, gateway.clientHeaders(), doc);
    assert.equal(JSON.parse(result.text).error.message, 'REQUEST_BUDGET'); assert.equal(received(), 2);
  }, { requestBudget: 2 }));
  await test('32_turns_then_budget_rejection', () => fixture(async (gateway, received) => {
    let doc = initial();
    for (let i = 0; i < 32; i++) doc = next(doc, answer(await send(gateway.port, gateway.clientHeaders(), doc)));
    assert.equal(gateway.diagnostics().closing, false);
    const result = await send(gateway.port, gateway.clientHeaders(), doc);
    assert.equal(result.status, 429); assert.equal(JSON.parse(result.text).error.message, 'REQUEST_BUDGET');
    assert.equal(received(), 32);
  }));
  await test('minimal_chat_options_and_luna_response', () => {
    const session = new OfflineSession({ inputPolicy: 'claude-code-chat', profile: 'luna-low' });
    try {
      const doc = initial('gpt-5.6-luna');
      for (const key of ['metadata', 'thinking', 'output_config', 'context_management', 'tools', 'system']) delete doc[key];
      const body = JSON.parse(session.prepare(JSON.stringify(doc)));
      assert.equal(body.model, 'gpt-5.6-luna'); assert.equal(body.tools.length, 0);
      session.begin({ endpoint: 'https://chatgpt.com/backend-api/codex/responses', httpStatus: 200,
        contentTypePresent: true, contentType: 'text/event-stream' });
      session.push(Buffer.from(wire(1, { model: 'gpt-5.6-luna' })));
      assert.equal(session.finish().message.model, 'gpt-5.6-luna');
      assert.equal(session.diagnostics.state, 'READY');
    } finally { session.cancel(); }
  });
  await test('launch_preserves_cwd_and_hides_secret', () => fixture(async gateway => {
    const source = { SystemRoot: process.env.SystemRoot };
    Object.defineProperty(source, 'CLAUDE_CODE_OAUTH_TOKEN', { enumerable: true, get() { throw new Error('DO_NOT_READ'); } });
    const launch = interactiveLaunch(gateway, source, root);
    assert.equal(launch.options.cwd, root); assert.equal(launch.options.stdio, 'inherit'); assert.equal(launch.options.shell, false);
    assert.equal(launch.args.includes('-p'), false); assert.equal(launch.args.includes('--tools'), false);
    assert.equal(launch.options.env.CLAUDE_CODE_OAUTH_TOKEN, '');
    assert.equal(JSON.stringify(launch.args).includes(launch.options.env.ANTHROPIC_AUTH_TOKEN), false);
    // --agents now always follows --settings, so locate the value instead of taking the tail.
    const settings = JSON.parse(launch.args[launch.args.indexOf('--settings') + 1]);
    assert.equal(settings.modelPicker.options[0].model, MODEL);
    assert.equal(settings.modelPicker.replaceBuiltInOptions, true);
    assert.equal(Object.hasOwn(settings.env, 'ANTHROPIC_AUTH_TOKEN'), false);
  }));
  await test('real_synthetic_child_three_turns_and_exit', () => fixture(async gateway => {
    const result = await runInteractive(gateway, endpoint => {
      const launch = interactiveLaunch(endpoint, process.env, root);
      return spawn(process.execPath, ['--permission', `--allow-fs-read=${root}`, self,
        '--synthetic-client', String(endpoint.port)], launch.options);
    });
    assert.equal(result.category, 'SUCCESS'); assert.equal(result.clientExitCode, 0); assert.equal(result.resourcesClosed, true);
    assert.equal(result.requestAttempts, 3);
  }));
  await test('cancel_before_child', () => fixture(async gateway => {
    const result = await runInteractive(gateway, () => { throw new Error('MUST_NOT_START'); }, { signal: AbortSignal.abort() });
    assert.equal(result.category, 'USER_CANCELLED'); assert.equal(result.resourcesClosed, true);
  }));
  await test('spawn_throw_closes_gateway', () => fixture(async gateway => {
    const result = await runInteractive(gateway, () => { throw new Error('SYNTHETIC_PRIVATE'); });
    assert.equal(result.category, 'CLIENT_START_FAILED'); assert.equal(result.resourcesClosed, true);
  }));
  await test('status_projection_failure_keeps_the_report', () => fixture(async gateway => {
    // This gateway's diagnostics carry no recentRequests, so the projection cannot run. A throw
    // used to travel out of runInteractive and cost main() the exit line, the status JSON and the
    // status file at once. The result must still arrive, name the gap, and keep what it can.
    const result = await runInteractive(gateway, () => { throw new Error('SYNTHETIC_PRIVATE'); });
    assert.equal(result.requestStatus.statusUnavailable, 'STATUS_UNAVAILABLE');
    assert.equal(Object.values(result.requestStatus.cleanup).every(value => value === true), true);
    assert.equal(result.category, 'CLIENT_START_FAILED');
    assert.ok(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE'));
  }));
  await test('status_projection_exception_never_becomes_output', () => fixture(async gateway => {
    const brokenProjection = { ...gateway, diagnostics: () => {
      const state = gateway.diagnostics();
      Object.defineProperty(state, 'recentRequests', { get() { throw new Error('SYNTHETIC_PRIVATE_STATUS'); } });
      return state;
    } };
    const result = await runInteractive(brokenProjection, () => { throw new Error('SYNTHETIC_START_FAILURE'); });
    assert.equal(result.requestStatus.statusUnavailable, 'STATUS_UNAVAILABLE');
    assert.ok(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_STATUS'));
    assert.ok(Object.values(result.requestStatus.cleanup).every(value => value === true));
  }));
  await test('spawn_error_event_closes_gateway', () => fixture(async gateway => {
    const result = await runInteractive(gateway, () => spawn(root + '/synthetic-missing-executable', [],
      { shell: false, windowsHide: true, env: {}, stdio: 'ignore' }));
    assert.equal(result.category, 'CLIENT_START_FAILED'); assert.equal(result.resourcesClosed, true);
  }));
  await test('cancel_stops_owned_running_child', () => fixture(async gateway => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 100);
    try {
      const result = await runInteractive(gateway, () => spawn(process.execPath, ['-e', 'setTimeout(()=>{},5000)'],
        { shell: false, windowsHide: true, env: {}, stdio: 'ignore' }), { signal: controller.signal });
      assert.equal(result.category, 'USER_CANCELLED'); assert.equal(result.resourcesClosed, true);
    } finally { clearTimeout(timer); }
  }));
  await test('gateway_lifetime_stops_owned_child', () => fixture(async gateway => {
    const result = await runInteractive(gateway, () => spawn(process.execPath, ['-e', 'setTimeout(()=>{},5000)'],
      { shell: false, windowsHide: true, env: {}, stdio: 'ignore' }));
    assert.equal(result.category, 'SESSION_TIMEOUT'); assert.equal(result.resourcesClosed, true);
  }, { limits: { lifetimeMs: 150 } }));
  for (const args of [['--settings', '{}'], ['--model', 'other'], ['--effort', 'other'], ['--help', '--dry-run'], ['--model']]) {
    await test(`options_${args[0]}_${args.length}`, () => assert.throws(() => launchOptions(args)));
  }
  await test('model_selection', () => {
    // No --model means DEFAULT_SELECTION, which is astra/low — what --help documents and what
    // test-launcher-native asserts on the launch arguments. astra's own default effort is
    // medium, and naming the model is what asks for it.
    assert.deepEqual(launchOptions([]).selected, { model: 'gpt-6-astra', effort: 'low' });
    assert.deepEqual(launchOptions(['--model', 'astra']).selected, { model: 'gpt-6-astra', effort: 'medium' });
    assert.deepEqual(launchOptions(['--model', 'gpt-5.6-luna']).selected, { model: 'gpt-5.6-luna', effort: 'max' });
  });
  await test('default_poc_budget_unchanged', () => {
    const session = new OfflineSession();
    assert.equal(session.diagnostics.inputPolicy, 'fixture'); session.cancel();
    assert.throws(() => createLoopbackCodexTransport(1, { requestBudget: 33 }));
  });
  clearTimeout(watchdog);
  process.stdout.write(JSON.stringify({ suite: 'chat-launch', passed, failed, actualClaudeExecutions: 0,
    actualCredentialReads: 0, externalRequests: 0 }) + '\n');
  process.exitCode = failed ? 1 : 0;
}
if (process.argv[2] === '--synthetic-client') {
  try { await client(); } catch { process.stderr.write('SYNTHETIC_CHAT_CLIENT_FAILED\n'); process.exitCode = 1; }
} else await tests();
