import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { checkClientVersion, entrySummary, safeEntryCategory } from '../poc/user-session.mjs';
import { createLoopbackCodexTransport, createCodexTransport } from '../poc/codex-transport.mjs';
import { createNativeLoopbackTransport, createNativeTransport } from './native-transport.mjs';
import { probeProfile } from '../poc/adapter.mjs';
import { liteSearchEnvelope } from './native-protocol.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';

// A detected update is not a query failure or a claim of compatibility.
assert.equal(checkClientVersion({ status: 0, stdout: 'codex-cli 0.154.0\r\n' }), '0.154.0');
assert.equal(checkClientVersion({ status: 0, stdout: 'codex-cli 0.153.4\n' }), '0.153.4');
for (const result of [undefined, { error: Error('SYNTHETIC_PRIVATE'), status: 0, stdout: 'codex-cli 0.154.0' },
  { status: 1, stdout: 'codex-cli 0.154.0' }, { status: null }, { status: 0, signal: 'SIGTERM' }]) {
  assert.throws(() => checkClientVersion(result), e => safeEntryCategory(e) === 'CLI_VERSION_UNAVAILABLE');
}
for (const stdout of ['', '0.154.0', 'codex-cli unknown', 'codex-cli 00.154.0',
  'codex-cli 0.154.0\nExtra: value', 'codex-cli 0.154.0 extra', 'codex-cli ' + '1'.repeat(5000)]) {
  assert.throws(() => checkClientVersion({ status: 0, stdout }), e => e.code === 'CLI_VERSION_INVALID'
    && !e.message.includes('SYNTHETIC_PRIVATE'));
}
for (const factory of [createCodexTransport, createNativeTransport]) {
  for (const clientVersion of [undefined, '', '0.154.0\n', '0.154.0\r\nX: bad', 'SYNTHETIC_PRIVATE']) {
    assert.throws(() => factory({ credential: {}, clientVersion }), e => e.code === 'CLI_VERSION_INVALID');
  }
}
// Valid live construction is intentionally not exercised here: checkRuntime
// rejects the restricted test process. Do not remove its protection to test it.

const received = [];
const server = createServer((req, res) => {
  received.push({ version: req.headers.version, agent: req.headers['user-agent'],
    originator: req.headers.originator, beta: req.headers['openai-beta'] });
  req.resume();
  res.writeHead(200, { 'content-type': 'text/event-stream' });
  res.end('event: response.created\ndata: {"type":"response.created"}\n\nevent: response.completed\ndata: {"type":"response.completed"}\n\ndata: [DONE]\n\n');
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
try {
  for (const clientVersion of ['0.153.4', '0.154.0', '0.155.0-beta.1']) {
    const expectedStatus = clientVersion === '0.153.4' ? 'reference' : 'unverified';
    const options = { clientVersion, profile: 'astra-low' };
    const transports = [createNativeLoopbackTransport(server.address().port, options),
      createLoopbackCodexTransport(server.address().port, options)];
    options.clientVersion = '9.9.9'; // Already-created senders keep their creation-time version.
    try {
      for (let index = 0; index < transports.length; index++) {
        const transport = transports[index];
        for (let attempt = 0; attempt < 2; attempt++) {
          const body = { model: probeProfile('astra-low').model, reasoning: { effort: 'low' }, stream: true, store: false };
          if (index === 0) await transport.send(body, AbortSignal.timeout(5000));
          else await transport.send(JSON.stringify(body), { begin() {}, push() {} }, AbortSignal.timeout(5000));
          assert.deepEqual(received.at(-1), { version: clientVersion,
            agent: `codex-cli/${clientVersion} (Windows; x64)`,
            originator: 'codex_cli_rs', beta: 'responses=experimental' });
        }
        const diagnostics = transport.diagnostics();
        assert.equal(diagnostics.clientVersion, clientVersion);
        assert.equal(diagnostics.clientVersionStatus, expectedStatus);
        assert.equal(entrySummary('USER_CANCELLED', { transport: diagnostics }).clientVersion, clientVersion);
        if (index === 0) {
          const gateway = await startNativeGateway({ transport });
          try {
            const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
              ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
            assert.equal(status.clientVersion, clientVersion);
            assert.equal(status.clientVersionStatus, expectedStatus);
          } finally { await gateway.close(); }
        }
      }
    } finally { await Promise.all(transports.map(t => t.close())); }
  }
  // A search request identifies itself as the reference client, because the lite envelope sends
  // no instructions and the backend picks them — and the built-in toolset with them — from that
  // identity. Carrying one of the fixed envelope headers is what selects it; an ordinary request
  // carries none and keeps the identity every other request has always sent.
  const transport = createNativeLoopbackTransport(server.address().port, { clientVersion: '0.154.0', profile: 'astra-low' });
  try {
    const body = { model: probeProfile('astra-low').model, reasoning: { effort: 'low' }, stream: true, store: false };
    await transport.send(body, AbortSignal.timeout(5000), { headers: liteSearchEnvelope().headers });
    const lite = received.at(-1);
    assert.equal(lite.originator, 'codex_exec');
    assert.equal(lite.beta, undefined);
    assert.equal(lite.version, undefined);
    assert.match(lite.agent, /^codex_exec\/0\.154\.0 \(.+\) xterm-256color \(codex_exec; 0\.154\.0\)$/);
    await transport.send(body, AbortSignal.timeout(5000));
    assert.equal(received.at(-1).originator, 'codex_cli_rs');
    assert.equal(received.at(-1).beta, 'responses=experimental');
  } finally { await transport.close(); }
} finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }
assert.equal(entrySummary('USER_CANCELLED', {}).clientVersion, null);
assert.equal(entrySummary('USER_CANCELLED', { transport: { clientVersion: 'SYNTHETIC_PRIVATE' } }).clientVersion, null);
console.log(JSON.stringify({ suite: 'client-version', passed: true, loopbackRequests: received.length,
  externalRequests: 0, credentialReads: 0 }));
