import assert from 'node:assert/strict';
import { createServer, Agent } from 'node:http';
import { Socket } from 'node:net';
import { createNativeLoopbackTransport } from './native-transport.mjs';

const signal = () => new AbortController().signal;
const frame = value => `event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`;
const complete = frame({ type: 'response.completed' });
const created = frame({ type: 'response.created' });
const good = created + complete + 'data: [DONE]\n\n';

async function testSocketClosedBeforeObservation() {
  // Each connection gets a distinct actual already-closed Socket. Reusing one
  // closed object across retries would itself accumulate Node HTTP listeners.
  // Request handling, retry limits and transport cleanup run unchanged.
  for (const [method, attempts, code] of [['send', 6, 'UPSTREAM_IO_ERROR'], ['search', 2, 'SEARCH_HTTP_ERROR']]) {
    const sockets = Array.from({ length: attempts }, () => new Socket());
    await Promise.all(sockets.map(socket => new Promise(done => { socket.once('close', done); socket.destroy(); })));
    const original = Agent.prototype.createConnection;
    let connections = 0, timer;
    Agent.prototype.createConnection = () => { assert.ok(connections < sockets.length); return sockets[connections++]; };
    const transport = createNativeLoopbackTransport(12345, { retryDelayMs: 0 });
    const controller = new AbortController();
    try {
      const failed = transport[method]({}, controller.signal).then(() => 'SUCCESS', error => error.code);
      const result = await Promise.race([failed, new Promise(done => { timer = setTimeout(() => done('UNSETTLED'), 500); })]);
      clearTimeout(timer);
      assert.equal(result, code, method);
      const stopped = await Promise.race([transport.close().then(() => true), new Promise(done => { timer = setTimeout(() => done(false), 500); })]);
      assert.equal(stopped, true, method);
      assert.equal(connections, attempts, method);
      assert.equal(transport.diagnostics().requestAttempts, attempts);
      assert.equal(transport.diagnostics().activeRequests, 0); assert.equal(transport.diagnostics().activeSockets, 0);
      assert.ok(sockets.every(socket => socket.closed));
    } finally { clearTimeout(timer); controller.abort(); Agent.prototype.createConnection = original; }
  }
}

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

