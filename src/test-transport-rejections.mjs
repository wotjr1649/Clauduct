import assert from 'node:assert/strict';
import { test } from 'node:test';
import { connect } from 'node:net';
import { request } from 'node:http';
import { startNativeGateway } from './native-gateway.mjs';
import { requestStatusSnapshot, readRequestStatus } from './request-status.mjs';
import { transportClientErrorCode, TRANSPORT_CLIENT_ERROR_CODES } from './native-protocol.mjs';

// Public wire fixtures only. No native child, upstream, profile or credential loader.
const marker = 'PUBLIC_UNTRUSTED_MARKER';
let upstreamCalls = 0, checks = 0;
const createGateway = () => startNativeGateway({ transport: {
  send: async () => { upstreamCalls++; throw new Error('UNEXPECTED_UPSTREAM'); },
  close: async () => {}, diagnostics: () => ({})
} });
async function wire(gateway, bytes) {
  return new Promise((resolve, reject) => {
    const socket = connect({ host: '127.0.0.1', port: gateway.port });
    let response = '', failure;
    const timer = setTimeout(() => { failure = new Error('FIXTURE_TIMEOUT'); socket.destroy(); }, 2000);
    socket.once('connect', () => socket.write(bytes));
    socket.on('data', chunk => {
      response += chunk.toString('latin1');
      if (response.length > 4096) { failure = new Error('FIXTURE_OUTPUT_LIMIT'); socket.destroy(); }
      // These rejection replies are chunked and Connection: close. Let the peer read
      // the complete frame before closing, as an HTTP client does on Windows.
      if (response.endsWith('\r\n0\r\n\r\n')) socket.end();
    });
    socket.on('error', error => { if (error.code !== 'ECONNRESET') failure = new Error('FIXTURE_SOCKET_ERROR'); });
    socket.once('close', () => { clearTimeout(timer); if (failure) reject(failure); else resolve(response); });
  });
}
async function head(gateway) {
  await new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port: gateway.port, method: 'HEAD', path: '/api/hello',
      agent: false, signal: AbortSignal.timeout(2000) }, res => {
      res.resume(); res.once('error', reject);
      res.once('end', () => { try { assert.equal(res.statusCode, 204); resolve(); } catch (error) { reject(error); } });
    });
    req.once('error', reject); req.end();
  });
}
async function fixture(action) {
  const gateway = await createGateway();
  try { await action(gateway); }
  finally {
    const closed = await gateway.close();
    assert.equal(closed.cleanupFailed, false);
    assert.equal(closed.activeSockets, 0);
    assert.equal(closed.activeJobs, 0);
    assert.equal(closed.activeTimers, 0);
  }
}
function fixtures(gateway) {
  const host = `Host: 127.0.0.1:${gateway.port}\r\n`;
  return [
    ['checkContinue', `POST /${marker} HTTP/1.1\r\n${host}Expect: 100-continue\r\nContent-Length: ${marker.length}\r\n\r\n${marker}`, null],
    ['checkExpectation', `POST /${marker} HTTP/1.1\r\n${host}Expect: ${marker}\r\nContent-Length: ${marker.length}\r\n\r\n${marker}`, null],
    ['connect', `CONNECT ${marker}:443 HTTP/1.1\r\n${host}\r\n`, null],
    ['upgrade', `GET /${marker} HTTP/1.1\r\n${host}Connection: Upgrade\r\nUpgrade: ${marker}\r\n\r\n`, null],
    ['clientError', `${marker} / HTTP/1.1\r\n${host}\r\n`, 'HPE_INVALID_METHOD'],
    ['clientError', `GET / HTTP/1.1\r\n${host}X-${marker}: ${'x'.repeat(17000)}\r\n\r\n`, 'HPE_HEADER_OVERFLOW']
  ];
}
for (let index = 0; index < 6; index++) {
  const name = ['checkContinue', 'checkExpectation', 'connect', 'upgrade', 'invalid-method', 'header-overflow'][index];
  await test(`real loopback rejection: ${name}`, () => fixture(async gateway => {
    const [event, bytes, code] = fixtures(gateway)[index];
    const before = gateway.diagnostics();
    const reply = await wire(gateway, bytes);
    if (event.startsWith('check')) assert.equal(/^HTTP\/1\.1 417 /.test(reply), true,
      JSON.stringify({ event, counts: gateway.diagnostics().lifetime.transportRejectionsByEvent }));
    else assert.equal(reply.length, 0);
    const after = gateway.diagnostics();
    assert.equal(after.lifetime.transportRejections, before.lifetime.transportRejections + 1);
    assert.equal(after.lifetime.transportRejectedConnections, before.lifetime.transportRejectedConnections + 1);
    assert.equal(after.lifetime.transportRejectionsByEvent[event], (before.lifetime.transportRejectionsByEvent[event] ?? 0) + 1);
    if (code) assert.equal(after.lifetime.transportClientErrorsByCode[code], 1);
    assert.equal(after.requests, before.requests);
    assert.equal(after.lifetime.rejectedBeforeStart, 0);
    assert.equal(after.lifetime.failed, 0);
    assert.equal(after.lifetime.started, 0);
    assert.equal(upstreamCalls, 0);
    assert.ok(!JSON.stringify(after).includes(marker)); checks++;
    const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
      ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
    assert.deepEqual(status.lifetime.transportRejectionsByEvent, { [event]: 1 });
    assert.deepEqual(status.lifetime.transportClientErrorsByCode, code ? { [code]: 1 } : {});
    assert.deepEqual(requestStatusSnapshot(status).lifetime, status.lifetime);
    const snapshot = gateway.diagnostics();
    snapshot.lifetime.transportRejectionsByEvent[event] = 999;
    snapshot.lifetime.transportClientErrorsByCode.HPE_INVALID_METHOD = 999;
    assert.equal(gateway.diagnostics().lifetime.transportRejectionsByEvent[event], 1);
    assert.notEqual(gateway.diagnostics().lifetime.transportClientErrorsByCode.HPE_INVALID_METHOD, 999);
  }));
}
await test('two rejection events on one connection remain two events', () => fixture(async gateway => {
  // Two rejected events can come from one socket: retain the old event total.
  const before = gateway.diagnostics().lifetime;
  const bytes = fixtures(gateway)[0][1];
  await wire(gateway, bytes + bytes);
  const after = gateway.diagnostics().lifetime;
  assert.equal(after.transportRejections, before.transportRejections + 2);
  assert.equal(after.transportRejectedConnections, before.transportRejectedConnections + 1); checks++;
}));
await test('normal hello and status do not reject transport or invoke upstream', () => fixture(async gateway => {
  await head(gateway);
  const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
    ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
  assert.deepEqual(status.lifetime.transportRejectionsByEvent, {});
  assert.deepEqual(status.lifetime.transportClientErrorsByCode, {});
  assert.equal(status.lifetime.transportRejections, 0);
  assert.equal(status.lifetime.transportRejectedConnections, 0);
  assert.equal(status.requestOutcome, 'no-requests');
  assert.deepEqual(status.failureHistory.records, []);
  assert.ok(!JSON.stringify(status).includes(marker));
  assert.deepEqual(requestStatusSnapshot(status).lifetime, status.lifetime); checks++;
  assert.equal(upstreamCalls, 0);
}));
await test('legacy and hostile status projections remain bounded and secret-free', () => {
  for (const code of TRANSPORT_CLIENT_ERROR_CODES) assert.equal(transportClientErrorCode(code), code);
  for (const code of [undefined, null, marker, `HPE_${marker}`, {}, 1]) {
    assert.equal(transportClientErrorCode(code), 'OTHER');
  }
  // A legacy snapshot lacks this evidence; it must not claim zero observed events.
  const legacy = requestStatusSnapshot({ recentRequests: [], lifetime: { transportRejections: 2 } });
  assert.equal(legacy.lifetime.transportRejections, 2);
  assert.equal(legacy.lifetime.transportRejectionsByEvent, null);
  assert.equal(legacy.lifetime.transportClientErrorsByCode, null);
  assert.equal(legacy.lifetime.transportRejectedConnections, null); checks++;
  const hostile = requestStatusSnapshot({ recentRequests: [], lifetime: {
    transportRejectedConnections: -1,
    transportRejectionsByEvent: { [marker]: 1, clientError: 2, connect: -1, upgrade: 1.5, checkContinue: marker },
    transportClientErrorsByCode: { [marker]: 1, HPE_INVALID_METHOD: 2, OTHER: 1, ECONNRESET: Infinity,
      HPE_HEADER_OVERFLOW: Number.MAX_SAFE_INTEGER + 1 }
  } });
  assert.deepEqual(hostile.lifetime.transportRejectionsByEvent, { clientError: 2 });
  assert.deepEqual(hostile.lifetime.transportClientErrorsByCode, { HPE_INVALID_METHOD: 2, OTHER: 1 });
  assert.equal(hostile.lifetime.transportRejectedConnections, null);
  assert.ok(!JSON.stringify(hostile).includes(marker)); checks++;
  for (const malformed of [null, [], marker, 1]) {
    const projected = requestStatusSnapshot({ recentRequests: [], lifetime: {
      transportRejectionsByEvent: malformed, transportClientErrorsByCode: malformed } });
    assert.equal(projected.lifetime.transportRejectionsByEvent, null);
    assert.equal(projected.lifetime.transportClientErrorsByCode, null); checks++;
  }
});
console.log(JSON.stringify({ suite: 'transport-rejections', checks, upstreamCalls, externalRequests: 0 }));
