import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync, unlinkSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createReadFixture, readLaunch, runReadClient } from './claude-read-once.mjs';
import { createLoopbackCodexTransport } from './codex-transport.mjs';
import { startGateway } from './gateway.mjs';
import { FIXTURE_PATH } from './adapter.mjs';
import { syntheticClaudeRequest } from './read-test-client.mjs';

const root = dirname(fileURLToPath(import.meta.url));
let passed = 0, failed = 0, clients = 0, receivedTotal = 0;
const watchdog = setTimeout(() => { process.stderr.write('READ_SUITE_TIMEOUT\n'); process.exit(1); }, 30000);
async function test(name, action) {
  try { await action(); passed++; } catch { failed++; process.stderr.write(JSON.stringify({ failure: name }) + '\n'); }
}
function response(number, value, { outputTokens = 5, noTool = false, cached = false } = {}) {
  const tool = number === 1 && !noTool;
  const item = tool ? { type: 'function_call', id: 'fc_1', call_id: 'call_1', name: 'Read',
    arguments: JSON.stringify({ file_path: FIXTURE_PATH }), status: 'completed' }
    : { type: 'message', id: `msg_${number}`, role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: value, annotations: [] }] };
  const pos = { item_id: item.id, output_index: 0, ...(tool ? {} : { content_index: 0 }) };
  const prefix = tool ? 'response.function_call_arguments' : 'response.output_text';
  const events = [
    { type: 'response.created', response: { id: `resp_${number}`, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', ...(tool ? { arguments: '' } : { content: [] }) } },
    { type: `${prefix}.delta`, ...pos, delta: tool ? item.arguments : value },
    { type: `${prefix}.done`, ...pos, ...(tool ? { arguments: item.arguments } : { text: value }) },
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: `resp_${number}`, status: 'completed', model: 'gpt-6-astra',
      reasoning: { effort: 'low' }, output: [item], usage: { input_tokens: 20, output_tokens: outputTokens, total_tokens: 20 + outputTokens,
        ...(cached ? { input_tokens_details: { cached_tokens: 12 } } : {}) } } }
  ];
  return events.map(e => `event: ${e.type}\ndata: ${JSON.stringify(e)}\n\n`).join('');
}
async function scenario(mode = 'normal', fault = '', signal) {
  const fixture = createReadFixture(); let requests = 0, invalid = false, gateway;
  const server = createServer((req, res) => {
    let raw = ''; req.on('data', chunk => { raw += chunk.toString(); });
    req.on('end', () => {
      requests++; receivedTotal++;
      const body = JSON.parse(raw);
      invalid ||= (fault === 'token400' ? body.max_output_tokens !== 64000 : Object.hasOwn(body, 'max_output_tokens'))
        || Object.hasOwn(body, 'max_tokens') || body.reasoning.effort !== 'low'
        || body.model !== 'gpt-6-astra' || raw.includes('SYNTHETIC_PRIVATE_ID')
        || (requests === 1 && raw.includes(fixture.marker));
      if (fault === 'http400') { res.writeHead(400); res.end('SYNTHETIC_PRIVATE_ERROR'); return; }
      if (fault === 'token400') {
        res.writeHead(400); res.end(JSON.stringify({ detail: 'Unsupported parameter: max_output_tokens' })); return;
      }
      if (fault === 'stall') return;
      let marker = 'WRONG';
      if (requests === 2) {
        const results = body.input.filter(item => item.type === 'function_call_output');
        const content = JSON.parse(results[0].output).content;
        marker = content.map(part => part.text).join('\n').match(/CLAUDUCT_READ_[A-F0-9]{32}/)?.[0] ?? 'WRONG';
        invalid ||= results.length !== 1 || results[0].call_id !== 'call_1' || raw.includes('cache_control');
        if (mode === 'extra-system') {
          invalid ||= JSON.stringify(body.input.map(item => item.type ?? item.role))
            !== JSON.stringify(['developer', 'user', 'developer', 'function_call', 'function_call_output', 'developer'])
            || body.input.at(-1).content[0].text !== 'SYNTHETIC_PRIVATE_SYSTEM';
        }
      }
      const wire = response(requests, fault === 'wrong-final' ? 'WRONG' : marker,
        { outputTokens: fault === 'over-limit' ? 64001 : 5, noTool: fault === 'no-tool', cached: fault === 'cached' });
      res.writeHead(200, { ...(fault === 'missing-header' ? {} : { 'Content-Type': 'text/event-stream' }),
        ...(fault === 'truncated' ? { 'Content-Length': Buffer.byteLength(wire) + 10 } : {}) });
      res.end(wire);
    });
  });
  try {
    await new Promise(done => server.listen(0, '127.0.0.1', done));
    const transport = createLoopbackCodexTransport(server.address().port, { profile: 'astra-low',
      tokenLimitPolicy: fault === 'token400' ? 'preserve' : 'backend-default' });
    gateway = await startGateway({ transport, profile: 'astra-low', readMarker: fixture.marker,
      headerPolicy: 'codex-missing-content-type', limits: { lifetimeMs: 4000, upstreamMs: 2000 } });
    const result = await runReadClient(gateway, endpoint => {
      clients++;
      const launch = readLaunch(endpoint, process.env);
      assert.equal(JSON.stringify(launch.args).includes(launch.options.env.ANTHROPIC_AUTH_TOKEN), false);
      assert.equal(JSON.stringify(launch.args).includes(fixture.marker), false);
      return spawn(process.execPath, ['--permission', `--allow-fs-read=${root}`,
        join(root, 'read-test-client.mjs'), String(endpoint.port), mode], launch.options);
    }, fixture.marker, { signal, finishMs: 1000 });
    assert.equal(invalid, false); assert.equal(result.resourcesClosed, true);
    const serialized = JSON.stringify(result);
    for (const forbidden of [fixture.marker, 'SYNTHETIC_PRIVATE', 'Bearer ']) assert.equal(serialized.includes(forbidden), false);
    return { result, requests, state: gateway.diagnostics() };
  } finally {
    if (gateway) await gateway.close();
    await new Promise(done => server.close(done));
    assert.equal(fixture.release(), true);
  }
}
for (const [mode, fault, category, requests] of [
  ['normal', '', 'SUCCESS', 2], ['late-cli', '', 'SUCCESS', 2], ['extra', '', 'SUCCESS', 2],
  ['normal', 'missing-header', 'SUCCESS', 2], ['denied', '', 'TOOL_EXECUTION_DENIED', 1],
  ['wrong-result', '', 'READ_RESULT_MISMATCH', 1], ['wrong-cli', '', 'CLI_FAILED', 2],
  ['bad-line', '', 'READ_RESULT_MISMATCH', 1],
  ['normal', 'wrong-final', 'FINAL_MARKER_MISMATCH', 2], ['normal', 'over-limit', 'OUTPUT_TOKEN_LIMIT_EXCEEDED', 1],
  ['normal', 'no-tool', 'NO_TOOL_CALL', 1], ['normal', 'http400', 'HTTP_ERROR', 1],
  ['normal', 'token400', 'UPSTREAM_TOKEN_LIMIT_REJECTED', 1],
  ['normal', 'stall', 'TIMEOUT', 1], ['normal', 'truncated', 'UPSTREAM_IO_ERROR', 1],
  ['bad-beta', '', 'UNSUPPORTED_CLIENT_VERSION_OR_BETA', 0],
  ['extra-system', '', 'SUCCESS', 2],
  ['extra-system', 'cached', 'SUCCESS', 2],
  ['changed-history', '', 'HISTORY_MISMATCH', 1],
  ['changed-options', '', 'HISTORY_MISMATCH', 1],
  ['changed-tool', '', 'HISTORY_MISMATCH', 1],
  ['changed-id', '', 'INVALID_TOOL_RESULT', 1],
  ['invalid-cache', '', 'UNSUPPORTED_CONTENT', 1],
  ['invalid-tail', '', 'UNSUPPORTED_FIELDS', 1],
  ['duplicate-result', '', 'INVALID_TOOL_RESULT', 1],
  ['extra-message', '', 'UNSUPPORTED_MESSAGES', 1],
  ['duplicate-beta', '', 'UNSUPPORTED_CLIENT_VERSION_OR_BETA', 0]
]) await test(`read_${mode}_${fault || 'normal'}`, async () => {
  const item = await scenario(mode, fault);
  assert.equal(item.result.category, category); assert.equal(item.requests, requests);
  assert.equal(item.result.passed, category === 'SUCCESS');
  assert.equal(item.result.tokenLimitPolicy, fault === 'token400' ? 'preserve' : 'backend-default');
  assert.equal(item.result.diagnosticVersion, 3);
  if (mode === 'extra-system') {
    assert.equal(item.result.requestShape.messageCount, 5);
    assert.equal(item.result.requestShape.following, true);
    assert.equal(item.result.requestShape.previousPrefixMatches, false);
    assert.equal(item.result.requestShape.canonicalPrefixMatches, true);
    assert.equal(item.result.requestShape.optionsMatch, true);
    assert.deepEqual(item.result.requestShape.messages.map(m => m.role), ['user', 'system', 'assistant', 'user', 'system']);
    assert.deepEqual(item.result.requestShape.messages[3].blocks, [{ type: 'tool_result', linked: true, isError: false }]);
  }
  assert.equal(item.result.thinkingTokenCountRequested, mode !== 'bad-beta');
  assert.equal(item.result.unknownBetaCount, mode === 'bad-beta' ? 1 : 0);
  if (category === 'SUCCESS') {
    assert.equal(item.result.readCalls, 1); assert.equal(item.result.linkedReadResults, 1);
    assert.equal(item.result.gatewayExactMarker, true); assert.equal(item.result.claudeExactMarker, true);
  }
});
await test('cancel_before_start_has_no_requests', async () => {
  const before = clients;
  const item = await scenario('normal', '', AbortSignal.abort());
  assert.equal(item.result.category, 'USER_CANCELLED'); assert.equal(clients, before); assert.equal(item.requests, 0);
});
await test('cancel_during_request_stops_child_and_upstream', async () => {
  const controller = new AbortController(), timer = setTimeout(() => controller.abort(), 150);
  try {
    const item = await scenario('normal', 'stall', controller.signal);
    assert.equal(item.result.category, 'USER_CANCELLED'); assert.equal(item.result.clientClosed, true);
    assert.ok(item.requests <= 1);
  } finally { clearTimeout(timer); }
});
await test('fixture_existing_preserved', () => {
  const fixture = createReadFixture();
  try { assert.throws(() => createReadFixture(), error => error.code === 'FIXTURE_EXISTS');
    assert.equal(readFileSync(FIXTURE_PATH, 'utf8'), fixture.marker + '\n'); }
  finally { assert.equal(fixture.release(), true); }
});
await test('fixture_changes_preserved', () => {
  const fixture = createReadFixture();
  try {
    writeFileSync(FIXTURE_PATH, 'SYNTHETIC_CHANGED');
    assert.equal(fixture.release(), false); assert.equal(readFileSync(FIXTURE_PATH, 'utf8'), 'SYNTHETIC_CHANGED');
  } finally { if (existsSync(FIXTURE_PATH) && readFileSync(FIXTURE_PATH, 'utf8') === 'SYNTHETIC_CHANGED') unlinkSync(FIXTURE_PATH); }
});
await test('fixture_outside_root_rejected', () => {
  assert.throws(() => createReadFixture(join(root, '..', 'outside.txt')), error => error.code === 'FIXTURE_BOUNDARY_REJECTED');
});
await test('transport_limit_policy_rejects_invalid', () => {
  assert.throws(() => createLoopbackCodexTransport(1, { tokenLimitPolicy: 'ignore' }), error => error.code === 'INVALID_TOKEN_LIMIT_POLICY');
});
clearTimeout(watchdog);
process.stdout.write(JSON.stringify({ suite: 'claude-read-once', passed, failed, syntheticClients: clients,
  syntheticUpstreamRequests: receivedTotal, actualClaudeExecutions: 0, actualCredentialReads: 0,
  externalRequests: 0, fixtureRemaining: existsSync(FIXTURE_PATH) }) + '\n');
process.exitCode = failed ? 1 : 0;
