import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeResponse, prepareNative } from './native-protocol.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';

const cases = [['response.failed', 'UPSTREAM_RESPONSE_FAILED'],
  ['response.incomplete', 'UPSTREAM_RESPONSE_INCOMPLETE'], ['error', 'UPSTREAM_ERROR_EVENT']];
const doc = { model: 'astra', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }],
  tools: [{ name: 'Read', input_schema: { type: 'object', properties: {} } }] };
const prefix = [{ type: 'response.created', response: { id: 'resp_test', status: 'in_progress' } },
  { type: 'response.output_item.added', output_index: 0,
    item: { type: 'message', id: 'msg_test', role: 'assistant', status: 'in_progress', content: [] } },
  { type: 'response.output_text.delta', output_index: 0, item_id: 'msg_test', content_index: 0, delta: 'hello' }];
const frame = event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`;
const failedEvent = type => ({ type, code: 'SYNTHETIC_PRIVATE', message: 'SYNTHETIC_PRIVATE',
  response: { id: 'resp_test', error: { code: 'SYNTHETIC_PRIVATE', message: 'SYNTHETIC_PRIVATE' } } });
let checks = 0;
for (const [type, code] of cases) {
  const completed = createNativeResponse(prepareNative(doc));
  completed.push(prefix[0]);
  completed.push({ type: 'response.completed', response: { id: 'resp_test' } });
  assert.throws(() => completed.push(failedEvent(type)), error => error.code === 'EVENT_AFTER_COMPLETION');
  checks++;
  for (const before of [[], prefix]) {
    const parser = createNativeResponse(prepareNative(doc));
    for (const event of before) parser.push(event);
    let first;
    assert.throws(() => parser.push(failedEvent(type)), error => {
      first = error;
      assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
      return error.code === code && error.message === code;
    });
    for (const next of [{ type: 'response.completed', response: {} }, { type: 'error' }, { type: 'SYNTHETIC_PRIVATE' }]) {
      assert.throws(() => parser.push(next), error => error === first);
    }
    assert.throws(() => parser.finish(), error => error === first);
    checks++;
  }
}

let wire = '', sends = 0, split = false;
const upstream = createServer((req, res) => {
  sends++; req.resume(); res.writeHead(200, { 'content-type': 'text/event-stream' });
  if (split) {
    const cut = frame(prefix[0]).length + 20, tail = wire.slice(cut);
    res.write(wire.slice(0, cut)); setImmediate(() => res.end(tail));
  }
  else res.end(wire);
});
await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
try {
  for (const [type, code] of cases) {
    split = false;
    // Envelope/sequence validation still takes precedence over classifying a failure.
    for (const sequence of [undefined, 4, 1]) {
      const transport = createNativeLoopbackTransport(upstream.address().port);
      wire = frame({ ...prefix[0], sequence_number: 0 })
        + frame({ ...failedEvent(type), ...(sequence !== undefined && { sequence_number: sequence }) });
      try {
        await assert.rejects(transport.send({}, AbortSignal.timeout(5000)),
          error => error.code === (sequence === 1 ? code : 'SEQUENCE_MISMATCH'));
        assert.equal(transport.diagnostics().retries, 0);
        checks++;
      } finally { await transport.close(); }
    }
    {
      const transport = createNativeLoopbackTransport(upstream.address().port);
      wire = frame(prefix[0]) + frame({ type: 'response.completed' }) + frame(failedEvent(type));
      try {
        await assert.rejects(transport.send({}, AbortSignal.timeout(5000)), error => error.code === 'EVENT_AFTER_COMPLETION');
        checks++;
      } finally { await transport.close(); }
    }
    for (const trailer of ['', frame({ type: 'response.completed', response: {} }),
      'data: [DONE]\n\n', 'data: SYNTHETIC_PRIVATE\n\n']) {
      for (const streaming of [false, true]) {
        split = streaming;
        const transport = createNativeLoopbackTransport(upstream.address().port);
        const attempts = [], delivered = [], beforeSends = sends;
        wire = frame(prefix[0]) + frame(failedEvent(type)) + trailer;
        try {
          await assert.rejects(transport.send({}, AbortSignal.timeout(5000), {
            attemptTimings: attempts, ...(streaming && { onEvent: event => delivered.push(event.type) })
          }), error => error.code === code && error.message === code);
          assert.equal(sends - beforeSends, 1);
          assert.equal(attempts.length, 1);
          assert.equal(attempts[0].terminalState, type);
          assert.equal(attempts[0].completed, false);
          assert.equal(attempts[0].postCompletionFrame, null);
          assert.equal(transport.diagnostics().retries, 0);
          assert.equal(transport.diagnostics().activeSockets, 0);
          assert.deepEqual(delivered, streaming ? ['response.created'] : []);
          assert.ok(!JSON.stringify(attempts).includes('SYNTHETIC_PRIVATE'));
          checks++;
        } finally { await transport.close(); }
      }
    }
    for (const before of [[], prefix]) {
      split = false;
      const transport = createNativeLoopbackTransport(upstream.address().port);
      const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
      wire = before.map(frame).join('') + frame(failedEvent(type)) + frame({ type: 'error' });
      try {
        const result = await new Promise((resolve, reject) => {
          const req = request({ host: '127.0.0.1', port: gateway.port, method: 'POST', path: '/v1/messages',
            agent: false, signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
              'anthropic-version': '2023-06-01', 'content-type': 'application/json' } }, res => {
            let text = ''; res.on('data', chunk => { text += chunk; }); res.on('error', reject);
            res.on('end', () => resolve({ status: res.statusCode, text }));
          });
          req.on('error', reject); req.end(JSON.stringify(doc));
        });
        assert.equal(result.status, before.length ? 200 : 502);
        assert.ok(result.text.includes(code));
        assert.ok(result.text.includes(`event=${type}`));
        assert.ok(!result.text.includes('SYNTHETIC_PRIVATE'));
        assert.ok(!result.text.includes('message_stop'));
        const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
          ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
        const row = status.recentRequests.at(-1);
        assert.equal(row.failureCategory, code);
        assert.equal(row.upstreamFailureEvent, type);
        assert.equal(row.failureStage, 'upstream');
        assert.equal(row.success, false);
        assert.equal(row.attempts[0].terminalState, type);
        assert.equal(row.attempts[0].status, 200);
        assert.equal(row.attempts.length, 1);
        assert.equal(status.lifetime.failed, 1);
        assert.equal(transport.diagnostics().activeSockets, 0);
        assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE'));
        checks++;
      } finally { await gateway.close(); }
    }
  }
} finally { upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve)); }
console.log(JSON.stringify({ suite: 'upstream-failures', passed: true, checks, loopbackRequests: sends,
  externalRequests: 0, credentialReads: 0 }));
