import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { test } from 'node:test';
import { createNativeLoopbackTransport, createNativeTransport } from './native-transport.mjs';
import { REFERENCE_CLIENT_VERSION } from './client-version.mjs';

test('production factory forwards the response observer option', {
  skip: 'checkRuntime denied this factory probe with DEBUG_RUNTIME_UNSUPPORTED; no alternate probe execution'
}, async () => {
  let production;
  try {
    assert.throws(() => { production = createNativeTransport({ clientVersion: REFERENCE_CLIENT_VERSION,
      credential: { accessToken: 'synthetic', account: 'synthetic' }, onResponseLimits: true }); }, error => error.code === 'INVALID_OPTIONS');
  } finally { await production?.close(); }
});

const headers = {
  'x-codex-primary-used-percent': '12.5', 'x-codex-primary-window-minutes': '300', 'x-codex-primary-reset-at': '1800000000',
  'x-codex-secondary-used-percent': '37', 'x-codex-secondary-window-minutes': '10080', 'x-codex-secondary-reset-at': '1800500000'
};
const observation = { state: 'observed', primary: { usedPercent: 12.5, windowMinutes: 300, resetAtSeconds: 1800000000 },
  secondary: { usedPercent: 37, windowMinutes: 10080, resetAtSeconds: 1800500000 }, otherLimitFamilies: 0 };
const wire = 'data: {"type":"response.created"}\n\ndata: {"type":"response.completed"}\n\n';
const observations = [];
let requests = 0;
const server = createServer((req, res) => {
  requests++; req.resume();
  res.writeHead(200, { 'Content-Type': 'text/event-stream', ...headers, 'x-public-omitted': 'SYNTHETIC_PRIVATE' }); res.end(wire);
});
await new Promise(done => server.listen(0, '127.0.0.1', done));
const transport = createNativeLoopbackTransport(server.address().port, { requestBudget: 1,
  onResponseLimits: value => { observations.push(value); } });
