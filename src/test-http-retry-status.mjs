import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';

const rows = [], failures = [];
for (const method of ['send', 'search']) for (const status of [429, 499, 500, 599, 600, 601, 999]) {
  let requests = 0;
  const server = createServer((req, res) => { requests++; req.resume(); res.writeHead(status); res.end(); });
  await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
  const transport = createNativeLoopbackTransport(server.address().port, { retryDelayMs: 0, requestBudget: 6 });
  let category = null;
  try {
    await transport[method]({}, AbortSignal.timeout(3000));
  } catch (error) {
    category = ['RATE_LIMITED', 'UPSTREAM_HTTP_ERROR', 'SEARCH_HTTP_ERROR'].includes(error.code) ? error.code : 'UNEXPECTED_ERROR';
  } finally { await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done)); }
  const retryable = status === 429 || status >= 500 && status <= 599;
  const expected = retryable ? (method === 'send' ? 6 : 2) : 1;
  const diagnostics = transport.diagnostics();
  const row = { method, status, requests, attempts: diagnostics.requestAttempts, category,
    closed: diagnostics.activeRequests === 0 && diagnostics.activeSockets === 0 };
  if (requests !== expected || diagnostics.requestAttempts !== expected || !row.closed
    || category !== (method === 'search' ? 'SEARCH_HTTP_ERROR' : status === 429 ? 'RATE_LIMITED' : 'UPSTREAM_HTTP_ERROR')) {
    failures.push({ method, status, requests, attempts: diagnostics.requestAttempts, expected, category });
  }
  rows.push(row);
}
console.log(JSON.stringify({ suite: 'http-retry-status', checks: rows.length, failures, rows, externalRequests: 0, actualCredentialReads: 0 }));
assert.equal(failures.length, 0, 'HTTP_RETRY_CLASS_MISMATCH');
