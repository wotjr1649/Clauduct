import assert from 'node:assert/strict';
import { Agent, createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus, requestStatusSnapshot } from './request-status.mjs';

// Controlled Node connection errors exercise the real request/retry boundary.
// No certificate trust settings, DNS configuration or actual credentials change.
const tlsCodes = ['DEPTH_ZERO_SELF_SIGNED_CERT', 'SELF_SIGNED_CERT_IN_CHAIN', 'CERT_HAS_EXPIRED',
  'CERT_NOT_YET_VALID', 'CERT_REVOKED', 'CERT_SIGNATURE_FAILURE', 'CERT_UNTRUSTED', 'CERT_REJECTED',
  'UNABLE_TO_VERIFY_LEAF_SIGNATURE', 'UNABLE_TO_GET_ISSUER_CERT', 'UNABLE_TO_GET_ISSUER_CERT_LOCALLY',
  'INVALID_CA', 'PATH_LENGTH_EXCEEDED', 'ERR_TLS_CERT_ALTNAME_INVALID', 'ERR_SSL_WRONG_VERSION_NUMBER'];
const cases = [
  ...tlsCodes.map(code => ({ code, category: 'UPSTREAM_TLS_ERROR', attempts: 1 })),
  ...['EACCES', 'EPERM'].map(code => ({ code, category: 'UPSTREAM_ACCESS_DENIED', attempts: 1 })),
  ...['EAI_AGAIN', 'ENOTFOUND'].map(code => ({ code, category: 'UPSTREAM_DNS_ERROR', attempts: 6 })),
  ...['ECONNREFUSED', 'ECONNRESET', 'SYNTHETIC_PRIVATE'].map(code => ({ code, category: 'UPSTREAM_IO_ERROR', attempts: 6 })),
  { code: 'HPE_INVALID_CONSTANT', category: 'UPSTREAM_IO_ERROR', attempts: 1 },
  { code: { toString() { throw new Error('UNTRUSTED_COERCION'); } }, category: 'UPSTREAM_IO_ERROR', attempts: 6 }
];
const listener = createServer((req, res) => {
  arrivals++; let body = '';
  req.setEncoding('utf8');
  req.on('data', chunk => { body += chunk; if (body.length > 1024) req.destroy(); });
  req.on('end', () => {
    originalTaskMatched = body === JSON.stringify({ task: 'PUBLIC_NETWORK_BOUNDARY' });
    res.writeHead(200, { 'content-type': 'text/event-stream' });
    res.end('data: {"type":"response.created"}\n\ndata: {"type":"response.completed"}\n\ndata: [DONE]\n\n');
  });
});
let arrivals = 0, connections = 0, injected, passed = 0, failureLimit = Infinity, originalTaskMatched = false, asynchronous = false;
await new Promise((done, reject) => { listener.once('error', reject); listener.listen(0, '127.0.0.1', done); });
const port = listener.address().port;
const original = Agent.prototype.createConnection;
Agent.prototype.createConnection = function (options, ...args) {
  if (Number(options.port) !== port) return original.call(this, options, ...args);
  connections++;
  if (connections > failureLimit) return original.call(this, options, ...args);
  const error = Object.assign(new Error('SYNTHETIC_PRIVATE'), { code: injected });
  if (asynchronous) { assert.equal(typeof args[0], 'function'); queueMicrotask(() => args[0](error)); return; }
  throw error;
};
try {
  for (const item of cases) {
    connections = 0; injected = item.code;
    const transport = createNativeLoopbackTransport(port, { retryDelayMs: 0, requestBudget: 8 });
    const attempts = [];
    try {
      await assert.rejects(transport.send({ task: 'PUBLIC_NETWORK_BOUNDARY' }, AbortSignal.timeout(2000), { attemptTimings: attempts }), error => {
        assert.equal(error.code, item.category);
        assert.equal(error.message, item.category);
        assert.equal(error.retryable, item.attempts > 1);
        assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
        return true;
      });
      assert.equal(connections, item.attempts);
      assert.equal(attempts.length, item.attempts);
      assert.ok(attempts.every(row => row.status === null && row.completed === false && row.endedMs !== null));
      assert.ok(attempts.every(row => row.failureCategory === item.category));
      const state = transport.diagnostics();
      assert.equal(state.requestAttempts, item.attempts); assert.equal(state.retries, item.attempts - 1);
      assert.equal(state.category, item.category); assert.equal(state.activeRequests, 0); assert.equal(state.activeSockets, 0);
      assert.ok(!JSON.stringify(state).includes('SYNTHETIC_PRIVATE')); passed++;
    } finally { await transport.close(); }
  }
  asynchronous = true;
  for (const item of cases) {
    connections = 0; injected = item.code;
    const category = item.category === 'UPSTREAM_IO_ERROR' ? 'SEARCH_HTTP_ERROR' : item.category;
    const expectedAttempts = item.attempts > 1 ? 2 : 1;
    const transport = createNativeLoopbackTransport(port, { retryDelayMs: 0, requestBudget: 2 });
    try {
      await assert.rejects(transport.search({ task: 'PUBLIC_NETWORK_BOUNDARY' }, AbortSignal.timeout(2000)), error => {
        assert.equal(error.code, category); assert.equal(error.message, category);
        assert.equal(error.retryable, expectedAttempts > 1);
        assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE')); return true;
      });
      assert.equal(connections, expectedAttempts);
      assert.equal(transport.diagnostics().requestAttempts, expectedAttempts);
      assert.equal(transport.diagnostics().retries, expectedAttempts - 1);
      assert.equal(transport.diagnostics().category, category);
      assert.equal(transport.diagnostics().activeRequests, 0);
      assert.equal(transport.diagnostics().activeSockets, 0); passed++;
    } finally { await transport.close(); }
  }
  asynchronous = false;
  connections = 0; injected = 'EAI_AGAIN'; failureLimit = 1;
  const recovered = createNativeLoopbackTransport(port, { retryDelayMs: 0, requestBudget: 2 });
  try {
    const attempts = [];
    const result = await recovered.send({ task: 'PUBLIC_NETWORK_BOUNDARY' }, AbortSignal.timeout(2000), { attemptTimings: attempts });
    assert.equal(result.at(-1).type, 'response.completed'); assert.equal(arrivals, 1); assert.equal(connections, 2);
    assert.equal(originalTaskMatched, true);
    assert.deepEqual(attempts.map(row => row.failureCategory), ['UPSTREAM_DNS_ERROR', null]);
    assert.deepEqual(attempts.map(row => row.completed), [false, true]);
    assert.equal(recovered.diagnostics().retries, 1); passed++;
  } finally { await recovered.close(); }
  const safe = requestStatusSnapshot({ recentRequests: [{ attempts: [
    { failureCategory: 'UPSTREAM_DNS_ERROR' }, { failureCategory: 'SYNTHETIC_PRIVATE' }, { failureCategory: {} }
  ] }] });
  assert.deepEqual(safe.recentRequests[0].attempts.map(row => row.failureCategory), ['UPSTREAM_DNS_ERROR', null, null]);
  assert.ok(!JSON.stringify(safe).includes('SYNTHETIC_PRIVATE')); passed++;
  // The native client and bounded diagnostic endpoint see the same fixed label.
  connections = 0; injected = 'DEPTH_ZERO_SELF_SIGNED_CERT'; failureLimit = Infinity;
  const transport = createNativeLoopbackTransport(port, { retryDelayMs: 0, requestBudget: 8 });
  const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
  try {
    const endpoint = `http://127.0.0.1:${gateway.port}`;
    const response = await fetch(`${endpoint}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(3000),
      headers: { ...gateway.clientHeaders(), 'content-type': 'application/json', 'anthropic-version': '2023-06-01' },
      body: JSON.stringify({ model: 'luna', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'PUBLIC_NETWORK_BOUNDARY' }] }) });
    const text = await response.text();
    assert.equal(response.status, 502); assert.ok(text.includes('UPSTREAM_TLS_ERROR'));
    assert.ok(!text.includes('message_stop')); assert.ok(!text.includes('SYNTHETIC_PRIVATE'));
    const status = await readRequestStatus({ ANTHROPIC_BASE_URL: endpoint,
      ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
    assert.equal(status.requestOutcome, 'has-failures'); assert.equal(status.lifetime.failed, 1);
    assert.equal(status.recentRequests.at(-1).failureCategory, 'UPSTREAM_TLS_ERROR');
    assert.equal(status.recentRequests.at(-1).attempts[0].failureCategory, 'UPSTREAM_TLS_ERROR');
    assert.equal(status.recentRequests.at(-1).attempts.length, 1); assert.equal(connections, 1);
    passed++;
  } finally { await gateway.close(); }
  assert.equal(arrivals, 1);
  console.log(JSON.stringify({ suite: 'network-failures', passed, actualConnectionErrorInjection: true,
    actualTlsHandshake: false, upstreamHttpArrivals: arrivals, externalRequests: 0, actualCredentialReads: 0 }));
} finally {
  Agent.prototype.createConnection = original;
  listener.closeAllConnections(); await new Promise(done => listener.close(done));
}