async function testPostCompletionDiagnostics() {
  const sequenced = frame({ type: 'response.completed', sequence_number: 0 });
  for (const [prefix, trailer, kind, sequence] of [
    [complete, frame({ type: 'rate_limits.updated' }), 'rate_limits.updated', 'unsequenced'],
    [sequenced, frame({ type: 'response.completed', sequence_number: 1 }), 'response.completed', 'expected'],
    [sequenced, frame({ type: 'response.created', sequence_number: 0 }), 'response.created', 'unexpected'],
    [sequenced, frame({ type: 'error', sequence_number: 'SYNTHETIC_PRIVATE' }), 'error', 'invalid'],
    [sequenced, frame({ type: 'ping' }), 'ping', 'missing'],
    [good, 'data: [DONE]\n\n', 'done', null],
    [good, frame({ type: 'SYNTHETIC_PRIVATE', text: 'SYNTHETIC_PRIVATE' }), 'other', 'unsequenced'],
    [complete, 'data: SYNTHETIC_PRIVATE\n\n', 'invalid-json', null],
    [complete, 'data: null\n\n', 'other', 'unsequenced'],
    [complete, frame({ type: 'ping', text: 'x'.repeat(16384) }), 'oversized', null]
  ]) {
    await fixture((_req, res) => {
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.write(prefix + trailer.slice(0, 5));
      setImmediate(() => res.end(trailer.slice(5)));
    }, {}, async transport => {
      const attemptTimings = [], delivered = [];
      await assert.rejects(transport.send({}, signal(), { attemptTimings, onEvent: event => delivered.push(event.type) }),
        error => error.code === 'EVENT_AFTER_COMPLETION');
      assert.equal(attemptTimings.length, 1);
      const timing = attemptTimings[0];
      assert.equal(timing.terminalState, prefix === good ? 'done' : 'completed');
      assert.equal(timing.postCompletionFrame, kind);
      assert.equal(timing.postCompletionSequence, sequence);
      assert.equal(timing.completed, false);
      assert.deepEqual(delivered, prefix === good ? ['response.created', 'response.completed'] : ['response.completed']);
      assert.equal(transport.diagnostics().retries, 0);
      assert.equal(transport.diagnostics().activeSockets, 0);
      assert.ok(!JSON.stringify(attemptTimings).includes('SYNTHETIC_PRIVATE'));
    });
  }
  await fixture((_req, res) => res.end(good + ': keepalive\n\n'), {}, async transport => {
    const attemptTimings = [];
    await transport.send({}, signal(), { attemptTimings });
    assert.equal(attemptTimings[0].terminalState, 'done');
    assert.equal(attemptTimings[0].completed, true);
    assert.equal(attemptTimings[0].postCompletionFrame, null);
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

// The standalone search request: its own path, its own headers, plain JSON, and failures that
// name themselves instead of arriving as an empty success.
async function testStandaloneSearch() {
  const seen = [];
  const handler = (req, res) => {
    let raw = '';
    req.setEncoding('utf8');
    req.on('data', chunk => { raw += chunk; });
    req.on('end', () => {
      seen.push({ url: req.url, method: req.method, headers: req.headers, body: raw });
      const mode = JSON.parse(raw).commands?.search_query?.[0]?.q;
      if (mode === 'UNAUTHORIZED') { res.writeHead(401).end('{}'); return; }
      if (mode === 'BROKEN') { res.writeHead(500).end('{}'); return; }
      if (mode === 'GONE') { res.writeHead(404).end('{}'); return; }
      if (mode === 'FLAKY') {
        if (seen.filter(entry => entry.body.includes('FLAKY')).length === 1) { res.writeHead(503).end('{}'); return; }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ encrypted_output: null, output: 'SYNTHETIC_RECOVERED', results: [] }));
        return;
      }
      if (mode === 'NOT_JSON') { res.writeHead(200, { 'Content-Type': 'application/json' }).end('<html>'); return; }
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ encrypted_output: 'opaque', output: 'SYNTHETIC_DIGEST', results: [] }));
    });
  };
  const ask = q => ({ id: 'ignored', model: 'gpt-5.6-luna', commands: { search_query: [{ q }] } });
  await fixture(handler, {}, async transport => {
    const result = await transport.search(ask('SYNTHETIC_QUERY'), signal());
    assert.equal(result.output, 'SYNTHETIC_DIGEST');
    const [request] = seen;
    assert.equal(request.method, 'POST');
    assert.equal(request.url, '/backend-api/codex/alpha/search');
    assert.equal(request.headers['content-type'], 'application/json');
    assert.equal(request.headers.accept, 'application/json');
    assert.equal(request.headers.authorization, 'Bearer synthetic');
    assert.equal(request.headers['chatgpt-account-id'], 'synthetic');
    assert.equal(request.headers.originator, 'codex_exec');
    // Turn identity travels, and it describes nothing local.
    const metadata = JSON.parse(request.headers['x-codex-turn-metadata']);
    assert.equal(Object.hasOwn(metadata, 'workspaces'), false);
    // The caller's id is replaced by one this transport generated for its own lifetime.
    const sent = JSON.parse(request.body);
    assert.notEqual(sent.id, 'ignored');
    assert.match(sent.id, /^[0-9a-f-]{36}$/);
    assert.deepEqual(sent.commands, { search_query: [{ q: 'SYNTHETIC_QUERY' }] });
    // A second search reuses the same generated session.
    await transport.search(ask('SYNTHETIC_QUERY'), signal());
    assert.equal(JSON.parse(seen[1].body).id, sent.id);
  });
  // One retry, and only for a failure that can pass: a search is an idempotent read.
  await fixture(handler, { retryBaseMs: 0 }, async transport => {
    const result = await transport.search(ask('FLAKY'), signal());
    assert.equal(result.output, 'SYNTHETIC_RECOVERED');
    assert.equal(seen.filter(entry => entry.body.includes('FLAKY')).length, 2);
  });
  for (const [query, code] of [['UNAUTHORIZED', 'UNAUTHENTICATED'], ['BROKEN', 'SEARCH_HTTP_ERROR'],
    ['GONE', 'SEARCH_UNAVAILABLE'], ['NOT_JSON', 'SEARCH_RESPONSE_SHAPE']]) {
    await fixture(handler, { retryBaseMs: 0 }, async transport => {
      const before = seen.length;
      await assert.rejects(() => transport.search(ask(query), signal()), error => error.code === code, query);
      // A vanished endpoint and a rejected credential are not retried; a sick one is, once.
      assert.equal(seen.length - before, query === 'BROKEN' ? 2 : 1, query);
    });
  }
  // A cancelled search reports cancellation, and a closed transport refuses outright.
  await fixture(handler, {}, async transport => {
    const controller = new AbortController();
    controller.abort();
    await assert.rejects(() => transport.search(ask('SYNTHETIC_QUERY'), controller.signal),
      error => error.code === 'CANCELLED');
  });
  const server = createServer(handler);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const transport = createNativeLoopbackTransport(server.address().port);
  await transport.close();
  await assert.rejects(() => transport.search(ask('SYNTHETIC_QUERY'), signal()),
    error => error.code === 'TRANSPORT_CLOSED');
  await new Promise(resolve => server.close(resolve));
}

await testStreamingBeforeEof();
await testSocketClosedBeforeObservation();
await testSplitUtf8();
await testKeepAliveReuse();
await testConcurrentResponseBounds();
await testLargeEventCount();
await testRetryBeforeOutput();
await testRetryLimitAndPostOutputStop();
await testRetryFenceBeforeCommit();
await testAbortClearsRequestAndDelay();
await testDoneAndMalformed();
await testPostCompletionDiagnostics();
await testBounds();
await testCredentialRotationAndBinding();
await testStandaloneSearch();
process.stdout.write(JSON.stringify({ suite: 'native-transport', passed: true, externalRequests: 0, credentialReads: 0 }) + '\n');
