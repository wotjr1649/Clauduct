import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { parseRetryAfterSeconds } from './retry-after-seconds.mjs';
import { parseRetryAfter } from './retry-after.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { requestStatusSnapshot } from './request-status.mjs';

let checks = 0;
const now = Date.UTC(2026, 8, 13);
for (const [header, delay] of [['0', 0], ['60', 60000], ['300', 300000], [' \t00060\t ', 60000]]) {
  assert.equal(parseRetryAfterSeconds(header), delay);
  assert.deepEqual(parseRetryAfter(header, now), { delayMs: delay, retryAtMs: now + delay }); checks++;
}
for (const header of ['60\n', '60\r', '60\r\n', '\u00a060', '1.5', '1e3', '+1', '-1', '', ' ', '0x10', 'Infinity', ['60'], null, 60]) {
  assert.equal(parseRetryAfterSeconds(header), null); assert.equal(parseRetryAfter(header, now), null); checks++;
}
for (const header of ['9007199254740', '9007199254741', '9'.repeat(129)]) {
  assert.deepEqual(parseRetryAfter(header, now), { unrepresentable: true }); checks++;
}
assert.equal(parseRetryAfterSeconds('9'.repeat(129)), null); checks++;
assert.deepEqual(parseRetryAfter('0'.repeat(300) + '60', now), { delayMs: 60000, retryAtMs: now + 60000 }); checks++;
for (const header of ['Sun, 13 Sep 2026 00:01:00 GMT', 'Sunday, 13-Sep-26 00:01:00 GMT', 'Sun Sep 13 00:01:00 2026']) {
  assert.deepEqual(parseRetryAfter(header, now), { delayMs: 60000, retryAtMs: now + 60000 }); checks++;
}
assert.deepEqual(parseRetryAfter('Sunday, 06-Nov-94 08:49:37 GMT', now), { delayMs: 0, retryAtMs: now }); checks++;
assert.equal(parseRetryAfter('Tuesday, 01-Jan-75 00:00:00 GMT', now).retryAtMs, Date.UTC(2075, 0, 1)); checks++;
assert.equal(parseRetryAfter('Thu, 31 Dec 2026 23:59:60 GMT', now).retryAtMs, Date.UTC(2027, 0, 1)); checks++;
for (const header of ['2026-09-13', 'Sun, 31 Feb 2026 00:00:00 GMT', 'Sun, 13 Sep 2026 24:00:00 GMT',
  'Sun, 13 Sep 2026 00:60:00 GMT', 'Sun, 13 Sep 2026 00:00:61 GMT', 'Sun, 13 Sep 2026 00:00:00 UTC', 'Sun, 13 Sep 2026 00:00:00 GMT\n']) {
  assert.equal(parseRetryAfter(header, now), null); checks++;
}
const good = 'data: {"type":"response.created"}\n\ndata: {"type":"response.completed"}\n\ndata: [DONE]\n\n';
async function fixture(handler, operation) {
  const server = createServer(handler);
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  let credentialReads = 0;
  const transport = createNativeLoopbackTransport(server.address().port, { retryDelayMs: 0, requestBudget: 8,
    credentialSupplier: () => { credentialReads++; return { accessToken: 'synthetic', account: 'synthetic' }; } });
  try { await operation(transport, () => credentialReads); }
  finally { await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done)); }
}
for (const kind of ['send', 'search']) for (const header of ['60', '300', new Date(Date.now() + 300000).toUTCString()]) {
  let received = 0;
  await fixture((_req, res) => { received++; res.writeHead(429, { 'Retry-After': header }); res.end(); }, async (transport, credentialReads) => {
    const before = Date.now(); let deferred;
    await assert.rejects(transport[kind]({}, AbortSignal.timeout(3000)), error => {
      deferred = error; return error.code === 'UPSTREAM_RETRY_DEFERRED';
    });
    assert.ok(Date.now() - before < 2000);
    assert.ok(deferred.retryAtMs >= before + (header === '60' ? 59000 : 298000));
    assert.equal(transport.diagnostics().retryNotBeforeMs, deferred.retryAtMs);
    const reads = credentialReads();
    for (const operation of ['send', 'search']) await assert.rejects(transport[operation]({}, AbortSignal.timeout(1000)), error => error.code === 'UPSTREAM_RETRY_DEFERRED');
    assert.equal(credentialReads(), reads); assert.equal(received, 1); assert.equal(transport.diagnostics().requestAttempts, 1);
    assert.equal(transport.diagnostics().retries, 0); checks++;
  });
}
for (const kind of ['send', 'search']) {
  let firstAt = 0, nextAt = 0, received = 0;
  await fixture((_req, res) => {
    if (++received === 1) { firstAt = Date.now(); res.writeHead(429, { 'Retry-After': '1' }); res.end(); }
    else { nextAt = Date.now(); res.writeHead(200, { 'Content-Type': kind === 'send' ? 'text/event-stream' : 'application/json' }); res.end(kind === 'send' ? good : '{"ok":true}'); }
  }, async transport => {
    await transport[kind]({}, AbortSignal.timeout(5000));
    assert.equal(received, 2); assert.ok(nextAt - firstAt >= 1000); checks++;
  });
}
// A syntactically valid delay outside the numeric deadline range must not
// become an ignored header followed by an early retry, including later calls.
for (const kind of ['send', 'search']) {
  let received = 0;
  await fixture((_req, res) => { received++; res.writeHead(503, { 'Retry-After': '9'.repeat(129) }); res.end(); }, async (transport, credentialReads) => {
    await assert.rejects(transport[kind]({}, AbortSignal.timeout(1000)), { code: 'UPSTREAM_RETRY_UNREPRESENTABLE' });
    const reads = credentialReads();
    for (const operation of ['send', 'search']) await assert.rejects(transport[operation]({}, AbortSignal.timeout(1000)), { code: 'UPSTREAM_RETRY_UNREPRESENTABLE' });
    assert.equal(received, 1); assert.equal(credentialReads(), reads);
    assert.equal(transport.diagnostics().retryAfterUnrepresentable, true);
    assert.equal(transport.diagnostics().activeRequests, 0);
    // A fully consumed search response can leave a reusable idle connection.
    // Assert socket release after the transport's explicit close boundary.
    await transport.close(); assert.equal(transport.diagnostics().activeSockets, 0); checks++;
  });
}
// The native gateway must preserve the deadline in its bounded status projection.
await fixture((_req, res) => { res.writeHead(503, { 'Retry-After': '300' }); res.end(); }, async transport => {
  const gateway = await startNativeGateway({ transport });
  try {
    const body = JSON.stringify({ model: 'gpt-5.6-luna', stream: true, max_tokens: 100,
      messages: [{ role: 'user', content: 'PUBLIC_RETRY_FIXTURE' }] });
    const response = await new Promise((done, fail) => {
      const req = request(`http://127.0.0.1:${gateway.port}/v1/messages`, { method: 'POST', agent: false,
        signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(), 'Content-Type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
        let text = ''; res.setEncoding('utf8'); res.on('data', value => { text += value; });
        res.once('end', () => done({ status: res.statusCode, text })); res.once('error', fail);
      }); req.once('error', fail); req.end(body);
    });
    assert.equal(response.status, 429); assert.match(response.text, /UPSTREAM_RETRY_DEFERRED/);
    const row = requestStatusSnapshot(gateway.diagnostics()).recentRequests[0];
    assert.equal(row.failureCategory, 'UPSTREAM_RETRY_DEFERRED'); assert.ok(row.retryAtMs > Date.now() + 298000);
    assert.equal(row.attempts.length, 1); checks++;
  } finally { await gateway.close(); }
});
console.log(JSON.stringify({ suite: 'retry-after', checks, longDelayEarlyRetries: 0, actualOneSecondWaits: 2,
  full60And300SecondRecovery: 'NOT_RUN', externalRequests: 0, actualCredentialReads: 0 }));
