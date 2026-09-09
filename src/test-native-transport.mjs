import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';

const signal = () => new AbortController().signal;
const frame = value => `event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`;
const complete = frame({ type: 'response.completed' });
const created = frame({ type: 'response.created' });
const good = created + complete + 'data: [DONE]\n\n';

async function fixture(handler, options, run) {
  const server = createServer(handler);
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const transport = createNativeLoopbackTransport(server.address().port, options);
  try { await run(transport); }
  finally {
    await transport.close();
    server.closeAllConnections?.();
    await new Promise(resolve => server.close(resolve));
  }
}

async function testStreamingBeforeEof() {
  let ended;
  let release;
  const first = new Promise(resolve => { release = resolve; });
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(created);
    release();
    setTimeout(() => { ended = true; res.end(complete + 'data: [DONE]\n\n'); }, 40);
  }, {}, async transport => {
    const events = [];
    const done = transport.send({}, signal(), { onEvent: event => { events.push(event); } });
    await first;
    assert.equal(ended, undefined);
    assert.equal(await done, undefined);
    assert.deepEqual(events.map(event => event.type), ['response.created', 'response.completed']);
  });
}

async function testSplitUtf8() {
  await fixture((_req, res) => {
    const wire = frame({ type: 'response.created', text: '합성' }) + complete;
    const bytes = Buffer.from(wire);
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(bytes.subarray(0, bytes.indexOf(Buffer.from('합')) + 1));
    res.end(bytes.subarray(bytes.indexOf(Buffer.from('합')) + 1));
  }, {}, async transport => {
    const events = await transport.send({}, signal());
    assert.equal(events[0].text, '합성');
  });
}

async function testKeepAliveReuse() {
  const sockets = new Set();
  await fixture((req, res) => {
    sockets.add(req.socket);
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(good);
  }, {}, async transport => {
    await transport.send({}, signal()); await transport.send({}, signal());
    assert.equal(sockets.size, 1);
    assert.equal(transport.diagnostics().connectionAttempts, 1);
  });
}

async function testConcurrentResponseBounds() {
  const wire = frame({ type: 'response.created', text: 'x'.repeat(100) }) + complete;
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(wire);
  }, { maxResponseBytes: 256 }, async transport => {
    const results = await Promise.all([transport.send({}, signal()), transport.send({}, signal())]);
    assert.equal(results.length, 2);
    assert.ok(results.every(events => events.length === 2));
  });
}

async function testLargeEventCount() {
  const wire = Array.from({ length: 513 }, (_, index) => frame({ type: 'response.in_progress', value: index })).join('') + complete;
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(wire);
  }, {}, async transport => {
    assert.equal((await transport.send({}, signal())).length, 514);
  });
}

async function testRetryBeforeOutput() {
  let count = 0;
  await fixture((_req, res) => {
    count++;
    if (count < 3) { res.writeHead(503, { 'Retry-After': '0' }); res.end(); return; }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(good);
  }, { retryDelayMs: 0 }, async transport => {
    const retried = [];
    const events = await transport.send({}, signal(), { onRetry: attempt => retried.push(attempt) });
    assert.deepEqual(events.map(event => event.type), ['response.created', 'response.completed']);
    assert.deepEqual(retried, [1, 2]);
    assert.equal(transport.diagnostics().requestAttempts, 3);
    assert.equal(transport.diagnostics().retries, 2);
  });
}

async function testRetryLimitAndPostOutputStop() {
  let count = 0;
  await fixture((_req, res) => { count++; res.writeHead(429); res.end(); }, { retryDelayMs: 0 }, async transport => {
    const attemptTimings = [];
    await assert.rejects(transport.send({}, signal(), { attemptTimings }), error => error.code === 'RATE_LIMITED');
    assert.equal(count, 6);
    assert.equal(transport.diagnostics().retries, 5);
    assert.equal(attemptTimings.length, 6);
    for (const [index, timing] of attemptTimings.entries()) {
      assert.equal(timing.attempt, index + 1); assert.equal(timing.status, 429);
      assert.equal(timing.completed, false);
      assert.ok(timing.endedMs >= timing.headersMs);
      assert.ok(timing.requestFlushedMs >= timing.startedMs);
    }
  });

  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(created);
    setTimeout(() => res.destroy(), 10);
  }, { retryDelayMs: 0 }, async transport => {
    const events = [];
    await assert.rejects(transport.send({}, signal(), { onEvent: event => events.push(event) }),
      error => ['TRUNCATED_STREAM', 'UPSTREAM_IO_ERROR'].includes(error.code));
    assert.deepEqual(events.map(event => event.type), ['response.created']);
    assert.equal(transport.diagnostics().requestAttempts, 1);
    assert.equal(transport.diagnostics().retries, 0);
  });
}

