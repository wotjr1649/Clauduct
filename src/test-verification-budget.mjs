import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { launchOptions } from './clauduct.mjs';

let checks = 0;
for (const value of ['0', '-1', '4097', '1.5', 'Infinity', '']) {
  assert.throws(() => launchOptions([`--verify-request-limit=${value}`]), /INVALID_ARGUMENTS/); checks++;
}
assert.equal(launchOptions(['--verify-request-limit', '16']).verifyRequestLimit, 16); checks++;
let received = 0, credentialReads = 0;
const server = createServer((_req, res) => { received++; res.writeHead(503); res.end(); });
await new Promise(done => server.listen(0, '127.0.0.1', done));
const transport = createNativeLoopbackTransport(server.address().port, { requestBudget: 1, retryDelayMs: 0,
  credentialSupplier: () => { credentialReads++; return { accessToken: 'synthetic', account: 'synthetic' }; } });
try {
  // A failed first attempt must consume the budget. Even its automatic retry is
  // rejected before creating a second request, regardless of the failure cause.
  await assert.rejects(transport.send({}, AbortSignal.timeout(5000)), error => error.code === 'REQUEST_BUDGET');
  assert.equal(transport.diagnostics().requestAttempts, 1);
  assert.equal(received, 1); checks++;
  const before = credentialReads;
  await assert.rejects(transport.send({}, AbortSignal.timeout(5000)), error => error.code === 'REQUEST_BUDGET'); checks++;
  await assert.rejects(transport.search({}, AbortSignal.timeout(5000)), error => error.code === 'REQUEST_BUDGET'); checks++;
  assert.equal(credentialReads, before); assert.equal(received, 1);
} finally { await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done)); }
console.log(JSON.stringify({ suite: 'verification-budget', checks, received, actualCredentialReads: 0, externalRequests: 0 }));