try {
  await transport.send({}, AbortSignal.timeout(2000), { onEvent: () => {
    assert.equal(observations.length, 1, 'headers must be recorded before each event');
  } });
  assert.equal(observations.length, 1, 'response limits must be observed before completion');
  assert.deepEqual(observations[0], { requestAttempt: 1, kind: 'responses', httpStatus: 200, observation });
  assert.ok(!JSON.stringify(observations).includes('SYNTHETIC_PRIVATE'));
  assert.equal(transport.diagnostics().requestAttempts, 1);
} finally { await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done)); }
let checks = 4;
for (const kind of ['responses', 'search']) for (const failure of ['throw', 'async']) {
  let calls = 0, deliveries = 0, credentialCalls = 0;
  const upstream = createServer((req, res) => {
    requests++; req.resume();
    res.writeHead(200, { 'Content-Type': kind === 'search' ? 'application/json' : 'text/event-stream', ...headers });
    res.end(kind === 'search' ? '{"output":"PUBLIC_SEARCH","results":[]}' : wire);
  });
  await new Promise(done => upstream.listen(0, '127.0.0.1', done));
  const guarded = createNativeLoopbackTransport(upstream.address().port, { requestBudget: 3,
    credentialSupplier: async () => { credentialCalls++; return { accessToken: 'synthetic', account: 'synthetic' }; }, onResponseLimits: () => {
    calls++;
    if (failure === 'throw') throw new Error('SYNTHETIC_PRIVATE');
    return Promise.reject(new Error('SYNTHETIC_PRIVATE'));
  } });
  try {
    const work = () => kind === 'search' ? guarded.search({}, AbortSignal.timeout(2000))
      : guarded.send({}, AbortSignal.timeout(2000), { onEvent: () => { deliveries++; } });
    await assert.rejects(work(), error => error.code === 'RESPONSE_OBSERVER_FAILED' && !String(error).includes('SYNTHETIC_PRIVATE'));
    await assert.rejects(guarded.send({}, AbortSignal.timeout(2000)), error => error.code === 'RESPONSE_OBSERVER_FAILED');
    await assert.rejects(guarded.search({}, AbortSignal.timeout(2000)), error => error.code === 'RESPONSE_OBSERVER_FAILED');
    assert.equal(calls, 1); assert.equal(deliveries, 0);
    assert.equal(guarded.diagnostics().requestAttempts, 1); assert.equal(credentialCalls, 1); checks += 7;
  } finally { await guarded.close(); upstream.closeAllConnections(); await new Promise(done => upstream.close(done)); }
}
for (const status of [200, 401, 429]) {
  const received = [];
  const upstream = createServer((req, res) => {
    requests++; req.resume();
    res.writeHead(status, { 'Content-Type': 'application/json', ...headers, ...(status === 429 ? { 'Retry-After': '300' } : {}) });
    res.end('{"output":"PUBLIC_SEARCH","results":[]}');
  });
  await new Promise(done => upstream.listen(0, '127.0.0.1', done));
  const search = createNativeLoopbackTransport(upstream.address().port, { requestBudget: 2,
    onResponseLimits: value => { received.push(value); } });
  try {
    const work = search.search({}, AbortSignal.timeout(2000));
    if (status === 200) assert.equal((await work).output, 'PUBLIC_SEARCH');
    else await assert.rejects(work, error => error.code === (status === 401 ? 'UNAUTHENTICATED' : 'UPSTREAM_RETRY_DEFERRED'));
    assert.deepEqual(received, [{ requestAttempt: 1, kind: 'search', httpStatus: status, observation }]);
    assert.equal(search.diagnostics().requestAttempts, 1); checks += 3;
  } finally { await search.close(); upstream.closeAllConnections(); await new Promise(done => upstream.close(done)); }
}
for (const status of [401, 429]) {
  const received = [];
  const upstream = createServer((req, res) => {
    requests++; req.resume(); res.writeHead(status, { ...headers, ...(status === 429 ? { 'Retry-After': '300' } : {}) }); res.end();
  });
  await new Promise(done => upstream.listen(0, '127.0.0.1', done));
  const model = createNativeLoopbackTransport(upstream.address().port, { requestBudget: 2,
    onResponseLimits: value => { received.push(value); } });
  try {
    await assert.rejects(model.send({}, AbortSignal.timeout(2000)), error => error.code === (status === 401 ? 'UNAUTHENTICATED' : 'UPSTREAM_RETRY_DEFERRED'));
    assert.deepEqual(received, [{ requestAttempt: 1, kind: 'responses', httpStatus: status, observation }]);
    assert.equal(model.diagnostics().requestAttempts, 1); checks += 3;
  } finally { await model.close(); upstream.closeAllConnections(); await new Promise(done => upstream.close(done)); }
}
{
  let count = 0;
  const received = [];
  const upstream = createServer((req, res) => {
    requests++; count++; req.resume();
    res.writeHead(200, { 'Content-Type': 'text/event-stream', ...(count === 1 ? headers : count === 2
      ? { ...headers, 'x-codex-primary-used-percent': ['10', '20'] } : {}) }); res.end(wire);
  });
  await new Promise(done => upstream.listen(0, '127.0.0.1', done));
  const model = createNativeLoopbackTransport(upstream.address().port, { requestBudget: 3,
    onResponseLimits: value => { received.push(value); } });
  try {
    for (let index = 0; index < 3; index++) await model.send({}, AbortSignal.timeout(2000));
    assert.deepEqual(received.map(value => value.observation.state), ['observed', 'invalid', 'missing']);
    assert.deepEqual(received.map(value => value.requestAttempt), [1, 2, 3]);
    assert.equal(received[2].observation.primary, null); checks += 3;
  } finally { await model.close(); upstream.closeAllConnections(); await new Promise(done => upstream.close(done)); }
}
{
  let count = 0, first;
  const received = [];
  const upstream = createServer((req, res) => {
    requests++; req.resume();
    if (++count === 1) { first = res; return; }
    res.writeHead(200, { 'Content-Type': 'text/event-stream', ...headers }); res.end(wire);
  });
  await new Promise(done => upstream.listen(0, '127.0.0.1', done));
  const model = createNativeLoopbackTransport(upstream.address().port, { requestBudget: 2,
    onResponseLimits: value => {
      received.push(value);
      if (received.length === 1) { first.writeHead(200, { 'Content-Type': 'text/event-stream' }); first.end(wire); }
    } });
  try {
    await Promise.all([model.send({}, AbortSignal.timeout(2000)), model.send({}, AbortSignal.timeout(2000))]);
    assert.deepEqual(received.map(value => value.requestAttempt), [2, 1]);
    assert.deepEqual(received.map(value => value.observation.state), ['observed', 'missing']); checks += 2;
  } finally { await model.close(); upstream.closeAllConnections(); await new Promise(done => upstream.close(done)); }
}
console.log(JSON.stringify({ suite: 'rate-limit-observation', checks, loopbackRequests: requests,
  productionFactoryOptionCheck: 'BLOCKED_NOT_RUN_DEBUG_RUNTIME_UNSUPPORTED',
  externalRequests: 0, actualCredentialReads: 0 }));