async function testRetryFenceBeforeCommit() {
  let count = 0;
  await fixture((_req, res) => {
    count++;
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    if (count === 1) { res.write(created); setTimeout(() => res.destroy(), 10); return; }
    res.end(good);
  }, { retryDelayMs: 0 }, async transport => {
    const events = [], retried = [];
    const result = await transport.send({}, signal(), {
      onEvent: event => events.push(event),
      canRetry: () => true,
      onRetry: attempt => { retried.push(attempt); events.length = 0; }
    });
    assert.equal(result, undefined);
    assert.deepEqual(retried, [1]);
    assert.deepEqual(events.map(event => event.type), ['response.created', 'response.completed']);
  });
}

async function testAbortClearsRequestAndDelay() {
  await fixture((_req, _res) => {}, { timeoutMs: 1000 }, async transport => {
    const controller = new AbortController();
    const pending = transport.send({}, controller.signal);
    setTimeout(() => controller.abort(), 10);
    await assert.rejects(pending, error => error.code === 'CANCELLED');
    assert.equal(transport.diagnostics().activeRequests, 0);
    assert.equal(transport.diagnostics().activeSockets, 0);
  });

  let count = 0;
  await fixture((_req, res) => { count++; res.writeHead(503); res.end(); }, { retryDelayMs: 1000 }, async transport => {
    const controller = new AbortController();
    const pending = transport.send({}, controller.signal);
    await new Promise(resolve => setTimeout(resolve, 20));
    controller.abort();
    await assert.rejects(pending, error => error.code === 'CANCELLED');
    assert.equal(count, 1);
    assert.equal(transport.diagnostics().retries, 0);
    assert.equal(transport.diagnostics().activeRequests, 0);
  });
}

async function testDoneAndMalformed() {
  await fixture((_req, res) => { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(good); }, {}, async transport => {
    assert.equal((await transport.send({}, signal())).length, 2);
  });
  for (const payload of [
    'data: [DONE]\n\n' + complete,
    complete + 'data: [DONE]\n\ndata: [DONE]\n\n',
    'id: bad\ndata: {"type":"response.created"}\n\n' + complete,
    frame({ type: 'response.created', sequence_number: 0 }) + frame({ type: 'response.completed', sequence_number: 2 })
  ]) await fixture((_req, res) => { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(payload); }, {}, async transport => {
    await assert.rejects(transport.send({}, signal()), error => ['INCOMPLETE_RESPONSE', 'EVENT_AFTER_COMPLETION', 'INVALID_SSE', 'SEQUENCE_MISMATCH'].includes(error.code));
    assert.equal(transport.diagnostics().retries, 0);
  });
}

async function testBounds() {
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end(`data: {"type":"${'x'.repeat(80)}"}\n\n` + complete);
  }, { maxFrameBytes: 32 }, async transport => {
    await assert.rejects(transport.send({}, signal()), error => error.code === 'FRAME_TOO_LARGE');
  });
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end(frame({ type: 'one' }) + frame({ type: 'two' }) + complete);
  }, { maxEvents: 2 }, async transport => {
    await assert.rejects(transport.send({}, signal()), error => error.code === 'TOO_MANY_EVENTS');
  });
  await fixture((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end(good);
  }, { maxResponseBytes: 8 }, async transport => {
    await assert.rejects(transport.send({}, signal()), error => error.code === 'RESPONSE_TOO_LARGE');
  });
}

async function testCredentialRotationAndBinding() {
  const seen = [];
  let count = 0;
  await fixture((req, res) => {
    seen.push(req.headers.authorization);
    count++;
    if (count === 1) { res.writeHead(401); res.end(); return; }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(good);
  }, {
    retryDelayMs: 0,
    credentialSupplier: async ({ force }) => ({ accessToken: force ? 'token-two' : 'token-one', account: 'account-one' })
  }, async transport => {
    await transport.send({}, signal());
    assert.deepEqual(seen, ['Bearer token-one', 'Bearer token-two']);
    assert.ok(!JSON.stringify(transport.diagnostics()).includes('token-'));
  });

  await fixture((_req, res) => { res.writeHead(401); res.end(); }, {
    retryDelayMs: 0,
    credentialSupplier: async ({ force }) => ({ accessToken: force ? 'token-two' : 'token-one', account: force ? 'account-two' : 'account-one' })
  }, async transport => {
    await assert.rejects(transport.send({}, signal()), error => error.code === 'CREDENTIAL_ACCOUNT_MISMATCH');
  });
}

await testStreamingBeforeEof();
await testSplitUtf8();
await testKeepAliveReuse();
await testConcurrentResponseBounds();
await testLargeEventCount();
await testRetryBeforeOutput();
await testRetryLimitAndPostOutputStop();
await testRetryFenceBeforeCommit();
await testAbortClearsRequestAndDelay();
await testDoneAndMalformed();
await testBounds();
await testCredentialRotationAndBinding();
process.stdout.write(JSON.stringify({ suite: 'native-transport', passed: true, externalRequests: 0, credentialReads: 0 }) + '\n');
