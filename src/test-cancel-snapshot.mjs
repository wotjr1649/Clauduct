import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { EventEmitter } from 'node:events';
import { startNativeGateway } from './native-gateway.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { createNativeResponse, prepareNative, NativeError } from './native-protocol.mjs';
import { requestStatusSnapshot } from './request-status.mjs';
import { runInteractive } from './clauduct.mjs';

const doc = { model: 'sol', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] };
const created = { type: 'response.created', response: { id: 'resp_test', status: 'in_progress' } };
const mismatch = { type: 'response.in_progress', response_id: 'wrong' };
const frame = event => `data: ${JSON.stringify(event)}\n\n`;
const defer = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
let checks = 0, phase = 'startup';
const watchdog = setTimeout(() => {
  console.error(`CANCEL_SNAPSHOT_TEST_TIMEOUT ${JSON.stringify({ phase, checks })}`); process.exit(1);
}, 15000);
try {
  // Real downstream disconnect, deterministic ordering at the protocol callback boundary.
  for (const order of ['cancel-first', 'mismatch-first']) {
    phase = `${order}-setup`;
    const ready = defer(), cancelled = defer();
    const gateway = await startNativeGateway({ admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
      send: async (_body, signal, { onEvent }) => {
        phase = `${order}-send`;
        signal.addEventListener('abort', cancelled.resolve, { once: true });
        await onEvent(created);
        if (order === 'cancel-first') {
          ready.resolve(); await cancelled.promise;
          await onEvent(mismatch); // Late callback must not parse after cancellation.
        } else {
          try { await onEvent(mismatch); }
          catch (error) { ready.resolve(); await cancelled.promise; throw error; }
        }
      }, close: async () => {}, diagnostics: () => ({ activeSockets: 0, activeRequests: 0 })
    } });
    const controller = new AbortController();
    const pending = new Promise(resolve => {
      const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
        agent: false, signal: controller.signal, headers: { ...gateway.clientHeaders(),
          'anthropic-version': '2023-06-01', 'content-type': 'application/json' } }, res => {
        res.resume(); res.on('error', resolve); res.on('end', resolve);
      });
      req.on('error', resolve); req.end(JSON.stringify(doc));
    });
    try {
      phase = `${order}-ready`; await ready.promise;
      phase = `${order}-abort`; controller.abort(); await cancelled.promise;
      phase = `${order}-client`; await pending;
      // Close joins the in-flight handle without creating another request.
      phase = `${order}-close`;
      await gateway.close();
      const row = requestStatusSnapshot(gateway.diagnostics()).recentRequests.at(-1);
      assert.equal(row.failureCategory, order === 'cancel-first' ? 'CANCELLED' : 'SNAPSHOT_MISMATCH');
      assert.equal(row.clientDisconnected, true);
      assert.equal(typeof row.clientDisconnectedMs, 'number');
      assert.equal(row.success, false);
      if (order === 'cancel-first') {
        assert.equal(row.snapshotMismatchMs, null); assert.equal(row.snapshotMismatchPhase, null);
      } else {
        assert.equal(row.snapshotMismatchPhase, 'stream');
        assert.ok(row.snapshotMismatchMs <= row.clientDisconnectedMs);
      }
      checks++;
    } finally { controller.abort(); await gateway.close(); }
  }

  // Real HTTP/SSE transport must not replace an already-raised validation error with cancellation.
  phase = 'http-transport';
  const server = createServer((req, res) => { req.resume(); res.end(frame(created) + frame(mismatch)); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const transport = createNativeLoopbackTransport(server.address().port);
  try {
    const controller = new AbortController(), parser = createNativeResponse(prepareNative(doc));
    await assert.rejects(transport.send({}, controller.signal, { onEvent: event => {
      try { parser.push(event); } catch (error) { controller.abort(); throw error; }
    } }), error => error instanceof NativeError && error.code === 'SNAPSHOT_MISMATCH');
    assert.equal(transport.diagnostics().retries, 0);
    assert.equal(transport.diagnostics().activeSockets, 0);
    checks++;
  } finally { await transport.close(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }

  // Snapshot after native child exit is a pure projection, not a loopback/model request.
  phase = 'snapshot';
  let sends = 0;
  const gateway = await startNativeGateway({ transport: {
    send: async () => { sends++; throw new Error('UNEXPECTED_SEND'); }, close: async () => {},
    diagnostics: () => ({ activeSockets: 0, activeRequests: 0, requestAttempts: 0, accessToken: 'SYNTHETIC_PRIVATE' })
  } });
  const contextEnv = { CLAUDE_CODE_MAX_CONTEXT_TOKENS: '400000', CLAUDE_CODE_AUTO_COMPACT_WINDOW: '400000',
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK: '1',
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '84.21052631578947' };
  const result = await runInteractive(gateway, () => {
    const child = new EventEmitter(); child.kill = () => {};
    setImmediate(() => child.emit('close', 0)); return child;
  }, { contextEnv });
  assert.equal(result.category, 'SUCCESS'); assert.equal(result.resourcesClosed, true);
  assert.equal(result.requestStatus.lifetime.started, 0);
  assert.equal(result.requestStatus.requestOutcome, 'no-requests');
  assert.equal(result.requestStatus.clientExecutionPolicy.nonStreamingFallbackDisabled, true);
  assert.deepEqual(result.requestStatus.failureHistory.records, []);
  assert.deepEqual(result.requestStatus.recentRequests, []);
  assert.equal(result.requestStatus.clientContextPolicy.window, 400000);
  assert.equal(sends, 0); assert.equal(gateway.diagnostics().requests, 0);
  assert.ok(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE'));
  const filtered = requestStatusSnapshot({ recentRequests: [{ clientDisconnectedMs: -1,
    snapshotMismatchMs: 'SYNTHETIC_PRIVATE', snapshotMismatchPhase: 'SYNTHETIC_PRIVATE' }] });
  for (const key of ['clientDisconnectedMs', 'snapshotMismatchMs', 'snapshotMismatchPhase']) {
    assert.equal(filtered.recentRequests[0][key], null);
  }
  checks++;
  // Successful keep-alive traffic must be fully closed before native exit returns.
  phase = 'keep-alive';
  const keepAliveServer = createServer((req, res) => {
    req.resume(); res.end(frame({ type: 'response.completed' }));
  });
  await new Promise(resolve => keepAliveServer.listen(0, '127.0.0.1', resolve));
  const keepAliveTransport = createNativeLoopbackTransport(keepAliveServer.address().port);
  const keepAliveGateway = await startNativeGateway({ transport: keepAliveTransport });
  try {
    await keepAliveTransport.send({}, AbortSignal.timeout(5000));
    assert.equal(keepAliveTransport.diagnostics().activeSockets, 1);
    const exit = await runInteractive(keepAliveGateway, () => {
      const child = new EventEmitter(); child.kill = () => {};
      setImmediate(() => child.emit('close', 0)); return child;
    });
    assert.equal(exit.category, 'SUCCESS');
    assert.equal(exit.resourcesClosed, true);
    assert.ok(Object.values(exit.requestStatus.cleanup).every(value => value === true));
    assert.equal(keepAliveTransport.diagnostics().activeSockets, 0);
    const closed = await Promise.all([keepAliveTransport.close(), keepAliveTransport.close()]);
    assert.ok(closed.every(state => state.activeSockets === 0 && state.activeRequests === 0));
    checks++;
  } finally {
    await keepAliveGateway.close(); keepAliveServer.closeAllConnections();
    await new Promise(resolve => keepAliveServer.close(resolve));
  }
  // Real cleanup rejection stays a failure; diagnostics expose fixed booleans, not error text.
  phase = 'cleanup-rejection';
  const failedGateway = await startNativeGateway({ transport: {
    send: async () => { throw new Error('UNEXPECTED_SEND'); },
    close: async () => { throw new Error('SYNTHETIC_PRIVATE'); },
    diagnostics: () => ({ activeSockets: 1, activeRequests: 0 })
  } });
  const failedExit = await runInteractive(failedGateway, () => {
    const child = new EventEmitter(); child.kill = () => {};
    setImmediate(() => child.emit('close', 0)); return child;
  });
  assert.equal(failedExit.category, 'CLEANUP_FAILED');
  assert.equal(failedExit.resourcesClosed, false);
  assert.equal(failedExit.requestStatus.cleanup.gatewayCleanupCompleted, false);
  assert.equal(failedExit.requestStatus.cleanup.transportSocketsClosed, false);
  assert.equal(failedExit.requestStatus.cleanup.childClosed, true);
  assert.ok(!JSON.stringify(failedExit).includes('SYNTHETIC_PRIVATE'));
  checks++;
} finally { clearTimeout(watchdog); }
console.log(JSON.stringify({ suite: 'cancel-snapshot', checks, passed: true, externalRequests: 0, credentialReads: 0, actualClaudeExecutions: 0 }));
