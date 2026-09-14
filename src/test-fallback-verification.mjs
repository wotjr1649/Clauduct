// The branch under test lives inside the client binary, so nothing here proves it ran. What
// this file pins is the stimulus and the two arms: the same injected stream error, and a child
// environment that differs only in the setting the client reads. The verdict comes from whether
// a live session answers with a non-streaming request, which the gateway names REQUEST_STREAM_FALSE.
import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { requestStatusSnapshot } from './request-status.mjs';
import { interactiveLaunch, launchOptions } from './clauduct.mjs';

const KEY = 'CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK';
const doc = { model: 'sol', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] };
const item = { id: 'msg_fallback', type: 'message', role: 'assistant', status: 'completed',
  content: [{ type: 'output_text', text: 'OK', annotations: [] }] };
// A complete, ordered stream: the injection has to land on a response the parser accepted,
// not on one it was already rejecting, or the arms would differ for the wrong reason.
const stream = [
  { type: 'response.created', response: { id: 'resp_fallback', status: 'in_progress' } },
  { type: 'response.output_item.added', output_index: 0, item: { ...item, content: [], status: 'in_progress' } },
  { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: 'OK' },
  { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text: 'OK' },
  { type: 'response.output_item.done', output_index: 0, item },
  { type: 'response.completed', response: { id: 'resp_fallback', status: 'completed', model: 'gpt-5.6-sol',
    output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
const streamBody = stream.map(value => 'data: ' + JSON.stringify(value) + '\n\n').join('');
let checks = 0;

function armTests() {
  for (const bad of [['--verify-fallback'], ['--verify-fallback', ''], ['--verify-fallback', 'on'],
    ['--verify-fallback', '1'], ['--verify-fallback', 'blocked', '--verify-fallback', 'allowed']]) {
    assert.throws(() => launchOptions(bad), /INVALID_ARGUMENTS/);
    checks++;
  }
  // Absent means absent: a normal run must not arm the injection or move the setting.
  const plain = launchOptions([]);
  assert.equal(plain.verifyFallback, undefined);
  const gateway = { port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) };
  const launchOf = options => interactiveLaunch(gateway, { [KEY]: 'SYNTHETIC_PARENT' },
    'D:/SYNTHETIC_PROJECT', options.selected, options.forward, options);
  const settingsOf = options => {
    const { args } = launchOf(options);
    return JSON.parse(args[args.indexOf('--settings') + 1]);
  };
  assert.equal(settingsOf(plain).env[KEY], '1');
  checks++;

  const blocked = launchOptions(['--verify-fallback', 'blocked']);
  assert.equal(blocked.verifyFallback, 'blocked');
  assert.equal(settingsOf(blocked).env[KEY], '1');
  checks++;

  const allowed = launchOptions(['--verify-fallback=allowed']);
  assert.equal(allowed.verifyFallback, 'allowed');
  assert.ok(!Object.hasOwn(settingsOf(allowed).env, KEY), 'the control arm removes the key, never writes 0');
  checks++;

  // The parent copy is a denylist, so a parent value has to be dropped for the control arm.
  assert.ok(!Object.hasOwn(launchOf(allowed).options.env, KEY));
  // And the blocked arm overrides whatever the parent held: a parent cannot weaken it.
  assert.equal(launchOf(blocked).options.env[KEY], '1');
  assert.equal(launchOf(plain).options.env[KEY], '1');
  checks++;
}

async function injectionTest() {
  const server = createServer((req, res) => { req.resume(); res.end(streamBody); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const transport = createNativeLoopbackTransport(server.address().port);
  const gateway = await startNativeGateway({ transport, injectStreamError: true,
    admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
  const send = () => new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
      agent: false, signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
        'content-type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
      res.resume(); res.on('error', reject); res.on('end', () => resolve(res.statusCode));
    });
    req.on('error', reject); req.end(JSON.stringify(doc));
  });
  try {
    await send();
    await send();
    await gateway.close();
    const status = requestStatusSnapshot(gateway.diagnostics());
    // Once per gateway: the second request must run clean, or every later observation is noise.
    assert.equal(status.lifetime.injectedStreamErrors, 1);
    const failed = status.recentRequests.filter(row => row.failureCategory === 'UPSTREAM_ERROR_EVENT');
    assert.equal(failed.length, 1);
    assert.equal(failed[0].upstreamFailureEvent, 'error');
    assert.equal(failed[0].upstreamErrorCode, 'server_error');
    assert.equal(failed[0].upstreamErrorType, 'api_error');
    // Content reached the client first: that is the shape the client's fallback path sees.
    assert.ok(failed[0].firstTextDeltaMs !== null);
    assert.equal(failed[0].failureStage, 'upstream');
    assert.equal(status.recentRequests.at(-1).success, true);
    checks += 8;
  } finally {
    await gateway.close(); server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  }
}

async function idleGatewayTest() {
  // Off by default: a gateway nobody armed never manufactures a failure.
  const server = createServer((req, res) => { req.resume(); res.end(streamBody); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const transport = createNativeLoopbackTransport(server.address().port);
  const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
  try {
    await new Promise((resolve, reject) => {
      const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
        agent: false, signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
          'content-type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
        res.resume(); res.on('error', reject); res.on('end', resolve);
      });
      req.on('error', reject); req.end(JSON.stringify(doc));
    });
    await gateway.close();
    const status = requestStatusSnapshot(gateway.diagnostics());
    assert.equal(status.lifetime.injectedStreamErrors, 0);
    assert.equal(status.lifetime.failed, 0);
    checks += 2;
  } finally {
    await gateway.close(); server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  }
}

armTests();
await injectionTest();
await idleGatewayTest();
process.stdout.write(JSON.stringify({ suite: 'fallback-verification', checks,
  externalRequests: 0, credentialReads: 0, actualClaudeExecutions: 0 }) + '\n');
