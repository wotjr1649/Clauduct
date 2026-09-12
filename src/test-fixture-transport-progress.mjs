import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { guardFixtureTransport } from '../verification/fixture-tool-policy.mjs';

const until = async predicate => {
  const deadline = performance.now() + 2000;
  while (!predicate() && performance.now() < deadline) await new Promise(done => setTimeout(done, 5));
  assert.ok(predicate(), 'PROGRESS_BOUNDARY_NOT_OBSERVED');
};
let response, releaseCredential, arrivals = 0, events = 0, checks = 0;
const credential = new Promise(done => { releaseCredential = done; });
const server = createServer((_request, current) => { response = current; arrivals++; });
await new Promise(done => server.listen(0, '127.0.0.1', done));
const transport = createNativeLoopbackTransport(server.address().port, { credentialSupplier: () => credential });
const guarded = guardFixtureTransport(transport, { version: 1, kind: 'none', workingRoot: 'SYNTHETIC_WORK' });
const controller = new AbortController();
const running = guarded.send({ public: true }, controller.signal, { onEvent: () => { events++; } });
try {
  assert.equal(guarded.fixtureProgress().active[0].phase, 'credentials'); checks++;
  releaseCredential({ accessToken: 'synthetic', account: 'synthetic' });
  await until(() => arrivals === 1 && guarded.fixtureProgress().active[0].phase === 'headers');
  assert.equal(guarded.fixtureProgress().active[0].status, null); checks++;
  response.writeHead(200, { 'Content-Type': 'text/event-stream' }); response.flushHeaders();
  await until(() => guarded.fixtureProgress().active[0].phase === 'body');
  assert.equal(guarded.fixtureProgress().active[0].status, 200); checks++;
  response.write('event: response.created\ndata: {"type":"response.created","response":{"id":"PUBLIC_RESPONSE"}}\n\n');
  await until(() => events === 1);
  assert.equal(guarded.fixtureProgress().active[0].phase, 'stream');
  assert.equal(guarded.fixtureProgress().active[0].sawCompletion, false); checks++;
  response.write('event: response.completed\ndata: {"type":"response.completed","response":{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}}\n\n');
  await until(() => events === 2);
  assert.equal(guarded.fixtureProgress().active[0].sawCompletion, true);
  assert.equal(guarded.fixtureProgress().active[0].phase, 'stream'); checks++;
  const snapshot = guarded.fixtureProgress();
  assert.deepEqual(Object.keys(snapshot.active[0]).sort(), ['attempt', 'elapsedMs', 'phase', 'request', 'sawCompletion', 'status']);
  assert.equal(snapshot.active[0].attempt, 1); assert.equal(snapshot.active[0].request, 1);
  snapshot.active[0].phase = 'changed';
  assert.equal(guarded.fixtureProgress().active[0].phase, 'stream'); checks++;
  response.end('data: [DONE]\n\n'); await running;
  assert.deepEqual(guarded.fixtureProgress(), { requestsStarted: 1, activeCount: 0, truncated: false, active: [] }); checks++;
  const cancelled = new AbortController();
  const pending = guarded.send({}, cancelled.signal, { onEvent: () => {} }).then(() => 'unexpected', error => error.code);
  await until(() => arrivals === 2 && guarded.fixtureProgress().active[0].phase === 'headers');
  cancelled.abort();
  assert.equal(await pending, 'CANCELLED');
  assert.equal(guarded.fixtureProgress().activeCount, 0); checks++;
  console.log(JSON.stringify({ suite: 'fixture-transport-progress', checks, loopbackRequests: arrivals,
    externalRequests: 0, actualCredentialReads: 0, rawPayloadFields: 0 }));
} finally {
  controller.abort(); await running.catch(() => {});
  await guarded.close(); server.closeAllConnections(); await new Promise(done => server.close(done));
}
