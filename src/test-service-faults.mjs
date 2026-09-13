import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';
import { SERVICE_FAULTS, publicServiceEvents, serviceFailureWire, frame } from '../verification/fixtures/service-faults.mjs';

const doc = { model: 'luna', stream: true, max_tokens: 100,
  messages: [{ role: 'user', content: 'PUBLIC_SERVICE_TASK' }],
  tools: [{ name: 'mcp__fixture__complete_report', input_schema: { type: 'object', properties: {}, additionalProperties: false } }] };
const expected = { 'error-200': 'UPSTREAM_ERROR_EVENT', truncated: 'TRUNCATED_STREAM',
  'invalid-utf8': 'INVALID_UTF8', 'sequence-gap': 'SEQUENCE_MISMATCH' };
let checks = 0, httpRequests = 0;
for (const kind of SERVICE_FAULTS) {
  let sends = 0;
  const timers = new Set();
  const server = createServer((req, res) => {
    sends++; httpRequests++; req.resume();
    if (kind === 'flapping-503') {
      if (sends <= 2) { res.writeHead(503).end(); return; }
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.end(publicServiceEvents('gpt-5.6-luna', 'max', 1, null).map(frame).join('')); return;
    }
    const failure = serviceFailureWire(kind);
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.write(failure.prefix);
    const timer = setTimeout(() => {
      timers.delete(timer);
      if (failure.destroy) res.destroy(); else res.end(failure.tail);
    }, 20); timers.add(timer);
  });
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  const transport = createNativeLoopbackTransport(server.address().port, { retryDelayMs: 0 });
  const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
  try {
    const reply = await new Promise((done, reject) => {
      const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST', agent: false,
        signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
          'anthropic-version': '2023-06-01', 'content-type': 'application/json' } }, res => {
        let text = ''; res.on('data', chunk => { text += chunk; }); res.on('error', reject);
        res.on('end', () => done({ status: res.statusCode, text }));
      });
      req.once('error', reject); req.end(JSON.stringify(doc));
    });
    assert.equal(reply.status, 200);
    const state = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
      ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
    const row = state.recentRequests.at(-1);
    if (kind === 'flapping-503') {
      assert.equal(sends, 3); assert.equal(row.success, true);
      assert.equal(transport.diagnostics().retries, 2);
      assert.ok(reply.text.includes('message_stop')); assert.ok(reply.text.includes('PUBLIC_SERVICE_RECOVERY_COMPLETE'));
    } else {
      assert.equal(sends, 1); assert.equal(row.failureCategory, expected[kind]);
      assert.equal(row.success, false); assert.equal(transport.diagnostics().retries, 0);
      assert.ok(reply.text.includes('PUBLIC_REPORT_PENDING')); assert.ok(reply.text.includes(expected[kind]));
      assert.ok(!reply.text.includes('message_stop')); assert.ok(!reply.text.includes('tool_use'));
      assert.ok(!reply.text.includes('mcp__fixture__complete_report'));
      assert.equal(row.attempts[0].status, 200); assert.equal(row.attempts[0].completed, false);
      assert.ok(Number.isFinite(row.firstTextDeltaMs));
    }
    assert.equal(row.attempts.length, kind === 'flapping-503' ? 3 : 1); checks++;
  } finally {
    await gateway.close();
    for (const timer of timers) clearTimeout(timer);
    server.closeAllConnections(); await new Promise(done => server.close(done));
    assert.equal(transport.diagnostics().activeRequests, 0);
    assert.equal(transport.diagnostics().activeSockets, 0);
  }
}
assert.throws(() => serviceFailureWire('unknown'), /SERVICE_STIMULUS_INVALID/); checks++;
assert.throws(() => publicServiceEvents('private', 'max', 1, null), /SERVICE_STIMULUS_INVALID/); checks++;
console.log(JSON.stringify({ suite: 'service-faults', checks, httpRequests, externalRequests: 0, credentialReads: 0 }));
